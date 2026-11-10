package homework_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/homework"
)

// homework-01 … homework-22: по тесту на строку homework.examples.yaml.

func TestHW_R1_homework01(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "planned",
		Blocks:  []string{blockOne},
		Mark:    &homework.Mark{TheorySet: true, Theory: blockOne},
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{variant("v-one", "hw-t-one", "sv-one", "M01", 1)},
	})

	rec := h.getDraft("hw-l1")
	h.expectCode(rec, http.StatusConflict, "lesson_not_marked")
	h.expectNoDraft("hw-l1")
}

func TestHW_R1_homework02(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:        "hw-l1",
		PlaceNo:   1,
		State:     "held",
		Cancelled: true,
		Blocks:    []string{blockOne},
		Recipe:    []homework.Series{{Source: "new_topic", TypeID: "hw-t-one", Count: 1}},
		Mark:      coverNone(),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{variant("v-one", "hw-t-one", "sv-one", "M01", 1)},
	})

	rec := h.getDraft("hw-l1")
	h.expectCode(rec, http.StatusConflict, "lesson_cancelled")
	h.expectNoDraft("hw-l1")
}

func TestHW_R4_homework03(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Tasks: []homework.Task{
			task(1, "r-used", "hw-t-r", "sv-r"),
			task(2, "u-1", "hw-t-u", "sv-u"),
		},
		Recipe: []homework.Series{
			{Source: "new_topic", TypeID: "hw-t-new", Count: 3},
			{Source: "review", TypeID: "hw-t-rev", Count: 1, Topic: "M05"},
		},
		Mark: coverSolved(1),
	}
	sibling := variant("r-1", "hw-t-r", "sv-r", "M02", 1)
	sibling.Statement = "n=20"
	used := variant("r-used", "hw-t-r", "sv-r", "M02", 1)
	used.Statement = "n=2"
	nA := variant("n-a", "hw-t-new", "sv-a", "M03", 2)
	nB := variant("n-b", "hw-t-new", "sv-b", "M03", 1)
	nC := variant("n-c", "hw-t-new", "sv-c", "M03", 1)
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants: []homework.Variant{
			used, sibling,
			variant("u-1", "hw-t-u", "sv-u", "M04", 1),
			nA, nB, nC,
			variant("rev-1", "hw-t-rev", "sv-rev", "M05", 1),
		},
	})

	body := h.mustDraft("hw-l1")
	assertSet(t, body.Items, []string{"r-1", "u-1", "n-a", "n-b", "n-c", "rev-1"})
	difficulty := map[string]int{"n-a": 2, "n-b": 1, "n-c": 1}
	assertGrouped(t, body.Items, difficulty)
	var newIDs []string
	for _, item := range body.Items {
		if item.Source == "new_topic" {
			newIDs = append(newIDs, item.VariantID)
		}
		if item.VariantID == "u-1" && item.Source != "unsolved" {
			t.Fatalf("недорешённое %s", item.Source)
		}
		if item.VariantID == "r-1" && item.Source != "reinforcement" {
			t.Fatalf("закрепление %s", item.Source)
		}
		if item.VariantID == "rev-1" && item.Source != "review" {
			t.Fatalf("повторение %s", item.Source)
		}
	}
	if !slices.Equal(newIDs, []string{"n-b", "n-c", "n-a"}) {
		t.Fatalf("новая тема %v", newIDs)
	}
	if body.Items[len(body.Items)-1].VariantID != "rev-1" {
		t.Fatalf("повторение не в конце: %v", itemIDs(body.Items))
	}
	head := itemIDs(body.Items[:2])
	slices.Sort(head)
	if !slices.Equal(head, []string{"r-1", "u-1"}) {
		t.Fatalf("начало списка %v", itemIDs(body.Items))
	}
}

