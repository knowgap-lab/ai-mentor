// Пакет recheck принимает пакет перепроверки и заново оценивает попытки.
// Баллы считает answerrules.Score: здесь меняется ключ версии correction_id,
// а не правила, записанные в попытке.
package recheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/answerrules"
)

const (
	codeStale     = "stale_correction"
	codeUnknown   = "variant_unknown"
	formatInteger = "ct.integer"
	portionSize   = 500
	acceptLock    = int64(7001)
)

// State — состояние пакета перепроверки (RC-R6).
type State struct {
	CorrectionID  int    `json:"correction_id"`
	State         string `json:"state"`
	Total         int    `json:"total"`
	Processed     int    `json:"processed"`
	Changed       int    `json:"changed"`
	SkippedManual int    `json:"skipped_manual"`
	Cursor        string `json:"cursor"`
}

// Error — отказ приёма. Error возвращает код.
type Error struct {
	Code string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

type batch struct {
	state    State
	variants []string
}

// Accept принимает пакет и коммитит его до возврата (RC-R2…RC-R5).
// Первый вызов — pending. Повтор с тем же correction_id и тем же списком
// возвращает уже записанное состояние и вторую строку не создаёт.
func Accept(ctx context.Context, conn *pgx.Conn, correctionID int, variants []string) (State, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, acceptLock); err != nil {
		return State{}, err
	}

	list := canonical(variants)
	existing, ok, err := loadBatch(ctx, tx, correctionID, false)
	if err != nil {
		return State{}, err
	}
	if ok {
		if !sameList(existing.variants, list) {
			return State{}, fmt.Errorf("recheck: variant list mismatch")
		}
		if err := tx.Commit(ctx); err != nil {
			return State{}, err
		}
		return existing.state, nil
	}

	if err := validateVariants(ctx, tx, correctionID, list); err != nil {
		return State{}, err
	}

	var maxID int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(correction_id), 0)
		FROM core.recheck_batch
	`).Scan(&maxID); err != nil {
		return State{}, err
	}
	if correctionID < maxID {
		return State{}, &Error{Code: codeStale}
	}

	total, err := countAttempts(ctx, tx, list)
	if err != nil {
		return State{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.recheck_batch (
			correction_id, state, total, processed, changed, skipped_manual, variants
		)
		VALUES ($1, 'pending', $2, 0, 0, 0, $3)
	`, correctionID, total, list); err != nil {
		return State{}, err
	}

	stored, ok, err := loadBatch(ctx, tx, correctionID, false)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, fmt.Errorf("recheck: batch %d was not stored", correctionID)
	}
	if err := tx.Commit(ctx); err != nil {
		return State{}, err
	}
	return stored.state, nil
}

// Run пересчитывает попытки вариантов пакета и коммитит итог до возврата.
// Ручной текущий вердикт не оценивается заново (RC-R13).
func Run(ctx context.Context, conn *pgx.Conn, correctionID int) (State, error) {
	for {
		state, err := runPortion(ctx, conn, correctionID)
		if err != nil {
			return State{}, err
		}
		if state.State == "done" || state.State == "failed" || state.State == "pending" {
			return state, nil
		}
	}
}

func runPortion(ctx context.Context, conn *pgx.Conn, correctionID int) (State, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stored, ok, err := loadBatch(ctx, tx, correctionID, true)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, fmt.Errorf("recheck: batch %d not found", correctionID)
	}
	if stored.state.State == "done" || stored.state.State == "failed" {
		if err := tx.Commit(ctx); err != nil {
			return State{}, err
		}
		return stored.state, nil
	}
	earlier, err := earlierOpen(ctx, tx, correctionID)
	if err != nil {
		return State{}, err
	}
	if earlier {
		if err := tx.Commit(ctx); err != nil {
			return State{}, err
		}
		return stored.state, nil
	}

	ids, err := nextAttempts(ctx, tx, stored.variants, stored.state.Cursor)
	if err != nil {
		return State{}, err
	}
	keys := map[string]scoreKey{}
	for _, attemptID := range ids {
		if err := rescoreAttempt(ctx, tx, correctionID, attemptID, keys, &stored.state); err != nil {
			return State{}, err
		}
	}

	done := len(ids) < portionSize
	if err := saveBatch(ctx, tx, stored.state, done); err != nil {
		return State{}, err
	}
	if done {
		if err := finishBatch(ctx, tx, correctionID); err != nil {
			return State{}, err
		}
	}
	stored, ok, err = loadBatch(ctx, tx, correctionID, false)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, fmt.Errorf("recheck: batch %d disappeared", correctionID)
	}
	if err := tx.Commit(ctx); err != nil {
		return State{}, err
	}
	return stored.state, nil
}

