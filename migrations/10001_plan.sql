-- +goose Up
-- План 0 и правки репетитора (PL-R1…PL-R11).
-- Версия 0 — снимок: UPDATE и DELETE отклоняет триггер.
-- Темы и предпосылки лежат отдельно: core.lesson.template остаётся массивом этапов.

CREATE TABLE core.plan_snapshot (
    student_id text NOT NULL,
    subject text NOT NULL,
    version integer NOT NULL,
    source_hashes jsonb NOT NULL,
    PRIMARY KEY (student_id, subject, version),
    FOREIGN KEY (student_id, subject) REFERENCES core.study_plan (student_id, subject),
    CONSTRAINT plan_snapshot_version_chk CHECK (version = 0)
);

CREATE TABLE core.plan_snapshot_lesson (
    student_id text NOT NULL,
    subject text NOT NULL,
    version integer NOT NULL,
    place_no integer NOT NULL,
    lesson_id text NOT NULL,
    template_id text NOT NULL,
    planned_week text NOT NULL,
    PRIMARY KEY (student_id, subject, version, place_no),
    FOREIGN KEY (student_id, subject, version)
        REFERENCES core.plan_snapshot (student_id, subject, version),
    CONSTRAINT plan_snapshot_lesson_place_chk CHECK (place_no >= 1)
);

CREATE TABLE core.lesson_topic (
    lesson_id text NOT NULL REFERENCES core.lesson (id),
    position integer NOT NULL,
    topic text NOT NULL,
    PRIMARY KEY (lesson_id, position),
    CONSTRAINT lesson_topic_position_chk CHECK (position >= 1)
);

CREATE TABLE core.topic_prereq (
    student_id text NOT NULL,
    subject text NOT NULL,
    topic text NOT NULL,
    prereq text NOT NULL,
    PRIMARY KEY (student_id, subject, topic, prereq),
    FOREIGN KEY (student_id, subject) REFERENCES core.study_plan (student_id, subject)
);

-- +goose StatementBegin
CREATE FUNCTION core.reject_plan0_change() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'plan 0 is immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER plan_snapshot_immutable
    BEFORE UPDATE OR DELETE ON core.plan_snapshot
    FOR EACH ROW
    EXECUTE FUNCTION core.reject_plan0_change();

CREATE TRIGGER plan_snapshot_lesson_immutable
    BEFORE UPDATE OR DELETE ON core.plan_snapshot_lesson
    FOR EACH ROW
    EXECUTE FUNCTION core.reject_plan0_change();

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.lesson_topic TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.topic_prereq TO core_app;
GRANT SELECT, INSERT ON TABLE core.plan_snapshot TO core_app;
GRANT SELECT, INSERT ON TABLE core.plan_snapshot_lesson TO core_app;
REVOKE UPDATE, DELETE ON TABLE core.plan_snapshot FROM core_app;
REVOKE UPDATE, DELETE ON TABLE core.plan_snapshot_lesson FROM core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
