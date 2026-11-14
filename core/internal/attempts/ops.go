package attempts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// RecordAttempt пишет одну попытку. Источник и место берутся из вида выдачи (AT-R8).
func RecordAttempt(ctx context.Context, conn *pgx.Conn, assignmentID string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM core.assignment WHERE id = $1`, assignmentID).Scan(&kind); err != nil {
		return err
	}
	var itemID string
	if err := tx.QueryRow(ctx, `
		SELECT item_id FROM core.assignment_item
		WHERE assignment_id = $1
		ORDER BY item_id
		LIMIT 1
	`, assignmentID).Scan(&itemID); err != nil {
		return err
	}
	var attemptNo int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_no), 0) + 1
		FROM core.attempts
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&attemptNo); err != nil {
		return err
	}
	source, place := sourcePlace(kind)
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.attempts (id, assignment_id, item_id, raw_input, source, place, attempt_no)
		VALUES ($1, $2, $3, '1', $4, $5, $6)
	`, "recorded-"+assignmentID, assignmentID, itemID, source, place, attemptNo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveDisputeByCorrection закрывает открытый спор ссылкой на исправление
// и добавляет одну запись attempt_verdict. HTTP перепроверки нет.
func ResolveDisputeByCorrection(ctx context.Context, conn *pgx.Conn, correctionID int) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var attemptID string
	err = tx.QueryRow(ctx, `
		SELECT a.id
		FROM core.dispute AS d
		JOIN core.attempts AS a
		  ON a.assignment_id = d.assignment_id AND a.item_id = d.item_id
		WHERE d.state = 'open'
		ORDER BY a.attempt_no
		LIMIT 1
	`).Scan(&attemptID)
	if err != nil {
		return err
	}
	if err := insertVerdict(ctx, tx, attemptID, "correct", pointsFor("correct"), nil); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE core.dispute
		SET state = 'resolved',
		    resolved_by = jsonb_build_object('correction_id', $1::int)
		WHERE state = 'open'
	`, correctionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("resolved disputes %d", tag.RowsAffected())
	}
	return tx.Commit(ctx)
}

// Anonymise удаляет ПДн и псевдоним ученика. Строки журнала не меняет (AT-R25).
func Anonymise(ctx context.Context, conn *pgx.Conn, studentID string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM core.student_pii WHERE student_id = $1`, studentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.student_pseudonym WHERE student_id = $1`, studentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}

type submitResponse struct {
	AssignmentID string       `json:"assignment_id"`
	Answered     int          `json:"answered"`
	Skipped      int          `json:"skipped"`
	Items        []submitItem `json:"items"`
	TotalPoints  int          `json:"total_points"`
	MaxPoints    int          `json:"max_points"`
}

type submitItem struct {
	ItemID    string `json:"item_id"`
	Verdict   string `json:"verdict,omitempty"`
	Points    int    `json:"points,omitempty"`
	MaxPoints int    `json:"max_points,omitempty"`
}

func (s *Service) submitAssignment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	var body struct {
		ConfirmUnanswered bool `json:"confirm_unanswered"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	status, payload, err := s.doSubmit(r.Context(), user, r.PathValue("id"), key, body.ConfirmUnanswered)
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeResult(w, status, payload)
}

