package auth_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

// Новые операции висят на auth.Handler: POST /auth/telegram, GET /invite/{token},
// POST /students/{id}/invites, GET /me/sessions, POST /me/sessions/revoke-all,
// POST /students/{id}/revoke-sessions, POST /students/{id}/unbind-telegram.
// Подпись считается здесь по AU-R12. Токен бота — фикстура TELEGRAM_BOT_TOKEN.
// Файл auth.examples.yaml — текстовые строки, не сценарии given → steps → expect,
// поэтому scenario.Run его не исполняет. Строка привязана именем теста.
//
// ВОПРОС: AU-R19. request_access=write — поведение виджета на странице web. В TelegramLogin и операциях S2 этого поля нет. Не проверяю.
// ВОПРОС: AU-R5. auth.examples.yaml не перечисляет маршруты, только строки auth-01…auth-24. Обход таблицы прав не проверяю.
// ВОПРОС: AU-R18. Согласие outdated и needs_personal в пределах 14 дней (PV-R15, PV-R16) — вход ещё открыт или уже 423? Эти состояния не проверяю.
// ВОПРОС: AU-R17. Подпись и приглашение верны, а согласия нет: гасится ли приглашение? used_at не проверяю. Проверяю 423 и что сеанс не создан.
// ВОПРОС: AU-R30. Формат нажатия «Это не я» не задан. Отзыв по кнопке не проверяю.
// Срок pending (AU-R29) считаю от core.session.created_at. Сеанс pending отдаётся cookie __Host-session: иначе нельзя показать, что прав нет у этого сеанса.

const (
	botDemo         = "bot-demo-token"
	deviceCookie    = "__Host-device"
	inviteLife      = 72 * time.Hour
	expireSlack     = 15 * time.Second
	profileFirst    = "ИмяДемоС2"
	profileLast     = "ФамилияДемоС2"
	profileUser     = "user_demo_s2"
	profilePhoto    = "https://example.test/photo-demo-s2.jpg"
	queryMarker     = "query-only-marker-s2"
	queryTelegramID = "id-query-only-909090"
)

type hashMode int

const (
	hashGood hashMode = iota
	hashBad
	hashMissing
)

