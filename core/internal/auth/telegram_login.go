package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/notify"
	"github.com/knowgap-lab/ai-mentor/core/internal/telegram"
)

const deviceCookieName = "__Host-device"

type loginResult struct {
	userID  string
	role    string
	token   string
	device  string
	pending bool
}

type account struct {
	id     string
	role   string
	status string
}

// loginTelegram — POST /auth/telegram.
// Сначала Origin, затем подпись и свежесть (AU-R12, AU-R13), затем приглашение и согласие.
func (s *Service) loginTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != s.cfg.AppOrigin {
		writeErr(w, errCSRF())
		return
	}
	fields, err := readWidget(r.Body)
	if err != nil || fields["id"] == "" {
		writeErr(w, errLoginInvalid())
		return
	}
	// Пустой TELEGRAM_BOT_TOKEN не ключ подписи: иначе HMAC от SHA-256("")
	// принимает подделанный hash (AU-R12). Токен читается при Open.
	if s.botToken == "" {
		writeErr(w, errLoginInvalid())
		return
	}
	if err := telegram.Check(fields, s.botToken, time.Now()); err != nil {
		writeTelegramErr(w, err)
		return
	}
	var result loginResult
	if invite := fields["invite_token"]; invite != "" {
		result, err = s.loginWithInvite(r.Context(), fields["id"], invite, time.Now().UTC())
	} else {
		result, err = s.loginKnown(r, fields["id"])
	}
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	http.SetCookie(w, sessionCookie(result.token))
	if result.device != "" {
		http.SetCookie(w, deviceCookie(result.device))
	}
	w.Header().Set("X-CSRF-Token", result.token)
	if result.pending {
		writeErr(w, errDevicePending())
		return
	}
	writeMe(w, result.userID, result.role)
}

func writeTelegramErr(w http.ResponseWriter, err error) {
	var api *telegram.Error
	if errors.As(err, &api) {
		writeErr(w, apiError(api.HTTPStatus, api.Code))
		return
	}
	writeErr(w, errUnavailable())
}

func readWidget(body io.Reader) (map[string]string, error) {
	dec := json.NewDecoder(io.LimitReader(body, 1<<20))
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("trailing json")
	}
	out := make(map[string]string, len(raw))
	for key, val := range raw {
		text, err := scalarString(val)
		if err != nil {
			return nil, err
		}
		out[key] = text
	}
	return out, nil
}

func scalarString(raw json.RawMessage) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || text[0] == '{' || text[0] == '[' {
		return "", errors.New("bad widget field")
	}
	if text[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	return text, nil
}

