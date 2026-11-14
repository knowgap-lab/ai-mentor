package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const startPhrase = "Это бот платформы подготовки: сюда приходят уведомления и ссылки на ДЗ"

func (s *Service) webhook(w http.ResponseWriter, r *http.Request) {
	if !secretMatch(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), s.secret) {
		writeAPIErr(w, errForbidden())
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	if isStart(body) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, startPhrase)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func secretMatch(got, want string) bool {
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func isStart(body []byte) bool {
	var upd struct {
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	if json.Unmarshal(body, &upd) != nil {
		return false
	}
	cmd, _, _ := strings.Cut(strings.TrimSpace(upd.Message.Text), " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return cmd == "/start"
}

func writeAPIErr(w http.ResponseWriter, api *Error) {
	if api == nil {
		api = errForbidden()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(api.HTTPStatus)
	_, _ = w.Write(api.Body())
}
