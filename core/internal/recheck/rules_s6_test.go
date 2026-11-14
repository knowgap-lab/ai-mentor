package recheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/answerrules"
	"github.com/knowgap-lab/ai-mentor/core/internal/recheck"
)

// Правила S6 поверх Accept и Run. Строки recheck-01…recheck-19 привязаны
// к тестам с тем же номером в имени. Баллы берёт answerrules.Score.
//
// ВОПРОС: answerrules.Score не принимает rules_version, другой версии правил в контракте нет. Варианты: любая непустая rules_version идёт в тот же Score; неизвестная версия — ошибка пакета. Не выбираю. Ветку другой версии не тестирую.
// ВОПРОС: RC-R8 ждёт паузу 30 с · 2^(попытка − 1) пять раз, это дольше 15 минут, а способа уронить порцию спецификация не даёт. Варианты: ждать паузы; проверять только итог failed. Не выбираю. RC-R8 не тестирую.
// ВОПРОС: тот же correction_id и другой список вариантов. RC-R2 говорит только про тот же список. Варианты: отказ; вернуть уже принятый пакет. Не выбираю.
// ВОПРОС: входит ли ручная попытка в processed и cursor. Проверяю skipped_manual и неизменность вердикта. processed и cursor не сравниваю.
// ВОПРОС: RC-R16 «ученик видит ключ» не называет поле. В RecheckReport ключа нет, другого маршрута в операциях S6 нет. Варианты: поле отчёта; ответ кабинета. Не выбираю. Проверяю, что элемент остаётся final и итог меняется.

const (
	s6Format       = "ct.integer"
	s6RulesVersion = "rikz-2026"
	s6Variant      = "ct-demo:D1.T01.S1.V003"
	s6OldKey       = 12
	s6NewKey       = 14
	s6Issued       = 7
	s6Correction   = 8
	s6Tutor        = "s6-tutor"
	s6Student      = "s6-student"
	s6StudentB     = "s6-student-b"
	s6Name         = "S6NAME"
)

func TestRC_R2_S6_recheck01(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	variants := []string{s6Variant}

	first, err := recheck.Accept(context.Background(), conn, s6Correction, variants)
	if err != nil {
		t.Fatalf("первый Accept: %v", err)
	}
	second, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant})
	if err != nil {
		t.Fatalf("повтор Accept: %v", err)
	}
	if second != first {
		t.Fatalf("повтор %+v, первый %+v", second, first)
	}
	if batchCount(t, look, s6Correction) != 1 || batchTotal(t, look) != 1 {
		t.Fatal("повтор создал второй пакет")
	}
}

func TestRC_R3_S6_recheck02(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Correction, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction+1, s6OldKey, "active")
	if _, err := recheck.Accept(context.Background(), conn, s6Correction+1, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	_, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant})
	var recErr *recheck.Error
	if !errors.As(err, &recErr) || recErr == nil || recErr.Code != "stale_correction" {
		t.Fatalf("ошибка %v", err)
	}
	if batchCount(t, look, s6Correction) != 0 || batchCount(t, look, s6Correction+1) != 1 {
		t.Fatal("устаревший correction_id изменил набор пакетов")
	}
}

func TestRC_R3_S6_LargerCorrectionAccepted(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Correction, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction+2, s6NewKey, "active")
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	got, err := recheck.Accept(context.Background(), conn, s6Correction+2, []string{s6Variant})
	if err != nil {
		t.Fatalf("больший correction_id: %v", err)
	}
	if got.State != "pending" || got.CorrectionID != s6Correction+2 {
		t.Fatalf("пакет %+v", got)
	}
	if batchCount(t, look, s6Correction) != 1 || batchCount(t, look, s6Correction+2) != 1 {
		t.Fatal("больший correction_id не принят отдельным пакетом")
	}
}

