package pdf_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/gotenbergtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/pdf"
)

// Публичный интерфейс фиксируют вызовы ниже.
//
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//
// Config содержит только DSN. Адрес Gotenberg читается из окружения.
// Имя переменной — единственное уже записанное в deploy (gotenbergtest.EnvName).
//
// Handler:
//
//	GET /assignments/{id}/sheet.pdf
//	GET /tests/{test}/form.pdf
//
// Сеанс — cookie __Host-session после POST /auth/dev.
// Чужой и несуществующий листок — 403 forbidden (AU-R4). Нет сеанса — 401 unauthorized.
//
// ВОПРОС: у getTestFormPdf нет парной JSON-операции. Чужого ученика и чужого
// репетитора на бланке тест не проверяет. Варианты: как getAssignment (403);
// любой вошедший репетитор; ученик своей выдачи. Проверяются 401 без сеанса
// и PDF для репетитора фикстуры.

const (
	s8Origin     = "http://localhost"
	s8Cookie     = "__Host-session"
	s8Subject    = "math"
	s8SheetID    = "s8-sheet"
	s8OtherSheet = "s8-sheet-other"
	s8TestID     = "demo:probe-s8"
	s8SheetKey   = "ключ-листка-7341"
	s8FormKey    = "ключ-бланка-9182"
	s8Statement  = "Условие выдуманного листка про две корзины."
	s8FormTitle  = "Выдуманный бланк среза"
	s8TutorTG    = "tg-s8-tutor"
	s8StudentTG  = "tg-s8-student"
	s8TutorBTG   = "tg-s8-tutor-b"
	s8StudentBTG = "tg-s8-student-b"
)

type s8Harness struct {
	t     *testing.T
	dsn   string
	conn  *pgx.Conn
	svc   *pdf.Service
	token string
}

func s8NewHarness(t *testing.T, gotenbergURL string) *s8Harness {
	t.Helper()
	s8UseGotenberg(t, gotenbergURL)
	dsn := dbtest.Start(t)
	s8Migrate(t, dsn)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			t.Errorf("conn close: %v", closeErr)
		}
	})
	if err := s8Seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc, err := pdf.Open(ctx, pdf.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := svc.Close(); closeErr != nil {
			t.Errorf("close: %v", closeErr)
		}
	})
	h := &s8Harness{t: t, dsn: dsn, conn: conn, svc: svc}
	h.as(s8StudentTG, "student")
	return h
}

func s8UseGotenberg(t *testing.T, rawURL string) {
	t.Helper()
	if rawURL == "" {
		return
	}
	name, err := gotenbergtest.EnvName()
	if err != nil {
		t.Fatalf("ВОПРОС: %s; варианты: записать одно имя в deploy/secrets.md или deploy/README.md", err)
	}
	t.Setenv(name, rawURL)
}

func (h *s8Harness) as(telegramID, role string) {
	h.t.Helper()
	h.token = s8Login(h.t, h.dsn, telegramID, role)
}

func (h *s8Harness) call(path string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if h.token != "" {
		req.AddCookie(&http.Cookie{Name: s8Cookie, Value: h.token})
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *s8Harness) sheet(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call("/assignments/" + id + "/sheet.pdf")
}

func (h *s8Harness) form() *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call("/tests/" + s8TestID + "/form.pdf")
}

func (h *s8Harness) expectCode(rec *httptest.ResponseRecorder, status int, code string) {
	h.t.Helper()
	if rec.Code != status {
		h.t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		h.t.Fatalf("ошибка: %v, тело %s", err, rec.Body.Bytes())
	}
	if payload.Code != code {
		h.t.Fatalf("код %s, нужен %s", payload.Code, code)
	}
}

func s8PDF(t *testing.T, rec *httptest.ResponseRecorder) []byte {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("статус %d, тело %s", rec.Code, rec.Body.Bytes())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/pdf") {
		t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
	}
	body := append([]byte(nil), rec.Body.Bytes()...)
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		n := len(body)
		if n > 24 {
			n = 24
		}
		t.Fatalf("нет %%PDF-: %q", body[:n])
	}
	return body
}

func s8HTML(t *testing.T, stub *gotenbergtest.Stub) string {
	t.Helper()
	bodies := stub.Bodies()
	if len(bodies) == 0 {
		t.Fatal("Gotenberg не получил документ")
	}
	return string(bodies[0])
}

func s8NoExternalURL(t *testing.T, html string) {
	t.Helper()
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Fatal("в HTML есть ссылка http:// или https://")
	}
}

