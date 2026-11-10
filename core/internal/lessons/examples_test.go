package lessons_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/lessons"
)

// lessons-01 … lessons-21 и три границы: чужой ученик, занятый слот, отметка не из состава.
// Плюс урок не проведён и не текущий — код lesson_not_held из раздела 5.

func TestLS_R2_FourSlots(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       anchorExam(),
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        plannedLessons(4),
	})

	rec := h.putSchedule(anchorSchedule())
	h.status(rec, http.StatusOK)
	got := decode[scheduleRule](t, rec)
	if !slices.Equal(got.Weekdays, []int{1, 4}) || got.StartTime != "18:00" || got.DurationMin != 60 {
		t.Fatalf("правило %+v", got)
	}

	plan := h.plan()
	assertOrder(t, plan, 4)
	wantStarts := []string{
		"2026-11-02T18:00:00Z",
		"2026-11-05T18:00:00Z",
		"2026-11-09T18:00:00Z",
		"2026-11-12T18:00:00Z",
	}
	for i, starts := range wantStarts {
		sameInstant(t, plan.Lessons[i].StartsAt, starts)
		if stringsHasDate(plan.Lessons[i].StartsAt, "2026-11-16") {
			t.Fatalf("дата экзамена попала в слот %s", plan.Lessons[i].StartsAt)
		}
	}
	if plan.Lessons[0].State != "frozen" || plan.Lessons[1].State != "frozen" {
		t.Fatalf("ближайшие %s и %s", plan.Lessons[0].State, plan.Lessons[1].State)
	}
	if plan.Lessons[2].State != "planned" || plan.Lessons[3].State != "planned" {
		t.Fatalf("дальние %s и %s", plan.Lessons[2].State, plan.Lessons[3].State)
	}

	event := decodePayload[scheduleEvent](t, h.lastRaw("schedule_rule_set"))
	if event.Subject != subjectMath || !slices.Equal(event.Weekdays, []int{1, 4}) ||
		event.StartTime != "18:00" || event.DurationMin != 60 {
		t.Fatalf("schedule_rule_set %+v", event)
	}
}

func TestLS_R3_WeekWithoutRule(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.PlannedWeek = "2026-11-09"
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson},
	})

	plan := h.plan()
	assertOrder(t, plan, 1)
	if plan.Lessons[0].PlannedWeek != "2026-11-09" {
		t.Fatalf("planned_week %q", plan.Lessons[0].PlannedWeek)
	}
	if plan.Lessons[0].StartsAt != "" {
		t.Fatalf("без правила появилось время %s", plan.Lessons[0].StartsAt)
	}
}

func TestLS_R4_MoveKeepsNumbers(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        plannedLessons(6),
	})

	before := h.plan()
	assertOrder(t, before, 6)
	target := "2026-11-17T18:00:00Z"
	lesson5 := byPlace(t, before, 5)
	stateBefore := lesson5.State
	rec := h.move(lesson5.ID, target)
	h.status(rec, http.StatusOK)
	moved := decode[lessonView](t, rec)
	if moved.PlaceNo != 5 || !moved.Moved || moved.State != stateBefore {
		t.Fatalf("перенос %+v, состояние было %s", moved, stateBefore)
	}
	sameInstant(t, moved.StartsAt, target)

	after := h.plan()
	assertOrder(t, after, 6)
	for place := 1; place <= 6; place++ {
		was := byPlace(t, before, place)
		nowLesson := byPlace(t, after, place)
		if nowLesson.ID != was.ID || nowLesson.PlaceNo != was.PlaceNo {
			t.Fatalf("номер урока %d изменился: %s/%d", place, nowLesson.ID, nowLesson.PlaceNo)
		}
		if place == 5 {
			sameInstant(t, nowLesson.StartsAt, target)
			continue
		}
		sameInstant(t, nowLesson.StartsAt, was.StartsAt)
		if nowLesson.Moved {
			t.Fatalf("урок %d помечен перенесённым", place)
		}
	}

	payload := decodePayload[movedPayload](t, h.lastRaw("lesson_moved"))
	if payload.LessonID != lesson5.ID {
		t.Fatalf("lesson_id %s", payload.LessonID)
	}
	sameInstant(t, payload.To, target)
	if payload.From != "" {
		sameInstant(t, payload.From, lesson5.StartsAt)
	}
}

