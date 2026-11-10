// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//	Seed(ctx, conn, Fixture) (World, error)
//	ReadPlan0(ctx, conn, studentID, subject string) (Snapshot, error)
//	ReplacePlan0(ctx, conn, studentID, subject string, hashes map[string]string) error
//
// Config: DSN и Now. Now — часы раскладки, не time.Now. Fixture.Now совпадает с Config.Now.
//
// LessonSpec: ID, PlaceNo, Title, Kind, TemplateID, State, Topics, PlannedWeek,
// StartsAt (RFC3339), Moved, Added. Пустой State — planned.
// Schedule: Weekdays, StartTime, DurationMin.
// Fixture: Now, ExamDate, Subject, CatalogVersion, ShiftWeeks, SourceHashes,
// Lessons, Schedule, Prereqs (тема → предпосылки), HomeworkPercent (*int),
// OtherStudent.
// World: StudentID, OtherStudentID, OtherLessonID, Subject, LessonIDs
// (по возрастанию PlaceNo, без чужого и в исходном порядке фикстуры),
// ActorTelegramID, ActorRole (tutor), CatalogVersion.
// Snapshot: SourceHashes, LessonIDs, TemplateIDs, PlannedWeeks — план 0
// в порядке мест. Урока с Added в снимке нет. Переноса (Moved) в снимке нет.
//
// Seed пишет те же строки, что читает пакет lessons:
// core.app_user (tutor и student, owner_tutor_id — этот репетитор, status active),
// core.user_pii.telegram_id, core.study_plan (exam_at, catalog_version >= 1),
// core.lesson (template jsonb — массив, пустой `[]`; snapshot NULL;
// planned_week, starts_at, moved, added, state).
// OtherStudent — второй ученик с другим владельцем и одним уроком OtherLessonID.
// Сеанс — cookie __Host-session после POST /auth/dev. token_hash в core.session —
// SHA-256 от 32 сырых байт hex-cookie, как у auth. Изменяющие запросы шлют
// Origin http://localhost и X-CSRF-Token, равный cookie.
//
// ShiftWeeks сдвигает planned_week на столько понедельников и в снимке, и в
// текущем плане. Номера мест и id уроков те же. 0 — недели как в фикстуре.
//
// Расклад, если задано Schedule. Дни 1–7, 1 — понедельник. start_time — часы UTC.
// Слот входит, если начало + duration_min > Now и календарная дата UTC слота
// строго раньше календарной даты экзамена. Слоты идут по времени.
// Их занимают уроки не held, не skipped и не moved, по возрастанию place_no.
// held и skipped слот не занимают. У Moved со StartsAt время сохраняется.
// Заморожены LESSON_FROZEN_COUNT ближайших не held и не skipped; тесты ставят 2.
// Пропуск этот пакет не выполняет: State skipped только читается.
//
// GET /students/{id}/plans/{subject} — PlanView (lessons по place_no, warnings).
// PUT /students/{id}/plans/{subject}/order — тело {"lesson_ids": [...]} —
// перестановка всех уроков этого ученика и предмета. Успех — PlanView и событие
// core.journal_event kind plan_reordered, payload {"subject","lesson_ids"}
// без конверта. place_no становится позицией в списке, template_id и id не меняются.
// Если frozen или held меняет место — 409, код lesson_frozen, журнал не пишется.
// Чужой ученик, чужой или несуществующий урок — 403, код forbidden.
// Коды ошибок этих тестов только lesson_frozen и forbidden.
//
// Предупреждение prereq_order — тема из Prereqs стоит раньше своей предпосылки;
// в text или params есть обе темы. Запрета нет.
// Предупреждение plan_does_not_fit — при заданном расписании у урока не held
// и не skipped нет слота; число недостающих — в text или params.
// Пропущенные в это число не входят.
//
// GET /students/{id}/plans/{subject}/divergence — массив Divergence
// (lesson_id, change, detail), справка, не ошибка. change: moved, added,
// cancelled, skipped, reordered, week_shift.
// Moved относительно снимка даёт moved. Added даёт added.
//
// HomeworkPercent: одна сданная ДЗ ученика, kind homework, state submitted,
// один item max_points 100, вердикт points = *HomeworkPercent.
// Порядок уроков от этого не меняется, plan_reordered не пишется.
// «ДЗ нет» — нет строки core.homework_draft этого урока.
//
// ReplacePlan0 пытается сменить хеши снимка версии 0 и возвращает ошибку.
// ReadPlan0 после этого совпадает с тем, что записал Seed.

