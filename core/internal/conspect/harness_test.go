// Публичный интерфейс фиксируют вызовы ниже. Реализации в пакете нет.
//
//	Open(ctx, Config) (*Service, error)
//	(*Service).Handler() http.Handler
//	(*Service).Close() error
//	Seed(ctx, conn, Fixture) (World, error)
//
// Config содержит только DSN.
//
// Fixture: Subject, CatalogVersion, SnapshotVersion, Blocks, Lessons, Carry,
// OtherStudent, Peer.
// BlockRev: ID, Kind, Title, Markdown, Version, Widget.
// WidgetSpec: Widget, Params, Schema.
// LessonSpec: ID, PlaceNo, Title, State, Stages, NoBlocks, LessonFile, Mark, Practice.
// StageSpec: ID, Blocks.
// CarrySpec: ToLessonID, FromLessonID, Blocks.
//
// World: TutorTelegramID, StudentTelegramID, ActorRole, OtherLessonID, PeerLessonID.
// ActorRole — tutor. Оба telegram id есть в core.user_pii, роли в core.app_user
// совпадают с ними, owner_tutor_id ученика — этот репетитор. Сеанс — cookie
// __Host-session после POST /auth/dev. Хеш сеанса — SHA-256 от 32 сырых байт
// токена, как в auth.
//
// Seed сохраняет id уроков фикстуры. Для frozen и held пишет снимок и одно
// событие core.journal_event kind=lesson_frozen: lesson_id и catalog_version
// (число SnapshotVersion). planned и NoBlocks событие не получают, snapshot
// остаётся NULL. Текущая версия плана — core.study_plan.catalog_version,
// это CatalogVersion, она может быть новее снимка.
// Ревизия каталога — строка catalog.revision: entity_id, catalog_version_from
// = BlockRev.Version, непустой content_hash, data. В data есть id, kind, title,
// body (это Markdown фикстуры) и, если виджет задан, widget {widget, params}
// и widget_schema (JSON Schema из WidgetSpec.Schema). widget_schema в ответ
// не попадает. Версия каталога N — последняя ревизия id с
// catalog_version_from ≤ N.
// Снимок в core.lesson.snapshot — этапы по порядку: id, blocks и, если у урока
// задан Practice, practice первого этапа с этими variant_id. Сборка читает
// снимок и событие lesson_frozen, не template и не текущую версию плана.
// Перенос — строка core.lesson_carry. Отметка — core.lesson_mark.last_theory_block,
// если Mark не пуст.
//
// GET /lessons/{lesson}/conspect отдаёт Conspect контракта.
// Порядок своих блоков — порядок этапов, внутри этапа порядок blocks.
// В ответ входят kind theory, rule, summary, worked_example, typical_error, widget.
// flashcard не входит. Блок, в body которого есть подстрока `<script`, не входит,
// и этой подстроки нет в теле ответа. Виджет без схемы входит. Виджет со схемой
// входит, только если params ей подходят: type object, required, properties.type
// integer (JSON-число без дробной части), additionalProperties false.
// markdown ответа равен body ревизии на версии снимка. Текст платформа не пишет.
// id вариантов из Practice нет в теле ответа.
// Перенесённые блоки — массив carried_over, в порядке Carry.Blocks, тексты
// на версии снимка открытого урока. Свои блоки в blocks, без id переноса.
//
// Ученик на held видит свои блоки от начала снимка до Mark включительно
// (после отбрасывания flashcard и `<script`) и весь перенос. Репетитор видит
// снимок целиком и до проведения, и после, отметка срез не режет.
//
// Порядок отказов: чужой или несуществующий урок — 403 forbidden, раньше
// состояния урока. NoBlocks — запасной путь, не 409: репетитору 200 и ровно
// один блок, markdown которого равен LessonFile, id и kind непусты,
// catalog_version — текущая версия плана; ученику 200, blocks и carried_over
// пустые, текста файла в теле нет. Нет снимка и это не NoBlocks — 409
// lesson_not_frozen. Ученик, а урок не held — 409 lesson_not_held.
// Нет ревизии блока снимка на версии снимка (блок не отброшен фильтром) — 500
// block_missing. Иначе 200.
//
// GET /lessons/{lesson}/conspect.pdf у своего репетитора по уроку со снимком —
// 503 и JSON Error с кодом pdf_unavailable. Байты PDF эти тесты не сравнивают.
//
// Трактовка: файл урока репетитору кладётся в markdown единственного блока.
// Отдельного поля в Conspect нет.
//
// Вопросы, которые тесты сознательно не закрывают.
//
// ВОПРОС: заголовок «Перенесено с урока N». N — place_no урока-источника.
// В Conspect нет поля заголовка, additionalProperties: false. Тест проверяет
// раздел через carried_over и не требует строку заголовка в JSON.
// ВОПРОС: CN-R9, очистка SVG и CSP. В Block нет поля рисунка. Не проверяю.
// ВОПРОС: held без отметки теории. Не проверяю.
// ВОПРОС: операция getConspect в контракте не перечисляет ответ 500.
// Раздел 5 спецификации требует 500 block_missing. Тест следует спецификации.

