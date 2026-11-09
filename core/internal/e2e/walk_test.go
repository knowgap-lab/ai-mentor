package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/knowgap-lab/ai-mentor/core/internal/answercheck"
	"github.com/knowgap-lab/ai-mentor/core/internal/attempts"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/compose"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/pressly/goose/v3"
)

// TestWalkSkeleton — один путь через базу и HTTP.
// Ученик отвечает, повтор с тем же ключом не создаёт вторую попытку,
// ключ части B не попадает в ответ, репетитор видит оба сырых ввода
// и только своего ученика.
//
// Вопросы, которые тест сознательно не закрывает.
//
// ВОПРОС: GET /students. Бриф задаёт поля user_id и role. Схема StudentSummary в docs/contracts/core-api.yaml задаёт id и display_name и не содержит role. Тест ждёт JSON-массив объектов с user_id и role и не принимает объект-обёртку. Лишние поля не запрещает.
// ВОПРОС: повтор POST с ключом k-a. Бриф требует, чтобы попытка осталась одна. Верный ответ части A ещё и закрывает элемент, поэтому 409 тоже оставил бы одну строку. Тест считает повтором только статус 200 и то же JSON-тело.
// ВОПРОС: бриф не называет поле сырого ввода в GET /assignments/{id}/attempts. Строки "[3]", "3" и "13" принимаются в любом месте JSON. Число 3 или массив [3] принимаются только в поле raw_input, answer или input, чтобы не спутать их с номером попытки.
// ВОПРОС: бриф не называет kind выдачи. Вставлена выдача homework, place home, catalog_version 1. Иначе ученик не получает verdict (checkpoint, mock, diagnostics) или видит ключ 12 уже на первой ошибке части B. Шаги 5 и 6 вместе с другим kind не сходятся.
// ВОПРОС: compose.Handler по брифу принимает только три http.Handler и не получает DSN. Тест не указывает, откуда GET /students читает owner_tutor_id.

const (
	appOrigin  = "http://localhost"
	cookieName = "__Host-session"

	tutorID      = "tutor-demo"
	studentID    = "student-demo"
	otherTutor   = "tutor-other"
	otherStudent = "student-other"

	tutorTelegram   = "tg-demo-tutor"
	studentTelegram = "tg-demo-student"
	otherTutorTG    = "tg-demo-other"
	otherStudentTG  = "tg-demo-other-student"

	assignmentID = "asg-demo"
	itemChoice   = "item-a"
	itemInteger  = "item-b"
)

func TestWalkSkeleton(t *testing.T) {
	dsn := dbtest.Start(t)
	applyMigrations(t, dsn)
	conn := connect(t, dsn)
	seedWalk(t, conn)

	ctx := context.Background()
	authSvc := openAuth(t, ctx, dsn)
	answerSvc := openAnswers(t, ctx, dsn)
	attemptSvc := openAttempts(t, ctx, dsn)
	handler := compose.Handler(dsn, authSvc.Handler(), answerSvc.Handler(), attemptSvc.Handler())

	student := login(t, handler, studentTelegram, "student", studentID)
	choiceBody := mustJSON(t, map[string]any{
		"answer":          []int{3},
		"idempotency_key": "k-a",
	})
	choicePath := "/assignments/" + assignmentID + "/items/" + itemChoice + "/answer"
	first := call(t, handler, http.MethodPost, choicePath, choiceBody, student.token)
	expectStatus(t, first, http.StatusOK)
	expectVerdict(t, first.Body.Bytes(), "correct")

	second := call(t, handler, http.MethodPost, choicePath, choiceBody, student.token)
	expectStatus(t, second, http.StatusOK)
	if compactJSON(t, first.Body.Bytes()) != compactJSON(t, second.Body.Bytes()) {
		t.Fatalf("повтор k-a вернул другое тело: %s и %s", first.Body.Bytes(), second.Body.Bytes())
	}
	if n := attemptCount(t, conn, assignmentID, itemChoice); n != 1 {
		t.Fatalf("попыток по %s: %d, нужна 1", itemChoice, n)
	}

	integerPath := "/assignments/" + assignmentID + "/items/" + itemInteger + "/answer"
	integerBody := mustJSON(t, map[string]any{
		"answer":          "13",
		"idempotency_key": "k-b",
	})
	integer := call(t, handler, http.MethodPost, integerPath, integerBody, student.token)
	expectStatus(t, integer, http.StatusOK)
	if strings.Contains(integer.Body.String(), "12") {
		t.Fatalf("ответ ученика содержит ключ 12: %s", integer.Body.Bytes())
	}
	if hasKey(decodeValue(t, integer.Body.Bytes()), "show") {
		t.Fatalf("ответ ученика содержит поле show: %s", integer.Body.Bytes())
	}

	tutor := login(t, handler, tutorTelegram, "tutor", tutorID)
	listed := call(t, handler, http.MethodGet, "/assignments/"+assignmentID+"/attempts", nil, tutor.token)
	expectStatus(t, listed, http.StatusOK)
	expectRawInputs(t, listed.Body.Bytes())

	students := call(t, handler, http.MethodGet, "/students", nil, tutor.token)
	expectStatus(t, students, http.StatusOK)
	expectOwnStudent(t, students.Body.Bytes())
}