package plan_test

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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/lessons"
	"github.com/knowgap-lab/ai-mentor/core/internal/plan"
)

const (
	appOrigin     = "http://localhost"
	sessionCookie = "__Host-session"
	subjectMath   = "math"
)

func anchorNow() time.Time {
	return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
}

func farExam() time.Time {
	return time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
}

func anchorSchedule() *plan.Schedule {
	return &plan.Schedule{Weekdays: []int{1, 4}, StartTime: "18:00", DurationMin: 60}
}

func numbered(n int) []plan.LessonSpec {
	out := make([]plan.LessonSpec, n)
	for place := 1; place <= n; place++ {
		out[place-1] = plan.LessonSpec{
			ID:          fmt.Sprintf("demo-l%02d", place),
			PlaceNo:     place,
			Title:       fmt.Sprintf("Урок %d", place),
			Kind:        "new",
			TemplateID:  fmt.Sprintf("demo:L%02d", place),
			PlannedWeek: "2026-11-02",
		}
	}
	return out
}

func mathFixture(now, exam time.Time, lessons []plan.LessonSpec) plan.Fixture {
	return plan.Fixture{
		Now:            now,
		ExamDate:       exam,
		Subject:        subjectMath,
		CatalogVersion: 1,
		SourceHashes:   map[string]string{"plan.md": "hash-plan"},
		Lessons:        lessons,
	}
}

type harness struct {
	t     *testing.T
	now   time.Time
	dsn   string
	conn  *pgx.Conn
	svc   *plan.Service
	world plan.World
	token string
}

func newHarness(t *testing.T, now time.Time) *harness {
	t.Helper()
	t.Setenv("LESSON_FROZEN_COUNT", "2")
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
	svc, err := plan.Open(ctx, plan.Config{DSN: dsn, Now: now})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := svc.Close(); closeErr != nil {
			t.Errorf("close: %v", closeErr)
		}
	})
	return &harness{t: t, now: now, dsn: dsn, conn: conn, svc: svc}
}

func (h *harness) seed(fx plan.Fixture) plan.World {
	h.t.Helper()
	if !fx.Now.Equal(h.now) {
		h.t.Fatalf("now фикстуры %s, часы сервиса %s", fx.Now.Format(time.RFC3339), h.now.Format(time.RFC3339))
	}
	world, err := plan.Seed(context.Background(), h.conn, fx)
	if err != nil {
		h.t.Fatalf("Seed: %v", err)
	}
	sorted := slices.Clone(fx.Lessons)
	slices.SortFunc(sorted, func(a, b plan.LessonSpec) int { return a.PlaceNo - b.PlaceNo })
	wantIDs := make([]string, len(sorted))
	for i, lesson := range sorted {
		wantIDs[i] = lesson.ID
	}
	if !slices.Equal(world.LessonIDs, wantIDs) {
		h.t.Fatalf("уроки %v, нужны %v", world.LessonIDs, wantIDs)
	}
	if world.Subject != fx.Subject || world.CatalogVersion != fx.CatalogVersion {
		h.t.Fatalf("предмет %s/%d, нужен %s/%d", world.Subject, world.CatalogVersion, fx.Subject, fx.CatalogVersion)
	}
	if world.ActorRole != "tutor" || world.ActorTelegramID == "" || world.StudentID == "" {
		h.t.Fatalf("актёр telegram=%q role=%q ученик=%q", world.ActorTelegramID, world.ActorRole, world.StudentID)
	}
	if fx.OtherStudent && (world.OtherStudentID == "" || world.OtherLessonID == "") {
		h.t.Fatal("Seed не заполнил чужого ученика")
	}
	h.world = world
	h.token = login(h.t, h.dsn, world.ActorTelegramID, world.ActorRole)
	return world
}

