package homework

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sessionCookie = "__Host-session"
	appOrigin     = "http://localhost"
)

// Service собирает черновик ДЗ, выдаёт его и отзывает выдачу.
type Service struct {
	pool    *pgxpool.Pool
	cfg     Config
	handler http.Handler
}

type actor struct {
	id   string
	role string
}

type apiError struct {
	status int
	code   string
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	return e.code
}

func fail(status int, code string) error {
	return &apiError{status: status, code: code}
}

type lessonRow struct {
	ID        string
	StudentID string
	Subject   string
	PlaceNo   int
	NoBlocks  bool
	OwnerID   string
	Catalog   int
	Blob      lessonBlob
}

type tutorView struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Deadline    string `json:"deadline"`
	ItemCount   int    `json:"item_count"`
	CanWithdraw bool   `json:"can_withdraw"`
}

// Open подключается к Postgres и собирает HTTP-обработчик.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	svc := &Service{pool: pool, cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lessons/{lesson}/homework-draft", svc.getDraft)
	mux.HandleFunc("PATCH /lessons/{lesson}/homework-draft", svc.patchDraft)
	mux.HandleFunc("POST /lessons/{lesson}/homework-draft/issue", svc.issueHomework)
	mux.HandleFunc("POST /assignments/{id}/withdraw", svc.withdrawAssignment)
	svc.handler = mux
	return svc, nil
}

// Close закрывает пул соединений.
func (s *Service) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// Handler обслуживает черновик, выдачу и отзыв.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) getDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	view, err := s.ensureDraft(r.Context(), user, r.PathValue("lesson"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Service) patchDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Remove   []string `json:"remove"`
		Add      []string `json:"add"`
		Deadline *string  `json:"deadline"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid")
		return
	}
	view, err := s.doPatch(r.Context(), user, r.PathValue("lesson"), body.Remove, body.Add, body.Deadline)
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Service) issueHomework(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	view, err := s.doIssue(r.Context(), user, r.PathValue("lesson"))
	if err != nil {
		writeHandlerErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Service) withdrawAssignment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid")
		return
	}
	if err := s.doWithdraw(r.Context(), user, r.PathValue("id"), body.Reason); err != nil {
		writeHandlerErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) ensureDraft(ctx context.Context, user actor, lessonID string) (draftView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return draftView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := openLesson(ctx, tx, user, lessonID)
	if err != nil {
		return draftView{}, err
	}
	draft, err := loadOrBuild(ctx, tx, row)
	if err != nil {
		return draftView{}, err
	}
	issued, err := homeworkIssued(ctx, tx, lessonID)
	if err != nil {
		return draftView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return draftView{}, err
	}
	return draft.view(lessonID, issued), nil
}

func (s *Service) doPatch(ctx context.Context, user actor, lessonID string, remove, add []string, deadline *string) (draftView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return draftView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := openLesson(ctx, tx, user, lessonID)
	if err != nil {
		return draftView{}, err
	}
	issued, err := homeworkIssued(ctx, tx, lessonID)
	if err != nil {
		return draftView{}, err
	}
	if issued {
		return draftView{}, fail(http.StatusConflict, "already_issued")
	}
	draft, err := loadOrBuild(ctx, tx, row)
	if err != nil {
		return draftView{}, err
	}
	next, err := applyPatch(row, draft, remove, add, deadline)
	if err != nil {
		return draftView{}, err
	}
	if err := saveDraft(ctx, tx, lessonID, next); err != nil {
		return draftView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return draftView{}, err
	}
	return next.view(lessonID, false), nil
}

func (s *Service) doIssue(ctx context.Context, user actor, lessonID string) (tutorView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return tutorView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := openLesson(ctx, tx, user, lessonID)
	if err != nil {
		return tutorView{}, err
	}
	issued, err := homeworkIssued(ctx, tx, lessonID)
	if err != nil {
		return tutorView{}, err
	}
	if issued {
		return tutorView{}, fail(http.StatusConflict, "already_issued")
	}
	draft, err := loadOrBuild(ctx, tx, row)
	if err != nil {
		return tutorView{}, err
	}
	if draft.Deadline == "" {
		return tutorView{}, fail(http.StatusUnprocessableEntity, "deadline_required")
	}
	deadline, err := time.Parse(time.RFC3339, draft.Deadline)
	if err != nil {
		return tutorView{}, fail(http.StatusUnprocessableEntity, "deadline_required")
	}
	assignmentID, err := newID()
	if err != nil {
		return tutorView{}, err
	}
	snapshot, err := json.Marshal(map[string]any{
		"lesson_id": lessonID,
		"items":     listOrEmpty(draft.Items),
	})
	if err != nil {
		return tutorView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.assignment (
			id, student_id, state, kind, catalog_version, deadline, place, snapshot
		) VALUES ($1, $2, 'issued', 'homework', $3, $4, 'home', $5::jsonb)
	`, assignmentID, row.StudentID, row.Catalog, deadline.UTC(), string(snapshot)); err != nil {
		return tutorView{}, err
	}
	payloadItems := make([]map[string]any, 0, len(draft.Items))
	for _, item := range draft.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.assignment_item (assignment_id, item_id, state, max_points)
			VALUES ($1, $2, 'open', 1)
		`, assignmentID, item.VariantID); err != nil {
			return tutorView{}, err
		}
		payloadItems = append(payloadItems, map[string]any{
			"item_id":       item.VariantID,
			"variant_id":    item.VariantID,
			"answer_format": item.AnswerFormat,
			"max_points":    1,
			"source":        item.Source,
		})
	}
	payload, err := json.Marshal(map[string]any{
		"assignment_id":   assignmentID,
		"kind":            "homework",
		"subject":         row.Subject,
		"catalog_version": row.Catalog,
		"rules_version":   "1",
		"deadline":        draft.Deadline,
		"place":           "home",
		"lesson_id":       lessonID,
		"items":           payloadItems,
	})
	if err != nil {
		return tutorView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('assignment_issued', $1::jsonb)
	`, string(payload)); err != nil {
		return tutorView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorView{}, err
	}
	return tutorView{
		ID:          assignmentID,
		Kind:        "homework",
		State:       "issued",
		Deadline:    draft.Deadline,
		ItemCount:   len(draft.Items),
		CanWithdraw: true,
	}, nil
}

