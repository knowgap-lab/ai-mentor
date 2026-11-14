package lessons_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/lessons"
)

// Новые операции и находки S4. Замороженные тесты этого пакета не меняются.
// Проза lessons.examples.yaml и plan.examples.yaml не содержит steps, поэтому
// scenario.Run получает HTTP-форму новых операций, а не сырой файл.

func TestLS_R1_S4_NumberedPlaces(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Lessons: plannedLessons(3),
	})
	view := h.plan()
	assertOrder(t, view, 3)
	for place := 1; place <= 3; place++ {
		lesson := byPlace(t, view, place)
		if lesson.ID != plannedLessons(3)[place-1].ID {
			t.Fatalf("место %d: %s", place, lesson.ID)
		}
	}
}

func TestLS_R2_S4_D148_UntilExam(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(14),
	})
	view := h.plan()
	assertOrder(t, view, 14)
	// 2026-11-02 + 42 суток = 2026-12-14. Четырнадцатый слот пн/чт — 17 декабря,
	// позже этой границы. Дата экзамена 2027-02-01 слотом не является.
	sameInstant(t, byPlace(t, view, 14).StartsAt, "2026-12-17T18:00:00Z")
	if byPlace(t, view, 14).StartsAt == "" {
		t.Fatal("слот после 42 суток заменён вехой")
	}
	horizon := now.Add(42 * 24 * time.Hour)
	got, err := time.Parse(time.RFC3339, byPlace(t, view, 14).StartsAt)
	if err != nil {
		t.Fatal(err)
	}
	if !got.After(horizon) {
		t.Fatalf("слот %s не позже %s", got, horizon)
	}
}

func TestLS_R3_S4_WeekWithoutRule(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.PlannedWeek = "2026-11-09"
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Lessons: []lessons.LessonSpec{lesson},
	})
	view := h.plan()
	if view.Lessons[0].PlannedWeek != "2026-11-09" || view.Lessons[0].StartsAt != "" {
		t.Fatalf("неделя %+v", view.Lessons[0])
	}
}

func TestLS_R4_S4_MoveKeepsNumbers(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(4),
	})
	before := h.plan()
	target := "2026-11-18T18:00:00Z"
	rec := h.move(byPlace(t, before, 4).ID, target)
	h.status(rec, http.StatusOK)
	after := h.plan()
	assertOrder(t, after, 4)
	for place := 1; place <= 4; place++ {
		if byPlace(t, after, place).ID != byPlace(t, before, place).ID || byPlace(t, after, place).PlaceNo != place {
			t.Fatalf("номер места %d изменился", place)
		}
	}
	sameInstant(t, byPlace(t, after, 4).StartsAt, target)
	s4CheckMoved(t, h.lastRaw("lesson_moved"))
}

func TestLS_R5_S4_AddLesson(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(3),
	})
	starts := "2026-11-03T18:00:00Z"
	path := s4AddPath(h.world.StudentID)
	body := s4AddBody(subjectMath, starts)

	noSession := s4CallRaw(h, "", http.MethodPost, path, body, true)
	if noSession.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута addLesson: %s", noSession.Body.Bytes())
	}
	h.expectCode(noSession, http.StatusUnauthorized, "unauthorized")

	noOrigin := s4CallRaw(h, h.token, http.MethodPost, path, body, false)
	h.expectCode(noOrigin, http.StatusForbidden, "csrf")
	if h.eventCount("lesson_added") != 0 {
		t.Fatal("отказ записал lesson_added")
	}

	before := h.plan()
	rec := h.call(http.MethodPost, path, body)
	if rec.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута addLesson: %s", rec.Body.Bytes())
	}
	h.status(rec, http.StatusCreated)
	added := decode[lessonView](t, rec)
	if added.ID == "" || !added.Added || added.Title == "" || added.TemplateID == "" {
		t.Fatalf("добавленный урок %+v", added)
	}
	sameInstant(t, added.StartsAt, starts)
	switch added.State {
	case "planned", "frozen", "held", "skipped":
	default:
		t.Fatalf("состояние %s", added.State)
	}
	if slices.Contains([]string{"demo-l01", "demo-l02", "demo-l03"}, added.ID) {
		t.Fatalf("id совпал с уроком плана %s", added.ID)
	}

	after := h.plan()
	if len(after.Lessons) != 4 {
		t.Fatalf("уроков %d", len(after.Lessons))
	}
	for _, lesson := range before.Lessons {
		got := s4ByID(t, after, lesson.ID)
		if lesson.StartsAt > starts {
			sameInstant(t, got.StartsAt, s4NextSlot(t, lesson.StartsAt))
			continue
		}
		sameInstant(t, got.StartsAt, lesson.StartsAt)
	}
	sameInstant(t, s4ByID(t, after, added.ID).StartsAt, starts)
	if !s4ByID(t, after, added.ID).Added {
		t.Fatal("в плане урок не помечен добавленным")
	}
	kind, _ := s4TemplateKind(t, h, added.ID)
	if kind != "array" {
		t.Fatalf("template %s", kind)
	}
	h.expectNoDraft(added.ID)
	if got := s4CheckAdded(t, h.lastRaw("lesson_added"), subjectMath, starts); got != added.ID {
		t.Fatalf("событие %s, ответ %s", got, added.ID)
	}
	s4AllTemplatesAreArrays(t, h)
}

