package privacy_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/scenario"
	"gopkg.in/yaml.v3"
)

var s3CodeRE = regexp.MustCompile(`^[0-9A-Z]{8}$`)

func TestPV_R1_S3_SchemaRejectsSchool(t *testing.T) {
	h := s3New(t)
	got := h.columns("student_pii")
	want := []string{"birth_date", "display_name", "student_id", "telegram_id"}
	if !slices.Equal(got, want) {
		t.Fatalf("столбцы student_pii %v", got)
	}
	if err := h.tryQuery(`SELECT school FROM core.student_pii`); err == nil {
		t.Fatal("схема приняла столбец school")
	}
}

func TestPV_R1_S3_CreateStoresPIIOnly(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r1", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	name, _, birth, ok := h.pii(card.ID)
	if !ok || name != s3Nick || !s3SameDay(birth, s3BirthFixed) {
		t.Fatalf("ПДн name=%q birth=%q ok=%v", name, birth, ok)
	}
	role, owner, status := h.user(card.ID)
	if role != "student" || owner != tutor.ID || status != "active" {
		t.Fatalf("пользователь role=%s owner=%s status=%s", role, owner, status)
	}
	for _, column := range h.columns("student_pii") {
		switch column {
		case "subjects", "school", "exam_kind", "exam_date", "surname":
			t.Fatalf("в student_pii есть %s", column)
		}
	}
}

func TestPV_R2_S3_DisplayNameIsNickname(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r2", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	rec := h.get(tutor, s3StudentPath(card.ID, ""))
	s3Expect(t, rec, http.StatusOK, "")
	if s3DecodeCard(t, rec.Body.Bytes()).DisplayName != s3Nick {
		t.Fatal("карточка не отдала прозвище")
	}
	name, _, _, ok := h.pii(card.ID)
	if !ok || name != s3Nick {
		t.Fatalf("в ПДн имя %q", name)
	}
}

func TestPV_R4_S3_StudentReadsOwnName(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r4", "tutor")
	own := h.create(tutor, s3Nick, s3BirthFixed)
	other := h.create(tutor, "demo-s3-other", s3BirthFixed)
	self := s3Actor{ID: own.ID, Token: h.mint(own.ID), Role: "student"}
	rec := h.get(self, s3StudentPath(own.ID, ""))
	s3Expect(t, rec, http.StatusOK, "")
	if s3DecodeCard(t, rec.Body.Bytes()).DisplayName != s3Nick {
		t.Fatal("ученик не видит своё имя")
	}
	s3Expect(t, h.get(self, s3StudentPath(other.ID, "")), http.StatusForbidden, "forbidden")
	seen := s3DecodeCard(t, h.get(tutor, s3StudentPath(own.ID, "")).Body.Bytes())
	if seen.DisplayName != s3Nick {
		t.Fatal("репетитор-владелец не видит имя")
	}
}

func TestPV_R5_S3_SubjectsRemainAfterAnonymise(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r5", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	s3Expect(t, h.call(tutor, http.MethodPost, s3StudentPath(card.ID, "/anonymise"), map[string]any{"confirm": true}), http.StatusNoContent, "")
	if _, _, _, ok := h.pii(card.ID); ok {
		t.Fatal("ПДн остались")
	}
	rec := h.get(tutor, s3StudentPath(card.ID, ""))
	s3Expect(t, rec, http.StatusOK, "")
	got := s3DecodeCard(t, rec.Body.Bytes())
	if got.Status != "anonymised" {
		t.Fatalf("status = %s", got.Status)
	}
	if len(got.Subjects) != 1 || got.Subjects[0] != s3Subject {
		t.Fatalf("subjects после обезличивания %v", got.Subjects)
	}
	if strings.Contains(rec.Body.String(), s3Nick) || strings.Contains(rec.Body.String(), s3BirthFixed) {
		t.Fatal("ответ после обезличивания содержит ПДн")
	}
}

func TestPV_R6_S3_PseudonymOnCreate(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r6", "tutor")
	first := h.create(tutor, s3Nick, s3BirthFixed)
	second := h.create(tutor, "demo-s3-other", s3BirthFixed)
	left, okLeft := h.pseudonym(first.ID)
	right, okRight := h.pseudonym(second.ID)
	if !okLeft || !okRight {
		t.Fatal("нет псевдонима")
	}
	if left == right || !s3CodeRE.MatchString(left) || !s3CodeRE.MatchString(right) {
		t.Fatalf("псевдонимы %q %q", left, right)
	}
	if h.count(`SELECT count(*) FROM core.student_pseudonym WHERE code = $1`, left) != 1 {
		t.Fatal("псевдоним не уникален")
	}
}

func TestPV_R8_S3_ConsentRecordFields(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r8", "tutor")
	s3GrantAndCheck(t, h, tutor, s3Birth(16), "parent")
	s3GrantAndCheck(t, h, tutor, s3Birth(18), "student")
}

