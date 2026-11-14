package telegram

import (
	"encoding/json"
	"net/http"
)

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

func errForbidden() *Error {
	return apiError(http.StatusForbidden, "forbidden")
}

func errLoginInvalid() *Error {
	return apiError(http.StatusUnauthorized, "login_invalid")
}

func errLoginStale() *Error {
	return apiError(http.StatusUnauthorized, "login_stale")
}

func errInviteGone() *Error {
	return apiError(http.StatusGone, "invite_gone")
}

func errTelegramBound() *Error {
	return apiError(http.StatusConflict, "telegram_already_bound")
}