func (h *harness) call(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.callOn(h.svc.Handler(), method, path, body)
}

func (h *harness) callOn(handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
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
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
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
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		h.t.Fatalf("ошибка: %v, тело %s", err, rec.Body.Bytes())
	}
	if payload.Code != code {
		h.t.Fatalf("код %s, нужен %s", payload.Code, code)
	}
}

func (h *harness) planPath(studentID string) string {
	return "/students/" + url.PathEscape(studentID) + "/plans/" + url.PathEscape(h.world.Subject)
}

func (h *harness) planOf(studentID string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodGet, h.planPath(studentID), nil)
}

func (h *harness) plan() planView {
	h.t.Helper()
	rec := h.planOf(h.world.StudentID)
	h.status(rec, http.StatusOK)
	return decode[planView](h.t, rec)
}

func (h *harness) reorder(studentID string, ids []string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPut, h.planPath(studentID)+"/order", map[string]any{
		"lesson_ids": ids,
	})
}

func (h *harness) divergence(studentID string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodGet, h.planPath(studentID)+"/divergence", nil)
}

func (h *harness) moveLesson(id, startsAt string) *httptest.ResponseRecorder {
	h.t.Helper()
	svc, err := lessons.Open(context.Background(), lessons.Config{DSN: h.dsn, Now: h.now})
	if err != nil {
		h.t.Fatalf("lessons.Open: %v", err)
	}
	h.t.Cleanup(func() {
		if closeErr := svc.Close(); closeErr != nil {
			h.t.Errorf("lessons close: %v", closeErr)
		}
	})
	return h.callOn(svc.Handler(), http.MethodPost, "/lessons/"+url.PathEscape(id)+"/move", map[string]string{
		"starts_at": startsAt,
	})
}

func (h *harness) snapshot() plan.Snapshot {
	h.t.Helper()
	snap, err := plan.ReadPlan0(context.Background(), h.conn, h.world.StudentID, h.world.Subject)
	if err != nil {
		h.t.Fatalf("ReadPlan0: %v", err)
	}
	return snap
}

func (h *harness) payloads(kind string) []json.RawMessage {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(),
		`SELECT payload FROM core.journal_event WHERE kind = $1 ORDER BY journal_seq`, kind)
	if err != nil {
		h.t.Fatalf("журнал %s: %v", kind, err)
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, json.RawMessage(raw))
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (h *harness) eventCount(kind string) int {
	h.t.Helper()
	return len(h.payloads(kind))
}

func (h *harness) lastRaw(kind string) json.RawMessage {
	h.t.Helper()
	all := h.payloads(kind)
	if len(all) == 0 {
		h.t.Fatalf("нет события %s", kind)
	}
	return all[len(all)-1]
}

func (h *harness) expectNoDraft(lessonID string) {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(),
		`SELECT count(*) FROM core.homework_draft WHERE lesson_id = $1`, lessonID).Scan(&n); err != nil {
		h.t.Fatalf("черновик: %v", err)
	}
	if n != 0 {
		h.t.Fatalf("у урока %s есть core.homework_draft", lessonID)
	}
}

func (h *harness) submittedHomework() (count, points, maxPoints int) {
	h.t.Helper()
	err := h.conn.QueryRow(context.Background(), `
		SELECT count(*), coalesce(max(v.points), 0), coalesce(max(i.max_points), 0)
		FROM core.assignment AS g
		JOIN core.assignment_item AS i ON i.assignment_id = g.id
		JOIN core.attempts AS a ON a.assignment_id = i.assignment_id AND a.item_id = i.item_id
		JOIN core.attempt_verdict AS v ON v.attempt_id = a.id
		WHERE g.student_id = $1 AND g.kind = 'homework' AND g.state = 'submitted'
	`, h.world.StudentID).Scan(&count, &points, &maxPoints)
	if err != nil {
		h.t.Fatalf("сданное ДЗ: %v", err)
	}
	return count, points, maxPoints
}

type warning struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params"`
}

