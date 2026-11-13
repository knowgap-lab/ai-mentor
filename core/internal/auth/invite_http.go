package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const inviteTTL = 72 * time.Hour

func (s *Service) getInvite(w http.ResponseWriter, r *http.Request) {
	hash, ok := inviteHash(r.PathValue("token"))
	if !ok {
		writeErr(w, errInviteGone())
		return
	}
	var expires time.Time
	var used *time.Time
	var name *string
	err := s.pool.QueryRow(r.Context(), `
		SELECT i.expires_at, i.used_at, p.display_name
		FROM core.telegram_invite AS i
		LEFT JOIN core.student_pii AS p ON p.student_id = i.student_id
		WHERE i.token_hash = $1
	`, hash).Scan(&expires, &used, &name)
	if errors.Is(err, pgx.ErrNoRows) || used != nil || time.Now().After(expires) {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, errUnavailable())
			return
		}
		writeErr(w, errInviteGone())
		return
	}
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	display := ""
	if name != nil {
		display = *name
	}
	writeJSON(w, http.StatusOK, struct {
		StudentDisplayName string `json:"student_display_name"`
		Valid              bool   `json:"valid"`
	}{StudentDisplayName: display, Valid: true})
}

func (s *Service) createInvite(w http.ResponseWriter, r *http.Request) {
	actor, token, ok := s.actorFrom(w, r)
	if !ok {
		return
	}
	if !s.csrfOK(w, r, token) {
		return
	}
	studentID := r.PathValue("id")
	if actor.Role != "tutor" {
		writeErr(w, errForbidden())
		return
	}
	if err := s.Authorize(actor, "write", Object{UserID: studentID}); err != nil {
		writeServiceErr(w, err)
		return
	}
	link, err := s.issueInvite(r.Context(), studentID)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

type inviteLink struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Service) issueInvite(ctx context.Context, studentID string) (inviteLink, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return inviteLink{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM core.app_user WHERE id = $1 FOR UPDATE`, studentID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return inviteLink{}, errForbidden()
	}
	if err != nil {
		return inviteLink{}, err
	}
	active, err := consentActive(ctx, tx, studentID)
	if err != nil {
		return inviteLink{}, err
	}
	if !active {
		return inviteLink{}, errConsentMissing()
	}

	now := time.Now().UTC()
	var live int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM core.telegram_invite
		WHERE student_id = $1
		  AND used_at IS NULL
		  AND expires_at > $2
	`, studentID, now).Scan(&live); err != nil {
		return inviteLink{}, err
	}
	if live > 0 {
		if err := s.reissue(ctx, tx, studentID, now); err != nil {
			return inviteLink{}, err
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return inviteLink{}, err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	hash := append([]byte(nil), sum[:]...)
	exp := now.Add(inviteTTL)
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.telegram_invite (token_hash, student_id, expires_at)
		VALUES ($1, $2, $3)
	`, hash, studentID, exp); err != nil {
		return inviteLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return inviteLink{}, err
	}
	return inviteLink{
		URL:       s.cfg.AppOrigin + "/invite/" + token,
		ExpiresAt: exp.UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) reissue(ctx context.Context, tx pgx.Tx, studentID string, now time.Time) error {
	if _, err := tx.Exec(ctx, `
		UPDATE core.telegram_invite
		SET used_at = $2
		WHERE student_id = $1 AND used_at IS NULL
	`, studentID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.student_pii SET telegram_id = '' WHERE student_id = $1
	`, studentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.user_pii SET telegram_id = '' WHERE user_id = $1
	`, studentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.session
		SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL
	`, studentID, now); err != nil {
		return err
	}
	_, err := insertJournal(ctx, tx, "session_revoked", map[string]string{
		"user_id": studentID,
		"reason":  "invite_reissue",
	})
	return err
}
