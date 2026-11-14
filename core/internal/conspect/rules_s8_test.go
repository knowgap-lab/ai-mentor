package conspect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/conspect"
	"github.com/knowgap-lab/ai-mentor/core/internal/gotenbergtest"
)

// ВОПРОС: CN-R3. В переносе нет номера урока-источника, в Conspect нет поля
// заголовка, additionalProperties: false. Поле мимо контракта не добавляется.
// Варианты: номер только в тексте PDF; не показывать «Перенесено с урока N»,
// пока в контракте нет поля. Этот случай не проверяется.
//
// ВОПРОС: имя переменной адреса Gotenberg не записано в deploy/secrets.md и
// deploy/README.md. Тест его не выбирает. Варианты: одно имя в одном из этих
// файлов, его прочитает gotenbergtest.EnvName.
//
// ВОПРОС: CN-R13. Уроки пишут core.lesson.template массивом этапов, при
// no_blocks это []. В этапе нет текста файла урока. PDF запасного пути идёт
// через Seed.LessonFile, как замороженный JSON. Варианты: текст лежит в другом
// месте; пустой массив — PDF без этого текста. Место текста не выбирается.
//
// ВОПРОС: D301, даты в метаданных PDF — время снимка. У снимка нет своей метки,
// у lesson_frozen есть at. Как дата попадает в документ, не задано. Варианты:
// поле запроса Gotenberg; правка PDF после ответа. Не проверяется.
// Версия шаблона в ключе хранения тоже не задана. Смена ключа не проверяется.

const (
	s8WidgetText = "Текст виджета выдуманный."
	s8TheoryText = "Теория выдуманных дробей."
	s8LessonName = "Тема выдуманных дробей"
	s8FileText   = "Файл урока про выдуманные дроби. Знаменатель не ноль."
	s8KeyText    = "ключ-конспекта-5519"
	s8PupilName  = "Выдуманный Ученик С8"
	s8Formula    = `Дробь $\frac{a}{b}$ в выдуманном тексте.`
)

func TestCN_R10_S8_WidgetPdf(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	s8UseGotenberg(t, stub.URL)
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
			rev("demo:theory", "theory", "Теория", s8TheoryText, 6),
			{ID: "demo:widget", Kind: "widget", Title: "Виджет", Markdown: s8WidgetText, Version: 6, Widget: &widget},
			rev("demo:frac", "theory", "Дробь", s8Formula, 6),
		},
		Lessons: []conspect.LessonSpec{{
			ID:      "demo-l17",
			PlaceNo: 17,
			Title:   s8LessonName,
			State:   "frozen",
			Stages: []conspect.StageSpec{{
				ID:     "demo-l17.G1",
				Blocks: []string{"demo:theory", "demo:widget", "demo:frac"},
			}},
		}},
	})

	body := s8PDF(t, h.pdf("demo-l17"))
	html := s8HTML(t, stub)
	s8NoExternalURL(t, html)
	for _, want := range []string{s8TheoryText, s8WidgetText, "fraction-bar", "frac"} {
		if !strings.Contains(html, want) {
			t.Fatalf("в HTML нет %q", want)
		}
	}
	lower := strings.ToLower(html)
	if !strings.Contains(lower, "<img") && !strings.Contains(lower, "<svg") {
		t.Fatal("на месте виджета нет статичной картинки")
	}
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		t.Fatalf("PDF не начинается с %%PDF-: %q", body[:min(16, len(body))])
	}
}

func TestCN_R11_S8_HeaderWithoutSecrets(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	s8UseGotenberg(t, stub.URL)
	h := newHarness(t)
	h.seed(conspect.Fixture{
		Subject:         subjectMath,
		CatalogVersion:  6,
		SnapshotVersion: 6,
		Blocks: []conspect.BlockRev{
			rev("demo:theory", "theory", "Теория", s8TheoryText, 6),
			rev("demo:example", "worked_example", "Пример", "Разобранный пример выдуман.", 6),
		},
		Lessons: []conspect.LessonSpec{{
			ID:       "demo-l17",
			PlaceNo:  17,
			Title:    s8LessonName,
			State:    "frozen",
			Practice: []string{"demo:Vsecret"},
			Stages: []conspect.StageSpec{{
				ID:     "demo-l17.G1",
				Blocks: []string{"demo:theory", "demo:example"},
			}},
		}},
	})
	s8InsertVariant(t, h, "demo:Vsecret", "Условие скрытого варианта.", s8KeyText)
	s8InsertPupilName(t, h)

	body := s8PDF(t, h.pdf("demo-l17"))
	html := s8HTML(t, stub)
	for _, want := range []string{subjectMath, "17", s8LessonName, s8TheoryText, "Разобранный пример выдуман."} {
		if !strings.Contains(html, want) {
			t.Fatalf("в заголовке или блоках нет %q", want)
		}
	}
	for _, secret := range []string{s8KeyText, s8PupilName, h.world.StudentTelegramID} {
		if strings.Contains(html, secret) || bytes.Contains(body, []byte(secret)) {
			t.Fatalf("в PDF есть %q", secret)
		}
	}
}

