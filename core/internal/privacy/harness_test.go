package privacy_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/privacy"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Open(ctx, Config) (*Service, error)
//	(*Service).Close() error
//	(*Service).Handler() http.Handler
//	(*Service).CreateLink(ctx, studentID, kind string, now time.Time) (token string, error)
//
// Config: DSN и Log. В Log токен ссылки не пишется.
// now — часы CreateLink, в том числе в прошлом: created_at = now,
// expires_at = now + 14 суток, возраст считается по birth_date на now.
// token — значение пути /consent/{token}. Это 32 байта в hex, как у сеанса,
// либо в base64url. В core.consent_link лежит только SHA-256, без сырого токена.
// Отказ CreateLink — *Error: HTTPStatus, Code и Body() с JSON схемы Error.
// GET и POST /consent/{token} идут без сеанса и без CSRF (security: []).

// Вопросы, которые тесты сознательно не закрывают.
//
// ВОПРОС: бриф пишет тело {"action":"grant"} и повторный grant. Контракт core-api.yaml: give, revoke, cancel_revoke. Тесты шлют give и для отмены cancel_revoke.
// ВОПРОС: в таблице брифа core.consent.state = active. В ConsentPage контракта такого значения нет, там given. Тест ждёт в базе active, в JSON given.
// ВОПРОС: в момент ровно expires_at ссылка ещё действует? Проверяю 15-е сутки, не саму границу.
// ВОПРОС: SHA-256 в token_hash считается от 32 сырых байт или от строки токена в пути? Годится любой из этих двух.
// ВОПРОС: как 32 байта кодируются в {token}? Не задано. Тест принимает hex и base64url.
// ВОПРОС: первичный ключ core.student_pii в спецификации не назван. Тест пишет столбец student_id и поля display_name, telegram_id, birth_date.
// ВОПРОС: нужна ли строка ученика кроме core.student_pii? Бриф для этих тестов называет student_pii. Другую таблицу ученика не создаю.
// ВОПРОС: PV-R8 хранит, кто дал согласие, и способ link_button. В таблице брифа этих столбцов нет. Не проверяю.
// ВОПРОС: PV-R17 требует закрытый вход, отзыв всех сеансов и сообщение репетитору. Этих таблиц и связи student-demo с сеансом бриф не задаёт. Проверяю revoked_at, anonymise_at и state.
// ВОПРОС: PV-R18 говорит, что вход вернулся. По той же причине проверяю только state active и пустые revoked_at и anonymise_at.
// ВОПРОС: какой код, если отменить отзыв после 7 суток? В таблице ошибок спецификации кода нет. Не проверяю.
// ВОПРОС: PV-R13 отдельным тестом не закрыт. Запросы /consent/{token} идут без сеанса и без личности нажавшего.
// ВОПРОС: GET отдаёт текст legal/consent-1.md целиком или без шапки? Проверяю text_version "1" и заголовок «Согласие на обработку данных ученика».
// ВОПРОС: отмена отзыва меняет granted_at? Не проверяю.
// ВОПРОС: формат времени в JSON контрактом не задан (type: string). Тест принимает RFC3339.
// ВОПРОС: за сутки до 18-летия ссылка parent ещё разрешена? Пример проверяет ровно 18 лет. Канун не проверяю.

const (
	studentID    = "student-demo"
	displayName  = "demo-nick"
	telegramID   = "tg-demo-1"
	consentTitle = "Согласие на обработку данных ученика"
	textVersion  = "1"

	kindParent  = "parent"
	kindStudent = "student"

	actionGive         = "give"
	actionRevoke       = "revoke"
	actionCancelRevoke = "cancel_revoke"

	apiNone    = "none"
	apiGiven   = "given"
	apiRevoked = "revoked"
	dbActive   = "active"
	dbRevoked  = "revoked"

	day   = 24 * time.Hour
	slack = 5 * time.Second
)

type harness struct {
	t         *testing.T
	svc       *privacy.Service
	dsn       string
	logs      *bytes.Buffer
	birthDate string
	at16      time.Time
	at18      time.Time
}

type linkRow struct {
	Hash    []byte
	Student string
	Kind    string
	Expires time.Time
	Created time.Time
}

type consentRow struct {
	Version   int
	State     string
	Granted   time.Time
	Revoked   *time.Time
	Anonymise *time.Time
}

type consentPage struct {
	TextVersion string  `json:"text_version"`
	Text        string  `json:"text"`
	Kind        string  `json:"kind"`
	State       string  `json:"state"`
	AnonymiseAt *string `json:"anonymise_at"`
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	h := openHarness(t, dsn)
	h.seedStudent()
	return h
}

