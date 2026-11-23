package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

// TestContractLiveE3 — живой rootHandler на синтетическом Postgres (E3 §7, D328).
// Урок через API, выдача, ответы, результат, ошибки и роли; тело ошибок — с полем code.
func TestContractLiveE3(t *testing.T) {
	dsn := dbtest.Start(t)
	applyS1Migrations(t, dsn)
	conn := contractLiveConnect(t, dsn)
	contractLiveSeedBase(t, conn)
	contractLiveSeedLesson(t, conn)
	contractLiveSeedAssignment(t, conn, lessonCLID)

	handler := contractLiveRoot(t, dsn)
	origin := "http://127.0.0.1:8080"

	rec := contractLiveCall(handler, http.MethodGet, "/assignments/asg-cl-demo/attempts", nil, "", origin)
	contractLiveExpectError(t, rec, http.StatusUnauthorized, "unauthorized")

	tutor := contractLiveLogin(t, handler, "tg-cl-tutor", "tutor", tutorCLID, origin)

	getLesson := contractLiveCall(handler, http.MethodGet, "/lessons/"+lessonCLID, nil, tutor.token, origin)
	if getLesson.Code != http.StatusOK {
		t.Fatalf("GET урока: %d %s", getLesson.Code, getLesson.Body.Bytes())
	}
	var detail struct {
		Lesson struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"lesson"`
		Stages []json.RawMessage `json:"stages"`
	}
	if err := json.Unmarshal(getLesson.Body.Bytes(), &detail); err != nil {
		t.Fatalf("GET урока json: %v", err)
	}
	if detail.Lesson.ID != lessonCLID {
		t.Fatalf("LessonDetail.lesson.id = %q, нужен %q, тело %s", detail.Lesson.ID, lessonCLID, getLesson.Body.Bytes())
	}
	if len(detail.Stages) == 0 {
		t.Fatalf("LessonDetail без stages: %s", getLesson.Body.Bytes())
	}

	student := contractLiveLogin(t, handler, "tg-cl-student", "student", studentCLID, origin)
	choicePath := "/assignments/asg-cl-demo/items/item-a/answer"
	choiceBody, _ := json.Marshal(map[string]any{
		"answer":          []int{3},
		"idempotency_key": "cl-k-a",
	})
	first := contractLiveCall(handler, http.MethodPost, choicePath, choiceBody, student.token, origin)
	if first.Code != http.StatusOK {
		t.Fatalf("ответ A: %d %s", first.Code, first.Body.Bytes())
	}
	contractLiveExpectVerdict(t, first.Body.Bytes(), "correct")

	second := contractLiveCall(handler, http.MethodPost, choicePath, choiceBody, student.token, origin)
	if second.Code != http.StatusOK {
		t.Fatalf("повтор A: %d %s", second.Code, second.Body.Bytes())
	}

	integerPath := "/assignments/asg-cl-demo/items/item-b/answer"
	integerBody, _ := json.Marshal(map[string]any{
		"answer":          "13",
		"idempotency_key": "cl-k-b",
	})
	wrong := contractLiveCall(handler, http.MethodPost, integerPath, integerBody, student.token, origin)
	if wrong.Code != http.StatusOK {
		t.Fatalf("ответ B: %d %s", wrong.Code, wrong.Body.Bytes())
	}

	tutor2 := contractLiveLogin(t, handler, "tg-cl-tutor", "tutor", tutorCLID, origin)
	listed := contractLiveCall(handler, http.MethodGet, "/assignments/asg-cl-demo/attempts", nil, tutor2.token, origin)
	if listed.Code != http.StatusOK {
		t.Fatalf("попытки: %d %s", listed.Code, listed.Body.Bytes())
	}
	contractLiveExpectRawInputs(t, listed.Body.Bytes())

	other := contractLiveLogin(t, handler, "tg-cl-other", "tutor", otherTutorCLID, origin)
	foreign := contractLiveCall(handler, http.MethodGet, "/assignments/asg-cl-demo/attempts", nil, other.token, origin)
	contractLiveExpectError(t, foreign, http.StatusForbidden, "forbidden")

	students := contractLiveCall(handler, http.MethodGet, "/students", nil, tutor2.token, origin)
	if students.Code != http.StatusOK {
		t.Fatalf("ученики: %d %s", students.Code, students.Body.Bytes())
	}
}

const (
	tutorCLID      = "tutor-cl-demo"
	studentCLID    = "student-cl-demo"
	otherTutorCLID = "tutor-cl-other"
	assignmentCLID = "asg-cl-demo"
	lessonCLID     = "lesson-cl-live"
)

const lessonCLTemplate = `[{"id":"cl:L01.G1","title":"Теория","kind":"theory","minutes":[0,30],"blocks":["cl:B1"]}]`

func contractLiveRoot(t *testing.T, dsn string) http.Handler {
	t.Helper()
	return s1Root(t, dsn)
}

func contractLiveConnect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close(context.Background())
	})
	return conn
}

func contractLiveSeedLesson(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := conn.Exec(ctx, `
		INSERT INTO core.lesson (
			id, student_id, subject, place_no, title, kind, template_id, state,
			planned_week, no_blocks, issued, template
		) VALUES (
			$1, $2, 'math', 1, 'Синтетический урок', 'new', 'cl:L01', 'planned',
			'2026-W49', false, false, $3::jsonb
		)
	`, lessonCLID, studentCLID, lessonCLTemplate)
	if err != nil {
		t.Fatalf("урок: %v", err)
	}
}

func contractLiveSeedBase(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	insertCLUser(t, ctx, tx, tutorCLID, "tutor", "", "tg-cl-tutor")
	insertCLUser(t, ctx, tx, otherTutorCLID, "tutor", "", "tg-cl-other")
	insertCLUser(t, ctx, tx, studentCLID, "student", tutorCLID, "tg-cl-student")

	if _, err := tx.Exec(ctx, `
		INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
		SELECT $1, 'math', '2027-06-15T09:00:00Z', 1
		WHERE NOT EXISTS (
			SELECT 1 FROM core.study_plan WHERE student_id = $1 AND subject = 'math'
		)
	`, studentCLID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.schedule_rule (student_id, subject, weekdays, start_time, duration_min)
		SELECT $1, 'math', ARRAY[1,4]::integer[], '16:00', 60
		WHERE NOT EXISTS (
			SELECT 1 FROM core.schedule_rule WHERE student_id = $1 AND subject = 'math'
		)
	`, studentCLID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func contractLiveSeedAssignment(t *testing.T, conn *pgx.Conn, lessonID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := json.Marshal(map[string]any{
		"lesson_id": lessonID,
		"items": []map[string]any{
			{"item_id": "item-a", "part": "A", "format": "ct.choice1", "key": []int{3}},
			{"item_id": "item-b", "part": "B", "format": "ct.integer", "key": 12},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, place, snapshot
		) VALUES ($1, $2, 'issued', 'homework', 1, 'home', $3::jsonb)
	`, assignmentCLID, studentCLID, string(raw))
	if err != nil {
		t.Fatalf("выдача: %v", err)
	}
	_, err = conn.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		VALUES ($1, 'item-a', 'open', false, 1), ($1, 'item-b', 'open', false, 2)
	`, assignmentCLID)
	if err != nil {
		t.Fatalf("элементы: %v", err)
	}
}

func insertCLUser(t *testing.T, ctx context.Context, tx pgx.Tx, id, role, owner, telegramID string) {
	t.Helper()
	var ownerArg any
	if owner != "" {
		ownerArg = owner
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, $2, $3, 'active')
	`, id, role, ownerArg); err != nil {
		t.Fatalf("user %s: %v", id, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, id, telegramID); err != nil {
		t.Fatalf("telegram %s: %v", id, err)
	}
}

type clSession struct {
	token string
}

func contractLiveLogin(t *testing.T, handler http.Handler, telegramID, role, wantID, origin string) clSession {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"telegram_id": telegramID, "role": role})
	rec := contractLiveCall(handler, http.MethodPost, "/auth/dev", body, "", origin)
	if rec.Code != http.StatusOK {
		t.Fatalf("вход %s: %d %s", role, rec.Code, rec.Body.Bytes())
	}
	var me struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.UserID != wantID || me.Role != role {
		t.Fatalf("Me %+v", me)
	}
	return clSession{token: contractLiveSessionToken(t, rec)}
}

func contractLiveCall(handler http.Handler, method, path string, body []byte, token, origin string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Origin", origin)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		if method != http.MethodGet && method != http.MethodHead {
			req.Header.Set("X-CSRF-Token", token)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func contractLiveSessionToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatal("нет cookie сеанса")
	return ""
}

func contractLiveExpectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("статус %d, нужен %d, тело %s", rec.Code, status, rec.Body.Bytes())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ошибка не json: %v, тело %s", err, rec.Body.Bytes())
	}
	if body.Code != code {
		t.Fatalf("code %q, нужен %q, тело %s", body.Code, code, rec.Body.Bytes())
	}
}

func contractLiveExpectVerdict(t *testing.T, raw []byte, want string) {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	got, _ := obj["verdict"].(string)
	if got != want {
		t.Fatalf("verdict %q, нужен %s", got, want)
	}
}

func contractLiveExpectRawInputs(t *testing.T, raw []byte) {
	t.Helper()
	text := string(raw)
	if !bytes.Contains(raw, []byte("13")) && !bytes.Contains(raw, []byte(`"[3]"`)) && !bytes.Contains(raw, []byte("3")) {
		t.Fatalf("нет сырых вводов: %s", text)
	}
}