type sessionItem struct {
	SessionID string  `json:"session_id"`
	Device    *string `json:"device"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt string  `json:"expires_at"`
	State     string  `json:"state"`
	Current   *bool   `json:"current"`
}

func TestAU_R4_auth01_auth02_ForeignAndMissingStudent(t *testing.T) {
	h, ip := openAuth(t)
	tutorA := h.loginDev(ip, "tg-s2-tutor-a", "tutor")
	tutorB := h.loginDev(ip, "tg-s2-tutor-b", "tutor")
	student := h.loginDev(ip, "tg-s2-student-actor", "student")
	const (
		owned   = "student-s2-owned"
		foreign = "student-s2-other"
		missing = "student-missing-s2"
	)
	h.seedOwnedStudent(tutorA.Me.UserID, owned, "НикСвой", "active")
	h.seedOwnedStudent(tutorB.Me.UserID, foreign, "НикЧужой", "active")
	paths := []string{
		"/students/%s/invites",
		"/students/%s/revoke-sessions",
		"/students/%s/unbind-telegram",
	}
	for _, pattern := range paths {
		foreignRec := h.postTutor(ip, fmt.Sprintf(pattern, foreign), tutorA)
		missingRec := h.postTutor(ip, fmt.Sprintf(pattern, missing), tutorA)
		expectAPI(t, foreignRec, http.StatusForbidden, "forbidden")
		expectAPI(t, missingRec, http.StatusForbidden, "forbidden")
		if canonicalJSON(t, foreignRec.Body.Bytes()) != canonicalJSON(t, missingRec.Body.Bytes()) {
			t.Fatalf("%s: чужой и несуществующий различаются: %s и %s", pattern, foreignRec.Body.Bytes(), missingRec.Body.Bytes())
		}
		bodyHides(t, foreignRec.Body.Bytes(), foreign, missing, owned)
	}
	rec := h.postTutor(ip, "/students/"+owned+"/invites", student)
	expectAPI(t, rec, http.StatusForbidden, "forbidden")
	if h.inviteCount(owned) != 0 {
		t.Fatal("чужой запрос создал приглашение")
	}
	if _, err := h.createInvite(ip, owned, tutorA); err != nil {
		t.Fatal(err)
	}
}

func TestAU_R8_auth04_InviteRequiresConsent(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-consent", "tutor")
	h.seedOwnedStudent(tutor.Me.UserID, "student-s2-no-consent", "НикБез", "")
	h.seedOwnedStudent(tutor.Me.UserID, "student-s2-revoked", "НикОтозван", "revoked")
	h.seedOwnedStudent(tutor.Me.UserID, "student-s2-consent", "НикЕсть", "active")

	for _, id := range []string{"student-s2-no-consent", "student-s2-revoked"} {
		rec := h.postTutor(ip, "/students/"+id+"/invites", tutor)
		expectAPI(t, rec, http.StatusLocked, "consent_missing")
		if h.inviteCount(id) != 0 {
			t.Fatalf("%s: приглашение создано без согласия", id)
		}
	}
	if _, err := h.createInvite(ip, "student-s2-consent", tutor); err != nil {
		t.Fatal(err)
	}
}

func TestAU_R9_InviteHashOnlyAndNotLogged(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-r9", "tutor")
	h.seedOwnedStudent(tutor.Me.UserID, "student-s2-r9a", "НикА", "active")
	h.seedOwnedStudent(tutor.Me.UserID, "student-s2-r9b", "НикБ", "active")
	first, err := h.createInvite(ip, "student-s2-r9a", tutor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.createInvite(ip, "student-s2-r9b", tutor)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("два приглашения получили один токен")
	}
	for _, token := range []string{first, second} {
		raw := mustDecode32(t, token)
		if !h.hashStored(raw, token) {
			t.Fatal("хеш приглашения не найден в базе")
		}
		h.assertSecretHidden(token, raw)
		h.logsHide(token)
	}
	if h.inviteCount("student-s2-r9a") != 1 || h.inviteCount("student-s2-r9b") != 1 {
		t.Fatal("у ученика не одно приглашение")
	}
	name, tg, ok := h.studentPII("student-s2-r9a")
	if !ok || name != "НикА" || tg != "" {
		t.Fatalf("выпуск приглашения изменил ПДн: %s %q %v", name, tg, ok)
	}
	if len(h.sessionsFor("student-s2-r9a")) != 0 {
		t.Fatal("выпуск приглашения создал сеанс")
	}
}

func TestAU_R10_auth05_ExpiredInviteIsGone(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-old", "tutor")
	const id = "student-s2-old"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикСтарый", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	h.exec(`UPDATE core.telegram_invite SET expires_at = now() - interval '1 hour' WHERE student_id = $1`, id)
	rec := h.getInvite(ip, token)
	expectAPI(t, rec, http.StatusGone, "invite_gone")
	body, _ := widgetBody(t, botDemo, "424205", time.Now().Add(-10*time.Second).Unix(), token, false, hashGood)
	login := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), nil)
	expectAPI(t, login, http.StatusGone, "invite_gone")
	if hasSessionCookie(login) {
		t.Fatal("просроченное приглашение выдало сеанс")
	}
	_, used, ok := h.findInvite(token)
	if !ok {
		t.Fatal("строка приглашения исчезла")
	}
	if used != nil {
		t.Fatal("просроченное приглашение помечено использованным")
	}
	if len(h.sessionsFor(id)) != 0 {
		t.Fatal("просроченное приглашение создало сеанс")
	}
}

func TestAU_R10_auth06_SecondUseIsGone(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-once", "tutor")
	const id = "student-s2-once"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикРаз", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, first := h.loginTelegram(ip, botDemo, "424206", 10*time.Second, token, false, "")
	if first.Me.UserID != id || first.Me.Role != "student" {
		t.Fatalf("Me = %+v", first.Me)
	}
	_, used, ok := h.findInvite(token)
	if !ok || used == nil {
		t.Fatal("первый вход не погасил приглашение")
	}
	againBody, _ := widgetBody(t, botDemo, "424206", time.Now().Add(-10*time.Second).Unix(), token, false, hashGood)
	again := h.callAs(http.MethodPost, "/auth/telegram", ip, againBody, ptr(appOrigin), nil)
	expectAPI(t, again, http.StatusGone, "invite_gone")
	if hasSessionCookie(again) {
		t.Fatal("повтор приглашения выдал сеанс")
	}
	if len(h.sessionsFor(id)) != 1 {
		t.Fatalf("сеансов %d, нужен один", len(h.sessionsFor(id)))
	}
	h.expectMe(first)
	page := h.getInvite(ip, token)
	expectAPI(t, page, http.StatusGone, "invite_gone")
}

func TestAU_R10_auth07_PreviewDoesNotChange(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-preview", "tutor")
	const id = "student-s2-preview"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикСтраница", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	beforeExp, beforeUsed, ok := h.findInvite(token)
	if !ok || beforeUsed != nil {
		t.Fatal("новое приглашение уже использовано")
	}
	sessions := len(h.sessionsFor(id))
	for i := 0; i < 2; i++ {
		rec := h.getInvite(ip, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET приглашения = %d %s", rec.Code, rec.Body.Bytes())
		}
		valid, name, hasName := decodeInvitePage(t, rec.Body.Bytes())
		if !valid || !hasName || name != "НикСтраница" {
			t.Fatalf("страница = valid:%v name:%q", valid, name)
		}
		exp, used, found := h.findInvite(token)
		if !found || used != nil || !exp.Equal(beforeExp) || len(h.sessionsFor(id)) != sessions {
			t.Fatal("открытие страницы изменило приглашение или сеансы")
		}
		nameNow, tg, piiOK := h.studentPII(id)
		if !piiOK || nameNow != "НикСтраница" || tg != "" {
			t.Fatal("открытие страницы изменило ПДн")
		}
	}
	h.logsHide(token)
}

func TestAU_R11_auth08_ReissueRevokesBindingAndSessions(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-reissue", "tutor")
	student := h.loginDev(ip, "tg-s2-student-reissue", "student")
	h.attachStudent(tutor.Me.UserID, student.Me.UserID, "НикПеревыпуск", "bind-s2-reissue")
	h.grantConsent(student.Me.UserID, "active")
	first, err := h.createInvite(ip, student.Me.UserID, tutor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.createInvite(ip, student.Me.UserID, tutor)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("перевыпуск вернул тот же токен")
	}
	expectAPI(t, h.getInvite(ip, first), http.StatusGone, "invite_gone")
	page := h.getInvite(ip, second)
	if page.Code != http.StatusOK {
		t.Fatalf("новое приглашение = %d %s", page.Code, page.Body.Bytes())
	}
	valid, _, _ := decodeInvitePage(t, page.Body.Bytes())
	if !valid {
		t.Fatal("новое приглашение не действует")
	}
	name, tg, ok := h.studentPII(student.Me.UserID)
	if !ok || name != "НикПеревыпуск" || tg != "" {
		t.Fatalf("привязка после перевыпуска: %s %q", name, tg)
	}
	if userTG, found := h.userTelegram(student.Me.UserID); found && userTG != "" {
		t.Fatalf("user_pii.telegram_id = %s", userTG)
	}
	if h.sessionByToken(student.Token).Revoked == nil {
		t.Fatal("перевыпуск не отозвал сеансы ученика")
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(student.Token))
	if h.countEvent("session_revoked", "invite_reissue") < 1 {
		t.Fatal("нет события session_revoked invite_reissue")
	}
	h.logsHide(first, second)
}

func TestAU_R12_auth09_HandlerRejectsBadSignature(t *testing.T) {
	h, ip := openAuth(t)
	now := time.Now().Add(-10 * time.Second).Unix()
	for _, mode := range []hashMode{hashBad, hashMissing} {
		body, _ := widgetBody(t, botDemo, "424209", now, "invite-demo-unused", true, mode)
		rec := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), nil)
		expectAPI(t, rec, http.StatusUnauthorized, "login_invalid")
		if hasSessionCookie(rec) {
			t.Fatal("неверная подпись выдала сеанс")
		}
	}
	if n := h.count(`SELECT count(*) FROM core.user_pii WHERE telegram_id = '424209'`) +
		h.count(`SELECT count(*) FROM core.student_pii WHERE telegram_id = '424209'`); n != 0 {
		t.Fatal("неверная подпись записала telegram_id")
	}
}

func TestAU_R13_auth10_AuthDateWindow(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-stale", "tutor")
	const id = "student-s2-stale"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикСвежесть", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := widgetBody(t, botDemo, "424210", time.Now().Add(-301*time.Second).Unix(), token, false, hashGood)
	rec := h.callAs(http.MethodPost, "/auth/telegram", ip, stale, ptr(appOrigin), nil)
	expectAPI(t, rec, http.StatusUnauthorized, "login_stale")
	if hasSessionCookie(rec) {
		t.Fatal("устаревший вход выдал сеанс")
	}
	_, used, ok := h.findInvite(token)
	if !ok || used != nil {
		t.Fatal("устаревший вход погасил приглашение")
	}
	_, in := h.loginTelegram(ip, botDemo, "424210", 299*time.Second, token, false, "")
	if in.Me.UserID != id {
		t.Fatalf("вход 299 с = %+v", in.Me)
	}
}

func TestAU_R14_LoginFieldsStayOutOfURL(t *testing.T) {
	h, ip := openAuth(t)
	body, sum := widgetBody(t, botDemo, "424214", time.Now().Add(-10*time.Second).Unix(), "", true, hashBad)
	q := url.Values{}
	q.Set("id", queryTelegramID)
	q.Set("marker", queryMarker)
	q.Set("hash", sum)
	rec := h.callAs(http.MethodPost, "/auth/telegram?"+q.Encode(), ip, body, ptr(appOrigin), nil)
	expectAPI(t, rec, http.StatusUnauthorized, "login_invalid")
	if h.count(`SELECT count(*) FROM core.user_pii WHERE telegram_id = $1`, queryTelegramID)+
		h.count(`SELECT count(*) FROM core.student_pii WHERE telegram_id = $1`, queryTelegramID) != 0 {
		t.Fatal("идентификатор из адреса записан как telegram_id")
	}
	h.logsHide(queryMarker, queryTelegramID, sum, profileFirst, profileLast, profileUser, profilePhoto, botDemo)
}

func TestAU_R15_auth11_StoresOnlyTelegramID(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-r15", "tutor")
	const id = "student-s2-r15"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикТолькоИд", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	rec, in := h.loginTelegram(ip, botDemo, "424215", 10*time.Second, token, true, "")
	if in.Me.UserID != id || in.Me.Role != "student" {
		t.Fatalf("Me = %+v", in.Me)
	}
	if strings.Contains(rec.Body.String(), profileFirst) || strings.Contains(rec.Body.String(), profileUser) || strings.Contains(rec.Body.String(), profilePhoto) {
		t.Fatal("ответ входа содержит профиль Telegram")
	}
	name, tg, ok := h.studentPII(id)
	if !ok || name != "НикТолькоИд" || tg != "424215" {
		t.Fatalf("ПДн = %s %q", name, tg)
	}
	h.assertTextAbsent(profileFirst, profileLast, profileUser, profilePhoto)
	h.logsHide(profileFirst, profileLast, profileUser, profilePhoto)
}

func TestAU_R16_auth12_TelegramAlreadyBound(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-bound", "tutor")
	h.seedTutor("tutor-s2-holds", "424217")
	const a = "student-s2-bound-a"
	const b = "student-s2-bound-b"
	const c = "student-s2-bound-c"
	h.seedOwnedStudent(tutor.Me.UserID, a, "НикА", "active")
	h.seedOwnedStudent(tutor.Me.UserID, b, "НикБ", "active")
	h.seedOwnedStudent(tutor.Me.UserID, c, "НикВ", "active")
	tokenA, err := h.createInvite(ip, a, tutor)
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := h.createInvite(ip, b, tutor)
	if err != nil {
		t.Fatal(err)
	}
	tokenC, err := h.createInvite(ip, c, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, first := h.loginTelegram(ip, botDemo, "424216", 10*time.Second, tokenA, false, "")
	if first.Me.UserID != a {
		t.Fatalf("первый ученик %+v", first.Me)
	}
	for _, item := range []struct {
		id    string
		tg    string
		token string
	}{
		{id: b, tg: "424216", token: tokenB},
		{id: c, tg: "424217", token: tokenC},
	} {
		body, _ := widgetBody(t, botDemo, item.tg, time.Now().Add(-10*time.Second).Unix(), item.token, false, hashGood)
		rec := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), nil)
		expectAPI(t, rec, http.StatusConflict, "telegram_already_bound")
		if hasSessionCookie(rec) {
			t.Fatal("занятый telegram_id выдал сеанс")
		}
		_, tg, ok := h.studentPII(item.id)
		if !ok || tg != "" {
			t.Fatalf("%s telegram_id = %q", item.id, tg)
		}
		if len(h.sessionsFor(item.id)) != 0 {
			t.Fatalf("%s получил сеанс", item.id)
		}
	}
	_, tg, ok := h.studentPII(a)
	if !ok || tg != "424216" {
		t.Fatalf("привязка первого ученика %q", tg)
	}
}

func TestAU_R17_auth13_FirstLoginByInvite(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-first", "tutor")
	const id = "student-s2-first"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикПервый", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, in := h.loginTelegram(ip, botDemo, "4242171", 10*time.Second, token, true, "")
	if in.Me.UserID != id || in.Me.Role != "student" {
		t.Fatalf("Me = %+v", in.Me)
	}
	assertTTL(t, h, in.Token, studentTTL)
	_, used, ok := h.findInvite(token)
	if !ok || used == nil {
		t.Fatal("приглашение не погашено")
	}
	name, tg, piiOK := h.studentPII(id)
	if !piiOK || name != "НикПервый" || tg != "4242171" {
		t.Fatalf("привязка %s %q", name, tg)
	}
	h.expectMe(in)
	if h.countEvent("account_bound", "") < 1 {
		t.Fatal("нет события account_bound")
	}
	if h.countOutbox("account_bound", tutor.Me.UserID) < 1 {
		t.Fatal("репетитору не ушло сообщение account_bound")
	}
	h.logsHide(token, profileFirst, profileUser, in.Token)
}

func TestAU_R17_SignatureThenInviteThenConsent(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-order", "tutor")
	const staleID = "student-s2-order-stale"
	const expiredID = "student-s2-order-expired"
	const bareID = "student-s2-order-bare"
	h.seedOwnedStudent(tutor.Me.UserID, staleID, "НикПорядок", "active")
	h.seedOwnedStudent(tutor.Me.UserID, expiredID, "НикПорядок2", "active")
	h.seedOwnedStudent(tutor.Me.UserID, bareID, "НикПорядок3", "active")
	staleToken, err := h.createInvite(ip, staleID, tutor)
	if err != nil {
		t.Fatal(err)
	}
	expiredToken, err := h.createInvite(ip, expiredID, tutor)
	if err != nil {
		t.Fatal(err)
	}
	bareToken, err := h.createInvite(ip, bareID, tutor)
	if err != nil {
		t.Fatal(err)
	}
	h.exec(`UPDATE core.consent SET state = 'revoked', revoked_at = now() WHERE student_id = $1`, bareID)
	h.exec(`UPDATE core.telegram_invite SET expires_at = now() - interval '1 hour' WHERE student_id = $1`, expiredID)

	badHash, _ := widgetBody(t, botDemo, "424218", time.Now().Add(-10*time.Second).Unix(), expiredToken, false, hashBad)
	bad := h.callAs(http.MethodPost, "/auth/telegram", ip, badHash, ptr(appOrigin), nil)
	expectAPI(t, bad, http.StatusUnauthorized, "login_invalid")

	stale, _ := widgetBody(t, botDemo, "424218", time.Now().Add(-301*time.Second).Unix(), staleToken, false, hashGood)
	staleRec := h.callAs(http.MethodPost, "/auth/telegram", ip, stale, ptr(appOrigin), nil)
	expectAPI(t, staleRec, http.StatusUnauthorized, "login_stale")
	if _, used, ok := h.findInvite(staleToken); !ok || used != nil {
		t.Fatal("проверка срока погасила приглашение")
	}

	expired, _ := widgetBody(t, botDemo, "424219", time.Now().Add(-10*time.Second).Unix(), expiredToken, false, hashGood)
	expiredRec := h.callAs(http.MethodPost, "/auth/telegram", ip, expired, ptr(appOrigin), nil)
	expectAPI(t, expiredRec, http.StatusGone, "invite_gone")
	if len(h.sessionsFor(expiredID)) != 0 {
		t.Fatal("просроченное приглашение создало сеанс")
	}

	bare, _ := widgetBody(t, botDemo, "424220", time.Now().Add(-10*time.Second).Unix(), bareToken, false, hashGood)
	bareRec := h.callAs(http.MethodPost, "/auth/telegram", ip, bare, ptr(appOrigin), nil)
	expectAPI(t, bareRec, http.StatusLocked, "consent_missing")
	if hasSessionCookie(bareRec) || len(h.sessionsFor(bareID)) != 0 {
		t.Fatal("вход без согласия создал сеанс")
	}
}

func TestAU_R18_RepeatLoginFindsSameUser(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-repeat", "tutor")
	const id = "student-s2-repeat"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикСнова", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, first := h.loginTelegram(ip, botDemo, "424221", 10*time.Second, token, false, "")
	_, second := h.loginTelegram(ip, botDemo, "424221", 10*time.Second, "", false, "")
	if first.Me != second.Me || second.Me.Role != "student" {
		t.Fatalf("повторный вход %+v и %+v", first.Me, second.Me)
	}
	if first.Token == second.Token {
		t.Fatal("повторный вход переиспользовал сеанс")
	}
	h.expectMe(first)
	h.expectMe(second)
}

func TestAU_R18_auth14_RevokedConsentAndInactive(t *testing.T) {
	h, ip := openAuth(t)
	h.seedTutor("tutor-s2-lock", "tg-s2-tutor-lock")
	h.seedOwnedStudent("tutor-s2-lock", "student-s2-revoked-login", "НикЗакрыт", "revoked")
	h.bindTelegram("student-s2-revoked-login", "424222")
	h.seedOwnedStudent("tutor-s2-lock", "student-s2-archived", "НикАрхив", "active")
	h.bindTelegram("student-s2-archived", "424223")
	h.exec(`UPDATE core.app_user SET status = 'archived' WHERE id = 'student-s2-archived'`)

	revokedBody, _ := widgetBody(t, botDemo, "424222", time.Now().Add(-10*time.Second).Unix(), "", false, hashGood)
	revoked := h.callAs(http.MethodPost, "/auth/telegram", ip, revokedBody, ptr(appOrigin), nil)
	expectAPI(t, revoked, http.StatusLocked, "consent_missing")
	if hasSessionCookie(revoked) {
		t.Fatal("отозванное согласие выдало сеанс")
	}

	archivedBody, _ := widgetBody(t, botDemo, "424223", time.Now().Add(-10*time.Second).Unix(), "", false, hashGood)
	archived := h.callAs(http.MethodPost, "/auth/telegram", ip, archivedBody, ptr(appOrigin), nil)
	expectAPI(t, archived, http.StatusLocked, "account_locked")
	if hasSessionCookie(archived) {
		t.Fatal("закрытая учётная запись выдала сеанс")
	}
}

func TestAU_R20_auth15_EleventhRequestPerIP(t *testing.T) {
	h, ip := openAuth(t)
	token := strings.Repeat("ab", 32)
	path := "/invite/" + url.PathEscape(token)
	for i := 1; i <= 10; i++ {
		rec := h.callAs(http.MethodGet, path, ip, nil, nil, nil)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("запрос %d уже 429", i)
		}
	}
	limited := h.callAs(http.MethodGet, path, ip, nil, nil, nil)
	expectAPI(t, limited, http.StatusTooManyRequests, "rate_limited")
	retryAfter, err := strconv.Atoi(limited.Header().Get("Retry-After"))
	if err != nil || retryAfter < 1 {
		t.Fatalf("Retry-After = %q", limited.Header().Get("Retry-After"))
	}
	other := "203.0.113.50"
	if other == ip {
		other = "203.0.113.51"
	}
	again := h.callAs(http.MethodGet, path, other, nil, nil, nil)
	if again.Code == http.StatusTooManyRequests {
		t.Fatal("лимит одного IP закрыл другой IP")
	}
}

func TestAU_R21_TelegramLoginStoresSessionHash(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-hash", "tutor")
	const id = "student-s2-hash"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикХеш", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, first := h.loginTelegram(ip, botDemo, "424224", 10*time.Second, token, false, "")
	_, second := h.loginTelegram(ip, botDemo, "424224", 10*time.Second, "", false, "")
	if first.Token == second.Token {
		t.Fatal("второй вход не создал новый сеанс")
	}
	for _, item := range []loggedIn{first, second} {
		row := h.sessionByToken(item.Token)
		if row.UserID != id {
			t.Fatalf("сеанс ученика %s", row.UserID)
		}
		raw := mustDecode32(t, item.Token)
		h.assertNoRawToken(item.Token, raw)
	}
}

func TestAU_R22_TelegramLoginHostCookie(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-cookie", "tutor")
	const id = "student-s2-cookie"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикКуки", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := h.loginTelegram(ip, botDemo, "424225", 10*time.Second, token, false, "")
	_, raw := sessionToken(t, rec)
	cookie, err := parseSetCookie(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Name != cookieName {
		t.Fatalf("имя cookie %s", cookie.Name)
	}
	if _, bad := cookie.flags["domain"]; bad {
		t.Fatal("у cookie сеанса есть Domain")
	}
	if cookie.flags["path"] != "/" {
		t.Fatalf("Path = %q", cookie.flags["path"])
	}
	if _, ok := cookie.flags["secure"]; !ok {
		t.Fatal("нет Secure")
	}
	if _, ok := cookie.flags["httponly"]; !ok {
		t.Fatal("нет HttpOnly")
	}
	if same := cookie.flags["samesite"]; !strings.EqualFold(same, "Lax") {
		t.Fatalf("SameSite = %q", same)
	}
}

func TestAU_R23_auth16_StudentSessionDay91(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-day91", "tutor")
	const id = "student-s2-day91"
	h.seedOwnedStudent(tutor.Me.UserID, id, "Ник91", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, in := h.loginTelegram(ip, botDemo, "424226", 10*time.Second, token, false, "")
	assertTTL(t, h, in.Token, studentTTL)
	before := h.sessionByToken(in.Token).Expires
	h.expectMe(in)
	if !h.sessionByToken(in.Token).Expires.Equal(before) {
		t.Fatal("GET /me продлил сеанс")
	}
	listed := h.callAs(http.MethodGet, "/me/sessions", ip, nil, nil, nil, sessionCookie(in.Token))
	if listed.Code != http.StatusOK {
		t.Fatalf("список сеансов = %d %s", listed.Code, listed.Body.Bytes())
	}
	items := decodeSessions(t, listed.Body.Bytes())
	if !oneCurrentActive(items) {
		t.Fatalf("сеансы %+v", items)
	}
	h.shiftSession(in.Token, 91)
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(in.Token))
}

func TestAU_R23_auth17_TutorSessionDay8(t *testing.T) {
	h, ip := openAuth(t)
	h.seedTutor("tutor-s2-day8", "424227")
	_, in := h.loginTutor(ip, botDemo, "424227", "")
	assertTTL(t, h, in.Token, tutorTTL)
	h.shiftSession(in.Token, 8)
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(in.Token))
}

func TestAU_R24_auth18_MutationsRequireCSRF(t *testing.T) {
	h, ip, tutor, student := devPair(t, "csrf", "НикЦСРФ", "bind-s2-csrf")
	if _, err := h.createInvite(ip, student.Me.UserID, tutor); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"/students/" + student.Me.UserID + "/invites",
		"/me/sessions/revoke-all",
		"/students/" + student.Me.UserID + "/revoke-sessions",
		"/students/" + student.Me.UserID + "/unbind-telegram",
	}
	for _, path := range paths {
		rec := h.callAs(http.MethodPost, path, ip, nil, ptr(appOrigin), nil, sessionCookie(tutor.Token))
		expectAPI(t, rec, http.StatusForbidden, "csrf")
	}
	if h.inviteCount(student.Me.UserID) != 1 {
		t.Fatal("запрос без CSRF изменил приглашения")
	}
	if _, tg, ok := h.studentPII(student.Me.UserID); !ok || tg != "bind-s2-csrf" {
		t.Fatal("запрос без CSRF снял привязку")
	}
	if h.sessionByToken(student.Token).Revoked != nil || h.sessionByToken(tutor.Token).Revoked != nil {
		t.Fatal("запрос без CSRF отозвал сеанс")
	}
	h.expectMe(student)
	h.expectMe(tutor)
}

func TestAU_R24_auth19_ForeignOriginRejected(t *testing.T) {
	h, ip, tutor, student := devPair(t, "origin", "НикОриджин", "bind-s2-origin")
	if _, err := h.createInvite(ip, student.Me.UserID, tutor); err != nil {
		t.Fatal(err)
	}
	foreign := "http://evil.example"
	paths := []string{
		"/students/" + student.Me.UserID + "/invites",
		"/me/sessions/revoke-all",
		"/students/" + student.Me.UserID + "/revoke-sessions",
		"/students/" + student.Me.UserID + "/unbind-telegram",
	}
	for _, path := range paths {
		rec := h.callAs(http.MethodPost, path, ip, nil, &foreign, &tutor.Token, sessionCookie(tutor.Token))
		expectAPI(t, rec, http.StatusForbidden, "csrf")
	}
	if h.inviteCount(student.Me.UserID) != 1 {
		t.Fatal("чужой Origin изменил приглашения")
	}
	if h.sessionByToken(student.Token).Revoked != nil {
		t.Fatal("чужой Origin отозвал сеанс ученика")
	}
	h.expectMe(student)
}

func TestAU_R24_TelegramPreSessionChecksOriginOnly(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-pre", "tutor")
	const id = "student-s2-pre"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикДоСеанса", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	authDate := time.Now().Add(-10 * time.Second).Unix()
	wrong := "csrf-demo-wrong"
	cases := []struct {
		name   string
		origin *string
	}{
		{name: "missing", origin: nil},
		{name: "empty", origin: ptr("")},
		{name: "foreign", origin: ptr("http://evil.example")},
		{name: "https", origin: ptr("https://localhost")},
	}
	for _, tc := range cases {
		body, _ := widgetBody(t, botDemo, "424228", authDate, token, false, hashGood)
		rec := h.callAs(http.MethodPost, "/auth/telegram", ip, body, tc.origin, &wrong)
		expectAPI(t, rec, http.StatusForbidden, "csrf")
		if hasSessionCookie(rec) {
			t.Fatalf("%s: чужой Origin выдал сеанс", tc.name)
		}
	}
	if _, used, ok := h.findInvite(token); !ok || used != nil {
		t.Fatal("чужой Origin погасил приглашение")
	}
	body, _ := widgetBody(t, botDemo, "424228", time.Now().Add(-10*time.Second).Unix(), token, false, hashGood)
	rec := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), &wrong)
	if rec.Code != http.StatusOK {
		t.Fatalf("верный Origin без CSRF = %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestAU_R25_RevokeAllKeepsCurrentSession(t *testing.T) {
	h, ip := openAuth(t)
	h.expectUnauthorized(http.MethodGet, "/me/sessions", nil)
	first := h.loginDev(ip, "tg-s2-revoke-all", "student")
	second := h.loginDev(ip, "tg-s2-revoke-all", "student")
	if first.Token == second.Token || first.Me != second.Me {
		t.Fatalf("два входа %+v %s %s", first.Me, first.Token, second.Token)
	}
	listed := h.callAs(http.MethodGet, "/me/sessions", ip, nil, nil, nil, sessionCookie(second.Token))
	if listed.Code != http.StatusOK {
		t.Fatalf("список = %d %s", listed.Code, listed.Body.Bytes())
	}
	if !oneCurrentActive(decodeSessions(t, listed.Body.Bytes())) {
		t.Fatalf("сеансы %s", listed.Body.Bytes())
	}
	rec := h.callAs(http.MethodPost, "/me/sessions/revoke-all", ip, nil, ptr(appOrigin), &second.Token, sessionCookie(second.Token))
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("revoke-all = %d %s", rec.Code, rec.Body.Bytes())
	}
	if h.sessionByToken(first.Token).Revoked == nil {
		t.Fatal("чужой сеанс этого пользователя не отозван")
	}
	if h.sessionByToken(second.Token).Revoked != nil {
		t.Fatal("текущий сеанс отозван")
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(first.Token))
	h.expectMe(second)
}

func TestAU_R25_UnbindAndTutorRevokeSessions(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-unbind", "tutor")
	unbound := h.loginDev(ip, "tg-s2-student-unbind", "student")
	revoked := h.loginDev(ip, "tg-s2-student-revoke", "student")
	h.attachStudent(tutor.Me.UserID, unbound.Me.UserID, "НикОтвязка", "bind-s2-unbind")
	h.attachStudent(tutor.Me.UserID, revoked.Me.UserID, "НикОтзыв", "bind-s2-revoke")
	h.grantConsent(unbound.Me.UserID, "active")
	h.grantConsent(revoked.Me.UserID, "active")

	rec := h.postTutor(ip, "/students/"+revoked.Me.UserID+"/revoke-sessions", tutor)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("revoke-sessions = %d %s", rec.Code, rec.Body.Bytes())
	}
	if h.sessionByToken(revoked.Token).Revoked == nil {
		t.Fatal("репетитор не отозвал сеансы ученика")
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(revoked.Token))
	if _, tg, ok := h.studentPII(revoked.Me.UserID); !ok || tg != "bind-s2-revoke" {
		t.Fatal("отзыв сеансов снял привязку Telegram")
	}
	if h.countEvent("session_revoked", "tutor_revoked_all") < 1 {
		t.Fatal("нет события session_revoked tutor_revoked_all")
	}

	unbind := h.postTutor(ip, "/students/"+unbound.Me.UserID+"/unbind-telegram", tutor)
	if unbind.Code != http.StatusNoContent || unbind.Body.Len() != 0 {
		t.Fatalf("unbind = %d %s", unbind.Code, unbind.Body.Bytes())
	}
	name, tg, ok := h.studentPII(unbound.Me.UserID)
	if !ok || name != "НикОтвязка" || tg != "" {
		t.Fatalf("ПДн после отвязки %s %q", name, tg)
	}
	if userTG, found := h.userTelegram(unbound.Me.UserID); found && userTG != "" {
		t.Fatalf("user_pii после отвязки %s", userTG)
	}
	if h.sessionByToken(unbound.Token).Revoked == nil {
		t.Fatal("отвязка не отозвала сеансы")
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(unbound.Token))
	if h.countEvent("session_revoked", "unbind") < 1 {
		t.Fatal("нет события session_revoked unbind")
	}
	h.expectMe(tutor)
}

func TestAU_R26_auth20_SixthLoginRevokesOldest(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-six", "tutor")
	bystander := h.loginDev(ip, "tg-s2-bystander", "student")
	const id = "student-s2-six"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикШесть", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	var sessions []loggedIn
	for i := 0; i < 6; i++ {
		invite := ""
		if i == 0 {
			invite = token
		}
		_, in := h.loginTelegram(ip, botDemo, "4242266", 10*time.Second, invite, false, "")
		if in.Me.UserID != id {
			t.Fatalf("вход %d: %+v", i+1, in.Me)
		}
		h.setCreatedAgo(in.Token, time.Duration(6-i)*time.Minute)
		sessions = append(sessions, in)
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(sessions[0].Token))
	if h.sessionByToken(sessions[0].Token).Revoked == nil {
		t.Fatal("самый старый сеанс не отозван")
	}
	for _, in := range sessions[1:] {
		h.expectMe(in)
		if h.sessionByToken(in.Token).Revoked != nil {
			t.Fatal("отозван не самый старый сеанс")
		}
	}
	h.expectMe(bystander)
}

func TestAU_R27_StudentLoginWithoutDeviceConfirm(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-device-free", "tutor")
	const id = "student-s2-device-free"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикБезУстройства", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	_, first := h.loginTelegram(ip, botDemo, "4242277", 10*time.Second, token, false, "")
	_, second := h.loginTelegram(ip, botDemo, "4242277", 10*time.Second, "", false, "")
	if first.Me != second.Me {
		t.Fatalf("второй вход с другого устройства: %+v и %+v", first.Me, second.Me)
	}
	h.expectMe(first)
	h.expectMe(second)
}

func TestAU_R28_DeviceCookieOnFirstTutorLogin(t *testing.T) {
	h, ip := openAuth(t)
	h.seedTutor("tutor-s2-device", "4242288")
	rec, in := h.loginTutor(ip, botDemo, "4242288", "")
	if in.Me.UserID != "tutor-s2-device" || in.Me.Role != "tutor" {
		t.Fatalf("Me = %+v", in.Me)
	}
	cookie, _ := cookieLine(t, rec, deviceCookie)
	raw, err := decode16(cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, bad := cookie.flags["domain"]; bad {
		t.Fatal("у cookie устройства есть Domain")
	}
	if cookie.flags["path"] != "/" {
		t.Fatalf("Path = %q", cookie.flags["path"])
	}
	if _, ok := cookie.flags["secure"]; !ok {
		t.Fatal("нет Secure")
	}
	assertAbout400Days(t, cookie.flags)
	if !h.hashStored(raw, cookie.Value) {
		t.Fatal("хеш устройства не найден в базе")
	}
	h.assertSecretHidden(cookie.Value, raw)
	h.logsHide(cookie.Value)
}

func TestAU_R29_auth21_UnknownDeviceIsPending(t *testing.T) {
	h, ip := openAuth(t)
	h.seedTutor("tutor-s2-pending", "515151")
	_, known := h.loginTutor(ip, botDemo, "515151", "")
	h.expectMe(known)
	body, _ := widgetBody(t, botDemo, "515151", time.Now().Add(-10*time.Second).Unix(), "", false, hashGood)
	pending := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), nil)
	expectAPI(t, pending, http.StatusUnauthorized, "device_pending")
	token, _ := sessionToken(t, pending)
	if token == known.Token {
		t.Fatal("pending-сеанс совпал с подтверждённым")
	}
	if h.sessionByToken(token).Revoked != nil {
		t.Fatal("сеанс отклонён раньше 10 минут")
	}
	early := h.callAs(http.MethodGet, pathMe, ip, nil, nil, nil, sessionCookie(token))
	if early.Code != http.StatusUnauthorized {
		t.Fatalf("у pending-сеанса есть права: %d %s", early.Code, early.Body.Bytes())
	}
	h.setCreatedAgo(token, 11*time.Minute)
	late := h.callAs(http.MethodGet, pathMe, ip, nil, nil, nil, sessionCookie(token))
	if late.Code != http.StatusUnauthorized {
		t.Fatalf("сеанс старше 10 минут ещё пускает: %d %s", late.Code, late.Body.Bytes())
	}
	if h.sessionByToken(token).Revoked == nil {
		t.Fatal("сеанс без подтверждения за 10 минут не отклонён")
	}
	h.expectMe(known)
}

func TestAU_R31_auth23_FirstTutorLoginConfirmsDevice(t *testing.T) {
	h, ip := openAuth(t)
	h.seedTutor("tutor-s2-first-device", "424231")
	rec, first := h.loginTutor(ip, botDemo, "424231", "")
	if first.Me.UserID != "tutor-s2-first-device" {
		t.Fatalf("Me = %+v", first.Me)
	}
	device, _ := cookieLine(t, rec, deviceCookie)
	_, second := h.loginTutor(ip, botDemo, "424231", device.Value)
	if first.Me != second.Me || first.Token == second.Token {
		t.Fatalf("повтор с тем же устройством %+v", second.Me)
	}
	h.expectMe(first)
	h.expectMe(second)
	if h.countEvent("device_confirmed", "first_login") < 1 {
		t.Fatal("нет события device_confirmed first_login")
	}
}

func TestAU_R35_TokenRotationInvalidatesLoginNotSession(t *testing.T) {
	const tokenA = "bot-demo-token-a"
	const tokenB = "bot-demo-token-b"
	useBotToken(t, tokenA)
	h := newHarness(t, "local", true)
	ip := testIP(t)
	h.seedTutor("tutor-s2-rotate", "616161")
	rec, in := h.loginTutor(ip, tokenA, "616161", "")
	device, _ := cookieLine(t, rec, deviceCookie)
	if err := os.Setenv("TELEGRAM_BOT_TOKEN", tokenB); err != nil {
		t.Fatal(err)
	}
	h2 := openHarness(t, h.dsn, "local", true)
	h2.expectMe(in)
	oldBody, _ := widgetBody(t, tokenA, "616161", time.Now().Add(-10*time.Second).Unix(), "", false, hashGood)
	old := h2.callAs(http.MethodPost, "/auth/telegram", ip, oldBody, ptr(appOrigin), nil, &http.Cookie{Name: deviceCookie, Value: device.Value})
	expectAPI(t, old, http.StatusUnauthorized, "login_invalid")
	h2.expectMe(in)
	freshBody, _ := widgetBody(t, tokenB, "616161", time.Now().Add(-10*time.Second).Unix(), "", false, hashGood)
	fresh := h2.callAs(http.MethodPost, "/auth/telegram", ip, freshBody, ptr(appOrigin), nil, &http.Cookie{Name: deviceCookie, Value: device.Value})
	if fresh.Code != http.StatusOK {
		t.Fatalf("вход новым токеном = %d %s", fresh.Code, fresh.Body.Bytes())
	}
	h2.expectMe(in)
}

func TestAU_R37_TelegramSecretsStayOutOfLogs(t *testing.T) {
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-logs", "tutor")
	const id = "student-s2-logs"
	h.seedOwnedStudent(tutor.Me.UserID, id, "НикЖурнал", "active")
	token, err := h.createInvite(ip, id, tutor)
	if err != nil {
		t.Fatal(err)
	}
	body, sum := widgetBody(t, botDemo, "424237", time.Now().Add(-10*time.Second).Unix(), token, true, hashGood)
	rec := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("вход = %d %s", rec.Code, rec.Body.Bytes())
	}
	session, _ := sessionToken(t, rec)
	h.logsHide(token, sum, session, botDemo, profileFirst, profileLast, profileUser, profilePhoto)
}

func TestAuthExamples_OperationRowsBound(t *testing.T) {
	raw := readRepo(t, "docs", "platform", "auth.examples.yaml")
	var rows []struct {
		ID   string `yaml:"id"`
		Rule string `yaml:"rule"`
	}
	if err := yaml.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	required := map[string]string{
		"auth-01": "AU-R4", "auth-02": "AU-R4", "auth-04": "AU-R8",
		"auth-05": "AU-R10", "auth-06": "AU-R10", "auth-07": "AU-R10",
		"auth-08": "AU-R11", "auth-09": "AU-R12", "auth-10": "AU-R13",
		"auth-11": "AU-R15", "auth-12": "AU-R16", "auth-13": "AU-R17",
		"auth-14": "AU-R18", "auth-15": "AU-R20", "auth-16": "AU-R23",
		"auth-17": "AU-R23", "auth-18": "AU-R24", "auth-19": "AU-R24",
		"auth-20": "AU-R26", "auth-21": "AU-R29", "auth-23": "AU-R31",
	}
	skip := map[string]bool{"auth-03": true, "auth-22": true, "auth-24": true}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.ID] = true
		want, isRequired := required[row.ID]
		if isRequired && row.Rule != want {
			t.Fatalf("%s: правило %s, нужно %s", row.ID, row.Rule, want)
		}
		if !isRequired && !skip[row.ID] {
			t.Fatalf("строка %s не привязана и не отложена", row.ID)
		}
	}
	src := currentSource(t)
	for id := range required {
		if !seen[id] {
			t.Fatalf("в auth.examples.yaml нет %s", id)
		}
		if !bytes.Contains(src, []byte(id)) {
			t.Fatalf("строка %s не упомянута в тесте", id)
		}
	}
}

func openAuth(t *testing.T) (*harness, string) {
	t.Helper()
	useBotToken(t, botDemo)
	return newHarness(t, "local", true), testIP(t)
}

func useBotToken(t *testing.T, token string) {
	t.Helper()
	prev, ok := os.LookupEnv("TELEGRAM_BOT_TOKEN")
	if err := os.Setenv("TELEGRAM_BOT_TOKEN", token); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv("TELEGRAM_BOT_TOKEN", prev)
			return
		}
		_ = os.Unsetenv("TELEGRAM_BOT_TOKEN")
	})
}

func testIP(t *testing.T) string {
	t.Helper()
	sum := sha256.Sum256([]byte(t.Name()))
	return fmt.Sprintf("198.%d.%d.%d", 18+int(sum[0]%2), int(sum[1]), int(sum[2]%254)+1)
}

func devPair(t *testing.T, slug, nick, telegramID string) (*harness, string, loggedIn, loggedIn) {
	t.Helper()
	h, ip := openAuth(t)
	tutor := h.loginDev(ip, "tg-s2-tutor-"+slug, "tutor")
	student := h.loginDev(ip, "tg-s2-student-"+slug, "student")
	h.attachStudent(tutor.Me.UserID, student.Me.UserID, nick, telegramID)
	h.grantConsent(student.Me.UserID, "active")
	return h, ip, tutor, student
}

func (h *harness) callAs(method, path, ip string, body []byte, origin, csrf *string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.RemoteAddr = ip + ":4242"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != nil {
		req.Header.Set("Origin", *origin)
	}
	if csrf != nil {
		req.Header.Set("X-CSRF-Token", *csrf)
	}
	for _, cookie := range cookies {
		if cookie != nil {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *harness) loginDev(ip, telegramID, role string) loggedIn {
	h.t.Helper()
	rec := h.callAs(http.MethodPost, pathDev, ip, devJSON(h.t, telegramID, role), ptr(appOrigin), nil)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("POST /auth/dev %s %s = %d %s", telegramID, role, rec.Code, rec.Body.Bytes())
	}
	token, _ := sessionToken(h.t, rec)
	me := decodeMe(h.t, rec.Body.Bytes())
	if me.UserID == "" || me.Role != role {
		h.t.Fatalf("Me = %+v, роль %s", me, role)
	}
	return loggedIn{Me: me, Token: token}
}

func (h *harness) postTutor(ip, path string, tutor loggedIn) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.callAs(http.MethodPost, path, ip, nil, ptr(appOrigin), &tutor.Token, sessionCookie(tutor.Token))
}

func (h *harness) createInvite(ip, studentID string, tutor loggedIn) (string, error) {
	h.t.Helper()
	rec := h.postTutor(ip, "/students/"+studentID+"/invites", tutor)
	if rec.Code != http.StatusCreated {
		return "", fmt.Errorf("createInvite %s = %d %s", studentID, rec.Code, rec.Body.Bytes())
	}
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	var link struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := dec.Decode(&link); err != nil {
		return "", fmt.Errorf("Link: %w тело %s", err, rec.Body.Bytes())
	}
	token := tokenFromLink(h.t, link.URL)
	around(h.t, parseStamp(h.t, link.ExpiresAt), time.Now(), inviteLife, expireSlack, "expires_at")
	raw := mustDecode32(h.t, token)
	if !h.hashStored(raw, token) {
		return "", errors.New("в базе нет хеша токена приглашения")
	}
	return token, nil
}

func (h *harness) getInvite(ip, token string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.callAs(http.MethodGet, "/invite/"+url.PathEscape(token), ip, nil, nil, nil)
}

func (h *harness) loginTelegram(ip, bot, tg string, age time.Duration, invite string, profile bool, device string) (*httptest.ResponseRecorder, loggedIn) {
	h.t.Helper()
	body, _ := widgetBody(h.t, bot, tg, time.Now().Add(-age).Unix(), invite, profile, hashGood)
	return h.finishLogin(ip, body, device)
}

func (h *harness) loginTutor(ip, bot, tg, device string) (*httptest.ResponseRecorder, loggedIn) {
	h.t.Helper()
	return h.loginTelegram(ip, bot, tg, 10*time.Second, "", false, device)
}

func (h *harness) finishLogin(ip string, body []byte, device string) (*httptest.ResponseRecorder, loggedIn) {
	h.t.Helper()
	var cookies []*http.Cookie
	if device != "" {
		cookies = append(cookies, &http.Cookie{Name: deviceCookie, Value: device})
	}
	rec := h.callAs(http.MethodPost, "/auth/telegram", ip, body, ptr(appOrigin), nil, cookies...)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("POST /auth/telegram = %d %s", rec.Code, rec.Body.Bytes())
	}
	token, _ := sessionToken(h.t, rec)
	if rec.Header().Get("X-CSRF-Token") != token {
		h.t.Fatal("X-CSRF-Token не равен токену сеанса")
	}
	me := decodeMe(h.t, rec.Body.Bytes())
	if strings.Contains(rec.Body.String(), token) {
		h.t.Fatal("токен сеанса попал в тело")
	}
	return rec, loggedIn{Me: me, Token: token}
}

func (h *harness) seedOwnedStudent(tutorID, studentID, nick, consent string) {
	h.t.Helper()
	h.exec(`
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, studentID, tutorID)
	h.exec(`
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, $2, '')
	`, studentID, nick)
	if consent != "" {
		h.grantConsent(studentID, consent)
	}
}