func TestRC_R4_S6_recheck03(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	_, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant, "ct-demo:missing"})
	var recErr *recheck.Error
	if !errors.As(err, &recErr) || recErr == nil || recErr.Code != "variant_unknown" {
		t.Fatalf("ошибка %v", err)
	}
	if batchTotal(t, look) != 0 {
		t.Fatal("неизвестный вариант что-то принял")
	}
}

func TestRC_R4_S6_RetiredVariantAccepted(t *testing.T) {
	conn, _ := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "retired")
	got, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "pending" || got.CorrectionID != s6Correction {
		t.Fatalf("retired не принят: %+v", got)
	}
}

func TestRC_R5_S6_AcceptPending(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	attemptID := s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-pending", Answer: "14", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
	})
	before := loadVerdicts(t, look, attemptID)
	got, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "pending" {
		t.Fatalf("состояние %s", got.State)
	}
	if rows := loadVerdicts(t, look, attemptID); !sameRows(rows, before) {
		t.Fatal("приём дописал вердикт")
	}
}

func TestRC_R6_S6_TimesStored(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-time", Answer: "15", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
	})
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	var started, finished *time.Time
	err := look.QueryRow(context.Background(), `
		SELECT started_at, finished_at FROM core.recheck_batch WHERE correction_id = $1
	`, s6Correction).Scan(&started, &finished)
	if err != nil {
		t.Fatal(err)
	}
	if started != nil || finished != nil {
		t.Fatalf("у pending уже есть время: %v %v", started, finished)
	}
	if _, err := recheck.Run(context.Background(), conn, s6Correction); err != nil {
		t.Fatal(err)
	}
	err = look.QueryRow(context.Background(), `
		SELECT started_at, finished_at FROM core.recheck_batch WHERE correction_id = $1
	`, s6Correction).Scan(&started, &finished)
	if err != nil {
		t.Fatal(err)
	}
	if started == nil || finished == nil || finished.Before(*started) {
		t.Fatalf("время пакета started=%v finished=%v", started, finished)
	}
}