func TestLS_R5_S4_AddScenario(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(3),
	})
	// LS-R5 нет в lessons.examples.yaml. Это HTTP-форма правила для прогонщика.
	s4RunScenario(t, h, `
- id: lessons-add
  given: {}
  steps:
    - op: addLesson
      request: {subject: math, starts_at: "2026-11-03T18:00:00Z"}
      expect: {http: 201, body: {added: true}, journal: {lesson_added: 1}}
`)
}

func TestLS_R5_S4_TakenSlot(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(2),
	})
	before := h.plan()
	occupied := byPlace(t, before, 1).StartsAt
	rec := h.call(http.MethodPost, s4AddPath(h.world.StudentID), s4AddBody(subjectMath, occupied))
	if rec.Code == http.StatusCreated || rec.Code == http.StatusNotFound {
		t.Fatalf("занятый слот: код %d, тело %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusConflict {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.Bytes())
	}
	after := h.plan()
	assertOrder(t, after, len(before.Lessons))
	for _, lesson := range before.Lessons {
		got := s4ByID(t, after, lesson.ID)
		s4SameStarts(t, got.StartsAt, lesson.StartsAt)
		if got.PlaceNo != lesson.PlaceNo {
			t.Fatalf("место %s изменилось", lesson.ID)
		}
	}
	if h.eventCount("lesson_added") != 0 {
		t.Fatal("занятый слот записал lesson_added")
	}
}

func TestLS_R5_S4_ForeignStudent(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), OtherStudent: true, Lessons: plannedLessons(2),
	})
	body := s4AddBody(subjectMath, "2026-11-03T18:00:00Z")
	foreign := h.call(http.MethodPost, s4AddPath(h.world.OtherStudentID), body)
	missing := h.call(http.MethodPost, s4AddPath("demo-missing"), body)
	if foreign.Code == http.StatusNotFound && missing.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута addLesson")
	}
	h.expectCode(foreign, http.StatusForbidden, "forbidden")
	h.expectCode(missing, http.StatusForbidden, "forbidden")
	if foreign.Body.String() != missing.Body.String() {
		t.Fatalf("чужой и несуществующий различимы: %s / %s", foreign.Body.Bytes(), missing.Body.Bytes())
	}
	if h.eventCount("lesson_added") != 0 {
		t.Fatal("чужой запрос записал lesson_added")
	}
}

func TestLS_R6_S4_CancelNoHomework(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		Subject: subjectMath, CatalogVersion: 1, Schedule: anchorSchedule(),
		Lessons: plannedLessons(3),
	})
	before := h.plan()
	states := map[int]string{}
	for place := 1; place <= 3; place++ {
		states[place] = byPlace(t, before, place).State
	}
	rec := h.cancel(byPlace(t, before, 1).ID)
	h.status(rec, http.StatusOK)
	after := h.plan()
	assertOrder(t, after, 3)
	for place := 1; place <= 3; place++ {
		lesson := byPlace(t, after, place)
		if lesson.ID != byPlace(t, before, place).ID || lesson.State != states[place] {
			t.Fatalf("урок %d %+v, состояние было %s", place, lesson, states[place])
		}
	}
	sameInstant(t, byPlace(t, after, 1).StartsAt, "2026-11-05T18:00:00Z")
	sameInstant(t, byPlace(t, after, 2).StartsAt, "2026-11-09T18:00:00Z")
	sameInstant(t, byPlace(t, after, 3).StartsAt, "2026-11-12T18:00:00Z")
	if h.draftCount() != 0 {
		t.Fatal("после отмены появился черновик")
	}
	s4CheckCancelled(t, h.lastRaw("lesson_cancelled"))
}

