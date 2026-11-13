// Пакет compose склеивает вход, приём ответа и журнал попыток
// и добавляет GET /students.
package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "__Host-session"

// Handler склеивает три обработчика и обслуживает GET /students.
// Список читает core.app_user: только ученики, у которых owner_tutor_id
// равен вошедшему репетитору. Объект содержит user_id и role.
func Handler(dsn string, auth, answers, attempts http.Handler) http.Handler {
	pool := openPool(dsn)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/students" {
			listStudents(w, r, pool)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/assignments" {
			listAssignments(w, r, pool)
			return
		}
		path := r.URL.Path
		switch {
		case path == "/me" || strings.HasPrefix(path, "/auth/"):
			auth.ServeHTTP(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/answer"):
			answers.ServeHTTP(w, r)
		default:
			attempts.ServeHTTP(w, r)
		}
	})
}

func openPool(dsn string) *pgxpool.Pool {
	if dsn == "" {
		return nil
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil
	}
	return pool
}

type studentView struct {
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type assignmentSummary struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	State     string `json:"state"`
	Total     int    `json:"total"`
	StudentID string `json:"student_id"`
}

func listStudents(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	tutorID, ok := currentTutor(w, r, pool)
	if !ok {
		return
	}
	rows, err := pool.Query(r.Context(), `
		SELECT id, role
		FROM core.app_user
		WHERE role = 'student' AND owner_tutor_id = $1
		ORDER BY id
	`, tutorID)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	out := make([]studentView, 0)
	for rows.Next() {
		var item studentView
		if err := rows.Scan(&item.UserID, &item.Role); err != nil {
			writeCode(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		// Экран репетитора читает id и display_name. Отдельного имени в сиде нет.
		item.ID = item.UserID
		item.DisplayName = item.UserID
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func listAssignments(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	tutorID, ok := currentTutor(w, r, pool)
	if !ok {
		return
	}
	studentID := r.URL.Query().Get("student_id")
	out := make([]assignmentSummary, 0)
	if studentID == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	rows, err := pool.Query(r.Context(), `
		SELECT a.id, a.kind, a.state, a.student_id,
		       (SELECT count(*)::int FROM core.assignment_item AS i WHERE i.assignment_id = a.id)
		FROM core.assignment AS a
		JOIN core.app_user AS u ON u.id = a.student_id
		WHERE a.student_id = $1
		  AND u.role = 'student'
		  AND u.owner_tutor_id = $2
		ORDER BY a.id
	`, studentID, tutorID)
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item assignmentSummary
		if err := rows.Scan(&item.ID, &item.Kind, &item.State, &item.StudentID, &item.Total); err != nil {
			writeCode(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func currentTutor(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) (string, bool) {
	if pool == nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return "", false
	}
	token, ok := cookieToken(r)
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	hash, ok := hashToken(token)
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	var tutorID, role string
	err := pool.QueryRow(r.Context(), `
		SELECT u.id, u.role
		FROM core.session AS sess
		JOIN core.app_user AS u ON u.id = sess.user_id
		WHERE sess.token_hash = $1
		  AND sess.revoked_at IS NULL
		  AND sess.expires_at > now()
	`, hash).Scan(&tutorID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	if err != nil {
		writeCode(w, http.StatusServiceUnavailable, "unavailable")
		return "", false
	}
	if role != "tutor" {
		writeCode(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	return tutorID, true
}

func cookieToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func hashToken(token string) ([]byte, bool) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
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
