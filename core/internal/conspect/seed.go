package conspect

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Дата экзамена нужна только столбцу плана. Тесты её не читают.
var planExamAt = time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)

// Seed сохраняет уроки фикстуры, ревизии каталога и снимок frozen/held.
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
	tutorTelegram, err := newID()
	if err != nil {
		return World{}, err
	}
	studentTelegram, err := newID()
	if err != nil {
		return World{}, err
	}
	if err := insertAccount(ctx, tx, tutorID, "tutor", nil, tutorTelegram); err != nil {
		return World{}, err
	}
	if err := insertAccount(ctx, tx, studentID, "student", &tutorID, studentTelegram); err != nil {
		return World{}, err
	}
	if err := insertPlan(ctx, tx, studentID, fx.Subject, fx.CatalogVersion); err != nil {
		return World{}, err
	}
	for _, block := range fx.Blocks {
		if err := insertRevision(ctx, tx, block); err != nil {
			return World{}, err
		}
	}
	for _, lesson := range fx.Lessons {
		if err := insertLesson(ctx, tx, studentID, fx.Subject, lesson, fx.SnapshotVersion); err != nil {
			return World{}, err
		}
	}
	for _, carry := range fx.Carry {
		if err := insertCarry(ctx, tx, carry); err != nil {
			return World{}, err
		}
	}

	world := World{
		TutorTelegramID:   tutorTelegram,
		StudentTelegramID: studentTelegram,
		ActorRole:         "tutor",
	}
	if fx.Peer {
		peerID, peerErr := insertPeer(ctx, tx, tutorID, fx)
		if peerErr != nil {
			return World{}, peerErr
		}
		world.PeerLessonID = peerID
	}
	if fx.OtherStudent {
		otherID, otherErr := insertOther(ctx, tx, fx)
		if otherErr != nil {
			return World{}, otherErr
		}
		world.OtherLessonID = otherID
	}
	if err := tx.Commit(ctx); err != nil {
		return World{}, err
	}
	return world, nil
}

func insertPeer(ctx context.Context, tx pgx.Tx, tutorID string, fx Fixture) (string, error) {
	studentID, err := newID()
	if err != nil {
		return "", err
	}
	telegramID, err := newID()
	if err != nil {
		return "", err
	}
	if err := insertAccount(ctx, tx, studentID, "student", &tutorID, telegramID); err != nil {
		return "", err
	}
	if err := insertPlan(ctx, tx, studentID, fx.Subject, fx.CatalogVersion); err != nil {
		return "", err
	}
	idMap := make(map[string]string, len(fx.Lessons))
	for _, lesson := range fx.Lessons {
		idMap[lesson.ID] = "peer-" + lesson.ID
	}
	var first string
	for i, lesson := range fx.Lessons {
		lesson.ID = idMap[lesson.ID]
		if err := insertLesson(ctx, tx, studentID, fx.Subject, lesson, fx.SnapshotVersion); err != nil {
			return "", err
		}
		if i == 0 {
			first = lesson.ID
		}
	}
	for _, carry := range fx.Carry {
		toID, okTo := idMap[carry.ToLessonID]
		fromID, okFrom := idMap[carry.FromLessonID]
		if !okTo || !okFrom {
			continue
		}
		carry.ToLessonID = toID
		carry.FromLessonID = fromID
		if err := insertCarry(ctx, tx, carry); err != nil {
			return "", err
		}
	}
	return first, nil
}

func insertOther(ctx context.Context, tx pgx.Tx, fx Fixture) (string, error) {
	tutorID, err := newID()
	if err != nil {
		return "", err
	}
	studentID, err := newID()
	if err != nil {
		return "", err
	}
	tutorTelegram, err := newID()
	if err != nil {
		return "", err
	}
	studentTelegram, err := newID()
	if err != nil {
		return "", err
	}
	lessonID, err := newID()
	if err != nil {
		return "", err
	}
	if err := insertAccount(ctx, tx, tutorID, "tutor", nil, tutorTelegram); err != nil {
		return "", err
	}
	if err := insertAccount(ctx, tx, studentID, "student", &tutorID, studentTelegram); err != nil {
		return "", err
	}
	if err := insertPlan(ctx, tx, studentID, fx.Subject, fx.CatalogVersion); err != nil {
		return "", err
	}
	spec := LessonSpec{
		ID:      "other-" + lessonID,
		PlaceNo: 1,
		Title:   "Чужой урок",
		State:   "planned",
	}
	if err := insertLesson(ctx, tx, studentID, fx.Subject, spec, fx.SnapshotVersion); err != nil {
		return "", err
	}
	return spec.ID, nil
}