type walkSession struct {
	token  string
	userID string
}

type snapItem struct {
	ItemID string          `json:"item_id"`
	Part   string          `json:"part"`
	Format string          `json:"format"`
	Key    json.RawMessage `json:"key"`
}

type snapDoc struct {
	Items []snapItem `json:"items"`
}

func openAuth(t *testing.T, ctx context.Context, dsn string) *auth.Service {
	t.Helper()
	svc, err := auth.Open(ctx, auth.Config{
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
		if err := svc.Close(); err != nil {
			t.Errorf("auth close: %v", err)
		}
	})
	return svc
}

func openAnswers(t *testing.T, ctx context.Context, dsn string) *answercheck.Service {
	t.Helper()
	svc, err := answercheck.Open(ctx, answercheck.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("answercheck.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Errorf("answercheck close: %v", err)
		}
	})
	return svc
}

func openAttempts(t *testing.T, ctx context.Context, dsn string) *attempts.Service {
	t.Helper()
	svc, err := attempts.Open(ctx, attempts.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("attempts.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Errorf("attempts close: %v", err)
		}
	})
	return svc
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("conn close: %v", err)
		}
	})
	return conn
}

func seedWalk(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	insertUser(t, ctx, tx, tutorID, "tutor", "", tutorTelegram)
	insertUser(t, ctx, tx, otherTutor, "tutor", "", otherTutorTG)
	insertUser(t, ctx, tx, studentID, "student", tutorID, studentTelegram)
	insertUser(t, ctx, tx, otherStudent, "student", otherTutor, otherStudentTG)

	raw, err := json.Marshal(snapDoc{Items: []snapItem{
		{ItemID: itemChoice, Part: "A", Format: "ct.choice1", Key: json.RawMessage(`[3]`)},
		{ItemID: itemInteger, Part: "B", Format: "ct.integer", Key: json.RawMessage(`12`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, place, snapshot
		)
		VALUES ($1, $2, 'issued', 'homework', 1, 'home', $3::jsonb)
	`, assignmentID, studentID, string(raw)); err != nil {
		t.Fatalf("выдача: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		VALUES ($1, $2, 'open', false, 1), ($1, $3, 'open', false, 2)
	`, assignmentID, itemChoice, itemInteger); err != nil {
		t.Fatalf("элементы: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func insertUser(t *testing.T, ctx context.Context, tx pgx.Tx, id, role, owner, telegramID string) {
	t.Helper()
	var ownerArg any
	if owner != "" {
		ownerArg = owner
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, $2, $3, 'active')
	`, id, role, ownerArg); err != nil {
		t.Fatalf("пользователь %s: %v", id, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, telegramID); err != nil {
		t.Fatalf("telegram %s: %v", id, err)
	}
}

func login(t *testing.T, handler http.Handler, telegramID, role, wantID string) walkSession {
	t.Helper()
	body := mustJSON(t, map[string]string{
		"telegram_id": telegramID,
		"role":        role,
	})
	rec := call(t, handler, http.MethodPost, "/auth/dev", body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("вход %s: %d %s", role, rec.Code, rec.Body.Bytes())
	}
	var me struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("Me: %v тело %s", err, rec.Body.Bytes())
	}
	if me.UserID != wantID || me.Role != role {
		t.Fatalf("Me = %+v, нужны %s %s", me, wantID, role)
	}
	return walkSession{token: sessionToken(t, rec), userID: me.UserID}
}

func call(t *testing.T, handler http.Handler, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req := httptest.NewRequest(method, path, reader).WithContext(ctx)
	req.Header.Set("Origin", appOrigin)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		req.Header.Set("X-CSRF-Token", token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
}

func expectVerdict(t *testing.T, raw []byte, want string) {
	t.Helper()
	obj, ok := decodeValue(t, raw).(map[string]any)
	if !ok {
		t.Fatalf("ответ не объект: %s", raw)
	}
	got, _ := obj["verdict"].(string)
	if got != want {
		t.Fatalf("вердикт %q, нужен %s, тело %s", got, want, raw)
	}
}

func expectRawInputs(t *testing.T, raw []byte) {
	t.Helper()
	var choice, integer bool
	walkInputs(decodeValue(t, raw), "", &choice, &integer)
	if !choice || !integer {
		t.Fatalf("сырые вводы: choice=%v integer=%v, тело %s", choice, integer, raw)
	}
}

func walkInputs(v any, key string, choice, integer *bool) {
	switch n := v.(type) {
	case map[string]any:
		for name, child := range n {
			walkInputs(child, name, choice, integer)
		}
	case []any:
		if inputKey(key) && len(n) == 1 {
			if f, ok := asNumber(n[0]); ok && f == 3 {
				*choice = true
			}
		}
		for _, child := range n {
			walkInputs(child, key, choice, integer)
		}
	case string:
		switch n {
		case "[3]", "3":
			*choice = true
		case "13":
			*integer = true
		}
	case float64:
		if !inputKey(key) {
			return
		}
		switch n {
		case 3:
			*choice = true
		case 13:
			*integer = true
		}
	}
}

func inputKey(key string) bool {
	switch key {
	case "raw_input", "answer", "input":
		return true
	default:
		return false
	}
}

func expectOwnStudent(t *testing.T, raw []byte) {
	t.Helper()
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("GET /students не массив: %v тело %s", err, raw)
	}
	roles := map[string]string{}
	for _, item := range items {
		id, _ := item["user_id"].(string)
		role, _ := item["role"].(string)
		if id == "" || role == "" {
			t.Fatalf("нужны user_id и role: %s", raw)
		}
		roles[id] = role
	}
	if roles[studentID] != "student" || len(roles) != 1 {
		t.Fatalf("список учеников = %v, нужен только %s", roles, studentID)
	}
	if strings.Contains(string(raw), otherStudent) {
		t.Fatalf("список содержит чужой id %s: %s", otherStudent, raw)
	}
}

func attemptCount(t *testing.T, conn *pgx.Conn, assignmentID, itemID string) int {
	t.Helper()
	var n int
	err := conn.QueryRow(context.Background(), `
		SELECT count(*) FROM core.attempts
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&n)
	if err != nil {
		t.Fatalf("счётчик попыток: %v", err)
	}
	return n
}

func sessionToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == cookieName && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatal("нет cookie __Host-session")
	return ""
}

func hasKey(v any, key string) bool {
	switch n := v.(type) {
	case map[string]any:
		for name, child := range n {
			if name == key || hasKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range n {
			if hasKey(child, key) {
				return true
			}
		}
	}
	return false
}

func decodeValue(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("json: %v тело %s", err, raw)
	}
	return v
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	out, err := json.Marshal(decodeValue(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func asNumber(v any) (float64, bool) {
	n, ok := v.(float64)
	return n, ok
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()
	dir, err := migrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("db close: %v", closeErr)
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			t.Errorf("goose close: %v", closeErr)
		}
	}()
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("миграции: %v", err)
	}
}

func migrationsDir() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		if dir, err := findMigrations(start); err == nil {
			return dir, nil
		}
	}
	return "", errors.New("каталог migrations не найден")
}

func findMigrations(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, "migrations", "00001_schemas.sql")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return filepath.Join(dir, "migrations"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("каталог migrations не найден")
		}
		dir = parent
	}
}
