package plan_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/plan"
)

func TestPL_R2_S4_ReorderKeepsPlan0(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(mathFixture(now, farExam(), numbered(4)))
	before := h.snapshot()
	order := []string{"demo-l01", "demo-l02", "demo-l04", "demo-l03"}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	sameSnapshot(t, h.snapshot(), before)
	s4AssertPlanContract(t, rec.Body.Bytes())
}

func TestPL_R3_S4_TemplateIDStable(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(4)
	specs[3].ID = "ct-math:L04"
	specs[3].TemplateID = "ct-math:L04"
	h.seed(mathFixture(now, farExam(), specs))
	order := []string{"demo-l01", "demo-l02", "ct-math:L04", "demo-l03"}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	view := decode[planView](t, rec)
	third := byPlace(t, view, 3)
	if third.ID != "ct-math:L04" || third.TemplateID != "ct-math:L04" {
		t.Fatalf("третье место %+v", third)
	}
	if byPlace(t, view, 4).TemplateID != "demo:L03" {
		t.Fatalf("шаблон четвёртого %+v", byPlace(t, view, 4))
	}
}

func TestPL_R4_S4_FrozenReorder(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(mathFixture(now, farExam(), numbered(4)))
	before := h.plan()
	if byPlace(t, before, 1).State != "frozen" {
		t.Fatalf("урок 1 %s", byPlace(t, before, 1).State)
	}
	events := h.eventCount("plan_reordered")
	rec := h.reorder(h.world.StudentID, []string{"demo-l02", "demo-l01", "demo-l03", "demo-l04"})
	h.expectCode(rec, http.StatusConflict, "lesson_frozen")
	after := h.plan()
	for place := 1; place <= 4; place++ {
		if byPlace(t, after, place).ID != byPlace(t, before, place).ID {
			t.Fatalf("место %d сменило урок", place)
		}
	}
	if h.eventCount("plan_reordered") != events {
		t.Fatal("отказ записан как plan_reordered")
	}
}

func TestPL_R4_S4_ForeignMissing(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, farExam(), numbered(3))
	fx.OtherStudent = true
	h.seed(fx)
	foreign := h.planOf(h.world.OtherStudentID)
	missing := h.planOf("demo-missing")
	h.expectCode(foreign, http.StatusForbidden, "forbidden")
	h.expectCode(missing, http.StatusForbidden, "forbidden")
	if foreign.Body.String() != missing.Body.String() {
		t.Fatalf("чужой и несуществующий различимы: %s / %s", foreign.Body.Bytes(), missing.Body.Bytes())
	}
}

func TestPL_R5_S4_WarningsInEnum(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(4)
	specs[2].Topics = []string{"M13"}
	specs[3].Topics = []string{"M14"}
	fx := mathFixture(now, farExam(), specs)
	fx.Prereqs = map[string][]string{"M14": {"M13"}}
	h.seed(fx)
	order := []string{"demo-l01", "demo-l02", "demo-l04", "demo-l03"}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	s4AssertPlanContract(t, rec.Body.Bytes())
	view := h.plan()
	s4AssertPlanContract(t, mustPlan(t, h))
	for _, item := range view.Warnings {
		if !s4WarningTypes[item.Type] {
			t.Fatalf("warning %q вне enum контракта", item.Type)
		}
	}
}

func TestPL_R6_S4_ReorderKeepsDates(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, farExam(), numbered(4))
	fx.Schedule = anchorSchedule()
	h.seed(fx)
	before := h.plan()
	assertOrder(t, before, 4)
	dates := map[string]string{}
	for _, lesson := range before.Lessons {
		dates[lesson.ID] = lesson.StartsAt
		if lesson.StartsAt == "" {
			t.Fatalf("у урока %s нет даты", lesson.ID)
		}
	}
	if byPlace(t, before, 3).State == "frozen" || byPlace(t, before, 4).State == "frozen" {
		t.Fatal("открытые уроки заморожены")
	}
	order := []string{"demo-l01", "demo-l02", "demo-l04", "demo-l03"}
	rec := h.reorder(h.world.StudentID, order)
	h.status(rec, http.StatusOK)
	view := decode[planView](t, rec)
	assertOrder(t, view, 4)
	if byPlace(t, view, 3).ID != "demo-l04" || byPlace(t, view, 3).PlaceNo != 3 {
		t.Fatalf("место 3 %+v", byPlace(t, view, 3))
	}
	if byPlace(t, view, 4).ID != "demo-l03" || byPlace(t, view, 4).PlaceNo != 4 {
		t.Fatalf("место 4 %+v", byPlace(t, view, 4))
	}
	for id, starts := range dates {
		sameInstant(t, byID(t, view, id).StartsAt, starts)
	}
	s4CheckReordered(t, h.lastRaw("plan_reordered"), subjectMath, order)
	s4TemplatesAreArrays(t, h)
	s4AssertPlanContract(t, rec.Body.Bytes())
}

