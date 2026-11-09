package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv("DATABASE_URL"), os.Stdout, os.Stderr))
}

func run(args []string, databaseURL string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 || command != "up" {
		if _, err := fmt.Fprintf(stderr, "unknown command %q\n", command); err != nil {
			return 1
		}
		return 1
	}
	if databaseURL == "" {
		if _, err := fmt.Fprintln(stderr, "DATABASE_URL is required"); err != nil {
			return 1
		}
		return 1
	}
	if err := applyUp(context.Background(), databaseURL, stdout); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "migrate: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

func applyUp(ctx context.Context, databaseURL string, stdout io.Writer) (err error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); err == nil {
			err = closeErr
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	dir, err := locateMigrations()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := provider.Close(); err == nil {
			err = closeErr
		}
	}()
	results, err := provider.Up(ctx)
	if err != nil {
		return err
	}
	for _, result := range results {
		if _, err := fmt.Fprintln(stdout, result.String()); err != nil {
			return err
		}
	}
	return nil
}

func locateMigrations() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if dir, findErr := findMigrations(wd); findErr == nil {
		return dir, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("migrations directory not found")
	}
	dir, findErr := findMigrations(filepath.Dir(exe))
	if findErr != nil {
		return "", fmt.Errorf("migrations directory not found")
	}
	return dir, nil
}

func findMigrations(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, "migrations", "00001_schemas.sql")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return filepath.Join(dir, "migrations"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("migrations directory not found from %s", start)
		}
		dir = parent
	}
}
