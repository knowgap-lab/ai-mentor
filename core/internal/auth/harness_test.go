package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	RejectDevMode(env string, enabled bool) error
//	Open(ctx, Config) (*Service, error)
//	(*Service).Close() error
//	(*Service).Handler() http.Handler
//	(*Service).Authorize(Actor, action string, Object) error
//	(*Service).Remember(userID, key string, response []byte) ([]byte, error)
//
// Config: DSN, Env, DevMode (это AUTH_DEV_MODE), AppOrigin (в тестах http://localhost), Log.
// Actor и Object содержат UserID; у Actor есть Role student или tutor.
// Object в этом пакете — ученик.
// Отказ Authorize — *Error: HTTPStatus, Code, Body() с JSON контракта Error.
// Повтор Remember с тем же пользователем и ключом возвращает первый ответ.

// Вопросы, которые тесты сознательно не закрывают.
//
// ВОПРОС: что Remember возвращает при первом сохранении, когда прежнего ответа нет? Проверяется только повторный вызов.
// ВОПРОС: один telegram_id с ролями student и tutor — два пользователя или отказ? Бриф ключует пару, AU-R16 (пакет G5) говорит об одном пользователе. Смену роли не проверяю.
// ВОПРОС: в момент ровно expires_at сеанс ещё действует? Проверяю срок при создании и явно истёкший сеанс.
// ВОПРОС: SHA-256 в core.session считается от 32 сырых байт или от hex-строки cookie? Годится любой из этих двух.
// ВОПРОС: POST /students/{id}/revoke-sessions и автоматический отзыв (отвязка Telegram, перевыпуск приглашения, согласие, архив, обезличивание) в раздел «Поведение» этого брифа не входят. Не проверяю.
// ВОПРОС: какой status получает пользователь, созданный через POST /auth/dev? Значение не проверяю.
// ВОПРОС: выключенный локальный вход и чужой Origin одновременно — код forbidden или csrf? Не проверяю.
// ВОПРОС: какое тело у POST /auth/dev без role или с ролью вне student|tutor? У loginDev в контракте нет кода для плохого тела. Не проверяю.

const (
	appOrigin  = "http://localhost"
	cookieName = "__Host-session"
	pathDev    = "/auth/dev"
	pathLogout = "/auth/logout"
	pathMe     = "/me"

	studentTTL = 90 * 24 * time.Hour
	tutorTTL   = 7 * 24 * time.Hour
	ttlSlack   = 5 * time.Second
)

func demoID(n int) string {
	return fmt.Sprintf("tg-demo-%d", n)
}

type meBody struct {
	UserID string
	Role   string
}

type loggedIn struct {
	Me    meBody
	Token string
}

type dbSession struct {
	Hash    []byte
	UserID  string
	Created time.Time
	Expires time.Time
	Revoked *time.Time
}

type harness struct {
	t    *testing.T
	svc  *auth.Service
	dsn  string
	logs *bytes.Buffer
}

func newHarness(t *testing.T, env string, dev bool) *harness {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	return openHarness(t, dsn, env, dev)
}

func openHarness(t *testing.T, dsn, env string, dev bool) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	log.SetOutput(logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	svc, err := auth.Open(context.Background(), auth.Config{
		DSN:       dsn,
		Env:       env,
		DevMode:   dev,
		AppOrigin: appOrigin,
		Log:       logs,
	})
	if err != nil {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		t.Fatalf("Open: %v", err)
	}
	h := &harness{t: t, svc: svc, dsn: dsn, logs: logs}
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		if err := svc.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return h
}

