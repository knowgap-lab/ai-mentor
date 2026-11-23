package recheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func earlierOpen(ctx context.Context, tx pgx.Tx, correctionID int) (bool, error) {
	var open bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM core.recheck_batch
			WHERE correction_id < $1
			  AND state IN ('pending', 'running')
		)
	`, correctionID).Scan(&open)
	return open, err
}

type changedItem struct {
	assignment string
	item       string
	student    string
}

func finishBatch(ctx context.Context, tx pgx.Tx, correctionID int) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT asg.id, a.item_id, asg.student_id
		FROM core.journal_event AS e
		JOIN core.attempts AS a ON a.id = e.payload->>'attempt_id'
		JOIN core.assignment AS asg ON asg.id = a.assignment_id
		WHERE e.kind = 'attempt_verdict'
		  AND e.payload->>'origin' = 'recheck'
		  AND e.payload->>'correction_id' = $1
	`, strconv.Itoa(correctionID))
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []changedItem
	for rows.Next() {
		var item changedItem
		if err := rows.Scan(&item.assignment, &item.item, &item.student); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if len(items) == 0 {
		return nil
	}

	seenItem := map[string]bool{}
	for _, item := range items {
		key := item.assignment + "\x00" + item.item
		if seenItem[key] {
			continue
		}
		seenItem[key] = true
		if err := refreshObservation(ctx, tx, item.assignment, item.item); err != nil {
			return err
		}
	}

	byStudent := map[string][]string{}
	for _, item := range items {
		if slices.Contains(byStudent[item.student], item.item) {
			continue
		}
		byStudent[item.student] = append(byStudent[item.student], item.item)
	}
	students := make([]string, 0, len(byStudent))
	for student := range byStudent {
		students = append(students, student)
	}
	slices.Sort(students)
	for _, student := range students {
		slices.Sort(byStudent[student])
		body, err := json.Marshal(struct {
			CorrectionID int      `json:"correction_id"`
			ChangedItems []string `json:"changed_items"`
		}{
			CorrectionID: correctionID,
			ChangedItems: byStudent[student],
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.journal_event (kind, payload)
			VALUES ('recheck_completed', $1::jsonb)
		`, string(body)); err != nil {
			return err
		}
	}

	tutors := map[string]struct{}{}
	for _, student := range students {
		var owner *string
		err := tx.QueryRow(ctx, `
			SELECT owner_tutor_id FROM core.app_user WHERE id = $1
		`, student).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if owner == nil || *owner == "" {
			continue
		}
		tutors[*owner] = struct{}{}
	}
	tutorIDs := make([]string, 0, len(tutors))
	for id := range tutors {
		tutorIDs = append(tutorIDs, id)
	}
	slices.Sort(tutorIDs)
	for _, tutorID := range tutorIDs {
		if err := enqueueRecheck(ctx, tx, correctionID, tutorID); err != nil {
			return err
		}
	}
	return nil
}

func refreshObservation(ctx context.Context, tx pgx.Tx, assignmentID, itemID string) error {
	rows, err := tx.Query(ctx, `
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
		return err
	}
	defer rows.Close()
	var attempts []scoredAttempt
	for rows.Next() {
		var row scoredAttempt
		if err := rows.Scan(&row.no, &row.verdict, &row.points); err != nil {
			return err
		}
		attempts = append(attempts, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if len(attempts) == 0 {
		return nil
	}
	first, total := itemOutcome(attempts)

	var revision int
	var prevFirst, prevTotal int
	err = tx.QueryRow(ctx, `
		SELECT revision, first_attempt_points, total_points
		FROM core.observation
		WHERE assignment_id = $1 AND item_id = $2
		ORDER BY revision DESC
		LIMIT 1
	`, assignmentID, itemID).Scan(&revision, &prevFirst, &prevTotal)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && prevFirst == first && prevTotal == total {
		return nil
	}
	next := revision + 1
	if errors.Is(err, pgx.ErrNoRows) {
		next = 1
	}
	var maxPoints int
	if err := tx.QueryRow(ctx, `
		SELECT max_points FROM core.assignment_item
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&maxPoints); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.observation (
			assignment_id, item_id, revision, first_attempt_points, total_points, max_points
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, assignmentID, itemID, next, first, total, maxPoints); err != nil {
		return err
	}
	return insertObservationEvent(ctx, tx, assignmentID, itemID, next, first, total, maxPoints)
}

type scoredAttempt struct {
	no      int
	verdict string
	points  int
}

// itemOutcome повторяет AT-R11: итог — баллы первой верной попытки,
// иначе баллы лучшей. Балл первой попытки берётся с попытки 1.
func itemOutcome(attempts []scoredAttempt) (int, int) {
	first := attempts[0].points
	for _, attempt := range attempts {
		if attempt.verdict == "correct" {
			return first, attempt.points
		}
	}
	best := attempts[0].points
	for _, attempt := range attempts[1:] {
		if attempt.points > best {
			best = attempt.points
		}
	}
	return first, best
}

func insertObservationEvent(ctx context.Context, tx pgx.Tx, assignmentID, itemID string, revision, first, total, maxPoints int) error {
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT payload
		FROM core.journal_event
		WHERE kind = 'attempt_recorded'
		  AND payload->>'assignment_id' = $1
		  AND payload->>'item_id' = $2
		ORDER BY journal_seq
		LIMIT 1
	`, assignmentID, itemID).Scan(&payload)
	if err != nil {
		return err
	}
	var recorded struct {
		VariantID      string `json:"variant_id"`
		TypeID         string `json:"type_id"`
		SubvariantID   string `json:"subvariant_id"`
		Source         string `json:"source"`
		Place          string `json:"place"`
		CatalogVersion int    `json:"catalog_version"`
		RulesVersion   string `json:"rules_version"`
	}
	if err := json.Unmarshal(payload, &recorded); err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		ObservationID      string   `json:"observation_id"`
		ItemID             string   `json:"item_id"`
		Revision           int      `json:"revision"`
		TypeID             string   `json:"type_id"`
		SubvariantID       string   `json:"subvariant_id"`
		VariantID          string   `json:"variant_id"`
		Source             string   `json:"source"`
		Place              string   `json:"place"`
		FirstAttemptPoints int      `json:"first_attempt_points"`
		TotalPoints        int      `json:"total_points"`
		MaxPoints          int      `json:"max_points"`
		TrapIDs            []string `json:"trap_ids"`
		Skipped            bool     `json:"skipped"`
		Late               bool     `json:"late"`
		CatalogVersion     int      `json:"catalog_version"`
		RulesVersion       string   `json:"rules_version"`
	}{
		ObservationID:      fmt.Sprintf("%s:%s:%d", assignmentID, itemID, revision),
		ItemID:             itemID,
		Revision:           revision,
		TypeID:             recorded.TypeID,
		SubvariantID:       recorded.SubvariantID,
		VariantID:          recorded.VariantID,
		Source:             recorded.Source,
		Place:              recorded.Place,
		FirstAttemptPoints: first,
		TotalPoints:        total,
		MaxPoints:          maxPoints,
		TrapIDs:            []string{},
		CatalogVersion:     recorded.CatalogVersion,
		RulesVersion:       recorded.RulesVersion,
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ('observation', $1::jsonb)
	`, string(body))
	return err
}

func enqueueRecheck(ctx context.Context, tx pgx.Tx, correctionID int, tutorID string) error {
	body, err := json.Marshal(struct {
		Report string `json:"report"`
	}{
		Report: fmt.Sprintf("/rechecks/%d", correctionID),
	})
	if err != nil {
		return err
	}
	id := fmt.Sprintf("recheck-%d-%s", correctionID, tutorID)
	_, err = tx.Exec(ctx, `
		INSERT INTO core.outbox (
			id, kind, payload, recipient_kind, recipient_id, template, params, dedup_key
		) VALUES ($1, 'recheck_changes', $2::jsonb, 'tutor', $3, 'recheck_changes', $2::jsonb, $1)
	`, id, string(body), tutorID)
	return err
}
