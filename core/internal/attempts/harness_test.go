package attempts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/knowgap-lab/ai-mentor/core/internal/attempts"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Seed(ctx, conn, given) (World, error)
//	ItemResult(ctx, conn, assignmentID, itemID string) (ItemResult, error)
//	RecordAttempt(ctx, conn, assignmentID string) error
//	ResolveDisputeByCorrection(ctx, conn, correctionID int) error
//	Anonymise(ctx, conn, studentID string) error
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//
// Config содержит только DSN.
// World: AssignmentID, ItemID, AttemptID, LessonID, StudentID, ActorTelegramID, ActorRole.
// given — те же поля, что в attempts.examples.yaml. Числа передаются как int,
// вложенные объекты как map[string]any, списки как []any.
// Выдача из given.assignment получает id a1 (тело at-13b).
// Если в given есть item, заполнены AssignmentID и ItemID.
// Если ручной вердикт или given.attempt относятся к одной попытке, заполнен AttemptID.
// Черновик адресуется LessonID.
// ActorRole — student или tutor, ActorTelegramID есть в core.user_pii.
// Без actor в given актёр такой, кому операция разрешена: репетитор для
// manualVerdict, patchHomeworkDraft, withdrawAssignment и extendDeadline,
// ученик-владелец для остальных. Сеанс — cookie __Host-session после POST /auth/dev.
// ItemResult.Verdict, FirstAttemptPoints, TotalPoints, EndsAtAttempt.
// Неверный ответ в этих тестах — 0 баллов, верный ответ части В — 2 из 2.
// Anonymise удаляет строки core.student_pii и core.student_pseudonym этого ученика
// и не меняет число строк core.attempts, core.attempt_verdict, core.journal_event
// и core.observation. Для at-25 Seed заранее пишет хотя бы одну строку журнала,
// одну строку student_pii и одну строку student_pseudonym.
// Для kind=mock Seed кладёт хотя бы один уже оценённый ответ: GET до «Сдать»
// не показывает вердикт, ответ «Сдать» его показывает.
// payload в core.journal_event — объект события, item_id лежит на верхнем уровне.
// Снимок core.assignment.snapshot хранит statement. При catalog_version 6 Seed
// пишет «условие-в6»; getAssignment возвращает эту строку и при catalog_latest 8.
// core.attempts хранит source и place. RecordAttempt для kind checkpoint пишет
// source checkpoint и place lesson.
// ResolveDisputeByCorrection ставит спор в resolved, пишет resolved_by.correction_id
// и добавляет одну строку attempt_verdict. HTTP перепроверки нет. Seed спор не закрывает.
//
// Вопросы, которые тесты сознательно не закрывают.
//
// ВОПРОС: телефон в споре. Бриф G2 блокирует 9 и больше цифр, PV-R24 — 7 и больше. Строку at-18b тестирую: в ней больше обоих порогов. Границу в 7 и 8 цифр не тестирую.
// ВОПРОС: at-25 и PV-R19. Пример требует journal_rows_changed 0, PV-R19 пишет событие student_anonymised. Сумма строк четырёх таблиц журнала из брифа не должна измениться, поэтому новое событие в journal_event тест не пропустит. Событие вне этих таблиц не проверяю.

const (
	appOrigin     = "http://localhost"
	sessionCookie = "__Host-session"
	assignmentID  = "a1"

	deadlineLater   = "2026-11-22T00:00:00Z"
	deadlineCurrent = "2026-11-20T00:00:00Z"
	deadlineSooner  = "2026-11-18T00:00:00Z"
	statementV6     = "условие-в6"
)

var journalTables = []string{
	"core.attempts",
	"core.attempt_verdict",
	"core.journal_event",
	"core.observation",
}

func TestAT_R1_at01(t *testing.T) {
	conn := newDB(t)
	for _, table := range journalTables {
		if !hasPrivilege(t, conn, table, "SELECT") || !hasPrivilege(t, conn, table, "INSERT") {
			t.Fatalf("%s: нет SELECT или INSERT у core_app", table)
		}
		if hasPrivilege(t, conn, table, "UPDATE") || hasPrivilege(t, conn, table, "DELETE") {
			t.Fatalf("%s: у core_app есть UPDATE или DELETE", table)
		}
	}
	for _, statement := range []string{
		`UPDATE attempts SET raw_input = 'x'`,
		`DELETE FROM attempts`,
		`UPDATE attempt_verdict SET verdict = 'wrong'`,
		`DELETE FROM attempt_verdict`,
		`UPDATE journal_event SET kind = 'x'`,
		`DELETE FROM journal_event`,
		`UPDATE observation SET revision = revision`,
		`DELETE FROM observation`,
	} {
		expectDenied(t, conn, statement)
	}
}