func (h *harness) call(method, path string, body []byte, origin, csrf *string, cookie *http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != nil {
		req.Header.Set("Origin", *origin)
	}
	if csrf != nil {
		req.Header.Set("X-CSRF-Token", *csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *harness) loginOK(telegramID, role string) loggedIn {
	h.t.Helper()
	rec := h.call(http.MethodPost, pathDev, devJSON(h.t, telegramID, role), ptr(appOrigin), nil, nil)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("POST /auth/dev %s %s = %d %s", telegramID, role, rec.Code, rec.Body.Bytes())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		h.t.Fatalf("Content-Type = %q", ct)
	}
	token, _ := sessionToken(h.t, rec)
	me := decodeMe(h.t, rec.Body.Bytes())
	if me.UserID == "" || me.Role != role {
		h.t.Fatalf("Me = %+v, роль %s", me, role)
	}
	if csrf := rec.Header().Get("X-CSRF-Token"); csrf != token {
		h.t.Fatal("X-CSRF-Token не равен токену сеанса")
	}
	if strings.Contains(rec.Body.String(), token) {
		h.t.Fatal("токен сеанса попал в тело Me")
	}
	return loggedIn{Me: me, Token: token}
}

func (h *harness) expectMe(in loggedIn) {
	h.t.Helper()
	rec := h.call(http.MethodGet, pathMe, nil, nil, nil, sessionCookie(in.Token))
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET /me = %d %s", rec.Code, rec.Body.Bytes())
	}
	me := decodeMe(h.t, rec.Body.Bytes())
	if me != in.Me {
		h.t.Fatalf("GET /me = %+v, после входа %+v", me, in.Me)
	}
}

func (h *harness) expectUnauthorized(method, path string, cookie *http.Cookie) {
	h.t.Helper()
	rec := h.call(method, path, nil, ptr(appOrigin), ptr("csrf-demo-unused"), cookie)
	expectAPI(h.t, rec, http.StatusUnauthorized, "unauthorized")
}

func devJSON(t *testing.T, telegramID, role string) []byte {
	t.Helper()
	body, err := json.Marshal(struct {
		TelegramID string `json:"telegram_id"`
		Role       string `json:"role"`
	}{TelegramID: telegramID, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func decodeMe(t *testing.T, body []byte) meBody {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var raw struct {
		UserID      string  `json:"user_id"`
		Role        string  `json:"role"`
		DisplayName *string `json:"display_name"`
	}
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("Me: %v тело %s", err, body)
	}
	return meBody{UserID: raw.UserID, Role: raw.Role}
}

func decodeErr(t *testing.T, body []byte) string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var raw struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("Error: %v тело %s", err, body)
	}
	if raw.Code == "" {
		t.Fatalf("в теле нет code: %s", body)
	}
	return raw.Code
}

func expectAPI(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
	if got := decodeErr(t, rec.Body.Bytes()); got != code {
		t.Fatalf("code = %s, нужен %s", got, code)
	}
}

func sessionCookie(token string) *http.Cookie {
	return &http.Cookie{Name: cookieName, Value: token}
}

func sessionToken(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var found string
	var raw string
	for _, line := range rec.Header().Values("Set-Cookie") {
		cookie, err := parseSetCookie(line)
		if err != nil || cookie.Name != cookieName {
			continue
		}
		if found != "" {
			t.Fatal("в ответе две cookie __Host-session")
		}
		found = cookie.Value
		raw = line
	}
	if found == "" {
		t.Fatal("нет cookie __Host-session")
	}
	if _, err := decodeToken(found); err != nil {
		t.Fatalf("токен сеанса: %v", err)
	}
	return found, raw
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, line := range rec.Header().Values("Set-Cookie") {
		cookie, err := parseSetCookie(line)
		if err == nil && cookie.Name == cookieName {
			return true
		}
	}
	return false
}

type wireCookie struct {
	Name  string
	Value string
	flags map[string]string
}

func parseSetCookie(raw string) (wireCookie, error) {
	parts := strings.Split(raw, ";")
	name, value, ok := strings.Cut(strings.TrimSpace(parts[0]), "=")
	if !ok || name == "" {
		return wireCookie{}, errors.New("нет пары имя=значение")
	}
	cookie := wireCookie{Name: name, Value: value, flags: map[string]string{}}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, attr, hasValue := strings.Cut(part, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !hasValue {
			cookie.flags[key] = ""
			continue
		}
		cookie.flags[key] = strings.Trim(strings.TrimSpace(attr), `"`)
	}
	return cookie, nil
}

