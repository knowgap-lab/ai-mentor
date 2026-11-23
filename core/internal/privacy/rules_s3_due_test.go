package privacy_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestPV_R19_S3_AnonymiseDue(t *testing.T) {
	h := s3New(t)
	tutor := h.login("tg-s3-r19", "tutor")
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	past := h.create(tutor, "demo-s3-past", s3Birth(16))
	edge := h.create(tutor, "demo-s3-edge", s3Birth(16))
	later := h.create(tutor, "demo-s3-later", s3Birth(16))
	plain := h.create(tutor, "demo-s3-plain", s3Birth(16))
	s3MarkDue(h, past.ID, now.Add(-time.Hour))
	s3MarkDue(h, edge.ID, now)
	s3MarkDue(h, later.ID, now.Add(time.Hour))
	s3SeedResidue(t, h, past.ID, tutor.ID)
	for _, id := range []string{past.ID, edge.ID, later.ID, plain.ID} {
		if _, ok := h.pseudonym(id); !ok {
			t.Fatalf("нет псевдонима %s", id)
		}
	}
	n, err := h.svc.AnonymiseDue(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("AnonymiseDue = %d, нужно 2", n)
	}
	s3ExpectAnonymised(t, h, tutor, past.ID, "demo-s3-past")
	s3ExpectAnonymised(t, h, tutor, edge.ID, "demo-s3-edge")
	s3ExpectPIIKept(t, h, later.ID)
	s3ExpectPIIKept(t, h, plain.ID)
	if h.count(`SELECT count(*) FROM core.attempts WHERE id = 's3-att'`) != 1 {
		t.Fatal("попытка удалена")
	}
	if h.count(`SELECT count(*) FROM core.assignment WHERE id = 's3-asg' AND student_id = $1`, past.ID) != 1 {
		t.Fatal("выдача удалена или сменила ученика")
	}
	if h.count(`SELECT count(*) FROM core.observation WHERE assignment_id = 's3-asg'`) != 1 {
		t.Fatal("наблюдение удалено")
	}
	if h.text(`SELECT state FROM core.outbox WHERE id = 's3-to-student'`) != "cancelled" {
		t.Fatal("ожидающее сообщение ученику не отменено")
	}
	if h.text(`SELECT state FROM core.outbox WHERE id = 's3-to-tutor'`) != "pending" {
		t.Fatal("сообщение репетитору отменено")
	}
	if h.count(`SELECT count(*) FROM core.consent_link WHERE student_id = $1`, past.ID) != 0 {
		t.Fatal("ссылка согласия осталась")
	}
	if h.count(`SELECT count(*) FROM core.telegram_invite WHERE student_id = $1`, past.ID) != 0 {
		t.Fatal("приглашение осталось")
	}
	if h.count(`SELECT count(*) FROM core.user_pii WHERE user_id = $1`, past.ID) != 0 {
		t.Fatal("привязка Telegram осталась")
	}
	if h.count(`SELECT count(*) FROM core.consent WHERE student_id = $1`, past.ID) != 1 {
		t.Fatal("запись согласия удалена")
	}
	nEvents := 0
	for _, ev := range h.events() {
		if ev.Kind != "student_anonymised" {
			continue
		}
		nEvents++
		s3PayloadExact(t, ev.Payload, "source")
		if s3String(t, ev.Payload, "source") != "schedule" {
			t.Fatalf("source = %v", ev.Payload["source"])
		}
	}
	if nEvents != 2 {
		t.Fatalf("событий обезличивания %d", nEvents)
	}
	again, err := h.svc.AnonymiseDue(context.Background(), now)
	if err != nil || again != 0 {
		t.Fatalf("повтор AnonymiseDue = %d %v", again, err)
	}
	s3ExpectPIIKept(t, h, later.ID)
}

func TestExamples_S3_Privacy13(t *testing.T) {
	var rows []s3ExampleRow
	if err := yaml.Unmarshal(s3RepoFile(t, "docs/platform/privacy.examples.yaml"), &rows); err != nil {
		t.Fatal(err)
	}
	var row s3ExampleRow
	for _, item := range rows {
		if item.ID == "privacy-13" {
			row = item
		}
	}
	if row.Rule != "PV-R19" || !strings.Contains(row.Expect, "ПДн и псевдоним удалены") {
		t.Fatalf("строка privacy-13: %+v", row)
	}
	h := s3New(t)
	tutor := h.login("tg-s3-ex13", "tutor")
	card := h.create(tutor, s3Nick, s3Birth(16))
	due := time.Now().UTC().Add(-time.Hour)
	s3MarkDue(h, card.ID, due)
	s3SeedResidue(t, h, card.ID, tutor.ID)
	n, err := h.svc.AnonymiseDue(context.Background(), time.Now().UTC())
	if err != nil || n != 1 {
		t.Fatalf("AnonymiseDue = %d %v", n, err)
	}
	if _, _, _, ok := h.pii(card.ID); ok {
		t.Fatal("ПДн на месте")
	}
	if _, ok := h.pseudonym(card.ID); ok {
		t.Fatal("псевдоним на месте")
	}
	if h.count(`SELECT count(*) FROM core.attempts WHERE id = 's3-att'`) != 1 {
		t.Fatal("попытка удалена")
	}
	if h.text(`SELECT state FROM core.outbox WHERE id = 's3-to-student'`) != "cancelled" {
		t.Fatal("ожидающее сообщение не отменено")
	}
}
