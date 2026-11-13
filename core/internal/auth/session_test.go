package auth_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAU_R21_SessionTokenIsHashed(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	raw, err := decodeToken(in.Token)
	if err != nil {
		t.Fatal(err)
	}
	row := h.sessionByToken(in.Token)
	if string(row.Hash) == string(raw) || string(row.Hash) == in.Token {
		t.Fatal("в core.session лежит сырой токен")
	}
	if row.UserID != in.Me.UserID {
		t.Fatalf("session.user_id = %s", row.UserID)
	}
	if row.Revoked != nil {
		t.Fatal("новый сеанс уже отозван")
	}
	h.assertNoRawToken(in.Token, raw)
}

func TestAU_R21_EachLoginCreatesNewSession(t *testing.T) {
	h := newHarness(t, "local", true)
	first := h.loginOK(demoID(1), "student")
	second := h.loginOK(demoID(1), "student")
	if first.Token == second.Token {
		t.Fatal("повторный вход не создал новый токен")
	}
	rows := h.sessionsFor(first.Me.UserID)
	if len(rows) != 2 {
		t.Fatalf("сеансов %d", len(rows))
	}
	h.expectMe(first)
	h.expectMe(second)
}

func TestAU_R21_UnknownTokenUnauthorized(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(strings.Repeat("ab", 32)))
	h.expectMe(in)
}

func TestAU_R22_HostSessionCookieAttributes(t *testing.T) {
	h := newHarness(t, "local", true)
	rec := h.call(http.MethodPost, pathDev, devJSON(t, demoID(1), "student"), ptr(appOrigin), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("вход = %d %s", rec.Code, rec.Body.Bytes())
	}
	_, raw := sessionToken(t, rec)
	cookie, err := parseSetCookie(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Name != cookieName {
		t.Fatalf("имя cookie %s", cookie.Name)
	}
	if _, bad := cookie.flags["domain"]; bad {
		t.Fatal("у cookie __Host-session есть Domain")
	}
	if path := cookie.flags["path"]; path != "/" {
		t.Fatalf("Path = %q", path)
	}
	if _, ok := cookie.flags["secure"]; !ok {
		t.Fatal("нет атрибута Secure")
	}
	if _, ok := cookie.flags["httponly"]; !ok {
		t.Fatal("нет атрибута HttpOnly")
	}
	if same := cookie.flags["samesite"]; !strings.EqualFold(same, "Lax") {
		t.Fatalf("SameSite = %q", same)
	}
	if len(rec.Header().Values("Set-Cookie")) != 1 {
		t.Fatalf("Set-Cookie: %v", rec.Header().Values("Set-Cookie"))
	}
}

func TestAU_R22_GetMeWithCookie(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	h.expectMe(in)
}

func TestAU_R22_GetMeWithoutCookieUnauthorized(t *testing.T) {
	h := newHarness(t, "local", true)
	h.expectUnauthorized(http.MethodGet, pathMe, nil)
}

func TestAU_R23_StudentExpiresIn90Days(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	assertTTL(t, h, in.Token, studentTTL)
}

func TestAU_R23_TutorExpiresIn7Days(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "tutor")
	assertTTL(t, h, in.Token, tutorTTL)
}

func TestAU_R23_SessionDoesNotExtend(t *testing.T) {
	h := newHarness(t, "local", true)
	for i, role := range []string{"student", "tutor"} {
		in := h.loginOK(demoID(i+1), role)
		before := h.sessionByToken(in.Token).Expires
		h.expectMe(in)
		after := h.sessionByToken(in.Token).Expires
		if !after.Equal(before) {
			t.Fatalf("%s: GET /me продлил сеанс", role)
		}
		again := h.loginOK(demoID(i+1), role)
		if again.Token == in.Token {
			t.Fatal("повторный вход переиспользовал сеанс")
		}
		if !h.sessionByToken(in.Token).Expires.Equal(before) {
			t.Fatalf("%s: новый вход продлил старый сеанс", role)
		}
	}
}

func TestAU_R23_StudentDay91Unauthorized(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	h.shiftSession(in.Token, 91)
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(in.Token))
}

func TestAU_R23_TutorDay8Unauthorized(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "tutor")
	h.shiftSession(in.Token, 8)
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(in.Token))
}

func TestAU_R24_DevLoginRequiresMatchingOrigin(t *testing.T) {
	h := newHarness(t, "local", true)
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
		rec := h.call(http.MethodPost, pathDev, devJSON(t, demoID(1), "student"), tc.origin, nil, nil)
		if rec.Code != http.StatusForbidden || decodeErr(t, rec.Body.Bytes()) != "csrf" {
			t.Fatalf("%s: %d %s", tc.name, rec.Code, rec.Body.Bytes())
		}
		if hasSessionCookie(rec) {
			t.Fatalf("%s: чужой Origin получил cookie сеанса", tc.name)
		}
	}
	h.loginOK(demoID(1), "student")
}

