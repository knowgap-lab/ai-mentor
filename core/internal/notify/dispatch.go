package notify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxSendsPerSecond = 20
	maxAttempts       = 5
	basePause         = 30 * time.Second
	networkError      = "transport"
	unreachableError  = "unreachable"
)

type dueRow struct {
	id            string
	recipientKind string
	recipientID   string
	template      string
	params        map[string]any
	attempts      int
}

// Dispatch забирает созревшие pending-строки и отдаёт их транспорту.
// now — часы правил, не время процесса. За вызов уходит не больше одной строки
// получателя и не больше 20 отправок. Ошибка транспорта остаётся в строке;
// наружу возвращается только сбой базы.
func Dispatch(ctx context.Context, conn *pgx.Conn, now time.Time, transport Transport) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := dispatchTx(ctx, tx, now, transport); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Unreachable истинен, если получателю уже поставлен признак недоставки
// (ответ транспорта 400 или 403). Неизвестный получатель — false без ошибки.
func Unreachable(ctx context.Context, conn *pgx.Conn, recipientID string) (bool, error) {
	var ok bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM core.outbox
			WHERE recipient_id = $1
			  AND last_error = $2
		)
	`, recipientID, unreachableError).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func dispatchTx(ctx context.Context, tx pgx.Tx, now time.Time, transport Transport) error {
	used, global, err := recentSends(ctx, tx, now)
	if err != nil {
		return err
	}
	due, err := lockDue(ctx, tx, now)
	if err != nil {
		return err
	}
	for _, row := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		chat, err := lookupChat(ctx, tx, row.recipientKind, row.recipientID)
		if err != nil {
			return err
		}
		key := rateKey(chat, row.recipientKind, row.recipientID)
		if last, blocked := used[key]; blocked {
			if err := postpone(ctx, tx, row.id, last.Add(time.Second)); err != nil {
				return err
			}
			continue
		}
		if global >= maxSendsPerSecond {
			if err := postpone(ctx, tx, row.id, now.Add(time.Second)); err != nil {
				return err
			}
			continue
		}
		if err := deliver(ctx, tx, transport, now, row, chat); err != nil {
			return err
		}
		used[key] = now
		global++
	}
	return nil
}

func recentSends(ctx context.Context, tx pgx.Tx, now time.Time) (map[string]time.Time, int, error) {
	rows, err := tx.Query(ctx, `
		SELECT o.recipient_kind,
		       o.recipient_id,
		       o.sent_at,
		       CASE
		           WHEN o.recipient_kind = 'student' THEN (
		               SELECT telegram_id FROM core.student_pii WHERE student_id = o.recipient_id
		           )
		           ELSE (
		               SELECT telegram_id FROM core.user_pii WHERE user_id = o.recipient_id
		           )
		       END
		FROM core.outbox o
		WHERE o.state = 'sent'
		  AND o.sent_at > $1
		  AND o.sent_at <= $2
	`, now.Add(-time.Second), now)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	used := map[string]time.Time{}
	global := 0
	for rows.Next() {
		var kind, id string
		var sent time.Time
		var chat *string
		if err := rows.Scan(&kind, &id, &sent, &chat); err != nil {
			return nil, 0, err
		}
		global++
		value := ""
		if chat != nil {
			value = *chat
		}
		key := rateKey(value, kind, id)
		if prev, ok := used[key]; !ok || sent.After(prev) {
			used[key] = sent
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return used, global, nil
}

func lockDue(ctx context.Context, tx pgx.Tx, now time.Time) ([]dueRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, recipient_kind, recipient_id, template,
		       COALESCE(params, '{}'::jsonb)::text, attempts
		FROM core.outbox
		WHERE state = 'pending'
		  AND next_attempt_at <= $1
		ORDER BY created_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []dueRow
	for rows.Next() {
		var row dueRow
		var raw string
		if err := rows.Scan(&row.id, &row.recipientKind, &row.recipientID, &row.template, &raw, &row.attempts); err != nil {
			return nil, err
		}
		params := map[string]any{}
		if raw != "" && raw != "null" {
			if err := json.Unmarshal([]byte(raw), &params); err != nil {
				return nil, err
			}
		}
		row.params = params
		due = append(due, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return due, nil
}

func deliver(ctx context.Context, tx pgx.Tx, transport Transport, now time.Time, row dueRow, chat string) error {
	if chat == "" {
		return applyResult(ctx, tx, row, now, errors.New(networkError))
	}
	displayName := ""
	if strings.Contains(row.template, "{display_name}") {
		name, err := lookupDisplayName(ctx, tx, studentIDOf(row.params))
		if err != nil {
			return err
		}
		displayName = name
	}
	text := render(row.template, row.params, displayName)
	sendErr := transport.Send(ctx, Delivery{
		RecipientKind: row.recipientKind,
		RecipientID:   row.recipientID,
		ChatID:        chat,
		Text:          text,
	})
	if sendErr != nil && ctx.Err() != nil && errors.Is(sendErr, ctx.Err()) {
		return sendErr
	}
	return applyResult(ctx, tx, row, now, sendErr)
}

func applyResult(ctx context.Context, tx pgx.Tx, row dueRow, now time.Time, sendErr error) error {
	if sendErr == nil {
		_, err := tx.Exec(ctx, `
			UPDATE core.outbox
			SET state = 'sent', sent_at = $2, last_error = NULL
			WHERE id = $1
		`, row.id, now)
		return err
	}
	var fail *Failure
	if errors.As(sendErr, &fail) && fail != nil {
		switch {
		case fail.Status == 429:
			_, err := tx.Exec(ctx, `
				UPDATE core.outbox
				SET next_attempt_at = $2
				WHERE id = $1
			`, row.id, now.Add(fail.RetryAfter))
			return err
		case fail.Status == 400 || fail.Status == 403:
			_, err := tx.Exec(ctx, `
				UPDATE core.outbox
				SET state = 'failed', last_error = $2
				WHERE id = $1
			`, row.id, unreachableError)
			return err
		}
	}
	nextAttempts := row.attempts + 1
	if nextAttempts >= maxAttempts {
		_, err := tx.Exec(ctx, `
			UPDATE core.outbox
			SET state = 'failed', attempts = $2, last_error = $3
			WHERE id = $1
		`, row.id, nextAttempts, networkError)
		return err
	}
	pause := basePause * time.Duration(uint(1)<<uint(nextAttempts-1))
	_, err := tx.Exec(ctx, `
		UPDATE core.outbox
		SET attempts = $2, next_attempt_at = $3, last_error = $4
		WHERE id = $1
	`, row.id, nextAttempts, now.Add(pause), networkError)
	return err
}

func postpone(ctx context.Context, tx pgx.Tx, id string, next time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE core.outbox
		SET next_attempt_at = $2
		WHERE id = $1
		  AND state = 'pending'
	`, id, next)
	return err
}

func lookupChat(ctx context.Context, tx pgx.Tx, kind, id string) (string, error) {
	query := `SELECT telegram_id FROM core.user_pii WHERE user_id = $1`
	if kind == "student" {
		query = `SELECT telegram_id FROM core.student_pii WHERE student_id = $1`
	}
	var chat string
	err := tx.QueryRow(ctx, query, id).Scan(&chat)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return chat, err
}

func lookupDisplayName(ctx context.Context, tx pgx.Tx, studentID string) (string, error) {
	if studentID == "" {
		return "", nil
	}
	var name string
	err := tx.QueryRow(ctx, `
		SELECT display_name FROM core.student_pii WHERE student_id = $1
	`, studentID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

func rateKey(chat, kind, id string) string {
	if chat != "" {
		return "c:" + chat
	}
	return "r:" + kind + ":" + id
}
