package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Сеанс хранится в core.session. alexedwards/scs не используется:
// библиотека пишет в базу сам токен cookie, а здесь лежит только SHA-256
// 32 сырых байт. CSRF — это тот же токен в X-CSRF-Token (AU-R24), не отдельная схема.

const revokeStudentOverflow = `
UPDATE core.session
SET revoked_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL
  AND expires_at > now()
  AND token_hash NOT IN (
      SELECT token_hash
      FROM core.session
      WHERE user_id = $1
        AND revoked_at IS NULL
        AND expires_at > now()
      ORDER BY created_at DESC, token_hash DESC
      LIMIT 5
  )
`

func (s *Service) openSession(ctx context.Context, telegramID, role string) (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	hash := append([]byte(nil), sum[:]...)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1::text), hashtext($2::text))`,
		telegramID, role,
	); err != nil {
		return "", "", err
	}

	var userID string
	err = tx.QueryRow(ctx, `
		SELECT u.id
		FROM core.app_user AS u
		JOIN core.user_pii AS p ON p.user_id = u.id
		WHERE p.telegram_id = $1 AND u.role = $2
		FOR UPDATE OF u
	`, telegramID, role).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		userID, err = insertUser(ctx, tx, telegramID, role)
	}
	if err != nil {
		return "", "", err
	}

	hours := int32(90 * 24)
	if role == "tutor" {
		hours = 7 * 24
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.session (token_hash, user_id, created_at, expires_at)
		VALUES ($1, $2, now(), now() + $3 * interval '1 hour')
	`, hash, userID, hours); err != nil {
		return "", "", err
	}
	if role == "student" {
		if _, err := tx.Exec(ctx, revokeStudentOverflow, userID); err != nil {
			return "", "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, token, nil
}

func insertUser(ctx context.Context, tx pgx.Tx, telegramID, role string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, $2, NULL, 'active')
	`, id, role); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id)
		VALUES ($1, $2)
	`, id, telegramID); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) sessionUser(ctx context.Context, token string) (string, string, bool, error) {
	hash, ok := hashToken(token)
	if !ok {
		return "", "", false, nil
	}
	var userID, role string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
	`, hash).Scan(&userID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return userID, role, true, nil
}

func (s *Service) revokeSession(ctx context.Context, token string) error {
	hash, ok := hashToken(token)
	if !ok {
		return errUnauthorized()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID, role string
	err = tx.QueryRow(ctx, `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
		FOR UPDATE OF sess
	`, hash).Scan(&userID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return errUnauthorized()
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE core.session
		SET revoked_at = now()
		WHERE token_hash = $1
		  AND revoked_at IS NULL
	`, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errUnauthorized()
	}
	payload, err := json.Marshal(map[string]string{
		"user_id": userID,
		"reason":  "logout",
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload, student_id, actor_role, actor_id)
		VALUES ('session_revoked', $1::jsonb, $2, $3, $4)
	`, string(payload), userID, role, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func hashToken(token string) ([]byte, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
}
