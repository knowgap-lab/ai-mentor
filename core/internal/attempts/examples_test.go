package attempts_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAT_R3_at03(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"attempt": map[string]any{"verdict": "wrong"},
	})
	rec := h.call(http.MethodPost, manualPath(h.attempt()), manualBody(), nil)
	h.status(rec, http.StatusOK)
	if got := h.verdicts(h.attempt()); !equalStrings(got, []string{"wrong", "correct"}) {
		t.Fatalf("вердикты %v", got)
	}
}

func TestAT_R4_at04(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"draft": map[string]any{"items": []any{"v1", "v2", "v3"}},
	})
	before := h.kinds()
	rec := h.call(http.MethodPatch, draftPath(h.lesson()), map[string]any{
		"remove": []any{"v2"},
	}, nil)
	h.status(rec, http.StatusOK)
	if len(before) != 0 || len(h.kinds()) != 0 {
		t.Fatalf("события журнала %v", h.kinds())
	}
	body := decodeDraft(t, rec.Body.Bytes())
	if !equalStrings(draftIDs(body), []string{"v1", "v3"}) {
		t.Fatalf("выдача черновика %v", draftIDs(body))
	}
	if !equalStrings(h.draftVariantIDs(), []string{"v1", "v3"}) {
		t.Fatalf("черновик в базе %v", h.draftVariantIDs())
	}
}

func TestAT_R5_at05(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment":     map[string]any{"state": "issued", "catalog_version": 6},
		"catalog_latest": 8,
	})
	id := h.assignment()
	version, snap := h.snapshot(id)
	if version != 6 {
		t.Fatalf("catalog_version %d", version)
	}
	expectStatement(t, snap)
	rec := h.call(http.MethodGet, assignmentPath(id, ""), nil, nil)
	h.status(rec, http.StatusOK)
	expectStatement(t, rec.Body.Bytes())
}

func TestAT_R6_at06a(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "student_answers": 3},
	})
	id := h.assignment()
	if h.count(`SELECT count(*) FROM core.attempts WHERE assignment_id = $1`, id) != 3 {
		t.Fatal("в выдаче не 3 ответа ученика")
	}
	before := h.kinds()
	rec := h.call(http.MethodPost, assignmentPath(id, "/withdraw"), map[string]any{
		"reason": "ошибка",
	}, nil)
	h.expectCode(rec, http.StatusConflict, "has_answers")
	h.expectAppended(before)
	if h.assignmentState(id) != "issued" {
		t.Fatalf("состояние %s", h.assignmentState(id))
	}
}

func TestAT_R6_at06b(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "student_answers": 0},
	})
	id := h.assignment()
	if h.count(`SELECT count(*) FROM core.attempts WHERE assignment_id = $1`, id) != 0 {
		t.Fatal("в выдаче уже есть ответы")
	}
	before := h.kinds()
	rec := h.call(http.MethodPost, assignmentPath(id, "/withdraw"), map[string]any{
		"reason": "ошибка",
	}, nil)
	h.status(rec, http.StatusNoContent)
	h.expectAppended(before, "assignment_withdrawn")
	if h.assignmentState(id) != "withdrawn" {
		t.Fatalf("состояние %s", h.assignmentState(id))
	}
}

func TestAT_R7_at07a(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "deadline": deadlineCurrent},
	})
	id := h.assignment()
	before := h.kinds()
	rec := h.call(http.MethodPut, assignmentPath(id, "/deadline"), map[string]any{
		"deadline": deadlineLater,
	}, nil)
	h.status(rec, http.StatusOK)
	h.expectAppended(before, "deadline_changed")
	if !h.deadlineIs(id, deadlineLater) {
		t.Fatal("срок не стал 22 ноября")
	}
	if h.assignmentState(id) != "issued" {
		t.Fatalf("состояние %s", h.assignmentState(id))
	}
}

func TestAT_R7_at07b(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "deadline": deadlineCurrent},
	})
	id := h.assignment()
	before := h.kinds()
	rec := h.call(http.MethodPut, assignmentPath(id, "/deadline"), map[string]any{
		"deadline": deadlineSooner,
	}, nil)
	h.expectCode(rec, http.StatusConflict, "deadline_shorter")
	h.expectAppended(before)
	if !h.deadlineIs(id, deadlineCurrent) {
		t.Fatal("срок сократился")
	}
}

