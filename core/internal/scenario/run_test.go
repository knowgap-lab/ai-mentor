package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// Свой OpenAPI и сценарии живут строками в этом файле, не в docs/.

const expectFailEnv = "SCENARIO_RUN_EXPECT_FAIL"
const handlerMark = "SCENARIO_HANDLER_HIT"

const openAPI = `
openapi: 3.1.0
info:
  title: scenario-fixture
  version: 0.0.0
paths:
  /assignments/{id}/items/{item}/answer:
    post:
      operationId: answerItem
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: item
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
        "201":
          description: created
        "409":
          description: conflict
        "500":
          description: error
  /health/{item}:
    get:
      operationId: getHealth
      parameters:
        - name: item
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`

const dupOpenAPI = `
openapi: 3.1.0
info:
  title: scenario-dup
  version: 0.0.0
paths:
  /a:
    get:
      operationId: dupOp
      responses:
        "200":
          description: ok
  /b:
    post:
      operationId: dupOp
      responses:
        "200":
          description: ok
`

func TestScenario_MatchPasses(t *testing.T) {
	rec := &capture{}
	var loads int
	var actor any
	var kind any
	opts := options(openAPI, `
- id: match
  given: {actor: student_a, assignment: {kind: homework}}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect:
        http: 200
        body:
          status: scored
          show: {key: "3"}
`, rec.fixed(200, map[string]any{
		"status": "scored",
		"points": 1,
		"show":   map[string]any{"key": "3", "extra": true},
		"extra":  "yes",
	}), func(given map[string]any) (Bindings, error) {
		loads++
		actor = given["actor"]
		if assignment, ok := given["assignment"].(map[string]any); ok {
			kind = assignment["kind"]
		}
		return answerBindings(), nil
	}, nil)

	Run(t, opts)

	if loads != 1 {
		t.Fatalf("Load вызван %d раз, нужен 1 на сценарий", loads)
	}
	if actor != "student_a" || kind != "homework" {
		t.Fatalf("given actor=%#v kind=%#v", actor, kind)
	}
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("вызовов %d", len(calls))
	}
	if calls[0].method != http.MethodPost || calls[0].path != "/assignments/asg1/items/it1/answer" {
		t.Fatalf("запрос %s %s", calls[0].method, calls[0].path)
	}
	mustHeader(t, calls[0].header, "Cookie", "session=abc")
	mustHeader(t, calls[0].header, "X-CSRF-Token", "tok")
	mustJSONEqual(t, calls[0].body, `{"answer":"13"}`)
}

func TestScenario_WrongHTTPFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: bad-http
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 201, body: {status: scored, hint: ab}}
`, handlerMarking(200, map[string]any{"status": "scored", "hint": "ab"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_WrongBodyFieldFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: bad-body
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {status: accepted, hint: ab}}
`, handlerMarking(200, map[string]any{"status": "scored", "hint": "ab"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_MissingBodyFieldFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: missing-field
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {status: scored, hint: ab}}
`, handlerMarking(200, map[string]any{"status": "scored"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_MissingNestedBodyFieldFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: missing-nested
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {show: {key: "3", solution: x}}}
`, handlerMarking(200, map[string]any{
			"show": map[string]any{"key": "3"},
		}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_NoFieldsCatchesPresentKey(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: no-fields-present
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {status: scored}, no_fields: [show]}
`, handlerMarking(200, map[string]any{
			"status": "scored",
			"show":   map[string]any{"key": "3"},
		}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_NoFieldsCatchesNullKey(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: no-fields-null
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, no_fields: [show]}
`, handlerMarking(200, map[string]any{"show": nil}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_NoFieldsPassesWhenAbsent(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: no-fields-absent
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {status: scored, hint: ab}, no_fields: [show, verdict]}
`, rec.fixed(200, map[string]any{"status": "scored", "hint": "ab"}), staticLoad(answerBindings()), nil))
	if len(rec.calls()) != 1 {
		t.Fatalf("вызовов %d", len(rec.calls()))
	}
}

func TestScenario_RepeatExpandsBeforeCall(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: repeat
  given: {actor: a}
  steps:
    - op: answerItem
      request:
        text: {$repeat: ["ab", 3]}
        nested:
          note: {$repeat: ["xy", 2]}
        items:
          - {$repeat: ["z", 3]}
        weird:
          $repeat: ["a", 2]
          other: 1
        raw: "{$repeat: [a, 2]}"
      expect: {http: 200, body: {status: ok}}
`, rec.fixed(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), nil))
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("вызовов %d", len(calls))
	}
	mustJSONEqual(t, calls[0].body, `{
		"text": "ababab",
		"nested": {"note": "xyxy"},
		"items": ["zzz"],
		"weird": {"$repeat": ["a", 2], "other": 1},
		"raw": "{$repeat: [a, 2]}"
	}`)
}

