// Package events читает журнал как конверты docs/contracts/events.schema.json.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Queryer — запрос к Postgres. Метод Query как у pgx: подходят пул, соединение и транзакция.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type envelope struct {
	SchemaVersion int             `json:"schema_version"`
	JournalSeq    int64           `json:"journal_seq"`
	OccurredAt    string          `json:"occurred_at"`
	Actor         actor           `json:"actor"`
	EventType     string          `json:"event_type"`
	StudentID     string          `json:"student_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

type actor struct {
	Role string `json:"role"`
	ID   string `json:"id,omitempty"`
}

const documentsSQL = `
SELECT
    e.journal_seq,
    e.kind,
    e.payload,
    e.at,
    e.actor_role,
    e.actor_id,
    COALESCE(
        e.student_id,
        a.student_id,
        att_a.student_id,
        les.student_id,
        les_from.student_id,
        les_ids.student_id,
        stu.id,
        obs_asg.student_id,
        recheck_stu.student_id
    ) AS student_id
FROM core.journal_event AS e
LEFT JOIN core.assignment AS a
    ON a.id = e.payload->>'assignment_id'
LEFT JOIN core.attempts AS att
    ON att.id = e.payload->>'attempt_id'
LEFT JOIN core.assignment AS att_a
    ON att_a.id = att.assignment_id
LEFT JOIN core.lesson AS les
    ON les.id = e.payload->>'lesson_id'
LEFT JOIN core.lesson AS les_from
    ON les_from.id = e.payload->>'from_lesson_id'
LEFT JOIN core.lesson AS les_ids
    ON les_ids.id = e.payload->'lesson_ids'->>0
LEFT JOIN core.app_user AS stu
    ON stu.role = 'student'
   AND stu.id = COALESCE(NULLIF(e.payload->>'user_id', ''), NULLIF(e.payload->>'consent_id', ''))
LEFT JOIN LATERAL (
    SELECT asg.student_id
    FROM core.assignment AS asg
    WHERE e.kind = 'observation'
      AND strpos(e.payload->>'observation_id', asg.id || ':') = 1
    ORDER BY length(asg.id) DESC
    LIMIT 1
) AS obs_asg ON true
LEFT JOIN LATERAL (
    SELECT min(matched.student_id) AS student_id
    FROM (
        SELECT asg.student_id
        FROM core.journal_event AS v
        JOIN core.attempts AS att_r ON att_r.id = v.payload->>'attempt_id'
        JOIN core.assignment AS asg ON asg.id = att_r.assignment_id
        WHERE e.kind = 'recheck_completed'
          AND v.kind = 'attempt_verdict'
          AND v.payload->>'origin' = 'recheck'
          AND v.payload->>'correction_id' = e.payload->>'correction_id'
        GROUP BY asg.student_id
        HAVING array_agg(DISTINCT att_r.item_id ORDER BY att_r.item_id)
             = ARRAY(
                 SELECT item
                 FROM jsonb_array_elements_text(e.payload->'changed_items') AS item
                 ORDER BY item
             )
    ) AS matched
    HAVING count(*) = 1
) AS recheck_stu ON true
WHERE e.kind <> 'answer_request'
ORDER BY e.journal_seq
`

// Documents возвращает записи журнала конвертами events.schema.json
// в порядке journal_seq. Каждый элемент — один JSON-документ.
// kind answer_request — старый счётчик лимита, его нет в схеме, поэтому он не конверт.
// session_revoked требует student_id. Выход репетитора остаётся строкой таблицы и не возвращается.
func Documents(ctx context.Context, db Queryer) ([][]byte, error) {
	rows, err := db.Query(ctx, documentsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs [][]byte
	for rows.Next() {
		var (
			seq       int64
			kind      string
			payload   []byte
			at        time.Time
			actorRole *string
			actorID   *string
			studentID *string
		)
		if err := rows.Scan(&seq, &kind, &payload, &at, &actorRole, &actorID, &studentID); err != nil {
			return nil, err
		}
		if tutorLogoutWithoutStudent(kind, actorRole, studentID) {
			continue
		}
		raw, err := json.Marshal(buildEnvelope(seq, kind, payload, at, actorRole, actorID, studentID))
		if err != nil {
			return nil, err
		}
		docs = append(docs, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return docs, nil
}

func buildEnvelope(seq int64, kind string, payload []byte, at time.Time, actorRole, actorID, studentID *string) envelope {
	who := actor{Role: "system"}
	if actorRole != nil && *actorRole != "" {
		who.Role = *actorRole
	}
	if actorID != nil {
		who.ID = *actorID
	}
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	doc := envelope{
		SchemaVersion: 1,
		JournalSeq:    seq,
		OccurredAt:    at.UTC().Format(time.RFC3339),
		Actor:         who,
		EventType:     kind,
		Payload:       payload,
	}
	if studentID != nil && !globalKind(kind) {
		doc.StudentID = *studentID
	}
	return doc
}

func tutorLogoutWithoutStudent(kind string, actorRole, studentID *string) bool {
	if kind != "session_revoked" || actorRole == nil || *actorRole != "tutor" {
		return false
	}
	return studentID == nil || *studentID == ""
}

func globalKind(kind string) bool {
	switch kind {
	case "catalog_version_loaded", "device_confirmed", "tutor_rebound":
		return true
	default:
		return false
	}
}