func (s *Service) loginWithInvite(ctx context.Context, telegramID, invite string, now time.Time) (loginResult, error) {
	hash, ok := inviteHash(invite)
	if !ok {
		return loginResult{}, errInviteGone()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return loginResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var studentID string
	var expires time.Time
	var used *time.Time
	err = tx.QueryRow(ctx, `
		SELECT student_id, expires_at, used_at
		FROM core.telegram_invite
		WHERE token_hash = $1
		FOR UPDATE
	`, hash).Scan(&studentID, &expires, &used)
	if errors.Is(err, pgx.ErrNoRows) || used != nil || now.After(expires) {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return loginResult{}, err
		}
		return loginResult{}, errInviteGone()
	}
	if err != nil {
		return loginResult{}, err
	}

	active, err := consentActive(ctx, tx, studentID)
	if err != nil {
		return loginResult{}, err
	}
	if !active {
		return loginResult{}, errConsentMissing()
	}
	taken, err := telegramTaken(ctx, tx, telegramID, studentID)
	if err != nil {
		return loginResult{}, err
	}
	if taken {
		return loginResult{}, errTelegramBound()
	}
	tag, err := tx.Exec(ctx, `
		UPDATE core.student_pii
		SET telegram_id = $1
		WHERE student_id = $2
	`, telegramID, studentID)
	if err != nil {
		return loginResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return loginResult{}, errors.New("student pii missing")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET telegram_id = EXCLUDED.telegram_id
	`, studentID, telegramID); err != nil {
		return loginResult{}, err
	}
	tag, err = tx.Exec(ctx, `
		UPDATE core.telegram_invite
		SET used_at = $2
		WHERE token_hash = $1
		  AND used_at IS NULL
	`, hash, now)
	if err != nil {
		return loginResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return loginResult{}, errInviteGone()
	}
	token, err := insertSessionTx(ctx, tx, studentID, "student", "active")
	if err != nil {
		return loginResult{}, err
	}
	seq, err := insertJournal(ctx, tx, "account_bound", map[string]string{
		"user_id": studentID,
		"role":    "student",
	})
	if err != nil {
		return loginResult{}, err
	}
	var owner *string
	if err := tx.QueryRow(ctx, `
		SELECT owner_tutor_id FROM core.app_user WHERE id = $1
	`, studentID).Scan(&owner); err != nil {
		return loginResult{}, err
	}
	if owner != nil && *owner != "" {
		id, err := randomID()
		if err != nil {
			return loginResult{}, err
		}
		if err := notify.Enqueue(ctx, tx, notify.Message{
			ID:            id,
			Kind:          "account_bound",
			DedupKey:      "account_bound:" + itoa(seq) + ":" + *owner,
			RecipientKind: "tutor",
			RecipientID:   *owner,
			Template:      "account_bound",
			Params:        map[string]any{"student_id": studentID},
		}); err != nil {
			return loginResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return loginResult{}, err
	}
	return loginResult{userID: studentID, role: "student", token: token}, nil
}

func (s *Service) loginKnown(r *http.Request, telegramID string) (loginResult, error) {
	ctx := r.Context()
	acc, ok, err := s.findAccount(ctx, telegramID)
	if err != nil {
		return loginResult{}, err
	}
	if !ok {
		return loginResult{}, errLoginInvalid()
	}
	if acc.status != "active" {
		return loginResult{}, errAccountLocked()
	}
	if acc.role == "student" {
		active, err := consentActive(ctx, s.pool, acc.id)
		if err != nil {
			return loginResult{}, err
		}
		if !active {
			return loginResult{}, errConsentMissing()
		}
		token, err := s.openKnown(ctx, acc.id, "student", "active")
		if err != nil {
			return loginResult{}, err
		}
		return loginResult{userID: acc.id, role: "student", token: token}, nil
	}
	if acc.role != "tutor" {
		return loginResult{}, errLoginInvalid()
	}
	return s.loginTutor(r, acc.id)
}

func (s *Service) openKnown(ctx context.Context, userID, role, state string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	token, err := insertSessionTx(ctx, tx, userID, role, state)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) findAccount(ctx context.Context, telegramID string) (account, bool, error) {
	var acc account
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.role, u.status
		FROM core.app_user AS u
		JOIN core.user_pii AS p ON p.user_id = u.id
		WHERE p.telegram_id = $1 AND p.telegram_id <> ''
	`, telegramID).Scan(&acc.id, &acc.role, &acc.status)
	if err == nil {
		return acc, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return account{}, false, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT u.id, u.role, u.status
		FROM core.app_user AS u
		JOIN core.student_pii AS p ON p.student_id = u.id
		WHERE p.telegram_id = $1 AND p.telegram_id <> ''
	`, telegramID).Scan(&acc.id, &acc.role, &acc.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return account{}, false, nil
	}
	if err != nil {
		return account{}, false, err
	}
	return acc, true, nil
}

func (s *Service) loginTutor(r *http.Request, tutorID string) (loginResult, error) {
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return loginResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	raw, has := deviceRaw(r)
	var confirmed int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM core.tutor_device
		WHERE user_id = $1 AND confirmed_at IS NOT NULL
	`, tutorID).Scan(&confirmed); err != nil {
		return loginResult{}, err
	}
	matched := false
	if has {
		err := tx.QueryRow(ctx, `
			SELECT confirmed_at IS NOT NULL
			FROM core.tutor_device
			WHERE user_id = $1 AND token_hash = $2
		`, tutorID, deviceHash(raw)).Scan(&matched)
		if errors.Is(err, pgx.ErrNoRows) {
			matched = false
		} else if err != nil {
			return loginResult{}, err
		}
	}

	if confirmed == 0 {
		if !has {
			raw = make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				return loginResult{}, err
			}
		}
		if err := saveDevice(ctx, tx, tutorID, raw, true); err != nil {
			return loginResult{}, err
		}
		token, err := insertSessionTx(ctx, tx, tutorID, "tutor", "active")
		if err != nil {
			return loginResult{}, err
		}
		if _, err := insertJournal(ctx, tx, "device_confirmed", map[string]string{
			"tutor_id": tutorID,
			"method":   "first_login",
		}); err != nil {
			return loginResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return loginResult{}, err
		}
		return loginResult{
			userID: tutorID,
			role:   "tutor",
			token:  token,
			device: hex.EncodeToString(raw),
		}, nil
	}

	if matched {
		token, err := insertSessionTx(ctx, tx, tutorID, "tutor", "active")
		if err != nil {
			return loginResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return loginResult{}, err
		}
		return loginResult{
			userID: tutorID,
			role:   "tutor",
			token:  token,
			device: hex.EncodeToString(raw),
		}, nil
	}

	if !has {
		raw = make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return loginResult{}, err
		}
	}
	if err := saveDevice(ctx, tx, tutorID, raw, false); err != nil {
		return loginResult{}, err
	}
	token, err := insertSessionTx(ctx, tx, tutorID, "tutor", "pending")
	if err != nil {
		return loginResult{}, err
	}
	id, err := randomID()
	if err != nil {
		return loginResult{}, err
	}
	dedup, err := randomID()
	if err != nil {
		return loginResult{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Message{
		ID:            id,
		Kind:          "new_device_login",
		DedupKey:      "new_device_login:" + dedup + ":" + tutorID,
		RecipientKind: "tutor",
		RecipientID:   tutorID,
		Template:      "new_device_login",
		Params:        map[string]any{"user_id": tutorID},
	}); err != nil {
		return loginResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return loginResult{}, err
	}
	return loginResult{
		userID:  tutorID,
		role:    "tutor",
		token:   token,
		device:  hex.EncodeToString(raw),
		pending: true,
	}, nil
}