func (s *Service) doWithdraw(ctx context.Context, user actor, assignmentID, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var state, owner string
	err = tx.QueryRow(ctx, `
		SELECT a.state, COALESCE(u.owner_tutor_id, '')
		FROM core.assignment AS a
		JOIN core.app_user AS u ON u.id = a.student_id
		WHERE a.id = $1
		FOR UPDATE OF a
	`, assignmentID).Scan(&state, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusNotFound, "not_found")
	}
	if err != nil {
		return err
	}
	if user.role != "tutor" || user.id != owner {
		return fail(http.StatusForbidden, "forbidden")
	}
	var answers int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM core.attempts WHERE assignment_id = $1
	`, assignmentID).Scan(&answers); err != nil {
		return err
	}
	if answers > 0 {
		return fail(http.StatusConflict, "has_answers")
	}
	if state != "issued" {
		return fail(http.StatusConflict, "assignment_closed")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.assignment SET state = 'withdrawn' WHERE id = $1
	`, assignmentID); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{
		"assignment_id": assignmentID,
		"reason":        reason,
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload) VALUES ('assignment_withdrawn', $1::jsonb)
	`, string(payload)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func openLesson(ctx context.Context, tx pgx.Tx, user actor, lessonID string) (lessonRow, error) {
	row, err := loadLesson(ctx, tx, lessonID)
	if err != nil {
		return lessonRow{}, err
	}
	if user.role != "tutor" || user.id != row.OwnerID {
		return lessonRow{}, fail(http.StatusForbidden, "forbidden")
	}
	if status, code := draftGate(row); status != 0 {
		return lessonRow{}, fail(status, code)
	}
	return row, nil
}

func draftGate(row lessonRow) (int, string) {
	if row.Blob.Cancelled {
		return http.StatusConflict, "lesson_cancelled"
	}
	if row.NoBlocks && row.Blob.Coverage == "all" && row.Blob.Mark == nil {
		return 0, ""
	}
	mark := row.Blob.Mark
	if mark == nil || !mark.TheorySet || !mark.PracticeSet {
		return http.StatusConflict, "lesson_not_marked"
	}
	if mark.Theory == "" && mark.PracticePosition == 0 {
		return http.StatusConflict, "lesson_not_marked"
	}
	return 0, ""
}

func loadLesson(ctx context.Context, tx pgx.Tx, lessonID string) (lessonRow, error) {
	var row lessonRow
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT l.id, l.student_id, l.subject, l.place_no, l.no_blocks, l.template,
		       COALESCE(u.owner_tutor_id, ''), p.catalog_version
		FROM core.lesson AS l
		JOIN core.app_user AS u ON u.id = l.student_id
		JOIN core.study_plan AS p ON p.student_id = l.student_id AND p.subject = l.subject
		WHERE l.id = $1
		FOR UPDATE OF l
	`, lessonID).Scan(
		&row.ID, &row.StudentID, &row.Subject, &row.PlaceNo, &row.NoBlocks, &raw,
		&row.OwnerID, &row.Catalog,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return lessonRow{}, fail(http.StatusNotFound, "not_found")
	}
	if err != nil {
		return lessonRow{}, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &row.Blob); err != nil {
			return lessonRow{}, err
		}
	}
	return row, nil
}