func TestLS_R7_S4_SkipNotCounted(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		Subject: subjectMath, CatalogVersion: 1, Schedule: anchorSchedule(),
		Lessons: plannedLessons(4),
	})
	before := h.plan()
	var fit []warning
	for _, item := range before.Warnings {
		if item.Type == "plan_does_not_fit" {
			fit = append(fit, item)
		}
	}
	if len(fit) != 1 || !hasNumber(fit[0], 2) {
		t.Fatalf("до пропуска %+v", before.Warnings)
	}
	rec := h.call(http.MethodPost, "/lessons/demo-l03/skip", nil)
	if rec.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута skipLesson: %s", rec.Body.Bytes())
	}
	h.status(rec, http.StatusOK)
	after := h.plan()
	if s4ByID(t, after, "demo-l03").State != "skipped" {
		t.Fatalf("урок 3 %+v", s4ByID(t, after, "demo-l03"))
	}
	fit = nil
	for _, item := range after.Warnings {
		if item.Type == "plan_does_not_fit" {
			fit = append(fit, item)
		}
	}
	if len(fit) != 1 || !hasNumber(fit[0], 1) {
		t.Fatalf("пропущенный вошёл в недостачу %+v", after.Warnings)
	}
	s4WarningEnum(t, after.Warnings)
}

func TestLS_R8_S4_SkipDoesNotHold(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(4),
	})
	before := h.plan()
	rec := h.call(http.MethodPost, "/lessons/demo-l03/skip", nil)
	if rec.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута skipLesson: %s", rec.Body.Bytes())
	}
	h.status(rec, http.StatusOK)
	after := h.plan()
	skipped := s4ByID(t, after, "demo-l03")
	if skipped.State != "skipped" || skipped.StartsAt != "" {
		t.Fatalf("пропуск %+v", skipped)
	}
	if s4ByID(t, after, "demo-l01").State != s4ByID(t, before, "demo-l01").State {
		t.Fatal("пропуск изменил состояние другого урока")
	}
	if s4ByID(t, after, "demo-l04").State == "held" {
		t.Fatal("следующий урок стал held")
	}
}

func TestLS_R9_S4_FreezeTwo(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(4),
	})
	view := h.plan()
	if byPlace(t, view, 1).State != "frozen" || byPlace(t, view, 2).State != "frozen" {
		t.Fatalf("ближайшие %s %s", byPlace(t, view, 1).State, byPlace(t, view, 2).State)
	}
	if byPlace(t, view, 3).State == "frozen" || byPlace(t, view, 4).State == "frozen" {
		t.Fatal("заморожено больше двух")
	}
}

func TestLS_R14_S4_Composition(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{teachLesson(1)},
	})
	rec := h.call(http.MethodGet, "/lessons/demo-l01", nil)
	h.status(rec, http.StatusOK)
	s4ForbidAnswerKeys(t, rec.Body.Bytes())
	detail := decode[lessonDetail](t, rec)
	if detail.Lesson.ID != "demo-l01" || len(detail.Stages) == 0 {
		t.Fatalf("состав %+v", detail.Stages)
	}
	if !slices.Contains(blocksOf(detail.Stages), "demo:B1") {
		t.Fatalf("нет блока шаблона: %v", blocksOf(detail.Stages))
	}
}

func TestLS_R16_S4_MarkAfterEnd(t *testing.T) {
	now := time.Date(2026, 11, 2, 19, 1, 0, 0, time.UTC)
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{markedSample(), quietLesson(2)},
	})
	// Часы сервиса стоят после слота, поэтому раскладка этот слот уже не берёт.
	// В базе слот урока сдвигается в прошлое: занятие закончилось, отметки ставятся после него.
	past := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	if _, err := h.conn.Exec(context.Background(),
		`UPDATE core.lesson SET starts_at = $2 WHERE id = $1`, "demo-l01", past); err != nil {
		t.Fatal(err)
	}
	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 3))
	h.status(rec, http.StatusOK)
	detail := decode[lessonDetail](t, rec)
	if detail.Lesson.State != "held" {
		t.Fatalf("состояние %s", detail.Lesson.State)
	}
	s4CheckMarked(t, h.lastRaw("lesson_marked"), "demo-l01")
}