func (s *Service) doSubmit(ctx context.Context, user actor, assignmentID, key string, confirm bool) (int, []byte, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	key = assignmentID + ":" + key
	var stored []byte
	err = tx.QueryRow(ctx, `
		SELECT response FROM core.idempotency WHERE user_id = $1 AND idem_key = $2
	`, user.id, key).Scan(&stored)
	if err == nil {
		return http.StatusOK, stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, err
	}

	if _, err := tx.Exec(ctx, `SELECT id FROM core.assignment WHERE id = $1 FOR UPDATE`, assignmentID); err != nil {
		return 0, nil, err
	}
	head, err := loadAssignment(ctx, tx, assignmentID)
	if err != nil {
		return 0, nil, err
	}
	if user.role != "student" || user.id != head.studentID {
		return 0, nil, fail(http.StatusForbidden, "forbidden", nil)
	}
	if head.state != "issued" {
		return 0, nil, fail(http.StatusConflict, "assignment_closed", nil)
	}
	items, err := loadItems(ctx, tx, assignmentID)
	if err != nil {
		return 0, nil, err
	}
	unanswered := 0
	answered := 0
	total := 0
	maxPoints := 0
	for _, item := range items {
		maxPoints += item.max
		if len(item.attempts) == 0 {
			unanswered++
			continue
		}
		answered++
		if result, ok := summarize(item.attempts); ok {
			total += result.TotalPoints
		}
	}
	if unanswered > 0 && !confirm {
		return 0, nil, fail(http.StatusConflict, "unanswered_confirmation_required", map[string]any{"left": unanswered})
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment_item AS ai
		SET skipped = true, state = 'final'
		WHERE ai.assignment_id = $1
		  AND state = 'open'
		  AND NOT EXISTS (
			SELECT 1 FROM core.attempts AS a
			WHERE a.assignment_id = ai.assignment_id AND a.item_id = ai.item_id
		  )
	`, assignmentID); err != nil {
		return 0, nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment_item
		SET state = 'final'
		WHERE assignment_id = $1 AND state = 'open'
	`, assignmentID); err != nil {
		return 0, nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment SET state = 'submitted' WHERE id = $1
	`, assignmentID); err != nil {
		return 0, nil, err
	}
	event, err := json.Marshal(map[string]any{
		"assignment_id": assignmentID,
		"answered":      answered,
		"skipped":       unanswered,
	})
	if err != nil {
		return 0, nil, err
	}
	if err := insertEvent(ctx, tx, "assignment_submitted", event); err != nil {
		return 0, nil, err
	}
	for _, item := range items {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM core.observation
				WHERE assignment_id = $1 AND item_id = $2
			)
		`, assignmentID, item.id).Scan(&exists); err != nil {
			return 0, nil, err
		}
		if exists {
			continue
		}
		result := ItemResult{}
		if len(item.attempts) > 0 {
			var ok bool
			result, ok = summarize(item.attempts)
			if !ok {
				continue
			}
		}
		if err := insertObservation(ctx, tx, assignmentID, item.id, result, item.max); err != nil {
			return 0, nil, err
		}
	}
	outbox, err := json.Marshal(map[string]any{
		"template":      "homework_submitted",
		"assignment_id": assignmentID,
		"student_id":    head.studentID,
	})
	if err != nil {
		return 0, nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.outbox (id, kind, payload, template, state, next_attempt_at)
		VALUES ($1, 'homework_submitted', $2::jsonb, 'homework_submitted', 'pending', now())
	`, "homework-submitted-"+assignmentID, string(outbox)); err != nil {
		return 0, nil, err
	}

	response := submitResponse{
		AssignmentID: assignmentID,
		Answered:     answered,
		Skipped:      unanswered,
		Items:        make([]submitItem, 0),
		TotalPoints:  total,
		MaxPoints:    maxPoints,
	}
	if isTestKind(head.kind) {
		for _, item := range items {
			result, ok := summarize(item.attempts)
			if !ok {
				continue
			}
			response.Items = append(response.Items, submitItem{
				ItemID:    item.id,
				Verdict:   result.Verdict,
				Points:    result.TotalPoints,
				MaxPoints: item.max,
			})
		}
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return 0, nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.idempotency (user_id, idem_key, response)
		VALUES ($1, $2, $3)
	`, user.id, key, payload); err != nil {
		return 0, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, payload, nil
}

