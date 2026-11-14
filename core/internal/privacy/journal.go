package privacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func insertEvent(ctx context.Context, tx pgx.Tx, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ($1, $2::jsonb)
	`, kind, raw)
	return err
}

func insertRevokeNotice(ctx context.Context, tx pgx.Tx, tutorID, studentID string) error {
	if tutorID == "" {
		return nil
	}
	id, err := newID()
	if err != nil {
		return err
	}
	params, err := json.Marshal(map[string]string{"student_id": studentID})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.outbox (
			id, kind, payload, recipient_kind, recipient_id, template, params, state
		) VALUES (
			$1, 'consent_revoked', '{}'::jsonb, 'tutor', $2, 'consent_revoked', $3::jsonb, 'pending'
		)
	`, id, tutorID, params)
	return err
}

func revokeSessions(ctx context.Context, tx pgx.Tx, userID string, now time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE core.session
		SET revoked_at = $2
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, userID, now)
	return err
}

func ownerTutor(ctx context.Context, tx pgx.Tx, studentID string) (string, error) {
	var owner *string
	err := tx.QueryRow(ctx, `
		SELECT owner_tutor_id
		FROM core.app_user
		WHERE id = $1
	`, studentID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if owner == nil {
		return "", nil
	}
	return *owner, nil
}

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
