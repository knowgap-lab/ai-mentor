package telegram_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

const (
	studentR9A = "student-demo-a"
	studentR9B = "student-demo-b"
	nickR9A    = "demo-nick-a"
	nickR9B    = "demo-nick-b"
	tgR9A      = "tg-free-a"
	tgR9B      = "tg-free-b"

	studentGet = "student-demo-get"
	nickGet    = "demo-nick-get"
	tgGet      = "tg-free-get"

	studentOld = "student-demo-old"
	nickOld    = "demo-nick-old"
	tgOld      = "tg-free-old"

	studentOnce = "student-demo-once"
	nickOnce    = "demo-nick-once"
	tgOnce      = "tg-free-once"
	tgOnceBound = "424210"

	student71 = "student-demo-71"
	nick71    = "demo-nick-71"
	tg71      = "tg-free-71"
	tg71Bound = "424271"

	studentR15 = "student-demo-r15"
	nickR15    = "demo-nick-r15"
	tgR15      = "tg-free-r15"
	tgR15Bound = "424242"

	studentOwn = "student-demo-own"
	studentNew = "student-demo-new"
	nickOwn    = "demo-nick-own"
	nickNew    = "demo-nick-new"
	tgOwn      = "900100"
	tgNew      = "tg-free-new"

	tutorID      = "tutor-demo"
	studentTutor = "student-demo-tutor"
	nickTutor    = "demo-nick-tutor"
	tgTutor      = "900200"
	tgStudentTut = "tg-free-tutor"
)

func TestAU_R9_InviteTokenIs32BytesAndOnlyHashIsStored(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(studentR9A, nickR9A, tgR9A)
	h.seedStudent(studentR9B, nickR9B, tgR9B)
	now := fixedNow()

	first := h.create(studentR9A, now)
	second := h.create(studentR9B, now)
	if first == second {
		t.Fatal("два приглашения получили один токен")
	}
	h.guardToken(first)
	h.guardToken(second)
	if h.count(`SELECT count(*) FROM core.telegram_invite`) != 2 {
		t.Fatal("в базе не две строки приглашений")
	}
	if h.telegramID(studentR9A) != tgR9A || h.telegramID(studentR9B) != tgR9B {
		t.Fatal("выпуск приглашения изменил telegram_id")
	}
}

func TestAU_R10_PreviewLeavesRowsUnchanged(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(studentGet, nickGet, tgGet)
	now := fixedNow()
	token := h.create(studentGet, now)
	before := h.snapshot()

	if err := h.svc.Preview(context.Background(), token, now); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	h.unchanged(token, before)
	h.guardToken(token)

	if err := h.svc.Preview(context.Background(), token, now.Add(time.Minute)); err != nil {
		t.Fatalf("повторный Preview: %v", err)
	}
	h.unchanged(token, before)
	if h.telegramID(studentGet) != tgGet || h.displayName(studentGet) != nickGet {
		t.Fatal("предпросмотр изменил ученика")
	}
	if h.sessions() != 0 {
		t.Fatal("предпросмотр создал сеанс")
	}
}

func TestAU_R10_ExpiredAfter73HoursIsInviteGone(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(studentOld, nickOld, tgOld)
	now := fixedNow()
	token := h.create(studentOld, now.Add(-73*time.Hour))
	before := h.snapshot()
	sessions := h.sessions()

	expectAPI(t, h.svc.Preview(context.Background(), token, now), http.StatusGone, "invite_gone")
	h.unchanged(token, before)

	expectAPI(t, h.svc.Exchange(context.Background(), token, loginData(tgR15Bound), now), http.StatusGone, "invite_gone")
	h.unchanged(token, before)
	h.guardToken(token)
	h.assertLoginSecretsHidden()
	if h.sessions() != sessions {
		t.Fatal("просроченное приглашение создало сеанс")
	}
	if h.telegramID(studentOld) != tgOld || h.displayName(studentOld) != nickOld {
		t.Fatal("просроченное приглашение изменило ученика")
	}
}

func TestAU_R10_SecondExchangeIsInviteGone(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(studentOnce, nickOnce, tgOnce)
	now := fixedNow()
	token := h.create(studentOnce, now)
	h.exchange(studentOnce, nickOnce, token, now, tgOnceBound)

	before := h.snapshot()
	sessions := h.sessions()
	later := now.Add(time.Minute)
	expectAPI(t, h.svc.Exchange(context.Background(), token, loginData(tgOnceBound), later), http.StatusGone, "invite_gone")
	h.unchanged(token, before)
	h.guardToken(token)
	if h.sessions() != sessions {
		t.Fatal("повторный обмен создал сеанс")
	}
	if h.telegramID(studentOnce) != tgOnceBound || h.displayName(studentOnce) != nickOnce {
		t.Fatal("повторный обмен изменил ученика")
	}
}

func TestAU_R10_ExchangeAt71HoursBinds(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(student71, nick71, tg71)
	now := fixedNow()
	token := h.create(student71, now.Add(-71*time.Hour))
	h.exchange(student71, nick71, token, now, tg71Bound)
}

func TestAU_R15_ExchangeStoresOnlyTelegramID(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(studentR15, nickR15, tgR15)
	now := fixedNow()
	token := h.create(studentR15, now)
	data := loginData(tgR15Bound)

	if err := h.svc.Exchange(context.Background(), token, data, now); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	h.bound(studentR15, nickR15, token, tgR15Bound, now)
	if h.count(`SELECT count(*) FROM core.student_pii WHERE telegram_id = $1`, tgR15Bound) != 1 {
		t.Fatal("telegram_id записан не одной строкой ученика")
	}
}

func TestAU_R16_SecondStudentWithSameTelegramIDIsAlreadyBound(t *testing.T) {
	h := newHarness(t)
	h.seedStudent(studentOwn, nickOwn, tgOwn)
	h.seedStudent(studentNew, nickNew, tgNew)
	now := fixedNow()
	token := h.create(studentNew, now)

	expectAPI(t, h.svc.Exchange(context.Background(), token, loginData(tgOwn), now), http.StatusConflict, "telegram_already_bound")
	if h.telegramID(studentOwn) != tgOwn || h.displayName(studentOwn) != nickOwn {
		t.Fatal("отказ переписал уже привязанного ученика")
	}
	if h.telegramID(studentNew) != tgNew || h.displayName(studentNew) != nickNew {
		t.Fatal("отказ привязал занятый telegram_id второму ученику")
	}
	h.guardToken(token)
	h.assertLoginSecretsHidden()
	if h.sessions() != 0 {
		t.Fatal("отказ привязки создал сеанс")
	}
}

func TestAU_R16_TelegramIDBoundToTutorRejectsStudent(t *testing.T) {
	h := newHarness(t)
	h.seedTutor(tutorID, tgTutor)
	h.seedStudent(studentTutor, nickTutor, tgStudentTut)
	now := fixedNow()
	token := h.create(studentTutor, now)

	expectAPI(t, h.svc.Exchange(context.Background(), token, loginData(tgTutor), now), http.StatusConflict, "telegram_already_bound")
	if h.telegramID(studentTutor) != tgStudentTut || h.displayName(studentTutor) != nickTutor {
		t.Fatal("отказ привязал ученику telegram_id репетитора")
	}
	if h.userTelegram(tutorID) != tgTutor {
		t.Fatal("отказ изменил telegram_id репетитора")
	}
	h.guardToken(token)
	h.assertLoginSecretsHidden()
	if h.sessions() != 0 {
		t.Fatal("отказ привязки создал сеанс")
	}
}
