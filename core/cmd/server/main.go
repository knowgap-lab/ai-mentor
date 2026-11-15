package main

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/opsobs"
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
	handler, err := rootHandler()
	if err != nil {
		slog.Error("server", "err", err.Error())
		os.Exit(1)
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