func (h *harness) attachStudent(tutorID, studentID, nick, telegramID string) {
	h.t.Helper()
	h.exec(`UPDATE core.app_user SET owner_tutor_id = $1, status = 'active' WHERE id = $2`, tutorID, studentID)
	h.exec(`
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (student_id) DO UPDATE
		SET display_name = EXCLUDED.display_name, telegram_id = EXCLUDED.telegram_id
	`, studentID, nick, telegramID)
	h.exec(`UPDATE core.user_pii SET telegram_id = $2 WHERE user_id = $1`, studentID, telegramID)
}

func (h *harness) grantConsent(studentID, state string) {
	h.t.Helper()
	if state == "revoked" {
		h.exec(`
			INSERT INTO core.consent (student_id, version, state, granted_at, revoked_at)
			VALUES ($1, 1, 'revoked', now() - interval '1 day', now())
			ON CONFLICT (student_id) DO UPDATE
			SET state = 'revoked', revoked_at = now()
		`, studentID)
		return
	}
	h.exec(`
		INSERT INTO core.consent (student_id, version, state, granted_at)
		VALUES ($1, 1, 'active', now())
		ON CONFLICT (student_id) DO UPDATE
		SET state = 'active', revoked_at = NULL
	`, studentID)
}

func (h *harness) seedTutor(id, telegramID string) {
	h.t.Helper()
	h.exec(`
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'tutor', NULL, 'active')
	`, id)
	h.exec(`INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)`, id, telegramID)
}

