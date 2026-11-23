package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Подключение S6 через rootHandler. Зелёным тест делает коммит оркестратора.
// Токен служебный, отказ по токену здесь не закрепляется: запрос идёт с верным Bearer.

func TestS6_MountOperations(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)
	t.Setenv("CONTENT_API_TOKEN", "s6-token")

	rows := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "S6_announce", method: http.MethodPost, path: "/internal/catalog-versions", body: `{"catalog_version":8}`},
		{name: "S6_start_recheck", method: http.MethodPost, path: "/internal/rechecks", body: `{"correction_id":8,"variants":["ct-demo:D1.T01.S1.V003"]}`},
		{name: "S6_recheck_state", method: http.MethodGet, path: "/internal/rechecks/8"},
		{name: "S6_import_plan", method: http.MethodPost, path: "/internal/plan-imports", body: `{"student_id":"s6-student","subject":"math","lessons":[{"subject":"math","lesson":1,"planned_week":"2026-11-16","phase":"cycle-s6","kind":"new","topics":["M01"],"review":[],"title":"Выдуманный урок","source_path":"lessons/s6.md","source_hash":"abababababababababababababababababababababababababababababababab"}],"snapshot":{"source_hashes":{"lessons/s6.md":"abababababababababababababababababababababababababababababababab"}}}`},
		{name: "S6_recheck_report", method: http.MethodGet, path: "/rechecks/8"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := s6MountCall(handler, row.method, row.path, row.body)
			if s1SPA(rec) {
				t.Fatalf("ответ SPA, код %d, тело %s", rec.Code, rec.Body.String())
			}
			if row.name == "S6_recheck_state" {
				s6RecheckStateMounted(t, handler, dsn, rec)
				return
			}
			if rec.Code == http.StatusNotFound {
				t.Fatalf("маршрут не подключён, код 404, тело %s", rec.Body.String())
			}
		})
	}
}

// Честный 404 обработчика — пакет ещё не создан. Текст ServeMux и SPA — маршрут не подключён.
func s6RecheckStateMounted(t *testing.T, handler http.Handler, dsn string, missing *httptest.ResponseRecorder) {
	t.Helper()
	if !s6HonestNotFound(missing) {
		t.Fatalf("маршрут не подключён, код %d, тело %s", missing.Code, missing.Body.String())
	}
	s6SeedRecheckVariant(t, dsn)
	started := s6MountCall(handler, http.MethodPost, "/internal/rechecks", `{"correction_id":8,"variants":["ct-demo:D1.T01.S1.V003"]}`)
	if s1SPA(started) || started.Code == http.StatusNotFound {
		t.Fatalf("приём не подключён, код %d, тело %s", started.Code, started.Body.String())
	}
	if started.Code != http.StatusAccepted {
		t.Fatalf("приём: код %d, тело %s", started.Code, started.Body.String())
	}
	found := s6MountCall(handler, http.MethodGet, "/internal/rechecks/8", "")
	if s1SPA(found) || found.Code == http.StatusNotFound {
		t.Fatalf("маршрут не подключён, код %d, тело %s", found.Code, found.Body.String())
	}
	if found.Code != http.StatusOK {
		t.Fatalf("состояние: код %d, тело %s", found.Code, found.Body.String())
	}
	var body struct {
		CorrectionID int    `json:"correction_id"`
		State        string `json:"state"`
	}
	if err := json.Unmarshal(found.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v, тело %s", err, found.Body.Bytes())
	}
	if body.CorrectionID != 8 || body.State == "" {
		t.Fatalf("состояние %+v", body)
	}
}

func s6HonestNotFound(rec *httptest.ResponseRecorder) bool {
	return rec.Code == http.StatusNotFound &&
		strings.Contains(rec.Header().Get("Content-Type"), "application/json") &&
		rec.Body.String() == `{"code":"not_found","message":"not_found"}`
}

func s6SeedRecheckVariant(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("db close: %v", closeErr)
		}
	})
	const body = `{"id":"ct-demo:D1.T01.S1.V003","status":"active","key":14,"answer_format":"ct.integer","statement":"выдуманное условие"}`
	if _, err := db.Exec(`
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, 8, 's6-mount', $2::jsonb)
	`, "ct-demo:D1.T01.S1.V003", body); err != nil {
		t.Fatal(err)
	}
}

func s6MountCall(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer s6-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
