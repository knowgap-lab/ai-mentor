package main

import (
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
	h := spaHandler{root: dir, files: http.FileServer(http.Dir(dir))}

	withSession := httptest.NewRequest(http.MethodGet, "/", nil)
	withSession.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	seen := httptest.NewRecorder()
	h.ServeHTTP(seen, withSession)
	if strings.Contains(seen.Header().Get("Set-Cookie"), "am-session") {
		t.Fatalf("маркер ушёл в cookie: %s", seen.Header().Get("Set-Cookie"))
	}
	if !strings.Contains(seen.Body.String(), sessionMarker) {
		t.Fatalf("нет признака сеанса: %s", seen.Body.String())
	}

	anonymous := httptest.NewRecorder()
	h.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(anonymous.Body.String(), sessionMarker) {
		t.Fatalf("признак без сеанса: %s", anonymous.Body.String())
	}
	if !strings.Contains(anonymous.Body.String(), `id="root"`) {
		t.Fatalf("главная сломалась: %s", anonymous.Body.String())
	}
}