func loadOrBuild(ctx context.Context, tx pgx.Tx, row lessonRow) (storedDraft, error) {
	draft, ok, err := readDraft(ctx, tx, row.ID)
	if err != nil {
		return storedDraft{}, err
	}
	if ok {
		return draft, nil
	}
	siblings, err := loadSiblings(ctx, tx, row.StudentID, row.Subject)
	if err != nil {
		return storedDraft{}, err
	}
	draft = buildDraft(row, siblings)
	if err := saveDraft(ctx, tx, row.ID, draft); err != nil {
		return storedDraft{}, err
	}
	return draft, nil
}

func readDraft(ctx context.Context, tx pgx.Tx, lessonID string) (storedDraft, bool, error) {
	var items, hints, warnings, topics []byte
	var deadline string
	err := tx.QueryRow(ctx, `
		SELECT items, deadline, hints, warnings, discuss_topics
		FROM core.homework_draft
		WHERE lesson_id = $1
	`, lessonID).Scan(&items, &deadline, &hints, &warnings, &topics)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedDraft{}, false, nil
	}
	if err != nil {
		return storedDraft{}, false, err
	}
	draft := storedDraft{Deadline: deadline}
	if err := decodeList(items, &draft.Items); err != nil {
		return storedDraft{}, false, err
	}
	if err := decodeList(hints, &draft.Hints); err != nil {
		return storedDraft{}, false, err
	}
	if err := decodeList(warnings, &draft.Warnings); err != nil {
		return storedDraft{}, false, err
	}
	if err := decodeList(topics, &draft.DiscussTopics); err != nil {
		return storedDraft{}, false, err
	}
	return draft, true, nil
}

