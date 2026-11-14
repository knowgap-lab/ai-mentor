package recheck_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/answerrules"
	"github.com/knowgap-lab/ai-mentor/core/internal/events"
	"github.com/knowgap-lab/ai-mentor/core/internal/recheck"
)

func TestS7_Recheck(t *testing.T) {
	conn, _ := newDB(t)
	answer := "14"
	scored := answerrules.Score(formatInteger, oldKey, answer, nil)
	if scored.Status != "scored" {
		t.Fatalf("ответ %q не оценён: %+v", answer, scored)
	}
	insertAttempt(t, conn, answer, scored, false)
	ctx := context.Background()
	if _, err := recheck.Accept(ctx, conn, correctionID, []string{variantID}); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if _, err := recheck.Run(ctx, conn, correctionID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	s7CheckDocuments(t, ctx, conn, "student-1")
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
