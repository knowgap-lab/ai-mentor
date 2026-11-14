package attempts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

const (
	s5EvilOrigin  = "https://evil.example"
	s5OtherAssign = "s5-a2"
	s5IdemKey     = "s5-same"
)

// ВОПРОС: код 409 у getSummary до «Сдать» в таблице ошибок attempts не назван.
// Варианты: не фиксировать имя кода; взять not_submitted; завести отдельный код.
// Тест проверяет только 409 и тело Error.

// ВОПРОС: listAttempts ученику на пробнике до «Сдать». AttemptView требует verdict,
// AC-R7 запрещает вердикт клиенту. Варианты: 403; 200 без verdict и points.
// Тест не выбирает форму и запрещает только утечку вердикта.

// ВОПРОС: core-api.yaml не называет code для 403 этих операций. Forbidden упоминает csrf,
// AU-R24 пишет csrf, таблица attempts пишет forbidden. Варианты: csrf; forbidden;
// не фиксировать имя. Тест проверяет только статус 403 и одинаковое тело Error.

func TestAU_R24_S5_MutatingRejectsForeignOrigin(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 0},
	})
	rec := s5Call(h, http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, h.token, s5Ptr(s5EvilOrigin))
	s5Expect403(t, h, rec)
	if h.assignmentState(h.assignment()) != "issued" {
		t.Fatalf("состояние %s", h.assignmentState(h.assignment()))
	}
	if h.count(`SELECT count(*) FROM core.journal_event WHERE kind = 'assignment_submitted'`) != 0 {
		t.Fatal("чужой Origin записал сдачу")
	}
}

func TestAU_R24_S5_MutatingRejectsMissingOrigin(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 0},
	})
	rec := s5Call(h, http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, h.token, nil)
	s5Expect403(t, h, rec)
	if h.count(`SELECT count(*) FROM core.journal_event WHERE kind = 'assignment_submitted'`) != 0 {
		t.Fatal("запрос без Origin записал сдачу")
	}
}

func TestAU_R24_S5_GetIgnoresOrigin(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 1},
	})
	rec := s5Call(h, http.MethodGet, assignmentPath(h.assignment(), ""), nil, h.token, s5Ptr(s5EvilOrigin))
	h.status(rec, http.StatusOK)
}

func TestAU_R4_S5_AT_R23_ForeignMatchesMissing(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"actor":      "student_b",
		"assignment": map[string]any{"owner": "student_a", "state": "issued"},
	})
	foreign := h.call(http.MethodGet, assignmentPath(h.assignment(), "/attempts"), nil, nil)
	owner := login(t, h.dsn, "student_a", "student")
	missing := s5Call(h, http.MethodGet, assignmentPath("s5-missing", "/attempts"), nil, owner, nil)
	s5ExpectSame403(t, foreign, missing)
}

func TestAU_R4_S5_SummaryForeignMatchesMissing(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"actor":      "student_b",
		"assignment": map[string]any{"owner": "student_a", "kind": "mock", "state": "issued"},
	})
	foreign := h.call(http.MethodGet, assignmentPath(h.assignment(), "/summary"), nil, nil)
	owner := login(t, h.dsn, "student_a", "student")
	missing := s5Call(h, http.MethodGet, assignmentPath("s5-missing", "/summary"), nil, owner, nil)
	s5ExpectSame403(t, foreign, missing)
}

func TestAU_R4_S5_ManualVerdictForeignMatchesMissing(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"attempt": map[string]any{"verdict": "wrong"},
	})
	other := s5OtherTutor(h)
	foreign := s5Call(h, http.MethodPost, manualPath(h.attempt()), manualBody(), other, s5Ptr(appOrigin))
	missing := h.call(http.MethodPost, manualPath("s5-missing"), manualBody(), nil)
	s5ExpectSame403(t, foreign, missing)
}

func TestAC_R7_S5_ListAttemptsHidesVerdictBeforeSubmit(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "mock", "state": "issued"},
	})
	rec := h.call(http.MethodGet, assignmentPath(h.assignment(), "/attempts"), nil, nil)
	if rec.Code != http.StatusOK && rec.Code != http.StatusForbidden {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Code == http.StatusOK && s5BodyHasVerdict(t, rec.Body.Bytes()) {
		t.Fatalf("ученик видит вердикт до сдачи: %s", rec.Body.Bytes())
	}
}

func TestAT_R24_S5_TutorSeesVerdictBeforeSubmit(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "mock", "state": "issued"},
	})
	rec := s5Call(h, http.MethodGet, assignmentPath(h.assignment(), "/attempts"), nil, s5TutorToken(h), nil)
	h.status(rec, http.StatusOK)
	if !s5BodyHasVerdict(t, rec.Body.Bytes()) {
		t.Fatalf("репетитор не видит вердикт до сдачи: %s", rec.Body.Bytes())
	}
}

