package conspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/knowgap-lab/ai-mentor/core/internal/pdf"
)

const sessionCookie = "__Host-session"

// Service отдаёт конспект урока и его PDF.
type Service struct {
	pool    *pgxpool.Pool
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
	OwnerID   *string
	Subject   string
	Title     string
	State     string
	NoBlocks  bool
	Template  []byte
	BlockIDs  []string
	HasSnap   bool
}

type blockOut struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Title    string     `json:"title"`
	Markdown string     `json:"markdown"`
	Widget   *widgetOut `json:"widget,omitempty"`
}

type widgetOut struct {
	Widget string         `json:"widget"`
	Params map[string]any `json:"params"`
}

type conspectOut struct {
	LessonID       string     `json:"lesson_id"`
	CatalogVersion int        `json:"catalog_version"`
	CarriedOver    []blockOut `json:"carried_over"`
	Blocks         []blockOut `json:"blocks"`
}

// Open подключается к Postgres.
func Open(ctx context.Context, cfg Config) (*Service, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	svc := &Service{pool: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lessons/{lesson}/conspect", svc.getConspect)
	mux.HandleFunc("GET /lessons/{lesson}/conspect.pdf", svc.getPDF)
	svc.handler = mux
	return svc, nil
}

// Close закрывает пул соединений.
func (s *Service) Close() error {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// Handler обслуживает конспект и его PDF.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func (s *Service) getConspect(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	out, err := s.build(r.Context(), user, r.PathValue("lesson"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) getPDF(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	lessonID := r.PathValue("lesson")
	out, err := s.build(r.Context(), user, lessonID)
	if err != nil {
		writeErr(w, err)
		return
	}
	subject, place, title, err := s.lessonHeader(r.Context(), lessonID)
	if err != nil {
		writeErr(w, err)
		return
	}
	body, err := pdf.Cached(r.Context(), s.pool, pdf.SnapshotKey{
		LessonID:       lessonID,
		CatalogVersion: out.CatalogVersion,
		Audience:       user.role,
	}, conspectHTML(subject, title, place, out))
	if errors.Is(err, pdf.ErrUnavailable) {
		writeCode(w, http.StatusServiceUnavailable, "pdf_unavailable")
		return
	}
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Service) lessonHeader(ctx context.Context, id string) (string, int, string, error) {
	var subject, title string
	var place int
	err := s.pool.QueryRow(ctx, `
		SELECT subject, place_no, title FROM core.lesson WHERE id = $1
	`, id).Scan(&subject, &place, &title)
	if err != nil {
		return "", 0, "", fail(http.StatusServiceUnavailable, "unavailable")
	}
	return subject, place, title, nil
}

func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (actor, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return actor{}, false
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

func (s *Service) build(ctx context.Context, user actor, lessonID string) (conspectOut, error) {
	row, err := s.loadLesson(ctx, lessonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return conspectOut{}, fail(http.StatusForbidden, "forbidden")
	}
	if err != nil {
		return conspectOut{}, fail(http.StatusServiceUnavailable, "unavailable")
	}
	if !canSee(user, row) {
		return conspectOut{}, fail(http.StatusForbidden, "forbidden")
	}
	if row.NoBlocks {
		return s.fallback(ctx, user, row)
	}
	if !row.HasSnap {
		return conspectOut{}, fail(http.StatusConflict, "lesson_not_frozen")
	}
	if user.role == "student" && row.State != "held" {
		return conspectOut{}, fail(http.StatusConflict, "lesson_not_held")
	}
	version, err := s.snapshotVersion(ctx, row.ID)
	if err != nil {
		return conspectOut{}, err
	}
	carryIDs, err := s.loadCarry(ctx, row.ID)
	if err != nil {
		return conspectOut{}, err
	}
	ownIDs := row.BlockIDs
	if user.role == "student" {
		mark, markErr := s.loadMark(ctx, row.ID)
		if markErr != nil {
			return conspectOut{}, markErr
		}
		ownIDs = cutMark(ownIDs, mark)
	}
	ownIDs = without(ownIDs, carryIDs)
	carried, err := s.materialize(ctx, version, carryIDs)
	if err != nil {
		return conspectOut{}, err
	}
	blocks, err := s.materialize(ctx, version, ownIDs)
	if err != nil {
		return conspectOut{}, err
	}
	return conspectOut{
		LessonID:       row.ID,
		CatalogVersion: version,
		CarriedOver:    carried,
		Blocks:         blocks,
	}, nil
}

func (s *Service) fallback(ctx context.Context, user actor, row lessonRow) (conspectOut, error) {
	var version int
	err := s.pool.QueryRow(ctx, `
		SELECT catalog_version
		FROM core.study_plan
		WHERE student_id = $1 AND subject = $2
	`, row.StudentID, row.Subject).Scan(&version)
	if err != nil {
		return conspectOut{}, fail(http.StatusServiceUnavailable, "unavailable")
	}
	out := conspectOut{
		LessonID:       row.ID,
		CatalogVersion: version,
		CarriedOver:    []blockOut{},
		Blocks:         []blockOut{},
	}
	if user.role != "tutor" {
		return out, nil
	}
	file, err := lessonFile(row.Template)
	if err != nil {
		return conspectOut{}, fail(http.StatusServiceUnavailable, "unavailable")
	}
	out.Blocks = []blockOut{{
		ID:       row.ID + ":file",
		Kind:     "theory",
		Title:    row.Title,
		Markdown: file,
	}}
	return out, nil
}

func (s *Service) loadLesson(ctx context.Context, id string) (lessonRow, error) {
	var row lessonRow
	var snapshot []byte
	err := s.pool.QueryRow(ctx, `
		SELECT l.id, l.student_id, u.owner_tutor_id, l.subject, l.title, l.state,
		       l.no_blocks, l.template, l.snapshot
		FROM core.lesson AS l
		JOIN core.app_user AS u ON u.id = l.student_id
		WHERE l.id = $1
	`, id).Scan(
		&row.ID, &row.StudentID, &row.OwnerID, &row.Subject, &row.Title, &row.State,
		&row.NoBlocks, &row.Template, &snapshot,
	)
	if err != nil {
		return lessonRow{}, err
	}
	if len(snapshot) == 0 || string(snapshot) == "null" {
		return row, nil
	}
	ids, err := snapshotBlockIDs(snapshot)
	if err != nil {
		return lessonRow{}, err
	}
	row.BlockIDs = ids
	row.HasSnap = true
	return row, nil
}

func (s *Service) snapshotVersion(ctx context.Context, lessonID string) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx, `
		SELECT (payload->>'catalog_version')::integer
		FROM core.journal_event
		WHERE kind = 'lesson_frozen'
		  AND payload->>'lesson_id' = $1
		ORDER BY journal_seq DESC
		LIMIT 1
	`, lessonID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fail(http.StatusConflict, "lesson_not_frozen")
	}
	if err != nil {
		return 0, fail(http.StatusServiceUnavailable, "unavailable")
	}
	return version, nil
}

