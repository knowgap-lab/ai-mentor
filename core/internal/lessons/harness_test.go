package lessons_test

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
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//	Seed(ctx, conn, Fixture) (World, error)
//
// Config: DSN и Now. Now — часы раскладки и проверка «текущего» урока, не time.Now.
// Fixture.Now совпадает с Config.Now.
//
// World: StudentID, OtherStudentID, OtherLessonID, Subject, LessonIDs
// (по возрастанию place_no, id уроков сохраняются), ActorTelegramID, ActorRole,
// CatalogVersion. ActorRole — tutor. Ученик принадлежит этому репетитору
// (owner_tutor_id). OtherStudent — второй ученик с другим владельцем и одним
// уроком OtherLessonID.
//
// Сеанс — cookie __Host-session после POST /auth/dev. Изменяющие запросы шлют
// Origin http://localhost и X-CSRF-Token, равный cookie.
// Ошибка — JSON контракта Error, поле code.
//
// Расклад (LS-R2). Дни 1–7, 1 — понедельник. start_time — часы UTC.
// Слот входит, если начало + duration_min > now и календарная дата UTC слота
// строго раньше календарной даты экзамена. Дату экзамена слотом не считать.
// Слоты идут по времени. Их занимают уроки planned по возрастанию place_no.
// held и skipped слот не занимают. held не замораживается.
// Заморожены LESSON_FROZEN_COUNT ближайших не held и не skipped, по умолчанию 2.
// Правило в Fixture раскладывает уже Seed. PutSchedule делает то же с часами Config.Now.
//
// Порядок блоков снимка — порядок этапов, внутри этапа порядок blocks.
// Практика при заморозке (LS-R11). Слот практики — type_id и position.
// Кандидаты этого type_id: пул Fixture.Pool и variant_id шаблона.
// Берутся невиданные и не резерв (checkpoint_reserve). Из них — наименьший id.
// Урок, получивший вариант, делает его виденным для следующих уроков.
// Разбор идёт по возрастанию place_no. Попытки в core.attempts не пишутся.
//
// Вместимость (LS-R15) = duration_min минус минуты этапов kind homework_review.
// Минуты этапа — minutes[1] − minutes[0]. Сверх вместимости с конца уходят
// целыми этапами на первый урок, который не frozen после того, как отмеченный
// урок стал held и заморозка пересчитана.
//
// Перенос стоит первым этапом LessonDetail.stages и дублируется в carried_in
// (blocks, from_lesson_id). «ДЗ нет» — нет строки core.homework_draft.
// «Задача в ДЗ» — её variant_id есть в items этой строки (массив строк или
// объектов DraftItem) и этой задачи нет на следующем уроке.
// Рецепт закрепления и PDF эти тесты не проверяют.
//
// core.journal_event: kind — имя события, payload — объект полезной нагрузки
// без конверта. Снимок после ReviseBlocks не меняется: в шаблоне уже новые
// блоки, в снимке и в lesson_frozen — прежние.
//
// Чужой ученик проверяется раньше состояния урока: 403 forbidden.
// Коды ошибок только mark_not_in_lesson, forbidden, already_marked,
// lesson_not_held, slot_taken. Предупреждение — type plan_does_not_fit,
// число недостающих уроков в text или в числовом params.

const (
	appOrigin     = "http://localhost"
	sessionCookie = "__Host-session"
	subjectMath   = "math"
)

func anchorNow() time.Time {
	return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
}

func anchorExam() time.Time {
	return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
}

func farExam() time.Time {
	return time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
}

func duringFirst() time.Time {
	return time.Date(2026, 11, 2, 18, 30, 0, 0, time.UTC)
}

func noonBefore() time.Time {
	return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
}

func anchorSchedule() *lessons.Schedule {
	return &lessons.Schedule{Weekdays: []int{1, 4}, StartTime: "18:00", DurationMin: 60}
}