func TestScenario_PathParamSubstituted(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: path
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200}
`, rec.fixed(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), nil))
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("вызовов %d", len(calls))
	}
	if calls[0].method != http.MethodPost {
		t.Fatalf("метод %s", calls[0].method)
	}
	if calls[0].path != "/assignments/asg1/items/it1/answer" {
		t.Fatalf("путь %s", calls[0].path)
	}
}

func TestScenario_MissingPathParamFails(t *testing.T) {
	runExpectingFailure(t, false, func() {
		Run(t, options(openAPI, `
- id: missing-path
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200}
`, handlerMarking(200, map[string]any{"status": "ok"}), staticLoad(Bindings{
			Path:   map[string]string{"id": "asg1"},
			Header: map[string]string{"Cookie": "session=abc"},
		}), nil))
	})
}

func TestScenario_JournalMatches(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	reads := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		mu.Lock()
		attempts++
		mu.Unlock()
		writeJSON(w, 200, map[string]any{"status": "scored"})
	})
	journal := func() map[string]any {
		mu.Lock()
		defer mu.Unlock()
		reads++
		return map[string]any{
			"attempts": attempts,
			"late":     true,
			"label":    "ok",
			"extra":    "x",
		}
	}
	Run(t, options(openAPI, `
- id: journal
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {status: scored}, journal: {attempts: 1, late: true, label: ok}}
`, h, staticLoad(answerBindings()), journal))
	mu.Lock()
	defer mu.Unlock()
	if attempts != 1 || reads < 1 {
		t.Fatalf("attempts=%d journalReads=%d", attempts, reads)
	}
}

func TestScenario_JournalNumberEquals(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: journal-num
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, journal: {points: 1, nums: [1, 2]}}
`, rec.fixed(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), func() map[string]any {
		return map[string]any{"points": float64(1), "nums": []any{float64(1), float64(2)}}
	}))
	if len(rec.calls()) != 1 {
		t.Fatalf("вызовов %d", len(rec.calls()))
	}
}

