package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// AU-R24: изменяющий запрос сверяет Origin с APP_ORIGIN.
// Кнопка «срок» шлёт PATCH /lessons/{id}/homework-draft в журнал попыток.
func TestAU_R24_DraftPatchUsesAppOrigin(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)
	origin := os.Getenv("APP_ORIGIN")
	if origin != "http://127.0.0.1:8080" {
		t.Fatalf("APP_ORIGIN %q", origin)
	}
	cookie := s1Session(t, handler)

	match := draftPatch(handler, cookie, origin)
	expectForbidden(t, match)

	expectCSRF(t, draftPatch(handler, cookie, "http://localhost"))
	expectCSRF(t, draftPatch(handler, cookie, "https://evil.example"))
	expectCSRF(t, draftPatch(handler, cookie, ""))

	journal := submitMissing(handler, cookie, "http://localhost")
	expectCSRF(t, journal)
	expectForbidden(t, submitMissing(handler, cookie, origin))
}

func draftPatch(handler http.Handler, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	body := strings.NewReader(`{"deadline":"2026-11-25T18:00:00Z"}`)
	req := httptest.NewRequest(http.MethodPatch, "/lessons/lesson-missing/homework-draft", body)
	return mutating(handler, req, cookie, origin)
}

func submitMissing(handler http.Handler, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/assignments/assignment-missing/submit", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "csrf-origin-once")
	return mutating(handler, req, cookie, origin)
}

func mutating(handler http.Handler, req *http.Request, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("X-CSRF-Token", cookie.Value)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func expectForbidden(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
		t.Fatalf("нужен 403 forbidden, код %d, тело %s", rec.Code, rec.Body.String())
	}
}

func expectCSRF(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "csrf" {
		t.Fatalf("нужен 403 csrf, код %d, тело %s", rec.Code, rec.Body.String())
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("не JSON: %v, тело %s", err, rec.Body.Bytes())
	}
	return body.Code
}
