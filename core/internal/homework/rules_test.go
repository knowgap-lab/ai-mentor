package homework_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/homework"
)

func TestHW_ForeignStudent(t *testing.T) {
	h := newHarness(t)
	fx := oneItemFixture("", "")
	fx.OtherStudent = true
	h.seed(fx)

	rec := h.getDraft(h.world.OtherLessonID)
	h.expectCode(rec, http.StatusForbidden, "forbidden")
	h.expectNoDraft(h.world.OtherLessonID)
}

func TestHW_R2_OneList(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("", ""))
	first := h.mustDraft("hw-l1")
	second := h.mustDraft("hw-l1")
	if h.draftRows() != 1 {
		t.Fatalf("строк черновика %d", h.draftRows())
	}
	if !sameIDs(first.Items, []string{"v-one"}) || !sameIDs(second.Items, []string{"v-one"}) {
		t.Fatalf("списки %v и %v", itemIDs(first.Items), itemIDs(second.Items))
	}
}

func TestHW_R3_Sources(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Tasks: []homework.Task{
			task(1, "rein-used", "hw-t-rein", "sv-rein"),
			task(2, "uns-1", "hw-t-uns", "sv-uns"),
		},
		Recipe: []homework.Series{
			{Source: "new_topic", TypeID: "hw-t-new", Count: 1},
			{Source: "review", TypeID: "hw-t-rev", Count: 1, Topic: "M05"},
			{Source: "ct_format", TypeID: "hw-t-ct", Count: 1},
		},
		Mark: coverSolved(1),
	}
	pastVar := variant("ph-01", "hw-t-past", "sv-past", "M01", 1)
	pastVar.Seen = true
	fresh := variant("th-01", "hw-t-add", "sv-add", "M01", 1)
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants: []homework.Variant{
			variant("rein-used", "hw-t-rein", "sv-rein", "M02", 1),
			variant("rein-free", "hw-t-rein", "sv-rein", "M02", 1),
			variant("uns-1", "hw-t-uns", "sv-uns", "M04", 1),
			variant("n-1", "hw-t-new", "sv-new", "M03", 1),
			variant("rev-1", "hw-t-rev", "sv-rev", "M05", 1),
			variant("ct-1", "hw-t-ct", "sv-ct", "M06", 1),
			pastVar, fresh,
		},
		Past: []homework.PastItem{
			{VariantID: "ph-01", Topic: "M01", MaxPoints: 2, FirstPoints: 0},
		},
	})

	body := h.mustDraft("hw-l1")
	assertSet(t, body.Items, []string{"rein-free", "uns-1", "n-1", "th-01", "rev-1", "ct-1"})
	assertGrouped(t, body.Items, map[string]int{"n-1": 1})
	got := map[string]string{}
	for _, item := range body.Items {
		got[item.VariantID] = item.Source
	}
	want := map[string]string{
		"rein-free": "reinforcement",
		"uns-1":     "unsolved",
		"n-1":       "new_topic",
		"th-01":     "threshold",
		"rev-1":     "review",
		"ct-1":      "ct_format",
	}
	for id, source := range want {
		if got[id] != source {
			t.Fatalf("%s source %s, нужен %s", id, got[id], source)
		}
	}
}

func TestHW_R5_NoTimer(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("", ""))
	rec := h.getDraft("hw-l1")
	h.status(rec, http.StatusOK)
	if keys := forbiddenKeys(t, rec.Body.Bytes()); len(keys) != 0 {
		t.Fatalf("в черновике есть %v", keys)
	}
}

func TestHW_R11_VariantSeen(t *testing.T) {
	h := newHarness(t)
	fx := oneItemFixture("", "")
	seen := variant("v-seen", "hw-t-seen", "sv-seen", "M01", 1)
	seen.Seen = true
	fx.Variants = append(fx.Variants, seen)
	h.seed(fx)

	before := h.mustDraft("hw-l1")
	rec := h.patchDraft("hw-l1", map[string]any{"add": []string{"v-seen"}})
	h.expectCode(rec, http.StatusConflict, "variant_seen")
	after := h.mustDraft("hw-l1")
	if !sameIDs(after.Items, itemIDs(before.Items)) {
		t.Fatalf("список изменился: %v", itemIDs(after.Items))
	}
}

func TestHW_R17_AlreadyIssued(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("2026-11-02T18:00:00Z", "2026-11-09T18:00:00Z"))
	h.mustDraft("hw-l1")
	rec := h.issue("hw-l1")
	h.status(rec, http.StatusCreated)
	issued := decode[issuedView](t, rec)
	if issued.State != "issued" {
		t.Fatalf("состояние %s", issued.State)
	}
	denied := h.patchDraft("hw-l1", map[string]any{"remove": []string{"v-one"}})
	h.expectCode(denied, http.StatusConflict, "already_issued")
	got := h.getDraft("hw-l1")
	h.status(got, http.StatusOK)
	body := decode[draftView](t, got)
	if !body.Issued {
		t.Fatal("issued не отмечен")
	}
	if !slices.Contains(itemIDs(body.Items), "v-one") {
		t.Fatalf("список после отказа %v", itemIDs(body.Items))
	}
}

func TestHW_R18_DeadlineRequired(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("", ""))
	body := h.mustDraft("hw-l1")
	if body.Deadline != "" {
		t.Fatalf("срок без даты %s", body.Deadline)
	}
	rec := h.issue("hw-l1")
	h.expectCode(rec, http.StatusUnprocessableEntity, "deadline_required")
	if h.assignmentCount() != 0 {
		t.Fatal("выдача без срока записана")
	}
}

func TestHW_R19_Manual(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:       "hw-l1",
		PlaceNo:  1,
		State:    "held",
		NoBlocks: true,
		Coverage: "all",
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{variant("v-man", "hw-t-man", "sv-man", "M01", 1)},
	})

	body := h.mustDraft("hw-l1")
	if len(body.Items) != 0 {
		t.Fatalf("автосборка без блоков: %v", itemIDs(body.Items))
	}
	rec := h.patchDraft("hw-l1", map[string]any{"add": []string{"v-man"}})
	h.status(rec, http.StatusOK)
	patched := decode[draftView](t, rec)
	if !slices.Contains(itemIDs(patched.Items), "v-man") {
		t.Fatalf("ручной вариант не добавлен: %v", itemIDs(patched.Items))
	}
}

func forbiddenKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	ban := map[string]struct{}{
		"timer": {}, "time_limit": {}, "conspect": {}, "without_conspect": {},
	}
	var found []string
	var walk func(any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if _, ok := ban[key]; ok {
					found = append(found, key)
				}
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(body)
	return found
}