func TestScenario_JournalMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: journal-bad
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, journal: {attempts: 2}}
`, handlerMarking(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), func() map[string]any {
			return map[string]any{"attempts": 1}
		}))
	})
}

func TestScenario_JournalNilFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: journal-nil
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, journal: {attempts: 1}}
`, handlerMarking(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_SameBodyCallsHandlerTwice(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: twice
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13", idempotency_key: k1}
      expect: {http: 200, body: {status: scored}}
    - op: answerItem
      request: {answer: "13", idempotency_key: k1}
      expect: {http: 200, body: {status: scored}}
`, rec.fixed(200, map[string]any{"status": "scored"}), staticLoad(answerBindings()), nil))
	calls := rec.calls()
	if len(calls) != 2 {
		t.Fatalf("вызовов %d, нужны оба шага", len(calls))
	}
	mustJSONEqual(t, calls[0].body, `{"answer":"13","idempotency_key":"k1"}`)
	mustJSONEqual(t, calls[1].body, `{"answer":"13","idempotency_key":"k1"}`)
	if calls[0].path != calls[1].path || calls[0].method != http.MethodPost || calls[1].method != http.MethodPost {
		t.Fatalf("запросы %#v %#v", calls[0], calls[1])
	}
}

func TestScenario_UnknownOperationFails(t *testing.T) {
	runExpectingFailure(t, false, func() {
		Run(t, options(openAPI, `
- id: unknown-op
  given: {actor: a}
  steps:
    - op: missingOp
      request: {answer: "13"}
      expect: {http: 200}
`, handlerMarking(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_ErrorCodeMatches(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: err-ok
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 409, error: {code: item_closed}}
`, rec.fixed(409, map[string]any{
		"error": map[string]any{"code": "item_closed", "details": map[string]any{"left": 1}},
		"trace": "t",
	}), staticLoad(answerBindings()), nil))
	if len(rec.calls()) != 1 {
		t.Fatalf("вызовов %d", len(rec.calls()))
	}
}

func TestScenario_ErrorCodeMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: err-bad
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 409, error: {code: item_closed}}
`, handlerMarking(409, map[string]any{
			"error": map[string]any{"code": "forbidden"},
		}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_ErrorCodeMustBeNested(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: err-nest
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 409, error: {code: item_closed}}
`, handlerMarking(409, map[string]any{"code": "item_closed"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_HeadersMatchIgnoringNameCase(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("Retry-After", "7")
		w.Header().Set("X-Trace", "AbC")
		w.Header().Set("X-Extra", "1")
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})
	Run(t, options(openAPI, `
- id: headers
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect:
        http: 200
        headers:
          retry-after: "7"
          x-trace: AbC
`, h, staticLoad(answerBindings()), nil))
}

func TestScenario_HeaderMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(os.Stderr, "%s\n", handlerMark)
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			w.Header().Set("Retry-After", "8")
			writeJSON(w, 200, map[string]any{"status": "ok"})
		})
		Run(t, options(openAPI, `
- id: header-bad
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, headers: {retry-after: "7"}}
`, h, staticLoad(answerBindings()), nil))
	})
}

func TestScenario_HeaderValueCaseMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(os.Stderr, "%s\n", handlerMark)
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			w.Header().Set("X-Trace", "AbC")
			writeJSON(w, 200, map[string]any{"status": "ok"})
		})
		Run(t, options(openAPI, `
- id: header-case
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, headers: {x-trace: abc}}
`, h, staticLoad(answerBindings()), nil))
	})
}

func TestScenario_MissingHeaderFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: header-missing
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, headers: {retry-after: "7"}}
`, handlerMarking(200, map[string]any{"status": "ok"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_NumbersOneAndOnePointZeroMatch(t *testing.T) {
	rec := &capture{}
	body := map[string]any{"points": 1, "nums": []any{1, 2}}
	Run(t, options(openAPI, `
- id: numbers
  given: {actor: a}
  steps:
    - op: answerItem
      request: {n: 1}
      expect: {http: 200, body: {points: 1, nums: [1, 2]}}
    - op: answerItem
      request: {n: 2}
      expect: {http: 200, body: {points: 1.0, nums: [1.0, 2.0]}}
