package dbtest_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
)

func TestMigrationSchemasAndReaderCatalogInsert(t *testing.T) {
	dsn := dbtest.Start(t)
	if strings.Contains(dsn, ":") && strings.Contains(dsn, "@") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if u.User != nil {
			if _, ok := u.User.Password(); ok {
				t.Fatal("DSN contains a password")
			}
		}
		if !strings.HasSuffix(u.Path, "/ai_mentor_test") {
			t.Fatalf("database = %s", u.Path)
		}
	}
	runMigrate(t, dsn)
	runMigrate(t, dsn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	var current string
	if err := conn.QueryRow(ctx, `SELECT current_user`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	schemas := []string{"catalog", "core", "compute", "queue"}
	rows, err := conn.Query(ctx, `
		SELECT nspname, pg_get_userbyid(nspowner)
		FROM pg_namespace
		WHERE nspname = ANY($1)
		ORDER BY nspname
	`, schemas)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for rows.Next() {
		var name, owner string
		if err := rows.Scan(&name, &owner); err != nil {
			t.Fatal(err)
		}
		found[name] = owner
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(found) != len(schemas) {
		t.Fatalf("schemas = %v", found)
	}
	for _, name := range schemas {
		owner, ok := found[name]
		if !ok {
			t.Fatalf("schema %s missing", name)
		}
		if owner != current || owner == "loader" || owner == "core_app" || owner == "reader" {
			t.Fatalf("schema %s owner = %s, current_user = %s", name, owner, current)
		}
		for _, role := range []string{"loader", "core_app", "reader"} {
			var create bool
			if err := conn.QueryRow(ctx, `SELECT has_schema_privilege($1, $2, 'CREATE')`, role, name).Scan(&create); err != nil {
				t.Fatal(err)
			}
			if create {
				t.Fatalf("%s has CREATE on %s", role, name)
			}
		}
	}
	usage := []struct {
		role   string
		schema string
		want   bool
	}{
		{"loader", "catalog", true},
		{"loader", "core", false},
		{"loader", "compute", false},
		{"loader", "queue", false},
		{"core_app", "catalog", false},
		{"core_app", "core", true},
		{"core_app", "compute", false},
		{"core_app", "queue", false},
		{"reader", "catalog", true},
		{"reader", "core", true},
		{"reader", "compute", true},
		{"reader", "queue", true},
	}
	for _, check := range usage {
		var got bool
		if err := conn.QueryRow(ctx, `SELECT has_schema_privilege($1, $2, 'USAGE')`, check.role, check.schema).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Fatalf("%s USAGE on %s = %v", check.role, check.schema, got)
		}
	}

	roleRows, err := conn.Query(ctx, `
		SELECT rolname, rolcanlogin, rolpassword IS NULL, rolsuper
		FROM pg_authid
		WHERE rolname IN ('loader', 'core_app', 'reader')
		ORDER BY rolname
	`)
	if err != nil {
		t.Fatal(err)
	}
	type roleInfo struct {
		login      bool
		noPassword bool
		super      bool
	}
	roles := map[string]roleInfo{}
	for roleRows.Next() {
		var name string
		var info roleInfo
		if err := roleRows.Scan(&name, &info.login, &info.noPassword, &info.super); err != nil {
			t.Fatal(err)
		}
		roles[name] = info
	}
	if err := roleRows.Err(); err != nil {
		t.Fatal(err)
	}
	roleRows.Close()
	for _, name := range []string{"loader", "core_app", "reader"} {
		info, ok := roles[name]
		if !ok {
			t.Fatalf("role %s missing", name)
		}
		if info.login || !info.noPassword || info.super {
			t.Fatalf("role %s = %+v", name, info)
		}
	}

	var versionOwner string
	if err := conn.QueryRow(ctx, `
		SELECT pg_get_userbyid(c.relowner)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = 'goose_db_version'
	`).Scan(&versionOwner); err != nil {
		t.Fatal(err)
	}
	if versionOwner != current {
		t.Fatalf("goose_db_version owner = %s", versionOwner)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback: %v", err)
		}
	}()
	for _, stmt := range []string{
		`CREATE TABLE catalog.probe (id integer)`,
		`CREATE TABLE core.probe (id integer)`,
		`CREATE TABLE compute.probe (id integer)`,
		`CREATE TABLE queue.probe (id integer)`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO core.probe (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		role  string
		table string
		priv  string
		want  bool
	}{
		{"reader", "catalog.probe", "SELECT", true},
		{"reader", "catalog.probe", "INSERT", false},
		{"reader", "catalog.probe", "UPDATE", false},
		{"reader", "catalog.probe", "DELETE", false},
		{"reader", "catalog.probe", "TRUNCATE", false},
		{"reader", "core.probe", "SELECT", true},
		{"reader", "core.probe", "INSERT", false},
		{"reader", "compute.probe", "SELECT", true},
		{"reader", "compute.probe", "INSERT", false},
		{"reader", "queue.probe", "SELECT", true},
		{"reader", "queue.probe", "INSERT", false},
		{"loader", "catalog.probe", "SELECT", true},
		{"loader", "catalog.probe", "INSERT", true},
		{"loader", "catalog.probe", "UPDATE", false},
		{"loader", "catalog.probe", "DELETE", false},
		{"loader", "catalog.probe", "TRUNCATE", false},
		{"loader", "core.probe", "SELECT", false},
		{"loader", "core.probe", "INSERT", false},
		{"core_app", "core.probe", "SELECT", true},
		{"core_app", "core.probe", "INSERT", true},
		{"core_app", "core.probe", "UPDATE", true},
		{"core_app", "core.probe", "DELETE", true},
		{"core_app", "core.probe", "TRUNCATE", false},
		{"core_app", "catalog.probe", "SELECT", false},
		{"core_app", "catalog.probe", "INSERT", false},
	}
	for _, check := range checks {
		var got bool
		if err := tx.QueryRow(ctx, `SELECT has_table_privilege($1, $2, $3)`, check.role, check.table, check.priv).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Fatalf("%s %s on %s = %v", check.role, check.priv, check.table, got)
		}
	}

	expectAsRole(t, ctx, tx, "reader", `INSERT INTO catalog.probe (id) VALUES (1)`, false, "table")
	expectAsRole(t, ctx, tx, "loader", `INSERT INTO catalog.probe (id) VALUES (1)`, true, "")
	expectAsRole(t, ctx, tx, "loader", `UPDATE catalog.probe SET id = 2`, false, "table")
	expectAsRole(t, ctx, tx, "loader", `TRUNCATE catalog.probe`, false, "table")
	expectAsRole(t, ctx, tx, "loader", `INSERT INTO core.probe (id) VALUES (1)`, false, "schema")
	expectAsRole(t, ctx, tx, "core_app", `INSERT INTO core.probe (id) VALUES (2)`, true, "")
	expectAsRole(t, ctx, tx, "core_app", `UPDATE core.probe SET id = 2 WHERE id = 1`, true, "")
	expectAsRole(t, ctx, tx, "core_app", `DELETE FROM core.probe WHERE id = 1`, true, "")
	expectAsRole(t, ctx, tx, "core_app", `TRUNCATE core.probe`, false, "table")
	expectAsRole(t, ctx, tx, "core_app", `INSERT INTO catalog.probe (id) VALUES (1)`, false, "schema")
}

func expectAsRole(t *testing.T, ctx context.Context, tx pgx.Tx, role, stmt string, wantOK bool, deny string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SAVEPOINT probe`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('role', $1, true)`, role); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(ctx, stmt)
	if _, rollErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT probe`); rollErr != nil {
		t.Fatal(rollErr)
	}
	if wantOK {
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		return
	}
	if err == nil {
		t.Fatalf("%s statement succeeded", role)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" || !strings.Contains(pgErr.Message, deny) {
		t.Fatalf("%s: %v", role, err)
	}
}

func runMigrate(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "-mod=readonly", "./cmd/migrate")
	cmd.Dir = filepath.Join(repoRoot(t), "core")
	cmd.Env = withoutEnv(os.Environ(), "DATABASE_URL")
	cmd.Env = append(cmd.Env, "DATABASE_URL="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		sqlPath := filepath.Join(dir, "migrations", "00001_schemas.sql")
		modPath := filepath.Join(dir, "core", "go.mod")
		if isFile(sqlPath) && isFile(modPath) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