func TestAT_R8_at08(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "checkpoint", "state": "issued"},
	})
	id := h.assignment()
	if h.count(`SELECT count(*) FROM core.attempts WHERE assignment_id = $1`, id) != 0 {
		t.Fatal("попытка уже записана до RecordAttempt")
	}
	h.recordAttempt()
	got := h.texts(`SELECT source || ':' || place FROM core.attempts WHERE assignment_id = $1`, id)
	if !equalStrings(got, []string{"checkpoint:lesson"}) {
		t.Fatalf("source:place %v", got)
	}
}

func TestAT_R11_at11a(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"item": map[string]any{"verdicts": []any{"wrong", "correct"}},
	})
	got := h.itemResult()
	if got.Verdict != "correct" || got.FirstAttemptPoints != 0 || got.TotalPoints != 2 {
		t.Fatalf("итог %+v", got)
	}
}

func TestAT_R11_at11b(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"item":   map[string]any{"verdicts": []any{"wrong", "correct"}},
		"manual": map[string]any{"attempt_no": 1, "verdict": "correct"},
	})
	got := h.itemResult()
	if got.EndsAtAttempt != 1 || got.FirstAttemptPoints != 2 || got.TotalPoints != 2 {
		t.Fatalf("итог %+v", got)
	}
}

func TestAT_R13_at13a(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 4},
	})
	id := h.assignment()
	before := h.kinds()
	rec := h.call(http.MethodPost, assignmentPath(id, "/submit"), map[string]any{}, idem("s1"))
	h.expectLeft(rec, http.StatusConflict, "unanswered_confirmation_required", 4)
	h.expectAppended(before)
	if h.assignmentState(id) != "issued" {
		t.Fatalf("состояние %s", h.assignmentState(id))
	}
}

func TestAT_R13_at13b(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 4},
	})
	id := h.assignment()
	before := h.kinds()
	rec := h.call(http.MethodPost, assignmentPath(id, "/submit"), map[string]any{
		"confirm_unanswered": true,
	}, idem("s2"))
	h.status(rec, http.StatusOK)
	body := decodeSubmit(t, rec.Body.Bytes())
	if body.AssignmentID != assignmentID || body.Answered == nil || *body.Answered != 6 ||
		body.Skipped == nil || *body.Skipped != 4 || len(body.Items) != 0 ||
		body.TotalPoints == nil || *body.TotalPoints != 8 ||
		body.MaxPoints == nil || *body.MaxPoints != 20 {
		t.Fatalf("итог %s", rec.Body.Bytes())
	}
	h.expectAppended(before, "assignment_submitted")
	if h.count(`SELECT count(*) FROM core.assignment_item WHERE assignment_id = $1 AND skipped`, id) != 4 {
		t.Fatal("пропущенных элементов не 4")
	}
}

