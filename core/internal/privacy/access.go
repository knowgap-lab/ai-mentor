package privacy

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

const sessionCookie = "__Host-session"

func (s *Service) consentOriginOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == s.origin
}

func (s *Service) tutor(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, role, ok := s.actor(w, r, true)
	if !ok {
		return "", false
	}
	if role != "tutor" {
		writeErr(w, errForbidden())
		return "", false
	}
	return id, true
}

func (s *Service) reader(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	return s.actor(w, r, false)
}

func (s *Service) actor(w http.ResponseWriter, r *http.Request, mutating bool) (string, string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeErr(w, errUnauthorized())
		return "", "", false
	}
	if mutating && !s.mutationOK(r, cookie.Value) {
		writeErr(w, errCSRF())
		return "", "", false
	}
	hash, ok := hashToken(cookie.Value)
	if !ok {
		writeErr(w, errUnauthorized())
		return "", "", false
	}
	var id, role string
	err = s.pool.QueryRow(r.Context(), `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
	`, hash).Scan(&id, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, errUnauthorized())
		return "", "", false
	}
	if err != nil {
		writeServiceErr(w, err)
		return "", "", false
	}
	return id, role, true
}

func (s *Service) mutationOK(r *http.Request, token string) bool {
	if r.Header.Get("Origin") != s.origin {
		return false
	}
	csrf := r.Header.Get("X-CSRF-Token")
	if csrf == "" || len(csrf) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(csrf), []byte(token)) == 1
}

func (s *Service) mustOwn(w http.ResponseWriter, r *http.Request, tutorID, studentID string) bool {
	var owner string
	err := s.pool.QueryRow(r.Context(), `
		SELECT coalesce(owner_tutor_id, '')
		FROM core.app_user
		WHERE id = $1 AND role = 'student'
	`, studentID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != tutorID) {
		writeErr(w, errForbidden())
		return false
	}
	if err != nil {
		writeServiceErr(w, err)
		return false
	}
	return true
}