func TestLS_R6_CancelShifts(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        plannedLessons(4),
	})

	before := h.plan()
	assertOrder(t, before, 4)
	states := map[int]string{}
	for place := 1; place <= 4; place++ {
		states[place] = byPlace(t, before, place).State
	}
	original := byPlace(t, before, 1).StartsAt
	rec := h.cancel(byPlace(t, before, 1).ID)
	h.status(rec, http.StatusOK)
	_ = decode[planView](t, rec)

	after := h.plan()
	assertOrder(t, after, 4)
	want := []string{
		"2026-11-05T18:00:00Z",
		"2026-11-09T18:00:00Z",
		"2026-11-12T18:00:00Z",
		"2026-11-16T18:00:00Z",
	}
	for place, starts := range want {
		lesson := byPlace(t, after, place+1)
		sameInstant(t, lesson.StartsAt, starts)
		if lesson.State != states[place+1] {
			t.Fatalf("урок %d: состояние %s, было %s", place+1, lesson.State, states[place+1])
		}
		if lesson.ID != byPlace(t, before, place+1).ID {
			t.Fatalf("урок %d сменил id", place+1)
		}
	}
	if h.draftCount() != 0 {
		t.Fatal("после отмены появилась строка core.homework_draft")
	}
	payload := decodePayload[cancelledPayload](t, h.lastRaw("lesson_cancelled"))
	if payload.LessonID != byPlace(t, before, 1).ID {
		t.Fatalf("lesson_id %s", payload.LessonID)
	}
	sameInstant(t, payload.Date, original)
}

func TestLS_R7_PlanDoesNotFit(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        plannedLessons(4),
	})

	plan := h.plan()
	assertOrder(t, plan, 4)
	sameInstant(t, byPlace(t, plan, 1).StartsAt, "2026-11-02T18:00:00Z")
	sameInstant(t, byPlace(t, plan, 2).StartsAt, "2026-11-05T18:00:00Z")
	if byPlace(t, plan, 3).StartsAt != "" || byPlace(t, plan, 4).StartsAt != "" {
		t.Fatalf("лишние слоты %q %q", byPlace(t, plan, 3).StartsAt, byPlace(t, plan, 4).StartsAt)
	}
	if stringsHasDate(byPlace(t, plan, 1).StartsAt, "2026-11-09") || stringsHasDate(byPlace(t, plan, 2).StartsAt, "2026-11-09") {
		t.Fatal("дата экзамена заняла слот")
	}
	var fit []warning
	for _, item := range plan.Warnings {
		if item.Type == "plan_does_not_fit" {
			fit = append(fit, item)
		}
	}
	if len(fit) != 1 || !hasNumber(fit[0], 2) {
		t.Fatalf("предупреждения %+v", plan.Warnings)
	}
}

func TestLS_R9_FreezeTwo(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	specs := make([]lessons.LessonSpec, 0, 9)
	for place := 1; place <= 6; place++ {
		specs = append(specs, heldLesson(place))
	}
	for place := 7; place <= 9; place++ {
		specs = append(specs, basicLesson(place))
	}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        specs,
	})

	plan := h.plan()
	assertOrder(t, plan, 9)
	for place := 1; place <= 6; place++ {
		if got := byPlace(t, plan, place).State; got != "held" {
			t.Fatalf("урок %d: %s", place, got)
		}
	}
	if byPlace(t, plan, 7).State != "frozen" || byPlace(t, plan, 8).State != "frozen" {
		t.Fatalf("7 и 8: %s %s", byPlace(t, plan, 7).State, byPlace(t, plan, 8).State)
	}
	if byPlace(t, plan, 9).State != "planned" {
		t.Fatalf("урок 9: %s", byPlace(t, plan, 9).State)
	}
}