func TestRC_R7_S6_recheck04_Portions(t *testing.T) {
	conn, look := newDB(t)
	const n = 1200
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutMany(t, conn, n, "15")
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	if _, err := look.Exec(context.Background(), `
		CREATE TABLE s6_cursor_log (
			processed integer NOT NULL,
			cursor text NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := look.Exec(context.Background(), `
		CREATE FUNCTION s6_log_cursor() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			INSERT INTO s6_cursor_log (processed, cursor)
			VALUES (NEW.processed, NEW.cursor);
			RETURN NEW;
		END;
		$$
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := look.Exec(context.Background(), `
		CREATE TRIGGER s6_log
		AFTER UPDATE ON core.recheck_batch
		FOR EACH ROW EXECUTE FUNCTION s6_log_cursor()
	`); err != nil {
		t.Fatal(err)
	}
	finished, err := recheck.Run(context.Background(), conn, s6Correction)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != "done" || finished.Total != n || finished.Processed != n {
		t.Fatalf("пакет %+v", finished)
	}
	if finished.Cursor != s6AttemptID(n) {
		t.Fatalf("cursor %q", finished.Cursor)
	}
	rows, err := look.Query(context.Background(), `
		SELECT processed, COALESCE(cursor, '') FROM s6_cursor_log ORDER BY processed, cursor
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[int]string{}
	for rows.Next() {
		var processed int
		var cursor string
		if err := rows.Scan(&processed, &cursor); err != nil {
			t.Fatal(err)
		}
		seen[processed] = cursor
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{500, 1000, 1200} {
		cursor, ok := seen[want]
		if !ok || cursor != s6AttemptID(want) {
			t.Fatalf("порция %d: cursor %q, журнал %v", want, cursor, seen)
		}
	}
}

func TestRC_R7_S6_recheck05_ResumeCursor(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	for i := 1; i <= 3; i++ {
		s6PutAttempt(t, conn, s6Attempt{
			ID: s6AttemptID(i), Answer: "14", Item: fmt.Sprintf("s6-item-%d", i),
			Assignment: "s6-asg", Student: s6Student,
		})
	}
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `
		UPDATE core.recheck_batch
		SET state = 'running', processed = 2, cursor = $2, started_at = now()
		WHERE correction_id = $1
	`, s6Correction, s6AttemptID(2)); err != nil {
		t.Fatal(err)
	}
	finished, err := recheck.Run(context.Background(), conn, s6Correction)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != "done" || finished.Processed != 3 || finished.Cursor != s6AttemptID(3) {
		t.Fatalf("пакет %+v", finished)
	}
	if n := len(loadVerdicts(t, look, s6AttemptID(1))); n != 1 {
		t.Fatalf("попытка до курсора пересчитана, строк %d", n)
	}
	if n := len(loadVerdicts(t, look, s6AttemptID(2))); n != 1 {
		t.Fatalf("попытка на курсоре пересчитана, строк %d", n)
	}
	if n := len(loadVerdicts(t, look, s6AttemptID(3))); n != 2 {
		t.Fatalf("попытка после курсора: строк %d", n)
	}
}

func TestRC_R9_S6_PendingDoesNotBlock(t *testing.T) {
	conn, _ := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := conn.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		) VALUES ('s6-asg-open', $1, 'issued', 'homework', $2, NULL, 'home', '{}'::jsonb)
	`, s6Student, s6Issued)
	if err != nil {
		t.Fatalf("пока пакет pending, запись ученика не прошла: %v", err)
	}
}

func TestRC_R10_S6_recheck07_NewKey(t *testing.T) {
	old := answerrules.Score(s6Format, s6OldKey, "14", nil)
	next := answerrules.Score(s6Format, s6NewKey, "14", nil)
	if old.Verdict != "wrong" || old.Points != 0 || next.Verdict != "correct" || next.Points != 2 {
		t.Fatalf("ВОПРОС: Score не даёт примеру recheck-07 0 → 2: %+v %+v", old, next)
	}
	got := s6RunOne(t, "14", false)
	if got.finished.Changed != 1 {
		t.Fatalf("пакет %+v", got.finished)
	}
	if got.verdicts[1].verdict != next.Verdict || got.verdicts[1].points != next.Points {
		t.Fatalf("вердикт %+v", got.verdicts[1])
	}
}

func TestRC_R10_S6_recheck08_OldKey(t *testing.T) {
	old := answerrules.Score(s6Format, s6OldKey, "12", nil)
	next := answerrules.Score(s6Format, s6NewKey, "12", nil)
	if old.Verdict != "correct" || next.Verdict != "wrong" {
		t.Fatalf("ВОПРОС: Score не даёт примеру recheck-08 correct → wrong: %+v %+v", old, next)
	}
	got := s6RunOne(t, "12", false)
	if got.verdicts[1].verdict != next.Verdict || got.verdicts[1].points != next.Points {
		t.Fatalf("вердикт %+v", got.verdicts[1])
	}
}

func TestRC_R11_S6_recheck09(t *testing.T) {
	got := s6RunOne(t, "15", false)
	if len(got.verdicts) != 1 || len(got.events) != 0 || got.finished.Changed != 0 {
		t.Fatalf("вердикты %+v, события %d, пакет %+v", got.verdicts, len(got.events), got.finished)
	}
}

func TestRC_R12_S6_recheck10_SchemaPayload(t *testing.T) {
	got := s6RunOne(t, "14", false)
	if len(got.verdicts) != 2 || got.verdicts[0].verdict == "" {
		t.Fatalf("прежний вердикт не сохранился: %+v", got.verdicts)
	}
	if len(got.events) != 1 {
		t.Fatalf("событий %d", len(got.events))
	}
	ev := got.events[0]
	if ev["origin"] != "recheck" || ev["correction_id"] != float64(s6Correction) {
		t.Fatalf("событие %v", ev)
	}
	if ev["previous_verdict"] == nil || ev["previous_points"] == nil || ev["max_points"] == nil {
		t.Fatalf("нет полей схемы: %v", ev)
	}
	if _, ok := ev["student_name"]; ok {
		t.Fatal("в событии лишнее поле")
	}
}

func TestRC_R13_S6_recheck11_ManualEvent(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	attemptID := s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-manual", Answer: "14", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
		ManualEvent: true,
	})
	before := loadVerdicts(t, look, attemptID)
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	finished, err := recheck.Run(context.Background(), conn, s6Correction)
	if err != nil {
		t.Fatal(err)
	}
	if finished.SkippedManual != 1 || finished.Changed != 0 {
		t.Fatalf("пакет %+v", finished)
	}
	if rows := loadVerdicts(t, look, attemptID); !sameRows(rows, before) {
		t.Fatalf("ручная попытка пересчитана: %+v", rows)
	}
	if n := len(loadRecheckEvents(t, look, attemptID)); n != 0 {
		t.Fatalf("событий recheck %d", n)
	}
}