func TestPV_R10_S3_LinkIsHashedAndReusable(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r10", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	start := time.Now()
	token := h.link(tutor, card.ID, "parent")
	end := time.Now()
	raw, err := decodeToken(token)
	if err != nil {
		t.Fatal(err)
	}
	s3AssertLinkRow(t, h, card.ID, token, "parent", start, end)
	if h.count(`SELECT count(*) FROM core.consent_link WHERE encode(token_hash, 'hex') = $1`, token) != 0 {
		t.Fatal("в базе сырой токен")
	}
	if h.count(`SELECT count(*) FROM core.consent_link WHERE token_hash = $1`, raw) != 0 {
		t.Fatal("в базе сырые байты токена")
	}
	h.logsHide(token, s3Nick, s3Birth(16))
	s3ExpectPage(t, h, token, "parent", apiNone)
	s3ExpectPage(t, h, token, "parent", apiNone)
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1`, card.ID) != 0 {
		t.Fatal("предпросмотр создал согласие")
	}
	page := h.consent(token, actionGive)
	if page.State != apiGiven {
		t.Fatalf("state = %s", page.State)
	}
	s3ExpectPage(t, h, token, "parent", apiGiven)
}

func TestPV_R10_S3_ExpiredAndUsedLinkGone(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r10b", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	h.consent(token, actionGive)
	h.exec(`
		UPDATE core.consent_link
		SET created_at = now() - interval '15 days',
		    expires_at = now() - interval '1 day'
		WHERE student_id = $1
	`, card.ID)
	rec := h.do(http.MethodGet, "/consent/"+url.PathEscape(token), s3Req{})
	s3Expect(t, rec, http.StatusGone, "consent_link_gone")
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("ошибка содержит токен")
	}
	rec = h.do(http.MethodPost, "/consent/"+url.PathEscape(token), s3Req{body: map[string]string{"action": actionRevoke}})
	s3Expect(t, rec, http.StatusGone, "consent_link_gone")
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1 AND state = 'active'`, card.ID) != 1 {
		t.Fatal("просроченная использованная ссылка изменила согласие")
	}
}

func TestPV_R10_S3_SecondLinkKeepsFirst(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r10c", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	rec := h.call(tutor, http.MethodPost, s3StudentPath(card.ID, "/consent-links"), map[string]string{"kind": "parent"})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusConflict {
		t.Fatalf("повтор ссылки: %d %s", rec.Code, rec.Body.Bytes())
	}
	s3ExpectPage(t, h, token, "parent", apiNone)
}

func TestPV_R11_S3_PreviewDoesNotChangeState(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r11", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	before := s3ConsentSnap(h, card.ID)
	s3ExpectPage(t, h, token, "parent", apiNone)
	if s3ConsentSnap(h, card.ID) != before {
		t.Fatal("предпросмотр изменил строки")
	}
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1`, card.ID) != 0 {
		t.Fatal("предпросмотр создал согласие")
	}
}

func TestPV_R12_S3_KindFollowsAge(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r12", "tutor")
	young := h.create(tutor, "demo-s3-young", s3Birth(16))
	adult := h.create(tutor, "demo-s3-adult", s3Birth(18))
	s3RefuseKind(t, h, tutor, young.ID, "student")
	s3RefuseKind(t, h, tutor, adult.ID, "parent")
	if s3KindOf(t, h, h.link(tutor, young.ID, "parent")) != "parent" {
		t.Fatal("ссылка родителя несовершеннолетнему другого вида")
	}
	if s3KindOf(t, h, h.link(tutor, adult.ID, "student")) != "student" {
		t.Fatal("ссылка совершеннолетнего другого вида")
	}
}

func TestPV_R13_S3_GrantWithoutIdentity(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r13", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	page := h.consent(token, actionGive)
	if page.Kind != "parent" || page.State != apiGiven {
		t.Fatalf("выдача без сеанса: %+v", page)
	}
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1 AND state = 'active'`, card.ID) != 1 {
		t.Fatal("согласие без сеанса не записано")
	}
}

func TestPV_R17_S3_RevokeClosesLogin(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r17", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	h.consent(token, actionGive)
	session := h.mint(card.ID)
	if h.openSessions(card.ID) != 1 {
		t.Fatal("сеанс ученика не создан")
	}
	start := time.Now()
	page := h.consent(token, actionRevoke)
	end := time.Now()
	if page.State != apiRevoked || page.Kind != "parent" {
		t.Fatalf("страница %+v", page)
	}
	expectAnonymise(t, page, start, end, token)
	s3ExpectRevokedRow(t, h, card.ID, start, end)
	if h.openSessions(card.ID) != 0 {
		t.Fatal("сеанс ученика после отзыва жив")
	}
	s3Expect(t, h.do(http.MethodGet, s3StudentPath(card.ID, ""), s3Req{cookie: session}), http.StatusUnauthorized, "unauthorized")
	if h.openSessions(tutor.ID) == 0 {
		t.Fatal("отзыв погасил сеанс репетитора")
	}
	seen := s3DecodeCard(t, h.get(tutor, s3StudentPath(card.ID, "")).Body.Bytes())
	if seen.Consent != "revoked" {
		t.Fatalf("consent карточки = %s", seen.Consent)
	}
	payload := h.oneEvent("consent_revoked")
	s3PayloadExact(t, payload, "consent_id", "anonymise_at")
	if s3String(t, payload, "consent_id") != card.ID {
		t.Fatalf("consent_id = %v", payload["consent_id"])
	}
	when := parseTS(t, s3String(t, payload, "anonymise_at"), token)
	within(t, when, start, end, 7*day, slack, "anonymise_at события")
	s3ExpectTutorNotice(t, h, tutor.ID, card.ID, s3Nick)
	h.logsHide(token, s3Nick, s3Birth(16))
}