func decodeToken(token string) ([]byte, error) {
	raw, err := hex.DecodeString(token)
	if err != nil {
		return nil, err
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("длина %d байт, нужны 32", len(raw))
	}
	return raw, nil
}

func tokenIsHash(hash []byte, token string) bool {
	raw, err := decodeToken(token)
	if err != nil || len(hash) != sha256.Size {
		return false
	}
	sumRaw := sha256.Sum256(raw)
	sumHex := sha256.Sum256([]byte(token))
	return bytes.Equal(hash, sumRaw[:]) || bytes.Equal(hash, sumHex[:])
}

func (h *harness) withConn(fn func(context.Context, *pgx.Conn) error) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, h.dsn)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			h.t.Errorf("close db: %v", closeErr)
		}
	}()
	if err := fn(ctx, conn); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, sql, args...)
		return err
	})
}

func (h *harness) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, sql, args...).Scan(&n)
	})
	return n
}

func (h *harness) userRow(id string) (role string, owner, status *string) {
	h.t.Helper()
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx,
			`SELECT role, owner_tutor_id, status FROM core.app_user WHERE id = $1`, id,
		).Scan(&role, &owner, &status)
	})
	return role, owner, status
}

// assertNoRawToken проверяет, что сырой токен не записан ни в один текстовый
// или bytea-столбец таблиц этого пакета. Хеш SHA-256 сам по себе не равен токену.
func (h *harness) assertNoRawToken(token string, raw []byte) {
	h.t.Helper()
	type column struct {
		table string
		name  string
		udt   string
	}
	var cols []column
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT table_name, column_name, udt_name
			FROM information_schema.columns
			WHERE table_schema = 'core'
			  AND table_name IN ('session', 'app_user', 'user_pii')
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var col column
			if err := rows.Scan(&col.table, &col.name, &col.udt); err != nil {
				return err
			}
			cols = append(cols, col)
		}
		return rows.Err()
	})
	ident := regexp.MustCompile(`^[a-z_]+$`)
	for _, col := range cols {
		if !ident.MatchString(col.table) || !ident.MatchString(col.name) {
			h.t.Fatalf("неожиданное имя столбца %s.%s", col.table, col.name)
		}
		switch col.udt {
		case "text", "varchar", "bpchar":
			//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
			n := h.count(fmt.Sprintf(`SELECT count(*) FROM core.%s WHERE %s::text = $1`, col.table, col.name), token)
			if n != 0 {
				h.t.Fatalf("сырой токен записан в core.%s.%s", col.table, col.name)
			}
		case "bytea":
			//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
			n := h.count(fmt.Sprintf(`SELECT count(*) FROM core.%s WHERE %s = $1`, col.table, col.name), raw)
			if n != 0 {
				h.t.Fatalf("сырые байты токена записаны в core.%s.%s", col.table, col.name)
			}
		}
	}
}

func (h *harness) sessionsFor(userID string) []dbSession {
	h.t.Helper()
	var rowsOut []dbSession
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT token_hash, user_id, created_at, expires_at, revoked_at
			FROM core.session
			WHERE user_id = $1
			ORDER BY created_at
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row dbSession
			if err := rows.Scan(&row.Hash, &row.UserID, &row.Created, &row.Expires, &row.Revoked); err != nil {
				return err
			}
			rowsOut = append(rowsOut, row)
		}
		return rows.Err()
	})
	return rowsOut
}

func (h *harness) sessionByToken(token string) dbSession {
	h.t.Helper()
	var found dbSession
	var ok bool
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT token_hash, user_id, created_at, expires_at, revoked_at
			FROM core.session
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row dbSession
			if err := rows.Scan(&row.Hash, &row.UserID, &row.Created, &row.Expires, &row.Revoked); err != nil {
				return err
			}
			if tokenIsHash(row.Hash, token) {
				if ok {
					return errors.New("токену соответствуют две строки core.session")
				}
				found = row
				ok = true
			}
		}
		return rows.Err()
	})
	if !ok {
		h.t.Fatal("сеанс по токену не найден или хеш не SHA-256 токена")
	}
	return found
}