func TestLS_R10_Snapshot(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(7)
	lesson.TemplateID = "demo:L07"
	lesson.Stages = []lessons.StageSpec{
		{
			ID:      "demo:L07.G1",
			Title:   "Теория",
			Kind:    "theory",
			Minutes: [2]int{0, 25},
			Blocks:  []string{"demo:B1", "demo:B2"},
		},
		{
			ID:      "demo:L07.G2",
			Title:   "Практика",
			Kind:    "practice",
			Minutes: [2]int{25, 60},
			Practice: []lessons.PracticeSpec{{
				VariantID:   "demo:V7",
				TypeID:      "demo:T7",
				Role:        "solve_alone",
				Position:    1,
				TheoryBlock: "demo:B1",
			}},
		},
	}
	specs := make([]lessons.LessonSpec, 0, 7)
	for place := 1; place <= 6; place++ {
		specs = append(specs, heldLesson(place))
	}
	specs = append(specs, lesson)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 6,
		Lessons:        specs,
	})

	if byPlace(t, h.plan(), 7).State != "frozen" {
		t.Fatal("урок 7 не заморожен")
	}
	detail := h.lesson("demo-l07")
	if detail.Source != "snapshot" {
		t.Fatalf("source %s", detail.Source)
	}
	if len(detail.Stages) != 2 || detail.Stages[0].ID != "demo:L07.G1" || detail.Stages[1].ID != "demo:L07.G2" {
		t.Fatalf("этапы %+v", detail.Stages)
	}
	if !slices.Equal(detail.Stages[0].Minutes, []int{0, 25}) || !slices.Equal(detail.Stages[0].Blocks, []string{"demo:B1", "demo:B2"}) {
		t.Fatalf("теория %+v", detail.Stages[0])
	}
	if !slices.Equal(detail.Stages[1].Minutes, []int{25, 60}) || len(detail.Stages[1].Practice) != 1 {
		t.Fatalf("практика %+v", detail.Stages[1])
	}
	item := detail.Stages[1].Practice[0]
	if item.VariantID != "demo:V7" || item.TypeID != "demo:T7" || item.Position != 1 || item.Role != "solve_alone" {
		t.Fatalf("вариант %+v", item)
	}

	event := decodePayload[frozenPayload](t, h.lastRaw("lesson_frozen"))
	if event.LessonID != "demo-l07" || event.TemplateID != "demo:L07" || event.CatalogVersion != 6 {
		t.Fatalf("снимок %+v", event)
	}
	if !slices.Equal(event.Blocks, []string{"demo:B1", "demo:B2"}) {
		t.Fatalf("блоки события %v", event.Blocks)
	}
	if len(event.Stages) != 2 || event.Stages[0].StageID != "demo:L07.G1" || !slices.Equal(event.Stages[0].Minutes, []int{0, 25}) ||
		event.Stages[1].StageID != "demo:L07.G2" || !slices.Equal(event.Stages[1].Minutes, []int{25, 60}) {
		t.Fatalf("этапы события %+v", event.Stages)
	}
	if len(event.Practice) != 1 || event.Practice[0].VariantID != "demo:V7" || event.Practice[0].TypeID != "demo:T7" ||
		event.Practice[0].StageID != "demo:L07.G2" || event.Practice[0].Position != 1 {
		t.Fatalf("практика события %+v", event.Practice)
	}
}

func TestLS_R11_SeenVariantExcluded(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := teachLesson(1)
	lesson.Stages[1].Practice[0].VariantID = "demo:V-seen"
	lesson.Stages[1].Practice[0].TypeID = "demo:T1"
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson},
		Pool: []lessons.VariantSpec{
			{ID: "demo:V-seen", TypeID: "demo:T1", Seen: true},
			{ID: "demo:V-reserve", TypeID: "demo:T1", Reserve: true},
			{ID: "demo:V-ok", TypeID: "demo:T1"},
		},
	})

	detail := h.lesson("demo-l01")
	got := practiceVariants(detail.Stages[1].Practice)
	if !slices.Equal(got, []string{"demo:V-ok"}) {
		t.Fatalf("практика %v", got)
	}
}

