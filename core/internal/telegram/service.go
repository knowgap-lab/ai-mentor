// Пакет telegram проверяет подпись входа и одноразовое приглашение.
// Сеть к Telegram не вызывается. Сырой токен приглашения не пишется
// ни в базу, ни в журнал. Из данных входа обмен читает только id.
package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const inviteTTL = 72 * time.Hour

// Config — параметры сервиса. В Log токен приглашения и профиль входа не пишутся.
type Config struct {
	DSN string
	Log io.Writer
}

// Service — приглашения Telegram в Postgres.
type Service struct {
	pool *pgxpool.Pool
}

type rower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Open подключается к Postgres. Стартовать сеть к Telegram не нужно.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Service{pool: pool}, nil
}

// Close закрывает пул соединений.
func (s *Service) Close() error {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// Create выпускает приглашение ученику.
// Токен — 32 случайных байта в hex; в базе только SHA-256 этих байт.
// Срок — 72 часа от now, used_at остаётся NULL. Строки ученика не меняются.
func (s *Service) Create(ctx context.Context, studentID string, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	hash := append([]byte(nil), sum[:]...)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO core.telegram_invite (token_hash, student_id, expires_at)
		VALUES ($1, $2, $3)
	`, hash, studentID, now.Add(inviteTTL))
	if err != nil {
		return "", err
	}
	return token, nil
}

// Preview проверяет, что приглашение ещё действует, и ничего не меняет.
func (s *Service) Preview(ctx context.Context, token string, now time.Time) error {
	_, err := s.load(ctx, s.pool, token, now, false)
	return err
}

// Exchange гасит действующее приглашение и пишет telegram_id в core.student_pii.
// Подпись виджета здесь не проверяется: её считает Check. Из data читается только id.
// Просроченное или уже использованное приглашение — invite_gone, без сеанса.
// Занятый учеником или репетитором id — telegram_already_bound, строка приглашения не гасится.
func (s *Service) Exchange(ctx context.Context, token string, data map[string]string, now time.Time) error {
	hash, ok := tokenHash(token)
	if !ok {
		return errInviteGone()
	}
	telegramID := data["id"]

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	studentID, err := s.loadLocked(ctx, tx, hash, now)
	if err != nil {
		return err
	}
	taken, err := telegramTaken(ctx, tx, telegramID, studentID)
	if err != nil {
		return err
	}
	if taken {
		return errTelegramBound()
	}
	tag, err := tx.Exec(ctx, `
		UPDATE core.student_pii
		SET telegram_id = $1
		WHERE student_id = $2
	`, telegramID, studentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("student not found")
	}
	tag, err = tx.Exec(ctx, `
		UPDATE core.telegram_invite
		SET used_at = $2
		WHERE token_hash = $1
		  AND used_at IS NULL
	`, hash, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errInviteGone()
	}
	return tx.Commit(ctx)
}

func (s *Service) load(ctx context.Context, q rower, token string, now time.Time, lock bool) (string, error) {
	hash, ok := tokenHash(token)
	if !ok {
		return "", errInviteGone()
	}
	return s.loadHash(ctx, q, hash, now, lock)
}

func (s *Service) loadLocked(ctx context.Context, q rower, hash []byte, now time.Time) (string, error) {
	return s.loadHash(ctx, q, hash, now, true)
}

func (s *Service) loadHash(ctx context.Context, q rower, hash []byte, now time.Time, lock bool) (string, error) {
	query := `
		SELECT student_id, expires_at, used_at
		FROM core.telegram_invite
		WHERE token_hash = $1
	`
	if lock {
		query += " FOR UPDATE"
	}
	var studentID string
	var expires time.Time
	var used *time.Time
	err := q.QueryRow(ctx, query, hash).Scan(&studentID, &expires, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errInviteGone()
	}
	if err != nil {
		return "", err
	}
	if used != nil || now.After(expires) {
		return "", errInviteGone()
	}
	return studentID, nil
}

func telegramTaken(ctx context.Context, q rower, telegramID, studentID string) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM core.student_pii
			WHERE telegram_id = $1
			  AND student_id <> $2
		) OR EXISTS (
			SELECT 1
			FROM core.user_pii
			WHERE telegram_id = $1
		)
	`, telegramID, studentID).Scan(&taken)
	return taken, err
}

func tokenHash(token string) ([]byte, bool) {
	raw, ok := decodeToken(token)
	if !ok {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
}

func decodeToken(token string) ([]byte, bool) {
	if raw, err := hex.DecodeString(token); err == nil && len(raw) == 32 && strings.EqualFold(hex.EncodeToString(raw), token) {
		return raw, true
	}
	if raw, err := base64.RawURLEncoding.DecodeString(token); err == nil && len(raw) == 32 {
		return raw, true
	}
	if raw, err := base64.URLEncoding.DecodeString(token); err == nil && len(raw) == 32 {
		return raw, true
	}
	return nil, false
}