func (h *harness) setCreatedAgo(token string, age time.Duration) {
	h.t.Helper()
	row := h.sessionByToken(token)
	secs := int64(age / time.Second)
	h.exec(`UPDATE core.session SET created_at = now() - $1 * interval '1 second' WHERE token_hash = $2`, secs, row.Hash)
}

func (h *harness) loginAged(telegramID, role string, n int) []loggedIn {
	h.t.Helper()
	out := make([]loggedIn, n)
	for i := 0; i < n; i++ {
		out[i] = h.loginOK(telegramID, role)
		h.setCreatedAgo(out[i].Token, time.Duration(n-i)*time.Minute)
	}
	return out
}

func ownerEmpty(owner *string) bool {
	return owner == nil || *owner == ""
}

func requireForbidden(t *testing.T, err error) *auth.Error {
	t.Helper()
	var api *auth.Error
	if !errors.As(err, &api) || api == nil {
		t.Fatalf("ожидалась *auth.Error, получено %v", err)
	}
	if api.HTTPStatus != http.StatusForbidden || api.Code != "forbidden" {
		t.Fatalf("status=%d code=%s", api.HTTPStatus, api.Code)
	}
	if got := decodeErr(t, api.Body()); got != "forbidden" {
		t.Fatalf("тело code=%s", got)
	}
	return api
}

func sameBody(t *testing.T, a, b *auth.Error) {
	t.Helper()
	if canonicalJSON(t, a.Body()) != canonicalJSON(t, b.Body()) {
		t.Fatalf("тела различаются: %s и %s", a.Body(), b.Body())
	}
}

func bodyHides(t *testing.T, body []byte, ids ...string) {
	t.Helper()
	text := string(body)
	for _, id := range ids {
		if id != "" && strings.Contains(text, id) {
			t.Fatal("тело раскрывает, существует ли ученик")
		}
	}
}

func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("json: %v тело %s", err, raw)
	}
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func sameDecision(t *testing.T, readErr, writeErr error) {
	t.Helper()
	if (readErr == nil) != (writeErr == nil) {
		t.Fatalf("read и write разошлись: %v и %v", readErr, writeErr)
	}
	if readErr == nil {
		return
	}
	sameBody(t, requireForbidden(t, readErr), requireForbidden(t, writeErr))
}

func ptr[T any](v T) *T { return &v }

func isPermission(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}

var (
	migrateOnce sync.Once
	migrateExe  string
	migrateErr  error
)

func migrateUp(t *testing.T, dsn string) {
	t.Helper()
	exe := migrateBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = moduleDir(t)
	cmd.Env = append(withoutEnv(os.Environ(), "DATABASE_URL"), "DATABASE_URL="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
}

func migrateBinary(t *testing.T) string {
	t.Helper()
	migrateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ai-mentor-migrate-")
		if err != nil {
			migrateErr = err
			return
		}
		exe := filepath.Join(dir, "migrate")
		mod, err := findModule()
		if err != nil {
			migrateErr = err
			return
		}
		cmd := exec.Command("go", "build", "-mod=readonly", "-o", exe, "./cmd/migrate")
		cmd.Dir = mod
		out, err := cmd.CombinedOutput()
		if err != nil {
			migrateErr = fmt.Errorf("%w\n%s", err, out)
			return
		}
		migrateExe = exe
	})
	if migrateErr != nil {
		t.Fatal(migrateErr)
	}
	return migrateExe
}

func moduleDir(t *testing.T) string {
	t.Helper()
	dir, err := findModule()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func findModule() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		main := filepath.Join(dir, "cmd", "migrate", "main.go")
		if isFile(mod) && isFile(main) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("модуль core не найден")
		}
		dir = parent
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