func (h *harness) bindTelegram(studentID, telegramID string) {
	h.t.Helper()
	h.exec(`UPDATE core.student_pii SET telegram_id = $2 WHERE student_id = $1`, studentID, telegramID)
	h.exec(`
		INSERT INTO core.user_pii (user_id, telegram_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET telegram_id = EXCLUDED.telegram_id
	`, studentID, telegramID)
}

func (h *harness) inviteCount(studentID string) int {
	h.t.Helper()
	return h.count(`SELECT count(*) FROM core.telegram_invite WHERE student_id = $1`, studentID)
}

func (h *harness) findInvite(token string) (time.Time, *time.Time, bool) {
	h.t.Helper()
	raw, err := decode32(token)
	if err != nil {
		return time.Time{}, nil, false
	}
	var exp time.Time
	var used *time.Time
	var n int
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `SELECT token_hash, expires_at, used_at FROM core.telegram_invite`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var hash []byte
			var rowExp time.Time
			var rowUsed *time.Time
			if err := rows.Scan(&hash, &rowExp, &rowUsed); err != nil {
				return err
			}
			if hashMatches(hash, token, raw) {
				n++
				exp = rowExp
				used = rowUsed
			}
		}
		return rows.Err()
	})
	if n != 1 {
		return time.Time{}, nil, false
	}
	return exp, used, true
}