func TestRC_R14_S6_HoldOneUntilDone(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction+1, s6NewKey, "active")
	attemptID := s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-order", Answer: "14", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
	})
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	if _, err := recheck.Accept(context.Background(), conn, s6Correction+1, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = recheck.Run(ctx, conn, s6Correction+1)
	later := loadBatch(t, look, s6Correction+1)
	if later.State == "done" || later.State == "running" {
		t.Fatalf("пакет %d начался до done предыдущего: %+v", s6Correction+1, later)
	}
	if n := len(loadVerdicts(t, look, attemptID)); n != 1 {
		t.Fatalf("более поздний пакет пересчитал попытку, строк %d", n)
	}
	earlier, err := recheck.Run(context.Background(), conn, s6Correction)
	if err != nil {
		t.Fatal(err)
	}
	if earlier.State != "done" {
		t.Fatalf("ранний пакет %+v", earlier)
	}
	next, err := recheck.Run(context.Background(), conn, s6Correction+1)
	if err != nil {
		t.Fatal(err)
	}
	if next.State != "done" {
		t.Fatalf("следующий пакет после done %+v", next)
	}
}

func TestRC_R15_S6_recheck12_Observation(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, s6Student)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-b1", Answer: "14", Item: "s6-item", Assignment: "s6-asg", Student: s6Student, AttemptNo: 1,
	})
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-b2", Answer: "12", Item: "s6-item", Assignment: "s6-asg", Student: s6Student, AttemptNo: 2,
		SkipAssignment: true,
	})
	s6PutObservation(t, conn, "s6-asg", "s6-item", 1, 0, 2, 2)
	s6Finish(t, conn)
	body := s6Observation(t, look, "s6-asg", "s6-item", 2)
	if body["first_attempt_points"] != float64(2) || body["total_points"] != float64(2) {
		t.Fatalf("наблюдение %v", body)
	}
	var rev1 int
	if err := look.QueryRow(context.Background(), `
		SELECT count(*) FROM core.observation
		WHERE assignment_id = 's6-asg' AND item_id = 's6-item' AND revision = 1
	`).Scan(&rev1); err != nil {
		t.Fatal(err)
	}
	if rev1 != 1 {
		t.Fatal("ревизия 1 не сохранилась")
	}
}

func TestRC_R16_S6_recheck13_StaysFinal(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, s6Student)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-final", Answer: "12", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
		ItemState: "final",
	})
	s6PutObservation(t, conn, "s6-asg", "s6-item", 1, 2, 2, 2)
	s6Finish(t, conn)
	var state string
	var attempts int
	if err := look.QueryRow(context.Background(), `
		SELECT state FROM core.assignment_item WHERE assignment_id = 's6-asg' AND item_id = 's6-item'
	`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "final" {
		t.Fatalf("состояние элемента %s", state)
	}
	if err := look.QueryRow(context.Background(), `
		SELECT count(*) FROM core.attempts WHERE assignment_id = 's6-asg' AND item_id = 's6-item'
	`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("попыток %d", attempts)
	}
	body := s6Observation(t, look, "s6-asg", "s6-item", 2)
	if body["first_attempt_points"] != float64(0) || body["total_points"] != float64(0) {
		t.Fatalf("итог %v", body)
	}
}

