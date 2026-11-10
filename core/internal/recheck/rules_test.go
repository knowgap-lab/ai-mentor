package recheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/answerrules"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/recheck"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
// База поднимается через dbtest. Нет Docker — тест пропускается там.
//
//	Accept(ctx, conn, correctionID, variants) (State, error)
//	Run(ctx, conn, correctionID) (State, error)
//
// State: CorrectionID, State, Total, Processed, Changed, SkippedManual, Cursor.
// Поле State — pending, running, done или failed.
// Первый Accept возвращает pending и не добавляет attempt_verdict.
// До возврата вызов коммитит свою работу: её видно с другого соединения.
// *Error содержит Code. Error() возвращает Code.
//
// Пакет — строка core.recheck_batch с колонками correction_id, state, total,
// processed, changed, skipped_manual, cursor. Повтор Accept с тем же
// correction_id и тем же списком возвращает тот же State и не добавляет строку.
// correction_id меньше уже принятого максимума — *Error с кодом stale_correction,
// строки с этим correction_id нет.
//
// Ключ версии N — catalog.revision с этим entity_id и наибольшим
// catalog_version_from не больше N. data.key — целое JSON. data.answer_format —
// ct.integer. data.status — active. В Score ключ передаётся как int или как
// десятичная строка. Снимок выдачи хранит ключ версии выдачи; перепроверка
// берёт ключ версии correction_id.
//
// Попытка варианта — событие core.journal_event kind attempt_recorded.
// В payload на верхнем уровне лежат attempt_id, variant_id, rules_version,
// catalog_version и raw_input. Тот же сырой ответ лежит в core.attempts.raw_input.
// Текущий вердикт — строка core.attempt_verdict с наибольшим seq.
// Seq 1 — оценка при записи: reason пустой, вердикт и баллы считает тест
// через answerrules.Score(ct.integer, ключ 12, ответ, nil).
// Если вердикт или баллы изменились, Run добавляет следующую строку и событие
// kind attempt_verdict. Payload этого события — поля на верхнем уровне:
// origin, attempt_id, verdict, points, correction_id, previous_verdict,
// previous_points. origin новой строки — recheck. Прежняя строка остаётся.
// Если вердикт и баллы те же, новой строки и события origin recheck нет.
//
// Ручной текущий вердикт: seq 2, reason не пустой, событие kind attempt_verdict
// с origin manual и событие kind manual_verdict с тем же attempt_id.
// Run такую попытку не пересчитывает и увеличивает SkippedManual на 1.
//
// Ожидаемый вердикт и баллы тест не берёт из таблицы примеров. Их даёт Score.
// Ответ ученика известен, ключ выдачи 12, ключ исправления 14, формат ct.integer.

// ВОПРОС: имя таблицы пакета в спецификации не названо. Тест считает строки core.recheck_batch и читает колонки correction_id, state, total, processed, changed, skipped_manual, cursor.
// ВОПРОС: answerrules.Score не принимает rules_version. Тест пишет в попытку rikz-2026 и сверяет новый вердикт со Score. Другую версию правил не подставляю.
// ВОПРОС: у роли core_app нет USAGE на схеме catalog. Тест вызывает Accept и Run соединением postgres и не делает SET ROLE.
// ВОПРОС: входит ли ручная попытка в processed и двигает ли она cursor. Проверяю skipped_manual, changed и неизменность строк вердикта. processed и cursor в этом случае не сравниваю.
// ВОПРОС: таблица ошибок даёт HTTP 409. Тест проверяет код stale_correction у *Error. Статус HTTP не проверяю.
// ВОПРОС: тот же correction_id и другой список вариантов. Не проверяю.
// ВОПРОС: список попыток в отчёте пакета сверх счётчика skipped_manual. Не проверяю.
// ВОПРОС: пустой cursor. Читаю COALESCE(cursor, ''). NULL и пустую строку не различаю.

const (
	formatInteger = "ct.integer"
	rulesVersion  = "rikz-2026"
	variantID     = "ct-math:M14.T07.S1.V003"
	oldKey        = 12
	newKey        = 14
	issuedVersion = 7
	correctionID  = 8

	manualVerdict = "partial"
	manualPoints  = 1
	manualReason  = "зачтено репетитором"
)

