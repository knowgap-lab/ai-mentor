package telegram_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/telegram"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
// Сеть к Telegram эти тесты не открывают. База нужна только приглашениям,
// через dbtest: нет Docker — такой тест пропускается, Check всё равно идёт.
//
//	Check(data map[string]string, botToken string, now time.Time) error
//	Open(ctx, Config) (*Service, error)
//	(*Service).Close() error
//	(*Service).Create(ctx, studentID string, now time.Time) (token string, error)
//	(*Service).Preview(ctx, token string, now time.Time) error
//	(*Service).Exchange(ctx, token string, data map[string]string, now time.Time) error
//
// Config: DSN и Log. В Log токен приглашения не пишется.
// Check не пишет в базу. Отказ Check и Invite — *Error: HTTPStatus, Code,
// Error() равен Code, Body() — JSON схемы Error (code, message, details).
// hash не сошёлся — 401 login_invalid. now−auth_date > 300 с — 401 login_stale.
// Create возвращает сырой токен из 32 байт. В core.telegram_invite только
// SHA-256, student_id, expires_at = now+72ч, used_at NULL.
// Preview ничего не меняет. Обмен старше 72 часов и второй обмен — 410 invite_gone,
// строка core.session не появляется. Exchange читает из данных только id и
// пишет его в core.student_pii.telegram_id. Занятый id ученика или репетитора
// (core.user_pii) — 409 telegram_already_bound.
// Часы правил — аргумент now, не время машины.

// ВОПРОС: SHA-256 в token_hash считается от 32 сырых байт или от строки токена? Годятся оба.
// ВОПРОС: как 32 байта кодируются в токене Create? Годятся hex и base64url.
// ВОПРОС: ровно 72 часа приглашение ещё действует? Проверяю 71 час и 73 часа. Границу ровно 72 не проверяю.
// ВОПРОС: auth_date из будущего. Правило отвергает только now−auth_date > 300. Такой вход не отвергаю.
// ВОПРОС: одновременно неверный hash и auth_date старше 300 секунд. Какой код? Не проверяю.
// ВОПРОС: пустой или нечисловой auth_date у Check. Код не задан. Не проверяю.
// ВОПРОС: верхний регистр hex в hash. Тест кладёт нижний регистр.
// ВОПРОС: при telegram_already_bound гасится ли приглашение? used_at не проверяю. Проверяю 409 и что чужой telegram_id не перешёл.
// ВОПРОС: успешный обмен создаёт сеанс? AU-R17 в брифе нет. Сеанс не требую. При invite_gone число строк core.session не растёт.
// ВОПРОС: согласие, сообщение репетитору и JSON страницы GET бриф не задаёт. Не проверяю.
// ВОПРОС: перевыпуск приглашения (AU-R11) не в брифе. Не проверяю.
// ВОПРОС: telegram_id ученика после обмена жду в core.student_pii. Дубль в core.user_pii не запрещаю.
// ВОПРОС: криптостойкость 256 бит не измеряю. Проверяю 32 байта и что два токена различаются.
// ВОПРОС: invite_token, если ключ передан, входит в строку проверки: бриф говорит «все поля, кроме hash». Совпадает ли это с подписью виджета Telegram — в источниках не сведено, тест следует брифу.
// ВОПРОС: Create возвращает сырой токен, не URL. Поле url схемы Link не проверяю.
// ВОПРОС: неизвестный токен на Preview. Код не задан. Не проверяю.

const (
	slack      = 5 * time.Second
	inviteLife = 72 * time.Hour
)

type harness struct {
	t    *testing.T
	svc  *telegram.Service
	dsn  string
	logs *bytes.Buffer
}

type inviteRow struct {
	Hash    []byte
	Student string
	Expires time.Time
	Used    *time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := dbtest.Start(t)
	migrateUp(t, dsn)
	return openHarness(t, dsn)
}

func openHarness(t *testing.T, dsn string) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	log.SetOutput(logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	svc, err := telegram.Open(context.Background(), telegram.Config{
		DSN: dsn,
		Log: logs,
	})
	if err != nil {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		t.Fatalf("Open: %v", err)
	}
	h := &harness{t: t, svc: svc, dsn: dsn, logs: logs}
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
		if err := svc.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return h
}

func (h *harness) seedStudent(id, nick, telegramID string) {
	h.t.Helper()
	h.exec(`
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, $2, $3)
	`, id, nick, telegramID)
}

func (h *harness) seedTutor(id, telegramID string) {
	h.t.Helper()
	h.exec(`
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'tutor', NULL, 'active')
	`, id)
	h.exec(`
		INSERT INTO core.user_pii (user_id, telegram_id)
		VALUES ($1, $2)
	`, id, telegramID)
}

func (h *harness) create(student string, now time.Time) string {
	h.t.Helper()
	token, err := h.svc.Create(context.Background(), student, now)
	if err != nil || token == "" {
		h.t.Fatalf("Create: %v", err)
	}
	h.requireInvite(token, student, now)
	h.guardToken(token)
	return token
}