func TestLS_R11_UsedVariantSeenNoAttempts(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	first := teachLesson(1)
	second := teachLesson(2)
	for _, lesson := range []*lessons.LessonSpec{&first, &second} {
		lesson.Stages[1].Practice[0].VariantID = "demo:V0"
		lesson.Stages[1].Practice[0].TypeID = "demo:T1"
		lesson.Stages[1].Practice[0].TheoryBlock = "demo:B1"
	}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{first, second},
		Pool: []lessons.VariantSpec{
			{ID: "demo:V0", TypeID: "demo:T1", Reserve: true},
			{ID: "demo:Va", TypeID: "demo:T1"},
			{ID: "demo:Vb", TypeID: "demo:T1"},
		},
	})

	if got := practiceVariants(h.lesson("demo-l01").Stages[1].Practice); !slices.Equal(got, []string{"demo:Va"}) {
		t.Fatalf("первый урок %v", got)
	}
	if got := practiceVariants(h.lesson("demo-l02").Stages[1].Practice); !slices.Equal(got, []string{"demo:Vb"}) {
		t.Fatalf("второй урок %v", got)
	}
	if n := h.attemptCount(); n != 0 {
		t.Fatalf("попыток %d", n)
	}
}

func TestLS_R12_SnapshotRefused(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.Stages[0].Blocks = []string{"demo:B1", "demo:B2"}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson},
		ReviseBlocks:   []string{"demo:B9"},
	})

	detail := h.lesson("demo-l01")
	if !slices.Equal(blocksOf(detail.Stages), []string{"demo:B1", "demo:B2"}) {
		t.Fatalf("снимок изменился: %v", blocksOf(detail.Stages))
	}
	if h.eventCount("lesson_frozen") != 1 {
		t.Fatalf("событий lesson_frozen %d", h.eventCount("lesson_frozen"))
	}
	event := decodePayload[frozenPayload](t, h.lastRaw("lesson_frozen"))
	if !slices.Equal(event.Blocks, []string{"demo:B1", "demo:B2"}) {
		t.Fatalf("блоки события %v", event.Blocks)
	}
}

func TestLS_R13_LessonFileManualHomework(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.NoBlocks = true
	lesson.Stages = nil
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson},
	})

	plan := h.plan()
	assertOrder(t, plan, 1)
	if plan.Lessons[0].State != "planned" && plan.Lessons[0].State != "frozen" {
		t.Fatalf("урок заблокирован состоянием %s", plan.Lessons[0].State)
	}
	detail := h.lesson("demo-l01")
	if detail.Source != "l_file" {
		t.Fatalf("source %s", detail.Source)
	}
	if len(blocksOf(detail.Stages)) != 0 {
		t.Fatalf("блоки %v", blocksOf(detail.Stages))
	}
	if h.eventCount("lesson_frozen") != 0 {
		t.Fatal("для урока без блоков записан снимок")
	}
	h.expectNoDraft("demo-l01")
}

func TestLS_R15_OverflowToUnfrozen(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	source := basicLesson(1)
	source.Stages = []lessons.StageSpec{{
		ID:      "demo:L01.G1",
		Title:   "Короткий кусок",
		Kind:    "theory",
		Minutes: [2]int{0, 10},
		Blocks:  []string{"demo:B-carry"},
	}}
	full := basicLesson(2)
	full.Stages = []lessons.StageSpec{
		{
			ID:      "demo:L02.R",
			Title:   "Разбор",
			Kind:    "homework_review",
			Minutes: [2]int{0, 10},
		},
		{
			ID:      "demo:L02.T",
			Title:   "Теория",
			Kind:    "theory",
			Minutes: [2]int{10, 50},
			Blocks:  []string{"demo:B-keep"},
		},
		{
			ID:      "demo:L02.P",
			Title:   "Конец",
			Kind:    "practice",
			Minutes: [2]int{50, 60},
			Practice: []lessons.PracticeSpec{{
				VariantID:   "demo:V-end",
				TypeID:      "demo:T-end",
				Role:        "solve_alone",
				Position:    1,
				TheoryBlock: "demo:B-keep",
			}},
		},
	}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{source, full, quietLesson(3), quietLesson(4)},
	})

	rec := h.mark("demo-l01", marks("", "", 0))
	h.status(rec, http.StatusOK)
	if decode[lessonDetail](t, rec).Lesson.State != "held" {
		t.Fatal("отмеченный урок не held")
	}

	second := h.lesson("demo-l02")
	assertCarry(t, second, "demo-l01", []string{"demo:B-carry"}, nil)
	if !slices.Contains(blocksOf(second.Stages), "demo:B-keep") {
		t.Fatalf("теория урока 2 пропала: %v", blocksOf(second.Stages))
	}
	if hasVariant(second.Stages, "demo:V-end") {
		t.Fatal("конец урока 2 остался на месте")
	}
	if hasVariant(h.lesson("demo-l03").Stages, "demo:V-end") {
		t.Fatal("конец ушёл на замороженный урок 3")
	}
	fourth := h.lesson("demo-l04")
	assertCarry(t, fourth, "demo-l02", nil, []string{"demo:V-end"})
	h.expectNoDraft("demo-l01")
}

