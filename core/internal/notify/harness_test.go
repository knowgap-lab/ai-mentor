package notify_test

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/notify"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
// Транспорт в тестах — заглушка: сеть и api.telegram.org не вызываются.
// База поднимается через dbtest. Нет Docker — тест пропускается там.
//
//	Enqueue(ctx, tx, message) error
//	Dispatch(ctx, conn, now, transport) error
//	CancelPending(ctx, tx, studentID) error
//	Unreachable(ctx, conn, recipientID) (bool, error)
//
// Message: ID, Kind, DedupKey, RecipientKind, RecipientID, Template,
// Params, CreatedAt, NextAttemptAt.
// RecipientKind — tutor, student или ops. Kind пишется в колонку kind.
// Params — map[string]any. Ключи display_name, telegram_id, name, text
// дают *Error с полем Code outbox_params_invalid. Строка не коммитится.
// *Error.Error возвращает Code.
// Повтор Enqueue с тем же DedupKey возвращает nil и не меняет первую строку.
// Нулевой CreatedAt заменяется временем базы. Заданный пишется как есть.
// Нулевой NextAttemptAt — строка сразу доступна отправителю.
// Заданный NextAttemptAt пишется как есть.
// Колонки id, kind, payload остаются. INSERT только этих трёх колонок жив.
//
// Transport.Send(ctx, Delivery) error.
// Delivery: RecipientKind, RecipientID, ChatID, Text, ParseMode.
// ParseMode пустой. Ученику ChatID берётся из core.student_pii.telegram_id,
// репетитору — из core.user_pii.telegram_id, оба в момент Dispatch.
// Неизвестный получатель: Unreachable == false, ошибка nil.
// {display_name} берётся из core.student_pii по params["student_id"]
// в момент Dispatch. {items} — params["items"] как []any строк, склеенные
// через перевод строки. Если строк больше 20, остаются первые 20 и строка
// «и ещё N» (N = len-20) в том же списке. {link} и прочие {имя} —
// строковые params. Шаблон без фигурных скобок уходит в Text как есть.
// Собранный текст не попадает в core.outbox и core.journal_event.
//
// Ошибка транспорта не возвращается из Dispatch: строка остаётся в outbox
// со новым состоянием, а Dispatch возвращает nil. Ненулевая ошибка Dispatch —
// только сбой самой базы.
// Ошибка транспорта: *Failure (метод Error) либо любая другая error.
// Status 0 и любая не-*Failure — сеть. Status 500…599 — тоже сеть.
// Пауза 30s * 2^(номер-1), номер попытки с 1. После 5-й неудачи state=failed.
// Status 429: next_attempt_at = now+RetryAfter, attempts не растёт.
// Status 400 и 403: state=failed, last_error ровно unreachable,
// Unreachable(получатель) становится true, повтора нет.
// Успех: state=sent, sent_at равен аргументу now.
// Часы правил — аргумент now.
// За вызов — не больше одной строки каждого получателя и не больше 20
// отправок. Шедшая сверх лимита остаётся pending, attempts не растёт,
// next_attempt_at = now+1s. Повторный Dispatch с тем же now этот срок не двигает.
// Следующая отправка в тот же чат доступна при now >= прежняя sent_at+1s.
//
// CancelPending в переданной транзакции переводит в cancelled все pending,
// где получатель — этот ученик либо params.student_id равен ему.
// sent и failed не меняются. Вызов после отмены новую строку не отменяет.

