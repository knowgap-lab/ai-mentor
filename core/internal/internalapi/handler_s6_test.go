package internalapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/internalapi"
	"github.com/knowgap-lab/ai-mentor/core/internal/recheck"
)

// HTTP четырёх операций internalapi.Handler и отчёта recheck.Handler.
// Обработчик внутреннего API токен не проверяет и cookie не читает.
// Статус отказа по токену эти тесты не закрепляют.
//
// ВОПРОС: когда announceCatalogVersion отвечает 409 stale_version. Варианты: номер меньше уже принятого; номер с пропуском. Не выбираю. 409 не тестирую.
// ВОПРОС: importPlan для несуществующего ученика. Кода в контракте нет. Варианты: 403 forbidden; 422. Не выбираю код. Тест проверяет только отсутствие строк.
// ВОПРОС: LP-R13 --dry-run нет в PlanImportRequest. Варианты: не тестировать HTTP; добавить поле вне контракта. Не выбираю. dry-run не тестирую.
// ВОПРОС: LP-R6 требует phase, source_path, source_hash урока и review, колонок под них нет. Варианты: template jsonb; миграция. Не выбираю. Эти поля не сверяю.

func TestLD_R11_S6_loader08_Announce(t *testing.T) {
	conn, h := s6Open(t)
	rec := s6Call(h, http.MethodPost, "/internal/catalog-versions", s6JSON(t, map[string]any{
		"catalog_version": 8,
		"commit":          "commit-s6",
	}), nil)
	s6Status(t, rec, http.StatusOK)
	var body struct {
		CatalogVersion int  `json:"catalog_version"`
		New            bool `json:"new"`
	}
	s6Decode(t, rec.Body.Bytes(), &body)
	if body.CatalogVersion != 8 || !body.New {
		t.Fatalf("тело %+v", body)
	}
	if n := s6EventCount(t, conn, "catalog_version_loaded"); n != 1 {
		t.Fatalf("событий %d", n)
	}
}

func TestLD_R13_S6_loader09_RepeatAnnounce(t *testing.T) {
	conn, h := s6Open(t)
	body := s6JSON(t, map[string]any{"catalog_version": 8})
	first := s6Call(h, http.MethodPost, "/internal/catalog-versions", body, nil)
	s6Status(t, first, http.StatusOK)
	second := s6Call(h, http.MethodPost, "/internal/catalog-versions", body, nil)
	s6Status(t, second, http.StatusOK)
	var again struct {
		New bool `json:"new"`
	}
	s6Decode(t, second.Body.Bytes(), &again)
	if again.New {
		t.Fatal("повтор той же версии создал новую")
	}
	if n := s6EventCount(t, conn, "catalog_version_loaded"); n != 1 {
		t.Fatalf("событий %d", n)
	}
}

func TestRC_R1_S6_HandlerIgnoresTokenAndCookie(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	payload := s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant},
	})
	cookie := &http.Cookie{Name: "__Host-session", Value: "ab"}
	withCookie := s6Call(h, http.MethodPost, "/internal/rechecks", payload, cookie)
	without := s6Call(h, http.MethodPost, "/internal/rechecks", payload, nil)
	s6Status(t, withCookie, http.StatusAccepted)
	s6Status(t, without, http.StatusAccepted)
	if withCookie.Body.String() != without.Body.String() {
		t.Fatalf("cookie изменил ответ: %s / %s", withCookie.Body.Bytes(), without.Body.Bytes())
	}
}

func TestRC_R2_S6_RepeatSameCorrection(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	payload := s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant},
	})
	first := s6Call(h, http.MethodPost, "/internal/rechecks", payload, nil)
	second := s6Call(h, http.MethodPost, "/internal/rechecks", payload, nil)
	s6Status(t, first, http.StatusAccepted)
	s6Status(t, second, http.StatusAccepted)
	if first.Body.String() != second.Body.String() {
		t.Fatalf("повтор %s, первый %s", second.Body.Bytes(), first.Body.Bytes())
	}
	if s6BatchCount(t, conn) != 1 {
		t.Fatal("повтор создал второй пакет")
	}
}

