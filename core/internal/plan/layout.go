package plan

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

func parseClock(value string) (int, int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
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

func loadRule(ctx context.Context, db q, studentID, subject string) (*Schedule, error) {
	var rule Schedule
	err := db.QueryRow(ctx, `
		SELECT weekdays, start_time, duration_min
		FROM core.schedule_rule
		WHERE student_id = $1 AND subject = $2
	`, studentID, subject).Scan(&rule.Weekdays, &rule.StartTime, &rule.DurationMin)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

func loadExam(ctx context.Context, db q, studentID, subject string) (time.Time, bool, error) {
	var exam time.Time
	err := db.QueryRow(ctx, `
		SELECT exam_at
		FROM core.study_plan
		WHERE student_id = $1 AND subject = $2
	`, studentID, subject).Scan(&exam)
	if isNoRows(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return exam, true, nil
}

func layoutSubject(ctx context.Context, db q, studentID, subject string, now time.Time) error {
	exam, ok, err := loadExam(ctx, db, studentID, subject)
	if err != nil {
		return err
	}
	if !ok {
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
	slots, err := buildSlots(*rule, exam, now)
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

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