func TestPV_R18_S3_CancelOnDay3(t *testing.T) {
	t.Run("link", func(t *testing.T) {
		h := s3New(t)
		tutor := h.login("tg-s3-r18a", "tutor")
		card, token := s3RevokeBackdated(t, h, tutor)
		page := h.consent(token, actionCancelRevoke)
		if page.State != apiGiven || page.AnonymiseAt != nil {
			t.Fatalf("страница %+v", page)
		}
		s3ExpectActiveConsent(t, h, card.ID)
		payload := h.oneEvent("consent_revocation_cancelled")
		s3PayloadExact(t, payload, "consent_id")
		if s3String(t, payload, "consent_id") != card.ID {
			t.Fatal("consent_id отмены")
		}
	})
	t.Run("tutor", func(t *testing.T) {
		h := s3New(t)
		tutor := h.login("tg-s3-r18b", "tutor")
		card, _ := s3RevokeBackdated(t, h, tutor)
		s3Expect(t, h.call(tutor, http.MethodPost, s3StudentPath(card.ID, "/consent/cancel-revoke"), nil), http.StatusNoContent, "")
		s3ExpectActiveConsent(t, h, card.ID)
		if s3String(t, h.oneEvent("consent_revocation_cancelled"), "consent_id") != card.ID {
			t.Fatal("consent_id отмены репетитором")
		}
		rec := h.call(tutor, http.MethodPost, s3StudentPath(card.ID, "/consent/cancel-revoke"), nil)
		s3ExpectConflict(t, rec)
		s3ExpectActiveConsent(t, h, card.ID)
		if n := s3CountKind(h, "consent_revocation_cancelled"); n != 1 {
			t.Fatalf("событий отмены %d", n)
		}
	})
}