func basicLesson(place int) lessons.LessonSpec {
	return lessons.LessonSpec{
		ID:         fmt.Sprintf("demo-l%02d", place),
		PlaceNo:    place,
		Title:      fmt.Sprintf("Урок %d", place),
		Kind:       "new",
		TemplateID: fmt.Sprintf("demo:L%02d", place),
		State:      "planned",
		Stages: []lessons.StageSpec{{
			ID:      fmt.Sprintf("demo:L%02d.G1", place),
			Title:   "Теория",
			Kind:    "theory",
			Minutes: [2]int{0, 60},
			Blocks:  []string{fmt.Sprintf("demo:B%d", place)},
		}},
	}
}

func heldLesson(place int) lessons.LessonSpec {
	lesson := basicLesson(place)
	lesson.State = "held"
	return lesson
}

func quietLesson(place int) lessons.LessonSpec {
	lesson := basicLesson(place)
	lesson.Stages[0].Minutes = [2]int{0, 10}
	return lesson
}

func teachLesson(place int) lessons.LessonSpec {
	block := fmt.Sprintf("demo:B%d", place)
	return lessons.LessonSpec{
		ID:         fmt.Sprintf("demo-l%02d", place),
		PlaceNo:    place,
		Title:      fmt.Sprintf("Урок %d", place),
		Kind:       "new",
		TemplateID: fmt.Sprintf("demo:L%02d", place),
		State:      "planned",
		Stages: []lessons.StageSpec{
			{
				ID:      fmt.Sprintf("demo:L%02d.T", place),
				Title:   "Теория",
				Kind:    "theory",
				Minutes: [2]int{0, 30},
				Blocks:  []string{block},
			},
			{
				ID:      fmt.Sprintf("demo:L%02d.P", place),
				Title:   "Практика",
				Kind:    "practice",
				Minutes: [2]int{30, 60},
				Practice: []lessons.PracticeSpec{{
					VariantID:   fmt.Sprintf("demo:V%d", place),
					TypeID:      fmt.Sprintf("demo:T%d", place),
					Role:        "solve_alone",
					Position:    1,
					TheoryBlock: block,
				}},
			},
		},
	}
}

func plannedLessons(n int) []lessons.LessonSpec {
	out := make([]lessons.LessonSpec, n)
	for place := 1; place <= n; place++ {
		out[place-1] = basicLesson(place)
	}
	return out
}

func marks(theory, stage string, position int) map[string]any {
	body := map[string]any{
		"last_theory_block": nil,
		"last_practice":     nil,
	}
	if theory != "" {
		body["last_theory_block"] = theory
	}
	if position > 0 {
		body["last_practice"] = map[string]any{
			"stage_id": stage,
			"position": position,
		}
	}
	return body
}

type harness struct {
	t     *testing.T
	now   time.Time
	dsn   string
	conn  *pgx.Conn
	svc   *lessons.Service
	world lessons.World
	token string
}

