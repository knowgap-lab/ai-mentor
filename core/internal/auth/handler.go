package auth

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
)

const cookieName = "__Host-session"

func (s *Service) loginDev(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Env != "local" || !s.cfg.DevMode {
		writeErr(w, errForbidden())
		return
	}
	if r.Header.Get("Origin") != s.cfg.AppOrigin {
		writeErr(w, errCSRF())
		return
	}
	telegramID, role, ok := readDevBody(r)
	if !ok {
		writeErr(w, errForbidden())
		return
	}
	userID, token, err := s.openSession(r.Context(), telegramID, role)
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	http.SetCookie(w, sessionCookie(token))
	w.Header().Set("X-CSRF-Token", token)
	writeMe(w, userID, role)
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	token, ok := cookieToken(r)
	if !ok {
		writeErr(w, errUnauthorized())
		return
	}
	if _, _, alive, err := s.sessionUser(r.Context(), token); err != nil {
		writeErr(w, errUnavailable())
		return
	} else if !alive {
		writeErr(w, errUnauthorized())
		return
	}
	if r.Header.Get("Origin") != s.cfg.AppOrigin || !sameToken(r.Header.Get("X-CSRF-Token"), token) {
		writeErr(w, errCSRF())
		return
	}
	if err := s.revokeSession(r.Context(), token); err != nil {
		writeServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	token, ok := cookieToken(r)
	if !ok {
		writeErr(w, errUnauthorized())
		return
	}
	userID, role, alive, err := s.sessionUser(r.Context(), token)
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	if !alive {
		writeErr(w, errUnauthorized())
		return
	}
	writeMe(w, userID, role)
}

func readDevBody(r *http.Request) (string, string, bool) {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body struct {
		TelegramID *string `json:"telegram_id"`
		Role       *string `json:"role"`
	}
	if err := dec.Decode(&body); err != nil {
		return "", "", false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", "", false
	}
	if body.TelegramID == nil || body.Role == nil {
		return "", "", false
	}
	if *body.Role != "student" && *body.Role != "tutor" {
		return "", "", false
	}
	return *body.TelegramID, *body.Role, true
}

func sessionCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func cookieToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func sameToken(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func writeMe(w http.ResponseWriter, userID, role string) {
	body, err := json.Marshal(struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}{UserID: userID, Role: role})
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeErr(w http.ResponseWriter, api *Error) {
	if api == nil {
		api = errUnavailable()
	}
	// json.Marshal — санитайзер HTML-стока (gosec G705). RawMessage сохраняет
	// байты Body(): это тот же JSON, что отдаёт контракт Error.
	body, err := json.Marshal(json.RawMessage(api.Body()))
	if err != nil || len(body) == 0 || string(body) == "null" {
		body = []byte(`{"code":"unavailable","message":"unavailable"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(api.HTTPStatus)
	_, _ = w.Write(body)
}

func writeServiceErr(w http.ResponseWriter, err error) {
	if api, ok := err.(*Error); ok {
		writeErr(w, api)
		return
	}
	writeErr(w, errUnavailable())
}
