-- +goose Up
-- Конверт журнала: ученик и актёр лежат рядом с kind и payload.
-- Счётчик лимита ответов не является событием схемы, поэтому он не в журнале.

ALTER TABLE core.journal_event
    ADD COLUMN student_id text NULL,
    ADD COLUMN actor_role text NULL,
    ADD COLUMN actor_id text NULL;

ALTER TABLE core.journal_event
    ADD CONSTRAINT journal_event_actor_role_chk
    CHECK (actor_role IS NULL OR actor_role IN ('student', 'tutor', 'system'));

CREATE TABLE core.answer_rate (
    student_id text NOT NULL,
    at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT ON TABLE core.answer_rate TO core_app;
REVOKE UPDATE, DELETE ON TABLE core.answer_rate FROM core_app;

INSERT INTO core.answer_rate (student_id, at)
SELECT payload->>'student_id', at
FROM core.journal_event
WHERE kind = 'answer_request'
  AND COALESCE(payload->>'student_id', '') <> '';

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