type lessonView struct {
	ID          string   `json:"id"`
	TemplateID  string   `json:"template_id"`
	PlaceNo     int      `json:"place_no"`
	Title       string   `json:"title"`
	Kind        string   `json:"kind"`
	Topics      []string `json:"topics"`
	State       string   `json:"state"`
	StartsAt    string   `json:"starts_at"`
	PlannedWeek string   `json:"planned_week"`
	Moved       bool     `json:"moved"`
	Added       bool     `json:"added"`
}

type planView struct {
	Lessons  []lessonView `json:"lessons"`
	Warnings []warning    `json:"warnings"`
}

type divergence struct {
	LessonID string `json:"lesson_id"`
	Change   string `json:"change"`
	Detail   string `json:"detail"`
}

type reorderedPayload struct {
	Subject   string   `json:"subject"`
	LessonIDs []string `json:"lesson_ids"`
}

type movedPayload struct {
	LessonID string `json:"lesson_id"`
	To       string `json:"to"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v, тело %s", err, rec.Body.Bytes())
	}
	return out
}

func decodePayload[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("payload: %v, тело %s", err, raw)
	}
	return out
}

func sameInstant(t *testing.T, got, want string) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("время %q: %v", got, err)
	}
	expected, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Equal(expected) {
		t.Fatalf("время %s, нужно %s", got, want)
	}
}

func assertOrder(t *testing.T, view planView, n int) {
	t.Helper()
	if len(view.Lessons) != n {
		t.Fatalf("уроков %d, нужно %d", len(view.Lessons), n)
	}
	for i := 1; i <= n; i++ {
		if view.Lessons[i-1].PlaceNo != i {
			t.Fatalf("порядок мест %v", places(view))
		}
	}
}

func places(view planView) []int {
	out := make([]int, len(view.Lessons))
	for i, lesson := range view.Lessons {
		out[i] = lesson.PlaceNo
	}
	return out
}

func byPlace(t *testing.T, view planView, place int) lessonView {
	t.Helper()
	for _, lesson := range view.Lessons {
		if lesson.PlaceNo == place {
			return lesson
		}
	}
	t.Fatalf("нет урока %d среди %v", place, places(view))
	return lessonView{}
}

func byID(t *testing.T, view planView, id string) lessonView {
	t.Helper()
	for _, lesson := range view.Lessons {
		if lesson.ID == id {
			return lesson
		}
	}
	t.Fatalf("нет урока %s", id)
	return lessonView{}
}

func templatesOf(view planView) map[string]string {
	out := make(map[string]string, len(view.Lessons))
	for _, lesson := range view.Lessons {
		out[lesson.ID] = lesson.TemplateID
	}
	return out
}

func sameSnapshot(t *testing.T, got, want plan.Snapshot) {
	t.Helper()
	if !mapsEqual(got.SourceHashes, want.SourceHashes) {
		t.Fatalf("хеши снимка %v, нужны %v", got.SourceHashes, want.SourceHashes)
	}
	if !slices.Equal(got.LessonIDs, want.LessonIDs) || !slices.Equal(got.TemplateIDs, want.TemplateIDs) ||
		!slices.Equal(got.PlannedWeeks, want.PlannedWeeks) {
		t.Fatalf("снимок уроки %v шаблоны %v недели %v", got.LessonIDs, got.TemplateIDs, got.PlannedWeeks)
	}
}

func mapsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func hasNumber(w warning, want int) bool {
	if slices.Contains(intsIn(w.Text), want) {
		return true
	}
	for _, value := range w.Params {
		if n, ok := asInt(value); ok && n == want {
			return true
		}
	}
	return false
}

func intsIn(s string) []int {
	var out []int
	n := 0
	in := false
	flush := func() {
		if !in {
			return
		}
		out = append(out, n)
		n = 0
		in = false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			in = true
			continue
		}
		flush()
	}
	flush()
	return out
}

func asInt(value any) (int, bool) {
	switch n := value.(type) {
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	case int:
		return n, true
	}
	return 0, false
}

func mentions(w warning, topic string) bool {
	if strings.Contains(w.Text, topic) {
		return true
	}
	raw, err := json.Marshal(w.Params)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), topic)
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
