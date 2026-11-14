package answercheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	fixtureAssignmentID = "a1"
	fixtureItemID       = "i1"
	fixtureTutorID      = "tutor1"
	fixtureTutorTG      = "tg-tutor"
	fixtureStudentID    = "student1"
	fixtureStudentTG    = "tg-student"
)

type storedItem struct {
	ItemID        string            `json:"item_id"`
	Part          string            `json:"part,omitempty"`
	Format        string            `json:"format,omitempty"`
	AnswerFormat  string            `json:"answer_format,omitempty"`
	Key           json.RawMessage   `json:"key,omitempty"`
	SolutionBlock string            `json:"solution_block,omitempty"`
	VariantID     string            `json:"variant_id,omitempty"`
	RulesVersion  string            `json:"rules_version,omitempty"`
	Traps         map[string]string `json:"traps,omitempty"`
}

type storedSnap struct {
	Items []storedItem `json:"items"`
}

// Seed кладёт given из answer-check.examples.yaml.
// Числа приходят как int, вложенные объекты как map[string]any, списки как []any.
func Seed(ctx context.Context, conn *pgx.Conn, given map[string]any) (World, error) {
	if given == nil {
		given = map[string]any{}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return World{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	world, actorUserID, err := insertActors(ctx, tx, given)
	if err != nil {
		return World{}, err
	}
	if err := insertAssignment(ctx, tx, given, &world); err != nil {
		return World{}, err
	}
	if n, ok := asInt(given["requests_in_last_minute"]); ok && n > 0 {
		if err := insertRate(ctx, tx, actorUserID, n); err != nil {
			return World{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return World{}, err
	}
	return world, nil
}

func insertActors(ctx context.Context, tx pgx.Tx, given map[string]any) (World, string, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'tutor', NULL, 'active')
	`, fixtureTutorID); err != nil {
		return World{}, "", fmt.Errorf("tutor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, fixtureTutorID, fixtureTutorTG); err != nil {
		return World{}, "", fmt.Errorf("tutor pii: %w", err)
	}

	ownerID := fixtureStudentID
	ownerTG := fixtureStudentTG
	if assignment, ok := asMap(given["assignment"]); ok {
		if owner, ok := asString(assignment["owner"]); ok && owner != "" {
			ownerID = owner
			ownerTG = owner
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, ownerID, fixtureTutorID); err != nil {
		return World{}, "", fmt.Errorf("student: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, ownerID, ownerTG); err != nil {
		return World{}, "", fmt.Errorf("student pii: %w", err)
	}

	world := World{
		StudentID:       ownerID,
		ActorRole:       "student",
		ActorTelegramID: ownerTG,
	}
	actorUserID := ownerID
	if actor, ok := asString(given["actor"]); ok && actor != "" && actor != ownerID {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.app_user (id, role, owner_tutor_id, status)
			VALUES ($1, 'student', $2, 'active')
		`, actor, fixtureTutorID); err != nil {
			return World{}, "", fmt.Errorf("actor: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
		`, actor, actor); err != nil {
			return World{}, "", fmt.Errorf("actor pii: %w", err)
		}
		world.ActorTelegramID = actor
		actorUserID = actor
	}
	return world, actorUserID, nil
}

func insertAssignment(ctx context.Context, tx pgx.Tx, given map[string]any, world *World) error {
	assignment, _ := asMap(given["assignment"])
	item, hasItem := asMap(given["item"])
	kind := "homework"
	state := "issued"
	version := 1
	var deadline *time.Time
	if value, ok := asString(assignment["kind"]); ok && value != "" {
		kind = value
	}
	if value, ok := asString(assignment["state"]); ok && value != "" {
		state = value
	}
	if value, ok := asInt(assignment["catalog_version"]); ok && value >= 1 {
		version = value
	}
	if asBool(assignment["deadline_passed"]) {
		past := time.Now().Add(-time.Hour).UTC()
		deadline = &past
	}
	source, place := sourcePlace(kind)

	spec := readItem(item, hasItem)
	keyRaw, err := marshalKey(spec.key)
	if err != nil {
		return err
	}
	snap, err := json.Marshal(storedSnap{Items: []storedItem{{
		ItemID:        fixtureItemID,
		Part:          spec.part,
		Format:        spec.format,
		Key:           keyRaw,
		SolutionBlock: solutionBlock,
		VariantID:     variantID,
		RulesVersion:  rulesVersion,
	}}})
	if err != nil {
		return err
	}
	world.AssignmentID = fixtureAssignmentID
	world.ItemID = fixtureItemID
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, fixtureAssignmentID, world.StudentID, state, kind, version, deadline, place, string(snap)); err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	maxPoints := maxFor(spec.format)
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
		VALUES ($1, $2, $3, false, $4)
	`, fixtureAssignmentID, fixtureItemID, spec.state, maxPoints); err != nil {
		return fmt.Errorf("item: %w", err)
	}
	n := spec.attempts
	if len(spec.verdicts) > n {
		n = len(spec.verdicts)
	}
	for i := 0; i < n; i++ {
		verdict := "wrong"
		if i < len(spec.verdicts) {
			verdict = spec.verdicts[i]
		}
		attemptID := fmt.Sprintf("t%d", i+1)
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.attempts (
				id, assignment_id, item_id, raw_input, source, place, attempt_no
			)
			VALUES ($1, $2, $3, '0', $4, $5, $6)
		`, attemptID, fixtureAssignmentID, fixtureItemID, source, place, i+1); err != nil {
			return fmt.Errorf("attempt: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
			VALUES ($1, 1, $2, $3)
		`, attemptID, verdict, seedPoints(spec.format, verdict)); err != nil {
			return fmt.Errorf("verdict: %w", err)
		}
	}
	return nil
}

type itemSpec struct {
	part     string
	format   string
	key      any
	state    string
	attempts int
	verdicts []string
}

func readItem(item map[string]any, hasItem bool) itemSpec {
	spec := itemSpec{state: "open"}
	if !hasItem {
		spec.part = "B"
		spec.format = "ct.integer"
		spec.key = 12
		return spec
	}
	if value, ok := asString(item["part"]); ok {
		spec.part = value
	}
	if value, ok := asString(item["format"]); ok && value != "" {
		spec.format = value
	}
	if value, ok := asString(item["answer_format"]); ok && value != "" {
		spec.format = value
	}
	if value, ok := item["key"]; ok {
		spec.key = normalizeKey(value)
	}
	if value, ok := asString(item["state"]); ok && value != "" {
		spec.state = value
	}
	if value, ok := asInt(item["attempts"]); ok {
		spec.attempts = value
	}
	if value, ok := asStrings(item["verdicts"]); ok {
		spec.verdicts = value
	}
	return spec
}

func normalizeKey(v any) any {
	items, ok := v.([]any)
	if !ok {
		return v
	}
	nums := make([]int, 0, len(items))
	for _, item := range items {
		n, ok := asInt(item)
		if !ok {
			return v
		}
		nums = append(nums, n)
	}
	return nums
}

func marshalKey(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func insertRate(ctx context.Context, tx pgx.Tx, userID string, n int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO core.answer_rate (student_id)
		SELECT $1
		FROM generate_series(1, $2)
	`, userID, n)
	return err
}

func seedPoints(format, verdict string) int {
	switch verdict {
	case "correct":
		return maxFor(format)
	case "partial":
		return 1
	default:
		return 0
	}
}

func sourcePlace(kind string) (string, string) {
	switch kind {
	case "checkpoint":
		return "checkpoint", "lesson"
	case "mock":
		return "mock", "home"
	case "diagnostics":
		return "diagnostics", "home"
	default:
		return "homework", "home"
	}
}

func maxFor(format string) int {
	switch format {
	case "ct.choice1", "ct.choiceN":
		return 1
	case "ct.integer", "ct.digits", "ct.match":
		return 2
	default:
		return 1
	}
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func asBool(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func asStrings(v any) ([]string, bool) {
	switch raw := v.(type) {
	case []string:
		return raw, true
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}