type harness struct {
	t     *testing.T
	dsn   string
	conn  *pgx.Conn
	svc   *attempts.Service
	world attempts.World
	token string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			t.Errorf("conn close: %v", closeErr)
		}
	})
	svc, err := attempts.Open(ctx, attempts.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := svc.Close(); closeErr != nil {
			t.Errorf("close: %v", closeErr)
		}
	})
	return &harness{t: t, dsn: dsn, conn: conn, svc: svc}
}

func newDB(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
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

func (h *harness) seed(given map[string]any) attempts.World {
	h.t.Helper()
	world, err := attempts.Seed(context.Background(), h.conn, given)
	if err != nil {
		h.t.Fatalf("Seed: %v", err)
	}
	if _, ok := given["assignment"]; ok && world.AssignmentID != assignmentID {
		h.t.Fatalf("id выдачи %s, в at-13b тело ждёт %s", world.AssignmentID, assignmentID)
	}
	if world.ActorTelegramID == "" || (world.ActorRole != "student" && world.ActorRole != "tutor") {
		h.t.Fatalf("актёр telegram=%q role=%q", world.ActorTelegramID, world.ActorRole)
	}
	h.world = world
	h.token = login(h.t, h.dsn, world.ActorTelegramID, world.ActorRole)
	return world
}

func (h *harness) call(method, path string, body any, header map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: h.token})
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("Origin", appOrigin)
		req.Header.Set("X-CSRF-Token", h.token)
	}
	for key, value := range header {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *harness) assignment() string {
	h.t.Helper()
	if h.world.AssignmentID == "" {
		h.t.Fatal("Seed не заполнил выдачу")
	}
	return h.world.AssignmentID
}

func (h *harness) item() string {
	h.t.Helper()
	if h.world.ItemID == "" {
		h.t.Fatal("Seed не заполнил элемент")
	}
	return h.world.ItemID
}

func (h *harness) attempt() string {
	h.t.Helper()
	if h.world.AttemptID == "" {
		h.t.Fatal("Seed не заполнил попытку")
	}
	return h.world.AttemptID
}

func (h *harness) lesson() string {
	h.t.Helper()
	if h.world.LessonID == "" {
		h.t.Fatal("Seed не заполнил урок черновика")
	}
	return h.world.LessonID
}

func (h *harness) itemResult() attempts.ItemResult {
	h.t.Helper()
	res, err := attempts.ItemResult(context.Background(), h.conn, h.assignment(), h.item())
	if err != nil {
		h.t.Fatalf("ItemResult: %v", err)
	}
	return res
}

func (h *harness) anonymise() {
	h.t.Helper()
	if h.world.StudentID == "" {
		h.t.Fatal("Seed не заполнил ученика")
	}
	if err := attempts.Anonymise(context.Background(), h.conn, h.world.StudentID); err != nil {
		h.t.Fatalf("Anonymise: %v", err)
	}
}

func (h *harness) recordAttempt() {
	h.t.Helper()
	if err := attempts.RecordAttempt(context.Background(), h.conn, h.assignment()); err != nil {
		h.t.Fatalf("RecordAttempt: %v", err)
	}
}

func (h *harness) resolveByCorrection(correctionID int) {
	h.t.Helper()
	if err := attempts.ResolveDisputeByCorrection(context.Background(), h.conn, correctionID); err != nil {
		h.t.Fatalf("ResolveDisputeByCorrection: %v", err)
	}
}

func (h *harness) snapshot(id string) (int, []byte) {
	h.t.Helper()
	var version int
	var raw []byte
	err := h.conn.QueryRow(context.Background(),
		`SELECT catalog_version, snapshot FROM core.assignment WHERE id = $1`, id).Scan(&version, &raw)
	if err != nil {
		h.t.Fatalf("снимок: %v", err)
	}
	return version, raw
}

func (h *harness) count(query string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		h.t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (h *harness) text(query string, args ...any) string {
	h.t.Helper()
	var value string
	if err := h.conn.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		h.t.Fatalf("%s: %v", query, err)
	}
	return value
}