func TestCN_R12_S8_SameBytes(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	s8UseGotenberg(t, stub.URL)
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "один и тот же снимок"))

	first := s8PDF(t, h.pdf("demo-l01"))
	second := s8PDF(t, h.pdf("demo-l01"))
	if !bytes.Equal(first, second) {
		t.Fatal("два запроса одного снимка вернули разные байты")
	}
	if !bytes.HasPrefix(first, []byte("%PDF-")) {
		t.Fatal("ответ не начинается с %PDF-")
	}
	bodies := stub.Bodies()
	if len(bodies) == 0 {
		t.Fatal("Gotenberg не получил документ")
	}
	if len(bodies) >= 2 && !bytes.Equal(bodies[0], bodies[1]) && bytes.Equal(first, second) {
		return
	}
	if len(bodies) >= 2 && bytes.Equal(bodies[0], bodies[1]) {
		t.Fatal("подставной сервер вернул одинаковые PDF, хранение первого не проверено")
	}
}

func TestCN_R13_S8_LessonFileTutorOnly(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	s8UseGotenberg(t, stub.URL)
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
			LessonFile: s8FileText,
		}},
	})

	_ = s8PDF(t, h.pdf("demo-l01"))
	if !strings.Contains(s8HTML(t, stub), s8FileText) {
		t.Fatal("репетитор не видит файл урока")
	}
	before := len(stub.Bodies())
	h.as(h.world.StudentTelegramID, "student")
	student := h.pdf("demo-l01")
	s8PDF(t, student)
	if strings.Contains(student.Body.String(), s8FileText) {
		t.Fatal("ученик видит файл урока")
	}
	for _, body := range stub.Bodies()[before:] {
		if strings.Contains(string(body), s8FileText) {
			t.Fatal("ученику ушёл файл урока")
		}
	}
}

func TestCN_R14_S8_ForeignStudentAndTutor(t *testing.T) {
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
	h.expectCode(h.pdf(h.world.OtherLessonID), http.StatusForbidden, "forbidden")
	h.expectCode(h.pdf("demo-missing"), http.StatusForbidden, "forbidden")

	h.as(h.world.TutorTelegramID, "tutor")
	h.expectCode(h.pdf(h.world.OtherLessonID), http.StatusForbidden, "forbidden")
	h.expectCode(h.pdf("demo-missing"), http.StatusForbidden, "forbidden")
}

func TestS8_ConspectPdfUnauthorized(t *testing.T) {
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "текст"))
	h.token = ""
	h.expectCode(h.pdf("demo-l01"), http.StatusUnauthorized, "unauthorized")
}

func TestS8_PdfUnavailableUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	s8UseGotenberg(t, "http://"+addr)
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "текст"))
	h.expectCode(h.pdf("demo-l01"), http.StatusServiceUnavailable, "pdf_unavailable")
}

func TestS8_PdfUnavailableUpstream(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	stub.Fail(http.StatusInternalServerError)
	s8UseGotenberg(t, stub.URL)
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "текст"))
	h.expectCode(h.pdf("demo-l01"), http.StatusServiceUnavailable, "pdf_unavailable")
	if len(stub.Bodies()) == 0 {
		t.Fatal("запрос к Gotenberg не ушёл")
	}
}

func TestCN_R10_S8_GotenbergContainer(t *testing.T) {
	base := gotenbergtest.Start(t)
	s8UseGotenberg(t, base)
	h := newHarness(t)
	h.seed(oneBlock("demo:A", "theory", "Тема", "настоящий сервис"))
	body := s8PDF(t, h.pdf("demo-l01"))
	again := s8PDF(t, h.pdf("demo-l01"))
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		t.Fatal("ответ не начинается с %PDF-")
	}
	if !bytes.Equal(body, again) {
		t.Fatal("два запроса вернули разные байты")
	}
}

func s8UseGotenberg(t *testing.T, rawURL string) {
	t.Helper()
	name, err := gotenbergtest.EnvName()
	if err != nil {
		t.Fatalf("ВОПРОС: %s; варианты: записать одно имя в deploy/secrets.md или deploy/README.md", err)
	}
	t.Setenv(name, rawURL)
}

func s8PDF(t *testing.T, rec *httptest.ResponseRecorder) []byte {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("статус %d, тело %s", rec.Code, rec.Body.Bytes())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/pdf") {
		t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
	}
	body := append([]byte(nil), rec.Body.Bytes()...)
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		t.Fatalf("нет %%PDF-: %q", body[:min(24, len(body))])
	}
	return body
}

func s8HTML(t *testing.T, stub *gotenbergtest.Stub) string {
	t.Helper()
	bodies := stub.Bodies()
	if len(bodies) == 0 {
		t.Fatal("Gotenberg не получил документ")
	}
	return string(bodies[0])
}

func s8NoExternalURL(t *testing.T, html string) {
	t.Helper()
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Fatal("в HTML есть ссылка http:// или https://")
	}
}

func s8InsertVariant(t *testing.T, h *harness, id, statement, key string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id":           id,
		"subvariant":   id + ".1",
		"statement":    statement,
		"key":          key,
		"minutes":      5,
		"theory_block": "demo:theory",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.conn.Exec(context.Background(), `
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, 6, 's8-hash', $2::jsonb)
	`, id, string(raw)); err != nil {
		t.Fatalf("ревизия: %v", err)
	}
}

func s8InsertPupilName(t *testing.T, h *harness) {
	t.Helper()
	var userID string
	err := h.conn.QueryRow(context.Background(), `
		SELECT user_id FROM core.user_pii WHERE telegram_id = $1
	`, h.world.StudentTelegramID).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.conn.Exec(context.Background(), `
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, $2, $3)
	`, userID, s8PupilName, h.world.StudentTelegramID); err != nil {
		t.Fatalf("имя: %v", err)
	}
}
