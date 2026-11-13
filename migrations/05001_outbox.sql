-- +goose Up
-- Колонки исходящих сообщений (NT-R2). Таблица core.outbox уже есть
-- (id, kind, payload); здесь только новые поля. NULL в dedup_key
-- различимы, поэтому прежний INSERT трёх колонок остаётся допустимым.

ALTER TABLE core.outbox
    ADD COLUMN dedup_key text,
    ADD COLUMN recipient_kind text,
    ADD COLUMN recipient_id text,
    ADD COLUMN template text,
    ADD COLUMN params jsonb,
    ADD COLUMN state text DEFAULT 'pending',
    ADD COLUMN attempts integer DEFAULT 0,
    ADD COLUMN next_attempt_at timestamptz,
    ADD COLUMN created_at timestamptz,
    ADD COLUMN sent_at timestamptz,
    ADD COLUMN last_error text;

ALTER TABLE core.outbox
    ADD CONSTRAINT outbox_dedup_key_key UNIQUE (dedup_key);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
