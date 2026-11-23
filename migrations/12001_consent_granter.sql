-- +goose Up
-- Кто дал согласие и способ (PV-R8). NULL остаётся у строк, записанных раньше;
-- новая выдача заполняет оба столбца. CHECK пропускает NULL.

ALTER TABLE core.consent
    ADD COLUMN granted_by text,
    ADD COLUMN method text;

ALTER TABLE core.consent
    ADD CONSTRAINT consent_granted_by_chk CHECK (granted_by IN ('parent', 'student')),
    ADD CONSTRAINT consent_method_chk CHECK (method = 'link_button');

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