func TestRC_R3_S6_SmallerCorrection(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	s6PutVariant(t, conn, 9, "active")
	s6Status(t, s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 9,
		"variants":      []string{s6Variant},
	}), nil), http.StatusAccepted)
	rec := s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant},
	}), nil)
	s6Status(t, rec, http.StatusConflict)
	if s6ErrCode(t, rec) != "stale_correction" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
}

func TestRC_R3_S6_LargerCorrection(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	s6PutVariant(t, conn, 10, "active")
	s6Status(t, s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant},
	}), nil), http.StatusAccepted)
	rec := s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 10,
		"variants":      []string{s6Variant},
	}), nil)
	s6Status(t, rec, http.StatusAccepted)
	var body struct {
		CorrectionID int    `json:"correction_id"`
		State        string `json:"state"`
	}
	s6Decode(t, rec.Body.Bytes(), &body)
	if body.CorrectionID != 10 || body.State != "pending" {
		t.Fatalf("тело %+v", body)
	}
}

func TestRC_R4_S6_UnknownVariant(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	rec := s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant, "ct-demo:missing"},
	}), nil)
	s6Status(t, rec, http.StatusUnprocessableEntity)
	if s6ErrCode(t, rec) != "variant_unknown" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
	if s6BatchCount(t, conn) != 0 {
		t.Fatal("неизвестный вариант принял пакет")
	}
}

func TestRC_R5_S6_AcceptedPending(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	rec := s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant},
	}), nil)
	s6Status(t, rec, http.StatusAccepted)
	var body struct {
		State string `json:"state"`
	}
	s6Decode(t, rec.Body.Bytes(), &body)
	if body.State != "pending" {
		t.Fatalf("state %s", body.State)
	}
}

func TestRC_R6_S6_GetState(t *testing.T) {
	conn, h := s6Open(t)
	s6PutVariant(t, conn, 8, "active")
	s6Status(t, s6Call(h, http.MethodPost, "/internal/rechecks", s6JSON(t, map[string]any{
		"correction_id": 8,
		"variants":      []string{s6Variant},
	}), nil), http.StatusAccepted)
	rec := s6Call(h, http.MethodGet, "/internal/rechecks/8", nil, nil)
	s6Status(t, rec, http.StatusOK)
	var body struct {
		CorrectionID int    `json:"correction_id"`
		State        string `json:"state"`
	}
	s6Decode(t, rec.Body.Bytes(), &body)
	if body.CorrectionID != 8 || body.State == "" {
		t.Fatalf("тело %+v", body)
	}
	missing := s6Call(h, http.MethodGet, "/internal/rechecks/99", nil, nil)
	s6Status(t, missing, http.StatusNotFound)
	if s6ErrCode(t, missing) != "not_found" {
		t.Fatalf("код %s", missing.Body.Bytes())
	}
}

func TestLP_R6_S6_planimport05(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{
		s6Lesson(7, "2026-11-23", "new", []string{"M10"}, []string{"M04", "M08", "M06", "M09"}),
	})), nil)
	s6Status(t, rec, http.StatusCreated)
	var body struct {
		Lessons int `json:"lessons"`
	}
	s6Decode(t, rec.Body.Bytes(), &body)
	if body.Lessons != 1 {
		t.Fatalf("lessons %d", body.Lessons)
	}
	var place int
	var week, kind, title string
	err := conn.QueryRow(context.Background(), `
		SELECT place_no, planned_week, kind, title
		FROM core.lesson
		WHERE student_id = 's6-student' AND subject = 'math'
	`).Scan(&place, &week, &kind, &title)
	if err != nil {
		t.Fatal(err)
	}
	if place != 7 || week != "2026-11-23" || kind != "new" || title != "Выдуманный урок" {
		t.Fatalf("урок %d %s %s %s", place, week, kind, title)
	}
	var topic string
	if err := conn.QueryRow(context.Background(), `
		SELECT topic FROM core.lesson_topic lt
		JOIN core.lesson l ON l.id = lt.lesson_id
		WHERE l.student_id = 's6-student' AND lt.position = 1
	`).Scan(&topic); err != nil {
		t.Fatal(err)
	}
	if topic != "M10" {
		t.Fatalf("тема %s", topic)
	}
	var hashes string
	if err := conn.QueryRow(context.Background(), `
		SELECT source_hashes::text FROM core.plan_snapshot
		WHERE student_id = 's6-student' AND subject = 'math' AND version = 0
	`).Scan(&hashes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hashes, "lessons/s6.md") {
		t.Fatalf("снимок %s", hashes)
	}
}

