package auth_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

func TestAU_R1_DevLoginReturnsStudentOrTutor(t *testing.T) {
	h := newHarness(t, "local", true)
	student := h.loginOK(demoID(1), "student")
	tutor := h.loginOK(demoID(2), "tutor")
	if student.Me.Role != "student" || tutor.Me.Role != "tutor" {
		t.Fatalf("роли %+v %+v", student.Me, tutor.Me)
	}
	if student.Me.UserID == tutor.Me.UserID {
		t.Fatal("разные пары telegram_id и role получили один user_id")
	}
	role, _, _ := h.userRow(student.Me.UserID)
	if role != "student" {
		t.Fatalf("role в базе = %s", role)
	}
	role, _, _ = h.userRow(tutor.Me.UserID)
	if role != "tutor" {
		t.Fatalf("role в базе = %s", role)
	}
}

func TestAU_R2_NewStudentAndTutorHaveEmptyOwner(t *testing.T) {
	h := newHarness(t, "local", true)
	student := h.loginOK(demoID(1), "student")
	tutor := h.loginOK(demoID(2), "tutor")
	for _, id := range []string{student.Me.UserID, tutor.Me.UserID} {
		_, owner, _ := h.userRow(id)
		if !ownerEmpty(owner) {
			t.Fatalf("owner_tutor_id пользователя %s = %q", id, *owner)
		}
	}
}

func TestAU_R36_RejectDevMode(t *testing.T) {
	if err := auth.RejectDevMode("prod", true); err == nil {
		t.Fatal("prod и включённый вход не остановили старт")
	}
	if err := auth.RejectDevMode("staging", true); err == nil {
		t.Fatal("staging и включённый вход не остановили старт")
	}
	if err := auth.RejectDevMode("Local", true); err == nil {
		t.Fatal("окружение Local не равно local")
	}
	if err := auth.RejectDevMode("", true); err == nil {
		t.Fatal("пустое окружение при включённом входе не остановило старт")
	}
	if err := auth.RejectDevMode("local", true); err != nil {
		t.Fatalf("local и включённый вход: %v", err)
	}
	if err := auth.RejectDevMode("local", false); err != nil {
		t.Fatalf("выключенный вход в local: %v", err)
	}
	if err := auth.RejectDevMode("prod", false); err != nil {
		t.Fatalf("выключенный вход в prod: %v", err)
	}
}

func TestAU_R36_DevLoginForbiddenUnlessLocalAndEnabled(t *testing.T) {
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	cases := []struct {
		name string
		env  string
		dev  bool
	}{
		{name: "local-off", env: "local", dev: false},
		{name: "prod-on", env: "prod", dev: true},
		{name: "staging-on", env: "staging", dev: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := openHarness(t, dsn, tc.env, tc.dev)
			rec := h.call(http.MethodPost, pathDev, devJSON(t, demoID(1), "student"), ptr(appOrigin), nil, nil)
			expectAPI(t, rec, http.StatusForbidden, "forbidden")
			if hasSessionCookie(rec) {
				t.Fatal("отказ всё равно выставил cookie сеанса")
			}
		})
	}
	ok := openHarness(t, dsn, "local", true)
	ok.loginOK(demoID(1), "student")
}

func TestAU_R36_RepeatLoginKeepsOneUser(t *testing.T) {
	h := newHarness(t, "local", true)
	first := h.loginOK(demoID(1), "student")
	second := h.loginOK(demoID(1), "student")
	if first.Me.UserID != second.Me.UserID {
		t.Fatalf("повторный вход сменил user_id: %s и %s", first.Me.UserID, second.Me.UserID)
	}
	if first.Token == second.Token {
		t.Fatal("повторный вход вернул тот же токен сеанса")
	}
	if h.count(`SELECT count(*) FROM core.app_user`) != 1 {
		t.Fatal("повторный вход создал второго пользователя")
	}
	if h.count(`SELECT count(*) FROM core.user_pii WHERE telegram_id = $1`, demoID(1)) != 1 {
		t.Fatal("telegram_id записан не одной строкой")
	}
	var linked string
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, `SELECT user_id FROM core.user_pii WHERE telegram_id = $1`, demoID(1)).Scan(&linked)
	})
	if linked != first.Me.UserID {
		t.Fatalf("user_pii.user_id = %s, Me = %s", linked, first.Me.UserID)
	}
	other := h.loginOK(demoID(2), "student")
	if other.Me.UserID == first.Me.UserID {
		t.Fatal("другой telegram_id получил тот же user_id")
	}
	if h.count(`SELECT count(*) FROM core.app_user`) != 2 {
		t.Fatal("второй ученик не создал свою учётную запись")
	}
}

func TestAU_R37_LogsOmitTokenCookieAndTelegramID(t *testing.T) {
	h := newHarness(t, "local", true)
	h.logs.Reset()
	in := h.loginOK(demoID(1), "student")
	origin := appOrigin
	rec := h.call(http.MethodPost, pathLogout, nil, &origin, &in.Token, sessionCookie(in.Token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	text := h.logs.String()
	if strings.Contains(text, demoID(1)) || strings.Contains(text, in.Token) {
		redacted := strings.ReplaceAll(text, in.Token, "[token]")
		redacted = strings.ReplaceAll(redacted, demoID(1), "[telegram_id]")
		t.Fatalf("журнал содержит данные входа или токен:\n%s", redacted)
	}
}

func TestUserPII_ReaderCannotSelect(t *testing.T) {
	h := newHarness(t, "local", true)
	in := h.loginOK(demoID(1), "student")
	var stored string
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx,
			`SELECT telegram_id FROM core.user_pii WHERE user_id = $1`, in.Me.UserID,
		).Scan(&stored)
	})
	if stored != demoID(1) {
		t.Fatalf("telegram_id в user_pii = %s", stored)
	}
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `SELECT set_config('role', 'reader', true)`); err != nil {
			return err
		}
		var leaked string
		err = tx.QueryRow(ctx, `SELECT telegram_id FROM core.user_pii WHERE user_id = $1`, in.Me.UserID).Scan(&leaked)
		if err == nil {
			t.Fatal("роль reader прочитала core.user_pii")
		}
		if !isPermission(err) {
			t.Fatalf("отказ reader: %v", err)
		}
		return nil
	})
}
