package attempts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ReadItemResult читает текущие вердикты элемента и сводит их по AT-R11.
// Вызов в harness назван ItemResult — так же, как этот тип; в Go это одно имя.
func ReadItemResult(ctx context.Context, conn *pgx.Conn, assignmentID, itemID string) (ItemResult, error) {
	attempts, err := loadScored(ctx, conn, assignmentID, itemID)
	if err != nil {
		return ItemResult{}, err
	}
	result, ok := summarize(attempts)
	if !ok {
		return ItemResult{}, fmt.Errorf("no attempts for %s/%s", assignmentID, itemID)
	}
	return result, nil
}

func loadScored(ctx context.Context, q queryer, assignmentID, itemID string) ([]scored, error) {
	rows, err := q.Query(ctx, `
		SELECT a.attempt_no, v.verdict, v.points
		FROM core.attempts AS a
		JOIN LATERAL (
			SELECT verdict, points
			FROM core.attempt_verdict
			WHERE attempt_id = a.id
			ORDER BY seq DESC
			LIMIT 1
		) AS v ON true
		WHERE a.assignment_id = $1 AND a.item_id = $2
		ORDER BY a.attempt_no
	`, assignmentID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scored
	for rows.Next() {
		var row scored
		if err := rows.Scan(&row.no, &row.verdict, &row.points); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type itemScore struct {
	id       string
	state    string
	max      int
	attempts []scored
}

func loadItems(ctx context.Context, q queryer, assignmentID string) ([]itemScore, error) {
	rows, err := q.Query(ctx, `
		SELECT item_id, state, max_points
		FROM core.assignment_item
		WHERE assignment_id = $1
		ORDER BY item_id
	`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []itemScore
	for rows.Next() {
		var item itemScore
		if err := rows.Scan(&item.id, &item.state, &item.max); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	attemptRows, err := q.Query(ctx, `
		SELECT a.item_id, a.attempt_no, v.verdict, v.points
		FROM core.attempts AS a
		JOIN LATERAL (
			SELECT verdict, points
			FROM core.attempt_verdict
			WHERE attempt_id = a.id
			ORDER BY seq DESC
			LIMIT 1
		) AS v ON true
		WHERE a.assignment_id = $1
		ORDER BY a.item_id, a.attempt_no
	`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer attemptRows.Close()
	byItem := map[string][]scored{}
	for attemptRows.Next() {
		var itemID string
		var row scored
		if err := attemptRows.Scan(&itemID, &row.no, &row.verdict, &row.points); err != nil {
			return nil, err
		}
		byItem[itemID] = append(byItem[itemID], row)
	}
	if err := attemptRows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].attempts = byItem[items[i].id]
	}
	return items, nil
}

func insertVerdict(ctx context.Context, q queryer, attemptID, verdict string, points int, reason *string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points, reason)
		VALUES (
			$1,
			(SELECT COALESCE(MAX(seq), 0) + 1 FROM core.attempt_verdict WHERE attempt_id = $1),
			$2, $3, $4
		)
	`, attemptID, verdict, points, reason)
	return err
}

func insertObservation(ctx context.Context, q queryer, assignmentID, itemID string, result ItemResult, maxPoints int) error {
	_, err := q.Exec(ctx, `
		INSERT INTO core.observation (
			assignment_id, item_id, revision, first_attempt_points, total_points, max_points
		)
		VALUES (
			$1, $2,
			(SELECT COALESCE(MAX(revision), 0) + 1
			 FROM core.observation
			 WHERE assignment_id = $1 AND item_id = $2),
			$3, $4, $5
		)
	`, assignmentID, itemID, result.FirstAttemptPoints, result.TotalPoints, maxPoints)
	return err
}

func insertEvent(ctx context.Context, q queryer, kind string, payload []byte) error {
	_, err := q.Exec(ctx, `
		INSERT INTO core.journal_event (journal_seq, kind, payload, at)
		VALUES (nextval('core.journal_seq'), $1, $2::jsonb, now())
	`, kind, string(payload))
	return err
}
