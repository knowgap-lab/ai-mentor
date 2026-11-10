-- +goose Up
-- Сборка ДЗ (HW-R1…HW-R20).
-- core.homework_draft уже есть: id, lesson_id, items, deadline.
-- Эта миграция только добавляет поля черновика.

ALTER TABLE core.homework_draft
    ADD COLUMN hints jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN discuss_topics jsonb NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
