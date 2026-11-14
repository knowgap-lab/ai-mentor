package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// AnonymiseDue обезличивает учеников, у которых anonymise_at уже наступил к now.
// Срок — 7 суток из PV-R17. Возвращает, скольких учеников обезличил этот вызов.
func (s *Service) AnonymiseDue(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.student_id
		FROM core.consent AS c
		JOIN core.app_user AS u ON u.id = c.student_id
		WHERE c.anonymise_at IS NOT NULL
		  AND c.anonymise_at <= $1
		  AND u.status <> 'anonymised'
	`, now)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		done, err := s.anonymiseOne(ctx, id, now, "schedule")
		if err != nil {
			return n, err
		}
		if done {
			n++
		}
	}
	return n, nil
}

func (s *Service) anonymiseStudent(w http.ResponseWriter, r *http.Request) {
	tutorID, ok := s.tutor(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !s.mustOwn(w, r, tutorID, id) {
		return
	}
	confirm, err := readConfirm(r)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	if !confirm {
		writeErr(w, errConfirmation())
		return
	}
	if _, err := s.anonymiseOne(r.Context(), id, time.Now(), "tutor_on_request"); err != nil {
		writeServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readConfirm(r *http.Request) (bool, error) {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := dec.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, errUnavailable()
	}
	return body.Confirm, nil
}

func (s *Service) anonymiseOne(ctx context.Context, studentID string, now time.Time, source string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE core.app_user
		SET status = 'anonymised'
		WHERE id = $1 AND status <> 'anonymised'
	`, studentID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	for _, stmt := range []string{
		`DELETE FROM core.student_pii WHERE student_id = $1`,
		`DELETE FROM core.student_pseudonym WHERE student_id = $1`,
		`DELETE FROM core.user_pii WHERE user_id = $1`,
		`DELETE FROM core.consent_link WHERE student_id = $1`,
		`DELETE FROM core.telegram_invite WHERE student_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, studentID); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.outbox
		SET state = 'cancelled'
		WHERE recipient_id = $1
		  AND state = 'pending'
	`, studentID); err != nil {
		return false, err
	}
	if err := revokeSessions(ctx, tx, studentID, now); err != nil {
		return false, err
	}
	if err := insertEvent(ctx, tx, "student_anonymised", map[string]string{"source": source}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
