-- +goose Up
-- Журнал попыток (AT-R1…AT-R25). Таблицы attempts, attempt_verdict,
-- journal_event и observation — только добавление: у core_app нет UPDATE и DELETE.
-- student_pii и student_pseudonym нужны обезличиванию (AT-R25); своей миграции
-- приватности в этом выпуске ещё нет.

CREATE SEQUENCE core.journal_seq AS bigint;

CREATE TABLE core.assignment (
    id text PRIMARY KEY,
    student_id text NOT NULL,
    state text NOT NULL,
    kind text NOT NULL,
    catalog_version integer NOT NULL,
    deadline timestamptz NULL,
    place text NOT NULL,
    snapshot jsonb NOT NULL,
    CONSTRAINT assignment_state_chk CHECK (state IN ('draft', 'issued', 'submitted', 'withdrawn')),
    CONSTRAINT assignment_kind_chk CHECK (kind IN ('homework', 'checkpoint', 'mock', 'diagnostics')),
    CONSTRAINT assignment_place_chk CHECK (place IN ('home', 'lesson'))
);

CREATE TABLE core.assignment_item (
    assignment_id text NOT NULL REFERENCES core.assignment (id),
    item_id text NOT NULL,
    state text NOT NULL,
    skipped boolean NOT NULL DEFAULT false,
    max_points integer NOT NULL,
    PRIMARY KEY (assignment_id, item_id),
    CONSTRAINT assignment_item_state_chk CHECK (state IN ('open', 'final')),
    CONSTRAINT assignment_item_max_chk CHECK (max_points >= 1)
);

CREATE TABLE core.attempts (
    id text PRIMARY KEY,
    assignment_id text NOT NULL,
    item_id text NOT NULL,
    raw_input text NOT NULL,
    journal_seq bigint NOT NULL UNIQUE DEFAULT nextval('core.journal_seq'),
    source text NOT NULL,
    place text NOT NULL,
    attempt_no integer NOT NULL,
    CONSTRAINT attempts_item_fk FOREIGN KEY (assignment_id, item_id)
        REFERENCES core.assignment_item (assignment_id, item_id),
    CONSTRAINT attempts_source_chk CHECK (source IN ('homework', 'checkpoint', 'mock', 'diagnostics', 'rt')),
    CONSTRAINT attempts_place_chk CHECK (place IN ('home', 'lesson')),
    CONSTRAINT attempts_no_chk CHECK (attempt_no >= 1)
);

CREATE TABLE core.attempt_verdict (
    attempt_id text NOT NULL REFERENCES core.attempts (id),
    seq integer NOT NULL,
    verdict text NOT NULL,
    points integer NOT NULL,
    reason text NULL,
    PRIMARY KEY (attempt_id, seq),
    CONSTRAINT attempt_verdict_seq_chk CHECK (seq >= 1),
    CONSTRAINT attempt_verdict_verdict_chk CHECK (verdict IN ('correct', 'partial', 'wrong')),
    CONSTRAINT attempt_verdict_points_chk CHECK (points >= 0)
);

CREATE TABLE core.journal_event (
    journal_seq bigint PRIMARY KEY DEFAULT nextval('core.journal_seq'),
    kind text NOT NULL,
    payload jsonb NOT NULL,
    at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.observation (
    assignment_id text NOT NULL,
    item_id text NOT NULL,
    revision integer NOT NULL,
    first_attempt_points integer NOT NULL,
    total_points integer NOT NULL,
    max_points integer NOT NULL,
    PRIMARY KEY (assignment_id, item_id, revision),
    CONSTRAINT observation_item_fk FOREIGN KEY (assignment_id, item_id)
        REFERENCES core.assignment_item (assignment_id, item_id),
    CONSTRAINT observation_rev_chk CHECK (revision >= 1)
);

CREATE TABLE core.dispute (
    assignment_id text NOT NULL,
    item_id text NOT NULL,
    state text NOT NULL,
    reason text NOT NULL,
    body text NULL,
    resolved_by jsonb NULL,
    PRIMARY KEY (assignment_id, item_id),
    CONSTRAINT dispute_item_fk FOREIGN KEY (assignment_id, item_id)
        REFERENCES core.assignment_item (assignment_id, item_id),
    CONSTRAINT dispute_state_chk CHECK (state IN ('open', 'resolved'))
);

CREATE TABLE core.homework_draft (
    id text PRIMARY KEY,
    lesson_id text NOT NULL UNIQUE,
    items jsonb NOT NULL,
    deadline text NOT NULL DEFAULT ''
);

CREATE TABLE core.outbox (
    id text PRIMARY KEY,
    kind text NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE core.student_pii (
    student_id text PRIMARY KEY,
    display_name text NOT NULL,
    telegram_id text NOT NULL,
    birth_date date NULL
);

CREATE TABLE core.student_pseudonym (
    student_id text PRIMARY KEY,
    code text NOT NULL UNIQUE
);

GRANT USAGE, SELECT ON SEQUENCE core.journal_seq TO core_app;

GRANT SELECT, INSERT ON TABLE core.attempts TO core_app;
GRANT SELECT, INSERT ON TABLE core.attempt_verdict TO core_app;
GRANT SELECT, INSERT ON TABLE core.journal_event TO core_app;
GRANT SELECT, INSERT ON TABLE core.observation TO core_app;

REVOKE UPDATE, DELETE ON TABLE core.attempts FROM core_app;
REVOKE UPDATE, DELETE ON TABLE core.attempt_verdict FROM core_app;
REVOKE UPDATE, DELETE ON TABLE core.journal_event FROM core_app;
REVOKE UPDATE, DELETE ON TABLE core.observation FROM core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