package conspect_test

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
	"github.com/knowgap-lab/ai-mentor/core/internal/auth"
	"github.com/knowgap-lab/ai-mentor/core/internal/conspect"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

const (
	appOrigin     = "http://localhost"
	sessionCookie = "__Host-session"
	subjectMath   = "math"
)

type harness struct {
	t     *testing.T
	dsn   string
	conn  *pgx.Conn
	svc   *conspect.Service
	world conspect.World
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
	svc, err := conspect.Open(ctx, conspect.Config{DSN: dsn})
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

func (h *harness) seed(fx conspect.Fixture) conspect.World {
	h.t.Helper()
	world, err := conspect.Seed(context.Background(), h.conn, fx)
	if err != nil {
		h.t.Fatalf("Seed: %v", err)
	}
	if world.TutorTelegramID == "" || world.StudentTelegramID == "" || world.ActorRole != "tutor" {
		h.t.Fatalf("актёр telegram=%q role=%q ученик=%q", world.TutorTelegramID, world.ActorRole, world.StudentTelegramID)
	}
	if fx.OtherStudent && world.OtherLessonID == "" {
		h.t.Fatal("Seed не заполнил чужой урок")
	}
	if fx.Peer && (world.PeerLessonID == "" || world.PeerLessonID == fx.Lessons[0].ID) {
		h.t.Fatalf("урок второго ученика %q", world.PeerLessonID)
	}
	h.world = world
	h.token = login(h.t, h.dsn, world.TutorTelegramID, "tutor")
	return world
}

func (h *harness) as(telegramID, role string) {
	h.t.Helper()
	h.token = login(h.t, h.dsn, telegramID, role)
}

func (h *harness) call(method, path string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if h.token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: h.token})
	}
	rec := httptest.NewRecorder()
	h.svc.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *harness) conspect(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodGet, "/lessons/"+url.PathEscape(id)+"/conspect")
}

func (h *harness) pdf(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.call(http.MethodGet, "/lessons/"+url.PathEscape(id)+"/conspect.pdf")
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

func (h *harness) okConspect(id string) (conspectView, *httptest.ResponseRecorder) {
	h.t.Helper()
	rec := h.conspect(id)
	h.status(rec, http.StatusOK)
	return decode[conspectView](h.t, rec), rec
}

type widgetView struct {
	Widget string         `json:"widget"`
	Params map[string]any `json:"params"`
}

type blockView struct {
	ID       string      `json:"id"`
	Kind     string      `json:"kind"`
	Title    string      `json:"title"`
	Markdown string      `json:"markdown"`
	Widget   *widgetView `json:"widget"`
}

type conspectView struct {
	LessonID       string      `json:"lesson_id"`
	CatalogVersion int         `json:"catalog_version"`
	CarriedOver    []blockView `json:"carried_over"`
	Blocks         []blockView `json:"blocks"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	var out T
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("json: %v, тело %s", err, rec.Body.Bytes())
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("после JSON есть ещё данные: %v", err)
	}
	return out
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
