package notify

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

const codeParamsInvalid = "outbox_params_invalid"

// Enqueue пишет сообщение в core.outbox внутри tx.
// Повтор DedupKey не меняет первую строку и возвращает nil.
// Запрещённые ключи params не создают строку.
func Enqueue(ctx context.Context, tx pgx.Tx, msg Message) error {
	if forbiddenParam(msg.Params) {
		return &Error{Code: codeParamsInvalid}
	}
	raw, err := paramsJSON(msg.Params)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.outbox (
			id, kind, payload, dedup_key, recipient_kind, recipient_id,
			template, params, state, attempts, next_attempt_at, created_at
		) VALUES (
			$1, $2, $3::jsonb, $4, $5, $6,
			$7, $3::jsonb, 'pending', 0,
			COALESCE($8::timestamptz, now()),
			COALESCE($9::timestamptz, now())
		)
		ON CONFLICT (dedup_key) DO NOTHING
	`, msg.ID, msg.Kind, raw, msg.DedupKey, msg.RecipientKind, msg.RecipientID,
		msg.Template, nullableTime(msg.NextAttemptAt), nullableTime(msg.CreatedAt))
	return err
}

// CancelPending переводит в cancelled ожидающие сообщения этому ученику
// и сообщения, в params которых указан его student_id.
// Уже отправленные и сорвавшиеся строки не меняются.
// Строка, записанная после этого вызова, остаётся pending.
func CancelPending(ctx context.Context, tx pgx.Tx, studentID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE core.outbox
		SET state = 'cancelled'
		WHERE state = 'pending'
		  AND (
		      (recipient_kind = 'student' AND recipient_id = $1)
		      OR params->>'student_id' = $1
		  )
	`, studentID)
	return err
}

func forbiddenParam(params map[string]any) bool {
	if len(params) == 0 {
		return false
	}
	for _, key := range []string{"display_name", "telegram_id", "name", "text"} {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

func paramsJSON(params map[string]any) (string, error) {
	if params == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}
