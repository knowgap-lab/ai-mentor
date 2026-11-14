package lessons

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (s *Service) addLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Subject  string `json:"subject"`
		StartsAt string `json:"starts_at"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Subject == "" || body.StartsAt == "" {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	target, err := time.Parse(time.RFC3339, body.StartsAt)
	if err != nil {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	target = target.UTC()
	studentID := r.PathValue("id")
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
	head, err := loadPlan(r.Context(), tx, studentID, body.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if !head.ok {
		writeCode(w, http.StatusUnprocessableEntity, "invalid")
		return
	}
	lessons, err := loadLessons(r.Context(), tx, studentID, body.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	for _, item := range lessons {
		if item.StartsAt != nil && item.StartsAt.UTC().Equal(target) {
			writeCode(w, http.StatusUnprocessableEntity, "invalid")
			return
		}
	}
	rule, err := loadRule(r.Context(), tx, studentID, body.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	id, err := newID()
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	place := 1
	for _, item := range lessons {
		if item.PlaceNo >= place {
			place = item.PlaceNo + 1
		}
	}
	raw, err := stagesJSON(nil)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO core.lesson (
			id, student_id, subject, place_no, title, kind, template_id, state,
			starts_at, added, template
		) VALUES (
			$1, $2, $3, $4, $5, 'new', $1, 'planned',
			$6, true, $7::jsonb
		)
	`, id, studentID, body.Subject, place, "Дополнительное занятие", target, raw); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if rule != nil {
		if err := shiftAfter(r.Context(), tx, lessons, rule, head.exam, target); err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
	}
	if err := freezeSubject(r.Context(), tx, studentID, body.Subject); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := insertEvent(r.Context(), tx, "lesson_added", struct {
		LessonID string `json:"lesson_id"`
		Subject  string `json:"subject"`
		StartsAt string `json:"starts_at"`
	}{LessonID: id, Subject: body.Subject, StartsAt: target.Format(time.RFC3339)}); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	lesson, err := loadLesson(r.Context(), s.pool, id)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusCreated, lessonJSON(lesson, nil))
}

