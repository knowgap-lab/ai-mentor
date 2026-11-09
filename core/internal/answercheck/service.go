// Пакет answercheck принимает один ответ ученика на элемент выдачи.
// Оценка — answerrules.Score. Журнал пишется в таблицы миграции 02001.
package answercheck

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "__Host-session"

const (
	solutionBlock = "ct-math:M10.B09"
	variantID     = "ct-math:M10.T01.S1.V001"
	rulesVersion  = "rikz-2026"
	integerHint   = "введите целое число"
)

// Config — параметры сервиса. В тестах задан только DSN.
type Config struct {
	DSN string
}

// World — идентификаторы, которые Seed подготовил для сценария.
type World struct {
	AssignmentID    string
	ItemID          string
	StudentID       string
	ActorTelegramID string
	ActorRole       string
}

// Service обслуживает POST /assignments/{id}/items/{item}/answer.
type Service struct {
	pool    *pgxpool.Pool
	handler http.Handler
}

type actor struct {
	id   string
	role string
}

type httpError struct {
	status int
	code   string
	retry  int
}

func (e *httpError) Error() string {
	if e == nil {
		return ""
	}
	return e.code
}

func fail(status int, code string) error {
	return &httpError{status: status, code: code}
}

// Open подключается к Postgres и собирает HTTP-обработчик.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	svc := &Service{pool: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /assignments/{id}/items/{item}/answer", svc.answer)
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

// Handler обслуживает приём ответа.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (actor, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(cookie.Value)) != 1 {
		writeCode(w, http.StatusForbidden, "csrf")
		return actor{}, false
	}
	hash, ok := hashToken(cookie.Value)
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	var user actor
	err = s.pool.QueryRow(r.Context(), `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
	`, hash).Scan(&user.id, &user.role)
	if errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return actor{}, false
	}
	return user, true
}

func hashToken(token string) ([]byte, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing json")
	}
	return nil
}

func writeCode(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code, "message": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"unavailable","message":"unavailable"}`))
		return
	}
	writeBytes(w, status, body)
}

func writeBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeHandlerErr(w http.ResponseWriter, err error) {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		if httpErr.retry > 0 {
			w.Header().Set("Retry-After", itoa(httpErr.retry))
		}
		writeCode(w, httpErr.status, httpErr.code)
		return
	}
	writeCode(w, http.StatusServiceUnavailable, "unavailable")
}
