package plan

import (
	"context"
	"fmt"
	"net/http"
	"sort"
)

type apiWarning struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params,omitempty"`
}

type apiLesson struct {
	ID          string   `json:"id"`
	TemplateID  string   `json:"template_id"`
	PlaceNo     int      `json:"place_no"`
	Title       string   `json:"title"`
	Kind        string   `json:"kind"`
	Topics      []string `json:"topics"`
	State       string   `json:"state"`
	StartsAt    string   `json:"starts_at"`
	PlannedWeek string   `json:"planned_week"`
	Moved       bool     `json:"moved"`
	Added       bool     `json:"added"`
}

type apiPlan struct {
	Lessons  []apiLesson  `json:"lessons"`
	Warnings []apiWarning `json:"warnings"`
}

type apiDivergence struct {
	LessonID string `json:"lesson_id"`
	Change   string `json:"change"`
	Detail   string `json:"detail"`
}

func lessonJSON(row lessonRow) apiLesson {
	topics := row.Topics
	if topics == nil {
		topics = []string{}
	}
	return apiLesson{
		ID:          row.ID,
		TemplateID:  row.TemplateID,
		PlaceNo:     row.PlaceNo,
		Title:       row.Title,
		Kind:        row.Kind,
		Topics:      topics,
		State:       row.State,
		StartsAt:    formatInstant(row.StartsAt),
		PlannedWeek: row.PlannedWeek,
		Moved:       row.Moved,
		Added:       row.Added,
	}
}

func loadPrereqs(ctx context.Context, db q, studentID, subject string) (map[string][]string, error) {
	rows, err := db.Query(ctx, `
		SELECT topic, prereq
		FROM core.topic_prereq
		WHERE student_id = $1 AND subject = $2
		ORDER BY topic, prereq
	`, studentID, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var topic, prereq string
		if err := rows.Scan(&topic, &prereq); err != nil {
			return nil, err
		}
		out[topic] = append(out[topic], prereq)
	}
	return out, rows.Err()
}

