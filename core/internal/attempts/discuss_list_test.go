package attempts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestAT_R17_DiscussListVariantWhenSnapshotHasItemOnly(t *testing.T) {
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
	if len(items) != 1 || items[0].ItemID != h.item() || items[0].AssignmentID != h.assignment() || items[0].VariantID != h.item() {
		t.Fatalf("список %+v", items)
	}
}

func TestAT_R17_DiscussListReadsIssuedVariant(t *testing.T) {
	const issuedVariant = "ct-demo:D1.T01.S1.V017"
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "submitted"},
	})
	if _, err := h.conn.Exec(context.Background(), `
		UPDATE core.assignment_item
		SET item_id = $2
		WHERE assignment_id = $1 AND item_id = $3
	`, h.assignment(), issuedVariant, h.item()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.conn.Exec(context.Background(), `
		UPDATE core.assignment
		SET snapshot = jsonb_build_object(
			'items', jsonb_build_array(jsonb_build_object(
				'variant_id', $2::text,
				'statement', 'условие'
			))
		)
		WHERE id = $1
	`, h.assignment(), issuedVariant); err != nil {
		t.Fatal(err)
	}
	path := itemPath(h.assignment(), issuedVariant, "/did-not-understand")
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
	if len(items) != 1 || items[0].ItemID != issuedVariant || items[0].AssignmentID != h.assignment() || items[0].VariantID != issuedVariant {
		t.Fatalf("список %+v", items)
	}
}
