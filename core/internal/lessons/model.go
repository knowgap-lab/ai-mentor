package lessons

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

type practiceSlot struct {
	StageID     string `json:"stage_id,omitempty"`
	VariantID   string `json:"variant_id"`
	TypeID      string `json:"type_id"`
	Role        string `json:"role,omitempty"`
	Position    int    `json:"position"`
	TheoryBlock string `json:"theory_block,omitempty"`
}

type stage struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Kind     string         `json:"kind"`
	Minutes  [2]int         `json:"minutes"`
	Blocks   []string       `json:"blocks,omitempty"`
	Practice []practiceSlot `json:"practice,omitempty"`
}

func (s stage) span() int {
	n := s.Minutes[1] - s.Minutes[0]
	if n < 0 {
		return 0
	}
	return n
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
	PlannedWeek string
	StartsAt    *time.Time
	Moved       bool
	Added       bool
	NoBlocks    bool
	Issued      bool
	Template    []stage
	Snapshot    []stage
	HasSnapshot bool
}

type carryRow struct {
	ToLessonID   string
	FromLessonID string
	Blocks       []string
	Practice     []practiceSlot
	Minutes      int
}

type poolItem struct {
	ID      string
	TypeID  string
	Seen    bool
	Reserve bool
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

func mustStages(raw []byte) ([]stage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var stages []stage
	if err := json.Unmarshal(raw, &stages); err != nil {
		return nil, err
	}
	return stages, nil
}

func stagesJSON(stages []stage) (string, error) {
	if stages == nil {
		stages = []stage{}
	}
	raw, err := json.Marshal(stages)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func fromSpecs(specs []StageSpec) []stage {
	out := make([]stage, 0, len(specs))
	for _, spec := range specs {
		slots := make([]practiceSlot, 0, len(spec.Practice))
		for _, item := range spec.Practice {
			slots = append(slots, practiceSlot{
				StageID:     spec.ID,
				VariantID:   item.VariantID,
				TypeID:      item.TypeID,
				Role:        item.Role,
				Position:    item.Position,
				TheoryBlock: item.TheoryBlock,
			})
		}
		out = append(out, stage{
			ID:       spec.ID,
			Title:    spec.Title,
			Kind:     spec.Kind,
			Minutes:  spec.Minutes,
			Blocks:   append([]string(nil), spec.Blocks...),
			Practice: slots,
		})
	}
	return out
}

func cloneStages(in []stage) []stage {
	out := make([]stage, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Blocks = append([]string(nil), item.Blocks...)
		out[i].Practice = append([]practiceSlot(nil), item.Practice...)
	}
	return out
}

func contentful(stages []stage) bool {
	for _, item := range stages {
		if len(item.Blocks) > 0 || len(item.Practice) > 0 {
			return true
		}
	}
	return false
}

func baseStages(row lessonRow) []stage {
	if row.HasSnapshot {
		return cloneStages(row.Snapshot)
	}
	if row.NoBlocks {
		return nil
	}
	return cloneStages(row.Template)
}

const lessonColumns = `
	id, student_id, subject, place_no, title, kind, template_id, state,
	planned_week, starts_at, moved, added, no_blocks, issued, template, snapshot`

func scanLesson(scan func(dest ...any) error) (lessonRow, error) {
	var row lessonRow
	var template, snapshot []byte
	if err := scan(
		&row.ID, &row.StudentID, &row.Subject, &row.PlaceNo, &row.Title, &row.Kind, &row.TemplateID, &row.State,
		&row.PlannedWeek, &row.StartsAt, &row.Moved, &row.Added, &row.NoBlocks, &row.Issued, &template, &snapshot,
	); err != nil {
		return lessonRow{}, err
	}
	stages, err := mustStages(template)
	if err != nil {
		return lessonRow{}, err
	}
	row.Template = stages
	if len(snapshot) > 0 && string(snapshot) != "null" {
		snap, err := mustStages(snapshot)
		if err != nil {
			return lessonRow{}, err
		}
		row.Snapshot = snap
		row.HasSnapshot = true
	}
	return row, nil
}

func loadLesson(ctx context.Context, db q, id string) (lessonRow, error) {
	row := db.QueryRow(ctx, `SELECT `+lessonColumns+` FROM core.lesson WHERE id = $1`, id)
	item, err := scanLesson(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return lessonRow{}, fail(http.StatusNotFound, "not_found")
	}
	if err != nil {
		return lessonRow{}, err
	}
	return item, nil
}

func loadLessons(ctx context.Context, db q, studentID, subject string) ([]lessonRow, error) {
	rows, err := db.Query(ctx, `
		SELECT `+lessonColumns+`
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
		item, err := scanLesson(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
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
	return insertOwnedEvent(ctx, db, kind, "", "", "", payload)
}

func insertOwnedEvent(ctx context.Context, db q, kind, studentID, actorRole, actorID string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload, student_id, actor_role, actor_id)
		VALUES ($1, $2::jsonb, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''))
	`, kind, string(raw), studentID, actorRole, actorID)
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
