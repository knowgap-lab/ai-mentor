package attempts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/events"
)

func TestS7_AnswerAndSubmit(t *testing.T) {
	h := newHarness(t)
	h.seed(map[string]any{
		"assignment": map[string]any{"state": "issued", "unanswered": 0},
	})
	h.recordAttempt()
	rec := h.call(http.MethodPost, assignmentPath(h.assignment(), "/submit"), map[string]any{}, idem("s7-submit"))
	h.status(rec, http.StatusOK)
	if h.world.StudentID == "" {
		t.Fatal("нет ученика")
	}
	s7CheckDocuments(t, context.Background(), h.conn, h.world.StudentID)
}

func s7CheckDocuments(t *testing.T, ctx context.Context, conn *pgx.Conn, studentID string) {
	t.Helper()
	docs, err := events.Documents(ctx, conn)
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("журнал пуст")
	}
	var prev int64
	for i, doc := range docs {
		if err := events.Validate(doc); err != nil {
			t.Fatalf("документ %d: %v\n%s", i, err, doc)
		}
		var head struct {
			JournalSeq int64   `json:"journal_seq"`
			OccurredAt string  `json:"occurred_at"`
			EventType  string  `json:"event_type"`
			StudentID  *string `json:"student_id"`
		}
		if err := json.Unmarshal(doc, &head); err != nil {
			t.Fatalf("документ %d: %v", i, err)
		}
		if head.JournalSeq <= prev {
			t.Fatalf("journal_seq %d не больше %d", head.JournalSeq, prev)
		}
		prev = head.JournalSeq
		if head.OccurredAt == "" {
			t.Fatalf("документ %d: пустой occurred_at", i)
		}
		switch head.EventType {
		case "catalog_version_loaded", "device_confirmed", "tutor_rebound":
		default:
			if head.StudentID == nil || *head.StudentID != studentID {
				got := ""
				if head.StudentID != nil {
					got = *head.StudentID
				}
				t.Fatalf("документ %d %s: student_id %q, нужен %s", i, head.EventType, got, studentID)
			}
		}
	}
}