func TestRC_R17_S6_recheck14_SecondAttemptOut(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, s6Student)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-p1", Answer: "14", Item: "s6-item", Assignment: "s6-asg", Student: s6Student, AttemptNo: 1,
	})
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-p2", Answer: "12", Item: "s6-item", Assignment: "s6-asg", Student: s6Student, AttemptNo: 2,
		SkipAssignment: true,
	})
	s6PutObservation(t, conn, "s6-asg", "s6-item", 1, 0, 2, 2)
	s6Finish(t, conn)
	body := s6Observation(t, look, "s6-asg", "s6-item", 2)
	if body["first_attempt_points"] != float64(2) || body["total_points"] != float64(2) {
		t.Fatalf("итог включил вторую попытку: %v", body)
	}
}

func TestRC_R19_S6_recheck16_OneEvent(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, s6StudentB)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	for i := 1; i <= 3; i++ {
		s6PutAttempt(t, conn, s6Attempt{
			ID: fmt.Sprintf("s6-att-ch-%d", i), Answer: "14",
			Item: fmt.Sprintf("s6-item-%d", i), Assignment: "s6-asg-b", Student: s6StudentB,
		})
	}
	s6Finish(t, conn)
	rows, err := look.Query(context.Background(), `
		SELECT payload FROM core.journal_event WHERE kind = 'recheck_completed'
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var payloads []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, body)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 {
		t.Fatalf("событий %d", len(payloads))
	}
	if payloads[0]["correction_id"] != float64(s6Correction) {
		t.Fatalf("payload %v", payloads[0])
	}
	items, _ := payloads[0]["changed_items"].([]any)
	if len(items) != 3 {
		t.Fatalf("элементы %v", payloads[0]["changed_items"])
	}
}

func TestRC_R20_S6_recheck17_OneTutorMessage(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, s6Student)
	s6PutUsers(t, conn, s6Tutor, s6StudentB)
	s6PutName(t, conn, s6Student, s6Name)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-a", Answer: "14", Item: "s6-item-a", Assignment: "s6-asg-a", Student: s6Student,
	})
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-b", Answer: "14", Item: "s6-item-b", Assignment: "s6-asg-b", Student: s6StudentB,
	})
	s6Finish(t, conn)
	s6ExpectTutorMail(t, look, 1)
}

func TestRC_R20_S6_recheck18_NoChangesNoMessage(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, s6Student)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-same", Answer: "15", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
	})
	s6Finish(t, conn)
	s6ExpectTutorMail(t, look, 0)
}

func TestRC_R21_S6_recheck19_ArchivedAndRT(t *testing.T) {
	conn, look := newDB(t)
	s6PutUsers(t, conn, s6Tutor, "s6-archived")
	s6PutUsers(t, conn, s6Tutor, "s6-anon")
	if _, err := conn.Exec(context.Background(), `
		UPDATE core.app_user SET status = 'archived' WHERE id = 's6-archived'
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `
		UPDATE core.app_user SET status = 'anonymised' WHERE id = 's6-anon'
	`); err != nil {
		t.Fatal(err)
	}
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	archived := s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-arch", Answer: "14", Item: "s6-item-arch", Assignment: "s6-asg-arch",
		Student: "s6-archived", Source: "rt",
	})
	anon := s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-anon", Answer: "14", Item: "s6-item-anon", Assignment: "s6-asg-anon",
		Student: "s6-anon",
	})
	finished := s6Finish(t, conn)
	if finished.Changed != 2 {
		t.Fatalf("пакет %+v", finished)
	}
	for _, id := range []string{archived, anon} {
		rows := loadVerdicts(t, look, id)
		if len(rows) != 2 || rows[0].verdict == rows[1].verdict && rows[0].points == rows[1].points {
			t.Fatalf("попытка %s не пересчитана: %+v", id, rows)
		}
	}
}