func TestRC_R2_RepeatAccept(t *testing.T) {
	conn, look := newDB(t)
	insertVariant(t, conn, correctionID, newKey)
	variants := []string{variantID}

	first, err := recheck.Accept(context.Background(), conn, correctionID, variants)
	if err != nil {
		t.Fatalf("первый Accept: %v", err)
	}
	if first.State != "pending" || first.CorrectionID != correctionID {
		t.Fatalf("первый State %+v", first)
	}
	if batchCount(t, look, correctionID) != 1 {
		t.Fatalf("пакетов %d", batchCount(t, look, correctionID))
	}
	if got := loadBatch(t, look, correctionID); got != first {
		t.Fatalf("в базе %+v, Accept вернул %+v", got, first)
	}

	second, err := recheck.Accept(context.Background(), conn, correctionID, []string{variantID})
	if err != nil {
		t.Fatalf("повтор Accept: %v", err)
	}
	if second != first {
		t.Fatalf("повтор %+v, первый %+v", second, first)
	}
	withID := batchCount(t, look, correctionID)
	if withID != 1 || batchTotal(t, look) != 1 {
		t.Fatalf("после повтора пакетов %d с этим id, всего %d", withID, batchTotal(t, look))
	}
}

func TestRC_R3_StaleCorrection(t *testing.T) {
	conn, look := newDB(t)
	insertVariant(t, conn, correctionID, oldKey)
	insertVariant(t, conn, correctionID+1, oldKey)
	variants := []string{variantID}

	if _, err := recheck.Accept(context.Background(), conn, correctionID+1, variants); err != nil {
		t.Fatalf("Accept %d: %v", correctionID+1, err)
	}
	_, err := recheck.Accept(context.Background(), conn, correctionID, []string{variantID})
	var recErr *recheck.Error
	if !errors.As(err, &recErr) || recErr == nil || recErr.Code != "stale_correction" {
		t.Fatalf("ошибка %v", err)
	}
	if batchCount(t, look, correctionID) != 0 {
		t.Fatal("устаревший correction_id создал пакет")
	}
	if batchCount(t, look, correctionID+1) != 1 || batchTotal(t, look) != 1 {
		t.Fatal("принятый пакет 9 не сохранился один")
	}
}

func TestRC_R10_NewKeyMatchesAnswer(t *testing.T) {
	got := rescore(t, "14", false)
	if got.old.Verdict == got.next.Verdict && got.old.Points == got.next.Points {
		t.Fatal("ключи 12 и 14 не различают ответ 14")
	}
	expectChange(t, got)
}

func TestRC_R10_OldKeyMatchesAnswer(t *testing.T) {
	got := rescore(t, "12", false)
	if got.old.Verdict == got.next.Verdict && got.old.Points == got.next.Points {
		t.Fatal("ключи 12 и 14 не различают ответ 12")
	}
	expectChange(t, got)
}

func TestRC_R11_VerdictUnchanged(t *testing.T) {
	got := rescore(t, "15", false)
	if got.old.Verdict != got.next.Verdict || got.old.Points != got.next.Points {
		t.Fatalf("ответ 15 разошёлся: %+v -> %+v", got.old, got.next)
	}
	if got.finished.State != "done" || got.finished.Total != 1 || got.finished.Changed != 0 || got.finished.Processed != 1 || got.finished.SkippedManual != 0 {
		t.Fatalf("пакет %+v", got.finished)
	}
	if got.finished.Cursor != got.attemptID {
		t.Fatalf("cursor %q", got.finished.Cursor)
	}
	if len(got.verdicts) != 1 || got.verdicts[0] != got.before[0] {
		t.Fatalf("вердикты %+v, было %+v", got.verdicts, got.before)
	}
	if len(got.recheckEvents) != 0 {
		t.Fatalf("событий recheck %d", len(got.recheckEvents))
	}
	if got.attemptRows != 1 || got.rawInput != "15" {
		t.Fatalf("попыток %d, ответ %q", got.attemptRows, got.rawInput)
	}
}

func TestRC_R13_SkippedManual(t *testing.T) {
	got := rescore(t, "14", true)
	if got.next.Verdict == manualVerdict && got.next.Points == manualPoints {
		t.Fatal("ручной вердикт совпал с новой оценкой")
	}
	if got.finished.State != "done" || got.finished.Total != 1 || got.finished.SkippedManual != 1 || got.finished.Changed != 0 {
		t.Fatalf("пакет %+v", got.finished)
	}
	if len(got.verdicts) != len(got.before) {
		t.Fatalf("стало %+v, было %+v", got.verdicts, got.before)
	}
	for i := range got.before {
		if got.verdicts[i] != got.before[i] {
			t.Fatalf("строка %d: %+v, было %+v", i, got.verdicts[i], got.before[i])
		}
	}
	if len(got.recheckEvents) != 0 {
		t.Fatal("ручная попытка пересчитана")
	}
	if got.attemptRows != 1 || got.rawInput != "14" {
		t.Fatalf("попыток %d, ответ %q", got.attemptRows, got.rawInput)
	}
}

