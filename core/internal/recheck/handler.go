package recheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// Handler отдаёт отчёт перепроверки репетитору (getRecheckReport, RC-R20).
// Пакет без попыток его учеников для репетитора неотличим от отсутствующего.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rechecks/{correction}", report)
	return mux
}

func report(w http.ResponseWriter, r *http.Request) {
	correctionID, err := strconv.Atoi(r.PathValue("correction"))
	if err != nil || correctionID < 1 {
		writeForbidden(w)
		return
	}
	conn, err := pgx.Connect(r.Context(), os.Getenv("DATABASE_URL"))
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	userID, role, ok, err := sessionUser(r.Context(), conn, r)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if role != "tutor" {
		writeForbidden(w)
		return
	}
	state, found, err := loadReport(r.Context(), conn, correctionID)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if !found {
		writeForbidden(w)
		return
	}
	owns, err := tutorOwnsBatch(r.Context(), conn, userID, correctionID)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if !owns {
		writeForbidden(w)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		CorrectionID int    `json:"correction_id"`
		State        string `json:"state"`
	}{
		CorrectionID: state.CorrectionID,
		State:        state.State,
	})
}

func sessionUser(ctx context.Context, conn *pgx.Conn, r *http.Request) (string, string, bool, error) {
	cookie, err := r.Cookie("__Host-session")
	if err != nil || cookie.Value == "" {
		return "", "", false, nil
	}
	hash, ok := hashSession(cookie.Value)
	if !ok {
		return "", "", false, nil
	}
	var userID, role string
	err = conn.QueryRow(ctx, `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
	`, hash).Scan(&userID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return userID, role, true, nil
}

func hashSession(token string) ([]byte, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}

func loadReport(ctx context.Context, conn *pgx.Conn, correctionID int) (State, bool, error) {
	var state State
	err := conn.QueryRow(ctx, `
		SELECT correction_id, state, total, processed, changed, skipped_manual,
		       COALESCE(cursor, '')
		FROM core.recheck_batch
		WHERE correction_id = $1
	`, correctionID).Scan(
		&state.CorrectionID, &state.State, &state.Total, &state.Processed,
		&state.Changed, &state.SkippedManual, &state.Cursor,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func tutorOwnsBatch(ctx context.Context, conn *pgx.Conn, tutorID string, correctionID int) (bool, error) {
	var owns bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM core.recheck_batch AS b
			JOIN core.attempts AS a ON a.id IN (
				SELECT e.payload->>'attempt_id'
				FROM core.journal_event AS e
				WHERE e.kind = 'attempt_recorded'
				  AND e.payload->>'variant_id' = ANY (b.variants)
			)
			JOIN core.assignment AS asg ON asg.id = a.assignment_id
			JOIN core.app_user AS u ON u.id = asg.student_id
			WHERE b.correction_id = $1
			  AND u.owner_tutor_id = $2
		)
	`, correctionID, tutorID).Scan(&owns)
	return owns, err
}

func writeForbidden(w http.ResponseWriter) {
	writeCode(w, http.StatusForbidden, "forbidden")
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
