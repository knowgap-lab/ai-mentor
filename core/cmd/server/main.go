package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/opsobs"
	"github.com/knowgap-lab/ai-mentor/core/internal/recheck"
)

func newRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", healthz)
	return r
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := io.WriteString(w, "ok"); err != nil {
		slog.Error("healthz", "err", err.Error())
	}
}

func main() {
	slog.SetDefault(opsobs.New(os.Stdout))
	if err := validateServerEnv(); err != nil {
		slog.Error("server", "err", err.Error())
		os.Exit(1)
	}
	handler, err := rootHandler()
	if err != nil {
		slog.Error("server", "err", err.Error())
		os.Exit(1)
	}
	if devStackEnabled() {
		go driveRechecks(os.Getenv("DATABASE_URL"))
	}
	srv := &http.Server{
		Addr:              listenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server", "err", err.Error())
		os.Exit(1)
	}
}

func driveRechecks(dsn string) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := recheck.Resume(ctx, dsn)
		cancel()
		if err != nil {
			slog.Error("recheck", "err", err.Error())
		}
		time.Sleep(200 * time.Millisecond)
	}
}
