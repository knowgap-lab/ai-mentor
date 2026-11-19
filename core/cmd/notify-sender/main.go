// Команда notify-sender забирает созревшие строки core.outbox и отдаёт их
// локальному отправителю. Отправитель пишет доставку в stdout и не открывает
// сеть: вызова Telegram здесь нет (NT-R1, отдельный цикл после коммита).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/notify"
)

func main() {
	os.Exit(run(os.Getenv("DATABASE_URL"), os.Stdout, os.Stderr))
}

func run(databaseURL string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if databaseURL == "" {
		return fail(stderr, "DATABASE_URL is required\n")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fail(stderr, "notify-sender: %v\n", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var now time.Time
	if err := conn.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return fail(stderr, "notify-sender: %v\n", err)
	}
	tr := &recorder{got: []notify.Delivery{}}
	if err := notify.Dispatch(ctx, conn, now, tr); err != nil {
		return fail(stderr, "notify-sender: %v\n", err)
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(tr.got); err != nil {
		return fail(stderr, "notify-sender: %v\n", err)
	}
	return 0
}

func fail(stderr io.Writer, format string, args ...any) int {
	if _, err := fmt.Fprintf(stderr, format, args...); err != nil {
		return 1
	}
	return 1
}

// recorder — тестовый отправитель. Send запоминает доставку и возвращает nil.
type recorder struct {
	got []notify.Delivery
}

func (r *recorder) Send(_ context.Context, d notify.Delivery) error {
	r.got = append(r.got, d)
	return nil
}
