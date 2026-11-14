package internalapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/knowgap-lab/ai-mentor/core/internal/recheck"
)

// Handler обслуживает внутренние операции без токена и без cookie.
// Токен проверяет сервер до этого обработчика.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/catalog-versions", announce)
	mux.HandleFunc("POST /internal/rechecks", startRecheck)
	mux.HandleFunc("GET /internal/rechecks/{correction_id}", recheckState)
	mux.HandleFunc("POST /internal/plan-imports", importPlan)
	return mux
}

func announce(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CatalogVersion int    `json:"catalog_version"`
		Commit         string `json:"commit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CatalogVersion < 1 {
		writeCode(w, http.StatusUnprocessableEntity, "artifact_invalid")
		return
	}
	_ = body.Commit
	conn, ok := connect(w, r)
	if !ok {
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	fresh, err := announceVersion(r.Context(), conn, body.CatalogVersion)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		CatalogVersion int  `json:"catalog_version"`
		New            bool `json:"new"`
	}{CatalogVersion: body.CatalogVersion, New: fresh})
}

func announceVersion(ctx context.Context, conn *pgx.Conn, version int) (bool, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.journal_event
			WHERE kind = 'catalog_version_loaded'
			  AND payload->>'catalog_version' = $1
		)
	`, strconv.Itoa(version)).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	payload, err := json.Marshal(struct {
		CatalogVersion int `json:"catalog_version"`
	}{CatalogVersion: version})
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload)
		VALUES ('catalog_version_loaded', $1::jsonb)
	`, string(payload)); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func startRecheck(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CorrectionID int      `json:"correction_id"`
		Variants     []string `json:"variants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CorrectionID < 1 || len(body.Variants) == 0 {
		writeCode(w, http.StatusUnprocessableEntity, "variant_unknown")
		return
	}
	conn, ok := connect(w, r)
	if !ok {
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()
	state, err := recheck.Accept(r.Context(), conn, body.CorrectionID, body.Variants)
	if err != nil {
		var recErr *recheck.Error
		if errors.As(err, &recErr) && recErr.Code == "stale_correction" {
			writeCode(w, http.StatusConflict, "stale_correction")
			return
		}
		if errors.As(err, &recErr) && recErr.Code == "variant_unknown" {
			writeCode(w, http.StatusUnprocessableEntity, "variant_unknown")
			return
		}
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, state)
}

