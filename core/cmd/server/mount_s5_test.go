package main

import (
	"net/http"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Новые операции S5 через rootHandler. Неизвестный маршрут — 404 или SPA.
// Зелёным тест делает подключение маршрутов на Handler журнала попыток.

func TestS5Mount(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)
	rows := []struct {
		name string
		path string
	}{
		{name: "S5_summary", path: "/assignments/s5-missing/summary"},
		{name: "S5_disputes", path: "/students/s5-missing/disputes"},
		{name: "S5_discuss_list", path: "/students/s5-missing/discuss-list"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := s1Call(handler, http.MethodGet, row.path, nil)
			if s1SPA(rec) {
				t.Fatalf("ответ SPA, код %d, content-type %q, тело %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
			}
			if rec.Code == http.StatusNotFound {
				t.Fatalf("неизвестный маршрут, код 404, тело %s", rec.Body.String())
			}
		})
	}
}
