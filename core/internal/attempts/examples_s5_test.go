package attempts_test

import (
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/scenario"
)

// Прогонщик держит один набор параметров пути на сценарий.
// didNotUnderstand и getDiscussList оба пишут {id}, но это id выдачи и id ученика.
// Поэтому «не понял» остаётся в TestAT_R17_S5_DiscussListOneItem, а здесь только GET списка.

func TestAT_R16_S5_SummaryScenario(t *testing.T) {
	s5RequireExample(t, "at-16", "AT-R16")
	h := newHarness(t)
	scenario.Run(t, scenario.Options{
		Spec: s5Spec(t),
		Scenarios: []byte(`
- id: at-16
  given: {assignment: {kind: mock, state: issued}}
  steps:
    - op: getSummary
      expect: {http: 409}
    - op: submitAssignment
      request: {confirm_unanswered: true}
      expect: {http: 200, body: {assignment_id: a1}}
    - op: getSummary
      expect: {http: 200, body: {assignment_id: a1}}
`),
		Handler: h.svc.Handler(),
		Load: func(given map[string]any) (scenario.Bindings, error) {
			h.seed(given)
			return scenario.Bindings{
				Path:   map[string]string{"id": h.assignment()},
				Header: s5SessionHeaders(h.token, map[string]string{"Idempotency-Key": "s5-at-16"}),
			}, nil
		},
	})
}

func TestAT_R17_S5_DiscussListScenario(t *testing.T) {
	s5RequireExample(t, "at-17", "AT-R17")
	h := newHarness(t)
	scenario.Run(t, scenario.Options{
		Spec: s5Spec(t),
		Scenarios: []byte(`
- id: at-17
  given: {assignment: {state: submitted}}
  steps:
    - op: getDiscussList
      expect: {http: 200}
`),
		Handler: h.svc.Handler(),
		Load: func(given map[string]any) (scenario.Bindings, error) {
			h.seed(given)
			return scenario.Bindings{
				Path:   map[string]string{"id": h.world.StudentID},
				Header: s5SessionHeaders(s5TutorToken(h), nil),
			}, nil
		},
	})
}

func TestAT_R18_S5_DisputeListScenario(t *testing.T) {
	s5RequireExample(t, "at-18c", "AT-R18")
	h := newHarness(t)
	scenario.Run(t, scenario.Options{
		Spec: s5Spec(t),
		Scenarios: []byte(`
- id: at-18c
  given: {assignment: {state: submitted}, dispute: {open: true, reason: key_wrong}}
  steps:
    - op: listDisputes
      expect: {http: 200}
`),
		Handler: h.svc.Handler(),
		Load: func(given map[string]any) (scenario.Bindings, error) {
			h.seed(given)
			return scenario.Bindings{
				Path:   map[string]string{"id": h.world.StudentID},
				Header: s5SessionHeaders(s5TutorToken(h), nil),
			}, nil
		},
	})
}