func TestAU_R24_DevLoginIgnoresCSRF(t *testing.T) {
	h := newHarness(t, "local", true)
	wrong := "csrf-demo-wrong"
	without := h.call(http.MethodPost, pathDev, devJSON(t, demoID(1), "student"), ptr(appOrigin), nil, nil)
	if without.Code != http.StatusOK {
		t.Fatalf("без CSRF = %d %s", without.Code, without.Body.Bytes())
	}
	withWrong := h.call(http.MethodPost, pathDev, devJSON(t, demoID(2), "student"), ptr(appOrigin), &wrong, nil)
	if withWrong.Code != http.StatusOK {
		t.Fatalf("чужой CSRF до сеанса = %d %s", withWrong.Code, withWrong.Body.Bytes())
	}
}

func TestAU_R24_CSRFHeaderMatchesTokenAndStaysOutOfMe(t *testing.T) {
	h := newHarness(t, "local", true)
	rec := h.call(http.MethodPost, pathDev, devJSON(t, demoID(1), "student"), ptr(appOrigin), nil, nil)
	token, _ := sessionToken(t, rec)
	me := decodeMe(t, rec.Body.Bytes())
	if rec.Header().Get("X-CSRF-Token") != token {
		t.Fatal("X-CSRF-Token не равен токену cookie")
	}
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("тело Me содержит токен")
	}
	if me.UserID == "" || me.Role != "student" {
		t.Fatalf("Me = %+v", me)
	}
}

func TestAU_R24_LogoutRejectsBadCSRFOrOrigin(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	origin := appOrigin
	wrong := "csrf-demo-wrong"
	empty := ""
	foreign := "http://evil.example"
	cases := []struct {
		name   string
		origin *string
		csrf   *string
	}{
		{name: "no-csrf", origin: &origin, csrf: nil},
		{name: "empty-csrf", origin: &origin, csrf: &empty},
		{name: "wrong-csrf", origin: &origin, csrf: &wrong},
		{name: "foreign-origin", origin: &foreign, csrf: &in.Token},
		{name: "no-origin", origin: nil, csrf: &in.Token},
	}
	for _, tc := range cases {
		rec := h.call(http.MethodPost, pathLogout, nil, tc.origin, tc.csrf, sessionCookie(in.Token))
		if rec.Code != http.StatusForbidden || decodeErr(t, rec.Body.Bytes()) != "csrf" {
			t.Fatalf("%s: %d %s", tc.name, rec.Code, rec.Body.Bytes())
		}
		h.expectMe(in)
		if h.sessionByToken(in.Token).Revoked != nil {
			t.Fatalf("%s: отказ CSRF отозвал сеанс", tc.name)
		}
	}
}

