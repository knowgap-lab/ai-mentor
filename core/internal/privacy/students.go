package privacy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type studentCard struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	BirthDate     string   `json:"birth_date,omitempty"`
	Status        string   `json:"status"`
	Consent       string   `json:"consent"`
	TelegramBound bool     `json:"telegram_bound"`
	Subjects      []string `json:"subjects"`
}

type linkResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Service) createStudent(w http.ResponseWriter, r *http.Request) {
	tutorID, ok := s.tutor(w, r)
	if !ok {
		return
	}
	body, err := readStudentCreate(r)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	id, err := s.insertStudent(r.Context(), tutorID, body)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	card, err := s.loadCard(r.Context(), id)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, card)
}

func (s *Service) getStudent(w http.ResponseWriter, r *http.Request) {
	actorID, role, ok := s.reader(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if role == "student" {
		if id != actorID {
			writeErr(w, errForbidden())
			return
		}
	} else if !s.mustOwn(w, r, actorID, id) {
		return
	}
	card, err := s.loadCard(r.Context(), id)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (s *Service) archiveStudent(w http.ResponseWriter, r *http.Request) {
	s.changeStatus(w, r, "active", "archived", "student_archived")
}

func (s *Service) unarchiveStudent(w http.ResponseWriter, r *http.Request) {
	s.changeStatus(w, r, "archived", "active", "student_unarchived")
}

func (s *Service) changeStatus(w http.ResponseWriter, r *http.Request, from, to, event string) {
	tutorID, ok := s.tutor(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !s.mustOwn(w, r, tutorID, id) {
		return
	}
	now := time.Now()
	err := s.setStatus(r.Context(), id, from, to, event, now)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) createConsentLink(w http.ResponseWriter, r *http.Request) {
	tutorID, ok := s.tutor(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !s.mustOwn(w, r, tutorID, id) {
		return
	}
	kind, err := readKind(r)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	now := time.Now().UTC()
	token, err := s.CreateLink(r.Context(), id, kind, now)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, linkResponse{
		URL:       "/consent/" + token,
		ExpiresAt: now.Add(linkTTL).Format(time.RFC3339Nano),
	})
}

func (s *Service) cancelConsentRevoke(w http.ResponseWriter, r *http.Request) {
	tutorID, ok := s.tutor(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !s.mustOwn(w, r, tutorID, id) {
		return
	}
	if err := s.apply(r.Context(), id, "", "cancel_revoke", time.Now()); err != nil {
		writeServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type studentCreate struct {
	DisplayName string   `json:"display_name"`
	BirthDate   string   `json:"birth_date"`
	Subjects    []string `json:"subjects"`
	ExamKind    string   `json:"exam_kind"`
	ExamDate    string   `json:"exam_date"`
}

func readStudentCreate(r *http.Request) (studentCreate, error) {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body studentCreate
	if err := dec.Decode(&body); err != nil {
		return studentCreate{}, errUnavailable()
	}
	if body.Subjects == nil {
		body.Subjects = []string{}
	}
	return body, nil
}

func readKind(r *http.Request) (string, error) {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body struct {
		Kind string `json:"kind"`
	}
	if err := dec.Decode(&body); err != nil {
		return "", errUnavailable()
	}
	return body.Kind, nil
}

func (s *Service) insertStudent(ctx context.Context, tutorID string, body studentCreate) (string, error) {
	subjects, err := json.Marshal(body.Subjects)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := newID()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.app_user (id, role, owner_tutor_id, status)
		VALUES ($1, 'student', $2, 'active')
	`, id, tutorID)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.student_pii (student_id, display_name, telegram_id, birth_date)
		VALUES ($1, $2, '', $3::date)
	`, id, body.DisplayName, body.BirthDate)
	if err != nil {
		return "", err
	}
	code, err := newPseudonym()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.student_pseudonym (student_id, code)
		VALUES ($1, $2)
	`, id, code)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.student_profile (student_id, subjects, exam_kind, exam_date)
		VALUES ($1, $2::jsonb, NULLIF($3, ''), NULLIF($4, '')::date)
	`, id, subjects, body.ExamKind, body.ExamDate)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func newPseudonym() (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	raw := make([]byte, len(alphabet))
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i := range out {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(out), nil
}

func (s *Service) loadCard(ctx context.Context, id string) (studentCard, error) {
	var card studentCard
	var state *string
	var subjects []byte
	err := s.pool.QueryRow(ctx, `
		SELECT u.status,
		       coalesce(p.display_name, ''),
		       coalesce(p.birth_date::text, ''),
		       c.state,
		       coalesce(pr.subjects, '[]'::jsonb),
		       (
		           EXISTS (
		               SELECT 1 FROM core.user_pii AS up
		               WHERE up.user_id = u.id AND up.telegram_id <> ''
		           )
		           OR EXISTS (
		               SELECT 1 FROM core.student_pii AS sp
		               WHERE sp.student_id = u.id AND sp.telegram_id <> ''
		           )
		       )
		FROM core.app_user AS u
		LEFT JOIN core.student_pii AS p ON p.student_id = u.id
		LEFT JOIN core.student_profile AS pr ON pr.student_id = u.id
		LEFT JOIN core.consent AS c ON c.student_id = u.id
		WHERE u.id = $1
	`, id).Scan(&card.Status, &card.DisplayName, &card.BirthDate, &state, &subjects, &card.TelegramBound)
	if err != nil {
		return studentCard{}, err
	}
	card.ID = id
	if state == nil {
		card.Consent = "none"
	} else {
		card.Consent = apiState(*state)
	}
	card.Subjects = []string{}
	if err := json.Unmarshal(subjects, &card.Subjects); err != nil {
		return studentCard{}, err
	}
	if card.Subjects == nil {
		card.Subjects = []string{}
	}
	return card, nil
}

func (s *Service) setStatus(ctx context.Context, id, from, to, event string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE core.app_user
		SET status = $3
		WHERE id = $1 AND status = $2
	`, id, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errConfirmation()
	}
	if to == "archived" {
		if err := revokeSessions(ctx, tx, id, now); err != nil {
			return err
		}
	}
	if err := insertEvent(ctx, tx, event, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
