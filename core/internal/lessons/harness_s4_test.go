package lessons_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/knowgap-lab/ai-mentor/core/internal/scenario"
	"gopkg.in/yaml.v3"
)

// Предупреждения плана, которые контракт перечисляет в Warning.type.
// prereq_order в этом списке нет.
var s4WarningTypes = map[string]bool{
	"variants_needed":   true,
	"plan_does_not_fit": true,
	"consent_outdated":  true,
	"not_delivered":     true,
}

var s4Timestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$`)

func s4ByID(t *testing.T, view planView, id string) lessonView {
	t.Helper()
	for _, lesson := range view.Lessons {
		if lesson.ID == id {
			return lesson
		}
	}
	t.Fatalf("нет урока %s", id)
	return lessonView{}
}

func s4SetIssued(t *testing.T, h *harness, lessonID string, issued bool) {
	t.Helper()
	tag, err := h.conn.Exec(context.Background(),
		`UPDATE core.lesson SET issued = $2 WHERE id = $1`, lessonID, issued)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("issued: строк %d", tag.RowsAffected())
	}
}

func s4Issued(t *testing.T, h *harness, lessonID string) bool {
	t.Helper()
	var issued bool
	if err := h.conn.QueryRow(context.Background(),
		`SELECT issued FROM core.lesson WHERE id = $1`, lessonID).Scan(&issued); err != nil {
		t.Fatal(err)
	}
	return issued
}

func s4TemplateKind(t *testing.T, h *harness, lessonID string) (kind, first string) {
	t.Helper()
	if err := h.conn.QueryRow(context.Background(), `
		SELECT jsonb_typeof(template), coalesce(jsonb_typeof(template -> 0), '')
		FROM core.lesson WHERE id = $1
	`, lessonID).Scan(&kind, &first); err != nil {
		t.Fatal(err)
	}
	return kind, first
}

func s4AllTemplatesAreArrays(t *testing.T, h *harness) {
	t.Helper()
	rows, err := h.conn.Query(context.Background(),
		`SELECT id, jsonb_typeof(template) FROM core.lesson`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, kind string
		if err := rows.Scan(&id, &kind); err != nil {
			t.Fatal(err)
		}
		n++
		if kind != "array" {
			t.Fatalf("урок %s: template %s, нужен массив этапов", id, kind)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("нет строк core.lesson")
	}
}

func s4StudentToken(t *testing.T, h *harness) string {
	t.Helper()
	const telegramID = "tg-s4-pupil"
	if _, err := h.conn.Exec(context.Background(),
		`INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)`,
		h.world.StudentID, telegramID); err != nil {
		t.Fatal(err)
	}
	return login(t, h.dsn, telegramID, "student")
}

func s4CallRaw(h *harness, token, method, path string, body any, origin bool) *httptest.ResponseRecorder {
	h.t.Helper()
	saved := h.token
	h.token = token
	defer func() { h.token = saved }()
	if origin {
		return h.call(method, path, body)
	}
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
	if token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		req.Header.Set("X-CSRF-Token", token)
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func s4AddBody(subject, startsAt string) map[string]string {
	return map[string]string{"subject": subject, "starts_at": startsAt}
}

func s4AddPath(studentID string) string {
	return "/students/" + studentID + "/lessons"
}

func s4Object(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("json: %v, тело %s", err, raw)
	}
	return obj
}

func s4Only(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()
	allow := map[string]bool{}
	for _, key := range keys {
		allow[key] = true
	}
	for key := range obj {
		if !allow[key] {
			t.Fatalf("лишнее поле %s", key)
		}
	}
}

func s4Need(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := obj[key]; !ok {
			t.Fatalf("нет поля %s", key)
		}
	}
}

func s4NonEmpty(t *testing.T, obj map[string]any, key string) string {
	t.Helper()
	value, _ := obj[key].(string)
	if value == "" {
		t.Fatalf("пустое %s", key)
	}
	return value
}

func s4CheckAdded(t *testing.T, raw []byte, subject, startsAt string) string {
	t.Helper()
	obj := s4Object(t, raw)
	s4Only(t, obj, "lesson_id", "subject", "starts_at")
	s4Need(t, obj, "lesson_id", "subject", "starts_at")
	if obj["subject"] != subject {
		t.Fatalf("subject %v", obj["subject"])
	}
	got := s4NonEmpty(t, obj, "starts_at")
	if !s4Timestamp.MatchString(got) {
		t.Fatalf("starts_at %q", got)
	}
	sameInstant(t, got, startsAt)
	return s4NonEmpty(t, obj, "lesson_id")
}

func s4CheckSkipped(t *testing.T, raw []byte, lessonID string) {
	t.Helper()
	obj := s4Object(t, raw)
	s4Only(t, obj, "lesson_id")
	s4Need(t, obj, "lesson_id")
	if obj["lesson_id"] != lessonID {
		t.Fatalf("lesson_id %v", obj["lesson_id"])
	}
}

func s4CheckMarked(t *testing.T, raw []byte, lessonID string) {
	t.Helper()
	obj := s4Object(t, raw)
	s4Need(t, obj, "lesson_id")
	if obj["lesson_id"] != lessonID {
		t.Fatalf("lesson_id %v", obj["lesson_id"])
	}
	if _, coverage := obj["coverage"]; coverage {
		s4Only(t, obj, "lesson_id", "coverage")
		value, _ := obj["coverage"].(string)
		if value != "all" && value != "none" {
			t.Fatalf("coverage %v", obj["coverage"])
		}
		return
	}
	s4Only(t, obj, "lesson_id", "last_theory_block", "last_practice")
	s4Need(t, obj, "last_theory_block", "last_practice")
	if practice, ok := obj["last_practice"].(map[string]any); ok {
		s4Only(t, practice, "stage_id", "position")
		s4Need(t, practice, "stage_id", "position")
		s4NonEmpty(t, practice, "stage_id")
		pos, ok := practice["position"].(float64)
		if !ok || pos < 1 || pos != float64(int(pos)) {
			t.Fatalf("position %v", practice["position"])
		}
	} else if obj["last_practice"] != nil {
		t.Fatalf("last_practice %T", obj["last_practice"])
	}
}

func s4CheckMoved(t *testing.T, raw []byte) {
	t.Helper()
	obj := s4Object(t, raw)
	s4Only(t, obj, "lesson_id", "from", "to")
	s4Need(t, obj, "lesson_id", "to")
	s4NonEmpty(t, obj, "lesson_id")
	if !s4Timestamp.MatchString(s4NonEmpty(t, obj, "to")) {
		t.Fatalf("to %v", obj["to"])
	}
	if from, ok := obj["from"]; ok && from != nil {
		text, _ := from.(string)
		if !s4Timestamp.MatchString(text) {
			t.Fatalf("from %v", from)
		}
	}
}

func s4CheckCancelled(t *testing.T, raw []byte) {
	t.Helper()
	obj := s4Object(t, raw)
	s4Only(t, obj, "lesson_id", "date")
	s4Need(t, obj, "lesson_id", "date")
	s4NonEmpty(t, obj, "lesson_id")
	if !s4Timestamp.MatchString(s4NonEmpty(t, obj, "date")) {
		t.Fatalf("date %v", obj["date"])
	}
}

func s4ForbidAnswerKeys(t *testing.T, raw []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("json: %v, тело %s", err, raw)
	}
	s4WalkForbidden(t, value)
}

func s4WalkForbidden(t *testing.T, value any) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "key", "solution_block", "solution", "correct":
				t.Fatalf("в ответе есть %s", key)
			}
			s4WalkForbidden(t, child)
		}
	case []any:
		for _, child := range node {
			s4WalkForbidden(t, child)
		}
	}
}

func s4WarningEnum(t *testing.T, warnings []warning) {
	t.Helper()
	for _, item := range warnings {
		if !s4WarningTypes[item.Type] {
			t.Fatalf("warning %q вне enum контракта", item.Type)
		}
	}
}

func s4NextSlot(t *testing.T, iso string) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 18, 0, 0, 0, time.UTC)
	if !day.After(parsed) {
		day = day.AddDate(0, 0, 1)
	}
	for {
		weekday := int(day.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if weekday == 1 || weekday == 4 {
			return day.UTC().Format(time.RFC3339)
		}
		day = day.AddDate(0, 0, 1)
	}
}

func s4RepoFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{moduleDir(t), ".."}, parts...)...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type s4ExampleRow struct {
	ID    string `yaml:"id"`
	Rule  string `yaml:"rule"`
	Steps []any  `yaml:"steps"`
}

func s4ExampleRows(t *testing.T, parts ...string) []s4ExampleRow {
	t.Helper()
	var rows []s4ExampleRow
	if err := yaml.Unmarshal(s4RepoFile(t, parts...), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func s4RunScenario(t *testing.T, h *harness, scenarios string) {
	t.Helper()
	scenario.Run(t, scenario.Options{
		Spec:      s4RepoFile(t, "docs", "contracts", "core-api.yaml"),
		Scenarios: []byte(scenarios),
		Handler:   h.svc.Handler(),
		Load: func(map[string]any) (scenario.Bindings, error) {
			return scenario.Bindings{
				Path: map[string]string{
					"id":      h.world.StudentID,
					"lesson":  "demo-l03",
					"subject": h.world.Subject,
				},
				Header: map[string]string{
					"Cookie":       sessionCookie + "=" + h.token,
					"Origin":       appOrigin,
					"X-CSRF-Token": h.token,
					"Content-Type": "application/json",
				},
			}, nil
		},
		Journal: func() map[string]any {
			return map[string]any{
				"lesson_added":   h.eventCount("lesson_added"),
				"lesson_skipped": h.eventCount("lesson_skipped"),
				"lesson_marked":  h.eventCount("lesson_marked"),
			}
		},
	})
}

func s4SameStarts(t *testing.T, got, want string) {
	t.Helper()
	if got == "" && want == "" {
		return
	}
	sameInstant(t, got, want)
}
