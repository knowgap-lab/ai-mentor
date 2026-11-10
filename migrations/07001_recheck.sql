-- +goose Up
-- Пакет перепроверки (RC-R2…RC-R7, RC-R13).
-- Строка пакета — рабочее состояние: её можно обновлять.
-- Журнал попыток эта миграция не меняет и по-прежнему только пополняется.

CREATE TABLE core.recheck_batch (
    correction_id integer PRIMARY KEY,
    state text NOT NULL,
    total integer NOT NULL,
    processed integer NOT NULL,
    changed integer NOT NULL,
    skipped_manual integer NOT NULL,
    cursor text NULL,
    variants text[] NOT NULL,
    started_at timestamptz NULL,
    finished_at timestamptz NULL,
    CONSTRAINT recheck_batch_state_chk CHECK (state IN ('pending', 'running', 'done', 'failed')),
    CONSTRAINT recheck_batch_counts_chk CHECK (
        total >= 0
        AND processed >= 0
        AND changed >= 0
        AND skipped_manual >= 0
    )
);

GRANT SELECT, INSERT, UPDATE ON TABLE core.recheck_batch TO core_app;
REVOKE DELETE ON TABLE core.recheck_batch FROM core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