`, rec.fixed(200, body), staticLoad(answerBindings()), nil))
	if len(rec.calls()) != 2 {
		t.Fatalf("вызовов %d", len(rec.calls()))
	}
}

func TestScenario_NumberMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: num-bad
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {points: 1}}
`, handlerMarking(200, map[string]any{"points": 2}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_StringDoesNotMatchNumber(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: type-bad
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {points: 1}}
`, handlerMarking(200, map[string]any{"points": "1"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_StringsAndListsExact(t *testing.T) {
	rec := &capture{}
	Run(t, options(openAPI, `
- id: exact
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {hint: ab, tags: [a, b]}}
`, rec.fixed(200, map[string]any{"hint": "ab", "tags": []any{"a", "b"}}), staticLoad(answerBindings()), nil))
	if len(rec.calls()) != 1 {
		t.Fatalf("вызовов %d", len(rec.calls()))
	}
}

func TestScenario_StringMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: str-bad
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {hint: ab}}
`, handlerMarking(200, map[string]any{"hint": "abc"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_ListOrderMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: list-order
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {tags: [a, b]}}
`, handlerMarking(200, map[string]any{"tags": []any{"b", "a"}}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_ListLengthMismatchFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: list-len
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200, body: {tags: [a, b]}}
`, handlerMarking(200, map[string]any{"tags": []any{"a", "b", "c"}}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_ParallelStepsBothRun(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	reads := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		_ = r.Body.Close()
		ans, _ := req["answer"].(string)
		mu.Lock()
		if err != nil {
			seen["decode-error"]++
		} else {
			seen[ans]++
		}
		mu.Unlock()
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})
	journal := func() map[string]any {
		mu.Lock()
		defer mu.Unlock()
		reads++
		return map[string]any{"n": len(seen)}
	}
	Run(t, options(openAPI, `
- id: parallel
  given: {actor: a}
  steps:
    - parallel:
        - op: answerItem
          request: {answer: "13"}
          expect: {http: 200, body: {status: ok}}
        - op: answerItem
          request: {answer: "14"}
          expect: {http: 200, body: {status: ok}}
      expect: {journal: {n: 2}}
`, h, staticLoad(answerBindings()), journal))
	mu.Lock()
	defer mu.Unlock()
	if seen["13"] != 1 || seen["14"] != 1 || seen["decode-error"] != 0 {
		t.Fatalf("вызовы %v", seen)
	}
	if reads < 1 {
		t.Fatal("Journal не вызван после parallel")
	}
}

func TestScenario_ParallelChildExpectFails(t *testing.T) {
	runExpectingFailure(t, true, func() {
		Run(t, options(openAPI, `
- id: parallel-bad
  given: {actor: a}
  steps:
    - parallel:
        - op: answerItem
          request: {answer: "13"}
          expect: {http: 200}
        - op: answerItem
          request: {answer: "14"}
          expect: {http: 200}
`, handlerMarking(500, map[string]any{"status": "err"}), staticLoad(answerBindings()), nil))
	})
}

func TestScenario_LoadOncePerScenario(t *testing.T) {
	var mu sync.Mutex
	loads := 0
	var actors []string
	rec := &capture{}
	load := func(given map[string]any) (Bindings, error) {
		actor, _ := given["actor"].(string)
		mu.Lock()
		loads++
		actors = append(actors, actor)
		mu.Unlock()
		return Bindings{
			Path:   map[string]string{"item": actor},
			Header: map[string]string{"X-Actor": actor},
		}, nil
	}
	Run(t, options(openAPI, `
- id: s1
  given: {actor: a1}
  steps:
    - op: getHealth
      expect: {http: 200}
    - op: getHealth
      expect: {http: 200}
- id: s2
  given: {actor: a2}
  steps:
    - op: getHealth
      expect: {http: 200}
`, rec.fixed(200, map[string]any{}), load, nil))
	mu.Lock()
	defer mu.Unlock()
	if loads != 2 || !reflect.DeepEqual(actors, []string{"a1", "a2"}) {
		t.Fatalf("Load %d раз, actors %v", loads, actors)
	}
	calls := rec.calls()
	if len(calls) != 3 {
		t.Fatalf("вызовов %d", len(calls))
	}
	want := []string{"/health/a1", "/health/a1", "/health/a2"}
	for i, c := range calls {
		if c.method != http.MethodGet || c.path != want[i] {
			t.Fatalf("запрос %d: %s %s", i, c.method, c.path)
		}
		if c.header.Get("X-Actor") != strings.TrimPrefix(want[i], "/health/") {
			t.Fatalf("X-Actor %q на %s", c.header.Get("X-Actor"), c.path)
		}
	}
}