func TestAT_R4_S5_DraftOwner(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"draft": map[string]any{"items": []any{"v1", "v2"}},
	})
	other := s5OtherTutor(h)
	foreign := s5Call(h, http.MethodPatch, draftPath(h.lesson()), map[string]any{"remove": []any{"v2"}}, other, s5Ptr(appOrigin))
	missing := s5Call(h, http.MethodPatch, draftPath("s5-missing"), map[string]any{"remove": []any{"v1"}}, other, s5Ptr(appOrigin))
	s5ExpectSame403(t, foreign, missing)
	if !equalStrings(h.draftVariantIDs(), []string{"v1", "v2"}) {
		t.Fatalf("чужой репетитор изменил черновик: %v", h.draftVariantIDs())
	}
}

func TestAT_R4_S5_DraftItemsStayObjects(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"draft": map[string]any{"items": []any{"v1", "v2"}},
	})
	const stored = `[{"variant_id":"v1","type_id":"t1","source":"review","statement":"n=1"},{"variant_id":"v2","type_id":"t2","source":"new_topic","statement":"n=2"}]`
	if _, err := h.conn.Exec(context.Background(), `
		UPDATE core.homework_draft SET items = $1::jsonb WHERE lesson_id = $2
	`, stored, h.lesson()); err != nil {
		t.Fatal(err)
	}
	rec := h.call(http.MethodPatch, draftPath(h.lesson()), map[string]any{
		"remove": []any{"v2"},
	}, nil)
	h.status(rec, http.StatusOK)
	var raw []byte
	if err := h.conn.QueryRow(context.Background(), `
		SELECT items FROM core.homework_draft WHERE lesson_id = $1
	`, h.lesson()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("черновик не массив объектов: %s", raw)
	}
	if len(items) != 1 || items[0]["variant_id"] != "v1" || items[0]["source"] != "review" {
		t.Fatalf("после правки %s", raw)
	}
}

func TestAT_R14_S5_KeyBoundToAssignment(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 0},
	})
	first := h.call(http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, idem(s5IdemKey))
	h.status(first, http.StatusOK)
	ctx := context.Background()
	if _, err := h.conn.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		)
		SELECT $2, student_id, 'issued', kind, catalog_version, deadline, place, snapshot
		FROM core.assignment WHERE id = $1
	`, h.assignment(), s5OtherAssign); err != nil {
		t.Fatal(err)
	}
	if _, err := h.conn.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		SELECT $2, item_id, 'open', false, max_points
		FROM core.assignment_item WHERE assignment_id = $1
	`, h.assignment(), s5OtherAssign); err != nil {
		t.Fatal(err)
	}
	second := h.call(http.MethodPost, assignmentPath(s5OtherAssign, "/submit"), map[string]any{
		"confirm_unanswered": true,
	}, idem(s5IdemKey))
	h.status(second, http.StatusOK)
	var body struct {
		AssignmentID string `json:"assignment_id"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AssignmentID != s5OtherAssign {
		t.Fatalf("повтор ключа вернул выдачу %s", body.AssignmentID)
	}
	if h.count(`SELECT count(*) FROM core.journal_event WHERE kind = 'assignment_submitted'`) != 2 {
		t.Fatal("вторая выдача не получила своё событие")
	}
}

func TestAT_R15_S5_OutboxReadyForSender(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"release":    1,
		"assignment": map[string]any{"state": "issued"},
	})
	rec := h.call(http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, idem("s5-outbox"))
	h.status(rec, http.StatusOK)
	ready := h.count(`
		SELECT count(*) FROM core.outbox
		WHERE (kind = 'homework_submitted' OR template = 'homework_submitted')
		  AND COALESCE(state, 'pending') = 'pending'
		  AND next_attempt_at IS NOT NULL
		  AND next_attempt_at <= now()
	`)
	if ready != 1 {
		t.Fatalf("строк outbox, которые заберёт отправитель: %d", ready)
	}
}

func TestAT_R15_S5_SubmitClosesOpenItemWithAttempts(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued"},
		"item":       map[string]any{"state": "open", "verdicts": []any{"wrong"}},
	})
	rec := h.call(http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, idem("s5-close"))
	h.status(rec, http.StatusOK)
	if h.assignmentState(h.assignment()) != "submitted" {
		t.Fatalf("состояние %s", h.assignmentState(h.assignment()))
	}
	var state string
	if err := h.conn.QueryRow(context.Background(), `
		SELECT state FROM core.assignment_item
		WHERE assignment_id = $1 AND item_id = $2
	`, h.assignment(), h.item()).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "final" {
		t.Fatalf("элемент с попыткой остался %s", state)
	}
}

func TestAT_R16_S5_SummaryHiddenUntilSubmit(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "mock", "state": "issued"},
	})
	before := h.call(http.MethodGet, assignmentPath(h.assignment(), "/summary"), nil, nil)
	if before.Code != http.StatusConflict {
		t.Fatalf("итог до сдачи: %d %s", before.Code, before.Body.Bytes())
	}
	decodeError(t, before.Body.Bytes())
	submitted := h.call(http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{
		"confirm_unanswered": true,
	}, idem("s5-summary"))
	h.status(submitted, http.StatusOK)
	after := h.call(http.MethodGet, assignmentPath(h.assignment(), "/summary"), nil, nil)
	h.status(after, http.StatusOK)
	if !s5BodyHasVerdict(t, after.Body.Bytes()) {
		t.Fatalf("после сдачи нет вердикта: %s", after.Body.Bytes())
	}
}

func TestAT_R17_S5_DiscussListOneItem(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
	})
	path := itemPath(h.assignment(), h.item(), "/did-not-understand")
	h.status(h.call(http.MethodPost, path, nil, nil), http.StatusNoContent)
	h.status(h.call(http.MethodPost, path, nil, nil), http.StatusNoContent)
	rec := s5Call(h, http.MethodGet, s5StudentPath(h.world.StudentID, "/discuss-list"), nil, s5TutorToken(h), nil)
	h.status(rec, http.StatusOK)
	var items []struct {
		ItemID       string `json:"item_id"`
		AssignmentID string `json:"assignment_id"`
		VariantID    string `json:"variant_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("список: %v тело %s", err, rec.Body.Bytes())
	}
	if len(items) != 1 || items[0].ItemID != h.item() || items[0].AssignmentID != h.assignment() || items[0].VariantID == "" {
		t.Fatalf("список %+v", items)
	}
}

