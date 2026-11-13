package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxAuthAge — сколько секунд данные входа ещё считаются свежими (AU-R13).
// Ровно 300 секунд принимается; старше — нет.
const maxAuthAge = 300

// Check сверяет подпись виджета и свежесть auth_date.
// Строка проверки — все поля, кроме hash, по алфавиту, «ключ=значение» через перевод строки.
// Ключ HMAC — SHA-256 от байтов токена бота, не от их hex.
// Сеть не вызывается, карта data не меняется, профиль никуда не пишется.
func Check(data map[string]string, botToken string, now time.Time) error {
	if !signed(data, botToken) {
		return errLoginInvalid()
	}
	authUnix, err := strconv.ParseInt(data["auth_date"], 10, 64)
	if err != nil {
		return errLoginInvalid()
	}
	if now.Unix()-authUnix > maxAuthAge {
		return errLoginStale()
	}
	return nil
}

func signed(data map[string]string, botToken string) bool {
	gotHex, ok := data["hash"]
	if !ok || gotHex == "" {
		return false
	}
	got, err := hex.DecodeString(gotHex)
	if err != nil || len(got) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, sum[:])
	_, _ = mac.Write([]byte(checkString(data)))
	return hmac.Equal(mac.Sum(nil), got)
}

func checkString(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + data[key]
	}
	return strings.Join(parts, "\n")
}
