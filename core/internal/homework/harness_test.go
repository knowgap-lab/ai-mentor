package homework_test

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
	"github.com/knowgap-lab/ai-mentor/core/internal/homework"
)

// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//	Seed(ctx, conn, Fixture) (World, error)
//
// Config содержит только DSN. conn — *pgx.Conn той же базы, что Config.DSN.
//
// World: StudentID, OtherStudentID, OtherLessonID, Subject, LessonIDs,
// ActorTelegramID, ActorRole. LessonIDs — id уроков фикстуры по возрастанию
// PlaceNo, сами id сохраняются. ActorRole — tutor. Ученик принадлежит этому
// репетитору. OtherStudent — второй ученик с другим владельцем и одним уроком
// OtherLessonID; этого урока нет в LessonIDs.
//
// Fixture: Subject, CatalogVersion, OtherStudent, Lessons, Variants, Past.
// Lesson: ID, PlaceNo, State, Cancelled, NoBlocks, Coverage, StartsAt, Blocks,
// Tasks, Recipe, Mark. State — planned, frozen, held или skipped.
// StartsAt — RFC3339 или пустая строка. Coverage — all, none или пусто.
// Task: Position, VariantID, TypeID, Subvariant, TheoryBlock.
// Series: Source (new_topic, review, ct_format), TypeID, Count, Topic.
// Mark: TheorySet, Theory, PracticeSet, PracticePosition.
// Variant: ID, TypeID, Subvariant, Topic, Difficulty, TheoryBlock, Statement,
// Minutes, AnswerFormat, CheckpointReserve, Seen.
// PastItem: VariantID, Topic, MaxPoints, FirstPoints. Past — последнее сданное
// ДЗ: эти варианты ученик уже видел.
//
// Черновик собирается, только если Mark != nil, TheorySet и PracticeSet, и урок
// не Cancelled. TheorySet без PracticeSet — одна отметка: строки нет, GET, PATCH
// и issue отвечают 409 lesson_not_marked. Cancelled — строки нет, те же вызовы
// отвечают 409 lesson_cancelled. Чужой урок — 403 forbidden, и это раньше
// состояния урока.
//
// PracticeSet и PracticePosition 0 — сохранённая отметка практики «нет»:
// решённых задач нет. PracticePosition > 0 — решены задачи с Position не больше
// этой. Theory — последний объяснённый блок; пустая Theory при TheorySet —
// отметка теории «нет».
//
// Блок объяснён, если он есть в Blocks и стоит не позже Theory. Вариант из
// Tasks этого урока для подбора уже виден. Seen — виденность до урока.
//
// Сборка. reinforcement: на каждую решённую задачу, чей TheoryBlock объяснён,
// невиденный нерезервный вариант того же Subvariant с другим id и наименьшим
// id; Statement этого варианта не равен Statement задачи урока. Нет такого —
// задача пропущена, предупреждение variants_needed, и в text или params есть
// TypeID и Subvariant. unsolved: задача после последней решённой, TheoryBlock
// объяснён — в список входит тот же VariantID. Необъяснённая в список не входит
// и предупреждения не даёт. new_topic: серия Recipe, только варианты с
// объяснённым TheoryBlock. review и ct_format: серии Recipe; Topic серии, если
// он не пуст, ограничивает Topic варианта. checkpoint_reserve не берётся.
// Из подходящих невиденных — наименьшие id, не больше Count. Не осталось ни
// одного — серия пропущена, variants_needed, и в предупреждении есть TypeID.
// threshold: по каждой теме Past доля полных первых попыток (FirstPoints равен
// MaxPoints) к числу задач темы. Ниже 80 % — столько задач темы, сколько первых
// попыток не полные: source threshold, невиденные, не резерв, наименьшие id.
// Ниже 60 % — id темы в discuss_topics. Сравнение целое: ниже 80 %, если
// full*5 < total*4; ниже 60 %, если full*5 < total*3. Темы идут по id темы.
// HOMEWORK_RATIO равен 2. Если задач в списке больше, чем решённых на уроке,
// умноженных на 2, в hints есть строка с обоими числами. Список она не меняет.
// Минуты список не режут.
//
// Порядок: unsolved и reinforcement, затем new_topic по возрастанию Difficulty,
// затем threshold, затем review, затем ct_format. Внутри одного source при равной
// сложности — по id варианта. Срок по умолчанию — StartsAt ближайшего урока с
// большим PlaceNo, State не skipped и непустым StartsAt. Нет такого — срок пуст.
// Выдача читает уже собранный черновик. Пустой срок у этой строки — 422
// deadline_required, строки core.assignment нет.
//
// NoBlocks и Coverage all, без Mark: автосборки нет, GET 200, items пуст, урок
// не блокируется. PATCH add невиденного варианта из Variants добавляет его id.
// Seen в add — 409 variant_seen, список прежний. После issue PATCH — 409
// already_issued. Отзыв без ответов — 204, состояние выдачи withdrawn. Есть
// строка core.attempts — 409 has_answers, состояние issued.
//
// Сеанс — cookie __Host-session после POST /auth/dev. Изменяющие запросы шлют
// Origin http://localhost и X-CSRF-Token, равный cookie. Ошибка — JSON Error,
// поле code. Ответ черновика — HomeworkDraft: lesson_id, deadline, items, hints,
// warnings, discuss_topics, issued. Элемент — DraftItem: variant_id, type_id,
// answer_format, minutes, source, statement. source — одно из reinforcement,
// unsolved, new_topic, threshold, review, ct_format. warning.type для нехватки
// вариантов — variants_needed. Выдача — AssignmentTutorView. Журнал:
// core.journal_event.kind, payload — объект без конверта.
//
// Вопросы, которые тесты сознательно не закрывают.
//
// ВОПРОС: обе отметки «нет». Строки черновика нет. Код GET не назван: отметки уже поставлены, lesson_not_marked может не подходить. Не тестирую.
// ВОПРОС: «переходит на следующий урок» у недорешённой с необъяснённой теорией — перенос пакета уроков. Здесь проверяю только, что варианта нет в черновике.
// ВОПРОС: взаимный порядок unsolved и reinforcement. Они одна группа до new_topic. Порядок внутри группы не проверяю.
// ВОПРОС: как именно review «вперемешку». Проверяю обе темы в одном списке и что вариант можно убрать. Чередование тем не проверяю.
// ВОПРОС: source у варианта, добавленного через add. В DraftPatch его нет. Проверяю variant_id, source не фиксирую.
// ВОПРОС: свободных вариантов меньше Count, но больше нуля. В правиле назван только случай, когда не осталось ни одного. Частичное заполнение не тестирую.
// ВОПРОС: следующий не пропущенный урок без даты, а более поздний — с датой. Не тестирую.

