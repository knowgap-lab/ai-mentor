package conspect_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/conspect"
)

// conspect-01 … conspect-14. Плюс урок без снимка и блок без ревизии:
// коды lesson_not_frozen и block_missing из раздела 5.

func TestCN_R1_R5_R6_conspect01(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  8,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:A", "theory", "Блок А", "а-в6", 6),
			rev("demo:A", "theory", "Блок А", "а-в8", 8),
			rev("demo:B", "rule", "Блок Б", "б-в6", 6),
			rev("demo:B", "rule", "Блок Б", "б-в8", 8),
			rev("demo:only8", "theory", "Только 8", "только-в8", 8),
		},
		Lessons: []conspect.LessonSpec{{
			ID:       "demo-l01",
			PlaceNo:  1,
			Title:    "Урок 1",
			State:    "frozen",
			Practice: []string{"demo:Vsecret"},
			Stages: []conspect.StageSpec{
				{ID: "demo-l01.G1", Blocks: []string{"demo:A"}},
				{ID: "demo-l01.G2", Blocks: []string{"demo:B"}},
			},
		}},
	})

	got, rec := h.okConspect("demo-l01")
	if got.LessonID != "demo-l01" || got.CatalogVersion != 6 {
		t.Fatalf("урок %s версия %d", got.LessonID, got.CatalogVersion)
	}
	if len(got.CarriedOver) != 0 {
		t.Fatalf("перенос %+v", got.CarriedOver)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("блоков %d", len(got.Blocks))
	}
	assertPlain(t, got.Blocks[0], "demo:A", "theory", "Блок А", "а-в6")
	assertPlain(t, got.Blocks[1], "demo:B", "rule", "Блок Б", "б-в6")
	body := rec.Body.String()
	for _, leaked := range []string{"а-в8", "б-в8", "только-в8", "demo:Vsecret", "demo:only8"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("в ответе есть %q", leaked)
		}
	}
}

func TestCN_R2_conspect02(t *testing.T) {
	h := newHarness(t)
	widget := conspect.WidgetSpec{
		Widget: "fraction-bar",
		Params: map[string]any{"side": 3},
		Schema: sideSchema(),
	}
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:theory", "theory", "Теория", "текст-теории", 6),
			rev("demo:card", "flashcard", "Карточка", "карточка-не-выводить", 6),
			rev("demo:rule", "rule", "Правило", "текст-правила", 6),
			rev("demo:summary", "summary", "Итог", "текст-итога", 6),
			rev("demo:example", "worked_example", "Пример", "текст-примера", 6),
			rev("demo:error", "typical_error", "Ошибка", "текст-ошибки", 6),
			{ID: "demo:widget", Kind: "widget", Title: "Виджет", Markdown: "", Version: 6, Widget: &widget},
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:theory", "demo:card", "demo:rule", "demo:summary", "demo:example", "demo:error", "demo:widget"),
		},
	})

	got, rec := h.okConspect("demo-l01")
	if got.CatalogVersion != 6 {
		t.Fatalf("версия %d", got.CatalogVersion)
	}
	want := []struct {
		id, kind, title, markdown string
	}{
		{"demo:theory", "theory", "Теория", "текст-теории"},
		{"demo:rule", "rule", "Правило", "текст-правила"},
		{"demo:summary", "summary", "Итог", "текст-итога"},
		{"demo:example", "worked_example", "Пример", "текст-примера"},
		{"demo:error", "typical_error", "Ошибка", "текст-ошибки"},
	}
	if len(got.Blocks) != len(want)+1 {
		t.Fatalf("блоков %d: %+v", len(got.Blocks), got.Blocks)
	}
	for i, item := range want {
		assertPlain(t, got.Blocks[i], item.id, item.kind, item.title, item.markdown)
	}
	last := got.Blocks[len(want)]
	if last.ID != "demo:widget" || last.Kind != "widget" || last.Title != "Виджет" || last.Markdown != "" {
		t.Fatalf("виджет %+v", last)
	}
	if last.Widget == nil || last.Widget.Widget != "fraction-bar" {
		t.Fatalf("параметры виджета %+v", last.Widget)
	}
	side, sideOK := asInt(last.Widget.Params["side"])
	if !sideOK || side != 3 {
		t.Fatalf("параметр side %+v", last.Widget.Params)
	}
	if strings.Contains(rec.Body.String(), "карточка-не-выводить") || strings.Contains(rec.Body.String(), "demo:card") {
		t.Fatal("flashcard попал в ответ")
	}
}