// ВОПРОС: синтаксис шаблона в спецификации не задан, NT-R23 указывает файлы templates/notifications/*.md. Тесты фиксируют подстановки в тексте template. Файлы шаблонов не читаю.
// ВОПРОС: список ровно из 20 строк без «и ещё» не проверяю. Проверяю 80: первые 20, строка «и ещё 60» и ссылка.
// ВОПРОС: таблица ошибок требует журнал и тревогу при outbox_params_invalid, бриф говорит, что вставка ничего не пишет. Проверяю код *Error и отсутствие строки outbox.
// ВОПРОС: NT-R8 называет журнал ошибок. Проверяю непустой last_error без текста сообщения и без ПДн. Отдельную строку journal_event или ops_alert не требую.
// ВОПРОС: пометка «не доставлено» в кабинете. Проверяю Unreachable(получатель) и last_error ровно unreachable. Экран кабинета не открываю. Строка выдачи остаётся issued.
// ВОПРОС: снятие unreachable после следующей успешной доставки не проверяю.
// ВОПРОС: telegram_id ученика есть и в student_pii, и в user_pii. Для получателя-ученика жду id из student_pii.
// ВОПРОС: вложенный ключ и другой регистр (Display_Name) не проверяю. Ключи answer и body бриф не запрещает.
// ВОПРОС: две строки с одинаковым created_at — порядок не проверяю.
// ВОПРОС: статус транспорта кроме 0, 400, 403, 429 и 500…599 не проверяю. retry_after равный 0 не проверяю.
// ВОПРОС: число json в params в текст не подставляю, баллы в NT-R5 — строки.
// ВОПРОС: остаётся ли транзакция рабочей после outbox_params_invalid. Тест откатывает её и проверяет, что строки нет.
// ВОПРОС: attempts при 400 и 403 не проверяю: важен state failed и отсутствие повтора.

const (
	tutorID         = "tutor-demo"
	studentID       = "student-demo"
	tutorTelegram   = "900001"
	studentTelegram = "900002"
)

var _ notify.Transport = (*scriptTransport)(nil)

type scriptTransport struct {
	mu     sync.Mutex
	sends  []notify.Delivery
	decide func(notify.Delivery) error
}

func (s *scriptTransport) Send(ctx context.Context, d notify.Delivery) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends = append(s.sends, d)
	if s.decide != nil {
		return s.decide(d)
	}
	return nil
}

func (s *scriptTransport) got() []notify.Delivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]notify.Delivery, len(s.sends))
	copy(out, s.sends)
	return out
}

type outboxRow struct {
	ID            string
	Kind          string
	Payload       string
	DedupKey      string
	RecipientKind string
	RecipientID   string
	Template      string
	Params        string
	State         string
	Attempts      int
	NextAttemptAt time.Time
	CreatedAt     time.Time
	SentAt        *time.Time
	LastError     *string
}

func newDB(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	return openConn(t, dsn), dsn
}

func openConn(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := conn.Exec(ctx, `SET TIME ZONE 'UTC'`); err != nil {
		t.Fatalf("timezone: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return conn
}

func insertTutor(t *testing.T, conn *pgx.Conn, id, telegram string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.app_user (id, role, status) VALUES ($1, 'tutor', 'active')
	`, id); err != nil {
		t.Fatalf("репетитор: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, telegram); err != nil {
		t.Fatalf("telegram репетитора: %v", err)
	}
}

func insertStudent(t *testing.T, conn *pgx.Conn, id, tutor, name, userTelegram, piiTelegram string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, id, tutor); err != nil {
		t.Fatalf("ученик: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, userTelegram); err != nil {
		t.Fatalf("telegram ученика: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, $2, $3)
	`, id, name, piiTelegram); err != nil {
		t.Fatalf("ПДн: %v", err)
	}
}

func seedDemo(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	insertTutor(t, conn, tutorID, tutorTelegram)
	insertStudent(t, conn, studentID, tutorID, "Маша", studentTelegram, studentTelegram)
}

func commitEnqueue(t *testing.T, conn *pgx.Conn, msg notify.Message) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := notify.Enqueue(ctx, tx, msg); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Enqueue %s: %v", msg.ID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func dispatchAt(t *testing.T, conn *pgx.Conn, now time.Time, tr notify.Transport) {
	t.Helper()
	if err := notify.Dispatch(context.Background(), conn, now, tr); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
}