func TestRC_R22_S6_CatalogUnchanged(t *testing.T) {
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	before := s6Keys(t, look)
	s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-cat", Answer: "14", Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
	})
	s6Finish(t, conn)
	after := s6Keys(t, look)
	if len(after) != len(before) {
		t.Fatalf("ревизий было %d, стало %d", len(before), len(after))
	}
	for id, key := range before {
		if after[id] != key {
			t.Fatalf("ключ %s был %d, стал %d", id, key, after[id])
		}
	}
}

type s6Attempt struct {
	ID             string
	Answer         string
	Item           string
	Assignment     string
	Student        string
	AttemptNo      int
	Source         string
	ItemState      string
	ManualEvent    bool
	SkipAssignment bool
}

type s6Ran struct {
	finished recheck.State
	verdicts []verdictRow
	events   []map[string]any
}

func s6RunOne(t *testing.T, answer string, manual bool) s6Ran {
	t.Helper()
	conn, look := newDB(t)
	s6PutVariant(t, conn, s6Variant, s6Issued, s6OldKey, "active")
	s6PutVariant(t, conn, s6Variant, s6Correction, s6NewKey, "active")
	attemptID := s6PutAttempt(t, conn, s6Attempt{
		ID: "s6-att-" + answer, Answer: answer, Item: "s6-item", Assignment: "s6-asg", Student: s6Student,
		ManualEvent: manual,
	})
	finished := s6Finish(t, conn)
	return s6Ran{
		finished: finished,
		verdicts: loadVerdicts(t, look, attemptID),
		events:   s6Events(t, look, "attempt_verdict", attemptID),
	}
}

func s6Finish(t *testing.T, conn *pgx.Conn) recheck.State {
	t.Helper()
	if _, err := recheck.Accept(context.Background(), conn, s6Correction, []string{s6Variant}); err != nil {
		t.Fatal(err)
	}
	finished, err := recheck.Run(context.Background(), conn, s6Correction)
	if err != nil {
		t.Fatal(err)
	}
	return finished
}

func s6PutVariant(t *testing.T, conn *pgx.Conn, id string, version, key int, status string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id": id, "status": status, "key": key, "answer_format": s6Format,
		"statement": "выдуманное условие",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(context.Background(), `
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, $2, $3, $4::jsonb)
	`, id, version, fmt.Sprintf("s6-%d-%d", version, key), body)
	if err != nil {
		t.Fatal(err)
	}
}