func TestCN_R3_conspect03(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:C", "theory", "Перенос дроби", "текст-переноса", 6),
			rev("demo:own", "rule", "Своё правило", "текст-урока", 6),
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l03", 3, "demo:C"),
			frozenLesson("demo-l05", 5, "demo:own"),
		},
		Carry: []conspect.CarrySpec{{
			ToLessonID:   "demo-l05",
			FromLessonID: "demo-l03",
			Blocks:       []string{"demo:C"},
		}},
	})

	got, _ := h.okConspect("demo-l05")
	if got.CatalogVersion != 6 || got.LessonID != "demo-l05" {
		t.Fatalf("урок %s версия %d", got.LessonID, got.CatalogVersion)
	}
	if len(got.CarriedOver) != 1 {
		t.Fatalf("перенос %+v", got.CarriedOver)
	}
	assertPlain(t, got.CarriedOver[0], "demo:C", "theory", "Перенос дроби", "текст-переноса")
	if len(got.Blocks) != 1 {
		t.Fatalf("свои блоки %+v", got.Blocks)
	}
	assertPlain(t, got.Blocks[0], "demo:own", "rule", "Своё правило", "текст-урока")
}

func TestCN_R4_conspect04(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  8,
		SnapshotVersion: 6,
		Peer:            true,
		Blocks: []conspect.BlockRev{
			rev("demo:A", "theory", "Общий", "общий-в6", 6),
			rev("demo:A", "theory", "Общий", "общий-в8", 8),
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:A"),
		},
	})

	first, rawFirst := h.okConspect("demo-l01")
	second, rawSecond := h.okConspect(h.world.PeerLessonID)
	if first.LessonID != "demo-l01" || second.LessonID != h.world.PeerLessonID || first.LessonID == second.LessonID {
		t.Fatalf("уроки %s и %s", first.LessonID, second.LessonID)
	}
	if first.CatalogVersion != 6 || second.CatalogVersion != 6 {
		t.Fatalf("версии %d и %d", first.CatalogVersion, second.CatalogVersion)
	}
	if len(first.CarriedOver) != 0 || len(second.CarriedOver) != 0 {
		t.Fatalf("перенос %+v %+v", first.CarriedOver, second.CarriedOver)
	}
	if len(first.Blocks) != 1 || len(second.Blocks) != 1 {
		t.Fatalf("блоки %+v %+v", first.Blocks, second.Blocks)
	}
	assertPlain(t, first.Blocks[0], "demo:A", "theory", "Общий", "общий-в6")
	assertPlain(t, second.Blocks[0], "demo:A", "theory", "Общий", "общий-в6")
	for _, body := range []string{rawFirst.Body.String(), rawSecond.Body.String()} {
		if strings.Contains(body, "общий-в8") {
			t.Fatal("конспект собран по текущей версии")
		}
	}
}

func TestCN_R7_conspect05(t *testing.T) {
	const formula = `Дробь $\frac{a}{b}$ в тексте.`
	h := newHarness(t)
	h.seed(oneBlock("demo:frac", "theory", "Дробь", formula))

	got, rec := h.okConspect("demo-l01")
	if len(got.Blocks) != 1 {
		t.Fatalf("блоков %d", len(got.Blocks))
	}
	assertPlain(t, got.Blocks[0], "demo:frac", "theory", "Дробь", formula)
	if strings.Contains(rec.Body.String(), "<") {
		t.Fatalf("в ответе есть HTML: %s", rec.Body.Bytes())
	}
}

func TestCN_R7_conspect06(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:safe", "theory", "Чистый", "безопасный текст", 6),
			rev("demo:html", "theory", "Сырой", "видно <script>alert(1)</script> конец", 6),
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:safe", "demo:html"),
		},
	})

	got, rec := h.okConspect("demo-l01")
	if len(got.Blocks) != 1 {
		t.Fatalf("блоки %+v", got.Blocks)
	}
	assertPlain(t, got.Blocks[0], "demo:safe", "theory", "Чистый", "безопасный текст")
	body := rec.Body.String()
	if strings.Contains(body, "<script") || strings.Contains(body, "alert(1)") || strings.Contains(body, "demo:html") {
		t.Fatalf("сырой HTML попал в ответ: %s", body)
	}
}

func TestCN_R8_conspect07(t *testing.T) {
	bad := conspect.WidgetSpec{
		Widget: "foreign-widget",
		Params: map[string]any{"side": "длинная"},
		Schema: sideSchema(),
	}
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:keep", "theory", "Оставить", "текст-оставить", 6),
			{ID: "demo:bad", Kind: "widget", Title: "Чужой", Markdown: "виджет-мимо-схемы", Version: 6, Widget: &bad},
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:keep", "demo:bad"),
		},
	})

	got, rec := h.okConspect("demo-l01")
	if len(got.Blocks) != 1 {
		t.Fatalf("блоки %+v", got.Blocks)
	}
	assertPlain(t, got.Blocks[0], "demo:keep", "theory", "Оставить", "текст-оставить")
	body := rec.Body.String()
	if strings.Contains(body, "demo:bad") || strings.Contains(body, "длинная") || strings.Contains(body, "foreign-widget") {
		t.Fatalf("виджет не по схеме попал в ответ: %s", body)
	}
}