func TestLS_R17_S4_NothingNoDraft(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{teachLesson(1), quietLesson(2)},
	})
	rec := h.mark("demo-l01", marks("", "", 0))
	h.status(rec, http.StatusOK)
	if decode[lessonDetail](t, rec).Lesson.State != "held" {
		t.Fatal("урок не held")
	}
	if h.draftCount() != 0 {
		t.Fatal("при двух «нет» собрался черновик")
	}
	assertCarry(t, h.lesson("demo-l02"), "demo-l01", []string{"demo:B1"}, []string{"demo:V1"})
	s4CheckMarked(t, h.lastRaw("lesson_marked"), "demo-l01")
}

func TestLS_R18_S4_UnexplainedToNext(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{threeBlocks(), quietLesson(2)},
	})
	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 2))
	h.status(rec, http.StatusOK)
	assertCarry(t, h.lesson("demo-l02"), "demo-l01", []string{"demo:B3"}, []string{"demo:V3"})
}

func TestLS_R19_S4_MarkBeforeEnd(t *testing.T) {
	now := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC)
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{markedSample()},
	})
	sameInstant(t, h.plan().Lessons[0].StartsAt, "2026-11-02T18:00:00Z")
	before := h.lesson("demo-l01").Lesson.State
	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 3))
	h.expectCode(rec, http.StatusConflict, "lesson_not_held")
	if got := h.lesson("demo-l01").Lesson.State; got != before {
		t.Fatalf("состояние стало %s", got)
	}
	if h.eventCount("lesson_marked") != 0 {
		t.Fatal("отметка до начала занятия записана")
	}
}

func TestLS_R19_S4_ForeignStudent(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), OtherStudent: true, Lessons: []lessons.LessonSpec{teachLesson(1)},
	})
	rec := h.mark(h.world.OtherLessonID, marks("demo:B1", "demo:L01.P", 1))
	h.expectCode(rec, http.StatusForbidden, "forbidden")
	if h.eventCount("lesson_marked") != 0 {
		t.Fatal("отметка чужого ученика записана")
	}
}

func TestLS_R19a_S4_IssuedColumn(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{markedSample(), quietLesson(2)},
	})
	first := h.mark("demo-l01", marks("demo:B2", "demo:P", 2))
	h.status(first, http.StatusOK)
	if s4Issued(t, h, "demo-l01") {
		t.Fatal("отметка сама выставила core.lesson.issued")
	}
	events := h.eventCount("lesson_marked")
	s4SetIssued(t, h, "demo-l01", true)
	if !s4Issued(t, h, "demo-l01") {
		t.Fatal("колонка issued не стала true")
	}
	rec := h.mark("demo-l01", marks("demo:B3", "demo:P", 3))
	h.expectCode(rec, http.StatusConflict, "already_marked")
	if h.eventCount("lesson_marked") != events {
		t.Fatal("после выдачи записана новая отметка")
	}
	got := h.lesson("demo-l01")
	if got.Lesson.Marks == nil || got.Lesson.Marks.LastTheoryBlock == nil || *got.Lesson.Marks.LastTheoryBlock != "demo:B2" {
		t.Fatalf("теория изменилась: %+v", got.Lesson.Marks)
	}
}

func TestLS_R20_S4_CoverageAll(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	lesson := basicLesson(1)
	lesson.NoBlocks = true
	lesson.Stages = nil
	lesson.State = "held"
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Lessons: []lessons.LessonSpec{lesson, quietLesson(2)},
	})
	rec := h.mark("demo-l01", map[string]string{"coverage": "all"})
	h.status(rec, http.StatusOK)
	if h.draftCount() != 0 || h.eventCount("lesson_carried_over") != 0 {
		t.Fatal("урок без снимка собрал ДЗ или перенос")
	}
	s4CheckMarked(t, h.lastRaw("lesson_marked"), "demo-l01")
}

