package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Authorize пускает репетитора к ученику с owner_tutor_id = его id,
// а ученика — только к себе. Чужой и несуществующий ученик дают один и тот же отказ.
// Имя action на решение не влияет: чтение и запись совпадают.
func (s *Service) Authorize(actor Actor, action string, object Object) error {
	switch actor.Role {
	case "student":
		if actor.UserID != "" && actor.UserID == object.UserID {
			return nil
		}
		return errForbidden()
	case "tutor":
		owns, err := s.tutorOwns(actor.UserID, object.UserID)
		if err != nil {
			return err
		}
		if !owns {
			return errForbidden()
		}
		return nil
	default:
		return errForbidden()
	}
}

func (s *Service) tutorOwns(tutorID, studentID string) (bool, error) {
	if tutorID == "" || studentID == "" {
		return false, nil
	}
	var owner *string
	err := s.pool.QueryRow(context.Background(), `
		SELECT owner_tutor_id
		FROM core.app_user
		WHERE id = $1 AND role = 'student'
	`, studentID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return owner != nil && *owner == tutorID, nil
}

// Remember сохраняет первый ответ пользователя на ключ и возвращает его.
// Повтор того же пользователя получает сохранённое тело. Другой пользователь — нет.
func (s *Service) Remember(userID, key string, response []byte) ([]byte, error) {
	if response == nil {
		response = []byte{}
	}
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO core.idempotency (user_id, idem_key, response)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, idem_key) DO NOTHING
	`, userID, key, response)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 1 {
		out := make([]byte, len(response))
		copy(out, response)
		return out, nil
	}
	var stored []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT response
		FROM core.idempotency
		WHERE user_id = $1 AND idem_key = $2
	`, userID, key).Scan(&stored); err != nil {
		return nil, err
	}
	return stored, nil
}
