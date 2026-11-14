package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type consentPage struct {
	TextVersion string  `json:"text_version"`
	Text        string  `json:"text"`
	Kind        string  `json:"kind"`
	State       string  `json:"state"`
	AnonymiseAt *string `json:"anonymise_at,omitempty"`
}

func (s *Service) getConsent(w http.ResponseWriter, r *http.Request) {
	row, err := s.findLink(r.Context(), r.PathValue("token"), time.Now())
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	page, err := s.loadPage(r.Context(), row.studentID, row.kind)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Service) postConsent(w http.ResponseWriter, r *http.Request) {
	if !s.consentOriginOK(r) {
		writeErr(w, errCSRF())
		return
	}
	now := time.Now()
	row, err := s.findLink(r.Context(), r.PathValue("token"), now)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	action, err := readAction(r)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	if err := s.apply(r.Context(), row.studentID, row.kind, action, now); err != nil {
		writeServiceErr(w, err)
		return
	}
	page, err := s.loadPage(r.Context(), row.studentID, row.kind)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func readAction(r *http.Request) (string, error) {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body struct {
		Action string `json:"action"`
	}
	if err := dec.Decode(&body); err != nil {
		return "", errUnavailable()
	}
	switch body.Action {
	case "give", "revoke", "cancel_revoke":
		return body.Action, nil
	default:
		return "", errUnavailable()
	}
}

func (s *Service) apply(ctx context.Context, studentID, kind, action string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	switch action {
	case "give":
		// PV-R10: ссылка многократна только до отзыва. PV-R18: вернуть согласие
		// может только отмена, и лишь пока не вышли 7 суток (anonymiseIn).
		// give после отзыва и после обезличивания запись не оживляет.
		blocked, blockErr := giveLeavesConsent(ctx, tx, studentID)
		if blockErr != nil {
			return blockErr
		}
		if blocked {
			return nil
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO core.consent (
				student_id, version, state, granted_at, revoked_at, anonymise_at, granted_by, method
			) VALUES ($1, $2, 'active', $3, NULL, NULL, $4, 'link_button')
			ON CONFLICT (student_id) DO UPDATE
			SET version = EXCLUDED.version,
			    state = 'active',
			    granted_at = EXCLUDED.granted_at,
			    revoked_at = NULL,
			    anonymise_at = NULL,
			    granted_by = EXCLUDED.granted_by,
			    method = EXCLUDED.method
		`, studentID, s.version, now, kind)
		if err != nil {
			return err
		}
		err = insertConsentEvent(ctx, tx, "consent_given", studentID, map[string]string{
			"consent_id":   studentID,
			"kind":         kind,
			"text_version": s.textVersion,
		})
	case "revoke":
		anonymiseAt := now.Add(anonymiseIn)
		tag, execErr := tx.Exec(ctx, `
			UPDATE core.consent
			SET state = 'revoked',
			    revoked_at = $2,
			    anonymise_at = $3
			WHERE student_id = $1
		`, studentID, now, anonymiseAt)
		if execErr != nil {
			return execErr
		}
		if tag.RowsAffected() == 0 {
			return errMissing()
		}
		if err = revokeSessions(ctx, tx, studentID, now); err != nil {
			return err
		}
		tutorID, ownErr := ownerTutor(ctx, tx, studentID)
		if ownErr != nil {
			return ownErr
		}
		if err = insertRevokeNotice(ctx, tx, tutorID, studentID); err != nil {
			return err
		}
		err = insertConsentEvent(ctx, tx, "consent_revoked", studentID, map[string]string{
			"consent_id":   studentID,
			"anonymise_at": anonymiseAt.UTC().Format(time.RFC3339Nano),
		})
	case "cancel_revoke":
		tag, execErr := tx.Exec(ctx, `
			UPDATE core.consent
			SET state = 'active',
			    revoked_at = NULL,
			    anonymise_at = NULL
			WHERE student_id = $1
			  AND state = 'revoked'
			  AND anonymise_at > $2
		`, studentID, now)
		if execErr != nil {
			return execErr
		}
		if tag.RowsAffected() == 0 {
			return errConfirmation()
		}
		err = insertConsentEvent(ctx, tx, "consent_revocation_cancelled", studentID, map[string]string{
			"consent_id": studentID,
		})
	default:
		return errUnavailable()
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// giveLeavesConsent сообщает, что give не должен менять согласие.
// PV-R10 оставляет повторный give только до отзыва. PV-R18 возвращает согласие
// действием cancel_revoke, пока не вышли 7 суток (anonymiseIn, поле anonymise_at).
// Поэтому give не оживляет согласие ни внутри этого срока, ни после него,
// и не оживляет его после обезличивания.
func giveLeavesConsent(ctx context.Context, tx pgx.Tx, studentID string) (bool, error) {
	var state, status *string
	err := tx.QueryRow(ctx, `
		SELECT c.state, u.status
		FROM (SELECT $1::text AS id) AS s
		LEFT JOIN core.consent AS c ON c.student_id = s.id
		LEFT JOIN core.app_user AS u ON u.id = s.id
	`, studentID).Scan(&state, &status)
	if err != nil {
		return false, err
	}
	if status != nil && *status == "anonymised" {
		return true, nil
	}
	return state != nil && *state == "revoked", nil
}

func insertConsentEvent(ctx context.Context, tx pgx.Tx, kind, studentID string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO core.journal_event (kind, payload, student_id, actor_role)
		VALUES ($1, $2::jsonb, $3, 'system')
	`, kind, string(raw), studentID)
	return err
}

func (s *Service) loadPage(ctx context.Context, studentID, kind string) (consentPage, error) {
	page := consentPage{
		TextVersion: s.textVersion,
		Text:        s.text,
		Kind:        kind,
		State:       "none",
	}
	var state string
	var anonymise *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT state, anonymise_at
		FROM core.consent
		WHERE student_id = $1
	`, studentID).Scan(&state, &anonymise)
	if errors.Is(err, pgx.ErrNoRows) {
		return page, nil
	}
	if err != nil {
		return consentPage{}, err
	}
	page.State = apiState(state)
	if anonymise != nil {
		formatted := anonymise.UTC().Format(time.RFC3339Nano)
		page.AnonymiseAt = &formatted
	}
	return page, nil
}

func apiState(state string) string {
	if state == "active" {
		return "given"
	}
	return state
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		writeErr(w, errUnavailable())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeErr(w http.ResponseWriter, api *Error) {
	if api == nil {
		api = errUnavailable()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(api.HTTPStatus)
	_, _ = w.Write(api.Body())
}

func writeServiceErr(w http.ResponseWriter, err error) {
	var api *Error
	if errors.As(err, &api) {
		writeErr(w, api)
		return
	}
	writeErr(w, errUnavailable())
}
