package auth_test

import (
	"bytes"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
)

type roster struct {
	h                  *harness
	tutorA, tutorB     loggedIn
	studentA, studentB loggedIn
	unowned            loggedIn
}

func newRoster(t *testing.T) roster {
	t.Helper()
	h := newHarness(t, "local", true)
	r := roster{
		h:        h,
		tutorA:   h.loginOK(demoID(1), "tutor"),
		tutorB:   h.loginOK(demoID(2), "tutor"),
		studentA: h.loginOK(demoID(3), "student"),
		studentB: h.loginOK(demoID(4), "student"),
		unowned:  h.loginOK(demoID(5), "student"),
	}
	h.exec(`UPDATE core.app_user SET owner_tutor_id = $1 WHERE id = $2`, r.tutorA.Me.UserID, r.studentA.Me.UserID)
	h.exec(`UPDATE core.app_user SET owner_tutor_id = $1 WHERE id = $2`, r.tutorB.Me.UserID, r.studentB.Me.UserID)
	_, owner, _ := h.userRow(r.studentA.Me.UserID)
	if owner == nil || *owner != r.tutorA.Me.UserID {
		t.Fatal("ученику не записан репетитор-владелец")
	}
	return r
}

func (h *harness) authorize(role, actorID, objectID, action string) error {
	h.t.Helper()
	return h.svc.Authorize(
		auth.Actor{UserID: actorID, Role: role},
		action,
		auth.Object{UserID: objectID},
	)
}

func TestAU_R3_AccessDependsOnlyOnRoleActorAndOwner(t *testing.T) {
	r := newRoster(t)
	h := r.h

	sameDecision(t,
		h.authorize("tutor", r.tutorA.Me.UserID, r.studentA.Me.UserID, "read"),
		h.authorize("tutor", r.tutorA.Me.UserID, r.studentA.Me.UserID, "write"),
	)
	if err := h.authorize("tutor", r.tutorA.Me.UserID, r.studentA.Me.UserID, "read"); err != nil {
		t.Fatalf("репетитор не видит своего ученика: %v", err)
	}
	if err := h.authorize("tutor", r.tutorB.Me.UserID, r.studentB.Me.UserID, "read"); err != nil {
		t.Fatalf("второй репетитор не видит своего ученика: %v", err)
	}

	sameDecision(t,
		h.authorize("tutor", r.tutorA.Me.UserID, r.studentB.Me.UserID, "read"),
		h.authorize("tutor", r.tutorA.Me.UserID, r.studentB.Me.UserID, "write"),
	)
	requireForbidden(t, h.authorize("tutor", r.tutorA.Me.UserID, r.studentB.Me.UserID, "read"))

	sameDecision(t,
		h.authorize("tutor", r.tutorA.Me.UserID, r.unowned.Me.UserID, "read"),
		h.authorize("tutor", r.tutorA.Me.UserID, r.unowned.Me.UserID, "write"),
	)
	requireForbidden(t, h.authorize("tutor", r.tutorA.Me.UserID, r.unowned.Me.UserID, "read"))

	sameDecision(t,
		h.authorize("student", r.studentA.Me.UserID, r.studentA.Me.UserID, "read"),
		h.authorize("student", r.studentA.Me.UserID, r.studentA.Me.UserID, "write"),
	)
	if err := h.authorize("student", r.studentA.Me.UserID, r.studentA.Me.UserID, "read"); err != nil {
		t.Fatalf("ученик не видит себя: %v", err)
	}

	sameDecision(t,
		h.authorize("student", r.studentA.Me.UserID, r.studentB.Me.UserID, "read"),
		h.authorize("student", r.studentA.Me.UserID, r.studentB.Me.UserID, "write"),
	)
	requireForbidden(t, h.authorize("student", r.studentA.Me.UserID, r.studentB.Me.UserID, "read"))
}

func TestAU_R4_ForeignAndMissingStudentSameForbidden(t *testing.T) {
	r := newRoster(t)
	h := r.h
	missing := "tg-demo-missing"

	tutorForeign := requireForbidden(t, h.authorize("tutor", r.tutorA.Me.UserID, r.studentB.Me.UserID, "read"))
	tutorMissing := requireForbidden(t, h.authorize("tutor", r.tutorA.Me.UserID, missing, "read"))
	sameBody(t, tutorForeign, tutorMissing)
	bodyHides(t, tutorForeign.Body(), r.studentB.Me.UserID, missing)
	bodyHides(t, tutorMissing.Body(), r.studentB.Me.UserID, missing)

	studentForeign := requireForbidden(t, h.authorize("student", r.studentA.Me.UserID, r.studentB.Me.UserID, "read"))
	studentMissing := requireForbidden(t, h.authorize("student", r.studentA.Me.UserID, missing, "read"))
	sameBody(t, studentForeign, studentMissing)
	bodyHides(t, studentForeign.Body(), r.studentB.Me.UserID, missing)
	bodyHides(t, studentMissing.Body(), r.studentB.Me.UserID, missing)
}

func TestAU_R6_IdempotencyKeyDoesNotCrossUsers(t *testing.T) {
	h := newHarness(t, "local", true)
	userA := h.loginOK(demoID(1), "student").Me.UserID
	userB := h.loginOK(demoID(2), "student").Me.UserID
	respA := []byte(`{"n":1}`)
	respB := []byte(`{"n":2}`)
	respC := []byte(`{"n":3}`)
	respD := []byte(`{"n":4}`)
	respE := []byte(`{"n":5}`)
	key := "idem-demo-1"
	otherKey := "idem-demo-2"

	if _, err := h.svc.Remember(userA, key, respA); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.Remember(userA, key, respB)
	if err != nil || !bytes.Equal(got, respA) {
		t.Fatalf("повтор тому же пользователю вернул %q, ошибка %v", got, err)
	}
	got, err = h.svc.Remember(userA, key, respB)
	if err != nil || !bytes.Equal(got, respA) {
		t.Fatalf("ещё один повтор вернул %q, ошибка %v", got, err)
	}

	firstOther, err := h.svc.Remember(userB, key, respC)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstOther, respA) {
		t.Fatal("чужой ключ вернул чужой ответ")
	}
	got, err = h.svc.Remember(userB, key, respD)
	if err != nil || !bytes.Equal(got, respC) || bytes.Equal(got, respA) {
		t.Fatalf("ответ другого пользователя = %q, ошибка %v", got, err)
	}
	got, err = h.svc.Remember(userA, key, respD)
	if err != nil || !bytes.Equal(got, respA) {
		t.Fatal("ответ первого пользователя подменился чужим ключом")
	}

	if _, err := h.svc.Remember(userA, otherKey, respD); err != nil {
		t.Fatal(err)
	}
	got, err = h.svc.Remember(userA, otherKey, respE)
	if err != nil || !bytes.Equal(got, respD) {
		t.Fatalf("другой ключ вернул %q, ошибка %v", got, err)
	}
	got, err = h.svc.Remember(userA, key, respE)
	if err != nil || !bytes.Equal(got, respA) {
		t.Fatal("ключи одного пользователя смешались")
	}
}
