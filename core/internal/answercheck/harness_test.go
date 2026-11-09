package answercheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/answercheck"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Seed(ctx, conn, given) (World, error)
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//
// Config содержит только DSN.
// World: AssignmentID, ItemID, StudentID, ActorTelegramID, ActorRole.
// given — те же поля, что в answer-check.examples.yaml. Числа передаются как int,
// вложенные объекты как map[string]any, списки как []any.
// POST /assignments/{id}/items/{item}/answer, тело — answer и idempotency_key.
//
// Выдача принадлежит ученику-владельцу. Без actor актёр — этот ученик, роль student.
// actor и assignment.owner — id учеников (ac-13: owner student_a, actor student_b).
// Seed заводит актёра в core.app_user и core.user_pii. ActorTelegramID входит через
// POST /auth/dev. Сеанс — cookie __Host-session. Запрос несёт Origin http://localhost
// и X-CSRF-Token, равный значению cookie.
//
// Seed всегда заполняет AssignmentID и ItemID. Если в given нет item, элемент открытый:
// маршрут всё равно ведёт на элемент этой выдачи.
// Ключ ct.choice1 — []any из int, ключ ct.integer — int. Ответ choice — такой же список,
// ответ integer — string.
// item.attempts и item.verdicts — уже записанные попытки: номера с 1 по порядку,
// вердикт из списка, у wrong баллы 0 в core.attempt_verdict. Пока элемент open,
// наблюдение Seed не пишет.
// Ключ, формат, часть и блок решения сохраняются так, чтобы обработчик оценил ответ
// через answerrules.Score. Когда AC-R7 разрешает show, solution_block — строка
// «ct-math:M10.B09» из ac-02 и ac-04. show.key в этих примерах — «3» и «12».
// assignment.deadline_passed true — срок выдачи уже прошёл.
// requests_in_last_minute — сколько запросов answerItem этот ученик уже сделал
// за последнюю минуту. Это не попытки и элемент они не закрывают. Проверяемый
// запрос в это число не входит.
//
// Журнал, который читают тесты:
//
//	attempts — число строк core.attempts этой выдачи и этого элемента;
//	observations, first_attempt_points, total_points — строки core.observation;
//	attempt_numbers — attempt_no по возрастанию;
//	late — payload.late событий core.journal_event с kind attempt_recorded,
//	у которых payload.item_id и payload.assignment_id совпадают с элементом.
//	JSON true читается как текст true. Колонки late в core.attempts нет:
//	её нет в миграции 02001, а в events.schema.json late лежит в попытке-событии.
//
// Тело ошибки — схема Error контракта: code на верхнем уровне, не внутри error.
// Подсказка malformed для целого — «введите целое число».
//
// ВОПРОС: в answer-check.examples.yaml нет id ac-14 (после ac-13 сразу ac-15). AC-R14 требует ключ версии каталога, на которой выдана выдача, но примера с двумя версиями ключа нет. Этот случай не тестирую.

const (
	appOrigin     = "http://localhost"
	sessionCookie = "__Host-session"
	solutionBlock = "ct-math:M10.B09"
)

var answerFields = map[string]struct{}{
	"status": {}, "hint": {}, "item_state": {}, "attempts_left": {},
	"verdict": {}, "points": {}, "max_points": {}, "show": {},
}

var showFields = map[string]struct{}{
	"key": {}, "solution_block": {},
}

var errorFields = map[string]struct{}{
	"code": {}, "message": {}, "details": {},
}

type harness struct {
	t     *testing.T
	dsn   string
	conn  *pgx.Conn
	svc   *answercheck.Service
	world answercheck.World
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
	svc, err := answercheck.Open(ctx, answercheck.Config{DSN: dsn})
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

func (h *harness) seed(given map[string]any) {
	h.t.Helper()
	world, err := answercheck.Seed(context.Background(), h.conn, given)
	if err != nil {
		h.t.Fatalf("Seed: %v", err)
	}
	if world.AssignmentID == "" || world.ItemID == "" {
		h.t.Fatalf("Seed не заполнил выдачу %q или элемент %q", world.AssignmentID, world.ItemID)
	}
	if world.ActorRole != "student" || world.ActorTelegramID == "" || world.StudentID == "" {
		h.t.Fatalf("актёр telegram=%q role=%q student=%q", world.ActorTelegramID, world.ActorRole, world.StudentID)
	}
	h.world = world
	h.token = login(h.t, h.dsn, world.ActorTelegramID, world.ActorRole)
}

func (h *harness) answerPath() string {
	h.t.Helper()
	return "/assignments/" + url.PathEscape(h.world.AssignmentID) + "/items/" + url.PathEscape(h.world.ItemID) + "/answer"
}

func (h *harness) call(body map[string]any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.serve(h.request(body))
}

func (h *harness) request(body map[string]any) *http.Request {
	h.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, h.answerPath(), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: h.token})
	req.Header.Set("Origin", appOrigin)
	req.Header.Set("X-CSRF-Token", h.token)
	return req
}

