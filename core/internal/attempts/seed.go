package attempts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	fixtureAssignmentID = "a1"
	fixtureLessonID     = "lsn1"
	fixtureTutorID      = "tutor1"
	fixtureTutorTG      = "tg-tutor"
	fixtureStudentID    = "student1"
	fixtureStudentTG    = "tg-student"
	statementVersion6   = "условие-в6"
	itemMaxPoints       = 2
)

// Seed кладёт given из attempts.examples.yaml. Числа — int, объекты — map[string]any.
func Seed(ctx context.Context, conn *pgx.Conn, given map[string]any) (World, error) {
	if given == nil {
		given = map[string]any{}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return World{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	world, err := insertActors(ctx, tx, given)
	if err != nil {
		return World{}, err
	}
	if draft, ok := given["draft"]; ok {
		if err := insertDraft(ctx, tx, draft); err != nil {
			return World{}, err
		}
		world.LessonID = fixtureLessonID
	}
	if _, ok := given["student"]; ok {
		if err := insertPrivacy(ctx, tx, world.StudentID); err != nil {
			return World{}, err
		}
	}
	if needsAssignment(given) {
		if err := insertFixture(ctx, tx, given, &world); err != nil {
			return World{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return World{}, err
	}
	return world, nil
}

func needsAssignment(given map[string]any) bool {
	for _, key := range []string{"assignment", "item", "attempt", "dispute", "student"} {
		if _, ok := given[key]; ok {
			return true
		}
	}
	return false
}

func chooseRole(given map[string]any) string {
	if _, ok := given["draft"]; ok {
		return "tutor"
	}
	if _, ok := given["attempt"]; ok {
		return "tutor"
	}
	if assignment, ok := asMap(given["assignment"]); ok {
		if _, has := assignment["student_answers"]; has {
			return "tutor"
		}
		if _, has := assignment["deadline"]; has {
			return "tutor"
		}
	}
	if item, ok := asMap(given["item"]); ok {
		if _, has := given["observations"]; has {
			return "tutor"
		}
		if item["state"] == "final" {
			return "tutor"
		}
	}
	return "student"
}

func insertActors(ctx context.Context, tx pgx.Tx, given map[string]any) (World, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'tutor', NULL, 'active')
	`, fixtureTutorID); err != nil {
		return World{}, fmt.Errorf("tutor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, fixtureTutorID, fixtureTutorTG); err != nil {
		return World{}, fmt.Errorf("tutor pii: %w", err)
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
		return World{}, fmt.Errorf("student: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
	`, ownerID, ownerTG); err != nil {
		return World{}, fmt.Errorf("student pii: %w", err)
	}

	role := chooseRole(given)
	if actor, ok := asString(given["actor"]); ok && actor != "" {
		if len(actor) >= 6 && actor[:6] == "tutor" {
			role = "tutor"
		} else {
			role = "student"
		}
	}
	world := World{StudentID: ownerID, ActorRole: role}
	switch role {
	case "tutor":
		world.ActorTelegramID = fixtureTutorTG
	default:
		world.ActorTelegramID = ownerTG
	}
	if actor, ok := asString(given["actor"]); ok && actor != "" && role == "student" && actor != ownerTG {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.app_user (id, role, owner_tutor_id, status)
			VALUES ($1, 'student', $2, 'active')
		`, actor, fixtureTutorID); err != nil {
			return World{}, fmt.Errorf("actor: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.user_pii (user_id, telegram_id) VALUES ($1, $2)
		`, actor, actor); err != nil {
			return World{}, fmt.Errorf("actor pii: %w", err)
		}
		world.ActorTelegramID = actor
	}
	return world, nil
}

func insertDraft(ctx context.Context, tx pgx.Tx, raw any) error {
	draft, ok := asMap(raw)
	if !ok {
		return fmt.Errorf("draft is %T", raw)
	}
	items, ok := asStrings(draft["items"])
	if !ok {
		return fmt.Errorf("draft items")
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.homework_draft (id, lesson_id, items)
		VALUES ('draft1', $1, $2::jsonb)
	`, fixtureLessonID, string(payload))
	return err
}

func insertPrivacy(ctx context.Context, tx pgx.Tx, studentID string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.student_pii (student_id, display_name, telegram_id)
		VALUES ($1, 'Ученик', 'tg-900')
	`, studentID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.student_pseudonym (student_id, code)
		VALUES ($1, 'M7QK2XP4')
	`, studentID)
	return err
}

type plannedAttempt struct {
	id      string
	no      int
	verdict string
	points  int
}

type plannedItem struct {
	id        string
	state     string
	maxPoints int
	statement string
	attempts  []plannedAttempt
	primary   bool
}

func insertFixture(ctx context.Context, tx pgx.Tx, given map[string]any, world *World) error {
	assignment, hasAssignment := asMap(given["assignment"])
	item, hasItem := asMap(given["item"])
	kind := "homework"
	state := "issued"
	version := 1
	var deadline *string
	if hasAssignment {
		if value, ok := asString(assignment["kind"]); ok {
			kind = value
		}
		if value, ok := asString(assignment["state"]); ok {
			state = value
		}
		if value, ok := asInt(assignment["catalog_version"]); ok {
			version = value
		}
		if value, ok := asString(assignment["deadline"]); ok {
			deadline = &value
		}
	} else if _, ok := given["dispute"]; ok {
		state = "submitted"
	}
	if version < 1 {
		version = 1
	}
	source, place := sourcePlace(kind)
	items := planItems(given, assignment, hasAssignment, item, hasItem, state, version)
	snapshot, err := snapshotJSON(items)
	if err != nil {
		return err
	}
	world.AssignmentID = fixtureAssignmentID
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		)
		VALUES ($1, $2, $3, $4, $5, $6::timestamptz, $7, $8::jsonb)
	`, fixtureAssignmentID, world.StudentID, state, kind, version, deadline, place, string(snapshot)); err != nil {
		return fmt.Errorf("assignment: %w", err)
	}

	var attemptIDs []string
	for _, planned := range items {
		if planned.primary || world.ItemID == "" {
			if planned.primary || world.ItemID == "" {
				world.ItemID = planned.id
			}
		}
		if planned.primary {
			world.ItemID = planned.id
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.assignment_item (assignment_id, item_id, state, skipped, max_points)
			VALUES ($1, $2, $3, false, $4)
		`, fixtureAssignmentID, planned.id, planned.state, planned.maxPoints); err != nil {
			return fmt.Errorf("item %s: %w", planned.id, err)
		}
		for _, attempt := range planned.attempts {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.attempts (
					id, assignment_id, item_id, raw_input, source, place, attempt_no
				)
				VALUES ($1, $2, $3, '1', $4, $5, $6)
			`, attempt.id, fixtureAssignmentID, planned.id, source, place, attempt.no); err != nil {
				return fmt.Errorf("attempt %s: %w", attempt.id, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.attempt_verdict (attempt_id, seq, verdict, points)
				VALUES ($1, 1, $2, $3)
			`, attempt.id, attempt.verdict, attempt.points); err != nil {
				return fmt.Errorf("verdict %s: %w", attempt.id, err)
			}
			attemptIDs = append(attemptIDs, attempt.id)
		}
	}
	if len(attemptIDs) == 1 {
		world.AttemptID = attemptIDs[0]
	}
	if err := applyManual(ctx, tx, given, items); err != nil {
		return err
	}
	if err := writeSeedObservations(ctx, tx, given, items); err != nil {
		return err
	}
	return insertDispute(ctx, tx, given, world.ItemID)
}

func planItems(given, assignment map[string]any, hasAssignment bool, item map[string]any, hasItem bool, state string, version int) []plannedItem {
	statement := "условие"
	if version == 6 {
		statement = statementVersion6
	}
	nextItem := 0
	nextAttempt := 0
	addItem := func(itemState string, verdicts []string) plannedItem {
		nextItem++
		planned := plannedItem{
			id:        fmt.Sprintf("i%d", nextItem),
			state:     itemState,
			maxPoints: itemMaxPoints,
			statement: statement,
		}
		for _, verdict := range verdicts {
			nextAttempt++
			planned.attempts = append(planned.attempts, plannedAttempt{
				id:      fmt.Sprintf("t%d", nextAttempt),
				no:      len(planned.attempts) + 1,
				verdict: verdict,
				points:  pointsFor(verdict),
			})
		}
		if itemState == "" {
			planned.state = "open"
			if len(verdicts) > 0 {
				planned.state = "final"
			}
		}
		return planned
	}

	if attempt, ok := asMap(given["attempt"]); ok {
		verdict, _ := asString(attempt["verdict"])
		if verdict == "" {
			verdict = "wrong"
		}
		planned := addItem("final", []string{verdict})
		planned.primary = true
		return []plannedItem{planned}
	}

	var items []plannedItem
	switch {
	case hasItem:
		verdicts, _ := asStrings(item["verdicts"])
		itemState, _ := asString(item["state"])
		if len(verdicts) == 0 {
			verdicts = []string{"wrong"}
		}
		planned := addItem(itemState, verdicts)
		planned.primary = true
		if itemState == "final" {
			planned.state = "final"
		}
		items = append(items, planned)
	case hasAssignment && hasKey(assignment, "unanswered"):
		unanswered, _ := asInt(assignment["unanswered"])
		if unanswered == 4 {
			for i := 0; i < 4; i++ {
				items = append(items, addItem("final", []string{"correct"}))
			}
			for i := 0; i < 2; i++ {
				items = append(items, addItem("final", []string{"wrong"}))
			}
		} else if unanswered == 0 {
			items = append(items, addItem("final", []string{"correct"}))
		}
		for i := 0; i < unanswered; i++ {
			items = append(items, addItem("open", nil))
		}
	case hasAssignment && hasKey(assignment, "student_answers"):
		count, _ := asInt(assignment["student_answers"])
		var verdicts []string
		for i := 0; i < count; i++ {
			verdicts = append(verdicts, "wrong")
		}
		itemState := "open"
		if count > 0 {
			itemState = "final"
		}
		items = append(items, addItem(itemState, verdicts))
	case kindOf(assignment) == "checkpoint":
		items = append(items, addItem("open", nil))
	case kindOf(assignment) == "mock":
		items = append(items, addItem("final", []string{"correct"}))
	case state == "submitted":
		items = append(items, addItem("final", nil))
	default:
		items = append(items, addItem("final", []string{"correct"}))
	}
	if _, ok := given["dispute"]; ok && !hasAttempts(items) {
		if len(items) == 0 {
			items = append(items, addItem("final", []string{"wrong"}))
		} else {
			last := &items[len(items)-1]
			nextAttempt++
			last.attempts = append(last.attempts, plannedAttempt{
				id:      fmt.Sprintf("t%d", nextAttempt),
				no:      1,
				verdict: "wrong",
				points:  0,
			})
			last.state = "final"
		}
	}
	if len(items) > 0 && !hasPrimary(items) {
		items[0].primary = true
	}
	return items
}

func hasAttempts(items []plannedItem) bool {
	for _, item := range items {
		if len(item.attempts) > 0 {
			return true
		}
	}
	return false
}

func hasPrimary(items []plannedItem) bool {
	for _, item := range items {
		if item.primary {
			return true
		}
	}
	return false
}

func kindOf(assignment map[string]any) string {
	if assignment == nil {
		return ""
	}
	kind, _ := asString(assignment["kind"])
	return kind
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func snapshotJSON(items []plannedItem) ([]byte, error) {
	type snapItem struct {
		ItemID       string `json:"item_id"`
		Statement    string `json:"statement"`
		AnswerFormat string `json:"answer_format"`
		MaxPoints    int    `json:"max_points"`
	}
	doc := struct {
		Items []snapItem `json:"items"`
	}{}
	for _, item := range items {
		doc.Items = append(doc.Items, snapItem{
			ItemID:       item.id,
			Statement:    item.statement,
			AnswerFormat: "ct.integer",
			MaxPoints:    item.maxPoints,
		})
	}
	return json.Marshal(doc)
}

func applyManual(ctx context.Context, tx pgx.Tx, given map[string]any, items []plannedItem) error {
	manual, ok := asMap(given["manual"])
	if !ok {
		return nil
	}
	attemptNo, ok := asInt(manual["attempt_no"])
	if !ok {
		return fmt.Errorf("manual attempt_no")
	}
	verdict, ok := asString(manual["verdict"])
	if !ok {
		return fmt.Errorf("manual verdict")
	}
	var attemptID string
	for _, item := range items {
		for _, attempt := range item.attempts {
			if attempt.no == attemptNo && item.primary {
				attemptID = attempt.id
			}
		}
	}
	if attemptID == "" {
		return fmt.Errorf("manual attempt %d", attemptNo)
	}
	return insertVerdict(ctx, tx, attemptID, verdict, pointsFor(verdict), nil)
}

func writeSeedObservations(ctx context.Context, tx pgx.Tx, given map[string]any, items []plannedItem) error {
	var primary plannedItem
	found := false
	for _, item := range items {
		if item.primary {
			primary = item
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	if raw, ok := given["observations"].([]any); ok {
		for _, row := range raw {
			obs, ok := asMap(row)
			if !ok {
				return fmt.Errorf("observation")
			}
			revision := 1
			if value, ok := asInt(obs["revision"]); ok {
				revision = value
			}
			first, total, maxPoints := 0, 0, itemMaxPoints
			if value, ok := asInt(obs["first_attempt_points"]); ok {
				first = value
			}
			if value, ok := asInt(obs["total_points"]); ok {
				total = value
			}
			if value, ok := asInt(obs["max_points"]); ok {
				maxPoints = value
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.observation (
					assignment_id, item_id, revision, first_attempt_points, total_points, max_points
				)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, fixtureAssignmentID, primary.id, revision, first, total, maxPoints); err != nil {
				return err
			}
		}
		return nil
	}
	if len(primary.attempts) == 0 {
		return nil
	}
	scoredRows, err := loadScored(ctx, tx, fixtureAssignmentID, primary.id)
	if err != nil {
		return err
	}
	result, ok := summarize(scoredRows)
	if !ok {
		return nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.observation (
			assignment_id, item_id, revision, first_attempt_points, total_points, max_points
		)
		VALUES ($1, $2, 1, $3, $4, $5)
	`, fixtureAssignmentID, primary.id, result.FirstAttemptPoints, result.TotalPoints, primary.maxPoints)
	return err
}

func insertDispute(ctx context.Context, tx pgx.Tx, given map[string]any, itemID string) error {
	raw, ok := given["dispute"]
	if !ok {
		return nil
	}
	dispute, ok := asMap(raw)
	if !ok {
		return fmt.Errorf("dispute")
	}
	state := "open"
	if value, ok := asString(dispute["state"]); ok && value != "" {
		state = value
	}
	if asBool(dispute["open"]) {
		state = "open"
	}
	reason := "other"
	if value, ok := asString(dispute["reason"]); ok && value != "" {
		reason = value
	}
	if itemID == "" {
		return fmt.Errorf("dispute without item")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.dispute (assignment_id, item_id, state, reason)
		VALUES ($1, $2, $3, $4)
	`, fixtureAssignmentID, itemID, state, reason)
	return err
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