func TestAU_R24_GetMeIgnoresCSRFAndOrigin(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	foreign := "http://evil.example"
	wrong := "csrf-demo-wrong"
	rec := h.call(http.MethodGet, pathMe, nil, &foreign, &wrong, sessionCookie(in.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /me с чужим Origin = %d %s", rec.Code, rec.Body.Bytes())
	}
	if decodeMe(t, rec.Body.Bytes()) != in.Me {
		t.Fatal("GET /me вернул другого пользователя")
	}
}

func TestAU_R25_LogoutRevokesOnlyCurrentSession(t *testing.T) {
	h := newHarness(t, "local", true)
	first := h.loginOK(demoID(1), "student")
	second := h.loginOK(demoID(1), "student")
	rec := h.call(http.MethodPost, pathLogout, nil, ptr(appOrigin), &first.Token, sessionCookie(first.Token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("тело 204: %q", rec.Body.String())
	}
	if h.sessionByToken(first.Token).Revoked == nil {
		t.Fatal("текущий сеанс не отозван")
	}
	if h.sessionByToken(second.Token).Revoked != nil {
		t.Fatal("выход отозвал другой сеанс")
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(first.Token))
	h.expectMe(second)
}

func TestAU_R25_LogoutWithoutSessionUnauthorized(t *testing.T) {
	h := newHarness(t, "local", true)
	h.loginOK(demoID(1), "student")
	rec := h.call(http.MethodPost, pathLogout, nil, ptr(appOrigin), ptr("csrf-demo-unused"), nil)
	expectAPI(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestAU_R26_SixthStudentSessionRevokesOldest(t *testing.T) {
	h := newHarness(t, "local", true)
	bystander := h.loginOK(demoID(2), "student")
	tutor := h.loginOK(demoID(3), "tutor")
	first := h.loginAged(demoID(1), "student", 5)
	sixth := h.loginOK(demoID(1), "student")
	rows := h.sessionsFor(first[0].Me.UserID)
	if len(rows) != 6 {
		t.Fatalf("сеансов ученика %d", len(rows))
	}
	var revoked []dbSession
	oldest := rows[0]
	for _, row := range rows {
		if row.Created.Before(oldest.Created) {
			oldest = row
		}
		if row.Revoked != nil {
			revoked = append(revoked, row)
		}
	}
	if len(revoked) != 1 {
		t.Fatalf("отозвано сеансов %d", len(revoked))
	}
	if !tokenIsHash(oldest.Hash, first[0].Token) || !tokenIsHash(revoked[0].Hash, first[0].Token) {
		t.Fatal("отозван не самый старый сеанс")
	}
	h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(first[0].Token))
	for _, in := range append(first[1:], sixth) {
		h.expectMe(in)
	}
	h.expectMe(bystander)
	h.expectMe(tutor)
}

func TestAU_R26_FiveStudentSessionsStay(t *testing.T) {
	h := newHarness(t, "local", true)
	five := h.loginAged(demoID(1), "student", 5)
	rows := h.sessionsFor(five[0].Me.UserID)
	if len(rows) != 5 {
		t.Fatalf("сеансов %d", len(rows))
	}
	for _, row := range rows {
		if row.Revoked != nil {
			t.Fatal("пятый сеанс отозвал предыдущий")
		}
	}
	for _, in := range five {
		h.expectMe(in)
	}
}

func TestAU_R26_InactiveSessionDoesNotCountTowardCap(t *testing.T) {
	t.Run("revoked", func(t *testing.T) {
		h := newHarness(t, "local", true)
		five := h.loginAged(demoID(1), "student", 5)
		rec := h.call(http.MethodPost, pathLogout, nil, ptr(appOrigin), &five[0].Token, sessionCookie(five[0].Token))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("logout = %d", rec.Code)
		}
		sixth := h.loginOK(demoID(1), "student")
		h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(five[0].Token))
		for _, in := range append(five[1:], sixth) {
			h.expectMe(in)
		}
	})
	t.Run("expired", func(t *testing.T) {
		h := newHarness(t, "local", true)
		five := h.loginAged(demoID(1), "student", 5)
		h.shiftSession(five[0].Token, 91)
		sixth := h.loginOK(demoID(1), "student")
		h.expectUnauthorized(http.MethodGet, pathMe, sessionCookie(five[0].Token))
		for _, in := range append(five[1:], sixth) {
			h.expectMe(in)
		}
	})
}

func TestAU_R26_TutorHasNoFiveSessionCap(t *testing.T) {
	h := newHarness(t, "local", true)
	var sessions []loggedIn
	for i := 0; i < 6; i++ {
		sessions = append(sessions, h.loginOK(demoID(1), "tutor"))
	}
	rows := h.sessionsFor(sessions[0].Me.UserID)
	if len(rows) != 6 {
		t.Fatalf("сеансов репетитора %d", len(rows))
	}
	for _, row := range rows {
		if row.Revoked != nil {
			t.Fatal("у репетитора отозван сеанс из-за числа входов")
		}
	}
	for _, in := range sessions {
		h.expectMe(in)
	}
}

func TestAU_R27_SecondStudentLoginWorksWithoutDeviceConfirm(t *testing.T) {
	h := newHarness(t, "local", true)
	first := h.loginOK(demoID(1), "student")
	second := h.loginOK(demoID(1), "student")
	if first.Me != second.Me {
		t.Fatalf("второй вход с другого устройства: %+v и %+v", first.Me, second.Me)
	}
	h.expectMe(first)
	h.expectMe(second)
	if h.sessionByToken(first.Token).Revoked != nil || h.sessionByToken(second.Token).Revoked != nil {
		t.Fatal("второй сеанс ученика требует подтверждения или гасит первый")
	}
}

func assertTTL(t *testing.T, h *harness, token string, want time.Duration) {
	t.Helper()
	row := h.sessionByToken(token)
	delta := row.Expires.Sub(row.Created)
	if delta < want-ttlSlack || delta > want+ttlSlack {
		t.Fatalf("срок %s, нужен %s", delta, want)
	}
}

func (h *harness) shiftSession(token string, days int) {
	h.t.Helper()
	row := h.sessionByToken(token)
	h.exec(`
		UPDATE core.session
		SET created_at = now() - $1 * interval '1 day',
		    expires_at = now() - interval '1 second'
		WHERE token_hash = $2
	`, days, row.Hash)
}
