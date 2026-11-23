package homework_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/homework"
	"github.com/knowgap-lab/ai-mentor/core/internal/lessons"
)

// ВОПРОС: обе отметки «нет» (D274). Строки черновика нет, код GET не назван.
// Варианты: lesson_not_marked; lesson_cancelled; 200 с пустым списком. Не тестирую код.
//
// ВОПРОС: этап урока не хранит подвариант и условие. HW-R6 выбирает другой вариант
// того же подварианта. Варианты: брать подвариант из каталога; не собирать reinforcement,
// пока каталог не подключён. TestHW_R9_S5_LessonStages проверяет только, что массив
// этапов и отметки lesson_mark дают 200 и lesson_id.

func TestHW_R9_S5_SpacingAndD274(t *testing.T) {
	h := newHarness(t)
	const current = 17
	const skipped = 16
	want := []int{15, 13, 9, 1}
	control := 14
	topicPlaces := map[int]bool{15: true, 13: true, 9: true, 1: true, control: true}

	rows := make([]homework.Lesson, 0, current)
	var variants []homework.Variant
	for place := 1; place < current; place++ {
		lesson := homework.Lesson{
			ID:      fmt.Sprintf("s5-l-%02d", place),
			PlaceNo: place,
			State:   "held",
			Blocks:  []string{blockOne},
			Mark:    coverNone(),
		}
		if place == skipped {
			lesson.State = "skipped"
			lesson.Mark = nil
			lesson.Blocks = nil
		}
		if topicPlaces[place] {
			usedID := fmt.Sprintf("s5-used-%02d", place)
			topic := fmt.Sprintf("M%02d", place)
			lesson.Tasks = []homework.Task{task(1, usedID, "s5-t-used", "s5-sv-"+usedID)}
			lesson.Mark = coverSolved(1)
			used := variant(usedID, "s5-t-used", "s5-sv-"+usedID, topic, 1)
			used.Seen = true
			rev := variant(fmt.Sprintf("s5-rev-%02d", place), "s5-t-rev", fmt.Sprintf("s5-sv-rev-%02d", place), topic, 1)
			variants = append(variants, used, rev)
		}
		rows = append(rows, lesson)
	}
	rows = append(rows, homework.Lesson{
		ID:      "s5-l-17",
		PlaceNo: current,
		State:   "held",
		Blocks:  []string{blockOne},
		Recipe:  []homework.Series{{Source: "new_topic", TypeID: "s5-t-new", Count: 1}},
		Mark:    coverNone(),
	})
	variants = append(variants, variant("s5-new-01", "s5-t-new", "s5-sv-new", "M99", 1))
	h.seed(homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        rows,
		Variants:       variants,
	})

	body := h.mustDraft("s5-l-17")
	var fresh, review []string
	for _, item := range body.Items {
		switch item.Source {
		case "new_topic":
			fresh = append(fresh, item.VariantID)
		case "review":
			review = append(review, item.VariantID)
		}
	}
	if !sameStrings(fresh, []string{"s5-new-01"}) {
		t.Fatalf("new_topic %v", fresh)
	}
	for _, place := range want {
		id := fmt.Sprintf("s5-rev-%02d", place)
		if !containsString(review, id) {
			t.Fatalf("нет повторения %s в %v", id, review)
		}
	}
	if containsString(itemIDs(body.Items), "s5-rev-14") || containsString(itemIDs(body.Items), "s5-used-14") {
		t.Fatalf("тема двух уроков назад попала в ДЗ: %v", itemIDs(body.Items))
	}
}

func TestHW_R9_S5_LessonStages(t *testing.T) {
	h := newHarness(t)
	theory := blockOne
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	world, err := lessons.Seed(context.Background(), h.conn, lessons.Fixture{
		Now:            now,
		ExamDate:       time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons: []lessons.LessonSpec{{
			ID:         "s5-lesson",
			PlaceNo:    1,
			Title:      "Урок",
			Kind:       "new",
			TemplateID: "s5-tpl",
			State:      "held",
			Stages: []lessons.StageSpec{{
				ID:      "s5-stage",
				Title:   "Практика",
				Kind:    "practice",
				Minutes: [2]int{0, 40},
				Blocks:  []string{blockOne},
				Practice: []lessons.PracticeSpec{{
					VariantID:   "s5-used",
					TypeID:      "s5-type",
					Role:        "solve_alone",
					Position:    1,
					TheoryBlock: blockOne,
				}},
			}},
			Mark: &lessons.MarkSpec{
				Theory:   &theory,
				Practice: &lessons.PracticeRef{StageID: "s5-stage", Position: 1},
			},
		}},
	})
	if err != nil {
		t.Fatalf("уроки: %v", err)
	}
	h.token = login(t, h.dsn, world.ActorTelegramID, world.ActorRole)
	rec := h.getDraft("s5-lesson")
	h.status(rec, http.StatusOK)
	body := decode[draftView](t, rec)
	if body.LessonID != "s5-lesson" {
		t.Fatalf("lesson_id %s, тело %s", body.LessonID, rec.Body.Bytes())
	}
}

func TestLS_R19a_S5_IssueSetsLessonIssued(t *testing.T) {
	h := newHarness(t)
	h.seed(oneItemFixture("2026-11-02T18:00:00Z", "2026-11-09T18:00:00Z"))
	if s5LessonIssued(h, "hw-l1") {
		t.Fatal("до выдачи issued уже true")
	}
	h.mustDraft("hw-l1")
	rec := h.issue("hw-l1")
	h.status(rec, http.StatusCreated)
	if !s5LessonIssued(h, "hw-l1") {
		t.Fatal("выдача не поставила core.lesson.issued")
	}
}

func s5LessonIssued(h *harness, lessonID string) bool {
	h.t.Helper()
	var issued bool
	if err := h.conn.QueryRow(context.Background(), `
		SELECT issued FROM core.lesson WHERE id = $1
	`, lessonID).Scan(&issued); err != nil {
		h.t.Fatal(err)
	}
	return issued
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