func TestLS_R16_MarksHeld(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{markedSample()},
	})

	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 3))
	h.status(rec, http.StatusOK)
	detail := decode[lessonDetail](t, rec)
	if detail.Lesson.State != "held" {
		t.Fatalf("состояние %s", detail.Lesson.State)
	}
	if detail.Lesson.Marks == nil || detail.Lesson.Marks.LastTheoryBlock == nil || *detail.Lesson.Marks.LastTheoryBlock != "demo:B2" {
		t.Fatalf("отметки теории %+v", detail.Lesson.Marks)
	}
	if detail.Lesson.Marks.LastPractice == nil || detail.Lesson.Marks.LastPractice.StageID != "demo:P" || detail.Lesson.Marks.LastPractice.Position != 3 {
		t.Fatalf("отметки практики %+v", detail.Lesson.Marks)
	}
	event := decodePayload[markedPayload](t, h.lastRaw("lesson_marked"))
	if event.LessonID != "demo-l01" || event.LastTheoryBlock == nil || *event.LastTheoryBlock != "demo:B2" {
		t.Fatalf("событие %+v", event)
	}
	if event.LastPractice == nil || event.LastPractice.StageID != "demo:P" || event.LastPractice.Position != 3 {
		t.Fatalf("практика события %+v", event.LastPractice)
	}
}

func TestLS_R17_NothingPassed(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	lesson := markedSample()
	lesson.Stages = []lessons.StageSpec{
		{
			ID:      "demo:T",
			Title:   "Теория",
			Kind:    "theory",
			Minutes: [2]int{0, 10},
			Blocks:  []string{"demo:B1", "demo:B2"},
		},
		{
			ID:      "demo:P",
			Title:   "Практика",
			Kind:    "practice",
			Minutes: [2]int{10, 20},
			Practice: []lessons.PracticeSpec{
				{VariantID: "demo:V1", TypeID: "demo:T1", Role: "solve_alone", Position: 1, TheoryBlock: "demo:B1"},
				{VariantID: "demo:V2", TypeID: "demo:T1", Role: "solve_alone", Position: 2, TheoryBlock: "demo:B2"},
			},
		},
	}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson, quietLesson(2)},
	})

	rec := h.mark("demo-l01", marks("", "", 0))
	h.status(rec, http.StatusOK)
	if decode[lessonDetail](t, rec).Lesson.State != "held" {
		t.Fatal("урок не held")
	}
	raw := h.lastRaw("lesson_marked")
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	if string(object["last_theory_block"]) != "null" || string(object["last_practice"]) != "null" {
		t.Fatalf("отметки %s", raw)
	}
	assertCarry(t, h.lesson("demo-l02"), "demo-l01", []string{"demo:B1", "demo:B2"}, []string{"demo:V1", "demo:V2"})
	if h.draftCount() != 0 {
		t.Fatal("при двух «нет» появилась строка core.homework_draft")
	}
	carried := decodePayload[struct {
		FromLessonID string   `json:"from_lesson_id"`
		ToLessonID   string   `json:"to_lesson_id"`
		Blocks       []string `json:"blocks"`
	}](t, h.lastRaw("lesson_carried_over"))
	if carried.FromLessonID != "demo-l01" || carried.ToLessonID != "demo-l02" || !slices.Equal(carried.Blocks, []string{"demo:B1", "demo:B2"}) {
		t.Fatalf("перенос %+v", carried)
	}
}

func TestLS_R18_UnexplainedToNext(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{threeBlocks(), quietLesson(2)},
	})

	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 2))
	h.status(rec, http.StatusOK)
	assertCarry(t, h.lesson("demo-l02"), "demo-l01", []string{"demo:B3"}, []string{"demo:V3"})
	if _, ids := h.draftIDs("demo-l01"); slices.Contains(ids, "demo:V3") {
		t.Fatal("задача к необъяснённому блоку попала в ДЗ")
	}
}