func newHarness(t *testing.T, now time.Time) *harness {
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
	svc, err := lessons.Open(ctx, lessons.Config{DSN: dsn, Now: now})
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

func (h *harness) seed(fx lessons.Fixture) lessons.World {
	h.t.Helper()
	if !fx.Now.Equal(h.now) {
		h.t.Fatalf("now фикстуры %s, часы сервиса %s", fx.Now.Format(time.RFC3339), h.now.Format(time.RFC3339))
	}
	world, err := lessons.Seed(context.Background(), h.conn, fx)
	if err != nil {
		h.t.Fatalf("Seed: %v", err)
	}
	sorted := slices.Clone(fx.Lessons)
	slices.SortFunc(sorted, func(a, b lessons.LessonSpec) int { return a.PlaceNo - b.PlaceNo })
	wantIDs := make([]string, len(sorted))
	for i, lesson := range sorted {
		wantIDs[i] = lesson.ID
	}
	if !slices.Equal(world.LessonIDs, wantIDs) {
		h.t.Fatalf("уроки %v, нужны %v", world.LessonIDs, wantIDs)
	}
	if fx.Subject != "" && world.Subject != fx.Subject {
		h.t.Fatalf("предмет %s, нужен %s", world.Subject, fx.Subject)
	}
	if fx.CatalogVersion != 0 && world.CatalogVersion != fx.CatalogVersion {
		h.t.Fatalf("версия каталога %d, нужна %d", world.CatalogVersion, fx.CatalogVersion)
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
	h.svc.Handler().ServeHTTP(rec, req)
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

func (h *harness) plan() planView {
	h.t.Helper()
	path := "/students/" + url.PathEscape(h.world.StudentID) + "/plans/" + url.PathEscape(h.world.Subject)
	rec := h.call(http.MethodGet, path, nil)
	h.status(rec, http.StatusOK)
	return decode[planView](h.t, rec)
}

func (h *harness) lesson(id string) lessonDetail {
	h.t.Helper()
	rec := h.call(http.MethodGet, "/lessons/"+url.PathEscape(id), nil)
	h.status(rec, http.StatusOK)
	return decode[lessonDetail](h.t, rec)
}

func (h *harness) putSchedule(rule *lessons.Schedule) *httptest.ResponseRecorder {
	h.t.Helper()
	path := "/students/" + url.PathEscape(h.world.StudentID) + "/schedules/" + url.PathEscape(h.world.Subject)
	return h.call(http.MethodPut, path, scheduleRule{
		Weekdays:    rule.Weekdays,
		StartTime:   rule.StartTime,
		DurationMin: rule.DurationMin,
	})
}

func (h *harness) move(id, startsAt string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPost, "/lessons/"+url.PathEscape(id)+"/move", map[string]string{
		"starts_at": startsAt,
	})
}

func (h *harness) cancel(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPost, "/lessons/"+url.PathEscape(id)+"/cancel", nil)
}

func (h *harness) mark(id string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPost, "/lessons/"+url.PathEscape(id)+"/marks", body)
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

func (h *harness) draftIDs(lessonID string) (bool, []string) {
	h.t.Helper()
	var raw []byte
	err := h.conn.QueryRow(context.Background(),
		`SELECT items FROM core.homework_draft WHERE lesson_id = $1`, lessonID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		h.t.Fatalf("черновик: %v", err)
	}
	return true, parseItemIDs(h.t, raw)
}

func (h *harness) expectNoDraft(lessonID string) {
	h.t.Helper()
	if ok, ids := h.draftIDs(lessonID); ok {
		h.t.Fatalf("у урока %s есть строка core.homework_draft: %v", lessonID, ids)
	}
}

func (h *harness) draftCount() int {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(), `SELECT count(*) FROM core.homework_draft`).Scan(&n); err != nil {
		h.t.Fatalf("черновики: %v", err)
	}
	return n
}

func (h *harness) attemptCount() int {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(), `SELECT count(*) FROM core.attempts`).Scan(&n); err != nil {
		h.t.Fatalf("попытки: %v", err)
	}
	return n
}

type scheduleRule struct {
	Weekdays    []int  `json:"weekdays"`
	StartTime   string `json:"start_time"`
	DurationMin int    `json:"duration_min"`
}

type scheduleEvent struct {
	Subject     string `json:"subject"`
	Weekdays    []int  `json:"weekdays"`
	StartTime   string `json:"start_time"`
	DurationMin int    `json:"duration_min"`
}

type warning struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params"`
}

type practiceRef struct {
	StageID  string `json:"stage_id"`
	Position int    `json:"position"`
}

type marksBody struct {
	LastTheoryBlock *string      `json:"last_theory_block"`
	LastPractice    *practiceRef `json:"last_practice"`
}

type lessonView struct {
	ID          string     `json:"id"`
	TemplateID  string     `json:"template_id"`
	PlaceNo     int        `json:"place_no"`
	Title       string     `json:"title"`
	Kind        string     `json:"kind"`
	Topics      []string   `json:"topics"`
	State       string     `json:"state"`
	StartsAt    string     `json:"starts_at"`
	PlannedWeek string     `json:"planned_week"`
	Marks       *marksBody `json:"marks"`
	Moved       bool       `json:"moved"`
	Added       bool       `json:"added"`
}

type planView struct {
	Lessons  []lessonView `json:"lessons"`
	Warnings []warning    `json:"warnings"`
}

type practiceItem struct {
	VariantID string `json:"variant_id"`
	TypeID    string `json:"type_id"`
	Role      string `json:"role"`
	Position  int    `json:"position"`
}

type stageView struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Kind     string         `json:"kind"`
	Minutes  []int          `json:"minutes"`
	Blocks   []string       `json:"blocks"`
	Practice []practiceItem `json:"practice"`
}

