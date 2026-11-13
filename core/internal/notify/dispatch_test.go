package notify_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/notify"
)

func TestNT_R5_SubstitutesAtSend(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	due := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	template := "{display_name} сдал(а) ДЗ: {score} из {total}"
	commitEnqueue(t, conn, message("r5-name", "dedup-r5", "homework_submitted", "tutor", tutorID,
		template, map[string]any{"student_id": studentID, "score": "11", "total": "15"}, due, due))
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		UPDATE core.student_pii SET display_name = 'Даша' WHERE student_id = $1
	`, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		UPDATE core.user_pii SET telegram_id = '900009' WHERE user_id = $1
	`, tutorID); err != nil {
		t.Fatal(err)
	}
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].Text != "Даша сдал(а) ДЗ: 11 из 15" || got[0].ParseMode != "" || got[0].ChatID != "900009" {
		t.Fatalf("доставка %+v", got)
	}
	row := rowByID(t, conn, "r5-name")
	if row.Template != template || row.State != "sent" || row.SentAt == nil || !sameInstant(*row.SentAt, due) {
		t.Fatalf("строка template=%s state=%s sent=%v", row.Template, row.State, row.SentAt)
	}
	requireMissing(t, conn, "Даша", "Маша", "900001", "900009")
}

func TestNT_R5_StudentTelegramFromPII(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	insertStudent(t, conn, studentID, tutorID, "Маша", "111111", "222222")
	due := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("r5-chat", "dedup-r5-chat", "homework_issued", "student", studentID,
		"пинг", map[string]any{"student_id": studentID}, due, due))
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `
		UPDATE core.student_pii SET telegram_id = '333333' WHERE student_id = $1
	`, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		UPDATE core.user_pii SET telegram_id = '444444' WHERE user_id = $1
	`, studentID); err != nil {
		t.Fatal(err)
	}
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].ChatID != "333333" || got[0].Text != "пинг" || got[0].ParseMode != "" {
		t.Fatalf("доставка %+v", got)
	}
	requireMissing(t, conn, "111111", "222222", "333333", "444444")
}

func TestNT_R6_SendsInCreatedOrder(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	due := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("late", "dedup-late", "homework_submitted", "tutor", tutorID,
		"потом", map[string]any{"student_id": studentID}, due.Add(time.Millisecond), due))
	commitEnqueue(t, conn, demoDue("early", "dedup-early", "сначала", due))
	if !sameInstant(rowByID(t, conn, "early").CreatedAt, due) {
		t.Fatal("created_at ранней строки не сохранён")
	}
	if !sameInstant(rowByID(t, conn, "late").CreatedAt, due.Add(time.Millisecond)) {
		t.Fatal("created_at поздней строки не сохранён")
	}
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].Text != "сначала" || got[0].ParseMode != "" {
		t.Fatalf("первая доставка %+v", got)
	}
	tr = &scriptTransport{}
	dispatchAt(t, conn, due.Add(time.Second), tr)
	got = tr.got()
	if len(got) != 1 || got[0].Text != "потом" {
		t.Fatalf("вторая доставка %+v", got)
	}
}

func TestNT_R6_FailureDoesNotBlockOther(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	due := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, demoDue("to-tutor", "dedup-tutor", "репетитору", due))
	commitEnqueue(t, conn, message("to-student", "dedup-student", "homework_issued", "student", studentID,
		"ученику", map[string]any{"student_id": studentID}, due, due))
	tr := &scriptTransport{decide: func(d notify.Delivery) error {
		if d.RecipientKind == "student" {
			return &notify.Failure{}
		}
		return nil
	}}
	dispatchAt(t, conn, due, tr)
	seen := map[string]int{}
	for _, d := range tr.got() {
		seen[d.RecipientID]++
		if d.ParseMode != "" {
			t.Fatalf("parse_mode %q", d.ParseMode)
		}
	}
	if seen[tutorID] != 1 || seen[studentID] != 1 {
		t.Fatalf("вызовы %+v", tr.got())
	}
	tutor := rowByID(t, conn, "to-tutor")
	if tutor.State != "sent" || tutor.SentAt == nil || !sameInstant(*tutor.SentAt, due) {
		t.Fatalf("репетитор %+v", tutor.State)
	}
	student := rowByID(t, conn, "to-student")
	if student.State != "pending" || student.Attempts != 1 || !sameInstant(student.NextAttemptAt, due.Add(30*time.Second)) {
		t.Fatalf("ученик state=%s attempts=%d next=%s", student.State, student.Attempts, student.NextAttemptAt)
	}
}

func TestNT_R7_TakesDueRow(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	requireNoSend(t, conn, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))

	commitEnqueue(t, conn, message("due-zero", "dedup-zero", "homework_submitted", "tutor", tutorID,
		"сразу", map[string]any{"student_id": studentID}, time.Time{}, time.Time{}))
	stored := rowByID(t, conn, "due-zero")
	if stored.NextAttemptAt.IsZero() || stored.CreatedAt.IsZero() {
		t.Fatal("срок или время создания пустые")
	}
	requireNoSend(t, conn, stored.NextAttemptAt.Add(-time.Second))
	sendAt := stored.NextAttemptAt.Add(time.Hour)
	tr := &scriptTransport{}
	dispatchAt(t, conn, sendAt, tr)
	if len(tr.got()) != 1 || tr.got()[0].Text != "сразу" {
		t.Fatalf("доставка %+v", tr.got())
	}
	sent := rowByID(t, conn, "due-zero")
	if sent.State != "sent" || sent.SentAt == nil || !sameInstant(*sent.SentAt, sendAt) {
		t.Fatalf("sent_at=%v, аргумент %s", sent.SentAt, sendAt)
	}

	future := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	commitEnqueue(t, conn, demoDue("due-future", "dedup-future", "позже", future))
	requireNoSend(t, conn, future.Add(-time.Second))
	tr = &scriptTransport{}
	dispatchAt(t, conn, future, tr)
	if len(tr.got()) != 1 {
		t.Fatalf("в срок ушло %d", len(tr.got()))
	}
	if row := rowByID(t, conn, "due-future"); row.SentAt == nil || !sameInstant(*row.SentAt, future) {
		t.Fatalf("sent_at=%v", row.SentAt)
	}
	requireNoSend(t, conn, future.Add(time.Hour))

	slot := future.Add(2 * time.Hour)
	commitEnqueue(t, conn, message("due-cancel", "dedup-cancel", "homework_issued", "student", studentID,
		"отмена", map[string]any{"student_id": studentID}, slot, slot))
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := notify.CancelPending(ctx, tx, studentID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	requireNoSend(t, conn, slot)
	if rowByID(t, conn, "due-cancel").State != "cancelled" {
		t.Fatal("отменённая строка ушла из pending иначе")
	}
}

func TestNT_R7_OneRowPerRecipient(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	base := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("tutor-late", "dedup-tl", "homework_submitted", "tutor", tutorID,
		"tutor-late", map[string]any{"student_id": studentID}, base.Add(time.Millisecond), base))
	commitEnqueue(t, conn, demoDue("tutor-early", "dedup-te", "tutor-early", base))
	commitEnqueue(t, conn, message("student-late", "dedup-sl", "homework_issued", "student", studentID,
		"student-late", map[string]any{"student_id": studentID}, base.Add(time.Millisecond), base))
	commitEnqueue(t, conn, message("student-early", "dedup-se", "homework_issued", "student", studentID,
		"student-early", map[string]any{"student_id": studentID}, base, base))
	tr := &scriptTransport{}
	dispatchAt(t, conn, base, tr)
	got := map[string]int{}
	for _, d := range tr.got() {
		got[d.Text]++
	}
	if len(tr.got()) != 2 || got["tutor-early"] != 1 || got["student-early"] != 1 {
		t.Fatalf("доставка %+v", tr.got())
	}
}

func TestNT_R7_SkipLocked(t *testing.T) {
	conn, dsn := newDB(t)
	seedDemo(t, conn)
	due := time.Date(2026, 11, 10, 16, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, demoDue("lock-1", "dedup-lock", "замок", due))
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM core.outbox WHERE id = 'lock-1' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	other := openConn(t, dsn)
	tr := &scriptTransport{}
	done := make(chan error, 1)
	go func() {
		callCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		done <- notify.Dispatch(callCtx, other, due, tr)
	}()
	select {
	case err := <-done:
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("Dispatch ждал блокировку")
		}
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		_ = tx.Rollback(ctx)
		t.Fatal("Dispatch не вернулся, пока строка заблокирована")
	}
	if len(tr.got()) != 0 {
		t.Fatal("заблокированная строка ушла в транспорт")
	}
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	dispatchAt(t, other, due, tr)
	if len(tr.got()) != 1 || tr.got()[0].Text != "замок" {
		t.Fatalf("после снятия блокировки %+v", tr.got())
	}
}

func TestNT_R7_ConcurrentRecipients(t *testing.T) {
	conn, dsn := newDB(t)
	seedDemo(t, conn)
	due := time.Date(2026, 11, 10, 16, 30, 0, 0, time.UTC)
	commitEnqueue(t, conn, demoDue("con-tutor", "dedup-ct", "репетитору", due))
	commitEnqueue(t, conn, message("con-student", "dedup-cs", "homework_issued", "student", studentID,
		"ученику", map[string]any{"student_id": studentID}, due, due))
	left := openConn(t, dsn)
	right := openConn(t, dsn)
	tr := &scriptTransport{}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, c := range []*pgx.Conn{left, right} {
		wg.Add(1)
		go func(c *pgx.Conn) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errCh <- notify.Dispatch(ctx, c, due, tr)
		}(c)
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
	}
	seen := map[string]int{}
	for _, d := range tr.got() {
		seen[d.RecipientID]++
	}
	if seen[tutorID] != 1 || seen[studentID] != 1 || len(tr.got()) != 2 {
		t.Fatalf("доставка %+v", tr.got())
	}
}

func TestNT_R8_FirstPause(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	errs := []struct {
		id  string
		err error
	}{
		{id: "net", err: &notify.Failure{}},
		{id: "s500", err: &notify.Failure{Status: 500}},
		{id: "s599", err: &notify.Failure{Status: 599}},
		{id: "plain", err: errors.New("network down")},
	}
	byID := map[string]error{}
	for i, tc := range errs {
		byID[tc.id] = tc.err
		tg := fmt.Sprintf("93100%d", i)
		insertStudent(t, conn, tc.id, tutorID, "Ученик", tg, tg)
		commitEnqueue(t, conn, message("ob-"+tc.id, "dedup-"+tc.id, "homework_issued", "student", tc.id,
			"шаблон-р8", map[string]any{"student_id": tc.id}, at, at))
	}
	tr := &scriptTransport{decide: func(d notify.Delivery) error {
		return byID[d.RecipientID]
	}}
	dispatchAt(t, conn, at, tr)
	if len(tr.got()) != len(errs) {
		t.Fatalf("вызовов %d", len(tr.got()))
	}
	ctx := context.Background()
	for _, tc := range errs {
		row := rowByID(t, conn, "ob-"+tc.id)
		if row.State != "pending" || row.Attempts != 1 || !sameInstant(row.NextAttemptAt, at.Add(30*time.Second)) {
			t.Fatalf("%s state=%s attempts=%d next=%s", tc.id, row.State, row.Attempts, row.NextAttemptAt)
		}
		if row.LastError == nil || *row.LastError == "" || *row.LastError == "unreachable" ||
			strings.Contains(*row.LastError, "шаблон-р8") || strings.Contains(*row.LastError, "93100") {
			t.Fatalf("%s last_error=%v", tc.id, row.LastError)
		}
		ok, err := notify.Unreachable(ctx, conn, tc.id)
		if err != nil || ok {
			t.Fatalf("%s unreachable %v %v", tc.id, ok, err)
		}
	}
}

func TestNT_R8_FifthFailed(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	insertStudent(t, conn, "slow", tutorID, "Ученик", "930001", "930001")
	t0 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("slow-1", "dedup-slow", "homework_issued", "student", "slow",
		"шаблон-р8", map[string]any{"student_id": "slow"}, t0, t0))
	now := t0
	pauses := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second}
	for i, pause := range pauses {
		tr := &scriptTransport{decide: func(notify.Delivery) error { return &notify.Failure{} }}
		dispatchAt(t, conn, now, tr)
		if len(tr.got()) != 1 {
			t.Fatalf("попытка %d: вызовов %d", i+1, len(tr.got()))
		}
		row := rowByID(t, conn, "slow-1")
		wantNext := now.Add(pause)
		if row.State != "pending" || row.Attempts != i+1 || !sameInstant(row.NextAttemptAt, wantNext) {
			t.Fatalf("попытка %d state=%s attempts=%d next=%s", i+1, row.State, row.Attempts, row.NextAttemptAt)
		}
		if row.LastError == nil || strings.Contains(*row.LastError, "шаблон-р8") || strings.Contains(*row.LastError, "930001") {
			t.Fatalf("last_error %v", row.LastError)
		}
		requireNoSend(t, conn, wantNext.Add(-time.Second))
		now = wantNext
	}
	tr := &scriptTransport{decide: func(notify.Delivery) error { return &notify.Failure{} }}
	dispatchAt(t, conn, now, tr)
	if len(tr.got()) != 1 {
		t.Fatalf("пятая попытка: вызовов %d", len(tr.got()))
	}
	row := rowByID(t, conn, "slow-1")
	if row.State != "failed" || row.Attempts != 5 {
		t.Fatalf("state=%s attempts=%d", row.State, row.Attempts)
	}
	ok, err := notify.Unreachable(context.Background(), conn, "slow")
	if err != nil || ok {
		t.Fatalf("unreachable %v %v", ok, err)
	}
	requireNoSend(t, conn, now.Add(24*time.Hour))
	if rowByID(t, conn, "slow-1").State != "failed" {
		t.Fatal("после паузы строка снова ушла в отправку")
	}
}

func TestNT_R9_WaitRetryAfter(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	insertStudent(t, conn, "s-429", tutorID, "Ученик", "940001", "940001")
	insertStudent(t, conn, "s-keep", tutorID, "Ученик", "940002", "940002")
	t0 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("m-429", "dedup-429", "homework_issued", "student", "s-429",
		"пинг", map[string]any{"student_id": "s-429"}, t0, t0))
	commitEnqueue(t, conn, message("m-keep", "dedup-keep", "homework_issued", "student", "s-keep",
		"пинг", map[string]any{"student_id": "s-keep"}, t0, t0))
	stage := map[string]int{}
	tr := &scriptTransport{decide: func(d notify.Delivery) error {
		stage[d.RecipientID]++
		switch d.RecipientID {
		case "s-429":
			if stage[d.RecipientID] == 1 {
				return &notify.Failure{Status: 429, RetryAfter: 20 * time.Second}
			}
			return nil
		case "s-keep":
			switch stage[d.RecipientID] {
			case 1:
				return &notify.Failure{}
			case 2:
				return &notify.Failure{Status: 429, RetryAfter: 20 * time.Second}
			default:
				return nil
			}
		default:
			t.Fatalf("лишний получатель %s", d.RecipientID)
			return nil
		}
	}}
	dispatchAt(t, conn, t0, tr)
	row429 := rowByID(t, conn, "m-429")
	if row429.State != "pending" || row429.Attempts != 0 || !sameInstant(row429.NextAttemptAt, t0.Add(20*time.Second)) {
		t.Fatalf("429 state=%s attempts=%d next=%s", row429.State, row429.Attempts, row429.NextAttemptAt)
	}
	rowKeep := rowByID(t, conn, "m-keep")
	if rowKeep.Attempts != 1 || !sameInstant(rowKeep.NextAttemptAt, t0.Add(30*time.Second)) {
		t.Fatalf("сеть attempts=%d next=%s", rowKeep.Attempts, rowKeep.NextAttemptAt)
	}
	dispatchAt(t, conn, t0.Add(20*time.Second), tr)
	row429 = rowByID(t, conn, "m-429")
	if row429.State != "sent" || row429.SentAt == nil || !sameInstant(*row429.SentAt, t0.Add(20*time.Second)) {
		t.Fatalf("после ожидания state=%s sent=%v", row429.State, row429.SentAt)
	}
	at := t0.Add(30 * time.Second)
	dispatchAt(t, conn, at, tr)
	rowKeep = rowByID(t, conn, "m-keep")
	if rowKeep.State != "pending" || rowKeep.Attempts != 1 || !sameInstant(rowKeep.NextAttemptAt, at.Add(20*time.Second)) {
		t.Fatalf("повтор 429 state=%s attempts=%d next=%s", rowKeep.State, rowKeep.Attempts, rowKeep.NextAttemptAt)
	}
	requireNoSend(t, conn, at.Add(20*time.Second-time.Millisecond))
	dispatchAt(t, conn, at.Add(20*time.Second), tr)
	rowKeep = rowByID(t, conn, "m-keep")
	if rowKeep.State != "sent" || rowKeep.SentAt == nil || !sameInstant(*rowKeep.SentAt, at.Add(20*time.Second)) {
		t.Fatalf("отправка после 429 state=%s sent=%v", rowKeep.State, rowKeep.SentAt)
	}
}

func TestNT_R10_StopOnForbidden(t *testing.T) {
	for _, status := range []int{403, 400} {
		t.Run(fmt.Sprintf("status%d", status), func(t *testing.T) {
			conn, _ := newDB(t)
			seedDemo(t, conn)
			hw := fmt.Sprintf("hw-%d", status)
			insertHomework(t, conn, hw, studentID)
			due := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
			commitEnqueue(t, conn, message("stop", "dedup-stop", "homework_issued", "student", studentID,
				"пинг", map[string]any{"student_id": studentID}, due, due))
			ctx := context.Background()
			ok, err := notify.Unreachable(ctx, conn, studentID)
			if err != nil || ok {
				t.Fatalf("до ответа unreachable %v %v", ok, err)
			}
			tr := &scriptTransport{decide: func(notify.Delivery) error {
				return &notify.Failure{Status: status}
			}}
			dispatchAt(t, conn, due, tr)
			got := tr.got()
			if len(got) != 1 || got[0].ParseMode != "" {
				t.Fatalf("доставка %+v", got)
			}
			row := rowByID(t, conn, "stop")
			if row.State != "failed" || row.LastError == nil || *row.LastError != "unreachable" {
				t.Fatalf("state=%s last=%v", row.State, row.LastError)
			}
			ok, err = notify.Unreachable(ctx, conn, studentID)
			if err != nil || !ok {
				t.Fatalf("unreachable ученика %v %v", ok, err)
			}
			ok, err = notify.Unreachable(ctx, conn, tutorID)
			if err != nil || ok {
				t.Fatalf("unreachable репетитора %v %v", ok, err)
			}
			ok, err = notify.Unreachable(ctx, conn, "student-other")
			if err != nil || ok {
				t.Fatalf("чужой unreachable %v %v", ok, err)
			}
			requireNoSend(t, conn, due.Add(24*time.Hour))
			if rowByID(t, conn, "stop").State != "failed" {
				t.Fatal("строка ушла на повтор")
			}
			var hwState string
			if err := conn.QueryRow(ctx, `SELECT state FROM core.assignment WHERE id = $1`, hw).Scan(&hwState); err != nil {
				t.Fatal(err)
			}
			if hwState != "issued" {
				t.Fatalf("выдача %s", hwState)
			}
		})
	}
}

func TestNT_R11_OnePerSecondPerChat(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	due := time.Date(2026, 11, 10, 19, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, demoDue("a", "dedup-a", "один", due))
	commitEnqueue(t, conn, message("b", "dedup-b", "homework_submitted", "tutor", tutorID,
		"два", map[string]any{"student_id": studentID}, due.Add(time.Millisecond), due))
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	if len(tr.got()) != 1 || tr.got()[0].Text != "один" {
		t.Fatalf("первая %+v", tr.got())
	}
	row := rowByID(t, conn, "b")
	if row.State != "pending" || row.Attempts != 0 || !sameInstant(row.NextAttemptAt, due.Add(time.Second)) {
		t.Fatalf("вторая state=%s attempts=%d next=%s", row.State, row.Attempts, row.NextAttemptAt)
	}
	requireNoSend(t, conn, due)
	row = rowByID(t, conn, "b")
	if !sameInstant(row.NextAttemptAt, due.Add(time.Second)) {
		t.Fatalf("срок сдвинут повторно: %s", row.NextAttemptAt)
	}
	requireNoSend(t, conn, due.Add(time.Second-time.Millisecond))
	tr = &scriptTransport{}
	dispatchAt(t, conn, due.Add(time.Second), tr)
	if len(tr.got()) != 1 || tr.got()[0].Text != "два" || tr.got()[0].ParseMode != "" {
		t.Fatalf("вторая доставка %+v", tr.got())
	}
}

func TestNT_R11_TwentyPerSecond(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	base := time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 21; i++ {
		id := fmt.Sprintf("student-%02d", i)
		tg := fmt.Sprintf("921%03d", i)
		insertStudent(t, conn, id, tutorID, "Ученик", tg, tg)
		commitEnqueue(t, conn, message("ob-"+id, "dedup-"+id, "homework_issued", "student", id,
			"пинг", map[string]any{"student_id": id}, base.Add(time.Duration(i)*time.Millisecond), base))
	}
	tr := &scriptTransport{}
	dispatchAt(t, conn, base, tr)
	seen := map[string]int{}
	for _, d := range tr.got() {
		seen[d.RecipientID]++
		if d.ParseMode != "" || d.Text != "пинг" {
			t.Fatalf("доставка %+v", d)
		}
	}
	if len(tr.got()) != 20 || seen["student-20"] != 0 {
		t.Fatalf("ушло %d, student-20=%d", len(tr.got()), seen["student-20"])
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("student-%02d", i)
		if seen[id] != 1 {
			t.Fatalf("%s ушло %d раз", id, seen[id])
		}
		row := rowByID(t, conn, "ob-"+id)
		if row.State != "sent" || row.SentAt == nil || !sameInstant(*row.SentAt, base) {
			t.Fatalf("%s state=%s sent=%v", id, row.State, row.SentAt)
		}
	}
	waiting := rowByID(t, conn, "ob-student-20")
	if waiting.State != "pending" || waiting.Attempts != 0 || !sameInstant(waiting.NextAttemptAt, base.Add(time.Second)) {
		t.Fatalf("21-я state=%s attempts=%d next=%s", waiting.State, waiting.Attempts, waiting.NextAttemptAt)
	}
	requireNoSend(t, conn, base)
	waiting = rowByID(t, conn, "ob-student-20")
	if !sameInstant(waiting.NextAttemptAt, base.Add(time.Second)) {
		t.Fatalf("срок 21-й сдвинут повторно: %s", waiting.NextAttemptAt)
	}
	tr = &scriptTransport{}
	dispatchAt(t, conn, base.Add(time.Second), tr)
	if len(tr.got()) != 1 || tr.got()[0].RecipientID != "student-20" || tr.got()[0].ChatID != "921020" {
		t.Fatalf("21-я %+v", tr.got())
	}
}

func TestNT_R12_KeepsMarkup(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	insertStudent(t, conn, studentID, tutorID, "<b>Маша</b>", studentTelegram, studentTelegram)
	due := time.Date(2026, 11, 10, 21, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("markup", "dedup-markup", "homework_submitted", "tutor", tutorID,
		"{display_name} сдал(а) ДЗ", map[string]any{"student_id": studentID}, due, due))
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].Text != "<b>Маша</b> сдал(а) ДЗ" || got[0].ParseMode != "" {
		t.Fatalf("доставка %+v", got)
	}
	requireMissing(t, conn, "<b>Маша</b>", "Маша")
}

func TestNT_R12_ShortensList(t *testing.T) {
	conn, _ := newDB(t)
	seedDemo(t, conn)
	items := make([]any, 80)
	for i := range items {
		items[i] = fmt.Sprintf("hw-%03d", i+1)
	}
	due := time.Date(2026, 11, 10, 21, 30, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("list", "dedup-list", "recheck_changes", "tutor", tutorID,
		"{items}\n{link}", map[string]any{
			"student_id": studentID,
			"items":      items,
			"link":       "/cabinet/homework/hw-demo",
		}, due, due))
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, "hw-%03d\n", i)
	}
	b.WriteString("и ещё 60\n")
	b.WriteString("/cabinet/homework/hw-demo")
	got := tr.got()
	if len(got) != 1 || got[0].Text != b.String() || got[0].ParseMode != "" {
		t.Fatalf("текст %q", textOf(got))
	}
	if utf8.RuneCountInString(got[0].Text) > 4096 {
		t.Fatalf("длина %d", utf8.RuneCountInString(got[0].Text))
	}
	requireMissing(t, conn, "и ещё")
}

func TestNT_R12_CapsLength(t *testing.T) {
	conn, _ := newDB(t)
	insertTutor(t, conn, tutorID, tutorTelegram)
	name := strings.Repeat("Я", 5000)
	insertStudent(t, conn, studentID, tutorID, name, studentTelegram, studentTelegram)
	due := time.Date(2026, 11, 10, 22, 0, 0, 0, time.UTC)
	commitEnqueue(t, conn, message("long", "dedup-long", "homework_submitted", "tutor", tutorID,
		"{display_name}", map[string]any{"student_id": studentID}, due, due))
	tr := &scriptTransport{}
	dispatchAt(t, conn, due, tr)
	got := tr.got()
	if len(got) != 1 || got[0].ParseMode != "" {
		t.Fatalf("доставка %+v", got)
	}
	n := utf8.RuneCountInString(got[0].Text)
	if n > 4096 || n < 100 || !strings.Contains(got[0].Text, strings.Repeat("Я", 100)) {
		t.Fatalf("длина %d", n)
	}
	if rowByID(t, conn, "long").State != "sent" {
		t.Fatal("длинное сообщение не отправлено")
	}
	requireMissing(t, conn, strings.Repeat("Я", 200))
}

func requireNoSend(t *testing.T, conn *pgx.Conn, now time.Time) {
	t.Helper()
	tr := &scriptTransport{}
	dispatchAt(t, conn, now, tr)
	if len(tr.got()) != 0 {
		t.Fatalf("транспорт вызван в %s: %+v", now.Format(time.RFC3339Nano), tr.got())
	}
}

func textOf(got []notify.Delivery) string {
	if len(got) == 0 {
		return ""
	}
	return got[0].Text
}
