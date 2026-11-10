package plan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type apiError struct {
	status int
	code   string
	detail string
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	if e.detail != "" {
		return e.code + ": " + e.detail
	}
	return e.code
}

func fail(status int, code string) error {
	return &apiError{status: status, code: code}
}

func failErr(err error) error {
	if err == nil {
		return fail(http.StatusServiceUnavailable, "unavailable")
	}
	var api *apiError
	if errors.As(err, &api) {
		return api
	}
	return &apiError{status: http.StatusServiceUnavailable, code: "unavailable", detail: err.Error()}
}

type lessonRow struct {
	ID          string
	StudentID   string
	Subject     string
	PlaceNo     int
	Title       string
	Kind        string
	TemplateID  string
	State       string
	Topics      []string
	PlannedWeek string
	StartsAt    *time.Time
	Moved       bool
	Added       bool
}

type snapLesson struct {
	ID         string
	PlaceNo    int
	TemplateID string
	Week       string
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func frozenLimit() int {
	raw := os.Getenv("LESSON_FROZEN_COUNT")
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 2
	}
	return n
}

func allowStudent(ctx context.Context, db q, actorID, studentID string) error {
	var owner *string
	err := db.QueryRow(ctx, `
		SELECT owner_tutor_id
		FROM core.app_user
		WHERE id = $1 AND role = 'student'
	`, studentID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusForbidden, "forbidden")
	}
	if err != nil {
		return err
	}
	if owner == nil || *owner != actorID {
		return fail(http.StatusForbidden, "forbidden")
	}
	return nil
}

func insertEvent(ctx context.Context, db q, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ($1, $2::jsonb)
	`, kind, string(raw))
	return err
}

func formatInstant(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func dateUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func isoWeekday(t time.Time) int {
	switch t.UTC().Weekday() {
	case time.Sunday:
		return 7
	default:
		return int(t.UTC().Weekday())
	}
}

func shiftWeek(week string, weeks int) (string, error) {
	if weeks == 0 || week == "" {
		return week, nil
	}
	parsed, err := time.Parse("2006-01-02", week)
	if err != nil {
		return "", err
	}
	return parsed.AddDate(0, 0, 7*weeks).Format("2006-01-02"), nil
}

func loadLessons(ctx context.Context, db q, studentID, subject string) ([]lessonRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, student_id, subject, place_no, title, kind, template_id, state,
		       planned_week, starts_at, moved, added,
		       COALESCE((
		           SELECT array_agg(topic ORDER BY position)
		           FROM core.lesson_topic
		           WHERE lesson_id = core.lesson.id
		       ), ARRAY[]::text[])
		FROM core.lesson
		WHERE student_id = $1 AND subject = $2
		ORDER BY place_no
	`, studentID, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []lessonRow
	for rows.Next() {
		var row lessonRow
		if err := rows.Scan(
			&row.ID, &row.StudentID, &row.Subject, &row.PlaceNo, &row.Title, &row.Kind, &row.TemplateID, &row.State,
			&row.PlannedWeek, &row.StartsAt, &row.Moved, &row.Added, &row.Topics,
		); err != nil {
			return nil, err
		}
		if row.Topics == nil {
			row.Topics = []string{}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadSnapshotLessons(ctx context.Context, db q, studentID, subject string) ([]snapLesson, error) {
	rows, err := db.Query(ctx, `
		SELECT lesson_id, place_no, template_id, planned_week
		FROM core.plan_snapshot_lesson
		WHERE student_id = $1 AND subject = $2 AND version = 0
		ORDER BY place_no
	`, studentID, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []snapLesson
	for rows.Next() {
		var row snapLesson
		if err := rows.Scan(&row.ID, &row.PlaceNo, &row.TemplateID, &row.Week); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
