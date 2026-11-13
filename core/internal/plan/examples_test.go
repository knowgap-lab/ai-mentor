package plan_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/plan"
)

// plan-01 … plan-12: по тесту на строку plan.examples.yaml.
// Чужой ученик и чужой урок — 403 forbidden.

func TestPL_R1_Plan0Immutable(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, farExam(), numbered(2))
	fx.SourceHashes = map[string]string{
		"plan.md":    "hash-plan",
		"lessons.md": "hash-lessons",
	}
	h.seed(fx)

	before := h.snapshot()
	if !maps.Equal(before.SourceHashes, fx.SourceHashes) {
		t.Fatalf("хеши снимка %v", before.SourceHashes)
	}
	if !slices.Equal(before.LessonIDs, []string{"demo-l01", "demo-l02"}) {
		t.Fatalf("уроки снимка %v", before.LessonIDs)
	}
	viewBefore := h.plan()
	assertOrder(t, viewBefore, 2)

	err := plan.ReplacePlan0(t.Context(), h.conn, h.world.StudentID, h.world.Subject, map[string]string{
		"plan.md": "hash-replaced",
	})
	if err == nil {
		t.Fatal("снимок плана 0 изменился")
	}
	sameSnapshot(t, h.snapshot(), before)
	viewAfter := h.plan()
	assertOrder(t, viewAfter, 2)
	if byPlace(t, viewAfter, 1).ID != byPlace(t, viewBefore, 1).ID || byPlace(t, viewAfter, 2).ID != byPlace(t, viewBefore, 2).ID {
		t.Fatalf("текущий план изменился: %+v", viewAfter.Lessons)
	}
}

func TestPL_R2_MoveKeepsPlan0(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(mathFixture(now, farExam(), numbered(9)))

	beforeSnap := h.snapshot()
	before := h.plan()
	assertOrder(t, before, 9)
	lesson9 := byPlace(t, before, 9)
	if lesson9.ID != "demo-l09" || lesson9.Moved {
		t.Fatalf("урок 9 до переноса %+v", lesson9)
	}
	target := "2026-12-01T18:00:00Z"
	rec := h.moveLesson(lesson9.ID, target)
	h.status(rec, http.StatusOK)

	sameSnapshot(t, h.snapshot(), beforeSnap)
	after := h.plan()
	assertOrder(t, after, 9)
	moved := byPlace(t, after, 9)
	if moved.ID != lesson9.ID || moved.PlaceNo != 9 || !moved.Moved {
		t.Fatalf("текущий урок 9 %+v", moved)
	}
	sameInstant(t, moved.StartsAt, target)
	for place := 1; place <= 8; place++ {
		was := byPlace(t, before, place)
		got := byPlace(t, after, place)
		if got.ID != was.ID || got.PlaceNo != was.PlaceNo || got.Moved {
			t.Fatalf("урок %d изменился сверх переноса девятого: %+v", place, got)
		}
	}
	payload := decodePayload[movedPayload](t, h.lastRaw("lesson_moved"))
	if payload.LessonID != lesson9.ID {
		t.Fatalf("lesson_id %s", payload.LessonID)
	}
	sameInstant(t, payload.To, target)
}

func TestPL_R3_TemplateIDStable(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(9)
	specs[8].ID = "ct-math:L09"
	specs[8].TemplateID = "ct-math:L09"
	h.seed(mathFixture(now, farExam(), specs))

	before := h.plan()
	assertOrder(t, before, 9)
	if byPlace(t, before, 1).State != "frozen" || byPlace(t, before, 2).State != "frozen" {
		t.Fatalf("ближайшие не заморожены: %s %s", byPlace(t, before, 1).State, byPlace(t, before, 2).State)
	}
	templates := templatesOf(before)
	order := []string{
		"demo-l01", "demo-l02", "ct-math:L09",
		"demo-l03", "demo-l04", "demo-l05", "demo-l06", "demo-l07", "demo-l08",
	}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	view := decode[planView](t, rec)
	assertOrder(t, view, 9)
	third := byPlace(t, view, 3)
	if third.ID != "ct-math:L09" || third.TemplateID != "ct-math:L09" {
		t.Fatalf("третье место %+v", third)
	}
	if !maps.Equal(templatesOf(view), templates) {
		t.Fatalf("шаблоны %v, были %v", templatesOf(view), templates)
	}
	event := decodePayload[reorderedPayload](t, h.lastRaw("plan_reordered"))
	if event.Subject != subjectMath || !slices.Equal(event.LessonIDs, order) {
		t.Fatalf("plan_reordered %+v", event)
	}
}

