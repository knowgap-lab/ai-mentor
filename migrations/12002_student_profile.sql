-- +goose Up
-- Учебные сведения ученика, не ПДн (PV-R5). После обезличивания строка остаётся.
-- Предметы, вид и дата экзамена не пишутся в core.student_pii (PV-R1).

CREATE TABLE core.student_profile (
    student_id text PRIMARY KEY REFERENCES core.app_user (id),
    subjects jsonb NOT NULL DEFAULT '[]'::jsonb,
    exam_kind text NULL,
    exam_date date NULL,
    CONSTRAINT student_profile_exam_kind_chk CHECK (exam_kind IS NULL OR exam_kind IN ('ct', 'ce'))
);

REVOKE ALL ON TABLE core.student_profile FROM PUBLIC;
REVOKE ALL ON TABLE core.student_profile FROM reader;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.student_profile TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