func (s *Service) loadCarry(ctx context.Context, lessonID string) ([]string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT blocks
		FROM core.lesson_carry
		WHERE to_lesson_id = $1
	`, lessonID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fail(http.StatusServiceUnavailable, "unavailable")
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, fail(http.StatusServiceUnavailable, "unavailable")
	}
	return ids, nil
}

func (s *Service) loadMark(ctx context.Context, lessonID string) (string, error) {
	var mark *string
	err := s.pool.QueryRow(ctx, `
		SELECT last_theory_block
		FROM core.lesson_mark
		WHERE lesson_id = $1
	`, lessonID).Scan(&mark)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fail(http.StatusServiceUnavailable, "unavailable")
	}
	if mark == nil {
		return "", nil
	}
	return *mark, nil
}

func (s *Service) materialize(ctx context.Context, version int, ids []string) ([]blockOut, error) {
	out := make([]blockOut, 0, len(ids))
	for _, id := range ids {
		rev, ok, err := s.revision(ctx, id, version)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fail(http.StatusInternalServerError, "block_missing")
		}
		block, keep := present(rev)
		if keep {
			out = append(out, block)
		}
	}
	return out, nil
}

func (s *Service) revision(ctx context.Context, id string, version int) (storedRev, bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data
		FROM catalog.revision
		WHERE entity_id = $1
		  AND catalog_version_from <= $2
		ORDER BY catalog_version_from DESC
		LIMIT 1
	`, id, version).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedRev{}, false, nil
	}
	if err != nil {
		return storedRev{}, false, fail(http.StatusServiceUnavailable, "unavailable")
	}
	var rev storedRev
	if err := json.Unmarshal(raw, &rev); err != nil {
		return storedRev{}, false, fail(http.StatusServiceUnavailable, "unavailable")
	}
	return rev, true, nil
}