func TestScenario_LoadErrorFails(t *testing.T) {
	runExpectingFailure(t, false, func() {
		Run(t, options(openAPI, `
- id: load-err
  given: {actor: a}
  steps:
    - op: answerItem
      request: {answer: "13"}
      expect: {http: 200}
`, handlerMarking(200, map[string]any{"status": "ok"}), failLoad, nil))
	})
}

func TestScenario_DuplicateOperationFails(t *testing.T) {
	runExpectingFailure(t, false, func() {
		Run(t, options(dupOpenAPI, `
- id: dup
  given: {actor: a}
  steps:
    - op: dupOp
      expect: {http: 200}
`, handlerMarking(200, map[string]any{"status": "ok"}), staticLoad(Bindings{}), nil))
	})
}

// runExpectingFailure перезапускает этот же тест в дочернем процессе.
// Run роняет дочерний *testing.T, внешний тест при этом остаётся зелёным.
func runExpectingFailure(t *testing.T, wantHandler bool, run func()) {
	t.Helper()
	if os.Getenv(expectFailEnv) == t.Name() {
		run()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^"+regexp.QuoteMeta(t.Name())+"$",
		"-test.count=1",
		"-test.timeout=20s",
	)
	cmd.Env = append(withoutEnv(os.Environ(), expectFailEnv), expectFailEnv+"="+t.Name())
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("дочерний тест завис\n%s", tail(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run должен провалить тест, err=%v\n%s", err, tail(out))
	}
	hit := strings.Contains(string(out), handlerMark)
	if hit != wantHandler {
		t.Fatalf("обработчик вызван=%v, нужно %v\n%s", hit, wantHandler, tail(out))
	}
}

func options(spec, scenarios string, h http.Handler, load Loader, journal func() map[string]any) Options {
	return Options{
		Spec:      []byte(spec),
		Scenarios: []byte(scenarios),
		Handler:   h,
		Load:      load,
		Journal:   journal,
	}
}

func answerBindings() Bindings {
	return Bindings{
		Path: map[string]string{"id": "asg1", "item": "it1"},
		Header: map[string]string{
			"Cookie":       "session=abc",
			"X-CSRF-Token": "tok",
		},
	}
}

func staticLoad(b Bindings) Loader {
	return func(map[string]any) (Bindings, error) {
		return b, nil
	}
}

func failLoad(map[string]any) (Bindings, error) {
	return Bindings{}, errors.New("load failed")
}

type call struct {
	method string
	path   string
	body   []byte
	header http.Header
}

type capture struct {
	mu   sync.Mutex
	recs []call
}

func (c *capture) fixed(code int, body any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		c.mu.Lock()
		c.recs = append(c.recs, call{
			method: r.Method,
			path:   r.URL.Path,
			body:   buf,
			header: r.Header.Clone(),
		})
		if err != nil {
			c.recs[len(c.recs)-1].body = []byte("read error")
		}
		c.mu.Unlock()
		writeJSON(w, code, body)
	})
}

func (c *capture) calls() []call {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]call, len(c.recs))
	copy(out, c.recs)
	return out
}

func handlerMarking(code int, body any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(os.Stderr, "%s\n", handlerMark)
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		writeJSON(w, code, body)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func mustHeader(t *testing.T, h http.Header, key, want string) {
	t.Helper()
	if got := h.Get(key); got != want {
		t.Fatalf("заголовок %s=%q, ожидали %q", key, got, want)
	}
}

func mustJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if len(bytes.TrimSpace(got)) == 0 {
		t.Fatalf("пустое тело, ожидали %s", want)
	}
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("json запроса: %v, тело %q", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("эталон: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("тело %s, ожидали %s", got, want)
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

func tail(b []byte) string {
	s := string(b)
	if len(s) > 4000 {
		return s[len(s)-4000:]
	}
	return s
}