func TestPV_R20_S3_ConfirmThenAnonymise(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r20", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	h.consent(token, actionGive)
	path := s3StudentPath(card.ID, "/anonymise")
	s3Expect(t, h.call(tutor, http.MethodPost, path, nil), http.StatusConflict, "confirmation_required")
	s3Expect(t, h.call(tutor, http.MethodPost, path, map[string]any{"confirm": false}), http.StatusConflict, "confirmation_required")
	if _, _, _, ok := h.pii(card.ID); !ok {
		t.Fatal("отказ стёр ПДн")
	}
	if s3CountKind(h, "student_anonymised") != 0 {
		t.Fatal("отказ записал обезличивание")
	}
	s3Expect(t, h.call(tutor, http.MethodPost, path, map[string]any{"confirm": true}), http.StatusNoContent, "")
	if _, _, _, ok := h.pii(card.ID); ok {
		t.Fatal("ПДн на месте")
	}
	if _, ok := h.pseudonym(card.ID); ok {
		t.Fatal("псевдоним на месте")
	}
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1`, card.ID) != 1 {
		t.Fatal("согласие удалено")
	}
	if h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, card.ID) != 0 {
		t.Fatal("ссылка осталась")
	}
	payload := h.oneEvent("student_anonymised")
	s3PayloadExact(t, payload, "source")
	if s3String(t, payload, "source") != "tutor_on_request" {
		t.Fatalf("source = %v", payload["source"])
	}
	rec := h.call(tutor, http.MethodPost, path, map[string]any{"confirm": true})
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusForbidden && rec.Code != http.StatusConflict {
		t.Fatalf("повтор обезличивания: %d %s", rec.Code, rec.Body.Bytes())
	}
	if _, _, _, ok := h.pii(card.ID); ok {
		t.Fatal("повтор вернул ПДн")
	}
	if s3CountKind(h, "student_anonymised") != 1 {
		t.Fatal("повтор записал второе событие")
	}
}

func TestPV_R22_S3_ArchiveKeepsPII(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r22", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	session := h.mint(card.ID)
	archive := s3StudentPath(card.ID, "/archive")
	unarchive := s3StudentPath(card.ID, "/unarchive")
	s3Expect(t, h.call(tutor, http.MethodPost, archive, nil), http.StatusNoContent, "")
	s3ExpectPIIKept(t, h, card.ID)
	if _, _, status := h.user(card.ID); status != "archived" {
		t.Fatalf("status = %s", status)
	}
	if h.openSessions(card.ID) != 0 {
		t.Fatal("сеанс после архивации жив")
	}
	s3Expect(t, h.do(http.MethodGet, s3StudentPath(card.ID, ""), s3Req{cookie: session}), http.StatusUnauthorized, "unauthorized")
	s3PayloadExact(t, h.oneEvent("student_archived"))
	s3ExpectConflict(t, h.call(tutor, http.MethodPost, archive, nil))
	if s3CountKind(h, "student_archived") != 1 {
		t.Fatal("повтор архивации записал второе событие")
	}
	s3ExpectPIIKept(t, h, card.ID)
	s3Expect(t, h.call(tutor, http.MethodPost, unarchive, nil), http.StatusNoContent, "")
	if _, _, status := h.user(card.ID); status != "active" {
		t.Fatalf("после возврата status = %s", status)
	}
	s3ExpectPIIKept(t, h, card.ID)
	seen := s3DecodeCard(t, h.get(tutor, s3StudentPath(card.ID, "")).Body.Bytes())
	if seen.Status != "active" || seen.DisplayName != s3Nick {
		t.Fatalf("карточка после возврата %+v", seen)
	}
	s3PayloadExact(t, h.oneEvent("student_unarchived"))
	s3ExpectConflict(t, h.call(tutor, http.MethodPost, unarchive, nil))
	if s3CountKind(h, "student_unarchived") != 1 {
		t.Fatal("повтор возврата записал второе событие")
	}
}

func TestPV_R27_S3_LogsOmitPII(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r27", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	token := h.link(tutor, card.ID, "parent")
	h.consent(token, actionGive)
	_, telegram, _, ok := h.pii(card.ID)
	if !ok {
		t.Fatal("нет ПДн")
	}
	h.logsHide(s3Nick, s3BirthFixed, token, telegram)
	for _, ev := range h.events() {
		raw := ev.Kind
		for _, value := range ev.Payload {
			if text, ok := value.(string); ok {
				raw += " " + text
			}
		}
		if strings.Contains(raw, s3Nick) || strings.Contains(raw, s3BirthFixed) || strings.Contains(raw, token) {
			t.Fatal("событие содержит ПДн или токен")
		}
	}
}

func TestPV_R28_S3_OutboxOmitsName(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r28", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	h.consent(token, actionGive)
	h.consent(token, actionRevoke)
	s3ExpectTutorNotice(t, h, tutor.ID, card.ID, s3Nick)
}

func TestAU_R4_S3_ForeignMatchesMissing(t *testing.T) {
	h := s3New(t)
	owner := h.login("tg-s3-owner", "tutor")
	other := h.login("tg-s3-other-tutor", "tutor")
	card := h.create(owner, s3Nick, s3BirthFixed)
	missing := "student-s3-missing"
	ops := []struct {
		method string
		tail   string
		body   any
	}{
		{method: http.MethodGet, tail: ""},
		{method: http.MethodPost, tail: "/archive"},
		{method: http.MethodPost, tail: "/unarchive"},
		{method: http.MethodPost, tail: "/anonymise", body: map[string]any{"confirm": true}},
		{method: http.MethodPost, tail: "/consent-links", body: map[string]string{"kind": "parent"}},
		{method: http.MethodPost, tail: "/consent/cancel-revoke"},
	}
	for _, op := range ops {
		foreign := s3As(h, other, op.method, s3StudentPath(card.ID, op.tail), op.body)
		absent := s3As(h, other, op.method, s3StudentPath(missing, op.tail), op.body)
		s3Expect(t, foreign, http.StatusForbidden, "forbidden")
		s3Expect(t, absent, http.StatusForbidden, "forbidden")
		s3SameJSON(t, foreign.Body.Bytes(), absent.Body.Bytes())
	}
	s3ExpectPIIKept(t, h, card.ID)
	if _, _, status := h.user(card.ID); status != "active" {
		t.Fatalf("чужой запрос сменил status на %s", status)
	}
}

func TestAU_R24_S3_CSRFAndOrigin(t *testing.T) {
	t.Run("consent_wrong_origin", func(t *testing.T) {
		h := s3New(t)
		h.exec(`
			INSERT INTO core.student_pii (student_id, display_name, telegram_id, birth_date)
			VALUES ('s3-origin-student', $1, '', $2::date)
		`, s3Nick, s3Birth(16))
		token, err := h.svc.CreateLink(context.Background(), "s3-origin-student", "parent", time.Now())
		if err != nil || token == "" {
			t.Fatalf("CreateLink: %v", err)
		}
		rec := h.do(http.MethodPost, "/consent/"+url.PathEscape(token), s3Req{
			origin: s3EvilOrigin,
			body:   map[string]string{"action": actionGive},
		})
		s3Expect(t, rec, http.StatusForbidden, "csrf")
		if strings.Contains(rec.Body.String(), token) {
			t.Fatal("ошибка содержит токен")
		}
		if h.count(`SELECT count(*) FROM core.consent WHERE student_id = 's3-origin-student'`) != 0 {
			t.Fatal("чужой Origin записал согласие")
		}
	})
	t.Run("tutor_post", func(t *testing.T) {
		h := s3New(t)
		tutor := h.login("tg-s3-r24", "tutor")
		body := map[string]any{
			"display_name": s3Nick,
			"birth_date":   s3BirthFixed,
			"subjects":     []string{s3Subject},
		}
		before := h.count(`SELECT count(*) FROM core.student_pii`)
		s3Expect(t, h.do(http.MethodPost, "/students", s3Req{cookie: tutor.Token, origin: s3Origin, body: body}), http.StatusForbidden, "csrf")
		s3Expect(t, h.do(http.MethodPost, "/students", s3Req{cookie: tutor.Token, origin: s3EvilOrigin, csrf: tutor.Token, body: body}), http.StatusForbidden, "csrf")
		s3Expect(t, h.do(http.MethodPost, "/students", s3Req{cookie: tutor.Token, csrf: tutor.Token, body: body}), http.StatusForbidden, "csrf")
		s3Expect(t, h.do(http.MethodPost, "/students", s3Req{cookie: tutor.Token, origin: s3Origin, csrf: "not-the-session", body: body}), http.StatusForbidden, "csrf")
		if h.count(`SELECT count(*) FROM core.student_pii`) != before {
			t.Fatal("отказ CSRF создал ученика")
		}
		card := h.create(tutor, s3Nick, s3BirthFixed)
		s3Expect(t, h.get(tutor, s3StudentPath(card.ID, "")), http.StatusOK, "")
	})
}

func TestS3_StudentBlockedFromTutorOps(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-block", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	pupil := s3Actor{ID: card.ID, Token: h.mint(card.ID), Role: "student"}
	stranger := h.login("tg-s3-stranger-student", "student")
	ops := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/students", map[string]any{"display_name": "demo-s3-x", "birth_date": s3BirthFixed, "subjects": []string{s3Subject}}},
		{http.MethodPost, s3StudentPath(card.ID, "/archive"), nil},
		{http.MethodPost, s3StudentPath(card.ID, "/unarchive"), nil},
		{http.MethodPost, s3StudentPath(card.ID, "/anonymise"), map[string]any{"confirm": true}},
		{http.MethodPost, s3StudentPath(card.ID, "/consent-links"), map[string]string{"kind": "parent"}},
		{http.MethodPost, s3StudentPath(card.ID, "/consent/cancel-revoke"), nil},
	}
	for _, actor := range []s3Actor{pupil, stranger} {
		for _, op := range ops {
			s3Expect(t, s3As(h, actor, op.method, op.path, op.body), http.StatusForbidden, "forbidden")
		}
	}
	s3ExpectPIIKept(t, h, card.ID)
	if _, _, status := h.user(card.ID); status != "active" {
		t.Fatalf("status = %s", status)
	}
}

func TestS3_NoSessionUnauthorized(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-anon", "tutor")
	card := h.create(tutor, s3Nick, s3BirthFixed)
	body := map[string]any{"display_name": "demo-s3-x", "birth_date": s3BirthFixed, "subjects": []string{s3Subject}}
	paths := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/students", body},
		{http.MethodGet, s3StudentPath(card.ID, ""), nil},
		{http.MethodPost, s3StudentPath(card.ID, "/archive"), nil},
		{http.MethodPost, s3StudentPath(card.ID, "/unarchive"), nil},
		{http.MethodPost, s3StudentPath(card.ID, "/anonymise"), map[string]any{"confirm": true}},
		{http.MethodPost, s3StudentPath(card.ID, "/consent-links"), map[string]string{"kind": "parent"}},
		{http.MethodPost, s3StudentPath(card.ID, "/consent/cancel-revoke"), nil},
	}
	for _, path := range paths {
		s3Expect(t, h.do(path.method, path.path, s3Req{origin: s3Origin, body: path.body}), http.StatusUnauthorized, "unauthorized")
	}
}

type s3ExampleRow struct {
	ID     string `yaml:"id"`
	Rule   string `yaml:"rule"`
	Expect string `yaml:"expect"`
}

func TestExamples_S3_Runner(t *testing.T) {
	var rows []s3ExampleRow
	if err := yaml.Unmarshal(s3RepoFile(t, "docs/platform/privacy.examples.yaml"), &rows); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]s3ExampleRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	spec := s3RepoFile(t, "docs/contracts/core-api.yaml")
	want := []struct {
		id, rule, part string
	}{
		{"privacy-01", "PV-R1", "отказ схемы"},
		{"privacy-03", "PV-R6", "student_pseudonym"},
		{"privacy-04", "PV-R10", "410"},
		{"privacy-05", "PV-R11", "состояние не изменилось"},
		{"privacy-06", "PV-R12", "отказ"},
		{"privacy-07", "PV-R12", "отказ"},
		{"privacy-11", "PV-R17", "anonymise_at"},
		{"privacy-12", "PV-R18", "согласие действует"},
		{"privacy-14", "PV-R20", "tutor_on_request"},
		{"privacy-15", "PV-R22", "unarchive"},
	}
	for _, item := range want {
		row, ok := byID[item.id]
		if !ok || row.Rule != item.rule || !strings.Contains(row.Expect, item.part) {
			t.Fatalf("строка %s: %+v", item.id, row)
		}
		t.Run(item.id, func(t *testing.T) {
			s3RunExample(t, spec, item.id)
		})
	}
}

func s3RunExample(t *testing.T, spec []byte, id string) {
	t.Helper()
	h := s3New(t)
	switch id {
	case "privacy-01":
		if err := h.tryQuery(`SELECT school FROM core.student_pii`); err == nil {
			t.Fatal("схема приняла столбец school")
		}
	case "privacy-03":
		var tutor s3Actor
		s3Run(t, h, spec, `
- id: privacy-03
  given: {id: privacy-03}
  steps:
    - op: createStudent
      request:
        display_name: demo-s3-nick
        birth_date: "2010-04-02"
        subjects: [algebra]
      expect: {http: 201}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor = h.login("tg-s3-ex03", "tutor")
			return scenario.Bindings{Header: s3AuthHeaders(tutor)}, nil
		}, nil)
		studentID := h.text(`SELECT student_id FROM core.student_pii WHERE display_name = $1`, s3Nick)
		code, ok := h.pseudonym(studentID)
		if !ok || !s3CodeRE.MatchString(code) {
			t.Fatalf("псевдоним %q", code)
		}
	case "privacy-04":
		var token string
		s3Run(t, h, spec, `
- id: privacy-04
  given: {id: privacy-04}
  steps:
    - op: getConsent
      expect: {http: 410}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor := h.login("tg-s3-ex04", "tutor")
			card := h.create(tutor, s3Nick, s3Birth(16))
			token = h.link(tutor, card.ID, "parent")
			h.exec(`
				UPDATE core.consent_link
				SET created_at = now() - interval '15 days',
				    expires_at = now() - interval '1 day'
				WHERE student_id = $1
			`, card.ID)
			return scenario.Bindings{Path: map[string]string{"token": token}}, nil
		}, nil)
	case "privacy-05":
		var snap string
		s3Run(t, h, spec, `
- id: privacy-05
  given: {id: privacy-05}
  steps:
    - op: getConsent
      expect: {http: 200}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor := h.login("tg-s3-ex05", "tutor")
			card := h.create(tutor, s3Nick, s3Birth(16))
			token := h.link(tutor, card.ID, "parent")
			snap = s3ConsentSnap(h, card.ID)
			return scenario.Bindings{Path: map[string]string{"token": token}}, nil
		}, nil)
		if s3ConsentSnap(h, h.text(`SELECT student_id FROM core.student_pii WHERE display_name = $1`, s3Nick)) != snap {
			t.Fatal("предпросмотр изменил состояние")
		}
	case "privacy-06":
		s3RunKindRefusal(t, h, spec, "privacy-06", 16, "student")
	case "privacy-07":
		s3RunKindRefusal(t, h, spec, "privacy-07", 18, "parent")
	case "privacy-11":
		var studentID, token string
		s3Run(t, h, spec, `
- id: privacy-11
  given: {id: privacy-11}
  steps:
    - op: postConsent
      request: {action: revoke}
      expect: {http: 200}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor := h.login("tg-s3-ex11", "tutor")
			card := h.create(tutor, s3Nick, s3Birth(16))
			studentID = card.ID
			token = h.link(tutor, card.ID, "parent")
			h.consent(token, actionGive)
			h.mint(card.ID)
			return scenario.Bindings{Path: map[string]string{"token": token}}, nil
		}, nil)
		if h.openSessions(studentID) != 0 {
			t.Fatal("сеансы не отозваны")
		}
		when := s3String(t, h.oneEvent("consent_revoked"), "anonymise_at")
		if when == "" {
			t.Fatal("нет anonymise_at")
		}
		s3ExpectTutorNotice(t, h, h.text(`SELECT id FROM core.app_user WHERE role = 'tutor'`), studentID, s3Nick)
	case "privacy-12":
		var studentID string
		s3Run(t, h, spec, `
- id: privacy-12
  given: {id: privacy-12}
  steps:
    - op: postConsent
      request: {action: cancel_revoke}
      expect: {http: 200}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor := h.login("tg-s3-ex12", "tutor")
			card, token := s3RevokeBackdated(t, h, tutor)
			studentID = card.ID
			return scenario.Bindings{Path: map[string]string{"token": token}}, nil
		}, nil)
		s3ExpectActiveConsent(t, h, studentID)
	case "privacy-14":
		var studentID string
		s3Run(t, h, spec, `
- id: privacy-14
  given: {id: privacy-14}
  steps:
    - op: anonymiseStudent
      request: {confirm: true}
      expect: {http: 204}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor := h.login("tg-s3-ex14", "tutor")
			card := h.create(tutor, s3Nick, s3Birth(16))
			studentID = card.ID
			return scenario.Bindings{Path: map[string]string{"id": card.ID}, Header: s3AuthHeaders(tutor)}, nil
		}, nil)
		if _, _, _, ok := h.pii(studentID); ok {
			t.Fatal("ПДн на месте")
		}
		if s3String(t, h.oneEvent("student_anonymised"), "source") != "tutor_on_request" {
			t.Fatal("source не tutor_on_request")
		}
	case "privacy-15":
		var studentID string
		s3Run(t, h, spec, `
- id: privacy-15
  given: {id: privacy-15}
  steps:
    - op: studentArchive
      expect:
        http: 204
        journal: {status: archived, pii: true, sessions: 0}
    - op: studentUnarchive
      expect:
        http: 204
        journal: {status: active, pii: true}
`, func(map[string]any) (scenario.Bindings, error) {
			tutor := h.login("tg-s3-ex15", "tutor")
			card := h.create(tutor, s3Nick, s3BirthFixed)
			studentID = card.ID
			h.mint(card.ID)
			return scenario.Bindings{Path: map[string]string{"id": card.ID}, Header: s3AuthHeaders(tutor)}, nil
		}, func() map[string]any {
			_, _, status := h.user(studentID)
			_, _, _, ok := h.pii(studentID)
			return map[string]any{"status": status, "pii": ok, "sessions": h.openSessions(studentID)}
		})
	default:
		t.Fatalf("нет привязки %s", id)
	}
}

func s3RunKindRefusal(t *testing.T, h *s3Harness, spec []byte, id string, years int, kind string) {
	t.Helper()
	var studentID string
	s3Run(t, h, spec, `
- id: `+id+`
  given: {id: `+id+`}
  steps:
    - op: createConsentLink
      request: {kind: `+kind+`}
      expect: {http: 422}
`, func(map[string]any) (scenario.Bindings, error) {
		tutor := h.login("tg-s3-"+id, "tutor")
		card := h.create(tutor, s3Nick, s3Birth(years))
		studentID = card.ID
		return scenario.Bindings{Path: map[string]string{"id": card.ID}, Header: s3AuthHeaders(tutor)}, nil
	}, nil)
	if h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, studentID) != 0 {
		t.Fatal("отказ записал ссылку")
	}
}

func s3Run(t *testing.T, h *s3Harness, spec []byte, doc string, load scenario.Loader, journal func() map[string]any) {
	t.Helper()
	scenario.Run(t, scenario.Options{
		Spec:      spec,
		Scenarios: []byte(doc),
		Handler:   s3JSONHandler(h.svc.Handler()),
		Load:      load,
		Journal:   journal,
	})
}

func s3AuthHeaders(actor s3Actor) map[string]string {
	return map[string]string{
		"Cookie":       s3Cookie + "=" + actor.Token,
		"Origin":       s3Origin,
		"X-CSRF-Token": actor.Token,
	}
}

func s3As(h *s3Harness, actor s3Actor, method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	if method == http.MethodGet {
		return h.get(actor, path)
	}
	return h.call(actor, method, path, body)
}

func s3GrantAndCheck(t *testing.T, h *s3Harness, tutor s3Actor, birth, kind string) {
	t.Helper()
	card := h.create(tutor, s3Nick+"-"+kind, birth)
	token := h.link(tutor, card.ID, kind)
	start := time.Now()
	page := h.consent(token, actionGive)
	end := time.Now()
	if page.State != apiGiven || page.Kind != kind || page.TextVersion != textVersion || page.AnonymiseAt != nil {
		t.Fatalf("страница %+v", page)
	}
	s3ScanConsent(t, h, card.ID, kind, start, end)
	row := h.text(`SELECT row_to_json(c)::text FROM core.consent c WHERE student_id = $1`, card.ID)
	if strings.Contains(row, s3Nick) || strings.Contains(row, birth) {
		t.Fatal("запись согласия содержит ПДн")
	}
	found := false
	for _, ev := range h.events() {
		if ev.Kind != "consent_given" {
			continue
		}
		if s3String(t, ev.Payload, "consent_id") != card.ID {
			continue
		}
		s3PayloadExact(t, ev.Payload, "consent_id", "kind", "text_version")
		if s3String(t, ev.Payload, "kind") != kind || s3String(t, ev.Payload, "text_version") != textVersion {
			t.Fatalf("payload %#v", ev.Payload)
		}
		found = true
	}
	if !found {
		t.Fatalf("нет consent_given для %s", card.ID)
	}
}

func s3ScanConsent(t *testing.T, h *s3Harness, studentID, kind string, start, end time.Time) {
	t.Helper()
	var version int
	var state, grantedBy, method string
	var granted time.Time
	var revoked, anonymise *time.Time
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT version, state, granted_at, revoked_at, anonymise_at, granted_by, method
			FROM core.consent WHERE student_id = $1
		`, studentID).Scan(&version, &state, &granted, &revoked, &anonymise, &grantedBy, &method)
	})
	if version != 1 || state != dbActive || grantedBy != kind || method != "link_button" {
		t.Fatalf("consent version=%d state=%s by=%s method=%s", version, state, grantedBy, method)
	}
	if revoked != nil || anonymise != nil {
		t.Fatal("у действующего согласия заполнены revoked_at или anonymise_at")
	}
	within(t, granted, start, end, 0, slack, "granted_at")
}