func s6PutAttempt(t *testing.T, conn *pgx.Conn, in s6Attempt) string {
	t.Helper()
	if in.AttemptNo == 0 {
		in.AttemptNo = 1
	}
	if in.Source == "" {
		in.Source = "homework"
	}
	if in.ItemState == "" {
		in.ItemState = "open"
	}
	scored := answerrules.Score(s6Format, s6OldKey, in.Answer, nil)
	if scored.Status != "scored" {
		t.Fatalf("ответ %q не оценён", in.Answer)
	}
	ceiling := answerrules.Score(s6Format, s6OldKey, "12", nil)
	ctx := context.Background()
	if !in.SkipAssignment {
		snap, err := json.Marshal(map[string]any{
			"items": []any{map[string]any{
				"item_id": in.Item, "variant_id": s6Variant, "answer_format": s6Format,
				"rules_version": s6RulesVersion, "key": s6OldKey,
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `
			INSERT INTO core.assignment (
				id, student_id, state, kind, catalog_version, deadline, place, snapshot
			) VALUES ($1, $2, 'issued', 'homework', $3, NULL, 'home', $4::jsonb)
			ON CONFLICT (id) DO NOTHING
		`, in.Assignment, in.Student, s6Issued, snap); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `
			INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
			VALUES ($1, $2, $3, false, $4)
		`, in.Assignment, in.Item, in.ItemState, ceiling.Points); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempts (
			id, assignment_id, item_id, raw_input, source, place, attempt_no
		) VALUES ($1, $2, $3, $4, $5, 'home', $6)
	`, in.ID, in.Assignment, in.Item, in.Answer, in.Source, in.AttemptNo); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		VALUES ($1, 1, $2, $3)
	`, in.ID, scored.Verdict, scored.Points); err != nil {
		t.Fatal(err)
	}
	recorded, err := json.Marshal(map[string]any{
		"attempt_id": in.ID, "assignment_id": in.Assignment, "item_id": in.Item,
		"variant_id": s6Variant, "catalog_version": s6Issued, "raw_input": in.Answer,
		"normalized_input": scored.Normalized, "attempt_no": in.AttemptNo,
		"verdict": scored.Verdict, "points": scored.Points, "source": in.Source, "place": "home",
		"rules_version": s6RulesVersion, "idempotency_key": "idem-" + in.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('attempt_recorded', $1::jsonb)
	`, recorded); err != nil {
		t.Fatal(err)
	}
	if !in.ManualEvent {
		return in.ID
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		VALUES ($1, 2, 'partial', 1)
	`, in.ID); err != nil {
		t.Fatal(err)
	}
	manualVerdict, err := json.Marshal(map[string]any{
		"attempt_id": in.ID, "origin": "manual", "verdict": "partial", "points": 1,
		"max_points": ceiling.Points, "reason": "зачтено по выдуманной причине",
	})
	if err != nil {
		t.Fatal(err)
	}
	manualRequest, err := json.Marshal(map[string]any{
		"attempt_id": in.ID, "verdict": "partial", "points": 1,
		"reason": "зачтено по выдуманной причине",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('attempt_verdict', $1::jsonb)
	`, manualVerdict); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('manual_verdict', $1::jsonb)
	`, manualRequest); err != nil {
		t.Fatal(err)
	}
	return in.ID
}

func s6PutMany(t *testing.T, conn *pgx.Conn, n int, answer string) {
	t.Helper()
	scored := answerrules.Score(s6Format, s6OldKey, answer, nil)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		) VALUES ('s6-asg', $1, 'issued', 'homework', $2, NULL, 'home', '{}'::jsonb)
	`, s6Student, s6Issued); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		SELECT 's6-asg', 's6-item-' || lpad(i::text, 4, '0'), 'open', false, 2
		FROM generate_series(1, $1) AS i
	`, n); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempts (id, assignment_id, item_id, raw_input, source, place, attempt_no)
		SELECT 's6-att-' || lpad(i::text, 4, '0'), 's6-asg', 's6-item-' || lpad(i::text, 4, '0'),
		       $2, 'homework', 'home', 1
		FROM generate_series(1, $1) AS i
	`, n, answer); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		SELECT 's6-att-' || lpad(i::text, 4, '0'), 1, $2, $3
		FROM generate_series(1, $1) AS i
	`, n, scored.Verdict, scored.Points); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		SELECT 'attempt_recorded', json_build_object(
			'attempt_id', 's6-att-' || lpad(i::text, 4, '0'),
			'assignment_id', 's6-asg',
			'item_id', 's6-item-' || lpad(i::text, 4, '0'),
			'variant_id', $2::text,
			'catalog_version', $3::int,
			'raw_input', $4::text,
			'normalized_input', $5::text,
			'attempt_no', 1,
			'verdict', $6::text,
			'points', $7::int,
			'source', 'homework',
			'place', 'home',
			'rules_version', $8::text,
			'idempotency_key', 'idem-' || i::text
		)
		FROM generate_series(1, $1) AS i
	`, n, s6Variant, s6Issued, answer, scored.Normalized, scored.Verdict, scored.Points, s6RulesVersion); err != nil {
		t.Fatal(err)
	}
}

func s6PutUsers(t *testing.T, conn *pgx.Conn, tutor, student string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.app_user (id, role, status)
		VALUES ($1, 'tutor', 'active')
		ON CONFLICT (id) DO NOTHING
	`, tutor); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, student, tutor); err != nil {
		t.Fatal(err)
	}
}