func TestLS_R18_ExplainedUnsolvedToHomework(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	lesson := threeBlocks()
	lesson.Stages[0].Blocks = []string{"demo:B1", "demo:B2"}
	lesson.Stages[1].Practice = []lessons.PracticeSpec{
		{VariantID: "demo:V1", TypeID: "demo:T1", Role: "solve_alone", Position: 1, TheoryBlock: "demo:B1"},
		{VariantID: "demo:V2", TypeID: "demo:T1", Role: "solve_alone", Position: 2, TheoryBlock: "demo:B1"},
		{VariantID: "demo:V3", TypeID: "demo:T1", Role: "solve_alone", Position: 3, TheoryBlock: "demo:B1"},
		{VariantID: "demo:V4", TypeID: "demo:T1", Role: "solve_alone", Position: 4, TheoryBlock: "demo:B1"},
		{VariantID: "demo:V5", TypeID: "demo:T1", Role: "solve_alone", Position: 5, TheoryBlock: "demo:B1"},
	}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson, quietLesson(2)},
	})

	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 4))
	h.status(rec, http.StatusOK)
	ok, ids := h.draftIDs("demo-l01")
	if !ok || !slices.Contains(ids, "demo:V5") {
		t.Fatalf("задачи 5 нет в items черновика: %v", ids)
	}
	if hasVariant(h.lesson("demo-l02").Stages, "demo:V5") {
		t.Fatal("задача 5 перенесена в следующий урок")
	}
	if h.lesson("demo-l02").CarriedIn != nil && slices.Contains(practicePositions(h.lesson("demo-l02").CarriedIn.Practice), 5) {
		t.Fatal("задача 5 есть в carried_in")
	}
}

func TestLS_R19_ForeignBlock(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{teachLesson(1)},
	})
	before := h.lesson("demo-l01").Lesson.State
	rec := h.mark("demo-l01", marks("demo:B-foreign", "demo:L01.P", 1))
	h.expectCode(rec, http.StatusUnprocessableEntity, "mark_not_in_lesson")
	if got := h.lesson("demo-l01").Lesson.State; got != before {
		t.Fatalf("состояние стало %s", got)
	}
	if h.eventCount("lesson_marked") != 0 {
		t.Fatal("чужая отметка записана")
	}
}

func TestLS_R19a_CorrectBeforeIssue(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{correctionSample(), quietLesson(2)},
	})

	first := h.mark("demo-l01", marks("", "", 0))
	h.status(first, http.StatusOK)
	second := h.mark("demo-l01", marks("demo:B2", "demo:P", 2))
	h.status(second, http.StatusOK)

	if h.eventCount("lesson_marked") < 2 {
		t.Fatalf("событий lesson_marked %d", h.eventCount("lesson_marked"))
	}
	event := decodePayload[markedPayload](t, h.lastRaw("lesson_marked"))
	if event.LastTheoryBlock == nil || *event.LastTheoryBlock != "demo:B2" ||
		event.LastPractice == nil || event.LastPractice.Position != 2 {
		t.Fatalf("последняя отметка %+v", event)
	}
	assertCarry(t, h.lesson("demo-l02"), "demo-l01", []string{"demo:B3"}, []string{"demo:V4"})
	if hasVariant(h.lesson("demo-l02").Stages, "demo:V3") {
		t.Fatal("пересчитанный перенос всё ещё содержит задачу 3")
	}
	ok, ids := h.draftIDs("demo-l01")
	if !ok || !slices.Contains(ids, "demo:V3") {
		t.Fatalf("после исправления в черновике нет demo:V3: %v", ids)
	}
	if slices.Contains(ids, "demo:V4") {
		t.Fatal("необъяснённая задача осталась в черновике")
	}
}

