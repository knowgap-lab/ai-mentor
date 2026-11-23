package auth

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Service) listSessions(w http.ResponseWriter, r *http.Request) {
	actor, token, ok := s.actorFrom(w, r)
	if !ok {
		return
	}
	mine, okHash := hashToken(token)
	if !okHash {
		writeErr(w, errUnauthorized())
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT token_hash, created_at, expires_at, state
		FROM core.session
		WHERE user_id = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
		ORDER BY created_at
	`, actor.UserID)
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	defer rows.Close()

	items := make([]sessionJSON, 0)
	for rows.Next() {
		var hash []byte
		var created, expires time.Time
		var state string
		if err := rows.Scan(&hash, &created, &expires, &state); err != nil {
			writeErr(w, errUnavailable())
			return
		}
		item := sessionJSON{
			SessionID: hex.EncodeToString(hash),
			CreatedAt: created.UTC().Format(time.RFC3339Nano),
			ExpiresAt: expires.UTC().Format(time.RFC3339Nano),
			State:     state,
		}
		if string(hash) == string(mine) {
			current := true
			item.Current = &current
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Service) revokeAllSessions(w http.ResponseWriter, r *http.Request) {
	actor, token, ok := s.actorFrom(w, r)
	if !ok {
		return
	}
	if !s.csrfOK(w, r, token) {
		return
	}
	hash, okHash := hashToken(token)
	if !okHash {
		writeErr(w, errUnauthorized())
		return
	}
	if _, err := s.pool.Exec(r.Context(), `
		UPDATE core.session
		SET revoked_at = now()
		WHERE user_id = $1
		  AND revoked_at IS NULL
		  AND token_hash <> $2
	`, actor.UserID, hash); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) revokeStudentSessions(w http.ResponseWriter, r *http.Request) {
	actor, token, ok := s.actorFrom(w, r)
	if !ok {
		return
	}
	if !s.csrfOK(w, r, token) {
		return
	}
	studentID := r.PathValue("id")
	if actor.Role != "tutor" {
		writeErr(w, errForbidden())
		return
	}
	if err := s.Authorize(actor, "write", Object{UserID: studentID}); err != nil {
		writeServiceErr(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, err := tx.Exec(r.Context(), `
		UPDATE core.session
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, studentID); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if _, err := insertJournal(r.Context(), tx, "session_revoked", map[string]string{
		"user_id": studentID,
		"reason":  "tutor_revoked_all",
	}); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) unbindTelegram(w http.ResponseWriter, r *http.Request) {
	actor, token, ok := s.actorFrom(w, r)
	if !ok {
		return
	}
	if !s.csrfOK(w, r, token) {
		return
	}
	studentID := r.PathValue("id")
	if actor.Role != "tutor" {
		writeErr(w, errForbidden())
		return
	}
	if err := s.Authorize(actor, "write", Object{UserID: studentID}); err != nil {
		writeServiceErr(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, err := tx.Exec(r.Context(), `
		UPDATE core.student_pii SET telegram_id = '' WHERE student_id = $1
	`, studentID); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE core.user_pii SET telegram_id = '' WHERE user_id = $1
	`, studentID); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE core.session
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, studentID); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if _, err := insertJournal(r.Context(), tx, "session_revoked", map[string]string{
		"user_id": studentID,
		"reason":  "unbind",
	}); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, errUnavailable())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) actorFrom(w http.ResponseWriter, r *http.Request) (Actor, string, bool) {
	token, ok := cookieToken(r)
	if !ok {
		writeErr(w, errUnauthorized())
		return Actor{}, "", false
	}
	userID, role, alive, err := s.sessionUser(r.Context(), token)
	if err != nil {
		writeErr(w, errUnavailable())
		return Actor{}, "", false
	}
	if !alive {
		writeErr(w, errUnauthorized())
		return Actor{}, "", false
	}
	return Actor{UserID: userID, Role: role}, token, true
}

func (s *Service) csrfOK(w http.ResponseWriter, r *http.Request, token string) bool {
	if r.Header.Get("Origin") != s.cfg.AppOrigin || !sameToken(r.Header.Get("X-CSRF-Token"), token) {
		writeErr(w, errCSRF())
		return false
	}
	return true
}

type sessionJSON struct {
	SessionID string `json:"session_id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	State     string `json:"state"`
	Current   *bool  `json:"current,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
