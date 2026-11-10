package telegram_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/telegram"
)

// Check — чистая функция: эти тесты не поднимают базу и не открывают сеть.
// Подпись считается здесь тем же алгоритмом, токен bot-demo-token.
// Строка проверки: все ключи, кроме hash, по алфавиту, «ключ=значение» через \n.
// Ключ HMAC — SHA-256 от байтов токена, не от их hex.

const (
	botToken         = "bot-demo-token"
	firstName        = "ИмяДемо"
	lastName         = "ФамилияДемо"
	userName         = "user_demo_g5"
	photoURL         = "https://example.test/photo-demo-g5.jpg"
	hashSentinel     = "hash-demo-not-stored"
	authDateSentinel = "auth-date-demo"
	demoTelegramID   = "424242"
)

func TestAU_R12_AcceptsSignedWidget(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, nil)
	expectNil(t, telegram.Check(data, botToken, now))
}

func TestAU_R12_RejectsWrongHash(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, nil)
	data["hash"] = flipHex(data["hash"])
	expectAPI(t, telegram.Check(data, botToken, now), http.StatusUnauthorized, "login_invalid")
}

func TestAU_R12_RejectsMissingHash(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, nil)
	delete(data, "hash")
	expectAPI(t, telegram.Check(data, botToken, now), http.StatusUnauthorized, "login_invalid")
}

func TestAU_R12_RejectsHashKeyedByHexDigest(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, nil)
	data["hash"] = loginHashHexKey(data, botToken)
	expectAPI(t, telegram.Check(data, botToken, now), http.StatusUnauthorized, "login_invalid")
}

func TestAU_R12_RejectsUnsortedCheckString(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, nil)
	data["hash"] = loginHashUnsorted(data, botToken)
	expectAPI(t, telegram.Check(data, botToken, now), http.StatusUnauthorized, "login_invalid")
}

func TestAU_R12_SignsInviteTokenWhenPresent(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, map[string]string{"invite_token": "invite-demo-token"})
	expectNil(t, telegram.Check(data, botToken, now))

	forged := cloneMap(data)
	forged["hash"] = loginHash(withoutKey(data, "invite_token"), botToken)
	expectAPI(t, telegram.Check(forged, botToken, now), http.StatusUnauthorized, "login_invalid")
}

func TestAU_R12_SignsPresentEmptyField(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, map[string]string{"empty_field": ""})
	expectNil(t, telegram.Check(data, botToken, now))

	forged := cloneMap(data)
	forged["hash"] = loginHash(withoutKey(data, "empty_field"), botToken)
	expectAPI(t, telegram.Check(forged, botToken, now), http.StatusUnauthorized, "login_invalid")
}

func TestAU_R13_AcceptsAuthDate299SecondsAgo(t *testing.T) {
	expectNil(t, checkAge(t, 299*time.Second))
}

func TestAU_R13_AcceptsAuthDate300SecondsAgo(t *testing.T) {
	expectNil(t, checkAge(t, 300*time.Second))
}

func TestAU_R13_RejectsAuthDate301SecondsAgo(t *testing.T) {
	expectAPI(t, checkAge(t, 301*time.Second), http.StatusUnauthorized, "login_stale")
}

func TestAU_R15_CheckKeepsProfileOutOfResult(t *testing.T) {
	now := fixedNow()
	data := signedWidget(now, 10*time.Second, nil)
	before := cloneMap(data)
	logs := captureLogs(t)

	expectNil(t, telegram.Check(data, botToken, now))
	if !mapsEqual(data, before) {
		t.Fatal("Check изменил данные входа")
	}
	assertProfileHidden(t, logs.String())

	bad := cloneMap(data)
	bad["hash"] = flipHex(bad["hash"])
	err := telegram.Check(bad, botToken, now)
	expectAPI(t, err, http.StatusUnauthorized, "login_invalid")
	assertProfileHidden(t, err.Error())
	var api *telegram.Error
	if !errors.As(err, &api) || api == nil {
		t.Fatal("нет *telegram.Error")
	}
	assertProfileHidden(t, string(api.Body()))
}