func TestLS_R19a_AlreadyMarked(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	lesson := teachLesson(1)
	lesson.State = "held"
	lesson.Issued = true
	theory := "demo:B1"
	lesson.Mark = &lessons.MarkSpec{
		Theory:   &theory,
		Practice: &lessons.PracticeRef{StageID: "demo:L01.P", Position: 1},
	}
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson},
	})

	before := h.eventCount("lesson_marked")
	if before < 1 {
		t.Fatal("исходная отметка не записана")
	}
	rec := h.mark("demo-l01", marks("demo:B1", "demo:L01.P", 1))
	h.expectCode(rec, http.StatusConflict, "already_marked")
	if h.eventCount("lesson_marked") != before {
		t.Fatal("после выдачи записана новая отметка")
	}
	got := h.lesson("demo-l01")
	if got.Lesson.Marks == nil || got.Lesson.Marks.LastTheoryBlock == nil || *got.Lesson.Marks.LastTheoryBlock != "demo:B1" {
		t.Fatalf("теория изменилась: %+v", got.Lesson.Marks)
	}
	if got.Lesson.Marks.LastPractice == nil || got.Lesson.Marks.LastPractice.Position != 1 {
		t.Fatalf("практика изменилась: %+v", got.Lesson.Marks.LastPractice)
	}
}

func TestLS_R20_CoverageAll(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.NoBlocks = true
	lesson.Stages = nil
	lesson.State = "held"
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson, quietLesson(2)},
	})

	rec := h.mark("demo-l01", map[string]string{"coverage": "all"})
	h.status(rec, http.StatusOK)
	detail := decode[lessonDetail](t, rec)
	if detail.Lesson.State != "held" || detail.Source != "l_file" {
		t.Fatalf("ответ %+v source %s", detail.Lesson.State, detail.Source)
	}
	event := decodePayload[markedPayload](t, h.lastRaw("lesson_marked"))
	if event.LessonID != "demo-l01" || event.Coverage != "all" {
		t.Fatalf("событие %+v", event)
	}
	if h.draftCount() != 0 {
		t.Fatal("для урока без снимка собрался черновик")
	}
	if h.eventCount("lesson_carried_over") != 0 {
		t.Fatal("для урока без снимка построен перенос")
	}
	if next := h.lesson("demo-l02"); next.CarriedIn != nil {
		t.Fatalf("перенос на следующий урок %+v", next.CarriedIn)
	}
}

func TestLS_R22_DiagnosticStored(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.Kind = "diagnostic"
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{lesson},
	})

	plan := h.plan()
	assertOrder(t, plan, 1)
	if plan.Lessons[0].Kind != "diagnostic" {
		t.Fatalf("kind %s", plan.Lessons[0].Kind)
	}
	if plan.Lessons[0].State != "planned" && plan.Lessons[0].State != "frozen" {
		t.Fatalf("состояние %s", plan.Lessons[0].State)
	}
	if plan.Lessons[0].StartsAt == "" {
		t.Fatal("diagnostic не получил слот обычного урока")
	}
	rec := h.call(http.MethodPost, "/lessons/demo-l01/answers", map[string]string{"raw_input": "1"})
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("пакет принял ответ diagnostic: %d %s", rec.Code, rec.Body.Bytes())
	}
	if n := h.attemptCount(); n != 0 {
		t.Fatalf("попыток %d", n)
	}
}

func TestLS_R19_ForeignStudent(t *testing.T) {
	now := noonBefore()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		OtherStudent:   true,
		Lessons:        []lessons.LessonSpec{teachLesson(1)},
	})

	rec := h.mark(h.world.OtherLessonID, marks("demo:B1", "demo:L01.P", 1))
	h.expectCode(rec, http.StatusForbidden, "forbidden")
	if h.eventCount("lesson_marked") != 0 {
		t.Fatal("отметка чужого ученика записана")
	}
}

func TestLS_R4_SlotTaken(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        plannedLessons(2),
	})

	before := h.plan()
	occupied := byPlace(t, before, 1).StartsAt
	rec := h.move(byPlace(t, before, 2).ID, occupied)
	h.expectCode(rec, http.StatusConflict, "slot_taken")
	after := h.plan()
	sameInstant(t, byPlace(t, after, 1).StartsAt, byPlace(t, before, 1).StartsAt)
	sameInstant(t, byPlace(t, after, 2).StartsAt, byPlace(t, before, 2).StartsAt)
	if h.eventCount("lesson_moved") != 0 {
		t.Fatal("перенос на занятый слот записан")
	}
}

