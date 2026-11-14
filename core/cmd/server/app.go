package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/answercheck"
	"github.com/knowgap-lab/ai-mentor/core/internal/attempts"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/compose"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

const sessionCookieName = "__Host-session"

// Читается страницей: сама cookie сеанса HttpOnly и из скрипта не видна.
const sessionSeenCookie = "am-session"

func rootHandler() (http.Handler, error) {
	if !devStackEnabled() {
		return withCSP(newRouter()), nil
	}
	handler, err := devHandler(newRouter())
	if err != nil {
		return nil, err
	}
	return withCSP(handler), nil
}

func devStackEnabled() bool {
	return os.Getenv("ENV") == "local" &&
		os.Getenv("AUTH_DEV_MODE") == "true" &&
		os.Getenv("DATABASE_URL") != ""
}

func listenAddr() string {
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr
	}
	return ":8080"
}

func appOrigin() string {
	if origin := os.Getenv("APP_ORIGIN"); origin != "" {
		return origin
	}
	addr := listenAddr()
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://localhost"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func devHandler(health http.Handler) (http.Handler, error) {
	dsn := os.Getenv("DATABASE_URL")
	origin := appOrigin()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	authSvc, err := auth.Open(ctx, auth.Config{
		DSN:       dsn,
		Env:       os.Getenv("ENV"),
		DevMode:   true,
		AppOrigin: origin,
		Log:       io.Discard,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	answerSvc, err := answercheck.Open(ctx, answercheck.Config{
		DSN:       dsn,
		AppOrigin: origin,
	})
	if err != nil {
		_ = authSvc.Close()
		return nil, fmt.Errorf("answercheck: %w", err)
	}
	attemptSvc, err := attempts.Open(ctx, attempts.Config{DSN: dsn})
	if err != nil {
		_ = answerSvc.Close()
		_ = authSvc.Close()
		return nil, fmt.Errorf("attempts: %w", err)
	}
	root, err := webRoot()
	if err != nil {
		_ = attemptSvc.Close()
		_ = answerSvc.Close()
		_ = authSvc.Close()
		return nil, err
	}
	fallback := compose.Handler(dsn, authSvc.Handler(), answerSvc.Handler(), attemptSvc.Handler())
	api, err := mountLocal(ctx, dsn, origin, fallback)
	if err != nil {
		_ = attemptSvc.Close()
		_ = answerSvc.Close()
		_ = authSvc.Close()
		return nil, err
	}
	web := spaHandler{root: root, files: http.FileServer(http.Dir(root))}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			health.ServeHTTP(w, r)
			return
		}
		if isAPI(r.URL.Path) {
			if token, ok := requestSession(r); ok {
				w.Header().Set("X-CSRF-Token", token)
			}
			api.ServeHTTP(w, r)
			return
		}
		web.ServeHTTP(w, r)
	}), nil
}

func withCSP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}

func isAPI(urlPath string) bool {
	p := path.Clean("/" + urlPath)
	switch {
	case p == "/healthz" || p == "/me" || p == "/status":
		return true
	case p == "/auth" || strings.HasPrefix(p, "/auth/"):
		return true
	case p == "/students" || strings.HasPrefix(p, "/students/"):
		return true
	case p == "/assignments" || strings.HasPrefix(p, "/assignments/"):
		return true
	case p == "/attempts" || strings.HasPrefix(p, "/attempts/"):
		return true
	case p == "/lessons" || strings.HasPrefix(p, "/lessons/"):
		return true
	case p == "/consent" || strings.HasPrefix(p, "/consent/"):
		return true
	case p == "/invite" || strings.HasPrefix(p, "/invite/"):
		return true
	case p == "/me/sessions" || strings.HasPrefix(p, "/me/sessions/"):
		return true
	case p == "/rechecks" || strings.HasPrefix(p, "/rechecks/"):
		return true
	case p == "/tests" || strings.HasPrefix(p, "/tests/"):
		return true
	case p == "/telegram" || strings.HasPrefix(p, "/telegram/"):
		return true
	case p == "/internal" || strings.HasPrefix(p, "/internal/"):
		return true
	default:
		return false
	}
}

func requestSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func webRoot() (string, error) {
	dir := os.Getenv("WEB_DIST")
	if dir == "" {
		found, err := findWebDist()
		if err != nil {
			return "", err
		}
		dir = found
	}
	info, err := os.Stat(filepath.Join(dir, "index.html")) //nolint:gosec // G703: WEB_DIST или путь от cwd, не ввод запроса
	if err != nil {
		return "", fmt.Errorf("web dist %s: %w", dir, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("web dist %s: index.html is a directory", dir)
	}
	return dir, nil
}

func findWebDist() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("web dist: %w", err)
	}
	dir := start
	for {
		candidate := filepath.Join(dir, "web", "dist")
		info, statErr := os.Stat(filepath.Join(candidate, "index.html"))
		if statErr == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("web dist not found from %s", start)
		}
		dir = parent
	}
}

type spaHandler struct {
	root  string
	files http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if path.Clean(r.URL.Path) == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if rel == "" || rel == "." {
		h.serveIndex(w, r)
		return
	}
	full := filepath.Join(h.root, filepath.FromSlash(rel))
	relative, err := filepath.Rel(h.root, full)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(full) //nolint:gosec // G703: filepath.Rel выше уже не пускает путь вне h.root
	if err == nil && !info.IsDir() {
		h.files.ServeHTTP(w, r)
		return
	}
	if path.Ext(rel) != "" {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	markSession(w, r)
	http.ServeFile(w, r, filepath.Join(h.root, "index.html"))
}

func markSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := requestSession(r); ok {
		http.SetCookie(w, &http.Cookie{
			Name:     sessionSeenCookie,
			Value:    "1",
			Path:     "/",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionSeenCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
