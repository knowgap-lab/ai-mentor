package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeIndexSessionMarker(t *testing.T) {
	dir := t.TempDir()
	const page = "<!doctype html><html><head></head><body><div id=\"root\"></div></body></html>"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	h := spaHandler{
		root:  dir,
		files: http.FileServer(http.Dir(dir)),
		live: func(_ context.Context, token string) (bool, error) {
			if token == "dead" {
				return false, nil
			}
			if token == "down" {
				return false, errors.New("db")
			}
			return token == "live", nil
		},
	}

	dead := httptest.NewRequest(http.MethodGet, "/", nil)
	dead.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "dead"})
	seen := httptest.NewRecorder()
	h.ServeHTTP(seen, dead)
	assertIndexCache(t, seen)
	if strings.Contains(seen.Header().Get("Set-Cookie"), "am-session") {
		t.Fatalf("маркер ушёл в cookie: %s", seen.Header().Get("Set-Cookie"))
	}
	if strings.Contains(seen.Body.String(), sessionMarker) {
		t.Fatalf("непустая cookie без живого сеанса: %s", seen.Body.String())
	}

	down := httptest.NewRequest(http.MethodGet, "/", nil)
	down.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "down"})
	failed := httptest.NewRecorder()
	h.ServeHTTP(failed, down)
	if strings.Contains(failed.Body.String(), sessionMarker) {
		t.Fatalf("ошибка проверки сеанса включила признак: %s", failed.Body.String())
	}

	live := httptest.NewRequest(http.MethodGet, "/", nil)
	live.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "live"})
	marked := httptest.NewRecorder()
	h.ServeHTTP(marked, live)
	assertIndexCache(t, marked)
	if !strings.Contains(marked.Body.String(), sessionMarker) {
		t.Fatalf("нет признака живого сеанса: %s", marked.Body.String())
	}

	anonymous := httptest.NewRecorder()
	h.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/", nil))
	assertIndexCache(t, anonymous)
	if strings.Contains(anonymous.Body.String(), sessionMarker) {
		t.Fatalf("признак без сеанса: %s", anonymous.Body.String())
	}
	if !strings.Contains(anonymous.Body.String(), `id="root"`) {
		t.Fatalf("главная сломалась: %s", anonymous.Body.String())
	}
}

func assertIndexCache(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Vary") != "Cookie" {
		t.Fatalf("Vary %q", rec.Header().Get("Vary"))
	}
}