func TestLP_R7_S6_planimport06(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lesson := s6Lesson(1, "2026-11-16", "seminar", []string{"M01"}, []string{})
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{lesson})), nil)
	s6Status(t, rec, http.StatusUnprocessableEntity)
	if s6ErrCode(t, rec) != "kind_unknown" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
	s6ExpectNoLessons(t, conn, "s6-student")
}

func TestLP_R8_S6_OrdinaryLessons(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{
		s6Lesson(1, "2026-11-16", "diagnostic", []string{"M01"}, []string{}),
		s6Lesson(2, "2026-11-23", "mock", []string{"M02"}, []string{"M01"}),
	})), nil)
	s6Status(t, rec, http.StatusCreated)
	var kinds string
	if err := conn.QueryRow(context.Background(), `
		SELECT string_agg(kind, ',' ORDER BY place_no) FROM core.lesson WHERE student_id = 's6-student'
	`).Scan(&kinds); err != nil {
		t.Fatal(err)
	}
	if kinds != "diagnostic,mock" {
		t.Fatalf("виды %s", kinds)
	}
	var assignments int
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.assignment WHERE student_id = 's6-student'
	`).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if assignments != 0 {
		t.Fatalf("выдач %d", assignments)
	}
}

func TestLP_R9_S6_planimport07(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lessons := []map[string]any{
		s6Lesson(1, "2026-10-12", "new", []string{"M01"}, []string{}),
		s6Lesson(2, "2026-10-19", "new", []string{"M02"}, []string{"M01"}),
		s6Lesson(3, "2026-10-26", "review", []string{"M03"}, []string{"M05"}),
		s6Lesson(4, "2026-11-02", "new", []string{"M04"}, []string{"M01"}),
		s6Lesson(5, "2026-11-09", "new", []string{"M05"}, []string{"M02"}),
	}
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, lessons)), nil)
	s6Status(t, rec, http.StatusUnprocessableEntity)
	if s6ErrCode(t, rec) != "review_before_intro" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
	s6ExpectNoLessons(t, conn, "s6-student")
}

func TestLP_R10_S6_planimport08_Gap(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lessons := []map[string]any{
		s6Lesson(1, "2026-11-02", "new", []string{"M01"}, []string{}),
		s6Lesson(2, "2026-11-09", "new", []string{"M02"}, []string{}),
		s6Lesson(4, "2026-11-16", "new", []string{"M04"}, []string{}),
	}
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, lessons)), nil)
	s6Status(t, rec, http.StatusUnprocessableEntity)
	if s6ErrCode(t, rec) != "lesson_gap" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
	s6ExpectNoLessons(t, conn, "s6-student")
}

func TestLP_R10_S6_planimport09_NotMonday(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lesson := s6Lesson(1, "2026-11-24", "new", []string{"M01"}, []string{})
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{lesson})), nil)
	s6Status(t, rec, http.StatusUnprocessableEntity)
	if s6ErrCode(t, rec) != "week_not_monday" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
	s6ExpectNoLessons(t, conn, "s6-student")
}

func TestLP_R10_S6_WeekNotIncreasing(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lessons := []map[string]any{
		s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{}),
		s6Lesson(2, "2026-11-09", "new", []string{"M02"}, []string{}),
	}
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, lessons)), nil)
	s6Status(t, rec, http.StatusUnprocessableEntity)
	if s6ErrCode(t, rec) != "week_not_increasing" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
	s6ExpectNoLessons(t, conn, "s6-student")
}

func TestLP_R11_S6_planimport10(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lesson := s6Lesson(1, "2026-10-12", "new", []string{"M01"}, []string{})
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 4, []map[string]any{lesson})), nil)
	s6Status(t, rec, http.StatusCreated)
	var week string
	if err := conn.QueryRow(context.Background(), `
		SELECT planned_week FROM core.lesson WHERE student_id = 's6-student'
	`).Scan(&week); err != nil {
		t.Fatal(err)
	}
	if week != "2026-11-09" {
		t.Fatalf("planned_week %s", week)
	}
}

func TestLP_R12_S6_NoClockFields(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lesson := s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{})
	s6Status(t, s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{lesson})), nil), http.StatusCreated)
	var starts *time.Time
	if err := conn.QueryRow(context.Background(), `
		SELECT starts_at FROM core.lesson WHERE student_id = 's6-student'
	`).Scan(&starts); err != nil {
		t.Fatal(err)
	}
	if starts != nil {
		t.Fatalf("starts_at %v", starts)
	}
	var rules int
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.schedule_rule WHERE student_id = 's6-student'
	`).Scan(&rules); err != nil {
		t.Fatal(err)
	}
	if rules != 0 {
		t.Fatalf("правил расписания %d", rules)
	}
}

