package plan

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

const (
	sessionCookie = "__Host-session"
	appOrigin     = "http://localhost"
)

// Service показывает текущий план и принимает смену порядка.
type Service struct {
	pool    *pgxpool.Pool
	cfg     Config
	handler http.Handler
}

type actor struct {
	id   string
	role string
}

// Open подключается к Postgres. Часы раскладки — Config.Now.
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
	mux.HandleFunc("GET /students/{id}/plans/{subject}", svc.getPlan)
	mux.HandleFunc("GET /students/{id}/plans/{subject}/divergence", svc.getDivergence)
	mux.HandleFunc("PUT /students/{id}/plans/{subject}/order", svc.putOrder)
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

// Handler обслуживает план, смену порядка и отчёт расхождений.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (actor, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		origin := s.cfg.AppOrigin
		if origin == "" {
			origin = appOrigin
		}
		if r.Header.Get("Origin") != origin {
			writeCode(w, http.StatusForbidden, "csrf")
			return actor{}, false
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(cookie.Value)) != 1 {
			writeCode(w, http.StatusForbidden, "csrf")
			return actor{}, false
		}
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

func writeHandlerErr(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		message := api.code
		if api.status == http.StatusServiceUnavailable && api.detail != "" {
			message = api.detail
		}
		writeJSON(w, api.status, struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{Code: api.code, Message: message})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: "unavailable", Message: err.Error()})
}

func (s *Service) getPlan(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	studentID := r.PathValue("id")
	subject := r.PathValue("subject")
	if err := allowStudent(r.Context(), s.pool, user.id, studentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	view, err := buildPlan(r.Context(), s.pool, studentID, subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Service) getDivergence(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	studentID := r.PathValue("id")
	subject := r.PathValue("subject")
	if err := allowStudent(r.Context(), s.pool, user.id, studentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	report, err := buildDivergence(r.Context(), s.pool, studentID, subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Service) putOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	studentID := r.PathValue("id")
	subject := r.PathValue("subject")
	var body struct {
		LessonIDs []string `json:"lesson_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if err := allowStudent(r.Context(), tx, user.id, studentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := reorder(r.Context(), tx, studentID, subject, body.LessonIDs); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	view, err := buildPlan(r.Context(), s.pool, studentID, subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}