func rescoreAttempt(ctx context.Context, tx pgx.Tx, correctionID int, attemptID string, keys map[string]scoreKey, state *State) error {
	current, err := loadCurrent(ctx, tx, attemptID)
	if err != nil {
		return err
	}
	manual, err := manualCurrent(ctx, tx, attemptID)
	if err != nil {
		return err
	}
	state.Cursor = attemptID
	state.Processed++
	if manual {
		state.SkippedManual++
		return nil
	}

	row, err := loadAttempt(ctx, tx, attemptID)
	if err != nil {
		return err
	}
	key, ok := keys[row.variantID]
	if !ok {
		key, err = loadKey(ctx, tx, row.variantID, correctionID)
		if err != nil {
			return err
		}
		keys[row.variantID] = key
	}
	scored := answerrules.Score(key.format, key.value, row.raw, nil)
	if scored.Status != "scored" {
		return fmt.Errorf("recheck: attempt %s not scored", attemptID)
	}
	if scored.Verdict == current.verdict && scored.Points == current.points {
		return nil
	}
	if err := insertVerdict(ctx, tx, attemptID, scored); err != nil {
		return err
	}
	if err := insertRecheckEvent(ctx, tx, attemptID, correctionID, row.maxPoints, current, scored); err != nil {
		return err
	}
	state.Changed++
	return nil
}

type scoreKey struct {
	format string
	value  any
}

type attemptRow struct {
	raw       string
	variantID string
	maxPoints int
}

type verdictNow struct {
	verdict string
	points  int
	reason  string
}

func loadAttempt(ctx context.Context, tx pgx.Tx, attemptID string) (attemptRow, error) {
	var row attemptRow
	err := tx.QueryRow(ctx, `
		SELECT a.raw_input, e.payload->>'variant_id', i.max_points
		FROM core.attempts AS a
		JOIN core.assignment_item AS i
		  ON i.assignment_id = a.assignment_id
		 AND i.item_id = a.item_id
		JOIN LATERAL (
			SELECT payload
			FROM core.journal_event
			WHERE kind = 'attempt_recorded'
			  AND payload->>'attempt_id' = a.id
			ORDER BY journal_seq
			LIMIT 1
		) AS e ON true
		WHERE a.id = $1
	`, attemptID).Scan(&row.raw, &row.variantID, &row.maxPoints)
	if err != nil {
		return attemptRow{}, err
	}
	return row, nil
}

func loadCurrent(ctx context.Context, tx pgx.Tx, attemptID string) (verdictNow, error) {
	var current verdictNow
	err := tx.QueryRow(ctx, `
		SELECT verdict, points, COALESCE(reason, '')
		FROM core.attempt_verdict
		WHERE attempt_id = $1
		ORDER BY seq DESC
		LIMIT 1
	`, attemptID).Scan(&current.verdict, &current.points, &current.reason)
	if err != nil {
		return verdictNow{}, err
	}
	return current, nil
}

func manualCurrent(ctx context.Context, tx pgx.Tx, attemptID string) (bool, error) {
	var manual bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM core.journal_event
			WHERE kind = 'manual_verdict'
			  AND payload->>'attempt_id' = $1
		)
		OR EXISTS (
			SELECT 1
			FROM core.journal_event
			WHERE kind = 'attempt_verdict'
			  AND payload->>'attempt_id' = $1
			  AND payload->>'origin' = 'manual'
		)
	`, attemptID).Scan(&manual)
	return manual, err
}

func loadKey(ctx context.Context, tx pgx.Tx, variantID string, correctionID int) (scoreKey, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT data
		FROM catalog.revision
		WHERE entity_id = $1
		  AND catalog_version_from <= $2
		ORDER BY catalog_version_from DESC
		LIMIT 1
	`, variantID, correctionID).Scan(&raw)
	if err != nil {
		return scoreKey{}, err
	}
	var data struct {
		AnswerFormat string          `json:"answer_format"`
		Key          json.RawMessage `json:"key"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return scoreKey{}, err
	}
	if data.AnswerFormat != formatInteger {
		return scoreKey{}, fmt.Errorf("recheck: format %s", data.AnswerFormat)
	}
	value, err := integerKey(data.Key)
	if err != nil {
		return scoreKey{}, err
	}
	return scoreKey{format: data.AnswerFormat, value: value}, nil
}

func integerKey(raw json.RawMessage) (any, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, err
	}
	return text, nil
}

func insertVerdict(ctx context.Context, tx pgx.Tx, attemptID string, scored answerrules.Result) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		VALUES (
			$1,
			(SELECT COALESCE(MAX(seq), 0) + 1
			 FROM core.attempt_verdict
			 WHERE attempt_id = $1),
			$2, $3
		)
	`, attemptID, scored.Verdict, scored.Points)
	return err
}

