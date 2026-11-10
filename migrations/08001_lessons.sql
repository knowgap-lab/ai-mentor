-- +goose Up
-- Расписание и уроки (LS-R1…LS-R24).
-- core.journal_event не меняется: события — новые строки.
-- core.homework_draft уже есть: черновик «задача в ДЗ» — строка этой таблицы.
-- Снимок урока пишется один раз и этой миграцией не защищается от приложения:
-- приложение само не обновляет уже записанный snapshot.

CREATE TABLE core.study_plan (
    student_id text NOT NULL REFERENCES core.app_user (id),
    subject text NOT NULL,
    exam_at timestamptz NOT NULL,
    catalog_version integer NOT NULL,
    PRIMARY KEY (student_id, subject),
    CONSTRAINT study_plan_version_chk CHECK (catalog_version >= 1)
);

CREATE TABLE core.schedule_rule (
    student_id text NOT NULL,
    subject text NOT NULL,
    weekdays integer[] NOT NULL,
    start_time text NOT NULL,
    duration_min integer NOT NULL,
    PRIMARY KEY (student_id, subject),
    FOREIGN KEY (student_id, subject) REFERENCES core.study_plan (student_id, subject),
    CONSTRAINT schedule_rule_duration_chk CHECK (duration_min >= 1),
    CONSTRAINT schedule_rule_weekdays_chk CHECK (cardinality(weekdays) >= 1)
);

CREATE TABLE core.lesson (
    id text PRIMARY KEY,
    student_id text NOT NULL,
    subject text NOT NULL,
    place_no integer NOT NULL,
    title text NOT NULL,
    kind text NOT NULL,
    template_id text NOT NULL,
    state text NOT NULL,
    planned_week text NOT NULL DEFAULT '',
    starts_at timestamptz NULL,
    moved boolean NOT NULL DEFAULT false,
    added boolean NOT NULL DEFAULT false,
    no_blocks boolean NOT NULL DEFAULT false,
    issued boolean NOT NULL DEFAULT false,
    template jsonb NOT NULL,
    snapshot jsonb NULL,
    UNIQUE (student_id, subject, place_no),
    FOREIGN KEY (student_id, subject) REFERENCES core.study_plan (student_id, subject),
    CONSTRAINT lesson_state_chk CHECK (state IN ('planned', 'frozen', 'held', 'skipped')),
    CONSTRAINT lesson_place_chk CHECK (place_no >= 1)
);

CREATE INDEX lesson_student_subject_idx ON core.lesson (student_id, subject, place_no);

CREATE TABLE core.lesson_mark (
    lesson_id text PRIMARY KEY REFERENCES core.lesson (id),
    last_theory_block text NULL,
    practice_stage_id text NULL,
    practice_position integer NULL,
    coverage text NULL,
    CONSTRAINT lesson_mark_coverage_chk CHECK (coverage IS NULL OR coverage IN ('all', 'none')),
    CONSTRAINT lesson_mark_practice_chk CHECK (
        (practice_stage_id IS NULL AND practice_position IS NULL)
        OR (practice_stage_id IS NOT NULL AND practice_position >= 1)
    )
);

CREATE TABLE core.lesson_carry (
    to_lesson_id text PRIMARY KEY REFERENCES core.lesson (id),
    from_lesson_id text NOT NULL REFERENCES core.lesson (id),
    blocks jsonb NOT NULL,
    practice jsonb NOT NULL,
    minutes integer NOT NULL,
    CONSTRAINT lesson_carry_minutes_chk CHECK (minutes >= 0)
);

CREATE TABLE core.variant_pool (
    student_id text NOT NULL REFERENCES core.app_user (id),
    variant_id text NOT NULL,
    type_id text NOT NULL,
    seen boolean NOT NULL,
    reserve boolean NOT NULL,
    PRIMARY KEY (student_id, variant_id)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.study_plan TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.schedule_rule TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.lesson TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.lesson_mark TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.lesson_carry TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.variant_pool TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