func TestPL_R4_FrozenReorder(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(mathFixture(now, farExam(), numbered(4)))

	before := h.plan()
	assertOrder(t, before, 4)
	if byPlace(t, before, 1).State != "frozen" {
		t.Fatalf("урок 1: %s", byPlace(t, before, 1).State)
	}
	events := h.eventCount("plan_reordered")
	rec := h.reorder(h.world.StudentID, []string{"demo-l03", "demo-l02", "demo-l01", "demo-l04"})
	h.expectCode(rec, http.StatusConflict, "lesson_frozen")
	after := h.plan()
	assertOrder(t, after, 4)
	for place := 1; place <= 4; place++ {
		if byPlace(t, after, place).ID != byPlace(t, before, place).ID {
			t.Fatalf("место %d сменило урок", place)
		}
	}
	if h.eventCount("plan_reordered") != events {
		t.Fatal("отказ записан как plan_reordered")
	}
}

func TestPL_R4_OpenReorder(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(mathFixture(now, farExam(), numbered(4)))

	before := h.plan()
	assertOrder(t, before, 4)
	for _, place := range []int{3, 4} {
		state := byPlace(t, before, place).State
		if state == "frozen" || state == "held" {
			t.Fatalf("урок %d закрыт состоянием %s", place, state)
		}
	}
	order := []string{"demo-l01", "demo-l02", "demo-l04", "demo-l03"}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	view := decode[planView](t, rec)
	assertOrder(t, view, 4)
	if byPlace(t, view, 3).ID != "demo-l04" || byPlace(t, view, 4).ID != "demo-l03" {
		t.Fatalf("порядок %+v", view.Lessons)
	}
	if byPlace(t, view, 3).TemplateID != "demo:L04" || byPlace(t, view, 4).TemplateID != "demo:L03" {
		t.Fatalf("шаблоны сменились %+v", view.Lessons)
	}
	event := decodePayload[reorderedPayload](t, h.lastRaw("plan_reordered"))
	if event.Subject != subjectMath || !slices.Equal(event.LessonIDs, order) {
		t.Fatalf("plan_reordered %+v", event)
	}
}

func TestPL_R4a_SkippedShifts(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(14)
	specs[11].State = "skipped"
	fx := mathFixture(now, farExam(), specs)
	fx.Schedule = anchorSchedule()
	h.seed(fx)

	view := h.plan()
	assertOrder(t, view, 14)
	skipped := byPlace(t, view, 12)
	if skipped.ID != "demo-l12" || skipped.State != "skipped" {
		t.Fatalf("урок 12 %+v", skipped)
	}
	if skipped.StartsAt != "" {
		t.Fatalf("пропущенный занял слот %s", skipped.StartsAt)
	}
	sameInstant(t, byPlace(t, view, 11).StartsAt, "2026-12-07T18:00:00Z")
	sameInstant(t, byPlace(t, view, 13).StartsAt, "2026-12-10T18:00:00Z")
	sameInstant(t, byPlace(t, view, 14).StartsAt, "2026-12-14T18:00:00Z")
	h.expectNoDraft("demo-l12")
}

