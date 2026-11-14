package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Новые операции S3 через rootHandler. Ответ не SPA и не 404 неизвестного маршрута.
// Зелёным тест делает коммит подключения. До него маршрут уходит в fallback и даёт 404.

func TestS3Mount(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	handler := s1Root(t, dsn)
	rows := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "S3_createStudent", method: http.MethodPost, path: "/students", body: `{"display_name":"demo-s3-nick","birth_date":"2010-04-02","subjects":["algebra"]}`},
		{name: "S3_getStudent", method: http.MethodGet, path: "/students/student-s3"},
		{name: "S3_studentArchive", method: http.MethodPost, path: "/students/student-s3/archive"},
		{name: "S3_studentUnarchive", method: http.MethodPost, path: "/students/student-s3/unarchive"},
		{name: "S3_anonymiseStudent", method: http.MethodPost, path: "/students/student-s3/anonymise", body: `{"confirm":true}`},
		{name: "S3_createConsentLink", method: http.MethodPost, path: "/students/student-s3/consent-links", body: `{"kind":"parent"}`},
		{name: "S3_cancelConsentRevoke", method: http.MethodPost, path: "/students/student-s3/consent/cancel-revoke"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := s3MountCall(handler, row.method, row.path, row.body)
			expectS3Mounted(t, rec)
		})
	}
}

func s3MountCall(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func expectS3Mounted(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if strings.Contains(rec.Header().Get("Content-Type"), "text/html") || strings.Contains(rec.Body.String(), `id="root"`) {
		t.Fatalf("ответ SPA, код %d, content-type %q, тело %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec.Code == http.StatusNotFound {
		t.Fatalf("404 неизвестного маршрута, content-type %q, тело %s", rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