func (h *harness) texts(query string, args ...any) []string {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), query, args...)
	if err != nil {
		h.t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (h *harness) kinds() []string {
	h.t.Helper()
	return h.texts(`SELECT kind FROM core.journal_event ORDER BY journal_seq`)
}

func (h *harness) expectAppended(before []string, want ...string) {
	h.t.Helper()
	after := h.kinds()
	if len(after) < len(before) || !equalStrings(after[:len(before)], before) {
		h.t.Fatalf("прежние события %v стали %v", before, after)
	}
	if got := after[len(before):]; !equalStrings(got, want) {
		h.t.Fatalf("новые события %v, нужны %v", got, want)
	}
}

func (h *harness) verdicts(attemptID string) []string {
	h.t.Helper()
	return h.texts(`SELECT verdict FROM core.attempt_verdict WHERE attempt_id = $1 ORDER BY seq`, attemptID)
}

func (h *harness) assignmentState(id string) string {
	h.t.Helper()
	return h.text(`SELECT state FROM core.assignment WHERE id = $1`, id)
}

func (h *harness) deadlineIs(id, value string) bool {
	h.t.Helper()
	var ok bool
	err := h.conn.QueryRow(context.Background(),
		`SELECT deadline = $2::timestamptz FROM core.assignment WHERE id = $1`,
		id, value).Scan(&ok)
	if err != nil {
		h.t.Fatalf("срок: %v", err)
	}
	return ok
}

func (h *harness) journalRows() int {
	h.t.Helper()
	return h.count(`SELECT
		(SELECT count(*) FROM core.attempts) +
		(SELECT count(*) FROM core.attempt_verdict) +
		(SELECT count(*) FROM core.journal_event) +
		(SELECT count(*) FROM core.observation)`)
}

func (h *harness) queueJobs() int {
	h.t.Helper()
	var schema bool
	err := h.conn.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'queue')`).Scan(&schema)
	if err != nil {
		h.t.Fatal(err)
	}
	if !schema {
		return 0
	}
	rows, err := h.conn.Query(context.Background(), `
		SELECT format('%I.%I', n.nspname, c.relname)
		FROM pg_class AS c
		JOIN pg_namespace AS n ON n.oid = c.relnamespace
		WHERE n.nspname = 'queue' AND c.relkind = 'r'
		ORDER BY c.relname`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			h.t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		h.t.Fatal(err)
	}
	rows.Close()
	total := 0
	for _, name := range names {
		total += h.count("SELECT count(*) FROM " + name)
	}
	return total
}

type obsRow struct {
	revision int
	first    int
	total    int
	max      int
}

func (h *harness) observations() []obsRow {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), `
		SELECT revision, first_attempt_points, total_points, max_points
		FROM core.observation
		ORDER BY revision`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []obsRow
	for rows.Next() {
		var row obsRow
		if err := rows.Scan(&row.revision, &row.first, &row.total, &row.max); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (h *harness) draftVariantIDs() []string {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), `SELECT items FROM core.homework_draft`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var raws [][]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			h.t.Fatal(err)
		}
		raws = append(raws, raw)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	if len(raws) != 1 {
		h.t.Fatalf("черновиков %d", len(raws))
	}
	ids, err := variantIDs(raws[0])
	if err != nil {
		h.t.Fatalf("items черновика: %v", err)
	}
	return ids
}

func (h *harness) status(rec *httptest.ResponseRecorder, status int) {
	h.t.Helper()
	if rec.Code != status {
		h.t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
}

func (h *harness) expectCode(rec *httptest.ResponseRecorder, status int, code string) {
	h.t.Helper()
	h.status(rec, status)
	if got, _ := decodeError(h.t, rec.Body.Bytes()); got != code {
		h.t.Fatalf("код %s, нужен %s", got, code)
	}
}

func (h *harness) expectLeft(rec *httptest.ResponseRecorder, status int, code string, left int) {
	h.t.Helper()
	h.status(rec, status)
	got, details := decodeError(h.t, rec.Body.Bytes())
	if got != code {
		h.t.Fatalf("код %s, нужен %s", got, code)
	}
	if numberValue(h.t, details["left"]) != left {
		h.t.Fatalf("left = %#v, нужно %d", details["left"], left)
	}
}

func login(t *testing.T, dsn, telegramID, role string) string {
	t.Helper()
	svc, err := auth.Open(context.Background(), auth.Config{
		DSN:       dsn,
		Env:       "local",
		DevMode:   true,
		AppOrigin: appOrigin,
		Log:       io.Discard,
	})
	if err != nil {
		t.Fatalf("auth.Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := svc.Close(); closeErr != nil {
			t.Errorf("auth close: %v", closeErr)
		}
	})
	body, err := json.Marshal(map[string]string{
		"telegram_id": telegramID,
		"role":        role,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/dev", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", appOrigin)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("вход %s %s: %d %s", telegramID, role, rec.Code, rec.Body.Bytes())
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookie && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatal("нет cookie сеанса")
	return ""
}

func hasPrivilege(t *testing.T, conn *pgx.Conn, table, priv string) bool {
	t.Helper()
	var ok bool
	err := conn.QueryRow(context.Background(),
		`SELECT has_table_privilege('core_app', $1, $2)`, table, priv).Scan(&ok)
	if err != nil {
		t.Fatalf("права %s %s: %v", table, priv, err)
	}
	return ok
}

func expectDenied(t *testing.T, conn *pgx.Conn, statement string) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO core`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE core_app`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, statement)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatalf("%s выполнился", statement)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("%s: %v", statement, err)
	}
}

func decodeError(t *testing.T, raw []byte) (string, map[string]any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("Error: %v тело %s", err, raw)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("лишние данные в ошибке: %s", raw)
	}
	if body.Code == "" {
		t.Fatalf("нет code: %s", raw)
	}
	if len(bytes.TrimSpace(body.Details)) == 0 || string(body.Details) == "null" {
		return body.Code, nil
	}
	var details map[string]any
	if err := json.Unmarshal(body.Details, &details); err != nil {
		t.Fatalf("details: %v тело %s", err, raw)
	}
	return body.Code, details
}

func numberValue(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			t.Fatalf("не целое %#v", v)
		}
		return int(n)
	case int:
		return n
	default:
		t.Fatalf("не число %#v", v)
		return 0
	}
}

func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("json: %v тело %s", err, raw)
	}
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

type draftItem struct {
	VariantID    string `json:"variant_id"`
	TypeID       string `json:"type_id"`
	AnswerFormat string `json:"answer_format"`
	Minutes      *int   `json:"minutes"`
	Source       string `json:"source"`
	Statement    string `json:"statement"`
}

type homeworkDraft struct {
	LessonID      string          `json:"lesson_id"`
	Deadline      string          `json:"deadline"`
	Items         []draftItem     `json:"items"`
	Hints         []string        `json:"hints"`
	Warnings      json.RawMessage `json:"warnings"`
	DiscussTopics []string        `json:"discuss_topics"`
	Issued        bool            `json:"issued"`
}

func decodeDraft(t *testing.T, raw []byte) homeworkDraft {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body homeworkDraft
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("черновик: %v тело %s", err, raw)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("лишние данные в черновике: %s", raw)
	}
	return body
}

func variantIDs(raw []byte) ([]string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("items имеет тип %T", value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch row := item.(type) {
		case string:
			out = append(out, row)
		case map[string]any:
			id, _ := row["variant_id"].(string)
			if id == "" {
				return nil, fmt.Errorf("элемент без variant_id")
			}
			out = append(out, id)
		default:
			return nil, fmt.Errorf("элемент типа %T", item)
		}
	}
	return out, nil
}

func draftIDs(body homeworkDraft) []string {
	out := make([]string, 0, len(body.Items))
	for _, item := range body.Items {
		out = append(out, item.VariantID)
	}
	return out
}

type submitBody struct {
	AssignmentID string            `json:"assignment_id"`
	Answered     *int              `json:"answered"`
	Skipped      *int              `json:"skipped"`
	Items        []json.RawMessage `json:"items"`
	TotalPoints  *int              `json:"total_points"`
	MaxPoints    *int              `json:"max_points"`
}

func decodeSubmit(t *testing.T, raw []byte) submitBody {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body submitBody
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("итог: %v тело %s", err, raw)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("лишние данные в итоге: %s", raw)
	}
	if body.Items == nil {
		t.Fatalf("нет items: %s", raw)
	}
	return body
}

func hasKey(v any, key string) bool {
	switch node := v.(type) {
	case map[string]any:
		for name, child := range node {
			if name == key || hasKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if hasKey(child, key) {
				return true
			}
		}
	}
	return false
}

func hasScoringVerdict(v any) bool {
	switch node := v.(type) {
	case map[string]any:
		if verdict, ok := node["verdict"].(string); ok {
			switch verdict {
			case "correct", "partial", "wrong":
				return true
			}
		}
		for _, child := range node {
			if hasScoringVerdict(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if hasScoringVerdict(child) {
				return true
			}
		}
	}
	return false
}

func idem(key string) map[string]string {
	return map[string]string{"Idempotency-Key": key}
}

func assignmentPath(id, suffix string) string {
	return "/assignments/" + url.PathEscape(id) + suffix
}

func itemPath(assignmentID, itemID, suffix string) string {
	return "/assignments/" + url.PathEscape(assignmentID) + "/items/" + url.PathEscape(itemID) + suffix
}

func manualPath(attemptID string) string {
	return "/attempts/" + url.PathEscape(attemptID) + "/manual-verdict"
}

func draftPath(lessonID string) string {
	return "/lessons/" + url.PathEscape(lessonID) + "/homework-draft"
}

func equalStrings(got, want []string) bool {
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

func manualBody() map[string]any {
	return map[string]any{
		"verdict": "correct",
		"points":  2,
		"reason":  "эквивалентный ответ",
	}
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
			return "", errors.New("модуль core не найден")
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