func hasSchedule(ctx context.Context, db q, studentID, subject string) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.schedule_rule
			WHERE student_id = $1 AND subject = $2
		)
	`, studentID, subject).Scan(&ok)
	return ok, err
}

func buildPlan(ctx context.Context, db q, studentID, subject string) (apiPlan, error) {
	lessons, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return apiPlan{}, err
	}
	prereqs, err := loadPrereqs(ctx, db, studentID, subject)
	if err != nil {
		return apiPlan{}, err
	}
	scheduled, err := hasSchedule(ctx, db, studentID, subject)
	if err != nil {
		return apiPlan{}, err
	}
	view := apiPlan{
		Lessons:  make([]apiLesson, 0, len(lessons)),
		Warnings: []apiWarning{},
	}
	first := map[string]int{}
	missing := 0
	for _, lesson := range lessons {
		view.Lessons = append(view.Lessons, lessonJSON(lesson))
		for _, topic := range lesson.Topics {
			if _, seen := first[topic]; !seen {
				first[topic] = lesson.PlaceNo
			}
		}
		if !scheduled || lesson.State == "held" || lesson.State == "skipped" || lesson.StartsAt != nil {
			continue
		}
		missing++
	}
	view.Warnings = append(view.Warnings, prereqWarnings(first, prereqs)...)
	if missing > 0 {
		view.Warnings = append(view.Warnings, apiWarning{
			Type:   "plan_does_not_fit",
			Text:   fmt.Sprintf("До экзамена не хватает %d занятий", missing),
			Params: map[string]any{"missing": missing},
		})
	}
	return view, nil
}

func prereqWarnings(first map[string]int, prereqs map[string][]string) []apiWarning {
	type pair struct {
		topic string
		pre   string
	}
	var pairs []pair
	for topic, deps := range prereqs {
		topicAt, ok := first[topic]
		if !ok {
			continue
		}
		for _, pre := range deps {
			preAt, seen := first[pre]
			if !seen || topicAt >= preAt {
				continue
			}
			pairs = append(pairs, pair{topic: topic, pre: pre})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].topic != pairs[j].topic {
			return pairs[i].topic < pairs[j].topic
		}
		return pairs[i].pre < pairs[j].pre
	})
	out := make([]apiWarning, 0, len(pairs))
	for _, item := range pairs {
		out = append(out, apiWarning{
			Type:   "prereq_order",
			Text:   fmt.Sprintf("Тема %s стоит раньше предпосылки %s", item.topic, item.pre),
			Params: map[string]any{"topic": item.topic, "prereq": item.pre},
		})
	}
	return out
}

func buildDivergence(ctx context.Context, db q, studentID, subject string) ([]apiDivergence, error) {
	lessons, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return nil, err
	}
	snaps, err := loadSnapshotLessons(ctx, db, studentID, subject)
	if err != nil {
		return nil, err
	}
	byID := map[string]snapLesson{}
	for _, snap := range snaps {
		byID[snap.ID] = snap
	}
	cancelled, err := cancelledLessons(ctx, db)
	if err != nil {
		return nil, err
	}
	out := []apiDivergence{}
	current := map[string]bool{}
	for _, lesson := range lessons {
		current[lesson.ID] = true
		snap, in := byID[lesson.ID]
		if !in || lesson.Added {
			out = append(out, apiDivergence{LessonID: lesson.ID, Change: "added", Detail: "добавлено"})
			if lesson.State == "skipped" {
				out = append(out, apiDivergence{LessonID: lesson.ID, Change: "skipped", Detail: "пропущен"})
			}
			continue
		}
		if lesson.Moved {
			out = append(out, apiDivergence{LessonID: lesson.ID, Change: "moved", Detail: "перенесено"})
		}
		if lesson.State == "skipped" {
			out = append(out, apiDivergence{LessonID: lesson.ID, Change: "skipped", Detail: "пропущен"})
		}
		if cancelled[lesson.ID] {
			out = append(out, apiDivergence{LessonID: lesson.ID, Change: "cancelled", Detail: "отменено"})
		}
		if lesson.PlaceNo != snap.PlaceNo {
			out = append(out, apiDivergence{LessonID: lesson.ID, Change: "reordered", Detail: "сменён порядок"})
		}
		if lesson.PlannedWeek != snap.Week {
			out = append(out, apiDivergence{LessonID: lesson.ID, Change: "week_shift", Detail: "сдвинута неделя"})
		}
	}
	for _, snap := range snaps {
		if current[snap.ID] {
			continue
		}
		out = append(out, apiDivergence{LessonID: snap.ID, Change: "cancelled", Detail: "нет в текущем плане"})
	}
	return out, nil
}

func cancelledLessons(ctx context.Context, db q) (map[string]bool, error) {
	rows, err := db.Query(ctx, `
		SELECT payload->>'lesson_id'
		FROM core.journal_event
		WHERE kind = 'lesson_cancelled'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id *string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != nil && *id != "" {
			out[*id] = true
		}
	}
	return out, rows.Err()
}

func reorder(ctx context.Context, db q, studentID, subject string, ids []string) error {
	lessons, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	byID := map[string]lessonRow{}
	for _, lesson := range lessons {
		byID[lesson.ID] = lesson
	}
	if len(ids) != len(lessons) {
		return fail(http.StatusForbidden, "forbidden")
	}
	seen := map[string]bool{}
	for i, id := range ids {
		lesson, ok := byID[id]
		if !ok || seen[id] {
			return fail(http.StatusForbidden, "forbidden")
		}
		seen[id] = true
		if lesson.PlaceNo != i+1 && (lesson.State == "frozen" || lesson.State == "held") {
			return fail(http.StatusConflict, "lesson_frozen")
		}
	}
	if _, err := db.Exec(ctx, `
		UPDATE core.lesson
		SET place_no = place_no + 1000000
		WHERE student_id = $1 AND subject = $2
	`, studentID, subject); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := db.Exec(ctx, `
			UPDATE core.lesson SET place_no = $2 WHERE id = $1
		`, id, i+1); err != nil {
			return err
		}
	}
	return insertEvent(ctx, db, "plan_reordered", struct {
		Subject   string   `json:"subject"`
		LessonIDs []string `json:"lesson_ids"`
	}{Subject: subject, LessonIDs: ids})
}
