package lessons

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type planHead struct {
	exam    time.Time
	version int
	ok      bool
}

func loadPlan(ctx context.Context, db q, studentID, subject string) (planHead, error) {
	var head planHead
	err := db.QueryRow(ctx, `
		SELECT exam_at, catalog_version
		FROM core.study_plan
		WHERE student_id = $1 AND subject = $2
	`, studentID, subject).Scan(&head.exam, &head.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return planHead{}, nil
	}
	if err != nil {
		return planHead{}, err
	}
	head.ok = true
	return head, nil
}

func loadRule(ctx context.Context, db q, studentID, subject string) (*Schedule, error) {
	var rule Schedule
	err := db.QueryRow(ctx, `
		SELECT weekdays, start_time, duration_min
		FROM core.schedule_rule
		WHERE student_id = $1 AND subject = $2
	`, studentID, subject).Scan(&rule.Weekdays, &rule.StartTime, &rule.DurationMin)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func saveSchedule(ctx context.Context, db q, studentID, subject string, rule Schedule, withEvent bool, actorRole, actorID string) error {
	if _, err := db.Exec(ctx, `
		INSERT INTO core.schedule_rule (student_id, subject, weekdays, start_time, duration_min)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (student_id, subject) DO UPDATE
		SET weekdays = EXCLUDED.weekdays,
		    start_time = EXCLUDED.start_time,
		    duration_min = EXCLUDED.duration_min
	`, studentID, subject, rule.Weekdays, rule.StartTime, rule.DurationMin); err != nil {
		return err
	}
	if !withEvent {
		return nil
	}
	return insertOwnedEvent(ctx, db, "schedule_rule_set", studentID, actorRole, actorID, struct {
		Subject     string `json:"subject"`
		Weekdays    []int  `json:"weekdays"`
		StartTime   string `json:"start_time"`
		DurationMin int    `json:"duration_min"`
	}{
		Subject:     subject,
		Weekdays:    rule.Weekdays,
		StartTime:   rule.StartTime,
		DurationMin: rule.DurationMin,
	})
}

func parseClock(value string) (int, int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
}

func validRule(rule Schedule) bool {
	if rule.DurationMin < 1 || len(rule.Weekdays) == 0 {
		return false
	}
	if _, _, ok := parseClock(rule.StartTime); !ok {
		return false
	}
	for _, day := range rule.Weekdays {
		if day < 1 || day > 7 {
			return false
		}
	}
	return true
}

func buildSlots(rule Schedule, exam, now time.Time) ([]time.Time, error) {
	hour, minute, ok := parseClock(rule.StartTime)
	if !ok {
		return nil, fail(http.StatusUnprocessableEntity, "invalid")
	}
	duration := time.Duration(rule.DurationMin) * time.Minute
	want := map[int]bool{}
	for _, day := range rule.Weekdays {
		want[day] = true
	}
	examDay := dateUTC(exam)
	var slots []time.Time
	for day := dateUTC(now); day.Before(examDay); day = day.AddDate(0, 0, 1) {
		if !want[isoWeekday(day)] {
			continue
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC)
		if start.Add(duration).After(now.UTC()) {
			slots = append(slots, start)
		}
	}
	return slots, nil
}

func layoutSubject(ctx context.Context, db q, studentID, subject string, now time.Time) error {
	head, err := loadPlan(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	if !head.ok {
		return fail(http.StatusNotFound, "not_found")
	}
	rule, err := loadRule(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	lessons, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	if rule == nil {
		return nil
	}
	slots, err := buildSlots(*rule, head.exam, now)
	if err != nil {
		return err
	}
	taken := map[int64]bool{}
	for _, lesson := range lessons {
		if lesson.Moved && lesson.StartsAt != nil {
			taken[lesson.StartsAt.UTC().Unix()] = true
		}
	}
	var assign []lessonRow
	for _, lesson := range lessons {
		if lesson.State == "held" || lesson.State == "skipped" || lesson.Moved {
			continue
		}
		assign = append(assign, lesson)
	}
	slotAt := 0
	for _, lesson := range assign {
		var start *time.Time
		for slotAt < len(slots) {
			candidate := slots[slotAt]
			slotAt++
			if taken[candidate.Unix()] {
				continue
			}
			value := candidate
			start = &value
			break
		}
		if _, err := db.Exec(ctx, `
			UPDATE core.lesson SET starts_at = $2 WHERE id = $1
		`, lesson.ID, start); err != nil {
			return err
		}
	}
	return nil
}

func freezeSubject(ctx context.Context, db q, studentID, subject string) error {
	head, err := loadPlan(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	if !head.ok {
		return nil
	}
	lessons, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	var candidates []lessonRow
	for _, lesson := range lessons {
		if lesson.State == "held" || lesson.State == "skipped" {
			continue
		}
		candidates = append(candidates, lesson)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.StartsAt == nil) != (b.StartsAt == nil) {
			return a.StartsAt != nil
		}
		if a.StartsAt != nil && b.StartsAt != nil && !a.StartsAt.Equal(*b.StartsAt) {
			return a.StartsAt.Before(*b.StartsAt)
		}
		return a.PlaceNo < b.PlaceNo
	})
	limit := frozenLimit()
	if limit > len(candidates) {
		limit = len(candidates)
	}
	chosen := map[string]bool{}
	for i := 0; i < limit; i++ {
		chosen[candidates[i].ID] = true
	}
	var fresh []lessonRow
	for _, lesson := range lessons {
		if chosen[lesson.ID] && !lesson.HasSnapshot && !lesson.NoBlocks && contentful(lesson.Template) {
			fresh = append(fresh, lesson)
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].PlaceNo < fresh[j].PlaceNo })
	pool, err := loadPool(ctx, db, studentID)
	if err != nil {
		return err
	}
	for _, lesson := range fresh {
		snap := cloneStages(lesson.Template)
		for i := range snap {
			for j := range snap[i].Practice {
				slot := &snap[i].Practice[j]
				if slot.StageID == "" {
					slot.StageID = snap[i].ID
				}
				picked, ok := pickVariant(pool, slot.TypeID, slot.VariantID)
				if !ok {
					continue
				}
				slot.VariantID = picked
				markSeen(pool, picked, slot.TypeID)
			}
		}
		raw, err := stagesJSON(snap)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			UPDATE core.lesson
			SET snapshot = $2::jsonb
			WHERE id = $1 AND snapshot IS NULL
		`, lesson.ID, raw); err != nil {
			return err
		}
		if err := insertEvent(ctx, db, "lesson_frozen", frozenEvent(lesson, snap, head.version)); err != nil {
			return err
		}
	}
	if err := savePool(ctx, db, studentID, pool); err != nil {
		return err
	}
	for _, lesson := range lessons {
		next := lesson.State
		if chosen[lesson.ID] {
			next = "frozen"
		} else if lesson.State == "frozen" {
			next = "planned"
		}
		if next == lesson.State {
			continue
		}
		if _, err := db.Exec(ctx, `UPDATE core.lesson SET state = $2 WHERE id = $1`, lesson.ID, next); err != nil {
			return err
		}
	}
	return nil
}

func loadPool(ctx context.Context, db q, studentID string) (map[string]*poolItem, error) {
	rows, err := db.Query(ctx, `
		SELECT variant_id, type_id, seen, reserve
		FROM core.variant_pool
		WHERE student_id = $1
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*poolItem{}
	for rows.Next() {
		var item poolItem
		if err := rows.Scan(&item.ID, &item.TypeID, &item.Seen, &item.Reserve); err != nil {
			return nil, err
		}
		out[item.ID] = &item
	}
	return out, rows.Err()
}

func savePool(ctx context.Context, db q, studentID string, pool map[string]*poolItem) error {
	for _, item := range pool {
		if _, err := db.Exec(ctx, `
			INSERT INTO core.variant_pool (student_id, variant_id, type_id, seen, reserve)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (student_id, variant_id) DO UPDATE
			SET type_id = EXCLUDED.type_id,
			    seen = EXCLUDED.seen,
			    reserve = EXCLUDED.reserve
		`, studentID, item.ID, item.TypeID, item.Seen, item.Reserve); err != nil {
			return err
		}
	}
	return nil
}

func pickVariant(pool map[string]*poolItem, typeID, hinted string) (string, bool) {
	cands := map[string]struct{}{}
	for id, item := range pool {
		if item.TypeID == typeID && !item.Seen && !item.Reserve {
			cands[id] = struct{}{}
		}
	}
	if hinted != "" {
		item, ok := pool[hinted]
		if !ok {
			cands[hinted] = struct{}{}
		} else if item.TypeID == typeID && !item.Seen && !item.Reserve {
			cands[hinted] = struct{}{}
		}
	}
	if len(cands) == 0 {
		return "", false
	}
	ids := make([]string, 0, len(cands))
	for id := range cands {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids[0], true
}

func markSeen(pool map[string]*poolItem, id, typeID string) {
	item, ok := pool[id]
	if !ok {
		pool[id] = &poolItem{ID: id, TypeID: typeID, Seen: true}
		return
	}
	item.Seen = true
}

type frozenStage struct {
	StageID string `json:"stage_id"`
	Minutes []int  `json:"minutes"`
}

type frozenPractice struct {
	StageID   string `json:"stage_id"`
	VariantID string `json:"variant_id"`
	TypeID    string `json:"type_id"`
	Position  int    `json:"position"`
}

func frozenEvent(lesson lessonRow, snap []stage, version int) any {
	blocks := make([]string, 0)
	stages := make([]frozenStage, 0, len(snap))
	practice := make([]frozenPractice, 0)
	for _, item := range snap {
		blocks = append(blocks, item.Blocks...)
		stages = append(stages, frozenStage{
			StageID: item.ID,
			Minutes: []int{item.Minutes[0], item.Minutes[1]},
		})
		for _, slot := range item.Practice {
			stageID := slot.StageID
			if stageID == "" {
				stageID = item.ID
			}
			practice = append(practice, frozenPractice{
				StageID:   stageID,
				VariantID: slot.VariantID,
				TypeID:    slot.TypeID,
				Position:  slot.Position,
			})
		}
	}
	return struct {
		LessonID       string           `json:"lesson_id"`
		TemplateID     string           `json:"template_id"`
		CatalogVersion int              `json:"catalog_version"`
		Blocks         []string         `json:"blocks"`
		Stages         []frozenStage    `json:"stages"`
		Practice       []frozenPractice `json:"practice"`
	}{
		LessonID:       lesson.ID,
		TemplateID:     lesson.TemplateID,
		CatalogVersion: version,
		Blocks:         blocks,
		Stages:         stages,
		Practice:       practice,
	}
}

func (s *Service) putSchedule(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	studentID := r.PathValue("id")
	subject := r.PathValue("subject")
	var rule Schedule
	if err := decodeJSON(r, &rule); err != nil || !validRule(rule) {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if err := allowStudent(r.Context(), tx, user.id, studentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	head, err := loadPlan(r.Context(), tx, studentID, subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if !head.ok {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if err := saveSchedule(r.Context(), tx, studentID, subject, rule, true, user.role, user.id); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := layoutSubject(r.Context(), tx, studentID, subject, s.cfg.Now); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := freezeSubject(r.Context(), tx, studentID, subject); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Service) getPlan(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	studentID := r.PathValue("id")
	subject := r.PathValue("subject")
	if err := allowStudent(r.Context(), s.pool, user.id, studentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	view, err := buildPlan(r.Context(), s.pool, studentID, subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Service) moveLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		StartsAt string `json:"starts_at"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	target, err := time.Parse(time.RFC3339, body.StartsAt)
	if err != nil {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	target = target.UTC()
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
	var taken bool
	if err := tx.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM core.lesson
			WHERE student_id = $1 AND subject = $2 AND id <> $3 AND starts_at = $4
		)
	`, lesson.StudentID, lesson.Subject, lesson.ID, target).Scan(&taken); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if taken {
		writeCode(w, http.StatusConflict, "slot_taken")
		return
	}
	from := formatInstant(lesson.StartsAt)
	if _, err := tx.Exec(r.Context(), `
		UPDATE core.lesson SET starts_at = $2, moved = true WHERE id = $1
	`, lesson.ID, target); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	payload := struct {
		LessonID string `json:"lesson_id"`
		From     string `json:"from,omitempty"`
		To       string `json:"to"`
	}{LessonID: lesson.ID, From: from, To: target.Format(time.RFC3339)}
	if err := insertEvent(r.Context(), tx, "lesson_moved", payload); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	lesson.StartsAt = &target
	lesson.Moved = true
	writeJSON(w, http.StatusOK, lessonJSON(lesson, nil))
}

func (s *Service) cancelLesson(w http.ResponseWriter, r *http.Request) {
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
	original := formatInstant(lesson.StartsAt)
	head, err := loadPlan(r.Context(), tx, lesson.StudentID, lesson.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	rule, err := loadRule(r.Context(), tx, lesson.StudentID, lesson.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	lessons, err := loadLessons(r.Context(), tx, lesson.StudentID, lesson.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if rule != nil && head.ok {
		slots, err := buildSlots(*rule, head.exam, s.cfg.Now)
		if err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
		index := map[int64]int{}
		for i, slot := range slots {
			index[slot.Unix()] = i
		}
		for _, item := range lessons {
			if item.PlaceNo < lesson.PlaceNo || item.StartsAt == nil {
				continue
			}
			at, ok := index[item.StartsAt.UTC().Unix()]
			if !ok {
				continue
			}
			var next *time.Time
			if at+1 < len(slots) {
				value := slots[at+1]
				next = &value
			}
			if _, err := tx.Exec(r.Context(), `
				UPDATE core.lesson SET starts_at = $2 WHERE id = $1
			`, item.ID, next); err != nil {
				writeHandlerErr(w, failErr(err))
				return
			}
		}
	}
	if err := insertEvent(r.Context(), tx, "lesson_cancelled", struct {
		LessonID string `json:"lesson_id"`
		Date     string `json:"date"`
	}{LessonID: lesson.ID, Date: original}); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	view, err := buildPlan(r.Context(), s.pool, lesson.StudentID, lesson.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type apiPracticeRef struct {
	StageID  string `json:"stage_id"`
	Position int    `json:"position"`
}

type apiMarks struct {
	LastTheoryBlock *string         `json:"last_theory_block"`
	LastPractice    *apiPracticeRef `json:"last_practice"`
}

type apiLesson struct {
	ID          string    `json:"id"`
	TemplateID  string    `json:"template_id"`
	PlaceNo     int       `json:"place_no"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind"`
	Topics      []string  `json:"topics,omitempty"`
	State       string    `json:"state"`
	StartsAt    string    `json:"starts_at,omitempty"`
	PlannedWeek string    `json:"planned_week,omitempty"`
	Marks       *apiMarks `json:"marks,omitempty"`
	Moved       bool      `json:"moved"`
	Added       bool      `json:"added"`
}

type apiWarning struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params,omitempty"`
}

type apiPlan struct {
	Lessons  []apiLesson  `json:"lessons"`
	Warnings []apiWarning `json:"warnings"`
}

func lessonJSON(row lessonRow, marks *apiMarks) apiLesson {
	return apiLesson{
		ID:          row.ID,
		TemplateID:  row.TemplateID,
		PlaceNo:     row.PlaceNo,
		Title:       row.Title,
		Kind:        row.Kind,
		State:       row.State,
		StartsAt:    formatInstant(row.StartsAt),
		PlannedWeek: row.PlannedWeek,
		Marks:       marks,
		Moved:       row.Moved,
		Added:       row.Added,
	}
}

func loadMarks(ctx context.Context, db q, ids []string) (map[string]*apiMarks, error) {
	out := map[string]*apiMarks{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `
		SELECT lesson_id, last_theory_block, practice_stage_id, practice_position, coverage
		FROM core.lesson_mark
		WHERE lesson_id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var theory, stage, coverage *string
		var position *int
		if err := rows.Scan(&id, &theory, &stage, &position, &coverage); err != nil {
			return nil, err
		}
		if coverage != nil && theory == nil && stage == nil {
			continue
		}
		marks := &apiMarks{LastTheoryBlock: theory}
		if stage != nil && position != nil {
			marks.LastPractice = &apiPracticeRef{StageID: *stage, Position: *position}
		}
		out[id] = marks
	}
	return out, rows.Err()
}

func buildPlan(ctx context.Context, db q, studentID, subject string) (apiPlan, error) {
	lessons, err := loadLessons(ctx, db, studentID, subject)
	if err != nil {
		return apiPlan{}, err
	}
	ids := make([]string, len(lessons))
	for i, lesson := range lessons {
		ids[i] = lesson.ID
	}
	marks, err := loadMarks(ctx, db, ids)
	if err != nil {
		return apiPlan{}, err
	}
	view := apiPlan{Lessons: make([]apiLesson, 0, len(lessons)), Warnings: []apiWarning{}}
	missing := 0
	for _, lesson := range lessons {
		view.Lessons = append(view.Lessons, lessonJSON(lesson, marks[lesson.ID]))
		if lesson.State == "held" || lesson.State == "skipped" {
			continue
		}
		if lesson.StartsAt == nil {
			missing++
		}
	}
	if missing > 0 {
		view.Warnings = append(view.Warnings, apiWarning{
			Type:   "plan_does_not_fit",
			Text:   "До экзамена не хватает " + strconv.Itoa(missing) + " занятий",
			Params: map[string]any{"missing": missing},
		})
	}
	return view, nil
}