func openHarness(t *testing.T, dsn string) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	log.SetOutput(logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	svc, err := privacy.Open(context.Background(), privacy.Config{
		DSN: dsn,
		Log: logs,
	})
	if err != nil {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		t.Fatalf("Open: %v", err)
	}
	birth, at16, at18 := birthAndClocks()
	h := &harness{
		t: t, svc: svc, dsn: dsn, logs: logs,
		birthDate: birth, at16: at16, at18: at18,
	}
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		if err := svc.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return h
}

func birthAndClocks() (date string, at16, at18 time.Time) {
	todayNoon := time.Now().UTC().Truncate(day).Add(12 * time.Hour)
	born := todayNoon.AddDate(-16, 0, 0)
	date = born.Format("2006-01-02")
	at16 = time.Date(born.Year()+16, born.Month(), born.Day(), 12, 0, 0, 0, time.UTC)
	at18 = time.Date(born.Year()+18, born.Month(), born.Day(), 12, 0, 0, 0, time.UTC)
	return date, at16, at18
}

func (h *harness) seedStudent() {
	h.t.Helper()
	h.exec(`
		INSERT INTO core.student_pii (student_id, display_name, telegram_id, birth_date)
		VALUES ($1, $2, $3, $4::date)
	`, studentID, displayName, telegramID, h.birthDate)
}

func (h *harness) createLink(kind string, now time.Time) string {
	h.t.Helper()
	token, err := h.svc.CreateLink(context.Background(), studentID, kind, now)
	if err != nil || token == "" {
		h.t.Fatalf("CreateLink %s: %v", kind, err)
	}
	h.guardToken(token)
	h.requireLink(token, kind, now)
	return token
}

func (h *harness) refuseLink(kind string, now time.Time) {
	h.t.Helper()
	before := h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, studentID)
	token, err := h.svc.CreateLink(context.Background(), studentID, kind, now)
	if token != "" {
		h.t.Fatal("отказ вернул токен")
	}
	var api *privacy.Error
	if !errors.As(err, &api) || api == nil {
		h.t.Fatalf("ожидалась *privacy.Error, получено %v", err)
	}
	if api.HTTPStatus != http.StatusUnprocessableEntity || api.Code != "consent_kind_invalid" {
		h.t.Fatalf("status=%d code=%s", api.HTTPStatus, api.Code)
	}
	if got := decodeErr(h.t, api.Body(), ""); got != "consent_kind_invalid" {
		h.t.Fatalf("тело code=%s", got)
	}
	if after := h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, studentID); after != before {
		h.t.Fatal("отказ записал ссылку")
	}
}

func (h *harness) grantParent() string {
	h.t.Helper()
	token := h.createLink(kindParent, time.Now())
	start := time.Now()
	rec := h.call(http.MethodPost, token, actionGive)
	end := time.Now()
	page := h.expectPage(rec, token, kindParent, apiGiven)
	if page.AnonymiseAt != nil {
		h.t.Fatal("выдача согласия заполнила anonymise_at")
	}
	h.expectGranted(start, end)
	return token
}

func (h *harness) revoke(token string) {
	h.t.Helper()
	start := time.Now()
	rec := h.call(http.MethodPost, token, actionRevoke)
	end := time.Now()
	page := h.expectPage(rec, token, kindParent, apiRevoked)
	expectAnonymise(h.t, page, start, end, token)
	h.expectRevoked(start, end)
}

func (h *harness) backdateRevoke() {
	h.t.Helper()
	h.exec(`
		UPDATE core.consent
		SET revoked_at = now() - interval '3 days',
		    anonymise_at = now() + interval '4 days'
		WHERE student_id = $1
	`, studentID)
	row := h.consentRow()
	if row.Revoked == nil || row.Anonymise == nil {
		h.t.Fatal("не удалось сдвинуть отзыв на 3-и сутки")
	}
	age := time.Since(*row.Revoked)
	if age < 3*day-time.Minute || age > 3*day+time.Minute {
		h.t.Fatalf("revoked_at не на 3-и сутки, прошло %s", age.Round(time.Second))
	}
}