func TestHW_R6_homework04(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Tasks: []homework.Task{
			task(1, "s1-a", "hw-t1", "sv-1"),
			task(2, "s2-a", "hw-t2", "sv-2"),
			task(3, "s3-a", "hw-t3", "sv-3"),
		},
		Mark: coverSolved(3),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants: []homework.Variant{
			stmt("s1-a", "hw-t1", "sv-1", "1+1"),
			stmt("s1-b", "hw-t1", "sv-1", "9+9"),
			stmt("s2-a", "hw-t2", "sv-2", "2+2"),
			stmt("s2-b", "hw-t2", "sv-2", "8+8"),
			stmt("s3-a", "hw-t3", "sv-3", "3+3"),
			stmt("s3-b", "hw-t3", "sv-3", "7+7"),
		},
	})

	body := h.mustDraft("hw-l1")
	if !sameIDs(body.Items, []string{"s1-b", "s2-b", "s3-b"}) {
		t.Fatalf("закрепление %v", itemIDs(body.Items))
	}
	want := map[string]string{"s1-b": "9+9", "s2-b": "8+8", "s3-b": "7+7"}
	for _, item := range body.Items {
		if item.Source != "reinforcement" {
			t.Fatalf("%s source %s", item.VariantID, item.Source)
		}
		if item.Statement != want[item.VariantID] {
			t.Fatalf("%s числа %q", item.VariantID, item.Statement)
		}
	}
}

func TestHW_R7_homework05(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{"hw-early", "hw-late"},
		Tasks: []homework.Task{
			{
				Position: 1, VariantID: "done-a", TypeID: "hw-t-done",
				Subvariant: "sv-done", TheoryBlock: "hw-early",
			},
			{
				Position: 2, VariantID: "late-a", TypeID: "hw-t-late",
				Subvariant: "sv-late", TheoryBlock: "hw-late",
			},
		},
		Mark: &homework.Mark{
			TheorySet: true, Theory: "hw-early",
			PracticeSet: true, PracticePosition: 1,
		},
	}
	done := variant("done-a", "hw-t-done", "sv-done", "M01", 1)
	done.TheoryBlock = "hw-early"
	done.Statement = "n=1"
	sibling := variant("done-b", "hw-t-done", "sv-done", "M01", 1)
	sibling.TheoryBlock = "hw-early"
	sibling.Statement = "n=11"
	late := variant("late-a", "hw-t-late", "sv-late", "M02", 1)
	late.TheoryBlock = "hw-late"
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{done, sibling, late},
	})

	body := h.mustDraft("hw-l1")
	if slices.Contains(itemIDs(body.Items), "late-a") {
		t.Fatalf("необъяснённое попало в ДЗ: %v", itemIDs(body.Items))
	}
	if !slices.Equal(sourcesOf(body.Items, "reinforcement"), []string{"done-b"}) {
		t.Fatalf("список %v", itemIDs(body.Items))
	}
	if warningMentions(body.Warnings, "hw-t-late") {
		t.Fatal("необъяснённая теория дала variants_needed")
	}
}

func TestHW_R8_homework06(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Tasks:   []homework.Task{task(1, "only-a", "hw-t-only", "sv-only")},
		Mark:    coverSolved(1),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{variant("only-a", "hw-t-only", "sv-only", "M01", 1)},
	})

	body := h.mustDraft("hw-l1")
	if len(body.Items) != 0 {
		t.Fatalf("закрепление не пропущено: %v", itemIDs(body.Items))
	}
	if !warningMentions(body.Warnings, "hw-t-only", "sv-only") {
		t.Fatalf("предупреждения %+v", body.Warnings)
	}
}

func TestHW_R9_homework07(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Recipe: []homework.Series{
			{Source: "review", TypeID: "hw-t-rev", Count: 2, Topic: "M05"},
			{Source: "review", TypeID: "hw-t-rev", Count: 2, Topic: "M10"},
		},
		Mark: coverNone(),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants: []homework.Variant{
			variant("a-m05", "hw-t-rev", "sv-a", "M05", 1),
			variant("b-m10", "hw-t-rev", "sv-b", "M10", 1),
			variant("c-m05", "hw-t-rev", "sv-c", "M05", 1),
			variant("d-m10", "hw-t-rev", "sv-d", "M10", 1),
		},
	})

	beforeKinds := h.journalKinds()
	body := h.mustDraft("hw-l1")
	assertSet(t, body.Items, []string{"a-m05", "b-m10", "c-m05", "d-m10"})
	for _, item := range body.Items {
		if item.Source != "review" {
			t.Fatalf("%s source %s", item.VariantID, item.Source)
		}
	}
	rec := h.patchDraft("hw-l1", map[string]any{"remove": []string{"a-m05"}})
	h.status(rec, http.StatusOK)
	patched := decode[draftView](t, rec)
	if slices.Contains(itemIDs(patched.Items), "a-m05") {
		t.Fatalf("вариант не убран: %v", itemIDs(patched.Items))
	}
	if !slices.Contains(itemIDs(patched.Items), "b-m10") || !slices.Contains(itemIDs(patched.Items), "c-m05") {
		t.Fatalf("после правки %v", itemIDs(patched.Items))
	}
	if slices.Contains(h.journalKinds(), "assignment_issued") || len(h.journalKinds()) < len(beforeKinds) {
		t.Fatalf("правка черновика изменила журнал %v", h.journalKinds())
	}
}

