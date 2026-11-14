package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "__Host-session"

// Config — подключение. Адрес Gotenberg читается из окружения, не из Config.
type Config struct {
	DSN string
}

// Service печатает листок и бланк.
type Service struct {
	pool    *pgxpool.Pool
	handler http.Handler
}

type actor struct {
	id   string
	role string
}

// Open подключается к Postgres.
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
	mux.HandleFunc("GET /assignments/{id}/sheet.pdf", svc.getSheet)
	mux.HandleFunc("GET /tests/{test}/form.pdf", svc.getForm)
	svc.handler = mux
	return svc, nil
}

// Close закрывает пул соединений.
func (s *Service) Close() error {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// Handler обслуживает листок и бланк.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) getSheet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	statements, err := s.sheetStatements(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	body, err := Render(r.Context(), sheetHTML(statements))
	if err != nil {
		writePDFErr(w, err)
		return
	}
	writePDF(w, body)
}

func (s *Service) getForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticate(w, r); !ok {
		return
	}
	// Чужой доступ к бланку тест не задаёт: сеанс есть — печатаем ревизию каталога.
	title, err := s.formTitle(r.Context(), r.PathValue("test"))
	if err != nil {
		writeErr(w, err)
		return
	}
	body, err := Render(r.Context(), formHTML(title))
	if err != nil {
		writePDFErr(w, err)
		return
	}
	writePDF(w, body)
}

func (s *Service) sheetStatements(ctx context.Context, user actor, id string) ([]string, error) {
	var studentID, owner string
	err := s.pool.QueryRow(ctx, `
		SELECT a.student_id, COALESCE(u.owner_tutor_id, '')
		FROM core.assignment AS a
		JOIN core.app_user AS u ON u.id = a.student_id
		WHERE a.id = $1
	`, id).Scan(&studentID, &owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canReadSheet(user, studentID, owner)) {
		return nil, errForbidden()
	}
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT snapshot FROM core.assignment WHERE id = $1
	`, id).Scan(&raw); err != nil {
		return nil, err
	}
	var doc struct {
		Items []struct {
			Statement string `json:"statement"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(doc.Items))
	for _, item := range doc.Items {
		out = append(out, item.Statement)
	}
	return out, nil
}

func canReadSheet(user actor, studentID, owner string) bool {
	if user.role == "student" && user.id == studentID {
		return true
	}
	return user.role == "tutor" && user.id == owner
}

func (s *Service) formTitle(ctx context.Context, testID string) (string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data
		FROM catalog.revision
		WHERE entity_id = $1
		ORDER BY catalog_version_from DESC
		LIMIT 1
	`, testID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errForbidden()
	}
	if err != nil {
		return "", err
	}
	var doc struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	return doc.Title, nil
}

func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (actor, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
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

type apiError struct {
	status int
	code   string
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	return e.code
}

func errForbidden() error {
	return &apiError{status: http.StatusForbidden, code: "forbidden"}
}

func writePDFErr(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnavailable) {
		writeCode(w, http.StatusServiceUnavailable, "pdf_unavailable")
		return
	}
	writeErr(w, err)
}

func writeErr(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		writeCode(w, api.status, api.code)
		return
	}
	writeCode(w, http.StatusServiceUnavailable, "unavailable")
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

func writePDF(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