func (h *harness) call(method, token, action string) *httptest.ResponseRecorder {
	h.t.Helper()
	path := "/consent/" + url.PathEscape(token)
	var reader io.Reader
	if action != "" {
		body, err := json.Marshal(struct {
			Action string `json:"action"`
		}{Action: action})
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if action != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	h.guardToken(token)
	if strings.Contains(rec.Body.String(), token) {
		h.t.Fatal("тело ответа содержит токен")
	}
	return rec
}

func (h *harness) expectPage(rec *httptest.ResponseRecorder, token, kind, state string) consentPage {
	h.t.Helper()
	if rec.Code != http.StatusOK {
		h.t.Fatalf("статус %d, нужен 200, тело %s", rec.Code, scrub(rec.Body.String(), token))
	}
	page := decodePage(h.t, rec.Body.Bytes(), token)
	if page.TextVersion != textVersion {
		h.t.Fatalf("text_version = %q, нужна %s", page.TextVersion, textVersion)
	}
	if !strings.Contains(page.Text, consentTitle) {
		h.t.Fatal("текст страницы без заголовка текущей версии согласия")
	}
	if page.Kind != kind || page.State != state {
		h.t.Fatalf("kind=%s state=%s, нужны %s и %s", page.Kind, page.State, kind, state)
	}
	return page
}

func (h *harness) expectGone(rec *httptest.ResponseRecorder, token string) {
	h.t.Helper()
	h.expectCode(rec, token, http.StatusGone, "consent_link_gone")
}

func (h *harness) expectCode(rec *httptest.ResponseRecorder, token string, status int, code string) {
	h.t.Helper()
	if rec.Code != status {
		h.t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, scrub(rec.Body.String(), token))
	}
	if got := decodeErr(h.t, rec.Body.Bytes(), token); got != code {
		h.t.Fatalf("code = %s, нужен %s", got, code)
	}
}

func (h *harness) expectGranted(start, end time.Time) {
	h.t.Helper()
	row := h.expectActiveCleared()
	within(h.t, row.Granted, start, end, 0, slack, "granted_at")
}

func (h *harness) expectActiveCleared() consentRow {
	h.t.Helper()
	row := h.consentRow()
	if row.Version != 1 || row.State != dbActive {
		h.t.Fatalf("consent version=%d state=%s", row.Version, row.State)
	}
	if row.Revoked != nil || row.Anonymise != nil {
		h.t.Fatal("у действующего согласия заполнены revoked_at или anonymise_at")
	}
	return row
}

func (h *harness) expectRevoked(start, end time.Time) {
	h.t.Helper()
	row := h.consentRow()
	if row.Version != 1 || row.State != dbRevoked {
		h.t.Fatalf("consent version=%d state=%s", row.Version, row.State)
	}
	if row.Revoked == nil || row.Anonymise == nil {
		h.t.Fatal("после отзыва нет revoked_at или anonymise_at")
	}
	within(h.t, *row.Revoked, start, end, 0, slack, "revoked_at")
	within(h.t, *row.Anonymise, start, end, 7*day, slack, "anonymise_at")
}

func (h *harness) requireLink(token, kind string, created time.Time) {
	h.t.Helper()
	row := h.linkByToken(token)
	if row.Student != studentID || row.Kind != kind {
		h.t.Fatalf("ссылка ученик=%s вид=%s", row.Student, row.Kind)
	}
	within(h.t, row.Created, created, created, 0, slack, "created_at")
	within(h.t, row.Expires, created, created, 14*day, slack, "expires_at")
}

func (h *harness) linkByToken(token string) linkRow {
	h.t.Helper()
	raw, err := decodeToken(token)
	if err != nil {
		h.t.Fatal("токен ссылки не равен 32 байтам")
	}
	var rows []linkRow
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		scanned, err := conn.Query(ctx, `
			SELECT token_hash, student_id, kind, expires_at, created_at
			FROM core.consent_link
		`)
		if err != nil {
			return err
		}
		defer scanned.Close()
		for scanned.Next() {
			var row linkRow
			if err := scanned.Scan(&row.Hash, &row.Student, &row.Kind, &row.Expires, &row.Created); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return scanned.Err()
	})
	var found linkRow
	n := 0
	for _, row := range rows {
		if tokenIsHash(row.Hash, token, raw) {
			found = row
			n++
		}
	}
	if n != 1 {
		h.t.Fatalf("строк ссылки с хешем токена: %d", n)
	}
	return found
}

func (h *harness) consentRow() consentRow {
	h.t.Helper()
	var row consentRow
	found := false
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		err := conn.QueryRow(ctx, `
			SELECT version, state, granted_at, revoked_at, anonymise_at
			FROM core.consent
			WHERE student_id = $1
		`, studentID).Scan(&row.Version, &row.State, &row.Granted, &row.Revoked, &row.Anonymise)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	if !found {
		h.t.Fatal("нет строки core.consent")
	}
	return row
}

func (h *harness) snapshot() string {
	h.t.Helper()
	pii := h.queryText(`
		SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.student_id)::text, '[]')
		FROM core.student_pii t
	`)
	links := h.queryText(`
		SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.token_hash)::text, '[]')
		FROM core.consent_link t
	`)
	consent := h.queryText(`
		SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.student_id)::text, '[]')
		FROM core.consent t
	`)
	return pii + "\n" + links + "\n" + consent
}

