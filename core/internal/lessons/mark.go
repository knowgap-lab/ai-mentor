package lessons

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

func (s *Service) markLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	lesson, err := loadLesson(r.Context(), tx, r.PathValue("lesson"))
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := allowStudent(r.Context(), tx, user.id, lesson.StudentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if lesson.Issued {
		writeCode(w, http.StatusConflict, "already_marked")
		return
	}
	if lesson.State != "held" && !s.lessonCurrent(r.Context(), tx, lesson) {
		writeCode(w, http.StatusConflict, "lesson_not_held")
		return
	}
	mode, coverage, theory, practice, err := readMarks(r)
	if err != nil {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	if mode == "coverage" {
		if lesson.HasSnapshot || contentful(baseStages(lesson)) {
			writeCode(w, http.StatusUnprocessableEntity, "mark_not_in_lesson")
			return
		}
		if err := applyCoverage(r.Context(), tx, lesson, coverage); err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
	} else {
		if !lesson.HasSnapshot && (lesson.NoBlocks || !contentful(baseStages(lesson))) {
			writeCode(w, http.StatusUnprocessableEntity, "mark_not_in_lesson")
			return
		}
		if err := applyMarks(r.Context(), tx, lesson, theory, practice); err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	updated, err := loadLesson(r.Context(), s.pool, lesson.ID)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	detail, err := detailOf(r.Context(), s.pool, updated)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Service) lessonCurrent(ctx context.Context, db q, lesson lessonRow) bool {
	if lesson.StartsAt == nil {
		return false
	}
	rule, err := loadRule(ctx, db, lesson.StudentID, lesson.Subject)
	if err != nil || rule == nil || rule.DurationMin <= 0 {
		return false
	}
	begin := lesson.StartsAt.UTC()
	end := begin.Add(time.Duration(rule.DurationMin) * time.Minute)
	now := s.cfg.Now.UTC()
	return !now.Before(begin) && now.Before(end)
}

func readMarks(r *http.Request) (mode, coverage string, theory *string, practice *apiPracticeRef, err error) {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	var raw map[string]json.RawMessage
	if err = dec.Decode(&raw); err != nil {
		return "", "", nil, nil, err
	}
	if body, ok := raw["coverage"]; ok {
		if err = json.Unmarshal(body, &coverage); err != nil {
			return "", "", nil, nil, err
		}
		if coverage != "all" && coverage != "none" {
			return "", "", nil, nil, errInvalid()
		}
		return "coverage", coverage, nil, nil, nil
	}
	var parsed struct {
		LastTheoryBlock *string         `json:"last_theory_block"`
		LastPractice    *apiPracticeRef `json:"last_practice"`
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", "", nil, nil, err
	}
	if err = json.Unmarshal(encoded, &parsed); err != nil {
		return "", "", nil, nil, err
	}
	return "marks", "", parsed.LastTheoryBlock, parsed.LastPractice, nil
}

func errInvalid() error {
	return fail(http.StatusUnprocessableEntity, "invalid")
}

func applyCoverage(ctx context.Context, db q, lesson lessonRow, coverage string) error {
	if _, err := db.Exec(ctx, `UPDATE core.lesson SET state = 'held' WHERE id = $1`, lesson.ID); err != nil {
		return err
	}
	if err := saveCoverage(ctx, db, lesson.ID, coverage); err != nil {
		return err
	}
	if err := writeMarked(ctx, db, lesson.ID, nil, nil, coverage); err != nil {
		return err
	}
	if err := deleteCarryFrom(ctx, db, lesson.ID); err != nil {
		return err
	}
	return deleteDraft(ctx, db, lesson.ID)
}

func applyMarks(ctx context.Context, db q, lesson lessonRow, theory *string, practice *apiPracticeRef) error {
	carries, err := loadCarries(ctx, db, lesson.StudentID, lesson.Subject)
	if err != nil {
		return err
	}
	composition := compositionOf(lesson, carries)
	outcome, err := deriveCarry(composition, theory, practice)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `UPDATE core.lesson SET state = 'held' WHERE id = $1`, lesson.ID); err != nil {
		return err
	}
	if err := saveMark(ctx, db, lesson.ID, theory, practiceRefOf(practice), ""); err != nil {
		return err
	}
	if err := writeMarked(ctx, db, lesson.ID, theory, practiceRefOf(practice), ""); err != nil {
		return err
	}
	if err := freezeSubject(ctx, db, lesson.StudentID, lesson.Subject); err != nil {
		return err
	}
	if err := deleteCarryFrom(ctx, db, lesson.ID); err != nil {
		return err
	}
	if len(outcome.blocks) > 0 || len(outcome.practice) > 0 {
		next, err := nextOpen(ctx, db, lesson)
		if err != nil {
			return err
		}
		if next != "" {
			row := carryRow{
				ToLessonID:   next,
				FromLessonID: lesson.ID,
				Blocks:       outcome.blocks,
				Practice:     outcome.practice,
				Minutes:      outcome.minutes,
			}
			if err := upsertCarry(ctx, db, row); err != nil {
				return err
			}
			if err := writeCarried(ctx, db, row); err != nil {
				return err
			}
		}
	}
	if len(outcome.homework) == 0 {
		return deleteDraft(ctx, db, lesson.ID)
	}
	return upsertDraft(ctx, db, lesson.ID, outcome.homework)
}

func compositionOf(lesson lessonRow, carries map[string]carryRow) []stage {
	var stages []stage
	if row, ok := carries[lesson.ID]; ok {
		stages = append(stages, row.asStage())
	}
	stages = append(stages, baseStages(lesson)...)
	return stages
}

type carryOutcome struct {
	blocks   []string
	practice []practiceSlot
	homework []string
	minutes  int
}

func deriveCarry(comp []stage, theory *string, practice *apiPracticeRef) (carryOutcome, error) {
	if theory == nil && practice == nil {
		var out carryOutcome
		for _, item := range comp {
			if item.Kind == "homework_review" {
				continue
			}
			out.blocks = append(out.blocks, item.Blocks...)
			for _, slot := range item.Practice {
				if slot.StageID == "" {
					slot.StageID = item.ID
				}
				out.practice = append(out.practice, slot)
			}
			out.minutes += item.span()
		}
		return out, nil
	}
	type located struct {
		slot  practiceSlot
		stage int
	}
	var blocks []string
	blockStage := map[int]int{}
	var tasks []located
	for si, item := range comp {
		for _, block := range item.Blocks {
			blockStage[len(blocks)] = si
			blocks = append(blocks, block)
		}
		for _, slot := range item.Practice {
			if slot.StageID == "" {
				slot.StageID = item.ID
			}
			tasks = append(tasks, located{slot: slot, stage: si})
		}
	}
	explainedUntil := -1
	if theory != nil {
		found := -1
		for i, block := range blocks {
			if block == *theory {
				found = i
			}
		}
		if found < 0 {
			return carryOutcome{}, fail(http.StatusUnprocessableEntity, "mark_not_in_lesson")
		}
		explainedUntil = found
	}
	solvedUntil := -1
	if practice != nil {
		found := -1
		for i, task := range tasks {
			if task.slot.StageID == practice.StageID && task.slot.Position == practice.Position {
				found = i
				break
			}
		}
		if found < 0 {
			return carryOutcome{}, fail(http.StatusUnprocessableEntity, "mark_not_in_lesson")
		}
		solvedUntil = found
	}
	var out carryOutcome
	used := map[int]bool{}
	if theory == nil {
		out.blocks = append(out.blocks, blocks...)
		for i := range blocks {
			used[blockStage[i]] = true
		}
	} else if explainedUntil+1 < len(blocks) {
		out.blocks = append(out.blocks, blocks[explainedUntil+1:]...)
		for i := explainedUntil + 1; i < len(blocks); i++ {
			used[blockStage[i]] = true
		}
	}
	for i, task := range tasks {
		if i <= solvedUntil {
			continue
		}
		blockAt := -1
		inLesson := false
		for bi, block := range blocks {
			if block == task.slot.TheoryBlock {
				blockAt = bi
				inLesson = true
			}
		}
		explained := !inLesson || blockAt <= explainedUntil
		if !explained {
			out.practice = append(out.practice, task.slot)
			used[task.stage] = true
			continue
		}
		if task.slot.VariantID != "" {
			out.homework = append(out.homework, task.slot.VariantID)
		}
	}
	for si, on := range used {
		if on {
			out.minutes += comp[si].span()
		}
	}
	return out, nil
}

func practiceRefOf(ref *apiPracticeRef) *PracticeRef {
	if ref == nil {
		return nil
	}
	return &PracticeRef{StageID: ref.StageID, Position: ref.Position}
}

func saveMark(ctx context.Context, db q, lessonID string, theory *string, practice *PracticeRef, coverage string) error {
	var stage *string
	var position *int
	if practice != nil {
		stage = &practice.StageID
		position = &practice.Position
	}
	var coveragePtr *string
	if coverage != "" {
		coveragePtr = &coverage
	}
	_, err := db.Exec(ctx, `
		INSERT INTO core.lesson_mark (lesson_id, last_theory_block, practice_stage_id, practice_position, coverage)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (lesson_id) DO UPDATE
		SET last_theory_block = EXCLUDED.last_theory_block,
		    practice_stage_id = EXCLUDED.practice_stage_id,
		    practice_position = EXCLUDED.practice_position,
		    coverage = EXCLUDED.coverage
	`, lessonID, theory, stage, position, coveragePtr)
	return err
}

func saveCoverage(ctx context.Context, db q, lessonID, coverage string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO core.lesson_mark (lesson_id, last_theory_block, practice_stage_id, practice_position, coverage)
		VALUES ($1, NULL, NULL, NULL, $2)
		ON CONFLICT (lesson_id) DO UPDATE
		SET last_theory_block = NULL,
		    practice_stage_id = NULL,
		    practice_position = NULL,
		    coverage = EXCLUDED.coverage
	`, lessonID, coverage)
	return err
}

func writeMarked(ctx context.Context, db q, lessonID string, theory *string, practice *PracticeRef, coverage string) error {
	if coverage != "" {
		return insertEvent(ctx, db, "lesson_marked", struct {
			LessonID string `json:"lesson_id"`
			Coverage string `json:"coverage"`
		}{LessonID: lessonID, Coverage: coverage})
	}
	var ref *apiPracticeRef
	if practice != nil {
		ref = &apiPracticeRef{StageID: practice.StageID, Position: practice.Position}
	}
	return insertEvent(ctx, db, "lesson_marked", struct {
		LessonID        string          `json:"lesson_id"`
		LastTheoryBlock *string         `json:"last_theory_block"`
		LastPractice    *apiPracticeRef `json:"last_practice"`
	}{LessonID: lessonID, LastTheoryBlock: theory, LastPractice: ref})
}

func deleteCarryFrom(ctx context.Context, db q, fromID string) error {
	_, err := db.Exec(ctx, `DELETE FROM core.lesson_carry WHERE from_lesson_id = $1`, fromID)
	return err
}

func upsertCarry(ctx context.Context, db q, row carryRow) error {
	blocks := row.Blocks
	if blocks == nil {
		blocks = []string{}
	}
	practice := row.Practice
	if practice == nil {
		practice = []practiceSlot{}
	}
	rawBlocks, err := json.Marshal(blocks)
	if err != nil {
		return err
	}
	rawPractice, err := json.Marshal(practice)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO core.lesson_carry (to_lesson_id, from_lesson_id, blocks, practice, minutes)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5)
		ON CONFLICT (to_lesson_id) DO UPDATE
		SET from_lesson_id = EXCLUDED.from_lesson_id,
		    blocks = EXCLUDED.blocks,
		    practice = EXCLUDED.practice,
		    minutes = EXCLUDED.minutes
	`, row.ToLessonID, row.FromLessonID, string(rawBlocks), string(rawPractice), row.Minutes)
	return err
}