type carriedIn struct {
	Blocks       []string      `json:"blocks"`
	Practice     []practiceRef `json:"practice"`
	FromLessonID string        `json:"from_lesson_id"`
}

type lessonDetail struct {
	Lesson    lessonView  `json:"lesson"`
	CarriedIn *carriedIn  `json:"carried_in"`
	Stages    []stageView `json:"stages"`
	Source    string      `json:"source"`
}

type movedPayload struct {
	LessonID string `json:"lesson_id"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type cancelledPayload struct {
	LessonID string `json:"lesson_id"`
	Date     string `json:"date"`
}

type markedPayload struct {
	LessonID        string       `json:"lesson_id"`
	LastTheoryBlock *string      `json:"last_theory_block"`
	LastPractice    *practiceRef `json:"last_practice"`
	Coverage        string       `json:"coverage"`
}

type frozenPayload struct {
	LessonID       string   `json:"lesson_id"`
	TemplateID     string   `json:"template_id"`
	CatalogVersion int      `json:"catalog_version"`
	Blocks         []string `json:"blocks"`
	Stages         []struct {
		StageID string `json:"stage_id"`
		Minutes []int  `json:"minutes"`
	} `json:"stages"`
	Practice []struct {
		StageID   string `json:"stage_id"`
		VariantID string `json:"variant_id"`
		TypeID    string `json:"type_id"`
		Position  int    `json:"position"`
	} `json:"practice"`
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

func assertOrder(t *testing.T, plan planView, n int) {
	t.Helper()
	if len(plan.Lessons) != n {
		t.Fatalf("уроков %d, нужно %d", len(plan.Lessons), n)
	}
	for i := 1; i <= n; i++ {
		if plan.Lessons[i-1].PlaceNo != i {
			t.Fatalf("порядок мест %v", places(plan))
		}
	}
}

func places(plan planView) []int {
	out := make([]int, len(plan.Lessons))
	for i, lesson := range plan.Lessons {
		out[i] = lesson.PlaceNo
	}
	return out
}

func byPlace(t *testing.T, plan planView, place int) lessonView {
	t.Helper()
	for _, lesson := range plan.Lessons {
		if lesson.PlaceNo == place {
			return lesson
		}
	}
	t.Fatalf("нет урока %d среди %v", place, places(plan))
	return lessonView{}
}

func blocksOf(stages []stageView) []string {
	var out []string
	for _, stage := range stages {
		out = append(out, stage.Blocks...)
	}
	return out
}

func practiceVariants(items []practiceItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.VariantID)
	}
	return out
}

func hasVariant(stages []stageView, id string) bool {
	for _, stage := range stages {
		if slices.Contains(practiceVariants(stage.Practice), id) {
			return true
		}
	}
	return false
}

func assertCarry(t *testing.T, detail lessonDetail, from string, blocks, variants []string) {
	t.Helper()
	if detail.CarriedIn == nil {
		t.Fatal("нет carried_in")
	}
	if detail.CarriedIn.FromLessonID != from {
		t.Fatalf("from_lesson_id %s, нужен %s", detail.CarriedIn.FromLessonID, from)
	}
	if !slices.Equal(detail.CarriedIn.Blocks, blocks) {
		t.Fatalf("блоки переноса %v, нужны %v", detail.CarriedIn.Blocks, blocks)
	}
	if len(detail.Stages) == 0 {
		t.Fatal("перенос не поставлен этапом в начало")
	}
	if !slices.Equal(detail.Stages[0].Blocks, blocks) {
		t.Fatalf("первый этап %v, нужны блоки %v", detail.Stages[0].Blocks, blocks)
	}
	if got := practiceVariants(detail.Stages[0].Practice); !slices.Equal(got, variants) {
		t.Fatalf("задачи переноса %v, нужны %v", got, variants)
	}
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

func parseItemIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("items: %v, тело %s", err, raw)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			ids = append(ids, value)
		case map[string]any:
			id, _ := value["variant_id"].(string)
			if id != "" {
				ids = append(ids, id)
			}
		default:
			t.Fatalf("элемент черновика %T", item)
		}
	}
	return ids
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