func TestLP_R13_S6_planimport11(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	body := s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{
		s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{}),
	}))
	s6Status(t, s6Call(h, http.MethodPost, "/internal/plan-imports", body, nil), http.StatusCreated)
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", body, nil)
	s6Status(t, rec, http.StatusConflict)
	if s6ErrCode(t, rec) != "already_imported" {
		t.Fatalf("код %s", rec.Body.Bytes())
	}
}

func TestLP_R14_S6_SecondImportKeepsRows(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	first := s6Plan("s6-student", "math", 0, []map[string]any{
		s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{}),
	})
	s6Status(t, s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, first), nil), http.StatusCreated)
	changed := s6Lesson(1, "2026-11-23", "review", []string{"M09"}, []string{})
	changed["title"] = "Другое название"
	s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{changed})), nil)
	var title, week string
	if err := conn.QueryRow(context.Background(), `
		SELECT title, planned_week FROM core.lesson WHERE student_id = 's6-student'
	`).Scan(&title, &week); err != nil {
		t.Fatal(err)
	}
	if title != "Выдуманный урок" || week != "2026-11-16" {
		t.Fatalf("план изменился: %s %s", title, week)
	}
}

func TestLP_R15_S6_SnapshotImmutable(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	body := s6JSON(t, s6Plan("s6-student", "math", 0, []map[string]any{
		s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{}),
	}))
	s6Status(t, s6Call(h, http.MethodPost, "/internal/plan-imports", body, nil), http.StatusCreated)
	var before string
	if err := conn.QueryRow(context.Background(), `
		SELECT source_hashes::text FROM core.plan_snapshot
		WHERE student_id = 's6-student' AND subject = 'math' AND version = 0
	`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Exec(context.Background(), `
		UPDATE core.plan_snapshot SET source_hashes = '{"lessons/s6.md":"ffff"}'::jsonb
		WHERE student_id = 's6-student'
	`)
	var after string
	if err := conn.QueryRow(context.Background(), `
		SELECT source_hashes::text FROM core.plan_snapshot
		WHERE student_id = 's6-student' AND subject = 'math' AND version = 0
	`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("снимок изменился: %s", after)
	}
}