func TestLS_R21_S4_DraftAfterMarks(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{markedSample(), quietLesson(2)},
	})
	rec := h.mark("demo-l01", marks("demo:B3", "demo:P", 2))
	h.status(rec, http.StatusOK)
	ok, ids := h.draftIDs("demo-l01")
	if !ok || !slices.Contains(ids, "demo:V3") {
		t.Fatalf("черновик после отметок: %v", ids)
	}
}

func TestLS_R24_S4_Preparation(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), OtherStudent: true, Lessons: []lessons.LessonSpec{teachLesson(1)},
	})
	tutorLesson := h.call(http.MethodGet, "/lessons/demo-l01", nil)
	h.status(tutorLesson, http.StatusOK)
	s4ForbidAnswerKeys(t, tutorLesson.Body.Bytes())

	student := s4StudentToken(t, h)
	studentLesson := s4CallRaw(h, student, http.MethodGet, "/lessons/demo-l01", nil, true)
	h.expectCode(studentLesson, http.StatusForbidden, "forbidden")
	s4ForbidAnswerKeys(t, studentLesson.Body.Bytes())

	studentPrep := s4CallRaw(h, student, http.MethodGet, "/lessons/demo-l01/preparation", nil, true)
	if studentPrep.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута getPreparation: %s", studentPrep.Body.Bytes())
	}
	h.expectCode(studentPrep, http.StatusForbidden, "forbidden")
	s4ForbidAnswerKeys(t, studentPrep.Body.Bytes())

	foreign := h.call(http.MethodGet, "/lessons/"+h.world.OtherLessonID+"/preparation", nil)
	missing := h.call(http.MethodGet, "/lessons/demo-missing/preparation", nil)
	h.expectCode(foreign, http.StatusForbidden, "forbidden")
	h.expectCode(missing, http.StatusForbidden, "forbidden")
	if foreign.Body.String() != missing.Body.String() {
		t.Fatalf("чужой и несуществующий различимы: %s / %s", foreign.Body.Bytes(), missing.Body.Bytes())
	}

	rec := h.call(http.MethodGet, "/lessons/demo-l01/preparation", nil)
	h.status(rec, http.StatusOK)
	body := s4Object(t, rec.Body.Bytes())
	s4Only(t, body, "lesson_id", "items")
	s4Need(t, body, "lesson_id", "items")
	if body["lesson_id"] != "demo-l01" {
		t.Fatalf("lesson_id %v", body["lesson_id"])
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items %v", body["items"])
	}
	found := false
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("элемент %T", item)
		}
		s4Only(t, obj, "variant_id", "statement", "key", "solution_block", "options")
		s4Need(t, obj, "variant_id", "statement", "key")
		if obj["key"] == nil {
			t.Fatal("key равен null")
		}
		if s4NonEmpty(t, obj, "statement") == "" {
			t.Fatal("пустое условие")
		}
		if obj["variant_id"] == "demo:V1" {
			found = true
		}
	}
	if !found {
		t.Fatal("нет задачи demo:V1")
	}
}

func TestPL_R4a_S4_SkipShifts(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(4),
	})
	before := h.plan()
	if byPlace(t, before, 3).State == "frozen" || byPlace(t, before, 3).State == "held" {
		t.Fatalf("урок 3 %s", byPlace(t, before, 3).State)
	}
	rec := h.call(http.MethodPost, "/lessons/demo-l03/skip", nil)
	if rec.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута skipLesson: %s", rec.Body.Bytes())
	}
	h.status(rec, http.StatusOK)
	view := decode[planView](t, rec)
	skipped := s4ByID(t, view, "demo-l03")
	if skipped.State != "skipped" || skipped.StartsAt != "" {
		t.Fatalf("пропуск %+v", skipped)
	}
	sameInstant(t, s4ByID(t, view, "demo-l01").StartsAt, "2026-11-02T18:00:00Z")
	sameInstant(t, s4ByID(t, view, "demo-l02").StartsAt, "2026-11-05T18:00:00Z")
	sameInstant(t, s4ByID(t, view, "demo-l04").StartsAt, "2026-11-09T18:00:00Z")
	h.expectNoDraft("demo-l03")
	s4CheckSkipped(t, h.lastRaw("lesson_skipped"), "demo-l03")
	if h.eventCount("lesson_skipped") != 1 {
		t.Fatalf("событий %d", h.eventCount("lesson_skipped"))
	}
}

