-- +goose Up
-- Согласие (PV-R8, PV-R10…PV-R13, PV-R17, PV-R18).
-- Сырого токена нет: в token_hash только SHA-256.
-- Запись согласия не ссылается на student_pii: после обезличивания она остаётся (PV-R8).
-- ПДн по-прежнему только в core.student_pii (PV-R1), эту таблицу миграция не меняет.

CREATE TABLE core.consent_link (
    token_hash bytea PRIMARY KEY,
    student_id text NOT NULL,
    kind text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT consent_link_hash_len_chk CHECK (octet_length(token_hash) = 32),
    CONSTRAINT consent_link_kind_chk CHECK (kind IN ('parent', 'student'))
);

CREATE INDEX consent_link_student_idx ON core.consent_link (student_id);

CREATE TABLE core.consent (
    student_id text PRIMARY KEY,
    version integer NOT NULL,
    state text NOT NULL,
    granted_at timestamptz NOT NULL,
    revoked_at timestamptz NULL,
    anonymise_at timestamptz NULL,
    CONSTRAINT consent_version_chk CHECK (version >= 1),
    CONSTRAINT consent_state_chk CHECK (state IN ('active', 'revoked', 'outdated', 'needs_personal'))
);

REVOKE ALL ON TABLE core.consent_link FROM PUBLIC;
REVOKE ALL ON TABLE core.consent FROM PUBLIC;
REVOKE ALL ON TABLE core.consent_link FROM reader;
REVOKE ALL ON TABLE core.consent FROM reader;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.consent_link TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.consent TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