func TestLS_R19_MarkNotInComposition(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{teachLesson(1)},
	})

	before := h.lesson("demo-l01").Lesson.State
	rec := h.mark("demo-l01", marks("demo:B1", "demo:missing", 1))
	h.expectCode(rec, http.StatusUnprocessableEntity, "mark_not_in_lesson")
	if got := h.lesson("demo-l01").Lesson.State; got != before {
		t.Fatalf("состояние стало %s", got)
	}
	if h.eventCount("lesson_marked") != 0 {
		t.Fatal("отметка не из состава записана")
	}
}

func TestLS_R19_NotHeld(t *testing.T) {
	now := noonBefore()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now:            now,
		ExamDate:       farExam(),
		Subject:        subjectMath,
		Schedule:       anchorSchedule(),
		CatalogVersion: 1,
		Lessons:        []lessons.LessonSpec{teachLesson(1), teachLesson(2)},
	})

	before := byPlace(t, h.plan(), 2).State
	rec := h.mark("demo-l02", marks("demo:B2", "", 0))
	h.expectCode(rec, http.StatusConflict, "lesson_not_held")
	if got := byPlace(t, h.plan(), 2).State; got != before {
		t.Fatalf("состояние стало %s", got)
	}
	if h.eventCount("lesson_marked") != 0 {
		t.Fatal("отметка будущего урока записана")
	}
}

func markedSample() lessons.LessonSpec {
	lesson := basicLesson(1)
	lesson.Stages = []lessons.StageSpec{
		{
			ID:      "demo:T",
			Title:   "Теория",
			Kind:    "theory",
			Minutes: [2]int{0, 30},
			Blocks:  []string{"demo:B1", "demo:B2", "demo:B3"},
		},
		{
			ID:      "demo:P",
			Title:   "Практика",
			Kind:    "practice",
			Minutes: [2]int{30, 60},
			Practice: []lessons.PracticeSpec{
				{VariantID: "demo:V1", TypeID: "demo:T1", Role: "solve_alone", Position: 1, TheoryBlock: "demo:B1"},
				{VariantID: "demo:V2", TypeID: "demo:T1", Role: "solve_alone", Position: 2, TheoryBlock: "demo:B2"},
				{VariantID: "demo:V3", TypeID: "demo:T1", Role: "solve_alone", Position: 3, TheoryBlock: "demo:B3"},
			},
		},
	}
	return lesson
}

func threeBlocks() lessons.LessonSpec {
	lesson := markedSample()
	lesson.Stages[0].Minutes = [2]int{0, 20}
	lesson.Stages[1].Minutes = [2]int{20, 40}
	return lesson
}

func correctionSample() lessons.LessonSpec {
	lesson := basicLesson(1)
	lesson.Stages = []lessons.StageSpec{
		{
			ID:      "demo:T1",
			Title:   "Блок 1",
			Kind:    "theory",
			Minutes: [2]int{0, 10},
			Blocks:  []string{"demo:B1"},
		},
		{
			ID:      "demo:T2",
			Title:   "Блок 2",
			Kind:    "theory",
			Minutes: [2]int{10, 20},
			Blocks:  []string{"demo:B2"},
		},
		{
			ID:      "demo:T3",
			Title:   "Блок 3",
			Kind:    "theory",
			Minutes: [2]int{20, 30},
			Blocks:  []string{"demo:B3"},
		},
		{
			ID:      "demo:P",
			Title:   "Практика",
			Kind:    "practice",
			Minutes: [2]int{30, 50},
			Practice: []lessons.PracticeSpec{
				{VariantID: "demo:V1", TypeID: "demo:T1", Role: "solve_alone", Position: 1, TheoryBlock: "demo:B1"},
				{VariantID: "demo:V2", TypeID: "demo:T1", Role: "solve_alone", Position: 2, TheoryBlock: "demo:B2"},
				{VariantID: "demo:V3", TypeID: "demo:T1", Role: "solve_alone", Position: 3, TheoryBlock: "demo:B2"},
				{VariantID: "demo:V4", TypeID: "demo:T1", Role: "solve_alone", Position: 4, TheoryBlock: "demo:B3"},
			},
		},
	}
	return lesson
}

func practicePositions(items []practiceRef) []int {
	out := make([]int, len(items))
	for i, item := range items {
		out[i] = item.Position
	}
	return out
}

func stringsHasDate(value, date string) bool {
	return len(value) >= len(date) && value[:len(date)] == date
}