func insertRecheckEvent(ctx context.Context, tx pgx.Tx, attemptID string, correctionID, maxPoints int, prev verdictNow, scored answerrules.Result) error {
	body, err := json.Marshal(struct {
		Origin          string `json:"origin"`
		AttemptID       string `json:"attempt_id"`
		Verdict         string `json:"verdict"`
		Points          int    `json:"points"`
		MaxPoints       int    `json:"max_points"`
		CorrectionID    int    `json:"correction_id"`
		PreviousVerdict string `json:"previous_verdict"`
		PreviousPoints  int    `json:"previous_points"`
	}{
		Origin:          "recheck",
		AttemptID:       attemptID,
		Verdict:         scored.Verdict,
		Points:          scored.Points,
		MaxPoints:       maxPoints,
		CorrectionID:    correctionID,
		PreviousVerdict: prev.verdict,
		PreviousPoints:  prev.points,
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ('attempt_verdict', $1::jsonb)
	`, string(body))
	return err
}

func validateVariants(ctx context.Context, tx pgx.Tx, correctionID int, variants []string) error {
	for _, id := range variants {
		if id == "" {
			return &Error{Code: codeUnknown}
		}
		var raw []byte
		err := tx.QueryRow(ctx, `
			SELECT data
			FROM catalog.revision
			WHERE entity_id = $1
			  AND catalog_version_from <= $2
			ORDER BY catalog_version_from DESC
			LIMIT 1
		`, id, correctionID).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return &Error{Code: codeUnknown}
		}
		if err != nil {
			return err
		}
		if !activeVariant(raw) {
			return &Error{Code: codeUnknown}
		}
	}
	return nil
}

func activeVariant(raw []byte) bool {
	var data struct {
		Status string          `json:"status"`
		Key    json.RawMessage `json:"key"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	if len(data.Key) == 0 || string(data.Key) == "null" {
		return false
	}
	switch data.Status {
	case "", "active", "retired":
		return true
	default:
		return false
	}
}

func countAttempts(ctx context.Context, tx pgx.Tx, variants []string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT payload->>'attempt_id')
		FROM core.journal_event
		WHERE kind = 'attempt_recorded'
		  AND COALESCE(payload->>'attempt_id', '') <> ''
		  AND payload->>'variant_id' = ANY($1::text[])
	`, variants).Scan(&n)
	return n, err
}

func nextAttempts(ctx context.Context, tx pgx.Tx, variants []string, cursor string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT id
		FROM (
			SELECT DISTINCT payload->>'attempt_id' AS id
			FROM core.journal_event
			WHERE kind = 'attempt_recorded'
			  AND payload->>'variant_id' = ANY($1::text[])
		) AS attempts
		WHERE id <> ''
		  AND id > $2
		ORDER BY id
		LIMIT $3
	`, variants, cursor, portionSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func loadBatch(ctx context.Context, tx pgx.Tx, correctionID int, lock bool) (batch, bool, error) {
	query := `
		SELECT correction_id, state, total, processed, changed, skipped_manual,
		       cursor, variants
		FROM core.recheck_batch
		WHERE correction_id = $1
	`
	if lock {
		query += ` FOR UPDATE`
	}
	var (
		got    batch
		cursor *string
	)
	err := tx.QueryRow(ctx, query, correctionID).Scan(
		&got.state.CorrectionID,
		&got.state.State,
		&got.state.Total,
		&got.state.Processed,
		&got.state.Changed,
		&got.state.SkippedManual,
		&cursor,
		&got.variants,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return batch{}, false, nil
	}
	if err != nil {
		return batch{}, false, err
	}
	if cursor != nil {
		got.state.Cursor = *cursor
	}
	if got.variants == nil {
		got.variants = []string{}
	}
	return got, true, nil
}

func saveBatch(ctx context.Context, tx pgx.Tx, state State, done bool) error {
	next := "running"
	if done {
		next = "done"
	}
	_, err := tx.Exec(ctx, `
		UPDATE core.recheck_batch
		SET state = $2,
		    processed = $3,
		    changed = $4,
		    skipped_manual = $5,
		    cursor = NULLIF($6, ''),
		    started_at = COALESCE(started_at, now()),
		    finished_at = CASE
		        WHEN $2 = 'done' THEN COALESCE(finished_at, now())
		        ELSE finished_at
		    END
		WHERE correction_id = $1
	`, state.CorrectionID, next, state.Processed, state.Changed, state.SkippedManual, state.Cursor)
	return err
}

func canonical(variants []string) []string {
	out := append([]string{}, variants...)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func sameList(a, b []string) bool {
	return slices.Equal(a, b)
}
