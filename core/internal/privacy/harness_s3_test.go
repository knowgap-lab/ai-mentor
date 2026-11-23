package privacy_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/privacy"
)

// Сеанс репетитора создаёт auth.Handler той же базы: POST /auth/dev,
// cookie __Host-session. privacy.Handler читает core.session так же:
// token_hash — SHA-256 от 32 сырых байт hex-токена.
// Изменяющие запросы репетитора несут Origin http://localhost и
// X-CSRF-Token, равный cookie (AU-R24).
// POST /consent/{token} без Origin проходит. Чужой Origin — 403 csrf.
//
// AnonymiseDue(ctx, now) обезличивает учеников, у которых anonymise_at
// наступил к моменту now, и возвращает их число.
//
// ВОПРОС: алфавит псевдонима — Crockford (0-9 без I,L,O,U) или RFC 4648 (A-Z2-7)? Пример K7MQ2XP4 подходит обоим. Проверяю длину 8 и [0-9A-Z].
// ВОПРОС: повтор POST /students/{id}/consent-links при живой ссылке — 201 или 409? Кода в спецификации нет. Проверяю только, что первая ссылка после второго запроса открывается как раньше.
// ВОПРОС: код 409 при повторной архивации, повторном unarchive и отмене отзыва без отзыва не назван. Проверяю статус 409 и что состояние не меняется.
// ВОПРОС: повтор POST /students/{id}/anonymise с confirm=true. Варианты: 204, 403, 409. Проверяю, что ПДн не возвращаются и событие tutor_on_request одно.
// ВОПРОС: exam_kind и exam_date есть во входе и нет в Student и в имени таблицы. Место хранения не выбираю. Проверяю, что запрос принят и этих полей нет в student_pii.
// ВОПРОС: createInvite не входит в операции S3, поэтому 423 consent_missing (AU-R8, PV-R14) здесь не проверяю. Варианты: тест в auth; перенести операцию в privacy.
// ВОПРОС: consent_id события. В core.consent первичный ключ — student_id. Тест ждёт consent_id, равный id ученика.
// ВОПРОС: POST /students не передаёт telegram_id, а столбец NOT NULL. Значение до привязки не выбираю и не проверяю. В ответе поля telegram нет.

const (
	s3Origin     = "http://localhost"
	s3Cookie     = "__Host-session"
	s3Nick       = "demo-s3-nick"
	s3BirthFixed = "2010-04-02"
	s3Subject    = "algebra"
	s3ExamKind   = "ct"
	s3ExamDate   = "2027-06-15"
	s3EvilOrigin = "https://evil.example"
)

type s3Actor struct {
	ID    string
	Token string
	Role  string
}

type s3Card struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	BirthDate     string   `json:"birth_date,omitempty"`
	Status        string   `json:"status"`
	Consent       string   `json:"consent"`
	TelegramBound bool     `json:"telegram_bound,omitempty"`
	Subjects      []string `json:"subjects,omitempty"`
	Warnings      []struct {
		Type   string          `json:"type"`
		Text   string          `json:"text,omitempty"`
		Params json.RawMessage `json:"params,omitempty"`
	} `json:"warnings,omitempty"`
}

type s3Link struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

type s3Req struct {
	cookie string
	origin string
	csrf   string
	body   any
}

type s3Event struct {
	Kind    string
	Payload map[string]any
}

type s3Harness struct {
	t    *testing.T
	svc  *privacy.Service
	auth *auth.Service
	dsn  string
	logs *bytes.Buffer
}