func TestHW_R10_homework08(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Recipe:  []homework.Series{{Source: "new_topic", TypeID: "hw-t-new", Count: 1}},
		Mark:    coverNone(),
	}
	reserve := variant("a-res", "hw-t-new", "sv-res", "M01", 1)
	reserve.CheckpointReserve = true
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{reserve, variant("b-ok", "hw-t-new", "sv-ok", "M01", 1)},
	})

	body := h.mustDraft("hw-l1")
	if slices.Contains(itemIDs(body.Items), "a-res") {
		t.Fatalf("резерв взят: %v", itemIDs(body.Items))
	}
	if !sameIDs(body.Items, []string{"b-ok"}) {
		t.Fatalf("список %v", itemIDs(body.Items))
	}
}

func TestHW_R11_homework09(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Recipe:  []homework.Series{{Source: "new_topic", TypeID: "hw-t-seen", Count: 1}},
		Mark:    coverNone(),
	}
	first := variant("seen-1", "hw-t-seen", "sv-1", "M01", 1)
	second := variant("seen-2", "hw-t-seen", "sv-2", "M01", 1)
	first.Seen = true
	second.Seen = true
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       []homework.Variant{first, second},
	})

	body := h.mustDraft("hw-l1")
	if len(body.Items) != 0 {
		t.Fatalf("виденный тип попал в ДЗ: %v", itemIDs(body.Items))
	}
	if !warningMentions(body.Warnings, "hw-t-seen") {
		t.Fatalf("предупреждения %+v", body.Warnings)
	}
}

func TestHW_R12_homework10(t *testing.T) {
	h := newHarness(t)
	full, total := countFull(thresholdFixture(6).Past, "M01")
	if full != 6 || total != 10 {
		t.Fatalf("доля собрана как %d/%d", full, total)
	}
	h.seed(thresholdFixture(6))
	body := h.mustDraft("hw-l1")
	if !slices.Equal(sourcesOf(body.Items, "threshold"), thresholdIDs(4)) {
		t.Fatalf("добавка при 6/10: %v", itemIDs(body.Items))
	}
	if slices.Contains(body.DiscussTopics, "M01") {
		t.Fatalf("пометка при 60 %%: %v", body.DiscussTopics)
	}
}

func TestHW_R13_homework11(t *testing.T) {
	h := newHarness(t)
	h.seed(thresholdFixture(7))
	body := h.mustDraft("hw-l1")
	if !slices.Equal(sourcesOf(body.Items, "threshold"), thresholdIDs(3)) {
		t.Fatalf("добавка при 70 %%: %v", itemIDs(body.Items))
	}
	if slices.Contains(body.DiscussTopics, "M01") {
		t.Fatalf("пометка при 70 %%: %v", body.DiscussTopics)
	}
}

func TestHW_R13_homework12(t *testing.T) {
	h := newHarness(t)
	h.seed(thresholdFixture(8))
	body := h.mustDraft("hw-l1")
	if len(sourcesOf(body.Items, "threshold")) != 0 {
		t.Fatalf("добавка при 80 %%: %v", itemIDs(body.Items))
	}
	if slices.Contains(itemIDs(body.Items), "th-01") {
		t.Fatalf("свежий вариант взят без порога: %v", itemIDs(body.Items))
	}
}

func TestHW_R14_homework13(t *testing.T) {
	h := newHarness(t)
	h.seed(thresholdFixture(5))
	body := h.mustDraft("hw-l1")
	if !slices.Equal(sourcesOf(body.Items, "threshold"), thresholdIDs(5)) {
		t.Fatalf("добавка при 50 %%: %v", itemIDs(body.Items))
	}
	if !slices.Contains(body.DiscussTopics, "M01") {
		t.Fatalf("нет пометки: %v", body.DiscussTopics)
	}
}

