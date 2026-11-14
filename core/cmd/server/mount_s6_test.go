package main

import (
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
			if rec.Code == http.StatusNotFound {
				t.Fatalf("маршрут не подключён, код 404, тело %s", rec.Body.String())
			}
		})
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
