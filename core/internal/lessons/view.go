package lessons

import (
	"context"
	"encoding/json"
	"net/http"
)

type apiPractice struct {
	VariantID string `json:"variant_id"`
	TypeID    string `json:"type_id"`
	Role      string `json:"role,omitempty"`
	Position  int    `json:"position"`
}

type apiStage struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Kind     string        `json:"kind"`
	Minutes  []int         `json:"minutes"`
	Blocks   []string      `json:"blocks,omitempty"`
	Practice []apiPractice `json:"practice,omitempty"`
}

type apiCarried struct {
	Blocks       []string         `json:"blocks"`
	Practice     []apiPracticeRef `json:"practice"`
	FromLessonID string           `json:"from_lesson_id"`
}

type apiDetail struct {
	Lesson    apiLesson   `json:"lesson"`
	CarriedIn *apiCarried `json:"carried_in,omitempty"`
	Stages    []apiStage  `json:"stages"`
	Source    string      `json:"source"`
}

type shown struct {
	stages  []stage
	carried *carryRow
}

func project(lessons []lessonRow, carries map[string]carryRow, duration int, hasSchedule bool) map[string]shown {
	overflow := map[string]carryRow{}
	out := make(map[string]shown, len(lessons))
	for _, lesson := range lessons {
		var stages []stage
		var carried *carryRow
		if row, ok := carries[lesson.ID]; ok {
			copyRow := row
			stages = append(stages, copyRow.asStage())
			carried = &copyRow
		} else if row, ok := overflow[lesson.ID]; ok {
			copyRow := row
			stages = append(stages, copyRow.asStage())
			carried = &copyRow
		}
		stages = append(stages, baseStages(lesson)...)
		kept, over := fitStages(stages, duration, hasSchedule)
		if len(over) > 0 {
			if dest := firstOpen(lessons, lesson.ID); dest != "" {
				overflow[dest] = mergeCarry(lesson.ID, over)
			} else {
				kept = stages
			}
		}
		if kept == nil {
			kept = []stage{}
		}
		out[lesson.ID] = shown{stages: kept, carried: carried}
	}
	return out
}

func (c carryRow) asStage() stage {
	minutes := c.Minutes
	if minutes < 0 {
		minutes = 0
	}
	return stage{
		ID:       "carried-from-" + c.FromLessonID,
		Title:    "Перенос",
		Kind:     "carried",
		Minutes:  [2]int{0, minutes},
		Blocks:   append([]string(nil), c.Blocks...),
		Practice: append([]practiceSlot(nil), c.Practice...),
	}
}

func mergeCarry(from string, stages []stage) carryRow {
	row := carryRow{FromLessonID: from}
	for _, item := range stages {
		row.Blocks = append(row.Blocks, item.Blocks...)
		for _, slot := range item.Practice {
			if slot.StageID == "" {
				slot.StageID = item.ID
			}
			row.Practice = append(row.Practice, slot)
		}
		row.Minutes += item.span()
	}
	return row
}