func TestHW_R14_homework14(t *testing.T) {
	h := newHarness(t)
	h.seed(thresholdFixture(6))
	body := h.mustDraft("hw-l1")
	if !slices.Equal(sourcesOf(body.Items, "threshold"), thresholdIDs(4)) {
		t.Fatalf("добавка при 60 %%: %v", itemIDs(body.Items))
	}
	if len(body.DiscussTopics) != 0 {
		t.Fatalf("пометка при 60 %%: %v", body.DiscussTopics)
	}
}

func TestHW_R15_homework15(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Recipe:  []homework.Series{{Source: "new_topic", TypeID: "hw-t-new", Count: 16}},
		Mark:    coverNone(),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       numbered("v", "hw-t-new", "M01", blockOne, 16, 5, 1),
	})

	body := h.mustDraft("hw-l1")
	if !sameIDs(body.Items, idsNumbered("v", 16)) {
		t.Fatalf("в ДЗ %d: %v", len(body.Items), itemIDs(body.Items))
	}
	for _, item := range body.Items {
		if item.Source != "new_topic" {
			t.Fatalf("%s source %s", item.VariantID, item.Source)
		}
	}
}

func TestHW_R15_homework16(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{"hw-ba", "hw-bb"},
		Recipe: []homework.Series{
			{Source: "new_topic", TypeID: "hw-t-a", Count: 2},
			{Source: "new_topic", TypeID: "hw-t-b", Count: 3},
		},
		Mark: &homework.Mark{TheorySet: true, Theory: "hw-ba", PracticeSet: true},
	}
	known := numbered("a", "hw-t-a", "M01", "hw-ba", 2, 5, 1)
	hidden := numbered("b", "hw-t-b", "M02", "hw-bb", 3, 5, 1)
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       append(known, hidden...),
	})

	body := h.mustDraft("hw-l1")
	if !sameIDs(body.Items, idsNumbered("a", 2)) {
		t.Fatalf("объяснённый тип: %v", itemIDs(body.Items))
	}
	for _, id := range itemIDs(body.Items) {
		if len(id) > 0 && id[0] == 'b' {
			t.Fatalf("необъяснённый тип добавлен: %v", itemIDs(body.Items))
		}
	}
}

func TestHW_R15a_homework17(t *testing.T) {
	h := newHarness(t)
	tasks := make([]homework.Task, 5)
	variants := make([]homework.Variant, 0, 10)
	for i := 1; i <= 5; i++ {
		used := stmt(idNum("used", i), "hw-t-sol", idNum("sv", i), "n=1")
		free := stmt(idNum("free", i), "hw-t-sol", idNum("sv", i), "n=2")
		tasks[i-1] = task(i, used.ID, used.TypeID, used.Subvariant)
		variants = append(variants, used, free)
	}
	variants = append(variants, numbered("n", "hw-t-new", "M09", blockOne, 11, 5, 1)...)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Tasks:   tasks,
		Recipe:  []homework.Series{{Source: "new_topic", TypeID: "hw-t-new", Count: 11}},
		Mark:    coverSolved(5),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       variants,
	})

	body := h.mustDraft("hw-l1")
	want := append(idsNumbered("free", 5), idsNumbered("n", 11)...)
	if !sameIDs(body.Items, want) {
		t.Fatalf("ДЗ изменилось: %v", itemIDs(body.Items))
	}
	if !hintsHave(body.Hints, 5, 16) {
		t.Fatalf("подсказка %v", body.Hints)
	}
}

func TestHW_R16_homework18(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Recipe:  []homework.Series{{Source: "new_topic", TypeID: "hw-t-long", Count: 9}},
		Mark:    coverNone(),
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       numbered("long", "hw-t-long", "M01", blockOne, 9, 60, 1),
	})

	body := h.mustDraft("hw-l1")
	if !sameIDs(body.Items, idsNumbered("long", 9)) {
		t.Fatalf("список урезан: %v", itemIDs(body.Items))
	}
	sum := 0
	for _, item := range body.Items {
		sum += item.Minutes
	}
	if sum != 9*60 {
		t.Fatalf("сумма минут %d", sum)
	}
}

func TestHW_R17_homework19(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("2026-11-02T18:00:00Z", "2026-11-09T18:00:00Z"))
	h.mustDraft("hw-l1")
	rec := h.issue("hw-l1")
	h.status(rec, http.StatusCreated)
	issued := decode[issuedView](t, rec)
	if issued.ID == "" || issued.Kind != "homework" || issued.State != "issued" || !issued.CanWithdraw || issued.ItemCount != 1 {
		t.Fatalf("выдача %+v", issued)
	}
	if !slices.Contains(h.journalKinds(), "assignment_issued") {
		t.Fatalf("журнал %v", h.journalKinds())
	}
	gone := h.withdraw(issued.ID)
	h.status(gone, http.StatusNoContent)
	if h.assignmentState(issued.ID) != "withdrawn" {
		t.Fatalf("состояние %s", h.assignmentState(issued.ID))
	}
}