func TestLP_R16_S6_Atomic(t *testing.T) {
	conn, h := s6Open(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	lessons := []map[string]any{
		s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{}),
		s6Lesson(2, "2026-11-23", "seminar", []string{"M02"}, []string{}),
	}
	rec := s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-student", "math", 0, lessons)), nil)
	if rec.Code == http.StatusCreated {
		t.Fatalf("частичный импорт принят: %s", rec.Body.Bytes())
	}
	s6ExpectNoLessons(t, conn, "s6-student")
	var snaps int
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.plan_snapshot WHERE student_id = 's6-student'
	`).Scan(&snaps); err != nil {
		t.Fatal(err)
	}
	if snaps != 0 {
		t.Fatalf("снимков %d", snaps)
	}
}

func TestLP_R6_S6_MissingStudentWritesNothing(t *testing.T) {
	conn, h := s6Open(t)
	lesson := s6Lesson(1, "2026-11-16", "new", []string{"M01"}, []string{})
	s6Call(h, http.MethodPost, "/internal/plan-imports", s6JSON(t, s6Plan("s6-missing", "math", 0, []map[string]any{lesson})), nil)
	s6ExpectNoLessons(t, conn, "s6-missing")
}

func TestRC_R20_S6_ReportForeignTutorAndStudent(t *testing.T) {
	conn := s6DB(t)
	s6PutStudent(t, conn, "s6-tutor", "s6-student")
	s6PutStudent(t, conn, "s6-other", "s6-other-student")
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ('s6-student', 'S6NAME', 'tg-s6-student')
	`); err != nil {
		t.Fatal(err)
	}
	s6PutVariant(t, conn, 8, "active")
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.recheck_batch (
			correction_id, state, total, processed, changed, skipped_manual, variants
		) VALUES (8, 'done', 1, 1, 1, 1, ARRAY[$1]::text[])
	`, s6Variant); err != nil {
		t.Fatal(err)
	}
	owner := s6Session(t, conn, "s6-tutor")
	other := s6Session(t, conn, "s6-other")
	student := s6Session(t, conn, "s6-student")
	h := recheck.Handler()
	own := s6Call(h, http.MethodGet, "/rechecks/8", nil, owner)
	s6Status(t, own, http.StatusOK)
	if bytes.Contains(own.Body.Bytes(), []byte("S6NAME")) || bytes.Contains(own.Body.Bytes(), []byte(`"key"`)) || bytes.Contains(own.Body.Bytes(), []byte("выдуманное условие")) {
		t.Fatalf("отчёт содержит лишнее: %s", own.Body.Bytes())
	}
	var report struct {
		CorrectionID int    `json:"correction_id"`
		State        string `json:"state"`
	}
	s6Decode(t, own.Body.Bytes(), &report)
	if report.CorrectionID != 8 || report.State != "done" {
		t.Fatalf("отчёт %+v", report)
	}
	foreign := s6Call(h, http.MethodGet, "/rechecks/8", nil, other)
	missing := s6Call(h, http.MethodGet, "/rechecks/99", nil, owner)
	s6Status(t, foreign, http.StatusForbidden)
	s6Status(t, missing, http.StatusForbidden)
	if s6ErrCode(t, foreign) != "forbidden" || foreign.Body.String() != missing.Body.String() {
		t.Fatalf("чужой %s, пустой %s", foreign.Body.Bytes(), missing.Body.Bytes())
	}
	pupil := s6Call(h, http.MethodGet, "/rechecks/8", nil, student)
	s6Status(t, pupil, http.StatusForbidden)
	if s6ErrCode(t, pupil) != "forbidden" {
		t.Fatalf("ученик %s", pupil.Body.Bytes())
	}
	anon := s6Call(h, http.MethodGet, "/rechecks/8", nil, nil)
	s6Status(t, anon, http.StatusUnauthorized)
	if s6ErrCode(t, anon) != "unauthorized" {
		t.Fatalf("без сеанса %s", anon.Body.Bytes())
	}
}

const s6Variant = "ct-demo:D1.T01.S1.V003"

func s6Open(t *testing.T) (*pgx.Conn, http.Handler) {
	t.Helper()
	conn := s6DB(t)
	return conn, internalapi.Handler()
}

func s6DB(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := dbtest.Start(t)
	s6Migrate(t, dsn)
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close(context.Background())
	})
	t.Setenv("DATABASE_URL", dsn)
	return conn
}

func s6Call(h http.Handler, method, path string, body []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func s6JSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func s6Status(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.Bytes())
	}
}

func s6Decode(t *testing.T, raw []byte, dest any) {
	t.Helper()
	if err := json.Unmarshal(raw, dest); err != nil {
		t.Fatalf("json: %v, тело %s", err, raw)
	}
}

func s6ErrCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	s6Decode(t, rec.Body.Bytes(), &body)
	return body.Code
}

func s6Plan(student, subject string, shift int, lessons []map[string]any) map[string]any {
	body := map[string]any{
		"student_id": student,
		"subject":    subject,
		"lessons":    lessons,
		"snapshot": map[string]any{
			"source_hashes": map[string]string{
				"lessons/s6.md": "abababababababababababababababababababababababababababababababab",
			},
		},
	}
	if shift != 0 {
		body["shift_weeks"] = shift
	}
	return body
}

func s6Lesson(number int, week, kind string, topics, review []string) map[string]any {
	if review == nil {
		review = []string{}
	}
	return map[string]any{
		"subject":      "math",
		"lesson":       number,
		"planned_week": week,
		"phase":        "cycle-s6",
		"kind":         kind,
		"topics":       topics,
		"review":       review,
		"title":        "Выдуманный урок",
		"source_path":  "lessons/s6.md",
		"source_hash":  "abababababababababababababababababababababababababababababababab",
	}
}

func s6PutStudent(t *testing.T, conn *pgx.Conn, tutor, student string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.app_user (id, role, status) VALUES ($1, 'tutor', 'active')
		ON CONFLICT (id) DO NOTHING
	`, tutor); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, student, tutor); err != nil {
		t.Fatal(err)
	}
}

