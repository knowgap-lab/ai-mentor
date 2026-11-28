-- +goose Up
-- Приглашение репетитора привязать Telegram (AU-R32, AU-R9, AU-R10).
-- student_id остаётся для учеников; tutor_id — для репетитора.

ALTER TABLE core.telegram_invite
    ALTER COLUMN student_id DROP NOT NULL;

ALTER TABLE core.telegram_invite
    ADD COLUMN tutor_id text NULL REFERENCES core.app_user (id);

ALTER TABLE core.telegram_invite
    ADD CONSTRAINT telegram_invite_target_chk CHECK (
        (student_id IS NOT NULL AND tutor_id IS NULL)
        OR (student_id IS NULL AND tutor_id IS NOT NULL)
    );

CREATE INDEX telegram_invite_tutor_idx ON core.telegram_invite (tutor_id);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd