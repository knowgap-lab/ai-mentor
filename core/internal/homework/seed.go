package homework

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type lessonBlob struct {
	Cancelled bool       `json:"cancelled"`
	Coverage  string     `json:"coverage"`
	Blocks    []string   `json:"blocks"`
	Tasks     []Task     `json:"tasks"`
	Recipe    []Series   `json:"recipe"`
	Mark      *Mark      `json:"mark"`
	Variants  []Variant  `json:"variants"`
	Past      []PastItem `json:"past"`
}

// Seed кладёт репетитора, ученика, уроки и каталог вариантов фикстуры.
// Черновик ДЗ не пишет: его собирает первый запрос.
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
	if err := insertUser(ctx, tx, tutorID, "tutor", ""); err != nil {
		return World{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, tutorID, telegramID); err != nil {
		return World{}, err
	}
	if err := insertUser(ctx, tx, studentID, "student", tutorID); err != nil {
		return World{}, err
	}
	if err := insertPlan(ctx, tx, studentID, fx.Subject, fx.CatalogVersion); err != nil {
		return World{}, err
	}
	for _, lesson := range fx.Lessons {
		if err := insertLesson(ctx, tx, studentID, fx.Subject, lesson, fx.Variants, fx.Past); err != nil {
			return World{}, err
		}
	}

	world := World{
		StudentID:       studentID,
		Subject:         fx.Subject,
		LessonIDs:       lessonIDs(fx.Lessons),
		ActorTelegramID: telegramID,
		ActorRole:       "tutor",
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
		otherLesson, err := newID()
		if err != nil {
			return World{}, err
		}
		if err := insertUser(ctx, tx, otherTutor, "tutor", ""); err != nil {
			return World{}, err
		}
		if err := insertUser(ctx, tx, otherStudent, "student", otherTutor); err != nil {
			return World{}, err
		}
		if err := insertPlan(ctx, tx, otherStudent, fx.Subject, fx.CatalogVersion); err != nil {
			return World{}, err
		}
		if err := insertLesson(ctx, tx, otherStudent, fx.Subject, Lesson{
			ID: otherLesson, PlaceNo: 1, State: "planned",
		}, nil, nil); err != nil {
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

func insertUser(ctx context.Context, tx pgx.Tx, id, role, owner string) error {
	var ownerID any
	if owner != "" {
		ownerID = owner
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, $2, $3, 'active')
	`, id, role, ownerID)
	return err
}

func insertPlan(ctx context.Context, tx pgx.Tx, studentID, subject string, version int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
		VALUES ($1, $2, $3, $4)
	`, studentID, subject, time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), version)
	return err
}

func insertLesson(ctx context.Context, tx pgx.Tx, studentID, subject string, lesson Lesson, variants []Variant, past []PastItem) error {
	state := lesson.State
	if state == "" {
		state = "planned"
	}
	var starts *time.Time
	if lesson.StartsAt != "" {
		parsed, err := time.Parse(time.RFC3339, lesson.StartsAt)
		if err != nil {
			return err
		}
		utc := parsed.UTC()
		starts = &utc
	}
	raw, err := json.Marshal(lessonBlob{
		Cancelled: lesson.Cancelled,
		Coverage:  lesson.Coverage,
		Blocks:    lesson.Blocks,
		Tasks:     lesson.Tasks,
		Recipe:    lesson.Recipe,
		Mark:      lesson.Mark,
		Variants:  variants,
		Past:      past,
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.lesson (
			id, student_id, subject, place_no, title, kind, template_id, state,
			starts_at, no_blocks, template
		) VALUES (
			$1, $2, $3, $4, 'Урок', 'new', 'hw-template', $5,
			$6, $7, $8::jsonb
		)
	`, lesson.ID, studentID, subject, lesson.PlaceNo, state, starts, lesson.NoBlocks, string(raw))
	return err
}

func lessonIDs(lessons []Lesson) []string {
	ordered := append([]Lesson(nil), lessons...)
	for i := 1; i < len(ordered); i++ {
		item := ordered[i]
		j := i
		for j > 0 && ordered[j-1].PlaceNo > item.PlaceNo {
			ordered[j] = ordered[j-1]
			j--
		}
		ordered[j] = item
	}
	ids := make([]string, len(ordered))
	for i, lesson := range ordered {
		ids[i] = lesson.ID
	}
	return ids
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
