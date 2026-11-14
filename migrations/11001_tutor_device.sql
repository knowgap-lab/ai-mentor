-- +goose Up
-- Сеанс может ждать подтверждения устройства (AU-R29).
-- Устройство репетитора: в базе только SHA-256 cookie и дата подтверждения (AU-R28).

ALTER TABLE core.session
    ADD COLUMN state text NOT NULL DEFAULT 'active';

ALTER TABLE core.session
    ADD CONSTRAINT session_state_chk CHECK (state IN ('active', 'pending'));

CREATE TABLE core.tutor_device (
    token_hash bytea PRIMARY KEY,
    user_id text NOT NULL REFERENCES core.app_user (id),
    confirmed_at timestamptz NULL,
    CONSTRAINT tutor_device_hash_len_chk CHECK (octet_length(token_hash) = 32)
);

CREATE INDEX tutor_device_user_idx ON core.tutor_device (user_id);

REVOKE ALL ON TABLE core.tutor_device FROM PUBLIC;
REVOKE ALL ON TABLE core.tutor_device FROM reader;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE core.tutor_device TO core_app;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
