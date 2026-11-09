-- +goose Up
-- Фундамент 00001–00099: схемы и роли. Пароль роли не задаётся, LOGIN нет.
-- Владелец объектов — пользователь, которым goose подключается, не роль приложения.
-- USAGE только на схемах, где у роли есть права на будущие таблицы. CREATE нет.

CREATE SCHEMA catalog;
CREATE SCHEMA core;
CREATE SCHEMA compute;
CREATE SCHEMA queue;

REVOKE ALL ON SCHEMA catalog FROM PUBLIC;
REVOKE ALL ON SCHEMA core FROM PUBLIC;
REVOKE ALL ON SCHEMA compute FROM PUBLIC;
REVOKE ALL ON SCHEMA queue FROM PUBLIC;

CREATE ROLE loader NOLOGIN;
CREATE ROLE core_app NOLOGIN;
CREATE ROLE reader NOLOGIN;

GRANT USAGE ON SCHEMA catalog TO loader;
GRANT USAGE ON SCHEMA core TO core_app;
GRANT USAGE ON SCHEMA catalog, core, compute, queue TO reader;

ALTER DEFAULT PRIVILEGES IN SCHEMA catalog
    GRANT SELECT, INSERT ON TABLES TO loader;

ALTER DEFAULT PRIVILEGES IN SCHEMA core
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO core_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA catalog
    GRANT SELECT ON TABLES TO reader;

ALTER DEFAULT PRIVILEGES IN SCHEMA core
    GRANT SELECT ON TABLES TO reader;

ALTER DEFAULT PRIVILEGES IN SCHEMA compute
    GRANT SELECT ON TABLES TO reader;

ALTER DEFAULT PRIVILEGES IN SCHEMA queue
    GRANT SELECT ON TABLES TO reader;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'expand-only migration (D127)';
END
$$;
-- +goose StatementEnd