const (
	appOrigin     = "http://localhost"
	sessionCookie = "__Host-session"
	subjectMath   = "math"
	blockOne      = "hw-b1"
	formatInt     = "ct.integer"
)

func coverNone() *homework.Mark {
	return &homework.Mark{TheorySet: true, Theory: blockOne, PracticeSet: true}
}

func coverSolved(position int) *homework.Mark {
	return &homework.Mark{
		TheorySet:        true,
		Theory:           blockOne,
		PracticeSet:      true,
		PracticePosition: position,
	}
}

func variant(id, typeID, sub, topic string, difficulty int) homework.Variant {
	return homework.Variant{
		ID:           id,
		TypeID:       typeID,
		Subvariant:   sub,
		Topic:        topic,
		Difficulty:   difficulty,
		TheoryBlock:  blockOne,
		Statement:    "n=" + id,
		Minutes:      5,
		AnswerFormat: formatInt,
	}
}

func task(position int, id, typeID, sub string) homework.Task {
	return homework.Task{
		Position:    position,
		VariantID:   id,
		TypeID:      typeID,
		Subvariant:  sub,
		TheoryBlock: blockOne,
	}
}

type harness struct {
	t     *testing.T
	dsn   string
	conn  *pgx.Conn
	svc   *homework.Service
	world homework.World
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
	svc, err := homework.Open(ctx, homework.Config{DSN: dsn})
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

func (h *harness) seed(fx homework.Fixture) homework.World {
	h.t.Helper()
	world, err := homework.Seed(context.Background(), h.conn, fx)
	if err != nil {
		h.t.Fatalf("Seed: %v", err)
	}
	sorted := slices.Clone(fx.Lessons)
	slices.SortFunc(sorted, func(a, b homework.Lesson) int { return a.PlaceNo - b.PlaceNo })
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

func draftPath(id string) string {
	return "/lessons/" + url.PathEscape(id) + "/homework-draft"
}

func (h *harness) getDraft(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodGet, draftPath(id), nil)
}

func (h *harness) patchDraft(id string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPatch, draftPath(id), body)
}

func (h *harness) issue(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPost, draftPath(id)+"/issue", nil)
}

func (h *harness) withdraw(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodPost, "/assignments/"+url.PathEscape(id)+"/withdraw", map[string]string{
		"reason": "ошибка",
	})
}

func (h *harness) mustDraft(id string) draftView {
	h.t.Helper()
	rec := h.getDraft(id)
	h.status(rec, http.StatusOK)
	body := decode[draftView](h.t, rec)
	if body.LessonID != id {
		h.t.Fatalf("lesson_id %s, нужен %s", body.LessonID, id)
	}
	if body.Issued {
		h.t.Fatal("черновик уже выдан")
	}
	return body
}