func TestPL_R5_PrereqOrder(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(4)
	specs[2].Topics = []string{"M13"}
	specs[3].Topics = []string{"M14"}
	fx := mathFixture(now, farExam(), specs)
	fx.Prereqs = map[string][]string{"M14": {"M13"}}
	h.seed(fx)

	before := h.plan()
	if byPlace(t, before, 3).State == "frozen" || byPlace(t, before, 4).State == "frozen" {
		t.Fatal("уроки тем M13 и M14 заморожены")
	}
	order := []string{"demo-l01", "demo-l02", "demo-l04", "demo-l03"}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	view := decode[planView](t, rec)
	assertOrder(t, view, 4)
	if byPlace(t, view, 3).ID != "demo-l04" || byPlace(t, view, 4).ID != "demo-l03" {
		t.Fatalf("порядок %+v", view.Lessons)
	}
	var found []warning
	for _, item := range view.Warnings {
		if item.Type == "prereq_order" && mentions(item, "M13") && mentions(item, "M14") {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		t.Fatalf("предупреждения %+v", view.Warnings)
	}
}

func TestPL_R7_ShiftWeeks(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := []plan.LessonSpec{
		{ID: "demo-l01", PlaceNo: 1, Title: "Урок 1", Kind: "new", TemplateID: "demo:L01", PlannedWeek: "2026-10-12"},
		{ID: "demo-l02", PlaceNo: 2, Title: "Урок 2", Kind: "new", TemplateID: "demo:L02", PlannedWeek: "2026-10-19"},
		{ID: "demo-l03", PlaceNo: 3, Title: "Урок 3", Kind: "new", TemplateID: "demo:L03", PlannedWeek: "2026-10-26"},
	}
	fx := mathFixture(now, farExam(), specs)
	fx.ShiftWeeks = 4
	h.seed(fx)

	view := h.plan()
	assertOrder(t, view, 3)
	wantWeeks := []string{"2026-11-09", "2026-11-16", "2026-11-23"}
	for i, week := range wantWeeks {
		lesson := byPlace(t, view, i+1)
		if lesson.ID != specs[i].ID || lesson.PlaceNo != i+1 || lesson.PlannedWeek != week {
			t.Fatalf("урок %d %+v, неделя %s", i+1, lesson, week)
		}
	}
	snap := h.snapshot()
	if !slices.Equal(snap.LessonIDs, []string{"demo-l01", "demo-l02", "demo-l03"}) ||
		!slices.Equal(snap.PlannedWeeks, wantWeeks) {
		t.Fatalf("снимок %+v", snap)
	}
}

func TestPL_R8_MovedMark(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(1)
	specs[0].Topics = []string{"M01"}
	specs[0].Moved = true
	specs[0].StartsAt = "2026-11-20T18:00:00Z"
	h.seed(mathFixture(now, farExam(), specs))

	view := h.plan()
	assertOrder(t, view, 1)
	lesson := view.Lessons[0]
	if lesson.ID != "demo-l01" || lesson.PlaceNo != 1 || lesson.Title != "Урок 1" || lesson.Kind != "new" {
		t.Fatalf("поля %+v", lesson)
	}
	if !slices.Equal(lesson.Topics, []string{"M01"}) || lesson.State == "" {
		t.Fatalf("темы или состояние %+v", lesson)
	}
	if lesson.State != "planned" && lesson.State != "frozen" && lesson.State != "held" && lesson.State != "skipped" {
		t.Fatalf("состояние %s", lesson.State)
	}
	if !lesson.Moved || lesson.Added {
		t.Fatalf("пометки moved=%v added=%v", lesson.Moved, lesson.Added)
	}
	if lesson.StartsAt == "" && lesson.PlannedWeek == "" {
		t.Fatal("нет ни даты, ни недели")
	}
	sameInstant(t, lesson.StartsAt, "2026-11-20T18:00:00Z")
}

func TestPL_R9_DivergenceThree(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(3)
	specs[0].Moved = true
	specs[0].StartsAt = "2026-11-20T18:00:00Z"
	specs[1].Moved = true
	specs[1].StartsAt = "2026-11-27T18:00:00Z"
	specs = append(specs, plan.LessonSpec{
		ID:          "demo-add",
		PlaceNo:     4,
		Title:       "Добавленный",
		Kind:        "new",
		TemplateID:  "demo:Ladd",
		PlannedWeek: "2026-11-02",
		Added:       true,
	})
	h.seed(mathFixture(now, farExam(), specs))

	snap := h.snapshot()
	if slices.Contains(snap.LessonIDs, "demo-add") {
		t.Fatalf("добавленный урок попал в план 0: %v", snap.LessonIDs)
	}
	rec := h.divergence(h.world.StudentID)
	h.status(rec, http.StatusOK)
	report := decode[[]divergence](t, rec)
	seen := map[string]int{}
	for _, item := range report {
		seen[item.LessonID+" "+item.Change]++
	}
	for _, key := range []string{"demo-l01 moved", "demo-l02 moved", "demo-add added"} {
		if seen[key] == 0 {
			t.Fatalf("в отчёте нет %s: %+v", key, report)
		}
	}
}

func TestPL_R10_DoesNotFit(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), numbered(4))
	fx.Schedule = anchorSchedule()
	h.seed(fx)

	view := h.plan()
	assertOrder(t, view, 4)
	sameInstant(t, byPlace(t, view, 1).StartsAt, "2026-11-02T18:00:00Z")
	sameInstant(t, byPlace(t, view, 2).StartsAt, "2026-11-05T18:00:00Z")
	if byPlace(t, view, 3).StartsAt != "" || byPlace(t, view, 4).StartsAt != "" {
		t.Fatalf("лишние слоты %q %q", byPlace(t, view, 3).StartsAt, byPlace(t, view, 4).StartsAt)
	}
	var fit []warning
	for _, item := range view.Warnings {
		if item.Type == "plan_does_not_fit" {
			fit = append(fit, item)
		}
	}
	if len(fit) != 1 || !hasNumber(fit[0], 2) {
		t.Fatalf("предупреждения %+v", view.Warnings)
	}
}

