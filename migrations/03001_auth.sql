-- +goose Up
-- Локальный вход: учётная запись, ПДн, сеанс, ключ идемпотентности.
-- reader не получает прав на core.user_pii (ПДн).

CREATE TABLE core.app_user (
    id text PRIMARY KEY,
    role text NOT NULL,
    owner_tutor_id text NULL,
    status text NOT NULL,
    CONSTRAINT app_user_role_chk CHECK (role IN ('student', 'tutor')),
    CONSTRAINT app_user_owner_fk FOREIGN KEY (owner_tutor_id) REFERENCES core.app_user (id)
);

CREATE TABLE core.user_pii (
    user_id text PRIMARY KEY REFERENCES core.app_user (id),
    telegram_id text NOT NULL
);

CREATE INDEX user_pii_telegram_id_idx ON core.user_pii (telegram_id);

CREATE TABLE core.session (
    token_hash bytea PRIMARY KEY,
    user_id text NOT NULL REFERENCES core.app_user (id),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz NULL
);

CREATE INDEX session_user_created_idx ON core.session (user_id, created_at);

CREATE TABLE core.idempotency (
    user_id text NOT NULL REFERENCES core.app_user (id),
    idem_key text NOT NULL,
    response bytea NOT NULL,
    PRIMARY KEY (user_id, idem_key)
);

REVOKE ALL ON TABLE core.user_pii FROM PUBLIC;
REVOKE ALL ON TABLE core.user_pii FROM reader;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.app_user TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.user_pii TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.session TO core_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.idempotency TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