type scoredCase struct {
	attemptID     string
	answer        string
	old           answerrules.Result
	next          answerrules.Result
	accepted      recheck.State
	finished      recheck.State
	before        []verdictRow
	verdicts      []verdictRow
	recheckEvents []verdictEvent
	attemptRows   int
	rawInput      string
}

type verdictRow struct {
	seq     int
	verdict string
	points  int
	reason  string
}

type verdictEvent struct {
	AttemptID       string `json:"attempt_id"`
	Origin          string `json:"origin"`
	Verdict         string `json:"verdict"`
	Points          int    `json:"points"`
	CorrectionID    int    `json:"correction_id"`
	PreviousVerdict string `json:"previous_verdict"`
	PreviousPoints  int    `json:"previous_points"`
}

func rescore(t *testing.T, answer string, manual bool) scoredCase {
	t.Helper()
	old := answerrules.Score(formatInteger, oldKey, answer, nil)
	next := answerrules.Score(formatInteger, newKey, answer, nil)
	if old.Status != "scored" || next.Status != "scored" {
		t.Fatalf("ответ %q не оценён: %+v %+v", answer, old, next)
	}
	conn, look := newDB(t)
	attemptID := insertAttempt(t, conn, answer, old, manual)
	before := loadVerdicts(t, look, attemptID)

	accepted, err := recheck.Accept(context.Background(), conn, correctionID, []string{variantID})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if accepted.State != "pending" || accepted.CorrectionID != correctionID {
		t.Fatalf("приём %+v", accepted)
	}
	if batchCount(t, look, correctionID) != 1 {
		t.Fatal("приём не сохранил один пакет")
	}
	if got := loadBatch(t, look, correctionID); got != accepted {
		t.Fatalf("в базе %+v, Accept вернул %+v", got, accepted)
	}
	if rows := loadVerdicts(t, look, attemptID); !sameRows(rows, before) {
		t.Fatalf("приём изменил вердикты %+v", rows)
	}

	finished, err := recheck.Run(context.Background(), conn, correctionID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := loadBatch(t, look, correctionID); got != finished {
		t.Fatalf("в базе %+v, Run вернул %+v", got, finished)
	}
	raw, n := loadAttempt(t, look, attemptID)
	return scoredCase{
		attemptID:     attemptID,
		answer:        answer,
		old:           old,
		next:          next,
		accepted:      accepted,
		finished:      finished,
		before:        before,
		verdicts:      loadVerdicts(t, look, attemptID),
		recheckEvents: loadRecheckEvents(t, look, attemptID),
		attemptRows:   n,
		rawInput:      raw,
	}
}

func expectChange(t *testing.T, got scoredCase) {
	t.Helper()
	if got.finished.State != "done" || got.finished.Total != 1 || got.finished.Changed != 1 || got.finished.Processed != 1 || got.finished.SkippedManual != 0 {
		t.Fatalf("пакет %+v", got.finished)
	}
	if got.finished.Cursor != got.attemptID {
		t.Fatalf("cursor %q", got.finished.Cursor)
	}
	if len(got.verdicts) != 2 || got.verdicts[0] != got.before[0] {
		t.Fatalf("вердикты %+v, было %+v", got.verdicts, got.before)
	}
	if got.verdicts[0].verdict != got.old.Verdict || got.verdicts[0].points != got.old.Points || got.verdicts[0].reason != "" {
		t.Fatalf("прежний вердикт %+v, оценка %+v", got.verdicts[0], got.old)
	}
	if got.verdicts[1].seq <= got.verdicts[0].seq || got.verdicts[1].verdict != got.next.Verdict || got.verdicts[1].points != got.next.Points {
		t.Fatalf("новый вердикт %+v, оценка %+v", got.verdicts[1], got.next)
	}
	if len(got.recheckEvents) != 1 {
		t.Fatalf("событий recheck %d", len(got.recheckEvents))
	}
	event := got.recheckEvents[0]
	if event.AttemptID != got.attemptID || event.Origin != "recheck" || event.CorrectionID != correctionID {
		t.Fatalf("событие %+v", event)
	}
	if event.Verdict != got.next.Verdict || event.Points != got.next.Points {
		t.Fatalf("событие %+v, новая оценка %+v", event, got.next)
	}
	if event.PreviousVerdict != got.old.Verdict || event.PreviousPoints != got.old.Points {
		t.Fatalf("событие %+v, прежняя оценка %+v", event, got.old)
	}
	if got.attemptRows != 1 || got.rawInput != got.answer {
		t.Fatalf("попыток %d, ответ %q", got.attemptRows, got.rawInput)
	}
}

func insertVariant(t *testing.T, conn *pgx.Conn, version, key int) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id":            variantID,
		"subvariant":    "ct-math:M14.T07.S1",
		"statement":     "выдуманное условие",
		"key":           key,
		"minutes":       5,
		"theory_block":  "ct-math:M14.B01",
		"status":        "active",
		"answer_format": formatInteger,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(context.Background(), `
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, $2, $3, $4::jsonb)
	`, variantID, version, fmt.Sprintf("v%d-k%d", version, key), body)
	if err != nil {
		t.Fatalf("revision %d: %v", version, err)
	}
}

