package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/knowgap-lab/ai-mentor/core/internal/conspect"
	"github.com/knowgap-lab/ai-mentor/core/internal/homework"
	"github.com/knowgap-lab/ai-mentor/core/internal/lessons"
	"github.com/knowgap-lab/ai-mentor/core/internal/plan"
	"github.com/knowgap-lab/ai-mentor/core/internal/privacy"
)

// mountLocal добавляет модули local-режима перед раздачей compose.
// Пути, которые журнал уже обслуживает, остаются в fallback.
func mountLocal(ctx context.Context, dsn, origin string, fallback http.Handler) (http.Handler, error) {
	var closeAll []func()
	ok := false
	defer func() {
		if ok {
			return
		}
		for i := len(closeAll) - 1; i >= 0; i-- {
			closeAll[i]()
		}
	}()

	lessonsSvc, err := lessons.Open(ctx, lessons.Config{
		DSN:       dsn,
		Now:       time.Now().UTC(),
		AppOrigin: origin,
	})
	if err != nil {
		return nil, fmt.Errorf("lessons: %w", err)
	}
	closeAll = append(closeAll, func() { _ = lessonsSvc.Close() })

	homeworkSvc, err := homework.Open(ctx, homework.Config{
		DSN:       dsn,
		AppOrigin: origin,
	})
	if err != nil {
		return nil, fmt.Errorf("homework: %w", err)
	}
	closeAll = append(closeAll, func() { _ = homeworkSvc.Close() })

	planSvc, err := plan.Open(ctx, plan.Config{
		DSN:       dsn,
		Now:       time.Now().UTC(),
		AppOrigin: origin,
	})
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	closeAll = append(closeAll, func() { _ = planSvc.Close() })

	conspectSvc, err := conspect.Open(ctx, conspect.Config{DSN: dsn})
	if err != nil {
		return nil, fmt.Errorf("conspect: %w", err)
	}
	closeAll = append(closeAll, func() { _ = conspectSvc.Close() })

	privacySvc, err := privacy.Open(ctx, privacy.Config{DSN: dsn, Log: io.Discard})
	if err != nil {
		return nil, fmt.Errorf("privacy: %w", err)
	}
	closeAll = append(closeAll, func() { _ = privacySvc.Close() })

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	closeAll = append(closeAll, pool.Close)
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	handler := newAPIMux(fallback, apiHandlers{
		lessons:  lessonsSvc.Handler(),
		homework: homeworkSvc.Handler(),
		plan:     planSvc.Handler(),
		conspect: conspectSvc.Handler(),
		privacy:  privacySvc.Handler(),
	}, pool)
	ok = true
	return handler, nil
}

type apiHandlers struct {
	lessons  http.Handler
	homework http.Handler
	plan     http.Handler
	conspect http.Handler
	privacy  http.Handler
}

func newAPIMux(fallback http.Handler, h apiHandlers, pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", statusHandler(pool))
	internal := internalHandler()
	mux.Handle("/internal/", internal)
	mux.Handle("/internal", internal)

	mux.Handle("GET /consent/{token}", h.privacy)
	mux.Handle("POST /consent/{token}", h.privacy)

	mux.Handle("PUT /students/{id}/schedules/{subject}", h.lessons)
	mux.Handle("GET /lessons/{lesson}", h.lessons)
	mux.Handle("POST /lessons/{lesson}/move", h.lessons)
	mux.Handle("POST /lessons/{lesson}/cancel", h.lessons)
	mux.Handle("POST /lessons/{lesson}/marks", h.lessons)

	// Тот же шаблон есть у lessons. По контракту это план.
	mux.Handle("GET /students/{id}/plans/{subject}", h.plan)
	mux.Handle("GET /students/{id}/plans/{subject}/divergence", h.plan)
	mux.Handle("PUT /students/{id}/plans/{subject}/order", h.plan)

	// PATCH этого пути остаётся журналу попыток через fallback.
	mux.HandleFunc("/lessons/{lesson}/homework-draft", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			h.homework.ServeHTTP(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
	mux.Handle("POST /lessons/{lesson}/homework-draft/issue", h.homework)

	mux.Handle("GET /lessons/{lesson}/conspect", h.conspect)
	mux.Handle("GET /lessons/{lesson}/conspect.pdf", h.conspect)

	mux.Handle("/", fallback)
	return mux
}

func statusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			writeCode(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	}
}

func internalHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !contentTokenOK(r) {
			writeCode(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeCode(w, http.StatusNotFound, "not_found")
	})
}

func contentTokenOK(r *http.Request) bool {
	want := os.Getenv("CONTENT_API_TOKEN")
	got := bearerToken(r.Header.Get("Authorization"))
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	same := subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
	return want != "" && got != "" && same
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return ""
	}
	return header[len(prefix):]
}

func writeCode(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"unavailable","message":"unavailable"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
