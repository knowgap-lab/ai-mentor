package answercheck

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/answerrules"
)

const rateLimit = 60

type answerRequest struct {
	Answer         json.RawMessage `json:"answer"`
	IdempotencyKey string          `json:"idempotency_key"`
}

type attemptEvent struct {
	AttemptID       string `json:"attempt_id"`
	AssignmentID    string `json:"assignment_id"`
	ItemID          string `json:"item_id"`
	VariantID       string `json:"variant_id"`
	CatalogVersion  int    `json:"catalog_version"`
	RawInput        string `json:"raw_input"`
	NormalizedInput string `json:"normalized_input"`
	AttemptNo       int    `json:"attempt_no"`
	Verdict         string `json:"verdict"`
	Points          int    `json:"points"`
	TrapID          string `json:"trap_id,omitempty"`
	Source          string `json:"source"`
	Place           string `json:"place"`
	RulesVersion    string `json:"rules_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	Late            bool   `json:"late"`
}

type remembered struct {
	Hash string          `json:"hash"`
	Body json.RawMessage `json:"body"`
}

type assignmentRow struct {
	studentID string
	state     string
	kind      string
	version   int
	place     string
	deadline  *time.Time
	snapshot  []byte
}

type task struct {
	part          string
	format        string
	key           json.RawMessage
	solutionBlock string
	variantID     string
	rulesVersion  string
	traps         map[string]string
}

func (s *Service) answer(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body answerRequest
	if err := decodeJSON(r, &body); err != nil || body.IdempotencyKey == "" || len(body.Answer) == 0 || string(body.Answer) == "null" {
		writeCode(w, http.StatusBadRequest, "invalid")
		return
	}
	raw, err := s.answerTx(r.Context(), user, r.PathValue("id"), r.PathValue("item"), body)
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeBytes(w, http.StatusOK, raw)
}

func (s *Service) answerTx(ctx context.Context, user actor, assignmentID, itemID string, body answerRequest) ([]byte, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	head, err := lockAssignment(ctx, tx, assignmentID)
	if err != nil {
		return nil, err
	}
	if user.role != "student" || user.id != head.studentID {
		return nil, fail(http.StatusForbidden, "forbidden")
	}
	hash, err := answerHash(body.Answer)
	if err != nil {
		return nil, fail(http.StatusBadRequest, "invalid")
	}
	stored, found, err := loadIdempotency(ctx, tx, user.id, scopeKey(assignmentID, itemID, body.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	if found {
		if stored.Hash != hash {
			return nil, fail(http.StatusConflict, "idempotency_conflict")
		}
		return stored.Body, nil
	}
	if err := enforceRate(ctx, tx, user.id); err != nil {
		return nil, err
	}
	if head.state != "issued" {
		return nil, fail(http.StatusConflict, "assignment_closed")
	}

	var itemState string
	err = tx.QueryRow(ctx, `
		SELECT state
		FROM core.assignment_item
		WHERE assignment_id = $1 AND item_id = $2
		FOR UPDATE
	`, assignmentID, itemID).Scan(&itemState)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fail(http.StatusForbidden, "forbidden")
	}
	if err != nil {
		return nil, err
	}
	if itemState == "final" {
		return nil, fail(http.StatusConflict, "item_closed")
	}

	task, err := taskFrom(head.snapshot, itemID)
	if err != nil {
		return nil, err
	}
	scored, rawInput, err := evaluate(task, body.Answer)
	if err != nil {
		return nil, err
	}
	used, err := attemptCount(ctx, tx, assignmentID, itemID)
	if err != nil {
		return nil, err
	}
	limit := attemptLimit(head.kind, task.part)
	response, final := buildResponse(head.kind, task, scored, used, limit)
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	if scored.Status == "scored" {
		if err := recordAttempt(ctx, tx, head, assignmentID, itemID, task, body.IdempotencyKey, rawInput, scored, final); err != nil {
			return nil, err
		}
	}
	if err := saveIdempotency(ctx, tx, user.id, scopeKey(assignmentID, itemID, body.IdempotencyKey), hash, raw); err != nil {
		return nil, err
	}
	if err := insertRateHit(ctx, tx, user.id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return raw, nil
}

func lockAssignment(ctx context.Context, tx pgx.Tx, id string) (assignmentRow, error) {
	var head assignmentRow
	err := tx.QueryRow(ctx, `
		SELECT student_id, state, kind, catalog_version, place, deadline, snapshot
		FROM core.assignment
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&head.studentID, &head.state, &head.kind, &head.version, &head.place, &head.deadline, &head.snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return assignmentRow{}, fail(http.StatusForbidden, "forbidden")
	}
	if err != nil {
		return assignmentRow{}, err
	}
	return head, nil
}