func s6PutVariant(t *testing.T, conn *pgx.Conn, version int, status string) {
	t.Helper()
	body := []byte(`{"id":"ct-demo:D1.T01.S1.V003","status":"` + status + `","key":14,"answer_format":"ct.integer","statement":"выдуманное условие"}`)
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, $2, $3, $4::jsonb)
	`, s6Variant, version, "s6-hash", body); err != nil {
		t.Fatal(err)
	}
}

func s6EventCount(t *testing.T, conn *pgx.Conn, kind string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.journal_event WHERE kind = $1
	`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func s6BatchCount(t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM core.recheck_batch`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func s6ExpectNoLessons(t *testing.T, conn *pgx.Conn, student string) {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.lesson WHERE student_id = $1
	`, student).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("уроков %d", n)
	}
}

func s6Session(t *testing.T, conn *pgx.Conn, userID string) *http.Cookie {
	t.Helper()
	sumID := sha256.Sum256([]byte("s6-session-" + userID))
	raw := sumID[:]
	sum := sha256.Sum256(raw)
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.session (token_hash, user_id, created_at, expires_at)
		VALUES ($1, $2, now(), now() + interval '1 day')
	`, sum[:], userID); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "__Host-session", Value: hex.EncodeToString(raw)}
}

func s6Migrate(t *testing.T, dsn string) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if s6File(filepath.Join(dir, "go.mod")) && s6File(filepath.Join(dir, "cmd", "migrate", "main.go")) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("модуль core не найден")
		}
		dir = parent
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "-mod=readonly", "./cmd/migrate")
	cmd.Dir = dir
	env := os.Environ()
	filtered := env[:0]
	for _, item := range env {
		if !strings.HasPrefix(item, "DATABASE_URL=") {
			filtered = append(filtered, item)
		}
	}
	cmd.Env = append(filtered, "DATABASE_URL="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
}

func s6File(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
