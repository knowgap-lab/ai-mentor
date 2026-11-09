// Пакет auth обслуживает локальный вход, сеанс и проверку прав.
// Telegram, приглашения и подтверждение устройства репетитора сюда не входят.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config — параметры сервиса. Log не получает токен, cookie и telegram_id (AU-R37).
type Config struct {
	DSN       string
	Env       string
	DevMode   bool
	AppOrigin string
	Log       io.Writer
}

// Actor — кто выполняет действие. Role: student или tutor.
type Actor struct {
	UserID string
	Role   string
}

// Object — ученик, к которому обращаются.
type Object struct {
	UserID string
}

// Error — отказ API. Body совпадает со схемой Error контракта.
type Error struct {
	HTTPStatus int
	Code       string
	message    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *Error) Body() []byte {
	code := "unavailable"
	message := code
	if e != nil && e.Code != "" {
		code = e.Code
		message = e.message
		if message == "" {
			message = code
		}
	}
	body, err := json.Marshal(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
	if err != nil {
		return []byte(`{"code":"unavailable","message":"unavailable"}`)
	}
	return body
}

func apiError(status int, code string) *Error {
	return &Error{HTTPStatus: status, Code: code, message: code}
}

func errForbidden() *Error { return apiError(http.StatusForbidden, "forbidden") }

func errCSRF() *Error { return apiError(http.StatusForbidden, "csrf") }

func errUnauthorized() *Error { return apiError(http.StatusUnauthorized, "unauthorized") }

func errUnavailable() *Error { return apiError(http.StatusServiceUnavailable, "unavailable") }

// RejectDevMode запрещает старт, если локальный вход включён не в env local (AU-R36).
func RejectDevMode(env string, enabled bool) error {
	if enabled && env != "local" {
		return fmt.Errorf("AUTH_DEV_MODE requires env local")
	}
	return nil
}

// Service — локальный вход и сеансы в Postgres.
type Service struct {
	pool    *pgxpool.Pool
	cfg     Config
	handler http.Handler
}

// Open подключается к Postgres и собирает HTTP-обработчик.
// Отказ старта при чужом окружении делает RejectDevMode, не Open:
// выключенный или запрещённый вход отвечает 403 уже из обработчика.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	svc := &Service{pool: pool, cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/dev", svc.loginDev)
	mux.HandleFunc("POST /auth/logout", svc.logout)
	mux.HandleFunc("GET /me", svc.me)
	svc.handler = mux
	return svc, nil
}

// Close закрывает пул соединений.
func (s *Service) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// Handler обслуживает POST /auth/dev, POST /auth/logout и GET /me.
func (s *Service) Handler() http.Handler {
	return s.handler
}