func present(rev storedRev) (blockOut, bool) {
	if strings.Contains(strings.ToLower(rev.Body), "<script") {
		return blockOut{}, false
	}
	switch rev.Kind {
	case "theory", "rule", "summary", "worked_example", "typical_error":
		return blockOut{
			ID:       rev.ID,
			Kind:     rev.Kind,
			Title:    rev.Title,
			Markdown: rev.Body,
		}, true
	case "widget":
		if !widgetAllowed(rev) {
			return blockOut{}, false
		}
		block := blockOut{
			ID:       rev.ID,
			Kind:     rev.Kind,
			Title:    rev.Title,
			Markdown: rev.Body,
		}
		if rev.Widget != nil {
			params := rev.Widget.Params
			if params == nil {
				params = map[string]any{}
			}
			block.Widget = &widgetOut{Widget: rev.Widget.Widget, Params: params}
		}
		return block, true
	default:
		return blockOut{}, false
	}
}

func widgetAllowed(rev storedRev) bool {
	if rev.WidgetSchema == nil {
		return true
	}
	params := map[string]any{}
	if rev.Widget != nil && rev.Widget.Params != nil {
		params = rev.Widget.Params
	}
	return paramsMatch(rev.WidgetSchema, params)
}

func paramsMatch(schema, params map[string]any) bool {
	if typ, ok := schema["type"].(string); ok && typ != "object" {
		return false
	}
	for _, key := range requiredKeys(schema) {
		if _, ok := params[key]; !ok {
			return false
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for key, rawProp := range props {
		prop, _ := rawProp.(map[string]any)
		if prop == nil || prop["type"] != "integer" {
			continue
		}
		value, ok := params[key]
		if !ok {
			continue
		}
		if !jsonInteger(value) {
			return false
		}
	}
	if deny, ok := schema["additionalProperties"].(bool); ok && !deny {
		for key := range params {
			if _, known := props[key]; !known {
				return false
			}
		}
	}
	return true
}

func requiredKeys(schema map[string]any) []string {
	switch req := schema["required"].(type) {
	case []any:
		out := make([]string, 0, len(req))
		for _, item := range req {
			key, ok := item.(string)
			if ok {
				out = append(out, key)
			}
		}
		return out
	case []string:
		return req
	default:
		return nil
	}
}

func jsonInteger(value any) bool {
	switch n := value.(type) {
	case float64:
		return !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n)
	case int:
		return true
	case int64:
		return true
	case json.Number:
		f, err := n.Float64()
		return err == nil && f == math.Trunc(f)
	default:
		return false
	}
}

func canSee(user actor, row lessonRow) bool {
	if user.role == "student" && user.id == row.StudentID {
		return true
	}
	return user.role == "tutor" && row.OwnerID != nil && *row.OwnerID == user.id
}

func cutMark(ids []string, mark string) []string {
	if mark == "" {
		return nil
	}
	for i, id := range ids {
		if id == mark {
			return ids[:i+1]
		}
	}
	return nil
}

func without(ids, skip []string) []string {
	if len(skip) == 0 {
		return ids
	}
	drop := make(map[string]bool, len(skip))
	for _, id := range skip {
		drop[id] = true
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !drop[id] {
			out = append(out, id)
		}
	}
	return out
}

func snapshotBlockIDs(raw []byte) ([]string, error) {
	var stages []snapStage
	if err := json.Unmarshal(raw, &stages); err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, stage := range stages {
		ids = append(ids, stage.Blocks...)
	}
	return ids, nil
}

func lessonFile(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	switch raw[0] {
	case '{':
		var doc struct {
			LessonFile string `json:"lesson_file"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return "", err
		}
		return doc.LessonFile, nil
	case '[':
		// Уроки пишут template массивом этапов. В этапе нет текста файла.
		var stages []struct {
			Blocks []string `json:"blocks"`
		}
		if err := json.Unmarshal(raw, &stages); err != nil {
			return "", err
		}
		return "", nil
	default:
		return "", nil
	}
}

func hashToken(token string) ([]byte, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
}

func writeErr(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		writeCode(w, api.status, api.code)
		return
	}
	writeCode(w, http.StatusServiceUnavailable, "unavailable")
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
