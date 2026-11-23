package main

import (
	"net/http"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Новые PDF-операции S8 через rootHandler. Зелёными их делает оркестратор,
// когда подключит обработчики. Ответ не SPA и не 404 неизвестного маршрута.

func TestS8_Mount(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)
	rows := []struct {
		name string
		path string
	}{
		{name: "S8_conspect_pdf", path: "/lessons/lesson-s8/conspect.pdf"},
		{name: "S8_sheet_pdf", path: "/assignments/assignment-s8/sheet.pdf"},
		{name: "S8_form_pdf", path: "/tests/test-s8/form.pdf"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := s1Call(handler, http.MethodGet, row.path, nil)
			expectS1NotSPA(t, rec)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("неизвестный маршрут, код 404, тело %s", rec.Body.String())
			}
		})
	}
}