func TestAU_R4_S5_DiscussForeignMatchesMissing(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
	})
	tutor := s5TutorToken(h)
	foreignID := s5ForeignStudent(h)
	foreign := s5Call(h, http.MethodGet, s5StudentPath(foreignID, "/discuss-list"), nil, tutor, nil)
	missing := s5Call(h, http.MethodGet, s5StudentPath("s5-missing", "/discuss-list"), nil, tutor, nil)
	s5ExpectSame403(t, foreign, missing)
}

func TestAT_R18_S5_ListDisputesForTutor(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
		"dispute":    map[string]any{"open": true, "reason": "key_wrong"},
	})
	student := h.call(http.MethodGet, s5StudentPath(h.world.StudentID, "/disputes"), nil, nil)
	s5Expect403(t, h, student)
	rec := s5Call(h, http.MethodGet, s5StudentPath(h.world.StudentID, "/disputes"), nil, s5TutorToken(h), nil)
	h.status(rec, http.StatusOK)
	var items []struct {
		AssignmentID string `json:"assignment_id"`
		ItemID       string `json:"item_id"`
		Reason       string `json:"reason"`
		State        string `json:"state"`
		Text         string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("споры: %v тело %s", err, rec.Body.Bytes())
	}
	if len(items) != 1 || items[0].Reason != "key_wrong" || items[0].State != "open" || items[0].AssignmentID != h.assignment() || items[0].ItemID != h.item() {
		t.Fatalf("споры %+v", items)
	}
}

func TestAU_R4_S5_DisputesForeignMatchesMissing(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
		"dispute":    map[string]any{"open": true, "reason": "key_wrong"},
	})
	tutor := s5TutorToken(h)
	foreignID := s5ForeignStudent(h)
	foreign := s5Call(h, http.MethodGet, s5StudentPath(foreignID, "/disputes"), nil, tutor, nil)
	missing := s5Call(h, http.MethodGet, s5StudentPath("s5-missing", "/disputes"), nil, tutor, nil)
	s5ExpectSame403(t, foreign, missing)
}

func TestAT_R19_S5_ManualVerdictEvent(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"attempt": map[string]any{"verdict": "wrong"},
	})
	rec := h.call(http.MethodPost, manualPath(h.attempt()), manualBody(), nil)
	h.status(rec, http.StatusOK)
	if !s5ManualEvent(h, h.attempt()) {
		t.Fatal("нет manual_verdict и нет attempt_verdict с origin manual")
	}
}

func TestAC_R9_S5_KeyHiddenBeforeAttempt(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 1},
	})
	rec := h.call(http.MethodGet, assignmentPath(h.assignment(), ""), nil, nil)
	h.status(rec, http.StatusOK)
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if hasKey(body, "key") {
		t.Fatalf("ключ до попытки: %s", rec.Body.Bytes())
	}
}

func s5BodyHasVerdict(t *testing.T, raw []byte) bool {
	t.Helper()
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json: %v тело %s", err, raw)
	}
	return hasScoringVerdict(body)
}