func TestHW_R17_homework20(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("2026-11-02T18:00:00Z", "2026-11-09T18:00:00Z"))
	h.mustDraft("hw-l1")
	rec := h.issue("hw-l1")
	h.status(rec, http.StatusCreated)
	issued := decode[issuedView](t, rec)
	h.insertAnswer(issued.ID, h.firstItem(issued.ID))
	denied := h.withdraw(issued.ID)
	h.expectCode(denied, http.StatusConflict, "has_answers")
	if h.assignmentState(issued.ID) != "issued" {
		t.Fatalf("состояние %s", h.assignmentState(issued.ID))
	}
}

func TestHW_R18_homework21(t *testing.T) {
	h := newHarness(t)
	current := homework.Lesson{
		ID:       "hw-l1",
		PlaceNo:  1,
		State:    "held",
		StartsAt: "2026-11-02T18:00:00Z",
		Blocks:   []string{blockOne},
		Recipe:   []homework.Series{{Source: "new_topic", TypeID: "hw-t-one", Count: 1}},
		Mark:     coverNone(),
	}
	skipped := homework.Lesson{
		ID: "hw-l2", PlaceNo: 2, State: "skipped", StartsAt: "2026-11-05T18:00:00Z",
	}
	next := homework.Lesson{
		ID: "hw-l3", PlaceNo: 3, State: "planned", StartsAt: "2026-11-09T18:00:00Z",
	}
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{current, skipped, next},
		Variants:       []homework.Variant{variant("v-one", "hw-t-one", "sv-one", "M01", 1)},
	})

	body := h.mustDraft("hw-l1")
	sameInstant(t, body.Deadline, "2026-11-09T18:00:00Z")
}

func TestHW_R20_homework22(t *testing.T) {
	h := newHarness(t)
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Tasks:   []homework.Task{task(1, "used-a", "hw-t-r", "sv-r")},
		Recipe: []homework.Series{
			{Source: "new_topic", TypeID: "hw-t-new", Count: 2},
			{Source: "review", TypeID: "hw-t-rev", Count: 1, Topic: "M05"},
		},
		Mark: coverSolved(1),
	}
	used := variant("used-a", "hw-t-r", "sv-r", "M01", 1)
	free := variant("free-b", "hw-t-r", "sv-r", "M01", 1)
	free.Statement = "n=99"
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants: []homework.Variant{
			used, free,
			variant("n-1", "hw-t-new", "sv-n1", "M03", 2),
			variant("n-2", "hw-t-new", "sv-n2", "M03", 1),
			variant("rev-1", "hw-t-rev", "sv-rev", "M05", 1),
		},
	})

	first := h.mustDraft("hw-l1")
	h.dropDraft("hw-l1")
	second := h.mustDraft("hw-l1")
	if !sameDraft(first, second) {
		t.Fatalf("черновики разошлись: %+v и %+v", first, second)
	}
	if len(first.Items) == 0 {
		t.Fatal("пустой черновик")
	}
}

func stmt(id, typeID, sub, statement string) homework.Variant {
	item := variant(id, typeID, sub, "M01", 1)
	item.Statement = statement
	return item
}

func idNum(prefix string, n int) string {
	return fmt.Sprintf("%s-%02d", prefix, n)
}

func sameDraft(a, b draftView) bool {
	if a.LessonID != b.LessonID || a.Deadline != b.Deadline || a.Issued != b.Issued {
		return false
	}
	if !slices.Equal(a.Hints, b.Hints) || !slices.Equal(a.DiscussTopics, b.DiscussTopics) {
		return false
	}
	if len(a.Items) != len(b.Items) || len(a.Warnings) != len(b.Warnings) {
		return false
	}
	for i := range a.Items {
		if a.Items[i] != b.Items[i] {
			return false
		}
	}
	for i := range a.Warnings {
		if a.Warnings[i].Type != b.Warnings[i].Type || a.Warnings[i].Text != b.Warnings[i].Text {
			return false
		}
	}
	return true
}