func saveDevice(ctx context.Context, tx pgx.Tx, userID string, raw []byte, confirm bool) error {
	hash := deviceHash(raw)
	if confirm {
		_, err := tx.Exec(ctx, `
			INSERT INTO core.tutor_device (token_hash, user_id, confirmed_at)
			VALUES ($1, $2, now())
			ON CONFLICT (token_hash) DO UPDATE
			SET user_id = EXCLUDED.user_id,
			    confirmed_at = now()
		`, hash, userID)
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.tutor_device (token_hash, user_id, confirmed_at)
		VALUES ($1, $2, NULL)
		ON CONFLICT (token_hash) DO NOTHING
	`, hash, userID)
	return err
}

func deviceRaw(r *http.Request) ([]byte, bool) {
	cookie, err := r.Cookie(deviceCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	raw, ok := decodeFixed(cookie.Value, 16)
	return raw, ok
}

func deviceCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     deviceCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   400 * 24 * 60 * 60,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func telegramTaken(ctx context.Context, tx pgx.Tx, telegramID, studentID string) (bool, error) {
	var taken bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.student_pii
			WHERE telegram_id = $1 AND telegram_id <> '' AND student_id <> $2
		) OR EXISTS (
			SELECT 1 FROM core.user_pii
			WHERE telegram_id = $1 AND telegram_id <> '' AND user_id <> $2
		)
	`, telegramID, studentID).Scan(&taken)
	return taken, err
}

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func consentActive(ctx context.Context, q queryRower, studentID string) (bool, error) {
	var state string
	err := q.QueryRow(ctx, `SELECT state FROM core.consent WHERE student_id = $1`, studentID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state == "active", nil
}

func insertJournal(ctx context.Context, tx pgx.Tx, kind string, payload any) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var seq int64
	err = tx.QueryRow(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ($1, $2::jsonb)
		RETURNING journal_seq
	`, kind, string(raw)).Scan(&seq)
	return seq, err
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
