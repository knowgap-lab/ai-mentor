package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/knowgap-lab/ai-mentor/core/internal/telegram"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		usage(os.Stdout)
		return 0
	}
	if args[0] != "create" {
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		usage(os.Stderr)
		return 1
	}
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var tutorID string
	fs.StringVar(&tutorID, "tutor", "", "id учётной записи репетитора")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	tutorID = strings.TrimSpace(tutorID)
	if tutorID == "" {
		fmt.Fprintln(os.Stderr, "--tutor is required")
		return 1
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 1
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 1
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 1
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role, status string
	err = tx.QueryRow(ctx, `
		SELECT role, status FROM core.app_user WHERE id = $1 FOR UPDATE
	`, tutorID).Scan(&role, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.app_user (id, role, owner_tutor_id, status)
			VALUES ($1, 'tutor', NULL, 'active')
		`, tutorID); err != nil {
			fmt.Fprintf(os.Stderr, "create tutor: %v\n", err)
			return 1
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.user_pii (user_id, telegram_id)
			VALUES ($1, '')
		`, tutorID); err != nil {
			fmt.Fprintf(os.Stderr, "create tutor: %v\n", err)
			return 1
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 1
	} else if role != "tutor" {
		fmt.Fprintln(os.Stderr, "user exists but is not a tutor")
		return 1
	} else if status != "active" {
		fmt.Fprintln(os.Stderr, "tutor is not active")
		return 1
	}

	var bound string
	if err := tx.QueryRow(ctx, `
		SELECT telegram_id FROM core.user_pii WHERE user_id = $1
	`, tutorID).Scan(&bound); errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, '')
		`, tutorID); err != nil {
			fmt.Fprintf(os.Stderr, "create tutor: %v\n", err)
			return 1
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 1
	} else if strings.TrimSpace(bound) != "" {
		fmt.Fprintln(os.Stderr, "tutor already has telegram_id; use tutorctl rebind (not in this branch)")
		return 1
	}

	now := time.Now().UTC()
	svc, err := telegram.Open(ctx, telegram.Config{DSN: dsn})
	if err != nil {
		fmt.Fprintf(os.Stderr, "telegram: %v\n", err)
		return 1
	}
	defer func() { _ = svc.Close() }()
	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 1
	}
	token, err := svc.CreateTutor(ctx, tutorID, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invite: %v\n", err)
		return 1
	}
	origin := strings.TrimRight(os.Getenv("APP_ORIGIN"), "/")
	if origin == "" {
		origin = "http://127.0.0.1:8080"
	}
	fmt.Printf("tutor_id=%s\n", tutorID)
	fmt.Printf("invite_token=%s\n", token)
	fmt.Printf("invite_url=%s/invite/%s\n", origin, token)
	return 0
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  tutorctl create --tutor <id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Needs DATABASE_URL. Prints one-time invite_token to stdout (AU-R9).")
}