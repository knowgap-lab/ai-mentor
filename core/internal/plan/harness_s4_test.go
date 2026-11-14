package plan_test

import (
	"context"
	"encoding/json"
	"testing"
)

var s4WarningTypes = map[string]bool{
	"variants_needed":   true,
	"plan_does_not_fit": true,
	"consent_outdated":  true,
	"not_delivered":     true,
}

var s4DivergenceChanges = map[string]bool{
	"moved":      true,
	"added":      true,
	"cancelled":  true,
	"skipped":    true,
	"reordered":  true,
	"week_shift": true,
}

func s4PlanObject(t *testing.T, raw []byte) map[string]any {
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

func s4AssertPlanContract(t *testing.T, raw []byte) {
	t.Helper()
	body := s4PlanObject(t, raw)
	s4Only(t, body, "lessons", "warnings")
	s4Need(t, body, "lessons")
	lessons, ok := body["lessons"].([]any)
	if !ok {
		t.Fatalf("lessons %T", body["lessons"])
	}
	for _, item := range lessons {
		lesson, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("урок %T", item)
		}
		s4Only(t, lesson,
			"id", "template_id", "place_no", "title", "kind", "topics",
			"state", "starts_at", "planned_week", "marks", "moved", "added")
		s4Need(t, lesson, "id", "template_id", "place_no", "title", "state")
		state, _ := lesson["state"].(string)
		switch state {
		case "planned", "frozen", "held", "skipped":
		default:
			t.Fatalf("состояние %q", state)
		}
		place, ok := lesson["place_no"].(float64)
		if !ok || place < 1 || place != float64(int(place)) {
			t.Fatalf("place_no %v", lesson["place_no"])
		}
		if id, _ := lesson["id"].(string); id == "" {
			t.Fatal("пустой id")
		}
		if title, _ := lesson["title"].(string); title == "" {
			t.Fatal("пустое название")
		}
		if template, _ := lesson["template_id"].(string); template == "" {
			t.Fatal("пустой template_id")
		}
	}
	if _, ok := body["warnings"]; !ok {
		return
	}
	warnings, ok := body["warnings"].([]any)
	if !ok {
		t.Fatalf("warnings %T", body["warnings"])
	}
	for _, item := range warnings {
		warning, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("warning %T", item)
		}
		s4Only(t, warning, "type", "text", "params")
		s4Need(t, warning, "type")
		kind, _ := warning["type"].(string)
		if !s4WarningTypes[kind] {
			t.Fatalf("warning %q вне enum контракта", kind)
		}
	}
}

func s4AssertDivergence(t *testing.T, raw []byte) {
	t.Helper()
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("отчёт: %v, тело %s", err, raw)
	}
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("строка %T", item)
		}
		s4Only(t, row, "lesson_id", "change", "detail")
		s4Need(t, row, "lesson_id", "change")
		change, _ := row["change"].(string)
		if !s4DivergenceChanges[change] {
			t.Fatalf("change %q", change)
		}
		if id, _ := row["lesson_id"].(string); id == "" {
			t.Fatal("пустой lesson_id")
		}
	}
}

func s4CheckReordered(t *testing.T, raw []byte, subject string, ids []string) {
	t.Helper()
	obj := s4PlanObject(t, raw)
	s4Only(t, obj, "subject", "lesson_ids")
	s4Need(t, obj, "subject", "lesson_ids")
	if obj["subject"] != subject {
		t.Fatalf("subject %v", obj["subject"])
	}
	got, ok := obj["lesson_ids"].([]any)
	if !ok || len(got) != len(ids) || len(got) == 0 {
		t.Fatalf("lesson_ids %v", obj["lesson_ids"])
	}
	for i, id := range ids {
		if got[i] != id {
			t.Fatalf("порядок события %v", got)
		}
	}
}

func s4TemplatesAreArrays(t *testing.T, h *harness) {
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
