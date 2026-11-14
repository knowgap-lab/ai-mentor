package telegram_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Webhook — POST /telegram/webhook на Handler() этого пакета (NT-R19, NT-R20).
// Сеть к Telegram не открывается. Секрет — фикстура, не боевой токен.
//
// ВОПРОС: AU-R30. Контракт telegramWebhook не задаёт callback_data кнопок «Это я» и «Это не я». Нажатие не проверяю.
// ВОПРОС: код JSON у 403 webhook в таблице ошибок не назван. Проверяю статус 403 и схему Error с непустым code, имя code не фиксирую.
// Ответ на /start наблюдаю локально: тело HTTP, строки базы или журнал процесса. Сеть к Telegram запрещена.

const (
	webhookSecret = "webhook-demo-secret"
	startPhrase   = "Это бот платформы подготовки: сюда приходят уведомления и ссылки на ДЗ"
	startFirst    = "ИмяВебхукДемо"
	startUser     = "user_demo_s2_start"
	otherFirst    = "ИмяЧужоеДемо"
	otherUser     = "user_demo_s2_other"
)

func TestNT_R19_notifications16_MissingOrWrongSecret(t *testing.T) {
	h := openWebhook(t)
	body := startUpdate()
	wrong := "webhook-demo-wrong"
	cases := []struct {
		name   string
		secret *string
	}{
		{name: "missing", secret: nil},
		{name: "empty", secret: new(string)},
		{name: "wrong", secret: &wrong},
	}
	for _, tc := range cases {
		rec := h.postWebhook(tc.secret, body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: статус %d, нужен 403, тело %s", tc.name, rec.Code, rec.Body.Bytes())
		}
		if got := errorCode(t, rec.Body.Bytes()); got == "" {
			t.Fatalf("%s: в теле 403 нет code", tc.name)
		}
		if strings.Contains(rec.Body.String(), webhookSecret) || strings.Contains(rec.Body.String(), wrong) {
			t.Fatalf("%s: тело содержит секрет webhook", tc.name)
		}
		if strings.Contains(h.logs.String(), webhookSecret) || strings.Contains(h.logs.String(), wrong) {
			t.Fatalf("%s: журнал содержит секрет webhook", tc.name)
		}
	}
}

func TestNT_R19_OtherMessageDoesNotChangeState(t *testing.T) {
	h := openWebhook(t)
	before := h.snapshot()
	outbox := h.count(`SELECT count(*) FROM core.outbox`)
	journal := h.count(`SELECT count(*) FROM core.journal_event`)
	body := []byte(`{"update_id":2,"message":{"message_id":11,"text":"не-команда-демо","chat":{"id":4242002,"type":"private"},"from":{"id":4242002,"is_bot":false,"first_name":"` + otherFirst + `","username":"` + otherUser + `"}}}`)
	secret := webhookSecret
	rec := h.postWebhook(&secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("постороннее сообщение: %d %s", rec.Code, rec.Body.Bytes())
	}
	if h.snapshot() != before {
		t.Fatal("постороннее сообщение изменило приглашения, ПДн или сеансы")
	}
	if h.count(`SELECT count(*) FROM core.outbox`) != outbox || h.count(`SELECT count(*) FROM core.journal_event`) != journal {
		t.Fatal("постороннее сообщение записало outbox или журнал")
	}
	text := rec.Body.String() + h.storedText()
	if strings.Contains(text, otherFirst) || strings.Contains(text, otherUser) {
		t.Fatal("постороннее сообщение отдало данные пользователя")
	}
}

func TestNT_R20_StartReplyIsOnlyThePhrase(t *testing.T) {
	h := openWebhook(t)
	secret := webhookSecret
	rec := h.postWebhook(&secret, startUpdate())
	if rec.Code != http.StatusOK {
		t.Fatalf("/start: %d %s", rec.Code, rec.Body.Bytes())
	}
	reply := rec.Body.String() + "\n" + h.storedText()
	observed := reply + "\n" + h.logs.String()
	if !strings.Contains(observed, startPhrase) {
		t.Fatal("ответ на /start не содержит фразу платформы")
	}
	if strings.Contains(reply, startFirst) || strings.Contains(reply, startUser) {
		t.Fatal("ответ на /start содержит данные пользователя")
	}
}

func TestNotifications16_ExampleRowIsBound(t *testing.T) {
	raw := readRepo(t, "docs", "platform", "notifications.examples.yaml")
	var rows []struct {
		ID   string `yaml:"id"`
		Rule string `yaml:"rule"`
	}
	if err := yaml.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range rows {
		if row.ID == "notifications-16" && row.Rule == "NT-R19" {
			found = true
		}
	}
	if !found {
		t.Fatal("в notifications.examples.yaml нет notifications-16 / NT-R19")
	}
	src := currentFile(t)
	if !bytes.Contains(src, []byte("notifications-16")) {
		t.Fatal("строка notifications-16 не привязана к тесту")
	}
}

func openWebhook(t *testing.T) *harness {
	t.Helper()
	prev, ok := os.LookupEnv("TELEGRAM_WEBHOOK_SECRET")
	if err := os.Setenv("TELEGRAM_WEBHOOK_SECRET", webhookSecret); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv("TELEGRAM_WEBHOOK_SECRET", prev)
			return
		}
		_ = os.Unsetenv("TELEGRAM_WEBHOOK_SECRET")
	})
	return newHarness(t)
}

func (h *harness) postWebhook(secret *string, body []byte) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != nil {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", *secret)
	}
	rec := httptest.NewRecorder()
	h.webhookHandler().ServeHTTP(rec, req)
	return rec
}

func (h *harness) webhookHandler() http.Handler {
	h.t.Helper()
	src, ok := any(h.svc).(interface{ Handler() http.Handler })
	if !ok {
		h.t.Fatal("нет Handler(): операция telegramWebhook не подключена")
	}
	handler := src.Handler()
	if handler == nil {
		h.t.Fatal("Handler() пуст")
	}
	return handler
}

func (h *harness) storedText() string {
	h.t.Helper()
	return h.queryText(`
		SELECT coalesce((
			SELECT string_agg(
				coalesce(kind, '') || ' ' || coalesce(template, '') || ' ' ||
				coalesce(payload::text, '') || ' ' || coalesce(params::text, '') || ' ' ||
				coalesce(last_error, ''),
				E'\n'
			)
			FROM core.outbox
		), '')
		|| E'\n' ||
		coalesce((
			SELECT string_agg(kind || ' ' || payload::text, E'\n')
			FROM core.journal_event
		), '')
	`)
}

func startUpdate() []byte {
	return []byte(`{"update_id":1,"message":{"message_id":10,"text":"/start","chat":{"id":4242001,"type":"private"},"from":{"id":4242001,"is_bot":false,"first_name":"` + startFirst + `","username":"` + startUser + `"}}}`)
}

func readRepo(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{moduleDir(t), ".."}, parts...)...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func currentFile(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(moduleDir(t), "internal", "telegram", "telegram_http_test.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
