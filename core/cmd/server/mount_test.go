package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/pressly/goose/v3"
)

// Тесты сборки S1. rootHandler в режиме local. Ответ на строку таблицы — не SPA.
// SPA — Content-Type с text/html или тело с id="root".
// Без сеанса код 401 или 404 принимается, если тело — JSON.
//
// ВОПРОС: бриф просит один запрос на строку таблицы, а в пяти строках перечислено
// несколько путей и не сказано, какой слать. Тест шлёт первый путь ячейки и не
// проверяет остальные: POST /consent/{token}; POST /lessons/{lesson}/move;
// POST /lessons/{lesson}/cancel; POST /lessons/{lesson}/marks;
// PUT /students/{id}/plans/{subject}/order; GET /lessons/{lesson}/conspect.pdf.
//
// ВОПРОС: у строки ДЗ первый путь — GET /lessons/{lesson}/homework-draft.
// Журнал попыток уже держит PATCH на этом пути, поэтому GET — 405 text/plain,
// и это не SPA. Критерий брифа такой ответ пропускает, хотя GET модуль не раздаёт.
// Для строки взят второй путь: POST /lessons/{lesson}/homework-draft/issue.
// GET /lessons/{lesson}/homework-draft тест не проверяет.
//
// ВОПРОС: строка GET /students/{id}/plans/{subject} отдаётся plan, не lessons.
// Раздел тестов требует только «не SPA». Без сеанса оба модуля отвечают одинаково,
// поэтому тест не проверяет, какой модуль ответил.
//
// ВОПРОС: GET /consent/{token} без сеанса. Бриф допускает 401 или 404 с JSON.
// У модуля privacy неизвестный токен — не эти коды. Тест не требует именно
// 401 или 404 и проверяет «не SPA». Если код всё же 401 или 404, тело — JSON.

func TestS1Mount(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)

	rows := []struct {
		name   string
		method string
		path   string
	}{
		{name: "S1_row_consent", method: http.MethodGet, path: "/consent/missing"},
		{name: "S1_row_schedule", method: http.MethodPut, path: "/students/student-s1/schedules/algebra"},
		{name: "S1_row_lesson", method: http.MethodGet, path: "/lessons/lesson-s1"},
		{name: "S1_row_plan", method: http.MethodGet, path: "/students/student-s1/plans/algebra"},
		{name: "S1_row_divergence", method: http.MethodGet, path: "/students/student-s1/plans/algebra/divergence"},
		{name: "S1_row_homework_draft", method: http.MethodPost, path: "/lessons/lesson-s1/homework-draft/issue"},
		{name: "S1_row_conspect", method: http.MethodGet, path: "/lessons/lesson-s1/conspect"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := s1Call(handler, row.method, row.path, nil)
			expectS1NotSPA(t, rec)
		})
	}

	t.Run("S1_status", func(t *testing.T) {
		rec := s1Call(handler, http.MethodGet, "/status", nil)
		expectS1StatusOK(t, rec)
	})

	t.Run("S1_internal_no_authorization", func(t *testing.T) {
		rec := s1Call(handler, http.MethodPost, "/internal/catalog-versions", nil)
		expectS1Unauthorized(t, rec)
	})

	t.Run("S1_internal_session_cookie", func(t *testing.T) {
		cookie := s1Session(t, handler)
		rec := s1Call(handler, http.MethodPost, "/internal/catalog-versions", cookie)
		expectS1Unauthorized(t, rec)
	})

	t.Run("S1_not_a_screen", func(t *testing.T) {
		rec := s1Call(handler, http.MethodGet, "/not-a-screen", nil)
		if !s1SPA(rec) {
			t.Fatalf("ожидался SPA, код %d, content-type %q, тело %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
	})
}

func s1Root(t *testing.T, dsn string) http.Handler {
	t.Helper()
	dir := t.TempDir()
	html := "<!doctype html>\n<html><body><div id=\"root\"></div></body></html>\n"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "local")
	t.Setenv("AUTH_DEV_MODE", "true")
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("APP_ORIGIN", "http://127.0.0.1:8080")
	t.Setenv("WEB_DIST", dir)
	handler, err := rootHandler()
	if err != nil {
		t.Fatalf("rootHandler: %v", err)
	}
	return handler
}

func s1Call(handler http.Handler, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func s1Session(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	body := bytes.NewBufferString(`{"telegram_id":"tg-s1","role":"tutor"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/dev", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", os.Getenv("APP_ORIGIN"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	if rec.Code != http.StatusOK {
		t.Fatalf("вход: код %d, тело %s", rec.Code, rec.Body.String())
	}
	for _, cookie := range res.Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatal("нет cookie сеанса")
	return nil
}

func expectS1NotSPA(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if s1SPA(rec) {
		t.Fatalf("ответ SPA, код %d, content-type %q, тело %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusNotFound {
		return
	}
	if !s1JSON(rec.Body.Bytes()) {
		t.Fatalf("код %d без JSON, content-type %q, тело %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func expectS1Unauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if s1SPA(rec) {
		t.Fatalf("ответ SPA, код %d, content-type %q, тело %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("не JSON: %v, тело %s", err, rec.Body.Bytes())
	}
	if body.Code != "unauthorized" {
		t.Fatalf("code = %q, тело %s", body.Code, rec.Body.Bytes())
	}
}

func expectS1StatusOK(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
	}
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	var body struct {
		Status string `json:"status"`
	}
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("тело: %v, %s", err, rec.Body.Bytes())
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("лишние данные в теле: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q", body.Status)
	}
}

func s1SPA(rec *httptest.ResponseRecorder) bool {
	if strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		return true
	}
	return strings.Contains(rec.Body.String(), `id="root"`)
}

func s1JSON(raw []byte) bool {
	var v any
	return json.Unmarshal(raw, &v) == nil
}

func applyS1Migrations(t *testing.T, dsn string) {
	t.Helper()
	dir, err := s1MigrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("db close: %v", closeErr)
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			t.Errorf("goose close: %v", closeErr)
		}
	}()
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("миграции: %v", err)
	}
}

func s1MigrationsDir() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		if dir, err := findS1Migrations(start); err == nil {
			return dir, nil
		}
	}
	return "", errors.New("каталог migrations не найден")
}

func findS1Migrations(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, "migrations", "00001_schemas.sql")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return filepath.Join(dir, "migrations"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("каталог migrations не найден")
		}
		dir = parent
	}
}