func (h *harness) studentPII(id string) (string, string, bool) {
	h.t.Helper()
	var name, tg string
	var found bool
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		err := conn.QueryRow(ctx, `
			SELECT display_name, telegram_id
			FROM core.student_pii WHERE student_id = $1
		`, id).Scan(&name, &tg)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	return name, tg, found
}

func (h *harness) userTelegram(id string) (string, bool) {
	h.t.Helper()
	var tg string
	var found bool
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		err := conn.QueryRow(ctx, `SELECT telegram_id FROM core.user_pii WHERE user_id = $1`, id).Scan(&tg)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	return tg, found
}

func (h *harness) countEvent(kind, piece string) int {
	h.t.Helper()
	return h.count(`
		SELECT count(*) FROM core.journal_event
		WHERE (kind = $1 OR strpos(payload::text, $1) > 0)
		  AND ($2 = '' OR strpos(payload::text, $2) > 0)
	`, kind, piece)
}

func (h *harness) countOutbox(kind, recipient string) int {
	h.t.Helper()
	return h.count(`
		SELECT count(*) FROM core.outbox
		WHERE (kind = $1 OR template = $1
			OR strpos(coalesce(payload::text, ''), $1) > 0
			OR strpos(coalesce(params::text, ''), $1) > 0)
		  AND ($2 = '' OR recipient_id = $2 OR recipient_kind = 'tutor'
			OR strpos(coalesce(payload::text, ''), $2) > 0)
	`, kind, recipient)
}

