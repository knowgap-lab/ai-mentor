package lessons

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

// Seed кладёт выдуманного репетитора, ученика и уроки.
// Если в фикстуре есть правило, уроки сразу раскладываются по слотам.
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
	for _, item := range fx.Pool {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.variant_pool (student_id, variant_id, type_id, seen, reserve)
			VALUES ($1, $2, $3, $4, $5)
		`, studentID, item.ID, item.TypeID, item.Seen, item.Reserve); err != nil {
			return World{}, err
		}
	}
	for _, spec := range fx.Lessons {
		stages := fromSpecs(spec.Stages)
		if spec.NoBlocks {
			stages = []stage{}
		}
		raw, err := stagesJSON(stages)
		if err != nil {
			return World{}, err
		}
		state := spec.State
		if state == "" {
			state = "planned"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.lesson (
				id, student_id, subject, place_no, title, kind, template_id, state,
				planned_week, no_blocks, issued, template
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8,
				$9, $10, $11, $12::jsonb
			)
		`, spec.ID, studentID, fx.Subject, spec.PlaceNo, spec.Title, spec.Kind, spec.TemplateID, state,
			spec.PlannedWeek, spec.NoBlocks, spec.Issued, raw); err != nil {
			return World{}, err
		}
	}
	if fx.Schedule != nil {
		if err := saveSchedule(ctx, tx, studentID, fx.Subject, *fx.Schedule, false); err != nil {
			return World{}, err
		}
		if err := layoutSubject(ctx, tx, studentID, fx.Subject, fx.Now); err != nil {
			return World{}, err
		}
	}
	if err := freezeSubject(ctx, tx, studentID, fx.Subject); err != nil {
		return World{}, err
	}
	if fx.ReviseBlocks != nil {
		if err := reviseTemplates(ctx, tx, studentID, fx.Subject, fx.ReviseBlocks); err != nil {
			return World{}, err
		}
	}
	for _, spec := range fx.Lessons {
		if spec.Mark == nil {
			continue
		}
		if err := saveMark(ctx, tx, spec.ID, spec.Mark.Theory, spec.Mark.Practice, ""); err != nil {
			return World{}, err
		}
		if err := writeMarked(ctx, tx, spec.ID, spec.Mark.Theory, spec.Mark.Practice, ""); err != nil {
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
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].PlaceNo < ordered[j].PlaceNo })
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
		raw, err := stagesJSON(nil)
		if err != nil {
			return World{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.lesson (
				id, student_id, subject, place_no, title, kind, template_id, state, template
			) VALUES ($1, $2, $3, 1, 'Чужой урок', 'new', 'demo:other', 'planned', $4::jsonb)
		`, otherLesson, otherStudent, fx.Subject, raw); err != nil {
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

func reviseTemplates(ctx context.Context, db q, studentID, subject string, blocks []string) error {
	rows, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	for _, row := range rows {
		template := cloneStages(row.Template)
		replaced := false
		for i := range template {
			if len(template[i].Blocks) == 0 || replaced {
				continue
			}
			template[i].Blocks = append([]string(nil), blocks...)
			replaced = true
		}
		if !replaced {
			continue
		}
		raw, err := stagesJSON(template)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			UPDATE core.lesson SET template = $2::jsonb WHERE id = $1
		`, row.ID, raw); err != nil {
			return err
		}
	}
	return nil
}