func TestAT_R14_at14(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 0},
	})
	id := h.assignment()
	path := assignmentPath(id, "/submit")
	first := h.call(http.MethodPost, path, map[string]any{}, idem("s1"))
	h.status(first, http.StatusOK)
	second := h.call(http.MethodPost, path, map[string]any{}, idem("s1"))
	h.status(second, http.StatusOK)
	if canonicalJSON(t, first.Body.Bytes()) != canonicalJSON(t, second.Body.Bytes()) {
		t.Fatalf("повтор %s, первый ответ %s", second.Body.Bytes(), first.Body.Bytes())
	}
	var echoed struct {
		AssignmentID string `json:"assignment_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &echoed); err != nil || echoed.AssignmentID == "" {
		t.Fatalf("первый итог без assignment_id: %s", first.Body.Bytes())
	}
	if h.count(`SELECT count(*) FROM core.journal_event WHERE kind = 'assignment_submitted'`) != 1 {
		t.Fatal("событие assignment_submitted не одно")
	}
}

func TestAT_R15_at15(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"release":    1,
		"assignment": map[string]any{"state": "issued"},
	})
	rec := h.call(http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, idem("s1"))
	h.status(rec, http.StatusOK)
	if h.count(`SELECT count(*) FROM core.outbox WHERE kind = 'homework_submitted'`) != 1 {
		t.Fatal("в outbox не одно сообщение homework_submitted")
	}
	if h.queueJobs() != 0 {
		t.Fatalf("заданий пересчёта %d", h.queueJobs())
	}
}

func TestAT_R16_at16(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "mock", "state": "issued"},
	})
	id := h.assignment()
	view := h.call(http.MethodGet, assignmentPath(id, ""), nil, nil)
	h.status(view, http.StatusOK)
	var hidden any
	if err := json.Unmarshal(view.Body.Bytes(), &hidden); err != nil {
		t.Fatalf("выдача: %v тело %s", err, view.Body.Bytes())
	}
	if hasKey(hidden, "verdict") || hasKey(hidden, "last_verdict") || hasKey(hidden, "points") {
		t.Fatalf("до «Сдать» видны вердикты: %s", view.Body.Bytes())
	}
	submitted := h.call(http.MethodPost, assignmentPath(id, "/submit"), map[string]any{
		"confirm_unanswered": true,
	}, idem("s1"))
	h.status(submitted, http.StatusOK)
	var shown any
	if err := json.Unmarshal(submitted.Body.Bytes(), &shown); err != nil {
		t.Fatalf("итог: %v тело %s", err, submitted.Body.Bytes())
	}
	if !hasScoringVerdict(shown) {
		t.Fatalf("после «Сдать» нет вердикта: %s", submitted.Body.Bytes())
	}
}

func TestAT_R17_at17(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
	})
	path := itemPath(h.assignment(), h.item(), "/did-not-understand")
	if h.count(`SELECT count(*) FROM core.journal_event WHERE kind = 'did_not_understand'`) != 0 {
		t.Fatal("событие «не понял» уже было до нажатия")
	}
	first := h.call(http.MethodPost, path, nil, nil)
	h.status(first, http.StatusNoContent)
	if h.count(`SELECT count(*) FROM core.journal_event WHERE kind = 'did_not_understand'`) != 1 {
		t.Fatal("первое «не понял» не записало один элемент")
	}
	before := h.kinds()
	second := h.call(http.MethodPost, path, nil, nil)
	h.status(second, http.StatusNoContent)
	h.expectAppended(before)
	if h.count(`SELECT count(DISTINCT payload->>'item_id') FROM core.journal_event WHERE kind = 'did_not_understand'`) != 1 {
		t.Fatal("в списке «разобрать на уроке» не один элемент")
	}
}

func TestAT_R18_at18a(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
	})
	before := h.kinds()
	rec := h.call(http.MethodPost, itemPath(h.assignment(), h.item(), "/dispute"), map[string]any{
		"reason": "other",
		"text":   strings.Repeat("а", 400),
	}, nil)
	h.expectCode(rec, http.StatusUnprocessableEntity, "dispute_text_invalid")
	h.expectAppended(before)
	if h.count(`SELECT count(*) FROM core.dispute`) != 0 {
		t.Fatal("длинный текст всё же записал спор")
	}
}

func TestAT_R18_at18b(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
	})
	before := h.kinds()
	rec := h.call(http.MethodPost, itemPath(h.assignment(), h.item(), "/dispute"), map[string]any{
		"reason": "other",
		"text":   "позвоните +375 29 000 00 00",
	}, nil)
	h.expectCode(rec, http.StatusUnprocessableEntity, "dispute_text_invalid")
	h.expectAppended(before)
	if h.count(`SELECT count(*) FROM core.dispute`) != 0 {
		t.Fatal("телефон в тексте всё же записал спор")
	}
}

func TestAT_R18_at18c(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
		"dispute":    map[string]any{"open": true},
	})
	if h.count(`SELECT count(*) FROM core.dispute WHERE state = 'open'`) != 1 {
		t.Fatal("открытый спор не подготовлен")
	}
	before := h.kinds()
	rec := h.call(http.MethodPost, itemPath(h.assignment(), h.item(), "/dispute"), map[string]any{
		"reason": "key_wrong",
	}, nil)
	h.expectCode(rec, http.StatusConflict, "dispute_exists")
	h.expectAppended(before)
	if h.count(`SELECT count(*) FROM core.dispute`) != 1 {
		t.Fatal("второй спор записан")
	}
}

func TestAT_R19_at19(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"dispute":            map[string]any{"state": "open", "reason": "key_wrong"},
		"catalog_correction": map[string]any{"correction_id": 8, "variant_fixed": true},
	})
	if h.count(`SELECT count(*) FROM core.dispute WHERE state = 'open'`) != 1 {
		t.Fatal("спор не открыт до ResolveDisputeByCorrection")
	}
	before := h.count(`SELECT count(*) FROM core.attempt_verdict`)
	h.resolveByCorrection(8)
	if h.count(`SELECT count(*) FROM core.dispute WHERE state = 'resolved' AND resolved_by->>'correction_id' = '8'`) != 1 {
		t.Fatal("спор не закрыт ссылкой на correction_id 8")
	}
	if h.count(`SELECT count(*) FROM core.dispute`) != 1 {
		t.Fatal("спор не один")
	}
	if h.count(`SELECT count(*) FROM core.attempt_verdict`) != before+1 {
		t.Fatal("новый attempt_verdict не добавлен")
	}
}

func TestAT_R20_at20(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"item": map[string]any{"part": "B", "verdicts": []any{"wrong", "correct"}},
	})
	got := h.observations()
	if len(got) != 1 || got[0].revision != 1 || got[0].first != 0 || got[0].total != 2 || got[0].max != 2 {
		t.Fatalf("наблюдения %+v", got)
	}
}

func TestAT_R21_at21(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"item":         map[string]any{"state": "final"},
		"observations": []any{map[string]any{"revision": 1}},
	})
	if got := revisions(h.observations()); !equalInts(got, []int{1}) {
		t.Fatalf("до ручного вердикта %v", got)
	}
	rec := h.call(http.MethodPost, manualPath(h.attempt()), manualBody(), nil)
	h.status(rec, http.StatusOK)
	if !equalInts(revisions(h.observations()), []int{1, 2}) {
		t.Fatalf("наблюдения %+v", h.observations())
	}
}

func TestAT_R23_at23(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"actor":      "student_b",
		"assignment": map[string]any{"owner": "student_a"},
	})
	if h.world.ActorRole != "student" {
		t.Fatalf("актёр %s", h.world.ActorRole)
	}
	rec := h.call(http.MethodGet, assignmentPath(h.assignment(), "/attempts"), nil, nil)
	h.expectCode(rec, http.StatusForbidden, "forbidden")
}

func TestAT_R25_at25(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"student": map[string]any{"consent": "revoked", "anonymise_due": true},
	})
	journal := h.journalRows()
	if journal < 1 {
		t.Fatal("в журнале нет строк, неизменность нечего проверять")
	}
	if h.count(`SELECT count(*) FROM core.student_pii`) < 1 {
		t.Fatal("нет строки ПДн")
	}
	if h.count(`SELECT count(*) FROM core.student_pseudonym`) < 1 {
		t.Fatal("нет связи псевдонима")
	}
	h.anonymise()
	if h.journalRows() != journal {
		t.Fatalf("строк журнала было %d, стало %d", journal, h.journalRows())
	}
	if h.count(`SELECT count(*) FROM core.student_pii`) != 0 {
		t.Fatal("строка ПДн осталась")
	}
	if h.count(`SELECT count(*) FROM core.student_pseudonym`) != 0 {
		t.Fatal("связь псевдонима осталась")
	}
}

func expectStatement(t *testing.T, raw []byte) {
	t.Helper()
	got := statementValues(t, raw)
	if len(got) == 0 {
		t.Fatalf("нет statement: %s", raw)
	}
	for _, statement := range got {
		if statement != statementV6 {
			t.Fatalf("statement %q, нужно %q", statement, statementV6)
		}
	}
}

func statementValues(t *testing.T, raw []byte) []string {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("json: %v тело %s", err, raw)
	}
	var out []string
	var walk func(any)
	walk = func(node any) {
		switch x := node.(type) {
		case map[string]any:
			if statement, ok := x["statement"].(string); ok {
				out = append(out, statement)
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func revisions(rows []obsRow) []int {
	out := make([]int, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.revision)
	}
	return out
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