func enforceRate(ctx context.Context, tx pgx.Tx, userID string) error {
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('answercheck'), hashtext($1::text))`,
		userID,
	); err != nil {
		return err
	}
	var n int
	var oldest *time.Time
	err := tx.QueryRow(ctx, `
		SELECT count(*)::int, min(at)
		FROM core.answer_rate
		WHERE student_id = $1
		  AND at > now() - interval '1 minute'
	`, userID).Scan(&n, &oldest)
	if err != nil {
		return err
	}
	if n < rateLimit {
		return nil
	}
	retry := 1
	if oldest != nil {
		wait := time.Until(oldest.Add(time.Minute))
		if wait > 0 {
			retry = int(wait/time.Second) + 1
		}
	}
	return &httpError{status: http.StatusTooManyRequests, code: "rate_limited", retry: retry}
}

func loadIdempotency(ctx context.Context, tx pgx.Tx, userID, key string) (remembered, bool, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT response FROM core.idempotency
		WHERE user_id = $1 AND idem_key = $2
	`, userID, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return remembered{}, false, nil
	}
	if err != nil {
		return remembered{}, false, err
	}
	var stored remembered
	if err := json.Unmarshal(raw, &stored); err != nil {
		return remembered{}, false, err
	}
	return stored, true, nil
}

func saveIdempotency(ctx context.Context, tx pgx.Tx, userID, key, hash string, body []byte) error {
	raw, err := json.Marshal(remembered{Hash: hash, Body: body})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.idempotency (user_id, idem_key, response)
		VALUES ($1, $2, $3)
	`, userID, key, raw)
	return err
}

func insertRateHit(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO core.answer_rate (student_id)
		VALUES ($1)
	`, userID)
	return err
}

func taskFrom(snapshot []byte, itemID string) (task, error) {
	var snap storedSnap
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return task{}, err
	}
	for _, item := range snap.Items {
		if item.ItemID != itemID {
			continue
		}
		format := itemFormat(item)
		rules := item.RulesVersion
		if rules == "" {
			rules = rulesVersion
		}
		return task{
			part:          itemPart(item, format),
			format:        format,
			key:           item.Key,
			solutionBlock: item.SolutionBlock,
			variantID:     item.VariantID,
			rulesVersion:  rules,
			traps:         item.Traps,
		}, nil
	}
	return task{}, fail(http.StatusForbidden, "forbidden")
}

func itemFormat(item storedItem) string {
	if item.AnswerFormat != "" {
		return item.AnswerFormat
	}
	return item.Format
}

func itemPart(item storedItem, format string) string {
	if item.Part != "" {
		return item.Part
	}
	switch format {
	case "ct.choice1", "ct.choiceN":
		return "A"
	case "ct.integer", "ct.digits", "ct.match":
		return "B"
	default:
		return ""
	}
}

func evaluate(task task, answer json.RawMessage) (answerrules.Result, string, error) {
	key, err := decodeKey(task.format, task.key)
	if err != nil {
		return answerrules.Result{}, "", err
	}
	raw, rawInput, ok := decodeAnswer(task.format, answer)
	if !ok {
		return answerrules.Result{Status: "malformed"}, rawInput, nil
	}
	return answerrules.Score(task.format, key, raw, task.traps), rawInput, nil
}

func decodeKey(format string, raw json.RawMessage) (any, error) {
	switch format {
	case "ct.choice1", "ct.choiceN":
		var nums []int
		if json.Unmarshal(raw, &nums) != nil {
			return nil, errors.New("choice key")
		}
		return nums, nil
	case "ct.integer":
		value, ok := decodeScalar(raw)
		if !ok {
			return nil, errors.New("integer key")
		}
		return value, nil
	case "ct.digits", "ct.match":
		text, ok := decodeString(raw)
		if !ok {
			return nil, errors.New("text key")
		}
		return text, nil
	default:
		return nil, errors.New("unknown format")
	}
}

func decodeAnswer(format string, raw json.RawMessage) (any, string, bool) {
	switch format {
	case "ct.choice1", "ct.choiceN":
		var nums []int
		if json.Unmarshal(raw, &nums) != nil {
			return nil, compactRaw(raw), false
		}
		return nums, compactRaw(raw), true
	case "ct.integer", "ct.digits", "ct.match":
		text, ok := decodeString(raw)
		if !ok {
			return nil, compactRaw(raw), false
		}
		return text, text, true
	default:
		return nil, compactRaw(raw), false
	}
}

func decodeScalar(raw json.RawMessage) (any, bool) {
	var n int
	if json.Unmarshal(raw, &n) == nil && looksInt(raw) {
		return n, true
	}
	text, ok := decodeString(raw)
	if !ok {
		return nil, false
	}
	return text, true
}

func looksInt(raw json.RawMessage) bool {
	if len(raw) == 0 || raw[0] == '"' {
		return false
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&n) != nil {
		return false
	}
	_, err := n.Int64()
	return err == nil
}

func decodeString(raw json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return "", false
	}
	return text, true
}

func buildResponse(kind string, task task, result answerrules.Result, used, limit int) (map[string]any, bool) {
	if result.Status != "scored" {
		left := limit - used
		if left < 0 {
			left = 0
		}
		return map[string]any{
			"status":        "malformed",
			"hint":          integerHint,
			"item_state":    "open",
			"attempts_left": left,
		}, false
	}
	attemptNo := used + 1
	final := result.Verdict == "correct" || attemptNo >= limit
	left := limit - attemptNo
	if final || left < 0 {
		left = 0
	}
	state := "open"
	if final {
		state = "final"
	}
	if !revealScores(kind) {
		return map[string]any{
			"status":        "accepted",
			"item_state":    state,
			"attempts_left": left,
		}, final
	}
	body := map[string]any{
		"status":        "scored",
		"item_state":    state,
		"attempts_left": left,
		"verdict":       result.Verdict,
		"points":        result.Points,
		"max_points":    maxFor(task.format),
	}
	if showSolution(result.Verdict, final) {
		show := map[string]any{
			"key": formatShowKey(task.format, task.key),
		}
		if task.solutionBlock != "" {
			show["solution_block"] = task.solutionBlock
		}
		body["show"] = show
	}
	return body, final
}

func revealScores(kind string) bool {
	switch kind {
	case "checkpoint", "mock", "diagnostics":
		return false
	default:
		return true
	}
}

func showSolution(verdict string, final bool) bool {
	return verdict != "correct" && final
}

func formatShowKey(format string, raw json.RawMessage) string {
	switch format {
	case "ct.choice1", "ct.choiceN":
		var nums []int
		if json.Unmarshal(raw, &nums) != nil {
			return ""
		}
		parts := make([]string, len(nums))
		for i, n := range nums {
			parts[i] = strconv.Itoa(n)
		}
		out := ""
		for i, part := range parts {
			if i > 0 {
				out += ","
			}
			out += part
		}
		return out
	case "ct.integer":
		if n, ok := decodeScalar(raw); ok {
			switch value := n.(type) {
			case int:
				return strconv.Itoa(value)
			case string:
				return value
			}
		}
	default:
		if text, ok := decodeString(raw); ok {
			return text
		}
	}
	return ""
}

func attemptLimit(kind, part string) int {
	if kind == "homework" && part == "B" {
		return 2
	}
	return 1
}

func attemptCount(ctx context.Context, tx pgx.Tx, assignmentID, itemID string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*)::int FROM core.attempts
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&n)
	return n, err
}

func recordAttempt(ctx context.Context, tx pgx.Tx, head assignmentRow, assignmentID, itemID string, task task, idemKey, rawInput string, result answerrules.Result, final bool) error {
	var next int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_no), 0) + 1
		FROM core.attempts
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&next); err != nil {
		return err
	}
	attemptID, err := newID()
	if err != nil {
		return err
	}
	source, place := sourcePlace(head.kind)
	if head.place != "" {
		place = head.place
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.attempts (
			id, assignment_id, item_id, raw_input, source, place, attempt_no
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, attemptID, assignmentID, itemID, rawInput, source, place, next); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		VALUES ($1, 1, $2, $3)
	`, attemptID, result.Verdict, result.Points); err != nil {
		return err
	}
	late := head.deadline != nil && head.deadline.Before(time.Now())
	event, err := json.Marshal(attemptEvent{
		AttemptID:       attemptID,
		AssignmentID:    assignmentID,
		ItemID:          itemID,
		VariantID:       task.variantID,
		CatalogVersion:  head.version,
		RawInput:        rawInput,
		NormalizedInput: result.Normalized,
		AttemptNo:       next,
		Verdict:         result.Verdict,
		Points:          result.Points,
		TrapID:          result.TrapID,
		Source:          source,
		Place:           place,
		RulesVersion:    task.rulesVersion,
		IdempotencyKey:  idemKey,
		Late:            late,
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ('attempt_recorded', $1::jsonb)
	`, string(event)); err != nil {
		return err
	}
	if !final {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment_item
		SET state = 'final'
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID); err != nil {
		return err
	}
	return insertObservation(ctx, tx, assignmentID, itemID, maxFor(task.format))
}

func insertObservation(ctx context.Context, tx pgx.Tx, assignmentID, itemID string, maxPoints int) error {
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
	type scoredRow struct {
		no      int
		verdict string
		points  int
	}
	var attempts []scoredRow
	for rows.Next() {
		var row scoredRow
		if err := rows.Scan(&row.no, &row.verdict, &row.points); err != nil {
			return err
		}
		attempts = append(attempts, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(attempts) == 0 {
		return errors.New("observation without attempts")
	}
	first := attempts[0].points
	total := attempts[0].points
	for _, row := range attempts {
		if row.verdict == "correct" {
			total = row.points
			break
		}
		if row.points > total {
			total = row.points
		}
	}
	_, err = tx.Exec(ctx, `
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
	`, assignmentID, itemID, first, total, maxPoints)
	return err
}

func answerHash(raw json.RawMessage) (string, error) {
	sum := sha256.Sum256([]byte(compactRaw(raw)))
	return hex.EncodeToString(sum[:]), nil
}

func compactRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

func scopeKey(assignmentID, itemID, key string) string {
	return "answer\x1f" + assignmentID + "\x1f" + itemID + "\x1f" + key
}

func newID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "at" + hex.EncodeToString(buf), nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