func s3AssertLinkRow(t *testing.T, h *s3Harness, studentID, token, kind string, start, end time.Time) {
	t.Helper()
	raw, err := decodeToken(token)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		hash    []byte
		student string
		kind    string
		created time.Time
		expires time.Time
	}
	var rows []row
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		scanned, err := conn.Query(ctx, `
			SELECT token_hash, student_id, kind, created_at, expires_at
			FROM core.consent_link WHERE student_id = $1
		`, studentID)
		if err != nil {
			return err
		}
		defer scanned.Close()
		for scanned.Next() {
			var item row
			if err := scanned.Scan(&item.hash, &item.student, &item.kind, &item.created, &item.expires); err != nil {
				return err
			}
			rows = append(rows, item)
		}
		return scanned.Err()
	})
	n := 0
	var found row
	for _, item := range rows {
		if tokenIsHash(item.hash, token, raw) {
			found = item
			n++
		}
	}
	if n != 1 || found.kind != kind || found.student != studentID {
		t.Fatalf("строк ссылки %d kind=%s", n, found.kind)
	}
	within(t, found.created, start, end, 0, slack, "created_at")
	within(t, found.expires, start, end, 14*day, slack, "expires_at")
}

func s3ExpectPage(t *testing.T, h *s3Harness, token, kind, state string) {
	t.Helper()
	rec := h.do(http.MethodGet, "/consent/"+url.PathEscape(token), s3Req{})
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("страница содержит токен")
	}
	s3Expect(t, rec, http.StatusOK, "")
	var page consentPage
	s3Decode(t, rec.Body.Bytes(), &page)
	if page.Kind != kind || page.State != state || page.TextVersion != textVersion {
		t.Fatalf("страница %+v", page)
	}
	if !strings.Contains(page.Text, consentTitle) {
		t.Fatal("нет текста согласия")
	}
}