func (h *harness) logsHide(secrets ...string) {
	h.t.Helper()
	text := h.logs.String()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			h.t.Fatalf("журнал содержит секрет, фрагмент длины %d", len(secret))
		}
	}
}

func (h *harness) assertTextAbsent(secrets ...string) {
	h.t.Helper()
	h.forTextColumns(func(table, name, query string) {
		for _, secret := range secrets {
			if secret == "" {
				continue
			}
			if h.count(query, secret) != 0 {
				h.t.Fatalf("значение записано в core.%s.%s", table, name)
			}
		}
	})
}

func (h *harness) assertSecretHidden(token string, raw []byte) {
	h.t.Helper()
	forms := []string{token, hex.EncodeToString(raw), strings.ToUpper(hex.EncodeToString(raw))}
	h.forTextColumns(func(table, name, query string) {
		for _, secret := range forms {
			if secret != "" && h.count(query, secret) != 0 {
				h.t.Fatalf("сырой секрет записан в core.%s.%s", table, name)
			}
		}
	})
	h.forByteaColumns(func(table, name, query string) {
		if h.count(query, raw) != 0 {
			h.t.Fatalf("сырые байты записаны в core.%s.%s", table, name)
		}
	})
}

func (h *harness) hashStored(raw []byte, text string) bool {
	h.t.Helper()
	sumRaw := sha256.Sum256(raw)
	sumText := sha256.Sum256([]byte(text))
	found := false
	h.forByteaColumns(func(table, name, query string) {
		if h.count(query, sumRaw[:]) > 0 || h.count(query, sumText[:]) > 0 {
			found = true
			_ = table
			_ = name
		}
	})
	return found
}

