-- +goose Up
-- Приглашение в Telegram (AU-R9, AU-R10).
-- Сырого токена нет: в token_hash только SHA-256.
-- Профиль виджета сюда не пишется: наружу из входа годится только telegram_id ученика.

CREATE TABLE core.telegram_invite (
    token_hash bytea PRIMARY KEY,
    student_id text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz NULL,
    CONSTRAINT telegram_invite_hash_len_chk CHECK (octet_length(token_hash) = 32)
);

CREATE INDEX telegram_invite_student_idx ON core.telegram_invite (student_id);

REVOKE ALL ON TABLE core.telegram_invite FROM PUBLIC;
REVOKE ALL ON TABLE core.telegram_invite FROM reader;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.telegram_invite TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
