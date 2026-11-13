package notify_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/notify"
)

func TestNT_R1_SameTransaction(t *testing.T) {
	conn, dsn := newDB(t)
	seedDemo(t, conn)
	ctx := context.Background()
	due := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insertJournal(t, tx, "assignment_submitted")
	if err := notify.Enqueue(ctx, tx, demoDue("r1-roll", "dedup-r1-roll", "запись", due)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if countOutbox(t, conn) != 0 {
		t.Fatal("после отката строка outbox осталась")
	}
	var journal int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM core.journal_event`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != 0 {
		t.Fatal("после отката событие журнала осталось")
	}

	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insertJournal(t, tx, "assignment_submitted")
	if err := notify.Enqueue(ctx, tx, demoDue("r1-open", "dedup-r1-open", "запись", due)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	other := openConn(t, dsn)
	tr := &scriptTransport{}
	dispatchAt(t, other, due, tr)
	if len(tr.got()) != 0 || countOutbox(t, other) != 0 {
		t.Fatal("незакоммиченная строка видна отправителю")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if countOutbox(t, conn) != 1 {
		t.Fatalf("строк %d", countOutbox(t, conn))
	}
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM core.journal_event WHERE kind = 'assignment_submitted'
	`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal < 1 {
		t.Fatal("нет события журнала в той же транзакции")
	}
	row := rowByID(t, conn, "r1-open")
	if row.State != "pending" || len(tr.got()) != 0 {
		t.Fatalf("после коммита state=%s, отправок %d", row.State, len(tr.got()))
	}

	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].Text != "запись" || got[0].ParseMode != "" || got[0].ChatID != tutorTelegram {
		t.Fatalf("доставка %+v", got)
	}
	row = rowByID(t, conn, "r1-open")
	if row.State != "sent" || row.SentAt == nil || !sameInstant(*row.SentAt, due) {
		t.Fatalf("state=%s sent_at=%v", row.State, row.SentAt)
	}
}

