package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/compose"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/events"
)

// ВОПРОС: сквозной ответ пишет kind answer_request. Этого event_type нет в
// events.schema.json. Варианты: не писать answer_request в журнал; добавить тип
// в схему. Тест не выбирает и проверяет каждый документ через Validate.

func TestS7_WalkDocuments(t *testing.T) {
	dsn := dbtest.Start(t)
	applyMigrations(t, dsn)
	conn := connect(t, dsn)
	seedWalk(t, conn)

	ctx := context.Background()
	authSvc := openAuth(t, ctx, dsn)
	answerSvc := openAnswers(t, ctx, dsn)
	attemptSvc := openAttempts(t, ctx, dsn)
	handler := compose.Handler(dsn, authSvc.Handler(), answerSvc.Handler(), attemptSvc.Handler())

	student := login(t, handler, studentTelegram, "student", studentID)
	choiceBody := mustJSON(t, map[string]any{
		"answer":          []int{3},
		"idempotency_key": "s7-k-a",
	})
	choicePath := "/assignments/" + assignmentID + "/items/" + itemChoice + "/answer"
	first := call(t, handler, http.MethodPost, choicePath, choiceBody, student.token)
	expectStatus(t, first, http.StatusOK)

	integerPath := "/assignments/" + assignmentID + "/items/" + itemInteger + "/answer"
	integerBody := mustJSON(t, map[string]any{
		"answer":          "13",
		"idempotency_key": "s7-k-b",
	})
	integer := call(t, handler, http.MethodPost, integerPath, integerBody, student.token)
	expectStatus(t, integer, http.StatusOK)

	tutor := login(t, handler, tutorTelegram, "tutor", tutorID)
	listed := call(t, handler, http.MethodGet, "/assignments/"+assignmentID+"/attempts", nil, tutor.token)
	expectStatus(t, listed, http.StatusOK)

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
			JournalSeq int64  `json:"journal_seq"`
			OccurredAt string `json:"occurred_at"`
			StudentID  string `json:"student_id"`
			EventType  string `json:"event_type"`
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
			if head.StudentID != studentID {
				t.Fatalf("документ %d %s: student_id %q, нужен %s", i, head.EventType, head.StudentID, studentID)
			}
		}
	}
}
