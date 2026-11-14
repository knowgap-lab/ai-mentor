package attempts

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

// Service обслуживает HTTP журнала попыток.
type Service struct {
	pool    *pgxpool.Pool
	cfg     Config
	handler http.Handler
}

type actor struct {
	id   string
	role string
}

type httpError struct {
	status  int
	code    string
	details map[string]any
}

func (e *httpError) Error() string {
	if e == nil {
		return ""
	}
	return e.code
}

func fail(status int, code string, details map[string]any) error {
	return &httpError{status: status, code: code, details: details}
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
	svc := &Service{pool: pool, cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assignments/{id}", svc.getAssignment)
	mux.HandleFunc("POST /assignments/{id}/withdraw", svc.withdrawAssignment)
	mux.HandleFunc("PUT /assignments/{id}/deadline", svc.extendDeadline)
	mux.HandleFunc("POST /assignments/{id}/submit", svc.submitAssignment)
	mux.HandleFunc("GET /assignments/{id}/summary", svc.getSummary)
	mux.HandleFunc("POST /assignments/{id}/items/{item}/did-not-understand", svc.didNotUnderstand)
	mux.HandleFunc("POST /assignments/{id}/items/{item}/dispute", svc.openDispute)
	mux.HandleFunc("GET /assignments/{id}/attempts", svc.listAttempts)
	mux.HandleFunc("POST /attempts/{attempt}/manual-verdict", svc.manualVerdict)
	mux.HandleFunc("GET /students/{id}/disputes", svc.listDisputes)
	mux.HandleFunc("GET /students/{id}/discuss-list", svc.getDiscussList)
	mux.HandleFunc("PATCH /lessons/{lesson}/homework-draft", svc.patchHomeworkDraft)
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

// Handler обслуживает маршруты выдачи, попыток и черновика.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (actor, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized", nil)
		return actor{}, false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(cookie.Value)) != 1 {
			writeCode(w, http.StatusForbidden, "csrf", nil)
			return actor{}, false
		}
		if r.Header.Get("Origin") != s.allowedOrigin() {
			writeCode(w, http.StatusForbidden, "csrf", nil)
			return actor{}, false
		}
	}
	hash, ok := hashToken(cookie.Value)
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthorized", nil)
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
		writeCode(w, http.StatusUnauthorized, "unauthorized", nil)
		return actor{}, false
	}
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable", nil)
		return actor{}, false
	}
	return user, true
}

func (s *Service) allowedOrigin() string {
	if s.cfg.AppOrigin != "" {
		return s.cfg.AppOrigin
	}
	return appOrigin
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

func writeCode(w http.ResponseWriter, status int, code string, details map[string]any) {
	payload := struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	}{Code: code, Message: code, Details: details}
	writeJSON(w, status, payload)
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

func writeResult(w http.ResponseWriter, status int, body []byte) {
	if status == http.StatusNoContent || len(body) == 0 {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeHandlerErr(w http.ResponseWriter, err error) {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		writeCode(w, httpErr.status, httpErr.code, httpErr.details)
		return
	}
	writeCode(w, http.StatusServiceUnavailable, "unavailable", nil)
}

type assignmentHead struct {
	studentID  string
	state      string
	kind       string
	ownerTutor string
}

func loadAssignment(ctx context.Context, q queryer, id string) (assignmentHead, error) {
	var head assignmentHead
	err := q.QueryRow(ctx, `
		SELECT a.student_id, a.state, a.kind, COALESCE(u.owner_tutor_id, '')
		FROM core.assignment AS a
		JOIN core.app_user AS u ON u.id = a.student_id
		WHERE a.id = $1
	`, id).Scan(&head.studentID, &head.state, &head.kind, &head.ownerTutor)
	if errors.Is(err, pgx.ErrNoRows) {
		return assignmentHead{}, fail(http.StatusForbidden, "forbidden", nil)
	}
	if err != nil {
		return assignmentHead{}, err
	}
	return head, nil
}

func canRead(user actor, head assignmentHead) bool {
	switch user.role {
	case "student":
		return user.id == head.studentID
	case "tutor":
		return head.ownerTutor == user.id
	default:
		return false
	}
}

func canTutor(user actor, head assignmentHead) bool {
	return user.role == "tutor" && head.ownerTutor == user.id
}
