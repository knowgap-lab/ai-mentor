package attempts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func s5Call(h *harness, method, path string, body any, token string, origin *string) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	}
	if origin != nil {
		req.Header.Set("Origin", *origin)
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set("X-CSRF-Token", token)
		if req.Header.Get("Idempotency-Key") == "" {
			req.Header.Set("Idempotency-Key", "s5-key")
		}
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func s5Ptr(value string) *string {
	return &value
}

func s5TutorTelegram(h *harness) string {
	h.t.Helper()
	var telegram string
	err := h.conn.QueryRow(context.Background(), `
		SELECT p.telegram_id
		FROM core.user_pii AS p
		JOIN core.app_user AS u ON u.id = p.user_id
		WHERE u.role = 'tutor'
		ORDER BY u.id
		LIMIT 1
	`).Scan(&telegram)
	if err != nil {
		h.t.Fatalf("репетитор: %v", err)
	}
	return telegram
}

func s5TutorToken(h *harness) string {
	h.t.Helper()
	return login(h.t, h.dsn, s5TutorTelegram(h), "tutor")
}

func s5ForeignStudent(h *harness) string {
	h.t.Helper()
	s5OtherTutor(h)
	const id = "s5-student-b"
	const telegram = "tg-s5-student-b"
	ctx := context.Background()
	if _, err := h.conn.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', 's5-tutor-b', 'active')
	`, id); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.conn.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, telegram); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func s5OtherTutor(h *harness) string {
	h.t.Helper()
	const id = "s5-tutor-b"
	const telegram = "tg-s5-tutor-b"
	ctx := context.Background()
	if _, err := h.conn.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'tutor', NULL, 'active')
	`, id); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.conn.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, telegram); err != nil {
		h.t.Fatal(err)
	}
	return login(h.t, h.dsn, telegram, "tutor")
}

func s5ExpectSame403(t *testing.T, foreign, missing *httptest.ResponseRecorder) {
	t.Helper()
	if foreign.Code != http.StatusForbidden || missing.Code != http.StatusForbidden {
		t.Fatalf("чужой %d %s; отсутствующий %d %s", foreign.Code, foreign.Body.Bytes(), missing.Code, missing.Body.Bytes())
	}
	decodeError(t, foreign.Body.Bytes())
	decodeError(t, missing.Body.Bytes())
	if canonicalJSON(t, foreign.Body.Bytes()) != canonicalJSON(t, missing.Body.Bytes()) {
		t.Fatalf("ответы различаются: %s | %s", foreign.Body.Bytes(), missing.Body.Bytes())
	}
}

func s5Expect403(t *testing.T, h *harness, rec *httptest.ResponseRecorder) {
	t.Helper()
	h.status(rec, http.StatusForbidden)
	decodeError(t, rec.Body.Bytes())
}

func s5StudentPath(id, suffix string) string {
	return "/students/" + url.PathEscape(id) + suffix
}

func s5SessionHeaders(token string, extra map[string]string) map[string]string {
	header := map[string]string{
		"Origin":       appOrigin,
		"Cookie":       sessionCookie + "=" + token,
		"X-CSRF-Token": token,
	}
	for key, value := range extra {
		header[key] = value
	}
	return header
}

func s5Spec(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleDir(t), "..", "docs", "contracts", "core-api.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func s5RequireExample(t *testing.T, id, rule string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleDir(t), "..", "docs", "platform", "attempts.examples.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID   string `yaml:"id"`
		Rule string `yaml:"rule"`
	}
	if err := yaml.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == id && row.Rule == rule {
			return
		}
	}
	t.Fatalf("в attempts.examples.yaml нет %s / %s", id, rule)
}

func s5ManualEvent(h *harness, attemptID string) bool {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), `
		SELECT kind, payload
		FROM core.journal_event
		WHERE kind IN ('manual_verdict', 'attempt_verdict')
	`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var raw []byte
		if err := rows.Scan(&kind, &raw); err != nil {
			h.t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			h.t.Fatal(err)
		}
		if payload["attempt_id"] != attemptID {
			continue
		}
		if kind == "manual_verdict" {
			return true
		}
		if payload["origin"] == "manual" {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return false
}