func saveDraft(ctx context.Context, tx pgx.Tx, lessonID string, draft storedDraft) error {
	items, err := json.Marshal(listOrEmpty(draft.Items))
	if err != nil {
		return err
	}
	hints, err := json.Marshal(listOrEmpty(draft.Hints))
	if err != nil {
		return err
	}
	warnings, err := json.Marshal(listOrEmpty(draft.Warnings))
	if err != nil {
		return err
	}
	topics, err := json.Marshal(listOrEmpty(draft.DiscussTopics))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.homework_draft (
			id, lesson_id, items, deadline, hints, warnings, discuss_topics
		) VALUES ($1, $2, $3::jsonb, $4, $5::jsonb, $6::jsonb, $7::jsonb)
		ON CONFLICT (lesson_id) DO UPDATE
		SET items = EXCLUDED.items,
		    deadline = EXCLUDED.deadline,
		    hints = EXCLUDED.hints,
		    warnings = EXCLUDED.warnings,
		    discuss_topics = EXCLUDED.discuss_topics
	`, lessonID, lessonID, string(items), draft.Deadline, string(hints), string(warnings), string(topics))
	return err
}

func loadSiblings(ctx context.Context, tx pgx.Tx, studentID, subject string) ([]sibling, error) {
	rows, err := tx.Query(ctx, `
		SELECT place_no, state, starts_at
		FROM core.lesson
		WHERE student_id = $1 AND subject = $2
	`, studentID, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sibling
	for rows.Next() {
		var item sibling
		var starts *time.Time
		if err := rows.Scan(&item.PlaceNo, &item.State, &starts); err != nil {
			return nil, err
		}
		if starts != nil {
			item.StartsAt = starts.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func homeworkIssued(ctx context.Context, tx pgx.Tx, lessonID string) (bool, error) {
	var issued bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM core.assignment
			WHERE kind = 'homework'
			  AND state = 'issued'
			  AND snapshot->>'lesson_id' = $1
		)
	`, lessonID).Scan(&issued)
	return issued, err
}

func applyPatch(row lessonRow, draft storedDraft, remove, add []string, deadline *string) (storedDraft, error) {
	for _, id := range add {
		if variantSeen(row.Blob, id) {
			return storedDraft{}, fail(http.StatusConflict, "variant_seen")
		}
		if _, ok := findVariant(row.Blob, id); !ok {
			return storedDraft{}, fail(http.StatusUnprocessableEntity, "invalid")
		}
	}
	drop := map[string]bool{}
	for _, id := range remove {
		drop[id] = true
	}
	kept := make([]draftItem, 0, len(draft.Items))
	have := map[string]bool{}
	for _, item := range draft.Items {
		if drop[item.VariantID] {
			continue
		}
		kept = append(kept, item)
		have[item.VariantID] = true
	}
	for _, id := range add {
		if have[id] {
			continue
		}
		variant, _ := findVariant(row.Blob, id)
		kept = append(kept, itemFrom(variant, ""))
		have[id] = true
	}
	draft.Items = kept
	if deadline != nil {
		draft.Deadline = *deadline
	}
	return draft, nil
}

func variantSeen(blob lessonBlob, id string) bool {
	for _, item := range blob.Variants {
		if item.ID == id && item.Seen {
			return true
		}
	}
	for _, task := range blob.Tasks {
		if task.VariantID == id {
			return true
		}
	}
	for _, item := range blob.Past {
		if item.VariantID == id {
			return true
		}
	}
	return false
}

func findVariant(blob lessonBlob, id string) (Variant, bool) {
	for _, item := range blob.Variants {
		if item.ID == id {
			return item, true
		}
	}
	return Variant{}, false
}

func decodeList(raw []byte, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (actor, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		origin := s.cfg.AppOrigin
		if origin == "" {
			origin = appOrigin
		}
		if r.Header.Get("Origin") != origin {
			writeCode(w, http.StatusForbidden, "csrf")
			return actor{}, false
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(cookie.Value)) != 1 {
			writeCode(w, http.StatusForbidden, "csrf")
			return actor{}, false
		}
	}
	hash, ok := hashToken(cookie.Value)
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	var user actor
	err = s.pool.QueryRow(r.Context(), `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
	`, hash).Scan(&user.id, &user.role)
	if errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
	}
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return actor{}, false
	}
	return user, true
}

func hashToken(token string) ([]byte, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing json")
	}
	return nil
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"unavailable","message":"unavailable"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeHandlerErr(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		writeCode(w, api.status, api.code)
		return
	}
	writeCode(w, http.StatusServiceUnavailable, "unavailable")
}
