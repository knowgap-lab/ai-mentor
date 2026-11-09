package answercheck_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAC_R7_ac01(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "A", "format": "ct.choice1", "key": []any{3},
			"state": "open", "attempts": 0,
		},
	})
	rec := h.call(answerBody([]any{3}, "k1"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "final", "attempts_left": 0,
		"verdict": "correct", "points": 1, "max_points": 1,
	}, []string{"show"})
}

func TestAC_R7_ac02(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "A", "format": "ct.choice1", "key": []any{3},
			"state": "open", "attempts": 0,
		},
	})
	rec := h.call(answerBody([]any{2}, "k1"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "final", "attempts_left": 0,
		"verdict": "wrong", "points": 0, "max_points": 1,
		"show": map[string]any{"key": "3", "solution_block": solutionBlock},
	}, nil)
}

func TestAC_R7_ac03(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 0,
		},
	})
	rec := h.call(answerBody("13", "k1"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "open", "attempts_left": 1,
		"verdict": "wrong", "points": 0, "max_points": 2,
	}, []string{"show"})
}

func TestAC_R7_ac04(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 1, "verdicts": []any{"wrong"},
		},
	})
	rec := h.call(answerBody("14", "k2"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "final", "attempts_left": 0,
		"verdict": "wrong", "points": 0, "max_points": 2,
		"show": map[string]any{"key": "12", "solution_block": solutionBlock},
	}, nil)
}

func TestAC_R11_ac05(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 1, "verdicts": []any{"wrong"},
		},
	})
	rec := h.call(answerBody("12", "k2"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "final", "attempts_left": 0,
		"verdict": "correct", "points": 2, "max_points": 2,
	}, []string{"show"})
	n, first, total := h.observation()
	if n != 1 || first != 0 || total != 2 {
		t.Fatalf("наблюдение: строк %d, первая %d, итог %d", n, first, total)
	}
}

func TestAC_R5_ac06(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 0,
		},
	})
	rec := h.call(answerBody("12,0", "k1"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "malformed", "hint": "введите целое число",
		"item_state": "open", "attempts_left": 2,
	}, nil)
	if h.attemptCount() != 0 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R6_ac07(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item":       map[string]any{"state": "final"},
	})
	rec := h.call(answerBody("12", "k1"))
	h.expectCode(rec, http.StatusConflict, "item_closed")
	if h.attemptCount() != 0 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R6_ac08(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "submitted"},
		"item":       map[string]any{"state": "final"},
	})
	rec := h.call(answerBody("12", "k1"))
	h.expectCode(rec, http.StatusConflict, "assignment_closed")
	if h.attemptCount() != 0 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R7_ac09(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "checkpoint", "state": "issued"},
		"item": map[string]any{
			"part": "A", "format": "ct.choice1", "key": []any{3},
			"state": "open", "attempts": 0,
		},
	})
	rec := h.call(answerBody([]any{5}, "k1"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "accepted", "item_state": "final",
	}, []string{"verdict", "points", "show"})
}

func TestAC_R4_ac10(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 0,
		},
	})
	body := answerBody("13", "k1")
	first := h.call(body)
	h.expectFields(first, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "open", "attempts_left": 1,
		"verdict": "wrong", "points": 0, "max_points": 2,
	}, []string{"show"})
	second := h.call(body)
	h.status(second, http.StatusOK)
	if canonicalJSON(t, first.Body.Bytes()) != canonicalJSON(t, second.Body.Bytes()) {
		t.Fatalf("повтор %s, первый ответ %s", second.Body.Bytes(), first.Body.Bytes())
	}
	if h.attemptCount() != 1 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R4_ac11(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 0,
		},
	})
	first := h.call(answerBody("13", "k1"))
	h.status(first, http.StatusOK)
	second := h.call(answerBody("14", "k1"))
	h.expectCode(second, http.StatusConflict, "idempotency_conflict")
	if h.attemptCount() != 1 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R2_ac12(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued"},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 0,
		},
	})
	reqs := []*http.Request{
		h.request(answerBody("13", "ka")),
		h.request(answerBody("14", "kb")),
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	var recs [2]*httptest.ResponseRecorder
	for i := range reqs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			recs[i] = h.serve(reqs[i])
		}(i)
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("параллельные ответы не завершились")
	}
	for i, rec := range recs {
		if rec == nil {
			t.Fatalf("ответ %d пуст", i)
		}
		h.status(rec, http.StatusOK)
	}
	if got := h.attemptNumbers(); !equalInts(got, []int{1, 2}) {
		t.Fatalf("номера попыток %v", got)
	}
}

func TestAC_R3_ac13(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued", "owner": "student_a"},
		"actor":      "student_b",
	})
	rec := h.call(answerBody("12", "k1"))
	h.expectCode(rec, http.StatusForbidden, "forbidden")
	if h.attemptCount() != 0 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R15_ac15(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment":              map[string]any{"kind": "homework", "state": "issued"},
		"requests_in_last_minute": 60,
	})
	rec := h.call(answerBody("12", "k61"))
	h.expectCode(rec, http.StatusTooManyRequests, "rate_limited")
	// В примере заголовок Retry-After: present. Контракт задаёт целое, число не задаёт.
	h.expectRetryAfter(rec)
	if h.attemptCount() != 0 {
		t.Fatalf("попыток %d", h.attemptCount())
	}
}

func TestAC_R16_ac16(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"kind": "homework", "state": "issued", "deadline_passed": true},
		"item": map[string]any{
			"part": "B", "format": "ct.integer", "key": 12,
			"state": "open", "attempts": 0,
		},
	})
	rec := h.call(answerBody("12", "k1"))
	h.expectFields(rec, http.StatusOK, map[string]any{
		"status": "scored", "item_state": "final",
		"verdict": "correct", "points": 2, "max_points": 2,
	}, []string{"show"})
	if got := h.lateFlags(); !equalStrings(got, []string{"true"}) {
		t.Fatalf("late %v", got)
	}
}

// ac-14 в answer-check.examples.yaml нет. См. ВОПРОС в harness_test.go.