func TestPL_R8_S4_PlanContract(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(2)
	specs[0].Topics = []string{"M01"}
	specs[0].Kind = "new"
	h.seed(mathFixture(now, farExam(), specs))
	rec := h.planOf(h.world.StudentID)
	h.status(rec, http.StatusOK)
	s4AssertPlanContract(t, rec.Body.Bytes())
	view := decode[planView](t, rec)
	assertOrder(t, view, 2)
	lesson := view.Lessons[0]
	if lesson.Title == "" || lesson.Kind == "" || len(lesson.Topics) == 0 {
		t.Fatalf("показ %+v", lesson)
	}
	if lesson.StartsAt == "" && lesson.PlannedWeek == "" {
		t.Fatal("нет ни даты, ни недели")
	}
}

func TestPL_R8_S4_D148_UntilExam(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, farExam(), numbered(14))
	fx.Schedule = anchorSchedule()
	h.seed(fx)
	rec := h.planOf(h.world.StudentID)
	h.status(rec, http.StatusOK)
	s4AssertPlanContract(t, rec.Body.Bytes())
	view := decode[planView](t, rec)
	assertOrder(t, view, 14)
	sameInstant(t, byPlace(t, view, 14).StartsAt, "2026-12-17T18:00:00Z")
	got, err := time.Parse(time.RFC3339, byPlace(t, view, 14).StartsAt)
	if err != nil {
		t.Fatal(err)
	}
	if !got.After(now.Add(42 * 24 * time.Hour)) {
		t.Fatalf("слот %s обрезан 42 сутками", got)
	}
}

func TestPL_R9_S4_DivergenceContract(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(2)
	specs[0].Moved = true
	specs[0].StartsAt = "2026-11-20T18:00:00Z"
	specs = append(specs, plan.LessonSpec{
		ID: "demo-add", PlaceNo: 3, Title: "Добавленный", Kind: "new",
		TemplateID: "demo:Ladd", PlannedWeek: "2026-11-02", Added: true,
	})
	h.seed(mathFixture(now, farExam(), specs))
	rec := h.divergence(h.world.StudentID)
	h.status(rec, http.StatusOK)
	s4AssertDivergence(t, rec.Body.Bytes())
	report := decode[[]divergence](t, rec)
	seen := map[string]bool{}
	for _, item := range report {
		seen[item.LessonID+" "+item.Change] = true
	}
	if !seen["demo-l01 moved"] || !seen["demo-add added"] {
		t.Fatalf("отчёт %+v", report)
	}
}

func TestPL_R10_S4_FitWarningInEnum(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	fx := mathFixture(now, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), numbered(4))
	fx.Schedule = anchorSchedule()
	h.seed(fx)
	rec := h.planOf(h.world.StudentID)
	h.status(rec, http.StatusOK)
	s4AssertPlanContract(t, rec.Body.Bytes())
	view := decode[planView](t, rec)
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

func TestPL_R11_S4_LowHomeworkNoReorder(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := numbered(3)
	fx := mathFixture(now, farExam(), specs)
	percent := 20
	fx.HomeworkPercent = &percent
	h.seed(fx)
	view := h.plan()
	assertOrder(t, view, 3)
	for i, spec := range specs {
		if byPlace(t, view, i+1).ID != spec.ID || byPlace(t, view, i+1).TemplateID != spec.TemplateID {
			t.Fatalf("порядок %+v", view.Lessons)
		}
	}
	if h.eventCount("plan_reordered") != 0 {
		t.Fatal("низкое ДЗ записало plan_reordered")
	}
	if !slices.Equal(places(view), []int{1, 2, 3}) {
		t.Fatalf("места %v", places(view))
	}
}

func mustPlan(t *testing.T, h *harness) []byte {
	t.Helper()
	rec := h.planOf(h.world.StudentID)
	h.status(rec, http.StatusOK)
	return rec.Body.Bytes()
}