func (s *Service) withdrawAssignment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	err := s.doWithdraw(r.Context(), user, r.PathValue("id"), body.Reason)
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) doWithdraw(ctx context.Context, user actor, assignmentID, reason string) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM core.assignment WHERE id = $1 FOR UPDATE`, assignmentID); err != nil {
		return err
	}
	head, err := loadAssignment(ctx, tx, assignmentID)
	if err != nil {
		return err
	}
	if !canTutor(user, head) {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	var answers int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM core.attempts WHERE assignment_id = $1
	`, assignmentID).Scan(&answers); err != nil {
		return err
	}
	if answers > 0 {
		return fail(http.StatusConflict, "has_answers", nil)
	}
	if head.state != "issued" {
		return fail(http.StatusConflict, "assignment_closed", nil)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment SET state = 'withdrawn' WHERE id = $1
	`, assignmentID); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"assignment_id": assignmentID,
		"reason":        reason,
	})
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, "assignment_withdrawn", payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) extendDeadline(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Deadline string `json:"deadline"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	updated, err := s.doExtend(r.Context(), user, r.PathValue("id"), body.Deadline)
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deadline": updated})
}

func (s *Service) doExtend(ctx context.Context, user actor, assignmentID, raw string) (string, error) {
	next, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", fail(http.StatusUnprocessableEntity, "invalid", nil)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current *time.Time
	var head assignmentHead
	err = tx.QueryRow(ctx, `
		SELECT a.student_id, a.state, a.kind, COALESCE(u.owner_tutor_id, ''), a.deadline
		FROM core.assignment AS a
		JOIN core.app_user AS u ON u.id = a.student_id
		WHERE a.id = $1
		FOR UPDATE OF a
	`, assignmentID).Scan(&head.studentID, &head.state, &head.kind, &head.ownerTutor, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fail(http.StatusNotFound, "not_found", nil)
	}
	if err != nil {
		return "", err
	}
	if !canTutor(user, head) {
		return "", fail(http.StatusForbidden, "forbidden", nil)
	}
	if current != nil && !next.After(*current) {
		return "", fail(http.StatusConflict, "deadline_shorter", nil)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment SET deadline = $2 WHERE id = $1
	`, assignmentID, next); err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"assignment_id": assignmentID,
		"deadline":      next.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	if err := insertEvent(ctx, tx, "deadline_changed", payload); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return next.UTC().Format(time.RFC3339), nil
}

func (s *Service) didNotUnderstand(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if err := s.doDidNotUnderstand(r.Context(), user, r.PathValue("id"), r.PathValue("item")); err != nil {
		writeHandlerErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) doDidNotUnderstand(ctx context.Context, user actor, assignmentID, itemID string) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	head, err := loadAssignment(ctx, tx, assignmentID)
	if err != nil {
		return err
	}
	if user.role != "student" || user.id != head.studentID {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	if head.state != "submitted" {
		return fail(http.StatusConflict, "not_submitted", nil)
	}
	var itemState string
	err = tx.QueryRow(ctx, `
		SELECT state FROM core.assignment_item
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&itemState)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusNotFound, "not_found", nil)
	}
	if err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.journal_event
			WHERE kind = 'did_not_understand'
			  AND payload->>'assignment_id' = $1
			  AND payload->>'item_id' = $2
		)
	`, assignmentID, itemID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit(ctx)
	}
	payload, err := json.Marshal(map[string]string{
		"assignment_id": assignmentID,
		"item_id":       itemID,
	})
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, "did_not_understand", payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) openDispute(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string  `json:"reason"`
		Text   *string `json:"text"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	if err := s.doDispute(r.Context(), user, r.PathValue("id"), r.PathValue("item"), body.Reason, body.Text); err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": r.PathValue("item"), "state": "open"})
}