func (h *harness) forTextColumns(fn func(table, name, query string)) {
	h.t.Helper()
	h.forColumns([]string{"text", "varchar", "bpchar", "json", "jsonb", "name"}, func(table, name, udt string) {
		//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
		fn(table, name, fmt.Sprintf(`SELECT count(*) FROM core.%s WHERE strpos(%s::text, $1) > 0`, table, name))
		_ = udt
	})
}

func (h *harness) forByteaColumns(fn func(table, name, query string)) {
	h.t.Helper()
	h.forColumns([]string{"bytea"}, func(table, name, udt string) {
		//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
		fn(table, name, fmt.Sprintf(`SELECT count(*) FROM core.%s WHERE %s = $1`, table, name))
		_ = udt
	})
}

func (h *harness) forColumns(udts []string, fn func(table, name, udt string)) {
	h.t.Helper()
	type column struct {
		table string
		name  string
		udt   string
	}
	var cols []column
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT table_name, column_name, udt_name
			FROM information_schema.columns
			WHERE table_schema = 'core'
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		want := map[string]bool{}
		for _, udt := range udts {
			want[udt] = true
		}
		for rows.Next() {
			var col column
			if err := rows.Scan(&col.table, &col.name, &col.udt); err != nil {
				return err
			}
			if want[col.udt] {
				cols = append(cols, col)
			}
		}
		return rows.Err()
	})
	ident := regexp.MustCompile(`^[a-z_]+$`)
	for _, col := range cols {
		if !ident.MatchString(col.table) || !ident.MatchString(col.name) {
			continue
		}
		fn(col.table, col.name, col.udt)
	}
}