func TestNT_R2_ColumnsAndDefaults(t *testing.T) {
	conn, _ := newDB(t)
	ctx := context.Background()
	rows, err := conn.Query(ctx, `
		SELECT column_name, udt_name, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'core' AND table_name = 'outbox'
	`)
	if err != nil {
		t.Fatal(err)
	}
	type col struct {
		udt      string
		nullable string
		def      string
	}
	got := map[string]col{}
	for rows.Next() {
		var name, udt, nullable string
		var def *string
		if err := rows.Scan(&name, &udt, &nullable, &def); err != nil {
			t.Fatal(err)
		}
		item := col{udt: udt, nullable: nullable}
		if def != nil {
			item.def = *def
		}
		got[name] = item
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	want := map[string]string{
		"id": "text", "kind": "text", "payload": "jsonb", "dedup_key": "text",
		"recipient_kind": "text", "recipient_id": "text", "template": "text",
		"params": "jsonb", "state": "text", "attempts": "int4",
		"next_attempt_at": "timestamptz", "created_at": "timestamptz",
		"sent_at": "timestamptz", "last_error": "text",
	}
	for name, udt := range want {
		item, ok := got[name]
		if !ok || item.udt != udt {
			t.Fatalf("колонка %s: %+v", name, item)
		}
	}
	if got["sent_at"].nullable != "YES" || got["last_error"].nullable != "YES" {
		t.Fatalf("sent_at=%s last_error=%s", got["sent_at"].nullable, got["last_error"].nullable)
	}
	if !strings.Contains(got["state"].def, "pending") || !strings.Contains(got["attempts"].def, "0") {
		t.Fatalf("defaults state=%q attempts=%q", got["state"].def, got["attempts"].def)
	}

	var state string
	var attempts int
	var sentAt *time.Time
	var lastErr *string
	if err := conn.QueryRow(ctx, `
		INSERT INTO core.outbox (id, kind, payload)
		VALUES ('legacy-1', 'legacy', '{}'::jsonb)
		RETURNING state, attempts, sent_at, last_error
	`).Scan(&state, &attempts, &sentAt, &lastErr); err != nil {
		t.Fatalf("старый INSERT: %v", err)
	}
	if state != "pending" || attempts != 0 || sentAt != nil || lastErr != nil {
		t.Fatalf("state=%s attempts=%d sent=%v last=%v", state, attempts, sentAt, lastErr)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.outbox (id, kind, payload, dedup_key)
		VALUES ('u1', 'k', '{}'::jsonb, 'dup-key')
	`); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `
		INSERT INTO core.outbox (id, kind, payload, dedup_key)
		VALUES ('u2', 'k', '{}'::jsonb, 'dup-key')
	`)
	expectUnique(t, err)
}

func TestNT_R2_ContentUnchangedOnSend(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	at := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	due := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	msg := message("r2-body", "dedup-r2", "homework_submitted", "tutor", tutorID,
		"уведомление {student_id}", map[string]any{"student_id": studentID}, at, due)
	commitEnqueue(t, conn, msg)
	before := rowByID(t, conn, "r2-body")
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].Text != "уведомление "+studentID || got[0].ParseMode != "" {
		t.Fatalf("доставка %+v", got)
	}
	if got[0].ChatID != tutorTelegram || got[0].RecipientID != tutorID || got[0].RecipientKind != "tutor" {
		t.Fatalf("получатель %+v", got[0])
	}
	after := rowByID(t, conn, "r2-body")
	if after.State != "sent" || after.SentAt == nil || !sameInstant(*after.SentAt, due) {
		t.Fatalf("state=%s sent=%v", after.State, after.SentAt)
	}
	if after.LastError != nil {
		t.Fatalf("last_error %q", *after.LastError)
	}
	if after.ID != before.ID || after.Kind != before.Kind || after.DedupKey != before.DedupKey ||
		after.RecipientKind != before.RecipientKind || after.RecipientID != before.RecipientID ||
		after.Template != before.Template || after.Payload != before.Payload || after.Params != before.Params ||
		!sameInstant(after.CreatedAt, before.CreatedAt) || after.Template != msg.Template ||
		after.Kind != "homework_submitted" {
		t.Fatalf("тело изменилось: %+v", after)
	}
	again := &scriptTransport{}
	dispatchAt(t, conn, due.Add(time.Hour), again)
	if len(again.got()) != 0 {
		t.Fatal("повторная отправка уже отправленной строки")
	}
}

func TestNT_R2_RecipientKinds(t *testing.T) {
	conn, _ := newDB(t)
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	for _, kind := range []string{"tutor", "student", "ops"} {
		id := "kind-" + kind
		commitEnqueue(t, conn, message(id, "dedup-"+kind, "ops_alert", kind, "user-"+kind,
			"текст", map[string]any{"code": "disk"}, at, at))
		row := rowByID(t, conn, id)
		if row.RecipientKind != kind || row.RecipientID != "user-"+kind || row.State != "pending" || row.Attempts != 0 {
			t.Fatalf("%s: %+v", kind, row)
		}
	}
}

func TestNT_R3_SecondInsertDoesNothing(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	first := demoDue("r3-a", "dedup-r3", "первый", at)
	commitEnqueue(t, conn, first)
	second := demoDue("r3-b", "dedup-other", "другой", at)
	commitEnqueue(t, conn, second)
	if countOutbox(t, conn) != 2 {
		t.Fatalf("строк %d", countOutbox(t, conn))
	}

	again := first
	again.ID = "r3-a2"
	again.Template = "перезапись"
	again.Kind = "homework_issued"
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := notify.Enqueue(ctx, tx, again); err != nil {
		t.Fatalf("повтор: %v", err)
	}
	third := demoDue("r3-c", "dedup-r3-c", "третий", at)
	if err := notify.Enqueue(ctx, tx, third); err != nil {
		t.Fatalf("новый ключ: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if countOutbox(t, conn) != 3 {
		t.Fatalf("строк %d", countOutbox(t, conn))
	}
	row := rowByID(t, conn, "r3-a")
	if row.Template != "первый" || row.Kind != "homework_submitted" {
		t.Fatalf("первая строка изменилась: %+v", row)
	}
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM core.outbox WHERE id = 'r3-a2'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("повтор создал строку")
	}
}

func TestNT_R3_ConcurrentInsertDoesNothing(t *testing.T) {
	conn, dsn := newDB(t)
	seedDemo(t, conn)
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	msgs := []notify.Message{
		demoDue("c1", "same-key", "альфа", at),
		demoDue("c2", "same-key", "бета", at),
	}
	conns := []*pgx.Conn{openConn(t, dsn), openConn(t, dsn)}
	var wg sync.WaitGroup
	errCh := make(chan error, len(msgs))
	for i, msg := range msgs {
		wg.Add(1)
		go func(c *pgx.Conn, msg notify.Message) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tx, err := c.Begin(ctx)
			if err != nil {
				errCh <- err
				return
			}
			if err := notify.Enqueue(ctx, tx, msg); err != nil {
				_ = tx.Rollback(ctx)
				errCh <- err
				return
			}
			errCh <- tx.Commit(ctx)
		}(conns[i], msg)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("параллельная вставка: %v", err)
		}
	}
	if countOutbox(t, conn) != 1 {
		t.Fatalf("строк %d", countOutbox(t, conn))
	}
	var id, template string
	if err := conn.QueryRow(context.Background(), `SELECT id, template FROM core.outbox`).Scan(&id, &template); err != nil {
		t.Fatal(err)
	}
	if (id == "c1" && template != "альфа") || (id == "c2" && template != "бета") || (id != "c1" && id != "c2") {
		t.Fatalf("смешанная строка id=%s template=%s", id, template)
	}
}

func TestNT_R4_ForbiddenParams(t *testing.T) {
	conn, _ := newDB(t)
	cases := []struct {
		name   string
		params map[string]any
		secret string
	}{
		{name: "display_name", params: map[string]any{"student_id": studentID, "display_name": "pii-display-masha"}, secret: "pii-display-masha"},
		{name: "telegram_id", params: map[string]any{"telegram_id": "pii-telegram-777"}, secret: "pii-telegram-777"},
		{name: "name", params: map[string]any{"name": "pii-name-ivan"}, secret: "pii-name-ivan"},
		{name: "text", params: map[string]any{"text": "pii-text-essay"}, secret: "pii-text-essay"},
		{name: "all", params: map[string]any{
			"display_name": "pii-display-masha",
			"telegram_id":  "pii-telegram-777",
			"name":         "pii-name-ivan",
			"text":         "pii-text-essay",
		}, secret: "pii-text-essay"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			before := countOutbox(t, conn)
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if rb := tx.Rollback(ctx); rb != nil && !errors.Is(rb, pgx.ErrTxClosed) {
					t.Errorf("rollback: %v", rb)
				}
			}()
			err = notify.Enqueue(ctx, tx, message("bad-"+tc.name, "bad-"+tc.name, "homework_submitted",
				"tutor", tutorID, "шаблон", tc.params, time.Time{}, time.Time{}))
			expectCode(t, err, "outbox_params_invalid")
			if countOutbox(t, conn) != before {
				t.Fatal("запрещённые параметры записаны")
			}
			requireMissing(t, conn, tc.secret)
		})
	}

	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("ok-params", "dedup-ok", "homework_submitted", "tutor", tutorID,
		"шаблон", map[string]any{"student_id": studentID, "score": "11", "subject": "math"}, at, at))
	row := rowByID(t, conn, "ok-params")
	if !strings.Contains(row.Params, studentID) || strings.Contains(row.Params, "display_name") {
		t.Fatalf("params %s", row.Params)
	}
}

func TestNT_R13_CancelsPendingOfStudent(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	insertStudent(t, conn, studentID, tutorID, "Маша", studentTelegram, studentTelegram)
	insertStudent(t, conn, "student-other", tutorID, "Петя", "900003", "900003")
	insertHomework(t, conn, "hw-demo", studentID)
	t0 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	later := t0.Add(24 * time.Hour)

	commitEnqueue(t, conn, message("sent-1", "dedup-sent", "homework_issued", "student", studentID,
		"уже ушло", map[string]any{"student_id": studentID}, t0, t0))
	tr := &scriptTransport{}
	dispatchAt(t, conn, t0, tr)
	if len(tr.got()) != 1 {
		t.Fatalf("отправок %d", len(tr.got()))
	}

	commitEnqueue(t, conn, message("fail-1", "dedup-fail", "homework_issued", "student", studentID,
		"сбой", map[string]any{"student_id": studentID}, t0.Add(time.Second), t0.Add(time.Second)))
	tr = &scriptTransport{decide: func(notify.Delivery) error {
		return &notify.Failure{Status: 403}
	}}
	dispatchAt(t, conn, t0.Add(time.Second), tr)

	commitEnqueue(t, conn, message("to-student", "dedup-to", "homework_issued", "student", studentID,
		"ученику", map[string]any{"lesson": "L1"}, later, later))
	commitEnqueue(t, conn, message("about", "dedup-about", "homework_submitted", "tutor", tutorID,
		"репетитору", map[string]any{"student_id": studentID}, later, later))
	commitEnqueue(t, conn, message("other", "dedup-other", "homework_submitted", "tutor", tutorID,
		"чужой", map[string]any{"student_id": "student-other"}, later, later))

	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insertJournal(t, tx, "student_anonymised")
	if err := notify.CancelPending(ctx, tx, studentID); err != nil {
		t.Fatalf("CancelPending: %v", err)
	}
	if err := notify.Enqueue(ctx, tx, message("revoked", "dedup-revoked", "consent_revoked", "tutor", tutorID,
		"отзыв", map[string]any{"student_id": studentID, "until": "2026-11-17"}, later.Add(time.Millisecond), later)); err != nil {
		t.Fatalf("сообщение об отзыве: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"sent-1":     "sent",
		"fail-1":     "failed",
		"to-student": "cancelled",
		"about":      "cancelled",
		"other":      "pending",
		"revoked":    "pending",
	}
	for id, state := range want {
		row := rowByID(t, conn, id)
		if row.State != state {
			t.Fatalf("%s: state=%s, ожидалось %s", id, row.State, state)
		}
	}
	var hw string
	if err := conn.QueryRow(ctx, `SELECT state FROM core.assignment WHERE id = 'hw-demo'`).Scan(&hw); err != nil {
		t.Fatal(err)
	}
	if hw != "issued" {
		t.Fatalf("выдача %s", hw)
	}

	tr = &scriptTransport{}
	dispatchAt(t, conn, later, tr)
	if len(tr.got()) != 1 || tr.got()[0].Text != "чужой" {
		t.Fatalf("первая после отмены %+v", tr.got())
	}
	tr = &scriptTransport{}
	dispatchAt(t, conn, later.Add(time.Second), tr)
	if len(tr.got()) != 1 || tr.got()[0].Text != "отзыв" {
		t.Fatalf("отзыв %+v", tr.got())
	}
}

func TestNT_R13_CancelRollsBack(t *testing.T) {
	conn, _ := newDB(t)
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("keep", "dedup-keep", "homework_issued", "student", studentID,
		"ждёт", map[string]any{"student_id": studentID}, at, at))
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := notify.CancelPending(ctx, tx, studentID); err != nil {
		t.Fatalf("CancelPending: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	row := rowByID(t, conn, "keep")
	if row.State != "pending" {
		t.Fatalf("после отката state=%s", row.State)
	}
}
