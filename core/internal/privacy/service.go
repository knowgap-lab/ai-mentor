// Пакет privacy хранит согласие и ссылку, по которой его дают или отзывают.
// Сырой токен ссылки не попадает ни в базу, ни в журнал.
package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const consentTitle = "Согласие на обработку данных ученика"

// Config — параметры сервиса. В Log токен ссылки не пишется.
type Config struct {
	DSN string
	Log io.Writer
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

func errGone() *Error {
	return apiError(http.StatusGone, "consent_link_gone")
}

func errKind() *Error {
	return apiError(http.StatusUnprocessableEntity, "consent_kind_invalid")
}

func errMissing() *Error {
	return apiError(http.StatusLocked, "consent_missing")
}

func errUnavailable() *Error {
	return apiError(http.StatusServiceUnavailable, "unavailable")
}

func errForbidden() *Error {
	return apiError(http.StatusForbidden, "forbidden")
}

func errCSRF() *Error {
	return apiError(http.StatusForbidden, "csrf")
}

func errUnauthorized() *Error {
	return apiError(http.StatusUnauthorized, "unauthorized")
}

func errConfirmation() *Error {
	return apiError(http.StatusConflict, "confirmation_required")
}

// Service — ссылки, согласие и карточка ученика в Postgres.
type Service struct {
	pool        *pgxpool.Pool
	cfg         Config
	handler     http.Handler
	origin      string
	textVersion string
	version     int
	text        string
}

// Open подключается к Postgres, читает текст legal/consent-1.md и собирает HTTP-обработчик.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	versionText, text, err := loadConsentText()
	if err != nil {
		pool.Close()
		return nil, err
	}
	version, err := strconv.Atoi(versionText)
	if err != nil || version < 1 {
		pool.Close()
		return nil, fmt.Errorf("consent text version %q", versionText)
	}
	svc := &Service{
		pool:        pool,
		cfg:         cfg,
		origin:      appOrigin(),
		textVersion: versionText,
		version:     version,
		text:        text,
	}
	svc.handler = svc.routes()
	return svc, nil
}

func appOrigin() string {
	if origin := os.Getenv("APP_ORIGIN"); origin != "" {
		return origin
	}
	return "http://localhost"
}

func (s *Service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /consent/{token}", s.getConsent)
	mux.HandleFunc("POST /consent/{token}", s.postConsent)
	mux.HandleFunc("POST /students", s.createStudent)
	mux.HandleFunc("GET /students/{id}", s.getStudent)
	mux.HandleFunc("POST /students/{id}/archive", s.archiveStudent)
	mux.HandleFunc("POST /students/{id}/unarchive", s.unarchiveStudent)
	mux.HandleFunc("POST /students/{id}/anonymise", s.anonymiseStudent)
	mux.HandleFunc("POST /students/{id}/consent-links", s.createConsentLink)
	mux.HandleFunc("POST /students/{id}/consent/cancel-revoke", s.cancelConsentRevoke)
	return mux
}

// Close закрывает пул соединений.
func (s *Service) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// Handler обслуживает согласие и операции ученика.
// GET и POST /consent/{token} идут без сеанса. POST без Origin проходит (D307);
// чужой Origin отклоняется как AU-R24.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func loadConsentText() (version, text string, err error) {
	path, err := findConsentFile()
	if err != nil {
		return "", "", err
	}
	raw, err := os.ReadFile(path) //nolint:gosec // G304: путь из cwd и фиксированного имени legal/consent-1.md, не из запроса
	if err != nil {
		return "", "", err
	}
	version, text, err = splitFrontMatter(string(raw))
	if err != nil {
		return "", "", err
	}
	if !strings.Contains(text, consentTitle) {
		return "", "", errors.New("consent text has no title")
	}
	return version, text, nil
}

func findConsentFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "legal", "consent-1.md")
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("legal/consent-1.md not found")
		}
		dir = parent
	}
}

func splitFrontMatter(raw string) (version, body string, err error) {
	const fence = "---"
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(s, fence+"\n") {
		return "", "", errors.New("consent text has no front matter")
	}
	rest := s[len(fence)+1:]
	idx := strings.Index(rest, "\n"+fence+"\n")
	if idx < 0 {
		return "", "", errors.New("consent text front matter is not closed")
	}
	head := rest[:idx]
	body = strings.TrimSpace(rest[idx+len("\n"+fence+"\n"):])
	for line := range strings.SplitSeq(head, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "version" {
			version = strings.TrimSpace(val)
		}
	}
	if version == "" {
		return "", "", errors.New("consent text has no version")
	}
	return version, body, nil
}