func (h *harness) serve(req *http.Request) *httptest.ResponseRecorder {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *harness) status(rec *httptest.ResponseRecorder, status int) {
	h.t.Helper()
	if rec.Code != status {
		h.t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
}

func (h *harness) expectFields(rec *httptest.ResponseRecorder, status int, want map[string]any, absent []string) {
	h.t.Helper()
	h.status(rec, status)
	obj := decodeObject(h.t, rec.Body.Bytes())
	for key := range obj {
		if _, ok := answerFields[key]; !ok {
			h.t.Fatalf("лишнее поле %s: %s", key, rec.Body.Bytes())
		}
	}
	if show, ok := obj["show"]; ok {
		fields, ok := show.(map[string]any)
		if !ok {
			h.t.Fatalf("show не объект: %s", rec.Body.Bytes())
		}
		for key := range fields {
			if _, allowed := showFields[key]; !allowed {
				h.t.Fatalf("лишнее поле show.%s: %s", key, rec.Body.Bytes())
			}
		}
	}
	for _, name := range absent {
		if _, ok := obj[name]; ok {
			h.t.Fatalf("поле %s должно отсутствовать: %s", name, rec.Body.Bytes())
		}
	}
	if err := matchMap("body", want, obj); err != nil {
		h.t.Fatalf("%v, тело %s", err, rec.Body.Bytes())
	}
}

func (h *harness) expectCode(rec *httptest.ResponseRecorder, status int, code string) {
	h.t.Helper()
	h.status(rec, status)
	obj := decodeObject(h.t, rec.Body.Bytes())
	for key := range obj {
		if _, ok := errorFields[key]; !ok {
			h.t.Fatalf("лишнее поле %s в ошибке: %s", key, rec.Body.Bytes())
		}
	}
	got, _ := obj["code"].(string)
	if got != code {
		h.t.Fatalf("код %q, нужен %s, тело %s", got, code, rec.Body.Bytes())
	}
}

func (h *harness) expectRetryAfter(rec *httptest.ResponseRecorder) {
	h.t.Helper()
	value := rec.Header().Get("Retry-After")
	if value == "" {
		h.t.Fatalf("нет заголовка Retry-After, заголовки %v", rec.Result().Header)
	}
	if _, err := strconv.Atoi(value); err != nil {
		h.t.Fatalf("Retry-After %q не целое", value)
	}
}

func (h *harness) attemptCount() int {
	h.t.Helper()
	return h.count(`
		SELECT count(*) FROM core.attempts
		WHERE assignment_id = $1 AND item_id = $2
	`, h.world.AssignmentID, h.world.ItemID)
}

func (h *harness) attemptNumbers() []int {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), `
		SELECT attempt_no FROM core.attempts
		WHERE assignment_id = $1 AND item_id = $2
		ORDER BY attempt_no
	`, h.world.AssignmentID, h.world.ItemID)
	if err != nil {
		h.t.Fatalf("attempt_no: %v", err)
	}
	defer rows.Close()
	var nums []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			h.t.Fatal(err)
		}
		nums = append(nums, n)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return nums
}

func (h *harness) observation() (n, first, total int) {
	h.t.Helper()
	n = h.count(`
		SELECT count(*) FROM core.observation
		WHERE assignment_id = $1 AND item_id = $2
	`, h.world.AssignmentID, h.world.ItemID)
	if n != 1 {
		return n, 0, 0
	}
	err := h.conn.QueryRow(context.Background(), `
		SELECT first_attempt_points, total_points FROM core.observation
		WHERE assignment_id = $1 AND item_id = $2
	`, h.world.AssignmentID, h.world.ItemID).Scan(&first, &total)
	if err != nil {
		h.t.Fatalf("observation: %v", err)
	}
	return n, first, total
}

func (h *harness) lateFlags() []string {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), `
		SELECT payload->>'late'
		FROM core.journal_event
		WHERE kind = 'attempt_recorded'
		  AND payload->>'assignment_id' = $1
		  AND payload->>'item_id' = $2
		ORDER BY journal_seq
	`, h.world.AssignmentID, h.world.ItemID)
	if err != nil {
		h.t.Fatalf("late: %v", err)
	}
	defer rows.Close()
	var flags []string
	for rows.Next() {
		var flag *string
		if err := rows.Scan(&flag); err != nil {
			h.t.Fatal(err)
		}
		if flag == nil {
			flags = append(flags, "")
			continue
		}
		flags = append(flags, *flag)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return flags
}

func (h *harness) count(query string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		h.t.Fatalf("count: %v", err)
	}
	return n
}

func answerBody(answer any, key string) map[string]any {
	return map[string]any{"answer": answer, "idempotency_key": key}
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

func decodeObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("json: %v тело %s", err, raw)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("лишние данные: %s", raw)
	}
	if obj == nil {
		t.Fatalf("тело не объект: %s", raw)
	}
	return obj
}

func matchMap(path string, expect, actual map[string]any) error {
	for key, ev := range expect {
		av, ok := actual[key]
		if !ok {
			return fmt.Errorf("%s.%s: нет ключа", path, key)
		}
		if err := matchValue(path+"."+key, ev, av); err != nil {
			return err
		}
	}
	return nil
}

func matchValue(path string, expect, actual any) error {
	if ef, ok := asFloat(expect); ok {
		af, ok := asFloat(actual)
		if !ok || af != ef {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return nil
	}
	switch ev := expect.(type) {
	case map[string]any:
		av, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return matchMap(path, ev, av)
	case string:
		got, ok := actual.(string)
		if !ok || got != ev {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return nil
	default:
		if expect != actual {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return nil
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
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

func equalInts(got, want []int) bool {
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