func s8Seed(ctx context.Context, conn *pgx.Conn) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s8User(ctx, tx, "s8-tutor", "tutor", s8TutorTG, nil); err != nil {
		return err
	}
	tutor := "s8-tutor"
	if err := s8User(ctx, tx, "s8-student", "student", s8StudentTG, &tutor); err != nil {
		return err
	}
	if err := s8User(ctx, tx, "s8-tutor-b", "tutor", s8TutorBTG, nil); err != nil {
		return err
	}
	other := "s8-tutor-b"
	if err := s8User(ctx, tx, "s8-student-b", "student", s8StudentBTG, &other); err != nil {
		return err
	}
	if err := s8PlanLesson(ctx, tx, "s8-student", "s8-lesson"); err != nil {
		return err
	}
	if err := s8Revision(ctx, tx, "demo:Vsheet", s8Variant("demo:Vsheet", s8Statement, s8SheetKey)); err != nil {
		return err
	}
	if err := s8Revision(ctx, tx, "demo:Vform", s8Variant("demo:Vform", "Условие выдуманного бланка про три корзины.", s8FormKey)); err != nil {
		return err
	}
	if err := s8Revision(ctx, tx, s8TestID, map[string]any{
		"id":    s8TestID,
		"title": s8FormTitle,
		"year":  2024,
		"stage": "репетиция",
		"appearances": []any{
			map[string]any{"position": 1, "type": "demo:type-s8"},
		},
	}); err != nil {
		return err
	}
	if err := s8Assignment(ctx, tx, s8SheetID, "s8-student", "homework", "s8-lesson", "demo:Vsheet", s8Statement); err != nil {
		return err
	}
	if err := s8Assignment(ctx, tx, s8OtherSheet, "s8-student-b", "homework", "s8-lesson", "demo:Vsheet", s8Statement); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func s8Variant(id, statement, key string) map[string]any {
	return map[string]any{
		"id":           id,
		"subvariant":   id + ".1",
		"statement":    statement,
		"key":          key,
		"minutes":      5,
		"theory_block": "demo:theory-s8",
	}
}

func s8User(ctx context.Context, tx pgx.Tx, id, role, telegram string, owner *string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, $2, $3, 'active')
	`, id, role, owner); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, telegram)
	return err
}

func s8PlanLesson(ctx context.Context, tx pgx.Tx, studentID, lessonID string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
		VALUES ($1, $2, $3, 1)
	`, studentID, s8Subject, time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.lesson (
			id, student_id, subject, place_no, title, kind, template_id, state, template
		) VALUES ($1, $2, $3, 1, 'Урок листка', 'new', $1, 'held', '[]'::jsonb)
	`, lessonID, studentID, s8Subject)
	return err
}

func s8Revision(ctx context.Context, tx pgx.Tx, id string, data map[string]any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, 1, $2, $3::jsonb)
	`, id, "s8-"+id, string(raw))
	return err
}

func s8Assignment(ctx context.Context, tx pgx.Tx, id, studentID, kind, lessonID, variantID, statement string) error {
	raw, err := json.Marshal(map[string]any{
		"lesson_id": lessonID,
		"items": []any{
			map[string]any{
				"variant_id":    variantID,
				"item_id":       variantID,
				"statement":     statement,
				"answer_format": "ct.integer",
				"type_id":       "demo:type-s8",
				"minutes":       5,
				"source":        "homework",
			},
		},
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		) VALUES ($1, $2, 'issued', $3, 1, $4, 'home', $5::jsonb)
	`, id, studentID, kind, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), string(raw)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, max_points)
		VALUES ($1, $2, 'open', 1)
	`, id, variantID)
	return err
}

func s8Login(t *testing.T, dsn, telegramID, role string) string {
	t.Helper()
	svc, err := auth.Open(context.Background(), auth.Config{
		DSN:       dsn,
		Env:       "local",
		DevMode:   true,
		AppOrigin: s8Origin,
		Log:       io.Discard,
	})
	if err != nil {
		t.Fatalf("auth.Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := svc.Close(); closeErr != nil {
			t.Errorf("auth close: %v", closeErr)
		}
	})
	body, err := json.Marshal(map[string]string{
		"telegram_id": telegramID,
		"role":        role,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/dev", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s8Origin)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("вход %s %s: %d %s", telegramID, role, rec.Code, rec.Body.Bytes())
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == s8Cookie && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatal("нет cookie сеанса")
	return ""
}

var (
	s8MigrateOnce sync.Once
	s8MigrateExe  string
	s8MigrateErr  error
)

func s8Migrate(t *testing.T, dsn string) {
	t.Helper()
	exe := s8MigrateBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = s8ModuleDir(t)
	cmd.Env = append(s8WithoutEnv(os.Environ(), "DATABASE_URL"), "DATABASE_URL="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
}

func s8MigrateBinary(t *testing.T) string {
	t.Helper()
	s8MigrateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ai-mentor-migrate-s8-")
		if err != nil {
			s8MigrateErr = err
			return
		}
		exe := filepath.Join(dir, "migrate")
		mod, err := s8FindModule()
		if err != nil {
			s8MigrateErr = err
			return
		}
		cmd := exec.Command("go", "build", "-mod=readonly", "-o", exe, "./cmd/migrate")
		cmd.Dir = mod
		out, err := cmd.CombinedOutput()
		if err != nil {
			s8MigrateErr = fmt.Errorf("%w\n%s", err, out)
			return
		}
		s8MigrateExe = exe
	})
	if s8MigrateErr != nil {
		t.Fatal(s8MigrateErr)
	}
	return s8MigrateExe
}

func s8ModuleDir(t *testing.T) string {
	t.Helper()
	dir, err := s8FindModule()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func s8FindModule() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		main := filepath.Join(dir, "cmd", "migrate", "main.go")
		info, err := os.Stat(mod)
		mainInfo, mainErr := os.Stat(main)
		if err == nil && !info.IsDir() && mainErr == nil && !mainInfo.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("модуль core не найден")
		}
		dir = parent
	}
}

func s8WithoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}