func (s *Service) skipLesson(w http.ResponseWriter, r *http.Request) {
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
	lesson, ok := s.ownedLesson(w, r, tx, user, r.PathValue("lesson"))
	if !ok {
		return
	}
	if lesson.State == "held" {
		writeCode(w, http.StatusConflict, "lesson_frozen")
		return
	}
	if lesson.State == "skipped" {
		if err := tx.Rollback(r.Context()); err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
		view, err := buildPlan(r.Context(), s.pool, lesson.StudentID, lesson.Subject)
		if err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	original := lesson.StartsAt
	if _, err := tx.Exec(r.Context(), `
		UPDATE core.lesson SET state = 'skipped', starts_at = NULL WHERE id = $1
	`, lesson.ID); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if original != nil {
		rule, err := loadRule(r.Context(), tx, lesson.StudentID, lesson.Subject)
		if err != nil {
			writeHandlerErr(w, failErr(err))
			return
		}
		head, err := loadPlan(r.Context(), tx, lesson.StudentID, lesson.Subject)
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
			if err := shiftBefore(r.Context(), tx, lessons, lesson.ID, rule, original.UTC()); err != nil {
				writeHandlerErr(w, failErr(err))
				return
			}
		}
	}
	if err := freezeSubject(r.Context(), tx, lesson.StudentID, lesson.Subject); err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	if err := insertEvent(r.Context(), tx, "lesson_skipped", struct {
		LessonID string `json:"lesson_id"`
	}{LessonID: lesson.ID}); err != nil {
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

func (s *Service) getPreparation(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	lesson, ok := s.ownedLesson(w, r, s.pool, user, r.PathValue("lesson"))
	if !ok {
		return
	}
	head, err := loadPlan(r.Context(), s.pool, lesson.StudentID, lesson.Subject)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	view, err := preparationFromCatalog(r.Context(), s.pool, lesson, head.version)
	if err != nil {
		writeHandlerErr(w, failErr(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// ownedLesson отдаёт 403 и на чужой, и на несуществующий урок (AU-R4).
func (s *Service) ownedLesson(w http.ResponseWriter, r *http.Request, db q, user actor, id string) (lessonRow, bool) {
	lesson, err := loadLesson(r.Context(), db, id)
	if err != nil {
		var api *apiError
		if errors.As(err, &api) && api.code == "not_found" {
			writeCode(w, http.StatusForbidden, "forbidden")
			return lessonRow{}, false
		}
		writeHandlerErr(w, failErr(err))
		return lessonRow{}, false
	}
	if err := allowStudent(r.Context(), db, user.id, lesson.StudentID); err != nil {
		writeHandlerErr(w, failErr(err))
		return lessonRow{}, false
	}
	return lesson, true
}

func shiftAfter(ctx context.Context, db q, lessons []lessonRow, rule *Schedule, exam, added time.Time) error {
	examDay := dateUTC(exam)
	for _, item := range lessons {
		if item.StartsAt == nil || !item.StartsAt.UTC().After(added) {
			continue
		}
		next, ok := stepSlot(*rule, item.StartsAt.UTC(), 1)
		var stored *time.Time
		if ok && dateUTC(next).Before(examDay) {
			stored = &next
		}
		if _, err := db.Exec(ctx, `UPDATE core.lesson SET starts_at = $2 WHERE id = $1`, item.ID, stored); err != nil {
			return err
		}
	}
	return nil
}

func shiftBefore(ctx context.Context, db q, lessons []lessonRow, skipped string, rule *Schedule, from time.Time) error {
	for _, item := range lessons {
		if item.ID == skipped || item.StartsAt == nil || !item.StartsAt.UTC().After(from) {
			continue
		}
		prev, ok := stepSlot(*rule, item.StartsAt.UTC(), -1)
		var stored *time.Time
		if ok {
			stored = &prev
		}
		if _, err := db.Exec(ctx, `UPDATE core.lesson SET starts_at = $2 WHERE id = $1`, item.ID, stored); err != nil {
			return err
		}
	}
	return nil
}

func stepSlot(rule Schedule, from time.Time, dir int) (time.Time, bool) {
	hour, minute, ok := parseClock(rule.StartTime)
	if !ok || dir == 0 {
		return time.Time{}, false
	}
	want := map[int]bool{}
	for _, day := range rule.Weekdays {
		want[day] = true
	}
	day := dateUTC(from)
	for i := 0; i < 400; i++ {
		day = day.AddDate(0, 0, dir)
		if !want[isoWeekday(day)] {
			continue
		}
		slot := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC)
		if (dir > 0 && slot.After(from)) || (dir < 0 && slot.Before(from)) {
			return slot, true
		}
	}
	return time.Time{}, false
}

type preparationItem struct {
	VariantID string          `json:"variant_id"`
	Statement string          `json:"statement"`
	Key       json.RawMessage `json:"key"`
}

type preparationView struct {
	LessonID string            `json:"lesson_id"`
	Items    []preparationItem `json:"items"`
}

type catalogVariant struct {
	Statement string          `json:"statement"`
	Key       json.RawMessage `json:"key"`
}

func preparationFromCatalog(ctx context.Context, db q, lesson lessonRow, version int) (preparationView, error) {
	view := preparationView{LessonID: lesson.ID, Items: []preparationItem{}}
	ids := lessonVariantIDs(lesson)
	if len(ids) == 0 || version < 1 {
		return view, nil
	}
	stored, err := loadCatalogVariants(ctx, db, ids, version)
	if err != nil {
		return preparationView{}, err
	}
	for _, id := range ids {
		item, ok := stored[id]
		if !ok || item.Statement == "" || len(item.Key) == 0 || string(item.Key) == "null" {
			continue
		}
		view.Items = append(view.Items, preparationItem{
			VariantID: id,
			Statement: item.Statement,
			Key:       item.Key,
		})
	}
	return view, nil
}

func lessonVariantIDs(lesson lessonRow) []string {
	var ids []string
	seen := map[string]bool{}
	for _, stage := range baseStages(lesson) {
		for _, slot := range stage.Practice {
			if slot.VariantID == "" || seen[slot.VariantID] {
				continue
			}
			seen[slot.VariantID] = true
			ids = append(ids, slot.VariantID)
		}
	}
	return ids
}

func loadCatalogVariants(ctx context.Context, db q, ids []string, version int) (map[string]catalogVariant, error) {
	rows, err := db.Query(ctx, `
		SELECT DISTINCT ON (entity_id) entity_id, data
		FROM catalog.revision
		WHERE entity_id = ANY($1)
		  AND catalog_version_from <= $2
		ORDER BY entity_id, catalog_version_from DESC
	`, ids, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]catalogVariant{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var item catalogVariant
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out[id] = item
	}
	return out, rows.Err()
}