func (h *harness) exchange(student, nick, token string, now time.Time, telegramID string) {
	h.t.Helper()
	if err := h.svc.Exchange(context.Background(), token, loginData(telegramID), now); err != nil {
		h.t.Fatalf("Exchange: %v", err)
	}
	h.bound(student, nick, token, telegramID, now)
}

func (h *harness) bound(student, nick, token, telegramID string, now time.Time) {
	h.t.Helper()
	if got := h.telegramID(student); got != telegramID {
		h.t.Fatalf("telegram_id = %s, нужен %s", got, telegramID)
	}
	if got := h.displayName(student); got != nick {
		h.t.Fatalf("display_name = %s, нужно %s", got, nick)
	}
	row := h.inviteByToken(token)
	if row.Used == nil {
		h.t.Fatal("обмен не поставил used_at")
	}
	within(h.t, *row.Used, now, now, 0, slack, "used_at")
	h.guardToken(token)
	h.assertLoginSecretsHidden()
}

func (h *harness) requireInvite(token, student string, created time.Time) {
	h.t.Helper()
	row := h.inviteByToken(token)
	if row.Student != student {
		h.t.Fatalf("приглашение ученика %s, нужен %s", row.Student, student)
	}
	if row.Used != nil {
		h.t.Fatal("новое приглашение уже использовано")
	}
	within(h.t, row.Expires, created, created, inviteLife, slack, "expires_at")
}

func (h *harness) inviteByToken(token string) inviteRow {
	h.t.Helper()
	raw, err := decodeToken(token)
	if err != nil {
		h.t.Fatal("токен приглашения не равен 32 байтам")
	}
	var rows []inviteRow
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		scanned, err := conn.Query(ctx, `
			SELECT token_hash, student_id, expires_at, used_at
			FROM core.telegram_invite
		`)
		if err != nil {
			return err
		}
		defer scanned.Close()
		for scanned.Next() {
			var row inviteRow
			if err := scanned.Scan(&row.Hash, &row.Student, &row.Expires, &row.Used); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return scanned.Err()
	})
	var found inviteRow
	n := 0
	for _, row := range rows {
		if tokenIsHash(row.Hash, token, raw) {
			found = row
			n++
		}
	}
	if n != 1 {
		h.t.Fatalf("строк приглашения с хешем токена: %d", n)
	}
	return found
}

func (h *harness) telegramID(student string) string {
	h.t.Helper()
	return h.queryText(`SELECT telegram_id FROM core.student_pii WHERE student_id = $1`, student)
}

func (h *harness) displayName(student string) string {
	h.t.Helper()
	return h.queryText(`SELECT display_name FROM core.student_pii WHERE student_id = $1`, student)
}

func (h *harness) userTelegram(userID string) string {
	h.t.Helper()
	return h.queryText(`SELECT telegram_id FROM core.user_pii WHERE user_id = $1`, userID)
}

func (h *harness) sessions() int {
	h.t.Helper()
	return h.count(`SELECT count(*) FROM core.session`)
}

func (h *harness) snapshot() string {
	h.t.Helper()
	return strings.Join([]string{
		h.queryText(`
			SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.token_hash)::text, '[]')
			FROM core.telegram_invite t
		`),
		h.queryText(`
			SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.student_id)::text, '[]')
			FROM core.student_pii t
		`),
		h.queryText(`
			SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.user_id)::text, '[]')
			FROM core.user_pii t
		`),
		h.queryText(`
			SELECT coalesce(json_agg(row_to_json(t) ORDER BY t.token_hash)::text, '[]')
			FROM core.session t
		`),
	}, "\n")
}

func (h *harness) unchanged(token, before string) {
	h.t.Helper()
	after := h.snapshot()
	if after != before {
		h.t.Fatalf("строки изменились\nбыло: %s\nстало: %s", scrub(before, token), scrub(after, token))
	}
}

func (h *harness) guardToken(token string) {
	h.t.Helper()
	raw, err := decodeToken(token)
	if err != nil {
		h.t.Fatal("токен приглашения не равен 32 байтам")
	}
	for _, secret := range tokenForms(token, raw) {
		if secret != "" && strings.Contains(h.logs.String(), secret) {
			h.t.Fatal("журнал содержит токен приглашения")
		}
	}
	h.assertNoRawToken(token, raw)
}

func (h *harness) assertLoginSecretsHidden() {
	h.t.Helper()
	assertSecretsHidden(h.t, h.logs.String(), loginSecrets())
	h.assertTextAbsent(loginSecrets())
}