func fixedNow() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}

func checkAge(t *testing.T, age time.Duration) error {
	t.Helper()
	now := fixedNow()
	data := signedWidget(now, age, nil)
	return telegram.Check(data, botToken, now)
}

func signedWidget(now time.Time, age time.Duration, extra map[string]string) map[string]string {
	data := map[string]string{
		"id":         demoTelegramID,
		"auth_date":  strconv.FormatInt(now.Add(-age).Unix(), 10),
		"first_name": firstName,
		"last_name":  lastName,
		"username":   userName,
		"photo_url":  photoURL,
	}
	for key, value := range extra {
		if key == "hash" {
			continue
		}
		data[key] = value
	}
	data["hash"] = loginHash(data, botToken)
	return data
}

func loginData(telegramID string) map[string]string {
	return map[string]string{
		"id":         telegramID,
		"auth_date":  authDateSentinel,
		"first_name": firstName,
		"last_name":  lastName,
		"username":   userName,
		"photo_url":  photoURL,
		"hash":       hashSentinel,
	}
}

func loginHash(data map[string]string, token string) string {
	sum := sha256.Sum256([]byte(token))
	return macHex(checkString(data, false), sum[:])
}

func loginHashHexKey(data map[string]string, token string) string {
	sum := sha256.Sum256([]byte(token))
	return macHex(checkString(data, false), []byte(hex.EncodeToString(sum[:])))
}

func loginHashUnsorted(data map[string]string, token string) string {
	sum := sha256.Sum256([]byte(token))
	return macHex(checkString(data, true), sum[:])
}

func checkString(data map[string]string, reverse bool) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if reverse {
		for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
			keys[i], keys[j] = keys[j], keys[i]
		}
	}
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + data[key]
	}
	return strings.Join(parts, "\n")
}

func macHex(message string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func flipHex(value string) string {
	raw := []byte(value)
	last := len(raw) - 1
	if raw[last] == '0' {
		raw[last] = '1'
		return string(raw)
	}
	raw[last] = '0'
	return string(raw)
}

func withoutKey(data map[string]string, key string) map[string]string {
	out := cloneMap(data)
	delete(out, key)
	return out
}

func cloneMap(data map[string]string) map[string]string {
	out := make(map[string]string, len(data))
	for key, value := range data {
		out[key] = value
	}
	return out
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func profileSecrets() []string {
	return []string{firstName, lastName, userName, photoURL}
}

func loginSecrets() []string {
	return append(profileSecrets(), hashSentinel, authDateSentinel)
}

func assertProfileHidden(t *testing.T, text string) {
	t.Helper()
	assertSecretsHidden(t, text, profileSecrets())
}

func assertSecretsHidden(t *testing.T, text string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			t.Fatalf("наружу вышло поле данных входа, фрагмент %q", secret)
		}
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	logs := &bytes.Buffer{}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	log.SetOutput(logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
	})
	return logs
}

func expectNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("ожидался успех, получено %v", err)
	}
}

func expectAPI(t *testing.T, err error, status int, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка %s", code)
	}
	var api *telegram.Error
	if !errors.As(err, &api) || api == nil {
		t.Fatalf("ожидалась *telegram.Error, получено %T %v", err, err)
	}
	if api.HTTPStatus != status || api.Code != code {
		t.Fatalf("status=%d code=%s, нужны %d и %s", api.HTTPStatus, api.Code, status, code)
	}
	if api.Error() != code {
		t.Fatalf("Error() = %q, нужен %s", api.Error(), code)
	}
	if got := errorCode(t, api.Body()); got != code {
		t.Fatalf("тело code=%s, нужен %s", got, code)
	}
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var raw struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("Error: %v тело %s", err, body)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("тело Error: %v", err)
	}
	if raw.Code == "" {
		t.Fatal("в теле нет code")
	}
	trimmed := bytes.TrimSpace(raw.Details)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.HasPrefix(trimmed, []byte("{")) {
		t.Fatal("details не объект")
	}
	return raw.Code
}