func insertAttempt(t *testing.T, conn *pgx.Conn, answer string, scored answerrules.Result, manual bool) string {
	t.Helper()
	insertVariant(t, conn, issuedVersion, oldKey)
	insertVariant(t, conn, correctionID, newKey)
	ceiling := answerrules.Score(formatInteger, oldKey, "12", nil)
	if ceiling.Status != "scored" || ceiling.Points < 1 {
		t.Fatalf("потолок формата %+v", ceiling)
	}
	attemptID := "t-" + answer
	if manual {
		attemptID += "-manual"
	}
	const (
		assignmentID = "a1"
		itemID       = "i1"
	)
	snap, err := json.Marshal(map[string]any{
		"items": []any{
			map[string]any{
				"item_id":       itemID,
				"variant_id":    variantID,
				"answer_format": formatInteger,
				"rules_version": rulesVersion,
				"key":           oldKey,
				"statement":     "выдуманное условие",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		)
		VALUES ($1, 'student-1', 'issued', 'homework', $2, NULL, 'home', $3::jsonb)
	`, assignmentID, issuedVersion, snap); err != nil {
		t.Fatalf("assignment: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		VALUES ($1, $2, 'open', false, $3)
	`, assignmentID, itemID, ceiling.Points); err != nil {
		t.Fatalf("item: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempts (
			id, assignment_id, item_id, raw_input, source, place, attempt_no
		)
		VALUES ($1, $2, $3, $4, 'homework', 'home', 1)
	`, attemptID, assignmentID, itemID, answer); err != nil {
		t.Fatalf("attempt: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		VALUES ($1, 1, $2, $3)
	`, attemptID, scored.Verdict, scored.Points); err != nil {
		t.Fatalf("verdict: %v", err)
	}
	recorded, err := json.Marshal(map[string]any{
		"attempt_id":       attemptID,
		"assignment_id":    assignmentID,
		"item_id":          itemID,
		"variant_id":       variantID,
		"catalog_version":  issuedVersion,
		"raw_input":        answer,
		"normalized_input": scored.Normalized,
		"attempt_no":       1,
		"verdict":          scored.Verdict,
		"points":           scored.Points,
		"source":           "homework",
		"place":            "home",
		"rules_version":    rulesVersion,
		"idempotency_key":  "idem-" + attemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ('attempt_recorded', $1::jsonb)
	`, recorded); err != nil {
		t.Fatalf("attempt_recorded: %v", err)
	}
	if !manual {
		return attemptID
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points, reason)
		VALUES ($1, 2, $2, $3, $4)
	`, attemptID, manualVerdict, manualPoints, manualReason); err != nil {
		t.Fatalf("manual verdict: %v", err)
	}
	manualEvent, err := json.Marshal(map[string]any{
		"attempt_id": attemptID,
		"origin":     "manual",
		"verdict":    manualVerdict,
		"points":     manualPoints,
		"reason":     manualReason,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('attempt_verdict', $1::jsonb)
	`, manualEvent); err != nil {
		t.Fatalf("manual event: %v", err)
	}
	request, err := json.Marshal(map[string]any{
		"attempt_id": attemptID,
		"verdict":    manualVerdict,
		"points":     manualPoints,
		"reason":     manualReason,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('manual_verdict', $1::jsonb)
	`, request); err != nil {
		t.Fatalf("manual_verdict: %v", err)
	}
	return attemptID
}

func batchCount(t *testing.T, conn *pgx.Conn, correction int) int {
	t.Helper()
	var n int
	err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.recheck_batch WHERE correction_id = $1
	`, correction).Scan(&n)
	if err != nil {
		t.Fatalf("пакеты: %v", err)
	}
	return n
}

func batchTotal(t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	var n int
	err := conn.QueryRow(context.Background(), `SELECT count(*) FROM core.recheck_batch`).Scan(&n)
	if err != nil {
		t.Fatalf("пакеты: %v", err)
	}
	return n
}

func loadBatch(t *testing.T, conn *pgx.Conn, correction int) recheck.State {
	t.Helper()
	var got recheck.State
	err := conn.QueryRow(context.Background(), `
		SELECT correction_id, state, total, processed, changed, skipped_manual,
		       COALESCE(cursor, '')
		FROM core.recheck_batch
		WHERE correction_id = $1
	`, correction).Scan(
		&got.CorrectionID, &got.State, &got.Total, &got.Processed,
		&got.Changed, &got.SkippedManual, &got.Cursor,
	)
	if err != nil {
		t.Fatalf("состояние пакета: %v", err)
	}
	return got
}

func loadVerdicts(t *testing.T, conn *pgx.Conn, attemptID string) []verdictRow {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT seq, verdict, points, reason
		FROM core.attempt_verdict
		WHERE attempt_id = $1
		ORDER BY seq
	`, attemptID)
	if err != nil {
		t.Fatalf("вердикты: %v", err)
	}
	defer rows.Close()
	var out []verdictRow
	for rows.Next() {
		var row verdictRow
		var reason *string
		if err := rows.Scan(&row.seq, &row.verdict, &row.points, &reason); err != nil {
			t.Fatal(err)
		}
		if reason != nil {
			row.reason = *reason
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func loadRecheckEvents(t *testing.T, conn *pgx.Conn, attemptID string) []verdictEvent {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT payload
		FROM core.journal_event
		WHERE kind = 'attempt_verdict'
		  AND payload->>'attempt_id' = $1
		  AND payload->>'origin' = 'recheck'
		ORDER BY journal_seq
	`, attemptID)
	if err != nil {
		t.Fatalf("события: %v", err)
	}
	defer rows.Close()
	var out []verdictEvent
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var event verdictEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func loadAttempt(t *testing.T, conn *pgx.Conn, attemptID string) (string, int) {
	t.Helper()
	var raw string
	var n int
	err := conn.QueryRow(context.Background(), `
		SELECT raw_input FROM core.attempts WHERE id = $1
	`, attemptID).Scan(&raw)
	if err != nil {
		t.Fatalf("попытка: %v", err)
	}
	err = conn.QueryRow(context.Background(), `SELECT count(*) FROM core.attempts`).Scan(&n)
	if err != nil {
		t.Fatalf("попытка: %v", err)
	}
	return raw, n
}

func sameRows(got, want []verdictRow) bool {
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

func newDB(t *testing.T) (*pgx.Conn, *pgx.Conn) {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	return openConn(t, dsn), openConn(t, dsn)
}

func openConn(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			t.Errorf("conn close: %v", closeErr)
		}
	})
	return conn
}

var (
	migrateOnce sync.Once
	migrateExe  string
	migrateErr  error
)

func migrateUp(t *testing.T, dsn string) {
	t.Helper()
	exe := migrateBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = moduleDir(t)
	cmd.Env = append(withoutEnv(os.Environ(), "DATABASE_URL"), "DATABASE_URL="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
}

func migrateBinary(t *testing.T) string {
	t.Helper()
	migrateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ai-mentor-migrate-")
		if err != nil {
			migrateErr = err
			return
		}
		exe := filepath.Join(dir, "migrate")
		mod, err := findModule()
		if err != nil {
			migrateErr = err
			return
		}
		cmd := exec.Command("go", "build", "-mod=readonly", "-o", exe, "./cmd/migrate")
		cmd.Dir = mod
		out, err := cmd.CombinedOutput()
		if err != nil {
			migrateErr = fmt.Errorf("%w\n%s", err, out)
			return
		}
		migrateExe = exe
	})
	if migrateErr != nil {
		t.Fatal(migrateErr)
	}
	return migrateExe
}

func moduleDir(t *testing.T) string {
	t.Helper()
	dir, err := findModule()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func findModule() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		main := filepath.Join(dir, "cmd", "migrate", "main.go")
		if isFile(mod) && isFile(main) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("модуль core не найден")
		}
		dir = parent
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