func widgetBody(t *testing.T, bot, id string, authDate int64, invite string, profile bool, mode hashMode) ([]byte, string) {
	t.Helper()
	fields := map[string]string{
		"id":        id,
		"auth_date": strconv.FormatInt(authDate, 10),
	}
	if invite != "" {
		fields["invite_token"] = invite
	}
	if profile {
		fields["first_name"] = profileFirst
		fields["last_name"] = profileLast
		fields["username"] = profileUser
		fields["photo_url"] = profilePhoto
	}
	sum := widgetHash(fields, bot)
	payload := map[string]any{
		"id":        id,
		"auth_date": authDate,
	}
	if invite != "" {
		payload["invite_token"] = invite
	}
	if profile {
		payload["first_name"] = profileFirst
		payload["last_name"] = profileLast
		payload["username"] = profileUser
		payload["photo_url"] = profilePhoto
	}
	switch mode {
	case hashBad:
		sum = flipHex(sum)
		payload["hash"] = sum
	case hashMissing:
	default:
		payload["hash"] = sum
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw, sum
}

func widgetHash(data map[string]string, token string) string {
	sum := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, sum[:])
	_, _ = mac.Write([]byte(checkLine(data)))
	return hex.EncodeToString(mac.Sum(nil))
}

func checkLine(data map[string]string) string {
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

func tokenFromLink(t *testing.T, rawURL string) string {
	t.Helper()
	if _, err := decode32(rawURL); err == nil {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("ссылка: %v", err)
	}
	var cands []string
	cands = append(cands, strings.Split(strings.Trim(parsed.Path, "/"), "/")...)
	for _, values := range parsed.Query() {
		cands = append(cands, values...)
	}
	var found string
	for _, cand := range cands {
		if _, err := decode32(cand); err != nil {
			continue
		}
		if found != "" && found != cand {
			t.Fatalf("в ссылке два токена")
		}
		found = cand
	}
	if found == "" {
		t.Fatal("в ссылке нет токена приглашения")
	}
	return found
}

func decodeInvitePage(t *testing.T, body []byte) (bool, string, bool) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var raw struct {
		StudentDisplayName *string `json:"student_display_name"`
		Valid              bool    `json:"valid"`
	}
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("страница приглашения: %v тело %s", err, body)
	}
	if raw.StudentDisplayName == nil {
		return raw.Valid, "", false
	}
	return raw.Valid, *raw.StudentDisplayName, true
}

func decodeSessions(t *testing.T, body []byte) []sessionItem {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var items []sessionItem
	if err := dec.Decode(&items); err != nil {
		t.Fatalf("сеансы: %v тело %s", err, body)
	}
	return items
}

func oneCurrentActive(items []sessionItem) bool {
	current := 0
	for _, item := range items {
		if item.SessionID == "" || item.CreatedAt == "" || item.ExpiresAt == "" {
			return false
		}
		if item.Current != nil && *item.Current {
			current++
			if item.State != "active" {
				return false
			}
		}
	}
	return current == 1
}

func cookieLine(t *testing.T, rec *httptest.ResponseRecorder, name string) (wireCookie, string) {
	t.Helper()
	var found wireCookie
	var raw string
	n := 0
	for _, line := range rec.Header().Values("Set-Cookie") {
		cookie, err := parseSetCookie(line)
		if err != nil || cookie.Name != name {
			continue
		}
		n++
		found = cookie
		raw = line
	}
	if n != 1 {
		t.Fatalf("cookie %s встретилась %d раз", name, n)
	}
	return found, raw
}

func assertAbout400Days(t *testing.T, flags map[string]string) {
	t.Helper()
	const want = 400 * 24 * time.Hour
	slack := 2 * time.Hour
	if maxAge, ok := flags["max-age"]; ok {
		sec, err := strconv.Atoi(maxAge)
		if err != nil {
			t.Fatalf("Max-Age %q", maxAge)
		}
		got := time.Duration(sec) * time.Second
		if got < want-slack || got > want+slack {
			t.Fatalf("срок устройства %s", got)
		}
		return
	}
	exp, ok := flags["expires"]
	if !ok {
		t.Fatal("у cookie устройства нет срока")
	}
	var parsed time.Time
	var err error
	for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC850, time.ANSIC} {
		parsed, err = time.Parse(layout, exp)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("Expires %q", exp)
	}
	around(t, parsed, time.Now(), want, slack, "срок устройства")
}

func around(t *testing.T, got, start time.Time, offset, slack time.Duration, what string) {
	t.Helper()
	low := start.Add(offset - slack)
	high := start.Add(offset + slack)
	if got.Before(low) || got.After(high) {
		t.Fatalf("%s = %s, ожидалось около %s", what, got.UTC().Format(time.RFC3339), start.Add(offset).UTC().Format(time.RFC3339))
	}
}

func parseStamp(t *testing.T, value string) time.Time {
	t.Helper()
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts
		}
	}
	t.Fatalf("время %q", value)
	return time.Time{}
}

func mustDecode32(t *testing.T, token string) []byte {
	t.Helper()
	raw, err := decode32(token)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decode32(token string) ([]byte, error) {
	return decodeN(token, 32)
}

func decode16(token string) ([]byte, error) {
	return decodeN(token, 16)
}

func decodeN(token string, n int) ([]byte, error) {
	if raw, err := hex.DecodeString(token); err == nil && len(raw) == n && strings.EqualFold(hex.EncodeToString(raw), token) {
		return raw, nil
	}
	if raw, err := base64.RawURLEncoding.DecodeString(token); err == nil && len(raw) == n {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(token); err == nil && len(raw) == n {
		return raw, nil
	}
	if raw, err := base64.StdEncoding.DecodeString(token); err == nil && len(raw) == n {
		return raw, nil
	}
	return nil, fmt.Errorf("токен не равен %d байтам", n)
}

func hashMatches(hash []byte, token string, raw []byte) bool {
	if len(hash) != sha256.Size || len(raw) == 0 {
		return false
	}
	sumRaw := sha256.Sum256(raw)
	sumText := sha256.Sum256([]byte(token))
	return bytes.Equal(hash, sumRaw[:]) || bytes.Equal(hash, sumText[:])
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

func currentSource(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(moduleDir(t), "internal", "auth", "auth_http_test.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
