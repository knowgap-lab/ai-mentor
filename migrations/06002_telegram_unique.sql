-- +goose Up
-- Непустой telegram_id не повторяется в ПДн (AU-R16).
-- Пустая строка индекс не занимает.

CREATE UNIQUE INDEX user_pii_telegram_id_uniq
    ON core.user_pii (telegram_id)
    WHERE telegram_id <> '';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'core'
          AND table_name = 'student_pii'
          AND column_name = 'telegram_id'
    ) THEN
        CREATE UNIQUE INDEX student_pii_telegram_id_uniq
            ON core.student_pii (telegram_id)
            WHERE telegram_id IS NOT NULL AND telegram_id <> '';
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