func (s *Service) doDispute(ctx context.Context, user actor, assignmentID, itemID, reason string, text *string) error {
	switch reason {
	case "key_wrong", "statement_error", "equivalent_answer", "other":
	default:
		return fail(http.StatusUnprocessableEntity, "dispute_text_invalid", nil)
	}
	if text != nil && disputeTextInvalid(*text) {
		return fail(http.StatusUnprocessableEntity, "dispute_text_invalid", nil)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	head, err := loadAssignment(ctx, tx, assignmentID)
	if err != nil {
		return err
	}
	if user.role != "student" || user.id != head.studentID {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	if head.state != "submitted" {
		return fail(http.StatusConflict, "not_submitted", nil)
	}
	var itemState string
	err = tx.QueryRow(ctx, `
		SELECT state FROM core.assignment_item
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&itemState)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusNotFound, "not_found", nil)
	}
	if err != nil {
		return err
	}
	var open int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM core.dispute
		WHERE assignment_id = $1 AND item_id = $2 AND state = 'open'
	`, assignmentID, itemID).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return fail(http.StatusConflict, "dispute_exists", nil)
	}
	var stored any
	if text != nil {
		stored = *text
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.dispute (assignment_id, item_id, state, reason, body)
		VALUES ($1, $2, 'open', $3, $4)
	`, assignmentID, itemID, reason, stored); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"assignment_id": assignmentID,
		"item_id":       itemID,
		"reason":        reason,
		"dispute_id":    itemID,
	})
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, "dispute_opened", payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func disputeTextInvalid(text string) bool {
	if utf8.RuneCountInString(text) > 300 {
		return true
	}
	digits := 0
	for _, r := range text {
		switch r {
		case ' ', '(', ')', '+', '-':
			continue
		}
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 9
}

func (s *Service) manualVerdict(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Verdict string `json:"verdict"`
		Points  int    `json:"points"`
		Reason  string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	if err := s.doManual(r.Context(), user, r.PathValue("attempt"), body.Verdict, body.Points, body.Reason); err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attempt_id": r.PathValue("attempt"),
		"verdict":    body.Verdict,
		"points":     body.Points,
	})
}

func (s *Service) doManual(ctx context.Context, user actor, attemptID, verdict string, points int, reason string) error {
	switch verdict {
	case "correct", "partial", "wrong":
	default:
		return fail(http.StatusUnprocessableEntity, "invalid", nil)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var assignmentID, itemID string
	err = tx.QueryRow(ctx, `
		SELECT assignment_id, item_id FROM core.attempts WHERE id = $1
	`, attemptID).Scan(&assignmentID, &itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	if err != nil {
		return err
	}
	head, err := loadAssignment(ctx, tx, assignmentID)
	if err != nil {
		return err
	}
	if !canTutor(user, head) {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	reasonValue := &reason
	if err := insertVerdict(ctx, tx, attemptID, verdict, points, reasonValue); err != nil {
		return err
	}
	scoredRows, err := loadScored(ctx, tx, assignmentID, itemID)
	if err != nil {
		return err
	}
	result, ok := summarize(scoredRows)
	if !ok {
		return fmt.Errorf("manual without attempts")
	}
	var maxPoints int
	if err := tx.QueryRow(ctx, `
		SELECT max_points FROM core.assignment_item
		WHERE assignment_id = $1 AND item_id = $2
	`, assignmentID, itemID).Scan(&maxPoints); err != nil {
		return err
	}
	if err := insertObservation(ctx, tx, assignmentID, itemID, result, maxPoints); err != nil {
		return err
	}
	verdictEvent, err := json.Marshal(map[string]any{
		"attempt_id": attemptID,
		"origin":     "manual",
		"verdict":    verdict,
		"points":     points,
		"max_points": maxPoints,
		"reason":     reason,
	})
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, "attempt_verdict", verdictEvent); err != nil {
		return err
	}
	manualEvent, err := json.Marshal(map[string]any{
		"attempt_id": attemptID,
		"verdict":    verdict,
		"points":     points,
		"reason":     reason,
	})
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, "manual_verdict", manualEvent); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) getAssignment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	body, err := s.doGet(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeResult(w, http.StatusOK, body)
}

func (s *Service) doGet(ctx context.Context, user actor, assignmentID string) ([]byte, error) {
	head, err := loadAssignment(ctx, s.pool, assignmentID)
	if err != nil {
		return nil, err
	}
	if !canRead(user, head) {
		return nil, fail(http.StatusForbidden, "forbidden", nil)
	}
	var snapshot []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT snapshot FROM core.assignment WHERE id = $1
	`, assignmentID).Scan(&snapshot); err != nil {
		return nil, err
	}
	var stored struct {
		Items []struct {
			ItemID       string `json:"item_id"`
			Statement    string `json:"statement"`
			AnswerFormat string `json:"answer_format"`
			Format       string `json:"format"`
			Options      []struct {
				N    int    `json:"n"`
				Text string `json:"text"`
			} `json:"options"`
		} `json:"items"`
	}
	if err := json.Unmarshal(snapshot, &stored); err != nil {
		return nil, err
	}
	states := map[string]string{}
	rows, err := s.pool.Query(ctx, `
		SELECT item_id, state FROM core.assignment_item WHERE assignment_id = $1
	`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			return nil, err
		}
		states[id] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	type viewOption struct {
		N    int    `json:"n"`
		Text string `json:"text"`
	}
	type viewItem struct {
		ItemID       string       `json:"item_id"`
		Statement    string       `json:"statement"`
		AnswerFormat string       `json:"answer_format"`
		ItemState    string       `json:"item_state"`
		Options      []viewOption `json:"options,omitempty"`
	}
	view := struct {
		ID    string     `json:"id"`
		Kind  string     `json:"kind"`
		State string     `json:"state"`
		Items []viewItem `json:"items"`
	}{
		ID:    assignmentID,
		Kind:  head.kind,
		State: head.state,
		Items: make([]viewItem, 0, len(stored.Items)),
	}
	for _, item := range stored.Items {
		format := item.AnswerFormat
		if format == "" {
			format = item.Format
		}
		var options []viewOption
		if len(item.Options) > 0 {
			options = make([]viewOption, 0, len(item.Options))
			for _, opt := range item.Options {
				options = append(options, viewOption{N: opt.N, Text: opt.Text})
			}
		}
		view.Items = append(view.Items, viewItem{
			ItemID:       item.ItemID,
			Statement:    item.Statement,
			AnswerFormat: format,
			ItemState:    states[item.ItemID],
			Options:      options,
		})
	}
	return json.Marshal(view)
}

func (s *Service) listAttempts(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	head, err := loadAssignment(r.Context(), s.pool, r.PathValue("id"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	if !canRead(user, head) {
		writeCode(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	items, err := loadAttemptList(r.Context(), s.pool, r.PathValue("id"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	if user.role == "student" && isTestKind(head.kind) && head.state != "submitted" {
		for i := range items {
			items[i].Verdict = ""
			items[i].Points = 0
		}
	}
	writeJSON(w, http.StatusOK, items)
}

type attemptListItem struct {
	AttemptID string `json:"attempt_id"`
	ItemID    string `json:"item_id"`
	AttemptNo int    `json:"attempt_no"`
	RawInput  string `json:"raw_input"`
	Verdict   string `json:"verdict"`
	Points    int    `json:"points"`
}

func loadAttemptList(ctx context.Context, q queryer, assignmentID string) ([]attemptListItem, error) {
	rows, err := q.Query(ctx, `
		SELECT a.id, a.item_id, a.attempt_no, a.raw_input,
		       COALESCE(v.verdict, ''), COALESCE(v.points, 0)
		FROM core.attempts AS a
		LEFT JOIN LATERAL (
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
	defer rows.Close()
	out := make([]attemptListItem, 0)
	for rows.Next() {
		var item attemptListItem
		if err := rows.Scan(&item.AttemptID, &item.ItemID, &item.AttemptNo, &item.RawInput, &item.Verdict, &item.Points); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type draftItemView struct {
	VariantID    string `json:"variant_id"`
	TypeID       string `json:"type_id"`
	AnswerFormat string `json:"answer_format"`
	Minutes      *int   `json:"minutes"`
	Source       string `json:"source"`
	Statement    string `json:"statement"`
}

type draftView struct {
	LessonID      string          `json:"lesson_id"`
	Deadline      string          `json:"deadline"`
	Items         []draftItemView `json:"items"`
	Hints         []string        `json:"hints"`
	Warnings      json.RawMessage `json:"warnings"`
	DiscussTopics []string        `json:"discuss_topics"`
	Issued        bool            `json:"issued"`
}

func (s *Service) patchHomeworkDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if user.role != "tutor" {
		writeCode(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	var body struct {
		Remove   []string `json:"remove"`
		Add      []string `json:"add"`
		Deadline *string  `json:"deadline"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid", nil)
		return
	}
	view, err := s.doPatchDraft(r.Context(), user, r.PathValue("lesson"), body.Remove, body.Add, body.Deadline)
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Service) doPatchDraft(ctx context.Context, user actor, lessonID string, remove, add []string, deadline *string) (draftView, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return draftView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireDraftOwner(ctx, tx, user, lessonID); err != nil {
		return draftView{}, err
	}
	var raw []byte
	var storedDeadline string
	err = tx.QueryRow(ctx, `
		SELECT items, deadline FROM core.homework_draft WHERE lesson_id = $1
	`, lessonID).Scan(&raw, &storedDeadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return draftView{}, fail(http.StatusForbidden, "forbidden", nil)
	}
	if err != nil {
		return draftView{}, err
	}
	items, err := decodeDraftItems(raw)
	if err != nil {
		return draftView{}, err
	}
	drop := map[string]bool{}
	for _, id := range remove {
		drop[id] = true
	}
	kept := make([]draftItemView, 0, len(items))
	for _, item := range items {
		if !drop[item.VariantID] {
			kept = append(kept, item)
		}
	}
	for _, id := range add {
		kept = append(kept, draftItemView{VariantID: id})
	}
	if deadline != nil {
		storedDeadline = *deadline
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		return draftView{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.homework_draft SET items = $2::jsonb, deadline = $3 WHERE lesson_id = $1
	`, lessonID, string(encoded), storedDeadline); err != nil {
		return draftView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return draftView{}, err
	}
	return draftView{
		LessonID:      lessonID,
		Deadline:      storedDeadline,
		Items:         kept,
		Hints:         []string{},
		Warnings:      json.RawMessage("[]"),
		DiscussTopics: []string{},
		Issued:        false,
	}, nil
}

func requireDraftOwner(ctx context.Context, q queryer, user actor, lessonID string) error {
	var owner string
	err := q.QueryRow(ctx, `
		SELECT COALESCE(u.owner_tutor_id, '')
		FROM core.lesson AS l
		JOIN core.app_user AS u ON u.id = l.student_id
		WHERE l.id = $1
	`, lessonID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (user.role != "tutor" || user.id != owner)) {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	if err != nil {
		return err
	}
	return nil
}

func decodeDraftItems(raw []byte) ([]draftItemView, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("items %T", value)
	}
	out := make([]draftItemView, 0, len(items))
	for _, item := range items {
		switch row := item.(type) {
		case string:
			out = append(out, draftItemView{VariantID: row})
		case map[string]any:
			view := draftItemView{VariantID: jsonString(row["variant_id"])}
			view.TypeID = jsonString(row["type_id"])
			view.AnswerFormat = jsonString(row["answer_format"])
			view.Source = jsonString(row["source"])
			view.Statement = jsonString(row["statement"])
			if view.VariantID == "" {
				return nil, fmt.Errorf("item without variant_id")
			}
			out = append(out, view)
		default:
			return nil, fmt.Errorf("item %T", item)
		}
	}
	return out, nil
}

func jsonString(v any) string {
	s, _ := v.(string)
	return s
}

func (s *Service) getSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	body, err := s.doSummary(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeResult(w, http.StatusOK, body)
}

func (s *Service) doSummary(ctx context.Context, user actor, assignmentID string) ([]byte, error) {
	head, err := loadAssignment(ctx, s.pool, assignmentID)
	if err != nil {
		return nil, err
	}
	if !canRead(user, head) {
		return nil, fail(http.StatusForbidden, "forbidden", nil)
	}
	if head.state != "submitted" {
		return nil, fail(http.StatusConflict, "not_submitted", nil)
	}
	items, err := loadItems(ctx, s.pool, assignmentID)
	if err != nil {
		return nil, err
	}
	response := submitResponse{
		AssignmentID: assignmentID,
		Items:        make([]submitItem, 0, len(items)),
	}
	for _, item := range items {
		response.MaxPoints += item.max
		if len(item.attempts) == 0 {
			response.Skipped++
			continue
		}
		response.Answered++
		result, ok := summarize(item.attempts)
		if !ok {
			continue
		}
		response.TotalPoints += result.TotalPoints
		response.Items = append(response.Items, submitItem{
			ItemID:    item.id,
			Verdict:   result.Verdict,
			Points:    result.TotalPoints,
			MaxPoints: item.max,
		})
	}
	return json.Marshal(response)
}

func (s *Service) listDisputes(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if err := s.tutorOwnsStudent(r.Context(), user, r.PathValue("id")); err != nil {
		writeHandlerErr(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT d.item_id, d.assignment_id, d.reason, d.state, COALESCE(d.body, '')
		FROM core.dispute AS d
		JOIN core.assignment AS a ON a.id = d.assignment_id
		WHERE a.student_id = $1
		ORDER BY d.assignment_id, d.item_id
	`, r.PathValue("id"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	defer rows.Close()
	out := make([]disputeListItem, 0)
	for rows.Next() {
		var item disputeListItem
		if err := rows.Scan(&item.ID, &item.AssignmentID, &item.Reason, &item.State, &item.Text); err != nil {
			writeHandlerErr(w, err)
			return
		}
		item.ItemID = item.ID
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) getDiscussList(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if err := s.tutorOwnsStudent(r.Context(), user, r.PathValue("id")); err != nil {
		writeHandlerErr(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT payload->>'item_id', payload->>'assignment_id', COALESCE(payload->>'variant_id', '')
		FROM core.journal_event
		WHERE kind = 'did_not_understand'
		  AND payload->>'assignment_id' IN (
			SELECT id FROM core.assignment WHERE student_id = $1
		  )
		ORDER BY at
	`, r.PathValue("id"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := make([]discussListItem, 0)
	for rows.Next() {
		var item discussListItem
		if err := rows.Scan(&item.ItemID, &item.AssignmentID, &item.VariantID); err != nil {
			writeHandlerErr(w, err)
			return
		}
		key := item.AssignmentID + ":" + item.ItemID
		if seen[key] || item.ItemID == "" {
			continue
		}
		seen[key] = true
		if item.VariantID == "" {
			item.VariantID = item.ItemID
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) tutorOwnsStudent(ctx context.Context, user actor, studentID string) error {
	if user.role != "tutor" {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	var owner string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(owner_tutor_id, '')
		FROM core.app_user
		WHERE id = $1 AND role = 'student'
	`, studentID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != user.id) {
		return fail(http.StatusForbidden, "forbidden", nil)
	}
	if err != nil {
		return err
	}
	return nil
}

type disputeListItem struct {
	ID           string `json:"id"`
	AssignmentID string `json:"assignment_id"`
	ItemID       string `json:"item_id"`
	Reason       string `json:"reason"`
	State        string `json:"state"`
	Text         string `json:"text,omitempty"`
}

type discussListItem struct {
	ItemID       string `json:"item_id"`
	VariantID    string `json:"variant_id"`
	AssignmentID string `json:"assignment_id"`
}