func (h *harness) expectNoDraft(lessonID string) {
	h.t.Helper()
	var n int
	err := h.conn.QueryRow(context.Background(),
		`SELECT count(*) FROM core.homework_draft WHERE lesson_id = $1`, lessonID).Scan(&n)
	if err != nil {
		h.t.Fatalf("черновик: %v", err)
	}
	if n != 0 {
		h.t.Fatalf("у урока %s есть строка core.homework_draft", lessonID)
	}
}

func (h *harness) draftRows() int {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(), `SELECT count(*) FROM core.homework_draft`).Scan(&n); err != nil {
		h.t.Fatalf("черновики: %v", err)
	}
	return n
}

func (h *harness) assignmentState(id string) string {
	h.t.Helper()
	var state string
	err := h.conn.QueryRow(context.Background(), `SELECT state FROM core.assignment WHERE id = $1`, id).Scan(&state)
	if err != nil {
		h.t.Fatalf("выдача: %v", err)
	}
	return state
}

func (h *harness) assignmentCount() int {
	h.t.Helper()
	var n int
	if err := h.conn.QueryRow(context.Background(), `SELECT count(*) FROM core.assignment`).Scan(&n); err != nil {
		h.t.Fatalf("выдачи: %v", err)
	}
	return n
}

func (h *harness) firstItem(assignmentID string) string {
	h.t.Helper()
	var id string
	err := h.conn.QueryRow(context.Background(),
		`SELECT item_id FROM core.assignment_item WHERE assignment_id = $1 ORDER BY item_id`, assignmentID).Scan(&id)
	if err != nil {
		h.t.Fatalf("элемент выдачи: %v", err)
	}
	return id
}

func (h *harness) insertAnswer(assignmentID, itemID string) {
	h.t.Helper()
	_, err := h.conn.Exec(context.Background(), `
		INSERT INTO core.attempts (id, assignment_id, item_id, raw_input, source, place, attempt_no)
		VALUES ($1, $2, $3, '1', 'homework', 'home', 1)`,
		"hw-ans-"+assignmentID, assignmentID, itemID)
	if err != nil {
		h.t.Fatalf("ответ: %v", err)
	}
}