func TestPL_R4a_S4_SkipRepeat(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(4),
	})
	first := h.call(http.MethodPost, "/lessons/demo-l03/skip", nil)
	if first.Code == http.StatusNotFound {
		t.Fatalf("нет маршрута skipLesson: %s", first.Body.Bytes())
	}
	h.status(first, http.StatusOK)
	after := h.plan()
	second := h.call(http.MethodPost, "/lessons/demo-l03/skip", nil)
	if second.Code != http.StatusOK && second.Code != http.StatusConflict {
		t.Fatalf("повтор: код %d, тело %s", second.Code, second.Body.Bytes())
	}
	again := h.plan()
	for _, id := range []string{"demo-l01", "demo-l02", "demo-l03", "demo-l04"} {
		was := s4ByID(t, after, id)
		got := s4ByID(t, again, id)
		if got.State != was.State || got.PlaceNo != was.PlaceNo {
			t.Fatalf("повтор изменил %s: %+v", id, got)
		}
		s4SameStarts(t, got.StartsAt, was.StartsAt)
	}
	if h.eventCount("lesson_skipped") != 1 {
		t.Fatalf("повтор записал ещё одно событие: %d", h.eventCount("lesson_skipped"))
	}
}

func TestPL_R4a_S4_ForeignStudent(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), OtherStudent: true, Lessons: plannedLessons(3),
	})
	foreign := h.call(http.MethodPost, "/lessons/"+h.world.OtherLessonID+"/skip", nil)
	missing := h.call(http.MethodPost, "/lessons/demo-missing/skip", nil)
	if foreign.Code == http.StatusNotFound && missing.Code == http.StatusNotFound {
		t.Fatal("нет маршрута skipLesson")
	}
	h.expectCode(foreign, http.StatusForbidden, "forbidden")
	h.expectCode(missing, http.StatusForbidden, "forbidden")
	if foreign.Body.String() != missing.Body.String() {
		t.Fatalf("чужой и несуществующий различимы: %s / %s", foreign.Body.Bytes(), missing.Body.Bytes())
	}
	if h.eventCount("lesson_skipped") != 0 {
		t.Fatal("чужой пропуск записан")
	}
}

func TestPL_R4a_S4_Plan06Scenario(t *testing.T) {
	now := anchorNow()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: plannedLessons(4),
	})
	// plan-06 в plan.examples.yaml — проза без steps. Прогонщик получает ту же операцию.
	s4RunScenario(t, h, `
- id: plan-06
  given: {}
  steps:
    - op: skipLesson
      expect: {http: 200, journal: {lesson_skipped: 1}}
`)
}

func TestS4_TemplateArray(t *testing.T) {
	now := duringFirst()
	h := newHarness(t, now)
	h.seed(lessons.Fixture{
		Now: now, ExamDate: farExam(), Subject: subjectMath, CatalogVersion: 1,
		Schedule: anchorSchedule(), Lessons: []lessons.LessonSpec{markedSample(), quietLesson(2)},
	})
	s4AllTemplatesAreArrays(t, h)
	kind, first := s4TemplateKind(t, h, "demo-l01")
	if kind != "array" || first != "object" {
		t.Fatalf("template %s, первый этап %s", kind, first)
	}
	rec := h.mark("demo-l01", marks("demo:B2", "demo:P", 2))
	h.status(rec, http.StatusOK)
	s4AllTemplatesAreArrays(t, h)
}

func TestS4_BindExampleRows(t *testing.T) {
	lessonsRows := s4ExampleRows(t, "docs", "personalization", "lessons.examples.yaml")
	planRows := s4ExampleRows(t, "docs", "personalization", "plan.examples.yaml")
	var sawPlan06 bool
	for _, row := range append(lessonsRows, planRows...) {
		if row.Rule == "LS-R5" {
			t.Fatalf("в примерах появилась строка LS-R5 %s: её steps надо отдать прогонщику", row.ID)
		}
		if row.ID == "plan-06" {
			sawPlan06 = true
			if row.Rule != "PL-R4a" {
				t.Fatalf("plan-06 правило %s", row.Rule)
			}
		}
		if (row.Rule == "PL-R4a" || row.Rule == "LS-R5") && len(row.Steps) > 0 {
			t.Fatalf("%s получил steps: прогонщик должен читать их из файла", row.ID)
		}
	}
	if !sawPlan06 {
		t.Fatal("нет строки plan-06")
	}
}