func s6PutName(t *testing.T, conn *pgx.Conn, student, name string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, $2, $3)
	`, student, name, "tg-"+student); err != nil {
		t.Fatal(err)
	}
}

func s6PutObservation(t *testing.T, conn *pgx.Conn, assignment, item string, revision, first, total, max int) {
	t.Helper()
	_, err := conn.Exec(context.Background(), `
		INSERT INTO core.observation (
			assignment_id, item_id, revision, first_attempt_points, total_points, max_points
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, assignment, item, revision, first, total, max)
	if err != nil {
		t.Fatal(err)
	}
}

func s6Observation(t *testing.T, conn *pgx.Conn, assignment, item string, revision int) map[string]any {
	t.Helper()
	var raw []byte
	err := conn.QueryRow(context.Background(), `
		SELECT payload
		FROM core.journal_event
		WHERE kind = 'observation'
		  AND payload->>'item_id' = $1
		  AND payload->>'revision' = $2
	`, item, fmt.Sprintf("%d", revision)).Scan(&raw)
	if err != nil {
		t.Fatalf("наблюдение revision %d: %v", revision, err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"observation_id", "revision", "type_id", "subvariant_id", "variant_id",
		"source", "place", "first_attempt_points", "total_points", "max_points",
		"trap_ids", "skipped", "late", "catalog_version", "rules_version",
	} {
		if _, ok := body[key]; !ok {
			t.Fatalf("в наблюдении нет %s: %v", key, body)
		}
	}
	if _, ok := body["item_id"]; !ok {
		if _, ok := body["rt_import"]; !ok {
			t.Fatalf("в наблюдении нет item_id: %v", body)
		}
	}
	allowed := map[string]bool{
		"observation_id": true, "item_id": true, "revision": true, "type_id": true,
		"subvariant_id": true, "variant_id": true, "source": true, "place": true,
		"first_attempt_points": true, "total_points": true, "max_points": true,
		"trap_ids": true, "skipped": true, "late": true, "catalog_version": true,
		"rules_version": true, "rt_import": true,
	}
	for key := range body {
		if !allowed[key] {
			t.Fatalf("в наблюдении лишнее поле %s", key)
		}
	}
	if assignment == "" {
		t.Fatal("пустая выдача")
	}
	return body
}

func s6Events(t *testing.T, conn *pgx.Conn, kind, attemptID string) []map[string]any {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT payload FROM core.journal_event
		WHERE kind = $1 AND payload->>'attempt_id' = $2 AND payload->>'origin' = 'recheck'
		ORDER BY journal_seq
	`, kind, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		out = append(out, body)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func s6ExpectTutorMail(t *testing.T, conn *pgx.Conn, want int) {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT COALESCE(recipient_kind, ''), COALESCE(recipient_id, ''), COALESCE(template, ''),
		       COALESCE(params::text, ''), COALESCE(payload::text, '')
		FROM core.outbox
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		var kind, id, template, params, payload string
		if err := rows.Scan(&kind, &id, &template, &params, &payload); err != nil {
			t.Fatal(err)
		}
		n++
		if kind != "tutor" || id != s6Tutor || template != "recheck_changes" {
			t.Fatalf("сообщение %s %s %s", kind, id, template)
		}
		text := params + payload
		if strings.Contains(text, s6Name) {
			t.Fatal("в outbox есть имя ученика")
		}
		if !strings.Contains(text, "/rechecks/") {
			t.Fatal("в сообщении нет ссылки на отчёт")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("сообщений %d, ожидалось %d", n, want)
	}
}

func s6Keys(t *testing.T, conn *pgx.Conn) map[string]int {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT entity_id || ':' || catalog_version_from::text, (data->>'key')::int
		FROM catalog.revision
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var key int
		if err := rows.Scan(&id, &key); err != nil {
			t.Fatal(err)
		}
		out[id] = key
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func s6AttemptID(n int) string {
	return fmt.Sprintf("s6-att-%04d", n)
}
