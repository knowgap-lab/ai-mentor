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
	if err := s.apply(r.Context(), row.studentID, action, now); err != nil {
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

func (s *Service) apply(ctx context.Context, studentID, action string, now time.Time) error {
	switch action {
	case "give":
		_, err := s.pool.Exec(ctx, `
			INSERT INTO core.consent (student_id, version, state, granted_at, revoked_at, anonymise_at)
			VALUES ($1, $2, 'active', $3, NULL, NULL)
			ON CONFLICT (student_id) DO UPDATE
			SET version = EXCLUDED.version,
			    state = 'active',
			    granted_at = EXCLUDED.granted_at,
			    revoked_at = NULL,
			    anonymise_at = NULL
		`, studentID, s.version, now)
		return err
	case "revoke":
		tag, err := s.pool.Exec(ctx, `
			UPDATE core.consent
			SET state = 'revoked',
			    revoked_at = $2,
			    anonymise_at = $3
			WHERE student_id = $1
		`, studentID, now, now.Add(anonymiseIn))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errMissing()
		}
		return nil
	case "cancel_revoke":
		tag, err := s.pool.Exec(ctx, `
			UPDATE core.consent
			SET state = 'active',
			    revoked_at = NULL,
			    anonymise_at = NULL
			WHERE student_id = $1
			  AND state = 'revoked'
			  AND anonymise_at > $2
		`, studentID, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errMissing()
		}
		return nil
	default:
		return errUnavailable()
	}
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