func (h *harness) assertNoRawToken(token string, raw []byte) {
	h.t.Helper()
	forms := tokenForms(token, raw)
	h.forColumns(func(table, name, udt string) {
		switch udt {
		case "text", "varchar", "bpchar", "json", "jsonb", "name":
			for _, secret := range forms {
				if secret == "" {
					continue
				}
				//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
				n := h.count(fmt.Sprintf(
					`SELECT count(*) FROM core.%s WHERE strpos(%s::text, $1) > 0`,
					table, name,
				), secret)
				if n != 0 {
					h.t.Fatalf("сырой токен записан в core.%s.%s", table, name)
				}
			}
		case "bytea":
			//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
			n := h.count(fmt.Sprintf(
				`SELECT count(*) FROM core.%s WHERE %s = $1`,
				table, name,
			), raw)
			if n != 0 {
				h.t.Fatalf("сырые байты токена записаны в core.%s.%s", table, name)
			}
		}
	})
}

func (h *harness) assertTextAbsent(secrets []string) {
	h.t.Helper()
	h.forColumns(func(table, name, udt string) {
		var query string
		switch udt {
		case "text", "varchar", "bpchar", "json", "jsonb", "name":
			//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
			query = fmt.Sprintf(
				`SELECT count(*) FROM core.%s WHERE strpos(%s::text, $1) > 0`,
				table, name,
			)
		case "bytea":
			//nolint:gosec // G201: имена сверены с ^[a-z_]+$ после information_schema.
			query = fmt.Sprintf(
				`SELECT count(*) FROM core.%s WHERE strpos(encode(%s, 'escape'), $1) > 0`,
				table, name,
			)
		default:
			return
		}
		for _, secret := range secrets {
			if secret == "" {
				continue
			}
			if h.count(query, secret) != 0 {
				h.t.Fatalf("поле данных входа записано в core.%s.%s", table, name)
			}
		}
	})
}

func (h *harness) forColumns(fn func(table, name, udt string)) {
	h.t.Helper()
	type column struct {
		table string
		name  string
		udt   string
	}
	var cols []column
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		scanned, err := conn.Query(ctx, `
			SELECT table_name, column_name, udt_name
			FROM information_schema.columns
			WHERE table_schema = 'core'
			  AND table_name IN ('telegram_invite', 'student_pii', 'user_pii', 'session', 'app_user')
		`)
		if err != nil {
			return err
		}
		defer scanned.Close()
		for scanned.Next() {
			var col column
			if err := scanned.Scan(&col.table, &col.name, &col.udt); err != nil {
				return err
			}
			cols = append(cols, col)
		}
		return scanned.Err()
	})
	ident := regexp.MustCompile(`^[a-z_]+$`)
	for _, col := range cols {
		if !ident.MatchString(col.table) || !ident.MatchString(col.name) {
			h.t.Fatalf("неожиданное имя столбца %s.%s", col.table, col.name)
		}
		fn(col.table, col.name, col.udt)
	}
}

func (h *harness) withConn(fn func(context.Context, *pgx.Conn) error) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, h.dsn)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			h.t.Errorf("close db: %v", closeErr)
		}
	}()
	if err := fn(ctx, conn); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, sql, args...)
		return err
	})
}

func (h *harness) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, sql, args...).Scan(&n)
	})
	return n
}

func (h *harness) queryText(sql string, args ...any) string {
	h.t.Helper()
	var out string
	h.withConn(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.QueryRow(ctx, sql, args...).Scan(&out)
	})
	return out
}

func decodeToken(token string) ([]byte, error) {
	if raw, err := hex.DecodeString(token); err == nil && len(raw) == 32 && strings.EqualFold(hex.EncodeToString(raw), token) {
		return raw, nil
	}
	if raw, err := base64.RawURLEncoding.DecodeString(token); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(token); err == nil && len(raw) == 32 {
		return raw, nil
	}
	return nil, errors.New("токен не 32 байта")
}

func tokenForms(token string, raw []byte) []string {
	sum := hex.EncodeToString(raw)
	return []string{
		token,
		sum,
		strings.ToUpper(sum),
		base64.RawURLEncoding.EncodeToString(raw),
		base64.StdEncoding.EncodeToString(raw),
		base64.URLEncoding.EncodeToString(raw),
	}
}

func tokenIsHash(hash []byte, token string, raw []byte) bool {
	if len(hash) != sha256.Size || len(raw) != 32 {
		return false
	}
	sumRaw := sha256.Sum256(raw)
	sumText := sha256.Sum256([]byte(token))
	return bytes.Equal(hash, sumRaw[:]) || bytes.Equal(hash, sumText[:])
}

func scrub(text, token string) string {
	raw, err := decodeToken(token)
	if err != nil {
		return text
	}
	for _, secret := range tokenForms(token, raw) {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[token]")
		}
	}
	return text
}

func within(t *testing.T, got, start, end time.Time, offset, slack time.Duration, what string) {
	t.Helper()
	low := start.Add(offset - slack)
	high := end.Add(offset + slack)
	if got.Before(low) || got.After(high) {
		t.Fatalf("%s = %s, ожидалось между %s и %s",
			what,
			got.UTC().Format(time.RFC3339),
			low.UTC().Format(time.RFC3339),
			high.UTC().Format(time.RFC3339))
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
