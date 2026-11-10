package plan

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// Seed кладёт репетитора, ученика, план 0 и текущие уроки.
// Сдвиг недель попадает и в снимок, и в текущий план. Расклад — по Fixture.Now.
func Seed(ctx context.Context, conn *pgx.Conn, fx Fixture) (World, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return World{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tutorID, err := newID()
	if err != nil {
		return World{}, err
	}
	studentID, err := newID()
	if err != nil {
		return World{}, err
	}
	telegramID, err := newID()
	if err != nil {
		return World{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'tutor', NULL, 'active')
	`, tutorID); err != nil {
		return World{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id)
		VALUES ($1, $2)
	`, tutorID, telegramID); err != nil {
		return World{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, studentID, tutorID); err != nil {
		return World{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
		VALUES ($1, $2, $3, $4)
	`, studentID, fx.Subject, fx.ExamDate.UTC(), fx.CatalogVersion); err != nil {
		return World{}, err
	}

	hashes := fx.SourceHashes
	if hashes == nil {
		hashes = map[string]string{}
	}
	rawHashes, err := json.Marshal(hashes)
	if err != nil {
		return World{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.plan_snapshot (student_id, subject, version, source_hashes)
		VALUES ($1, $2, 0, $3::jsonb)
	`, studentID, fx.Subject, string(rawHashes)); err != nil {
		return World{}, err
	}

	for _, spec := range fx.Lessons {
		week, err := shiftWeek(spec.PlannedWeek, fx.ShiftWeeks)
		if err != nil {
			return World{}, err
		}
		state := spec.State
		if state == "" {
			state = "planned"
		}
		var starts *time.Time
		if spec.StartsAt != "" {
			parsed, err := time.Parse(time.RFC3339, spec.StartsAt)
			if err != nil {
				return World{}, err
			}
			utc := parsed.UTC()
			starts = &utc
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.lesson (
				id, student_id, subject, place_no, title, kind, template_id, state,
				planned_week, starts_at, moved, added, template
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8,
				$9, $10, $11, $12, '[]'::jsonb
			)
		`, spec.ID, studentID, fx.Subject, spec.PlaceNo, spec.Title, spec.Kind, spec.TemplateID, state,
			week, starts, spec.Moved, spec.Added); err != nil {
			return World{}, err
		}
		for i, topic := range spec.Topics {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.lesson_topic (lesson_id, position, topic)
				VALUES ($1, $2, $3)
			`, spec.ID, i+1, topic); err != nil {
				return World{}, err
			}
		}
		if spec.Added {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.plan_snapshot_lesson (
				student_id, subject, version, place_no, lesson_id, template_id, planned_week
			) VALUES ($1, $2, 0, $3, $4, $5, $6)
		`, studentID, fx.Subject, spec.PlaceNo, spec.ID, spec.TemplateID, week); err != nil {
			return World{}, err
		}
	}
	for topic, prereqs := range fx.Prereqs {
		seen := map[string]bool{}
		for _, prereq := range prereqs {
			if prereq == "" || seen[prereq] {
				continue
			}
			seen[prereq] = true
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.topic_prereq (student_id, subject, topic, prereq)
				VALUES ($1, $2, $3, $4)
			`, studentID, fx.Subject, topic, prereq); err != nil {
				return World{}, err
			}
		}
	}
	if fx.Schedule != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.schedule_rule (student_id, subject, weekdays, start_time, duration_min)
			VALUES ($1, $2, $3, $4, $5)
		`, studentID, fx.Subject, fx.Schedule.Weekdays, fx.Schedule.StartTime, fx.Schedule.DurationMin); err != nil {
			return World{}, err
		}
		if err := layoutSubject(ctx, tx, studentID, fx.Subject, fx.Now); err != nil {
			return World{}, err
		}
	}
	if err := freezeSubject(ctx, tx, studentID, fx.Subject); err != nil {
		return World{}, err
	}
	if fx.HomeworkPercent != nil {
		if err := seedHomework(ctx, tx, studentID, fx.CatalogVersion, *fx.HomeworkPercent); err != nil {
			return World{}, err
		}
	}

	world := World{
		StudentID:       studentID,
		Subject:         fx.Subject,
		ActorTelegramID: telegramID,
		ActorRole:       "tutor",
		CatalogVersion:  fx.CatalogVersion,
	}
	ordered := append([]LessonSpec(nil), fx.Lessons...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].PlaceNo < ordered[j].PlaceNo })
	world.LessonIDs = make([]string, len(ordered))
	for i, spec := range ordered {
		world.LessonIDs[i] = spec.ID
	}
	if fx.OtherStudent {
		otherTutor, err := newID()
		if err != nil {
			return World{}, err
		}
		otherStudent, err := newID()
		if err != nil {
			return World{}, err
		}
		otherLesson := "demo-other"
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.app_user (id, role, owner_tutor_id, status)
			VALUES ($1, 'tutor', NULL, 'active')
		`, otherTutor); err != nil {
			return World{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.app_user (id, role, owner_tutor_id, status)
			VALUES ($1, 'student', $2, 'active')
		`, otherStudent, otherTutor); err != nil {
			return World{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
			VALUES ($1, $2, $3, $4)
		`, otherStudent, fx.Subject, fx.ExamDate.UTC(), fx.CatalogVersion); err != nil {
			return World{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.lesson (
				id, student_id, subject, place_no, title, kind, template_id, state, template
			) VALUES ($1, $2, $3, 1, 'Чужой урок', 'new', 'demo:other', 'planned', '[]'::jsonb)
		`, otherLesson, otherStudent, fx.Subject); err != nil {
			return World{}, err
		}
		world.OtherStudentID = otherStudent
		world.OtherLessonID = otherLesson
	}
	if err := tx.Commit(ctx); err != nil {
		return World{}, err
	}
	return world, nil
}

func seedHomework(ctx context.Context, db q, studentID string, version, points int) error {
	if version < 1 {
		version = 1
	}
	assignmentID, err := newID()
	if err != nil {
		return err
	}
	attemptID, err := newID()
	if err != nil {
		return err
	}
	verdict := "partial"
	if points <= 0 {
		verdict = "wrong"
	} else if points >= 100 {
		verdict = "correct"
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		) VALUES ($1, $2, 'submitted', 'homework', $3, NULL, 'home', '{}'::jsonb)
	`, assignmentID, studentID, version); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		VALUES ($1, 'hw-item', 'final', false, 100)
	`, assignmentID); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO core.attempts (
			id, assignment_id, item_id, raw_input, source, place, attempt_no
		) VALUES ($1, $2, 'hw-item', '1', 'homework', 'home', 1)
	`, attemptID, assignmentID); err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
		VALUES ($1, 1, $2, $3)
	`, attemptID, verdict, points)
	return err
}

// ReadPlan0 читает снимок версии 0 в порядке мест.
func ReadPlan0(ctx context.Context, conn *pgx.Conn, studentID, subject string) (Snapshot, error) {
	var raw []byte
	err := conn.QueryRow(ctx, `
		SELECT source_hashes
		FROM core.plan_snapshot
		WHERE student_id = $1 AND subject = $2 AND version = 0
	`, studentID, subject).Scan(&raw)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{
		SourceHashes: map[string]string{},
		LessonIDs:    []string{},
		TemplateIDs:  []string{},
		PlannedWeeks: []string{},
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &snap.SourceHashes); err != nil {
			return Snapshot{}, err
		}
	}
	if snap.SourceHashes == nil {
		snap.SourceHashes = map[string]string{}
	}
	lessons, err := loadSnapshotLessons(ctx, conn, studentID, subject)
	if err != nil {
		return Snapshot{}, err
	}
	for _, lesson := range lessons {
		snap.LessonIDs = append(snap.LessonIDs, lesson.ID)
		snap.TemplateIDs = append(snap.TemplateIDs, lesson.TemplateID)
		snap.PlannedWeeks = append(snap.PlannedWeeks, lesson.Week)
	}
	return snap, nil
}

// ReplacePlan0 пытается сменить хеши снимка версии 0. Снимок неизменяем.
func ReplacePlan0(ctx context.Context, conn *pgx.Conn, studentID, subject string, hashes map[string]string) error {
	if hashes == nil {
		hashes = map[string]string{}
	}
	raw, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `
		UPDATE core.plan_snapshot
		SET source_hashes = $3::jsonb
		WHERE student_id = $1 AND subject = $2 AND version = 0
	`, studentID, subject, string(raw))
	if err != nil {
		return err
	}
	return errImmutable
}

var errImmutable = immutableError{}

type immutableError struct{}

func (immutableError) Error() string { return "plan 0 is immutable" }