func s3New(t *testing.T) *s3Harness {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	t.Setenv("APP_ORIGIN", s3Origin)
	logs := &bytes.Buffer{}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	log.SetOutput(logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	authSvc, err := auth.Open(context.Background(), auth.Config{
		DSN:       dsn,
		Env:       "local",
		DevMode:   true,
		AppOrigin: s3Origin,
		Log:       logs,
	})
	if err != nil {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		t.Fatalf("auth.Open: %v", err)
	}
	svc, err := privacy.Open(context.Background(), privacy.Config{DSN: dsn, Log: logs})
	if err != nil {
		_ = authSvc.Close()
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		t.Fatalf("privacy.Open: %v", err)
	}
	h := &s3Harness{t: t, svc: svc, auth: authSvc, dsn: dsn, logs: logs}
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		if err := svc.Close(); err != nil {
			t.Errorf("privacy close: %v", err)
		}
		if err := authSvc.Close(); err != nil {
			t.Errorf("auth close: %v", err)
		}
	})
	return h
}

func s3Birth(years int) string {
	return time.Now().UTC().AddDate(-years, 0, 0).Format("2006-01-02")
}

func s3RepoFile(t *testing.T, rel string) []byte {
	t.Helper()
	path := filepath.Join(moduleDir(t), "..", filepath.FromSlash(rel))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func (h *s3Harness) login(telegramID, role string) s3Actor {
	h.t.Helper()
	body, err := json.Marshal(map[string]string{"telegram_id": telegramID, "role": role})
	if err != nil {
		h.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/dev", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s3Origin)
	rec := httptest.NewRecorder()
	h.auth.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("вход %s %s: %d %s", telegramID, role, rec.Code, rec.Body.Bytes())
	}
	res := rec.Result()
	defer res.Body.Close()
	var token string
	for _, cookie := range res.Cookies() {
		if cookie.Name == s3Cookie && cookie.Value != "" {
			token = cookie.Value
		}
	}
	if token == "" {
		h.t.Fatal("нет cookie сеанса")
	}
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		h.t.Fatal("токен сеанса не равен 32 байтам hex")
	}
	var me struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		h.t.Fatal(err)
	}
	if me.UserID == "" || me.Role != role {
		h.t.Fatalf("Me = %+v", me)
	}
	return s3Actor{ID: me.UserID, Token: token, Role: role}
}

func (h *s3Harness) do(method, path string, req s3Req) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if req.body != nil {
		raw, err := json.Marshal(req.body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	httpReq := httptest.NewRequest(method, path, reader)
	if req.body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.origin != "" {
		httpReq.Header.Set("Origin", req.origin)
	}
	if req.csrf != "" {
		httpReq.Header.Set("X-CSRF-Token", req.csrf)
	}
	if req.cookie != "" {
		httpReq.AddCookie(&http.Cookie{Name: s3Cookie, Value: req.cookie})
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, httpReq)
	return rec
}

func (h *s3Harness) call(actor s3Actor, method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(method, path, s3Req{
		cookie: actor.Token,
		origin: s3Origin,
		csrf:   actor.Token,
		body:   body,
	})
}

func (h *s3Harness) get(actor s3Actor, path string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, path, s3Req{cookie: actor.Token})
}

