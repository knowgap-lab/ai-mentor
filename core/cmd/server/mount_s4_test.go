package main

import (
	"net/http"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Новые операции S4 через rootHandler. Красный, пока оркестратор не подключит маршруты.
func TestS4_MountNewOperations(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)
	rows := []struct {
		name   string
		method string
		path   string
	}{
		{name: "S4_addLesson", method: http.MethodPost, path: "/students/student-s4/lessons"},
		{name: "S4_skipLesson", method: http.MethodPost, path: "/lessons/lesson-s4/skip"},
		{name: "S4_getPreparation", method: http.MethodGet, path: "/lessons/lesson-s4/preparation"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := s1Call(handler, row.method, row.path, nil)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("неизвестный маршрут, код %d, тело %s", rec.Code, rec.Body.Bytes())
			}
			expectS1NotSPA(t, rec)
		})
	}
}