func insertAccount(ctx context.Context, tx pgx.Tx, id, role string, owner *string, telegramID string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, $2, $3, 'active')
	`, id, role, owner); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id)
		VALUES ($1, $2)
	`, id, telegramID)
	return err
}

func insertPlan(ctx context.Context, tx pgx.Tx, studentID, subject string, version int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
		VALUES ($1, $2, $3, $4)
	`, studentID, subject, planExamAt, version)
	return err
}

func insertRevision(ctx context.Context, tx pgx.Tx, block BlockRev) error {
	doc := storedRev{
		ID:    block.ID,
		Kind:  block.Kind,
		Title: block.Title,
		Body:  block.Markdown,
	}
	if block.Widget != nil {
		params := block.Widget.Params
		if params == nil {
			params = map[string]any{}
		}
		doc.Widget = &storedWidget{Widget: block.Widget.Widget, Params: params}
		if block.Widget.Schema != nil {
			doc.WidgetSchema = block.Widget.Schema
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	_, err = tx.Exec(ctx, `
		INSERT INTO catalog.revision (entity_id, catalog_version_from, content_hash, data)
		VALUES ($1, $2, $3, $4::jsonb)
	`, block.ID, block.Version, hex.EncodeToString(sum[:]), string(raw))
	return err
}

func insertLesson(ctx context.Context, tx pgx.Tx, studentID, subject string, spec LessonSpec, snapVersion int) error {
	state := spec.State
	if state == "" {
		state = "planned"
	}
	template := "[]"
	if spec.NoBlocks {
		raw, err := json.Marshal(struct {
			LessonFile string `json:"lesson_file"`
		}{LessonFile: spec.LessonFile})
		if err != nil {
			return err
		}
		template = string(raw)
	}
	var snapshot any
	writeSnap := !spec.NoBlocks && (state == "frozen" || state == "held")
	if writeSnap {
		raw, err := snapshotJSON(spec)
		if err != nil {
			return err
		}
		snapshot = string(raw)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.lesson (
			id, student_id, subject, place_no, title, kind, template_id, state,
			no_blocks, template, snapshot
		) VALUES (
			$1, $2, $3, $4, $5, 'new', $6, $7,
			$8, $9::jsonb, $10::jsonb
		)
	`, spec.ID, studentID, subject, spec.PlaceNo, spec.Title, spec.ID, state,
		spec.NoBlocks, template, snapshot); err != nil {
		return err
	}
	if writeSnap {
		raw, err := json.Marshal(struct {
			LessonID       string `json:"lesson_id"`
			CatalogVersion int    `json:"catalog_version"`
		}{LessonID: spec.ID, CatalogVersion: snapVersion})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.journal_event (kind, payload)
			VALUES ('lesson_frozen', $1::jsonb)
		`, string(raw)); err != nil {
			return err
		}
	}
	if spec.Mark == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.lesson_mark (lesson_id, last_theory_block)
		VALUES ($1, $2)
	`, spec.ID, spec.Mark)
	return err
}

func insertCarry(ctx context.Context, tx pgx.Tx, carry CarrySpec) error {
	blocks := carry.Blocks
	if blocks == nil {
		blocks = []string{}
	}
	raw, err := json.Marshal(blocks)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.lesson_carry (to_lesson_id, from_lesson_id, blocks, practice, minutes)
		VALUES ($1, $2, $3::jsonb, '[]'::jsonb, 0)
	`, carry.ToLessonID, carry.FromLessonID, string(raw))
	return err
}

func snapshotJSON(spec LessonSpec) (string, error) {
	stages := make([]snapStage, 0, len(spec.Stages))
	for i, item := range spec.Stages {
		stage := snapStage{
			ID:     item.ID,
			Blocks: append([]string{}, item.Blocks...),
		}
		if stage.Blocks == nil {
			stage.Blocks = []string{}
		}
		if i == 0 {
			for _, variantID := range spec.Practice {
				stage.Practice = append(stage.Practice, snapSlot{VariantID: variantID})
			}
		}
		stages = append(stages, stage)
	}
	raw, err := json.Marshal(stages)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