func fitStages(stages []stage, duration int, hasSchedule bool) (kept, overflow []stage) {
	if !hasSchedule {
		return append([]stage(nil), stages...), nil
	}
	review := 0
	total := 0
	for _, item := range stages {
		if item.Kind == "homework_review" {
			review += item.span()
			continue
		}
		total += item.span()
	}
	capacity := duration - review
	if capacity < 0 {
		capacity = 0
	}
	kept = append([]stage(nil), stages...)
	for total > capacity {
		idx := -1
		for i := len(kept) - 1; i >= 0; i-- {
			if kept[i].Kind != "homework_review" {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		item := kept[idx]
		overflow = append([]stage{item}, overflow...)
		kept = append(kept[:idx], kept[idx+1:]...)
		total -= item.span()
	}
	return kept, overflow
}

func firstOpen(lessons []lessonRow, fromID string) string {
	seen := false
	for _, lesson := range lessons {
		if lesson.ID == fromID {
			seen = true
			continue
		}
		if !seen {
			continue
		}
		if lesson.State == "frozen" || lesson.State == "skipped" || lesson.State == "held" {
			continue
		}
		return lesson.ID
	}
	return ""
}

func loadCarries(ctx context.Context, db q, studentID, subject string) (map[string]carryRow, error) {
	rows, err := db.Query(ctx, `
		SELECT c.to_lesson_id, c.from_lesson_id, c.blocks, c.practice, c.minutes
		FROM core.lesson_carry AS c
		JOIN core.lesson AS lesson ON lesson.id = c.to_lesson_id
		WHERE lesson.student_id = $1 AND lesson.subject = $2
	`, studentID, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]carryRow{}
	for rows.Next() {
		var row carryRow
		var blocks, practice []byte
		if err := rows.Scan(&row.ToLessonID, &row.FromLessonID, &blocks, &practice, &row.Minutes); err != nil {
			return nil, err
		}
		if len(blocks) > 0 && string(blocks) != "null" {
			if err := json.Unmarshal(blocks, &row.Blocks); err != nil {
				return nil, err
			}
		}
		if len(practice) > 0 && string(practice) != "null" {
			if err := json.Unmarshal(practice, &row.Practice); err != nil {
				return nil, err
			}
		}
		out[row.ToLessonID] = row
	}
	return out, rows.Err()
}

func scheduleDuration(ctx context.Context, db q, studentID, subject string) (int, bool, error) {
	rule, err := loadRule(ctx, db, studentID, subject)
	if err != nil || rule == nil {
		return 0, false, err
	}
	return rule.DurationMin, true, nil
}

func detailOf(ctx context.Context, db q, lesson lessonRow) (apiDetail, error) {
	lessons, err := loadLessons(ctx, db, lesson.StudentID, lesson.Subject)
	if err != nil {
		return apiDetail{}, err
	}
	current := lesson
	for _, item := range lessons {
		if item.ID == lesson.ID {
			current = item
			break
		}
	}
	carries, err := loadCarries(ctx, db, lesson.StudentID, lesson.Subject)
	if err != nil {
		return apiDetail{}, err
	}
	duration, hasSchedule, err := scheduleDuration(ctx, db, lesson.StudentID, lesson.Subject)
	if err != nil {
		return apiDetail{}, err
	}
	shown := project(lessons, carries, duration, hasSchedule)[current.ID]
	marks, err := loadMarks(ctx, db, []string{current.ID})
	if err != nil {
		return apiDetail{}, err
	}
	detail := apiDetail{
		Lesson: lessonJSON(current, marks[current.ID]),
		Stages: toAPIStages(shown.stages),
		Source: sourceOf(current),
	}
	if shown.carried != nil {
		refs := make([]apiPracticeRef, 0, len(shown.carried.Practice))
		for _, slot := range shown.carried.Practice {
			refs = append(refs, apiPracticeRef{StageID: slot.StageID, Position: slot.Position})
		}
		blocks := shown.carried.Blocks
		if blocks == nil {
			blocks = []string{}
		}
		if refs == nil {
			refs = []apiPracticeRef{}
		}
		detail.CarriedIn = &apiCarried{
			Blocks:       blocks,
			Practice:     refs,
			FromLessonID: shown.carried.FromLessonID,
		}
	}
	return detail, nil
}

func sourceOf(lesson lessonRow) string {
	if lesson.HasSnapshot {
		return "snapshot"
	}
	return "l_file"
}

func toAPIStages(stages []stage) []apiStage {
	if len(stages) == 0 {
		return []apiStage{}
	}
	out := make([]apiStage, 0, len(stages))
	for _, item := range stages {
		practice := make([]apiPractice, 0, len(item.Practice))
		for _, slot := range item.Practice {
			practice = append(practice, apiPractice{
				VariantID: slot.VariantID,
				TypeID:    slot.TypeID,
				Role:      slot.Role,
				Position:  slot.Position,
			})
		}
		blocks := item.Blocks
		api := apiStage{
			ID:      item.ID,
			Title:   item.Title,
			Kind:    item.Kind,
			Minutes: []int{item.Minutes[0], item.Minutes[1]},
			Blocks:  blocks,
		}
		if len(practice) > 0 {
			api.Practice = practice
		}
		out = append(out, api)
	}
	return out
}

func (s *Service) getLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	lesson, err := loadLesson(r.Context(), s.pool, r.PathValue("lesson"))
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := allowStudent(r.Context(), s.pool, user.id, lesson.StudentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	detail, err := detailOf(r.Context(), s.pool, lesson)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