func recheckState(w http.ResponseWriter, r *http.Request) {
	correctionID, err := strconv.Atoi(r.PathValue("correction_id"))
	if err != nil || correctionID < 1 {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	conn, ok := connect(w, r)
	if !ok {
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()
	var state recheck.State
	err = conn.QueryRow(r.Context(), `
		SELECT correction_id, state, total, processed, changed, skipped_manual,
		       COALESCE(cursor, '')
		FROM core.recheck_batch
		WHERE correction_id = $1
	`, correctionID).Scan(
		&state.CorrectionID, &state.State, &state.Total, &state.Processed,
		&state.Changed, &state.SkippedManual, &state.Cursor,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type planLesson struct {
	Subject     string   `json:"subject"`
	Lesson      int      `json:"lesson"`
	PlannedWeek string   `json:"planned_week"`
	Phase       string   `json:"phase"`
	Kind        string   `json:"kind"`
	Topics      []string `json:"topics"`
	Review      []string `json:"review"`
	Title       string   `json:"title"`
	SourcePath  string   `json:"source_path"`
	SourceHash  string   `json:"source_hash"`
}

type planRequest struct {
	StudentID  string       `json:"student_id"`
	Subject    string       `json:"subject"`
	ShiftWeeks int          `json:"shift_weeks"`
	Lessons    []planLesson `json:"lessons"`
	Snapshot   struct {
		SourceHashes map[string]string `json:"source_hashes"`
	} `json:"snapshot"`
}

var planKinds = map[string]bool{
	"diagnostic": true, "new": true, "consolidation": true,
	"review": true, "mock": true, "analysis": true,
}

func importPlan(w http.ResponseWriter, r *http.Request) {
	var body planRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StudentID == "" || body.Subject == "" || len(body.Lessons) == 0 {
		writeCode(w, http.StatusUnprocessableEntity, "frontmatter_invalid")
		return
	}
	if code := validatePlan(body); code != "" {
		writeCode(w, http.StatusUnprocessableEntity, code)
		return
	}
	conn, ok := connect(w, r)
	if !ok {
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()
	n, code, status, err := writePlan(r.Context(), conn, body)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if code != "" {
		writeCode(w, status, code)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Lessons int `json:"lessons"`
	}{Lessons: n})
}

func validatePlan(body planRequest) string {
	dated := make([]planLesson, len(body.Lessons))
	copy(dated, body.Lessons)
	for i := range dated {
		if !planKinds[dated[i].Kind] {
			return "kind_unknown"
		}
		day, err := time.Parse("2006-01-02", dated[i].PlannedWeek)
		if err != nil || day.Weekday() != time.Monday {
			return "week_not_monday"
		}
	}
	slices.SortFunc(dated, func(a, b planLesson) int { return a.Lesson - b.Lesson })
	for i := 1; i < len(dated); i++ {
		if dated[i].Lesson != dated[i-1].Lesson+1 {
			return "lesson_gap"
		}
		prev, _ := time.Parse("2006-01-02", dated[i-1].PlannedWeek)
		next, _ := time.Parse("2006-01-02", dated[i].PlannedWeek)
		if !next.After(prev) {
			return "week_not_increasing"
		}
	}
	introduced := map[string]int{}
	for _, lesson := range body.Lessons {
		for _, topic := range lesson.Topics {
			if n, ok := introduced[topic]; !ok || lesson.Lesson < n {
				introduced[topic] = lesson.Lesson
			}
		}
	}
	for _, lesson := range body.Lessons {
		for _, topic := range lesson.Review {
			if n, ok := introduced[topic]; ok && n >= lesson.Lesson {
				return "review_before_intro"
			}
		}
	}
	return ""
}

func writePlan(ctx context.Context, conn *pgx.Conn, body planRequest) (int, string, int, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, "", 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var student bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.app_user WHERE id = $1 AND role = 'student'
		)
	`, body.StudentID).Scan(&student); err != nil {
		return 0, "", 0, err
	}
	if !student {
		return 0, "forbidden", http.StatusForbidden, nil
	}
	var imported bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.study_plan WHERE student_id = $1 AND subject = $2
		)
	`, body.StudentID, body.Subject).Scan(&imported); err != nil {
		return 0, "", 0, err
	}
	if imported {
		return 0, "already_imported", http.StatusConflict, nil
	}

	hashes, err := json.Marshal(body.Snapshot.SourceHashes)
	if err != nil {
		return 0, "", 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.study_plan (student_id, subject, exam_at, catalog_version)
		VALUES ($1, $2, $3, 1)
	`, body.StudentID, body.Subject, time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return 0, "", 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.plan_snapshot (student_id, subject, version, source_hashes)
		VALUES ($1, $2, 0, $3::jsonb)
	`, body.StudentID, body.Subject, string(hashes)); err != nil {
		return 0, "", 0, err
	}
	for _, lesson := range body.Lessons {
		day, err := time.Parse("2006-01-02", lesson.PlannedWeek)
		if err != nil {
			return 0, "", 0, err
		}
		week := day.AddDate(0, 0, 7*body.ShiftWeeks).Format("2006-01-02")
		subject := lesson.Subject
		if subject == "" {
			subject = body.Subject
		}
		id := fmt.Sprintf("%s:%s:%d", body.StudentID, subject, lesson.Lesson)
		_ = lesson.Phase
		_ = lesson.SourceHash
		templateID := lesson.SourcePath
		if templateID == "" {
			templateID = id
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.lesson (
				id, student_id, subject, place_no, title, kind, template_id, state,
				planned_week, template
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 'planned', $8, '[]'::jsonb)
		`, id, body.StudentID, subject, lesson.Lesson, lesson.Title, lesson.Kind, templateID, week); err != nil {
			return 0, "", 0, err
		}
		for i, topic := range lesson.Topics {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.lesson_topic (lesson_id, position, topic)
				VALUES ($1, $2, $3)
			`, id, i+1, topic); err != nil {
				return 0, "", 0, err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.plan_snapshot_lesson (
				student_id, subject, version, place_no, lesson_id, template_id, planned_week
			) VALUES ($1, $2, 0, $3, $4, $5, $6)
		`, body.StudentID, subject, lesson.Lesson, id, templateID, week); err != nil {
			return 0, "", 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", 0, err
	}
	return len(body.Lessons), "", 0, nil
}

func connect(w http.ResponseWriter, r *http.Request) (*pgx.Conn, bool) {
	conn, err := pgx.Connect(r.Context(), os.Getenv("DATABASE_URL"))
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return nil, false
	}
	return conn, true
}

func writeCode(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"code":"unavailable","message":"unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