func (h *harness) unchanged(token, before string) {
	h.t.Helper()
	after := h.snapshot()
	if after != before {
		h.t.Fatalf("строки изменились\nбыло: %s\nстало: %s", scrub(before, token), scrub(after, token))
	}
}

func (h *harness) guardToken(token string) {
	h.t.Helper()
	raw, err := decodeToken(token)
	if err != nil {
		h.t.Fatal("токен ссылки не равен 32 байтам")
	}
	h.assertLogsHide(token, raw)
	h.assertNoRawToken(token, raw)
}

func (h *harness) assertLogsHide(token string, raw []byte) {
	h.t.Helper()
	text := h.logs.String()
	secrets := []string{
		token,
		hex.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
		base64.StdEncoding.EncodeToString(raw),
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			h.t.Fatal("журнал содержит токен ссылки")
		}
	}
}

func (h *harness) assertNoRawToken(token string, raw []byte) {
	h.t.Helper()
	type column struct {
		table string
		name  string
		udt   string
	}
	var cols []column
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		scanned, err := conn.Query(ctx, `
			SELECT table_name, column_name, udt_name
			FROM information_schema.columns
			WHERE table_schema = 'core'
			  AND table_name IN ('consent_link', 'consent', 'student_pii')
		`)
		if err != nil {
			return err
		}
		defer scanned.Close()
		for scanned.Next() {
			var col column
			if err := scanned.Scan(&col.table, &col.name, &col.udt); err != nil {
				return err
			}
			cols = append(cols, col)
		}
		return scanned.Err()
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

func (h *harness) queryText(sql string, args ...any) string {
	h.t.Helper()
	var out string
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, sql, args...).Scan(&out)
	})
	return out
}

func decodeToken(token string) ([]byte, error) {
	if raw, err := hex.DecodeString(token); err == nil && len(raw) == 32 && strings.EqualFold(hex.EncodeToString(raw), token) {
		return raw, nil
	}
	if raw, err := base64.RawURLEncoding.DecodeString(token); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(token); err == nil && len(raw) == 32 {
		return raw, nil
	}
	return nil, errors.New("токен не 32 байта")
}

func tokenIsHash(hash []byte, token string, raw []byte) bool {
	if len(hash) != sha256.Size || len(raw) != 32 {
		return false
	}
	sumRaw := sha256.Sum256(raw)
	sumText := sha256.Sum256([]byte(token))
	return bytes.Equal(hash, sumRaw[:]) || bytes.Equal(hash, sumText[:])
}

func decodePage(t *testing.T, body []byte, token string) consentPage {
	t.Helper()
	if token != "" && strings.Contains(string(body), token) {
		t.Fatal("тело страницы содержит токен")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var page consentPage
	if err := dec.Decode(&page); err != nil {
		t.Fatalf("ConsentPage: %v тело %s", err, scrub(string(body), token))
	}
	return page
}

func decodeErr(t *testing.T, body []byte, token string) string {
	t.Helper()
	if token != "" && strings.Contains(string(body), token) {
		t.Fatal("тело ошибки содержит токен")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var raw struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("Error: %v тело %s", err, scrub(string(body), token))
	}
	if raw.Code == "" {
		t.Fatal("в теле нет code")
	}
	return raw.Code
}

func expectAnonymise(t *testing.T, page consentPage, start, end time.Time, token string) {
	t.Helper()
	if page.AnonymiseAt == nil || *page.AnonymiseAt == "" {
		t.Fatal("в ответе нет anonymise_at")
	}
	got := parseTS(t, *page.AnonymiseAt, token)
	within(t, got, start, end, 7*day, slack, "anonymise_at в ответе")
}

func parseTS(t *testing.T, value, token string) time.Time {
	t.Helper()
	if token != "" && strings.Contains(value, token) {
		t.Fatal("метка времени содержит токен")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed
		}
	}
	t.Fatalf("время не RFC3339: %q", value)
	return time.Time{}
}

func within(t *testing.T, got, start, end time.Time, offset, slack time.Duration, what string) {
	t.Helper()
	low := start.Add(offset - slack)
	high := end.Add(offset + slack)
	if got.Before(low) || got.After(high) {
		t.Fatalf("%s = %s, ожидалось между %s и %s",
			what,
			got.UTC().Format(time.RFC3339),
			low.UTC().Format(time.RFC3339),
			high.UTC().Format(time.RFC3339))
	}
}

func scrub(text, token string) string {
	if token == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[token]")
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