func s3RefuseKind(t *testing.T, h *s3Harness, tutor s3Actor, studentID, kind string) {
	t.Helper()
	before := h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, studentID)
	rec := h.call(tutor, http.MethodPost, s3StudentPath(studentID, "/consent-links"), map[string]string{"kind": kind})
	s3Expect(t, rec, http.StatusUnprocessableEntity, "consent_kind_invalid")
	if h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, studentID) != before {
		t.Fatal("отказ записал ссылку")
	}
}

func s3KindOf(t *testing.T, h *s3Harness, token string) string {
	t.Helper()
	rec := h.do(http.MethodGet, "/consent/"+url.PathEscape(token), s3Req{})
	var page consentPage
	s3Decode(t, rec.Body.Bytes(), &page)
	return page.Kind
}

func s3ExpectRevokedRow(t *testing.T, h *s3Harness, studentID string, start, end time.Time) {
	t.Helper()
	var state string
	var revoked, anonymise *time.Time
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT state, revoked_at, anonymise_at
			FROM core.consent WHERE student_id = $1
		`, studentID).Scan(&state, &revoked, &anonymise)
	})
	if state != dbRevoked || revoked == nil || anonymise == nil {
		t.Fatalf("state=%s revoked=%v anonymise=%v", state, revoked, anonymise)
	}
	within(t, *revoked, start, end, 0, slack, "revoked_at")
	within(t, *anonymise, start, end, 7*day, slack, "anonymise_at")
}

func s3ExpectActiveConsent(t *testing.T, h *s3Harness, studentID string) {
	t.Helper()
	var state string
	var revoked, anonymise *time.Time
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT state, revoked_at, anonymise_at FROM core.consent WHERE student_id = $1
		`, studentID).Scan(&state, &revoked, &anonymise)
	})
	if state != dbActive || revoked != nil || anonymise != nil {
		t.Fatalf("state=%s revoked=%v anonymise=%v", state, revoked, anonymise)
	}
}