func (h *s3Harness) create(actor s3Actor, name, birth string) s3Card {
	h.t.Helper()
	rec := h.call(actor, http.MethodPost, "/students", map[string]any{
		"display_name": name,
		"birth_date":   birth,
		"subjects":     []string{s3Subject},
		"exam_kind":    s3ExamKind,
		"exam_date":    s3ExamDate,
	})
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("createStudent: %d %s", rec.Code, rec.Body.Bytes())
	}
	card := s3DecodeCard(h.t, rec.Body.Bytes())
	if card.ID == "" || card.DisplayName != name || card.Status != "active" || card.Consent != "none" {
		h.t.Fatalf("карточка %+v", card)
	}
	if !s3SameDay(card.BirthDate, birth) {
		h.t.Fatalf("birth_date = %q, нужна %s", card.BirthDate, birth)
	}
	if len(card.Subjects) != 1 || card.Subjects[0] != s3Subject {
		h.t.Fatalf("subjects = %v", card.Subjects)
	}
	if card.TelegramBound {
		h.t.Fatal("telegram_bound без привязки")
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"telegram_id"`)) {
		h.t.Fatal("ответ содержит telegram_id")
	}
	return card
}

func (h *s3Harness) link(actor s3Actor, studentID, kind string) string {
	h.t.Helper()
	start := time.Now()
	rec := h.call(actor, http.MethodPost, s3StudentPath(studentID, "/consent-links"), map[string]string{"kind": kind})
	end := time.Now()
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("createConsentLink: %d %s", rec.Code, rec.Body.Bytes())
	}
	var body s3Link
	s3Decode(h.t, rec.Body.Bytes(), &body)
	token := s3TokenFromURL(h.t, body.URL)
	if strings.Contains(rec.Body.String(), token) && !strings.Contains(body.URL, token) {
		h.t.Fatal("токен попал в ответ вне url")
	}
	expires := parseTS(h.t, body.ExpiresAt, token)
	within(h.t, expires, start, end, 14*day, slack, "expires_at ссылки")
	h.logsHide(token)
	return token
}

func (h *s3Harness) consent(token, action string) consentPage {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/consent/"+url.PathEscape(token), s3Req{
		body: map[string]string{"action": action},
	})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("consent %s: %d %s", action, rec.Code, s3Scrub(rec.Body.String(), token))
	}
	if strings.Contains(rec.Body.String(), token) {
		h.t.Fatal("тело согласия содержит токен")
	}
	var page consentPage
	s3Decode(h.t, rec.Body.Bytes(), &page)
	return page
}

func (h *s3Harness) mint(userID string) string {
	h.t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		h.t.Fatal(err)
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	h.exec(`
		INSERT INTO core.session (token_hash, user_id, created_at, expires_at)
		VALUES ($1, $2, now(), now() + interval '90 days')
	`, sum[:], userID)
	return token
}

func (h *s3Harness) user(id string) (role, owner, status string) {
	h.t.Helper()
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT role, coalesce(owner_tutor_id, ''), status
			FROM core.app_user WHERE id = $1
		`, id).Scan(&role, &owner, &status)
	})
	return role, owner, status
}

