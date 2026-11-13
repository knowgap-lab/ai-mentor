package privacy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	linkTTL     = 14 * 24 * time.Hour
	anonymiseIn = 7 * 24 * time.Hour
	adultAge    = 18
)

type consentLink struct {
	studentID string
	kind      string
	expires   time.Time
}

// CreateLink выпускает ссылку согласия.
// Токен — 32 случайных байта в hex; в базе хранится только SHA-256 этих байт.
// created_at равен now, срок — 14 суток. Возраст считается по дате рождения на now.
// Младше 18 лет нельзя выпустить ссылку student, с 18 лет — ссылку parent.
func (s *Service) CreateLink(ctx context.Context, studentID, kind string, now time.Time) (string, error) {
	if err := s.kindAllowed(ctx, studentID, kind, now); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	hash := append([]byte(nil), sum[:]...)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO core.consent_link (token_hash, student_id, kind, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, hash, studentID, kind, now.Add(linkTTL), now)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) kindAllowed(ctx context.Context, studentID, kind string, now time.Time) error {
	if kind != "parent" && kind != "student" {
		return errKind()
	}
	birth, err := s.birthDate(ctx, studentID)
	if err != nil {
		return err
	}
	age := ageYears(birth, now)
	if (kind == "student" && age < adultAge) || (kind == "parent" && age >= adultAge) {
		return errKind()
	}
	return nil
}

func (s *Service) birthDate(ctx context.Context, studentID string) (time.Time, error) {
	var text *string
	err := s.pool.QueryRow(ctx, `
		SELECT birth_date::text
		FROM core.student_pii
		WHERE student_id = $1
	`, studentID).Scan(&text)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (text == nil || *text == "")) {
		return time.Time{}, errors.New("birth date is missing")
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("2006-01-02", *text, time.UTC)
}

func ageYears(birth, now time.Time) int {
	by, bm, bd := birth.In(time.UTC).Date()
	ny, nm, nd := now.In(time.UTC).Date()
	age := ny - by
	if nm < bm || (nm == bm && nd < bd) {
		age--
	}
	return age
}

func (s *Service) findLink(ctx context.Context, token string, now time.Time) (consentLink, error) {
	hash, ok := hashToken(token)
	if !ok {
		return consentLink{}, errGone()
	}
	var row consentLink
	err := s.pool.QueryRow(ctx, `
		SELECT student_id, kind, expires_at
		FROM core.consent_link
		WHERE token_hash = $1
	`, hash).Scan(&row.studentID, &row.kind, &row.expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return consentLink{}, errGone()
	}
	if err != nil {
		return consentLink{}, err
	}
	if !now.Before(row.expires) {
		return consentLink{}, errGone()
	}
	return row, nil
}

func hashToken(token string) ([]byte, bool) {
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