func s3ExpectTutorNotice(t *testing.T, h *s3Harness, tutorID, studentID, name string) {
	t.Helper()
	dump := h.text(`
		SELECT coalesce(string_agg(
			coalesce(kind, '') || ' ' || coalesce(template, '') || ' ' ||
			coalesce(recipient_kind, '') || ' ' || coalesce(recipient_id, '') || ' ' ||
			coalesce(state, '') || ' ' || coalesce(payload::text, '') || ' ' ||
			coalesce(params::text, ''),
			E'\n'), '')
		FROM core.outbox
	`)
	if strings.Contains(dump, name) {
		t.Fatal("outbox хранит имя ученика")
	}
	var kind, template, recipientID, state, params, payload string
	found := false
	h.with(func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT coalesce(kind, ''), coalesce(template, ''), coalesce(recipient_id, ''),
			       coalesce(state, ''), coalesce(params::text, ''), coalesce(payload::text, '')
			FROM core.outbox
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k, tmpl, rid, st, par, pay string
			if err := rows.Scan(&k, &tmpl, &rid, &st, &par, &pay); err != nil {
				return err
			}
			if k == "consent_revoked" || tmpl == "consent_revoked" {
				kind, template, recipientID, state, params, payload = k, tmpl, rid, st, par, pay
				found = true
			}
		}
		return rows.Err()
	})
	if !found {
		t.Fatal("нет сообщения consent_revoked")
	}
	_ = kind
	_ = template
	if recipientID != tutorID || state != "pending" {
		t.Fatalf("получатель %s state %s", recipientID, state)
	}
	if !strings.Contains(params, studentID) && !strings.Contains(payload, studentID) {
		t.Fatal("в сообщении нет id ученика")
	}
}