func writeCarried(ctx context.Context, db q, row carryRow) error {
	blocks := row.Blocks
	if blocks == nil {
		blocks = []string{}
	}
	refs := make([]apiPracticeRef, 0, len(row.Practice))
	for _, slot := range row.Practice {
		refs = append(refs, apiPracticeRef{StageID: slot.StageID, Position: slot.Position})
	}
	return insertEvent(ctx, db, "lesson_carried_over", struct {
		FromLessonID string           `json:"from_lesson_id"`
		ToLessonID   string           `json:"to_lesson_id"`
		Blocks       []string         `json:"blocks"`
		Practice     []apiPracticeRef `json:"practice"`
	}{
		FromLessonID: row.FromLessonID,
		ToLessonID:   row.ToLessonID,
		Blocks:       blocks,
		Practice:     refs,
	})
}

func nextOpen(ctx context.Context, db q, lesson lessonRow) (string, error) {
	lessons, err := loadLessons(ctx, db, lesson.StudentID, lesson.Subject)
	if err != nil {
		return "", err
	}
	seen := false
	for _, item := range lessons {
		if item.ID == lesson.ID {
			seen = true
			continue
		}
		if seen && item.State != "skipped" {
			return item.ID, nil
		}
	}
	return "", nil
}

func deleteDraft(ctx context.Context, db q, lessonID string) error {
	_, err := db.Exec(ctx, `DELETE FROM core.homework_draft WHERE lesson_id = $1`, lessonID)
	return err
}

func upsertDraft(ctx context.Context, db q, lessonID string, ids []string) error {
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO core.homework_draft (id, lesson_id, items)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (lesson_id) DO UPDATE SET items = EXCLUDED.items
	`, lessonID, lessonID, string(raw))
	return err
}
