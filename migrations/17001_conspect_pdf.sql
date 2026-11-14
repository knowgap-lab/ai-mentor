-- +goose Up
-- Первый PDF конспекта (CN-R12, D301). Ключ — урок, версия каталога снимка
-- и роль: ученик и репетитор видят разный состав (CN-R13, CN-R14a).

CREATE TABLE core.conspect_pdf (
    lesson_id text NOT NULL,
    catalog_version integer NOT NULL,
    audience text NOT NULL,
    pdf bytea NOT NULL,
    PRIMARY KEY (lesson_id, catalog_version, audience),
    CONSTRAINT conspect_pdf_audience_chk CHECK (audience IN ('student', 'tutor'))
);

GRANT SELECT, INSERT ON TABLE core.conspect_pdf TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