func (h *s3Harness) pii(id string) (name, telegram, birth string, ok bool) {
	h.t.Helper()
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		err := conn.QueryRow(ctx, `
			SELECT display_name, telegram_id, coalesce(birth_date::text, '')
			FROM core.student_pii WHERE student_id = $1
		`, id).Scan(&name, &telegram, &birth)
		if err == nil {
			ok = true
			return nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return name, telegram, birth, ok
}

func (h *s3Harness) pseudonym(id string) (string, bool) {
	h.t.Helper()
	var code string
	ok := false
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		err := conn.QueryRow(ctx, `
			SELECT code FROM core.student_pseudonym WHERE student_id = $1
		`, id).Scan(&code)
		if err == nil {
			ok = true
			return nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return code, ok
}

func (h *s3Harness) columns(table string) []string {
	h.t.Helper()
	var out []string
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_schema = 'core' AND table_name = $1
			ORDER BY column_name
		`, table)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			out = append(out, name)
		}
		return rows.Err()
	})
	return out
}

func (h *s3Harness) events() []s3Event {
	h.t.Helper()
	var out []s3Event
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT kind, payload
			FROM core.journal_event
			ORDER BY journal_seq
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ev s3Event
			var payload []byte
			if err := rows.Scan(&ev.Kind, &payload); err != nil {
				return err
			}
			if err := json.Unmarshal(payload, &ev.Payload); err != nil {
				return err
			}
			if ev.Payload == nil {
				ev.Payload = map[string]any{}
			}
			out = append(out, ev)
		}
		return rows.Err()
	})
	return out
}

func (h *s3Harness) oneEvent(kind string) map[string]any {
	h.t.Helper()
	var found map[string]any
	n := 0
	for _, ev := range h.events() {
		if ev.Kind != kind {
			continue
		}
		found = ev.Payload
		n++
	}
	if n != 1 {
		h.t.Fatalf("событий %s: %d", kind, n)
	}
	return found
}

func (h *s3Harness) openSessions(userID string) int {
	h.t.Helper()
	return h.count(`
		SELECT count(*) FROM core.session
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
	`, userID)
}

func (h *s3Harness) logsHide(secrets ...string) {
	h.t.Helper()
	text := h.logs.String()
	var all []string
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		all = append(all, secret)
		raw, err := decodeToken(secret)
		if err != nil {
			continue
		}
		all = append(all,
			hex.EncodeToString(raw),
			base64.RawURLEncoding.EncodeToString(raw),
			base64.StdEncoding.EncodeToString(raw),
		)
	}
	for _, secret := range all {
		if secret != "" && strings.Contains(text, secret) {
			h.t.Fatal("журнал содержит ПДн или токен")
		}
	}
}

func (h *s3Harness) with(fn func(context.Context, *pgx.Conn) error) {
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

func (h *s3Harness) exec(sql string, args ...any) {
	h.t.Helper()
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, sql, args...)
		return err
	})
}

func (h *s3Harness) tryQuery(sql string, args ...any) error {
	h.t.Helper()
	var queryErr error
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, sql, args...)
		if err != nil {
			queryErr = err
			return nil
		}
		rows.Close()
		queryErr = rows.Err()
		return nil
	})
	return queryErr
}

func (h *s3Harness) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, sql, args...).Scan(&n)
	})
	return n
}

func (h *s3Harness) text(sql string, args ...any) string {
	h.t.Helper()
	var out string
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, sql, args...).Scan(&out)
	})
	return out
}

func s3StudentPath(id, tail string) string {
	return "/students/" + url.PathEscape(id) + tail
}

func s3TokenFromURL(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	path := parsed.Path
	if path == "" {
		path = raw
	}
	token := path[strings.LastIndex(path, "/")+1:]
	token, err = url.PathUnescape(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeToken(token); err != nil {
		t.Fatalf("токен ссылки: %v", err)
	}
	return token
}

func s3Decode(t *testing.T, body []byte, dest any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		t.Fatalf("json: %v тело %s", err, body)
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("лишние данные: %v", err)
	}
}

func s3DecodeCard(t *testing.T, body []byte) s3Card {
	t.Helper()
	var card s3Card
	s3Decode(t, body, &card)
	return card
}

func s3Expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
	if code == "" {
		return
	}
	if got := decodeErr(t, rec.Body.Bytes(), ""); got != code {
		t.Fatalf("code = %s, нужен %s", got, code)
	}
}

func s3ExpectConflict(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	s3Expect(t, rec, http.StatusConflict, "")
	if decodeErr(t, rec.Body.Bytes(), "") == "" {
		t.Fatal("у 409 нет code")
	}
}

func s3SameDay(got, want string) bool {
	return got == want || strings.HasPrefix(got, want)
}

func s3SameJSON(t *testing.T, a, b []byte) {
	t.Helper()
	if !s3JSONEqual(a, b) {
		t.Fatalf("тела различаются\n%s\n%s", a, b)
	}
}

func s3JSONEqual(a, b []byte) bool {
	var left, right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return s3ValueEqual(left, right)
}

func s3ValueEqual(left, right any) bool {
	lb, err1 := json.Marshal(left)
	rb, err2 := json.Marshal(right)
	return err1 == nil && err2 == nil && bytes.Equal(lb, rb)
}

func s3PayloadExact(t *testing.T, payload map[string]any, keys ...string) {
	t.Helper()
	want := make(map[string]bool, len(keys))
	for _, key := range keys {
		want[key] = true
		if _, ok := payload[key]; !ok {
			t.Fatalf("в payload нет %s: %#v", key, payload)
		}
	}
	for key := range payload {
		if !want[key] {
			t.Fatalf("лишнее поле payload %s", key)
		}
	}
}

func s3String(t *testing.T, payload map[string]any, key string) string {
	t.Helper()
	value, ok := payload[key].(string)
	if !ok || value == "" {
		t.Fatalf("payload.%s = %#v", key, payload[key])
	}
	return value
}

func s3Scrub(text, token string) string {
	if token == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[token]")
}

func s3JSONHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.ContentLength != 0 && r.Header.Get("Content-Type") == "" {
			r.Header.Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}
