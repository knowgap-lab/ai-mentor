package privacy_test

import (
	"net/http"
	"testing"
	"time"
)

func TestPV_R10_LinkOnDay15Returns410(t *testing.T) {
	h := newHarness(t)
	clock := time.Now().Add(-15 * day)
	token := h.createLink(kindParent, clock)
	before := h.snapshot()

	h.expectGone(h.call(http.MethodGet, token, ""), token)
	h.unchanged(token, before)

	h.expectGone(h.call(http.MethodPost, token, actionGive), token)
	h.unchanged(token, before)
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1`, studentID) != 0 {
		t.Fatal("просроченная ссылка записала согласие")
	}
}

func TestPV_R11_PreviewGetLeavesRowsUnchanged(t *testing.T) {
	h := newHarness(t)
	token := h.createLink(kindParent, time.Now())
	before := h.snapshot()

	page := h.expectPage(h.call(http.MethodGet, token, ""), token, kindParent, apiNone)
	if page.AnonymiseAt != nil {
		t.Fatal("предпросмотр заполнил anonymise_at")
	}
	h.unchanged(token, before)

	h.expectPage(h.call(http.MethodGet, token, ""), token, kindParent, apiNone)
	h.unchanged(token, before)
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1`, studentID) != 0 {
		t.Fatal("предпросмотр создал согласие")
	}
}

func TestPV_R12_RejectsStudentLinkUnder18AndParentLinkAt18(t *testing.T) {
	h := newHarness(t)
	t.Run("age16_student", func(t *testing.T) {
		prev := h.t
		h.t = t
		t.Cleanup(func() { h.t = prev })
		h.refuseLink(kindStudent, h.at16)
	})
	t.Run("age18_parent", func(t *testing.T) {
		prev := h.t
		h.t = t
		t.Cleanup(func() { h.t = prev })
		h.refuseLink(kindParent, h.at18)
	})
}

func TestPV_R17_RevokeSetsRevokedAtAndAnonymiseIn7Days(t *testing.T) {
	h := newHarness(t)
	token := h.grantParent()
	h.revoke(token)
}

func TestPV_R18_CancelRevokeOnDay3RestoresActive(t *testing.T) {
	h := newHarness(t)
	token := h.grantParent()
	h.revoke(token)
	h.backdateRevoke()

	page := h.expectPage(h.call(http.MethodPost, token, actionCancelRevoke), token, kindParent, apiGiven)
	if page.AnonymiseAt != nil {
		t.Fatal("отмена отзыва оставила anonymise_at")
	}
	h.expectActiveCleared()
}