func (h *harness) journalKinds() []string {
	h.t.Helper()
	rows, err := h.conn.Query(context.Background(), `SELECT kind FROM core.journal_event ORDER BY journal_seq`)
	if err != nil {
		h.t.Fatalf("журнал: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, kind)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (h *harness) dropDraft(lessonID string) {
	h.t.Helper()
	_, err := h.conn.Exec(context.Background(), `DELETE FROM core.homework_draft WHERE lesson_id = $1`, lessonID)
	if err != nil {
		h.t.Fatalf("удаление черновика: %v", err)
	}
}

type warning struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params"`
}

type draftItem struct {
	VariantID    string `json:"variant_id"`
	TypeID       string `json:"type_id"`
	AnswerFormat string `json:"answer_format"`
	Minutes      int    `json:"minutes"`
	Source       string `json:"source"`
	Statement    string `json:"statement"`
}

type draftView struct {
	LessonID      string      `json:"lesson_id"`
	Deadline      string      `json:"deadline"`
	Items         []draftItem `json:"items"`
	Hints         []string    `json:"hints"`
	Warnings      []warning   `json:"warnings"`
	DiscussTopics []string    `json:"discuss_topics"`
	Issued        bool        `json:"issued"`
}

type issuedView struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Deadline    string `json:"deadline"`
	ItemCount   int    `json:"item_count"`
	CanWithdraw bool   `json:"can_withdraw"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v, тело %s", err, rec.Body.Bytes())
	}
	return out
}

func itemIDs(items []draftItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.VariantID
	}
	return out
}

func sameIDs(items []draftItem, want []string) bool {
	return slices.Equal(itemIDs(items), want)
}

func sourceRank(source string) int {
	switch source {
	case "unsolved", "reinforcement":
		return 0
	case "new_topic":
		return 1
	case "threshold":
		return 2
	case "review":
		return 3
	case "ct_format":
		return 4
	default:
		return 9
	}
}

func assertGrouped(t *testing.T, items []draftItem, difficulty map[string]int) {
	t.Helper()
	prev := -1
	for _, item := range items {
		rank := sourceRank(item.Source)
		if rank < prev {
			t.Fatalf("порядок источников %v", itemIDs(items))
		}
		prev = rank
	}
	bySource := map[string][]draftItem{}
	for _, item := range items {
		bySource[item.Source] = append(bySource[item.Source], item)
	}
	for source, group := range bySource {
		for i := 1; i < len(group); i++ {
			if source == "new_topic" {
				left := difficulty[group[i-1].VariantID]
				right := difficulty[group[i].VariantID]
				if right < left || (right == left && group[i].VariantID < group[i-1].VariantID) {
					t.Fatalf("new_topic %v", itemIDs(group))
				}
				continue
			}
			if group[i].VariantID < group[i-1].VariantID {
				t.Fatalf("%s %v", source, itemIDs(group))
			}
		}
	}
}

func assertSet(t *testing.T, items []draftItem, want []string) {
	t.Helper()
	got := slices.Clone(itemIDs(items))
	slices.Sort(got)
	need := slices.Clone(want)
	slices.Sort(need)
	if !slices.Equal(got, need) {
		t.Fatalf("варианты %v, нужны %v", itemIDs(items), want)
	}
}

func warningMentions(list []warning, parts ...string) bool {
	for _, item := range list {
		if item.Type != "variants_needed" {
			continue
		}
		blob := item.Text
		for key, value := range item.Params {
			blob += " " + key + " " + fmt.Sprint(value)
		}
		ok := true
		for _, part := range parts {
			if !strings.Contains(blob, part) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func containsInt(s string, n int) bool {
	want := fmt.Sprintf("%d", n)
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] != want {
			continue
		}
		if i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
			continue
		}
		if j := i + len(want); j < len(s) && s[j] >= '0' && s[j] <= '9' {
			continue
		}
		return true
	}
	return false
}

func hintsHave(hints []string, a, b int) bool {
	blob := strings.Join(hints, " ")
	return containsInt(blob, a) && containsInt(blob, b)
}

func sourcesOf(items []draftItem, source string) []string {
	var out []string
	for _, item := range items {
		if item.Source == source {
			out = append(out, item.VariantID)
		}
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

func countFull(past []homework.PastItem, topic string) (full, total int) {
	for _, item := range past {
		if item.Topic != topic {
			continue
		}
		total++
		if item.FirstPoints == item.MaxPoints {
			full++
		}
	}
	return full, total
}

func thresholdFixture(full int) homework.Fixture {
	const (
		topic = "M01"
		total = 10
	)
	variants := make([]homework.Variant, 0, total*2)
	past := make([]homework.PastItem, 0, total)
	for i := 1; i <= total; i++ {
		id := fmt.Sprintf("ph-%02d", i)
		item := variant(id, "hw-t-past", "sv-past", topic, 1)
		item.Seen = true
		variants = append(variants, item)
		points := 0
		if i <= full {
			points = 2
		}
		past = append(past, homework.PastItem{
			VariantID: id, Topic: topic, MaxPoints: 2, FirstPoints: points,
		})
	}
	for i := 1; i <= total; i++ {
		item := variant(fmt.Sprintf("th-%02d", i), "hw-t-add", "sv-add", topic, 1)
		variants = append(variants, item)
	}
	lesson := homework.Lesson{
		ID:      "hw-l1",
		PlaceNo: 1,
		State:   "held",
		Blocks:  []string{blockOne},
		Mark:    coverNone(),
	}
	return homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        []homework.Lesson{lesson},
		Variants:       variants,
		Past:           past,
	}
}

func thresholdIDs(n int) []string {
	out := make([]string, n)
	for i := 1; i <= n; i++ {
		out[i-1] = fmt.Sprintf("th-%02d", i)
	}
	return out
}

func numbered(prefix, typeID, topic, block string, n, minutes, difficulty int) []homework.Variant {
	out := make([]homework.Variant, n)
	for i := 1; i <= n; i++ {
		item := variant(fmt.Sprintf("%s-%02d", prefix, i), typeID, "sv-"+prefix, topic, difficulty)
		item.TheoryBlock = block
		item.Minutes = minutes
		out[i-1] = item
	}
	return out
}

func idsNumbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := 1; i <= n; i++ {
		out[i-1] = fmt.Sprintf("%s-%02d", prefix, i)
	}
	return out
}

func oneItemFixture(starts, next string) homework.Fixture {
	lesson := homework.Lesson{
		ID:       "hw-l1",
		PlaceNo:  1,
		State:    "held",
		StartsAt: starts,
		Blocks:   []string{blockOne},
		Recipe:   []homework.Series{{Source: "new_topic", TypeID: "hw-t-one", Count: 1}},
		Mark:     coverNone(),
	}
	lessons := []homework.Lesson{lesson}
	if next != "" {
		lessons = append(lessons, homework.Lesson{
			ID: "hw-l2", PlaceNo: 2, State: "planned", StartsAt: next,
		})
	}
	return homework.Fixture{
		Subject:        subjectMath,
		CatalogVersion: 1,
		Lessons:        lessons,
		Variants:       []homework.Variant{variant("v-one", "hw-t-one", "sv-one", "M01", 1)},
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