func TestPL_R11_LowHomework(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(4)
	fx := mathFixture(now, farExam(), specs)
	percent := 20
	fx.HomeworkPercent = &percent
	h.seed(fx)

	count, points, maxPoints := h.submittedHomework()
	if count != 1 || points != 20 || maxPoints != 100 {
		t.Fatalf("сданное ДЗ: штук %d, баллов %d из %d", count, points, maxPoints)
	}
	view := h.plan()
	assertOrder(t, view, 4)
	for i, spec := range specs {
		lesson := byPlace(t, view, i+1)
		if lesson.ID != spec.ID || lesson.TemplateID != spec.TemplateID {
			t.Fatalf("порядок сам изменился: %+v", view.Lessons)
		}
	}
	if h.eventCount("plan_reordered") != 0 {
		t.Fatal("низкое ДЗ записало plan_reordered")
	}
}

func TestForbidden_ForeignStudent(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, farExam(), numbered(3))
	fx.OtherStudent = true
	h.seed(fx)

	before := h.plan()
	assertOrder(t, before, 3)
	events := h.eventCount("plan_reordered")

	h.expectCode(h.planOf(h.world.OtherStudentID), http.StatusForbidden, "forbidden")
	h.expectCode(h.divergence(h.world.OtherStudentID), http.StatusForbidden, "forbidden")
	h.expectCode(h.reorder(h.world.OtherStudentID, []string{h.world.OtherLessonID}), http.StatusForbidden, "forbidden")
	h.expectCode(h.reorder(h.world.StudentID, []string{"demo-l01", "demo-l02", h.world.OtherLessonID}), http.StatusForbidden, "forbidden")
	h.expectCode(h.reorder(h.world.StudentID, []string{"demo-l01", "demo-l02", "demo-missing"}), http.StatusForbidden, "forbidden")

	after := h.plan()
	assertOrder(t, after, 3)
	for place := 1; place <= 3; place++ {
		if byPlace(t, after, place).ID != byPlace(t, before, place).ID {
			t.Fatalf("место %d изменилось после отказа", place)
		}
	}
	if h.eventCount("plan_reordered") != events {
		t.Fatal("чужой запрос записал plan_reordered")
	}
}