func TestCN_R10_conspect08(t *testing.T) {
	widget := conspect.WidgetSpec{
		Widget: "fraction-bar",
		Params: map[string]any{"side": 3},
		Schema: sideSchema(),
	}
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			{ID: "demo:widget", Kind: "widget", Title: "Виджет", Markdown: "", Version: 6, Widget: &widget},
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:widget"),
		},
	})

	h.expectCode(h.pdf("demo-l01"), http.StatusServiceUnavailable, "pdf_unavailable")
}

func TestCN_R11_conspect09(t *testing.T) {
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "текст без ключа"))
	h.expectCode(h.pdf("demo-l01"), http.StatusServiceUnavailable, "pdf_unavailable")
}

func TestCN_R12_conspect10(t *testing.T) {
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "один и тот же снимок"))
	h.expectCode(h.pdf("demo-l01"), http.StatusServiceUnavailable, "pdf_unavailable")
	h.expectCode(h.pdf("demo-l01"), http.StatusServiceUnavailable, "pdf_unavailable")
}

func TestCN_R13_conspect11(t *testing.T) {
	const file = "Файл урока про дроби. Знаменатель не равен нулю."
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 6,
		Lessons: []conspect.LessonSpec{{
			ID:         "demo-l01",
			PlaceNo:    1,
			Title:      "Урок 1",
			State:      "held",
			NoBlocks:   true,
			LessonFile: file,
		}},
	})

	tutor, _ := h.okConspect("demo-l01")
	if tutor.LessonID != "demo-l01" || tutor.CatalogVersion != 6 {
		t.Fatalf("репетитор: урок %s версия %d", tutor.LessonID, tutor.CatalogVersion)
	}
	if len(tutor.Blocks) != 1 || len(tutor.CarriedOver) != 0 {
		t.Fatalf("репетитор: блоки %+v перенос %+v", tutor.Blocks, tutor.CarriedOver)
	}
	if tutor.Blocks[0].ID == "" || tutor.Blocks[0].Kind == "" || tutor.Blocks[0].Markdown != file {
		t.Fatalf("файл урока %+v", tutor.Blocks[0])
	}

	h.as(h.world.StudentTelegramID, "student")
	rec := h.conspect("demo-l01")
	h.status(rec, http.StatusOK)
	student := decode[conspectView](t, rec)
	if student.LessonID != "demo-l01" {
		t.Fatalf("ученик: урок %s", student.LessonID)
	}
	if len(student.Blocks) != 0 || len(student.CarriedOver) != 0 {
		t.Fatalf("ученик видит %+v %+v", student.Blocks, student.CarriedOver)
	}
	if strings.Contains(rec.Body.String(), file) {
		t.Fatalf("ученик видит файл: %s", rec.Body.Bytes())
	}
}

func TestCN_R14_conspect12(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		OtherStudent:    true,
		Blocks:          []conspect.BlockRev{rev("demo:A", "theory", "Свой", "свой-текст", 6)},
		Lessons:         []conspect.LessonSpec{frozenLesson("demo-l01", 1, "demo:A")},
	})

	h.as(h.world.StudentTelegramID, "student")
	h.expectCode(h.conspect(h.world.OtherLessonID), http.StatusForbidden, "forbidden")
	h.expectCode(h.conspect("demo-missing"), http.StatusForbidden, "forbidden")
}

func TestCN_R14a_conspect13(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:A", "theory", "Первый", "текст-1", 6),
			rev("demo:B", "theory", "Второй", "текст-2", 6),
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:A", "demo:B"),
		},
	})

	tutor, _ := h.okConspect("demo-l01")
	if len(tutor.Blocks) != 2 {
		t.Fatalf("репетитор видит %+v", tutor.Blocks)
	}
	assertPlain(t, tutor.Blocks[0], "demo:A", "theory", "Первый", "текст-1")
	assertPlain(t, tutor.Blocks[1], "demo:B", "theory", "Второй", "текст-2")

	h.as(h.world.StudentTelegramID, "student")
	h.expectCode(h.conspect("demo-l01"), http.StatusConflict, "lesson_not_held")
}