func rowByID(t *testing.T, conn *pgx.Conn, id string) outboxRow {
	t.Helper()
	var row outboxRow
	err := conn.QueryRow(context.Background(), `
		SELECT id, kind, payload::text, dedup_key, recipient_kind, recipient_id,
		       template, params::text, state, attempts, next_attempt_at, created_at,
		       sent_at, last_error
		FROM core.outbox
		WHERE id = $1
	`, id).Scan(
		&row.ID, &row.Kind, &row.Payload, &row.DedupKey, &row.RecipientKind, &row.RecipientID,
		&row.Template, &row.Params, &row.State, &row.Attempts, &row.NextAttemptAt, &row.CreatedAt,
		&row.SentAt, &row.LastError,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("нет строки %s", id)
	}
	if err != nil {
		t.Fatalf("строка %s: %v", id, err)
	}
	return row
}

func countOutbox(t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM core.outbox`).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

func storedText(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	ctx := context.Background()
	var b strings.Builder
	rows, err := conn.Query(ctx, `
		SELECT coalesce(id, '') || E'\n' || coalesce(kind, '') || E'\n'
			|| coalesce(payload::text, '') || E'\n' || coalesce(dedup_key, '') || E'\n'
			|| coalesce(recipient_kind, '') || E'\n' || coalesce(recipient_id, '') || E'\n'
			|| coalesce(template, '') || E'\n' || coalesce(params::text, '') || E'\n'
			|| coalesce(state, '') || E'\n' || coalesce(attempts::text, '') || E'\n'
			|| coalesce(last_error, '')
		FROM core.outbox
	`)
	if err != nil {
		t.Fatalf("outbox text: %v", err)
	}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	events, err := conn.Query(ctx, `
		SELECT coalesce(kind, '') || E'\n' || coalesce(payload::text, '')
		FROM core.journal_event
	`)
	if err != nil {
		t.Fatalf("journal text: %v", err)
	}
	defer events.Close()
	for events.Next() {
		var line string
		if err := events.Scan(&line); err != nil {
			t.Fatal(err)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := events.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func requireMissing(t *testing.T, conn *pgx.Conn, parts ...string) {
	t.Helper()
	blob := storedText(t, conn)
	for _, part := range parts {
		if part != "" && strings.Contains(blob, part) {
			t.Fatalf("в outbox или журнале сохранено %q", part)
		}
	}
}

func sameInstant(a, b time.Time) bool {
	return a.UTC().Truncate(time.Microsecond).Equal(b.UTC().Truncate(time.Microsecond))
}

func insertJournal(t *testing.T, tx pgx.Tx, kind string) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), `
		INSERT INTO core.journal_event (kind, payload) VALUES ($1, '{}'::jsonb)
	`, kind); err != nil {
		t.Fatalf("journal: %v", err)
	}
}

func insertHomework(t *testing.T, conn *pgx.Conn, id, student string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, place, snapshot
		)
		VALUES ($1, $2, 'issued', 'homework', 1, 'home', '{"statement":"демо"}'::jsonb)
	`, id, student); err != nil {
		t.Fatalf("выдача: %v", err)
	}
}

func message(id, dedup, kind, recipientKind, recipientID, template string, params map[string]any, created, next time.Time) notify.Message {
	return notify.Message{
		ID:            id,
		Kind:          kind,
		DedupKey:      dedup,
		RecipientKind: recipientKind,
		RecipientID:   recipientID,
		Template:      template,
		Params:        params,
		CreatedAt:     created,
		NextAttemptAt: next,
	}
}

func demoDue(id, dedup, template string, at time.Time) notify.Message {
	return message(id, dedup, "homework_submitted", "tutor", tutorID, template,
		map[string]any{"student_id": studentID}, at, at)
}

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var nerr *notify.Error
	if !errors.As(err, &nerr) || nerr == nil || nerr.Code != code {
		t.Fatalf("код %s, ошибка %v", code, err)
	}
}

func expectUnique(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("ожидалось уникальное ограничение, ошибка %v", err)
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