func s3RevokeBackdated(t *testing.T, h *s3Harness, tutor s3Actor) (s3Card, string) {
	t.Helper()
	card := h.create(tutor, s3Nick, s3Birth(16))
	token := h.link(tutor, card.ID, "parent")
	h.consent(token, actionGive)
	h.consent(token, actionRevoke)
	h.exec(`
		UPDATE core.consent
		SET revoked_at = now() - interval '3 days',
		    anonymise_at = now() + interval '4 days'
		WHERE student_id = $1
	`, card.ID)
	return card, token
}

func s3MarkDue(h *s3Harness, studentID string, at time.Time) {
	h.t.Helper()
	revoked := at.Add(-8 * 24 * time.Hour)
	h.exec(`
		INSERT INTO core.consent (
			student_id, version, state, granted_at, revoked_at, anonymise_at, granted_by, method
		) VALUES ($1, 1, 'revoked', $2, $2, $3, 'parent', 'link_button')
	`, studentID, revoked, at)
}

func s3SeedResidue(t *testing.T, h *s3Harness, studentID, tutorID string) {
	t.Helper()
	var invite [32]byte
	var link [32]byte
	if _, err := rand.Read(invite[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(link[:]); err != nil {
		t.Fatal(err)
	}
	h.exec(`UPDATE core.student_pii SET telegram_id = 'tg-s3-bound' WHERE student_id = $1`, studentID)
	h.exec(`INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, 'tg-s3-bound')`, studentID)
	h.exec(`
		INSERT INTO core.telegram_invite (token_hash, student_id, expires_at)
		VALUES ($1, $2, now() + interval '1 day')
	`, invite[:], studentID)
	h.exec(`
		INSERT INTO core.consent_link (token_hash, student_id, kind, expires_at, created_at)
		VALUES ($1, $2, 'parent', now() + interval '1 day', now())
	`, link[:], studentID)
	h.exec(`
		INSERT INTO core.assignment (id, student_id, state, kind, catalog_version, place, snapshot)
		VALUES ('s3-asg', $1, 'issued', 'homework', 1, 'home', '{}'::jsonb)
	`, studentID)
	h.exec(`
		INSERT INTO core.assignment_item (assignment_id, item_id, state, max_points)
		VALUES ('s3-asg', 's3-item', 'open', 1)
	`)
	h.exec(`
		INSERT INTO core.attempts (id, assignment_id, item_id, raw_input, source, place, attempt_no)
		VALUES ('s3-att', 's3-asg', 's3-item', '42', 'homework', 'home', 1)
	`)
	h.exec(`
		INSERT INTO core.observation (
			assignment_id, item_id, revision, first_attempt_points, total_points, max_points
		) VALUES ('s3-asg', 's3-item', 1, 0, 0, 1)
	`)
	h.exec(`
		INSERT INTO core.outbox (id, kind, payload, recipient_kind, recipient_id, template, params, state)
		VALUES (
			's3-to-student', 'homework_issued', '{}'::jsonb, 'student', $1,
			'homework_issued', jsonb_build_object('student_id', $1::text), 'pending'
		)
	`, studentID)
	h.exec(`
		INSERT INTO core.outbox (id, kind, payload, recipient_kind, recipient_id, template, params, state)
		VALUES (
			's3-to-tutor', 'homework_submitted', '{}'::jsonb, 'tutor', $1,
			'homework_submitted', jsonb_build_object('student_id', $2::text), 'pending'
		)
	`, tutorID, studentID)
}

func s3ExpectAnonymised(t *testing.T, h *s3Harness, tutor s3Actor, studentID, name string) {
	t.Helper()
	if _, _, _, ok := h.pii(studentID); ok {
		t.Fatal("ПДн на месте")
	}
	if _, ok := h.pseudonym(studentID); ok {
		t.Fatal("псевдоним на месте")
	}
	if _, _, status := h.user(studentID); status != "anonymised" {
		t.Fatalf("status = %s", status)
	}
	rec := h.get(tutor, s3StudentPath(studentID, ""))
	s3Expect(t, rec, http.StatusOK, "")
	if strings.Contains(rec.Body.String(), name) {
		t.Fatal("карточка содержит прежнее имя")
	}
	if s3DecodeCard(t, rec.Body.Bytes()).Status != "anonymised" {
		t.Fatal("карточка не anonymised")
	}
}

func s3ExpectPIIKept(t *testing.T, h *s3Harness, studentID string) {
	t.Helper()
	if _, _, _, ok := h.pii(studentID); !ok {
		t.Fatal("ПДн удалены")
	}
	if _, ok := h.pseudonym(studentID); !ok {
		t.Fatal("псевдоним удалён")
	}
}

func s3ConsentSnap(h *s3Harness, studentID string) string {
	h.t.Helper()
	links := h.text(`
		SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.created_at)::text, '[]')
		FROM core.consent_link t WHERE student_id = $1
	`, studentID)
	consent := h.text(`
		SELECT coalesce(json_agg(row_to_json(t))::text, '[]')
		FROM core.consent t WHERE student_id = $1
	`, studentID)
	return links + "\n" + consent
}

func s3CountKind(h *s3Harness, kind string) int {
	h.t.Helper()
	n := 0
	for _, ev := range h.events() {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}