func TestCN_R14a_conspect14(t *testing.T) {
	h := newHarness(t)
	lesson := frozenLesson("demo-l02", 2, "demo:B1", "demo:B2", "demo:B3", "demo:B4")
	lesson.State = "held"
	lesson.Mark = "demo:B2"
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:C", "theory", "Перенос", "текст-переноса", 6),
			rev("demo:B1", "theory", "Один", "текст-1", 6),
			rev("demo:B2", "rule", "Два", "текст-2", 6),
			rev("demo:B3", "summary", "Три", "текст-3", 6),
			rev("demo:B4", "worked_example", "Четыре", "текст-4", 6),
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:C"),
			lesson,
		},
		Carry: []conspect.CarrySpec{{
			ToLessonID:   "demo-l02",
			FromLessonID: "demo-l01",
			Blocks:       []string{"demo:C"},
		}},
	})

	h.as(h.world.StudentTelegramID, "student")
	student, studentRaw := h.okConspect("demo-l02")
	if len(student.CarriedOver) != 1 {
		t.Fatalf("перенос ученика %+v", student.CarriedOver)
	}
	assertPlain(t, student.CarriedOver[0], "demo:C", "theory", "Перенос", "текст-переноса")
	if len(student.Blocks) != 2 {
		t.Fatalf("блоки ученика %+v", student.Blocks)
	}
	assertPlain(t, student.Blocks[0], "demo:B1", "theory", "Один", "текст-1")
	assertPlain(t, student.Blocks[1], "demo:B2", "rule", "Два", "текст-2")
	studentBody := studentRaw.Body.String()
	if strings.Contains(studentBody, "текст-3") || strings.Contains(studentBody, "текст-4") {
		t.Fatal("ученик видит блоки после отметки")
	}

	h.as(h.world.TutorTelegramID, "tutor")
	tutor, _ := h.okConspect("demo-l02")
	if len(tutor.CarriedOver) != 1 || tutor.CarriedOver[0].ID != "demo:C" {
		t.Fatalf("перенос репетитора %+v", tutor.CarriedOver)
	}
	if len(tutor.Blocks) != 4 {
		t.Fatalf("репетитор видит %+v", tutor.Blocks)
	}
	assertPlain(t, tutor.Blocks[2], "demo:B3", "summary", "Три", "текст-3")
	assertPlain(t, tutor.Blocks[3], "demo:B4", "worked_example", "Четыре", "текст-4")
}

func TestCN_R1_LessonNotFrozen(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 6,
		Lessons: []conspect.LessonSpec{{
			ID:      "demo-l01",
			PlaceNo: 1,
			Title:   "Урок 1",
			State:   "planned",
		}},
	})

	h.expectCode(h.conspect("demo-l01"), http.StatusConflict, "lesson_not_frozen")
	h.as(h.world.StudentTelegramID, "student")
	h.expectCode(h.conspect("demo-l01"), http.StatusConflict, "lesson_not_frozen")
}

func TestCN_R1_BlockMissing(t *testing.T) {
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  8,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:keep", "theory", "Есть", "текст-в6", 6),
			rev("demo:gone", "theory", "Позже", "текст-в8", 8),
		},
		Lessons: []conspect.LessonSpec{
			frozenLesson("demo-l01", 1, "demo:keep", "demo:gone"),
		},
	})

	h.expectCode(h.conspect("demo-l01"), http.StatusInternalServerError, "block_missing")
}

func rev(id, kind, title, markdown string, version int) conspect.BlockRev {
	return conspect.BlockRev{
		ID:       id,
		Kind:     kind,
		Title:    title,
		Markdown: markdown,
		Version:  version,
	}
}

func frozenLesson(id string, place int, blockIDs ...string) conspect.LessonSpec {
	return conspect.LessonSpec{
		ID:      id,
		PlaceNo: place,
		Title:   fmt.Sprintf("Урок %d", place),
		State:   "frozen",
		Stages: []conspect.StageSpec{{
			ID:     id + ".G1",
			Blocks: blockIDs,
		}},
	}
}

func oneBlock(id, kind, title, markdown string) conspect.Fixture {
	return conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks:          []conspect.BlockRev{rev(id, kind, title, markdown, 6)},
		Lessons:         []conspect.LessonSpec{frozenLesson("demo-l01", 1, id)},
	}
}

func sideSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{
			"side",
		},
		"properties": map[string]any{
			"side": map[string]any{"type": "integer"},
		},
		"additionalProperties": false,
	}
}

func assertPlain(t *testing.T, got blockView, id, kind, title, markdown string) {
	t.Helper()
	if got.ID != id || got.Kind != kind || got.Title != title || got.Markdown != markdown || got.Widget != nil {
		t.Fatalf("блок %+v, нужен %s %s %q %q", got, id, kind, title, markdown)
	}
}

func asInt(value any) (int, bool) {
	switch n := value.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
