package pdf

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SnapshotKey — урок, версия каталога собранного конспекта и роль зрителя.
type SnapshotKey struct {
	LessonID       string
	CatalogVersion int
	Audience       string
}

type storeDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Cached возвращает уже сохранённый PDF или печатает и сохраняет первый.
func Cached(ctx context.Context, db storeDB, key SnapshotKey, html string) ([]byte, error) {
	stored, ok, err := loadPDF(ctx, db, key)
	if err != nil {
		return nil, err
	}
	if ok {
		return stored, nil
	}
	body, err := Render(ctx, html)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO core.conspect_pdf (lesson_id, catalog_version, audience, pdf)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
	`, key.LessonID, key.CatalogVersion, key.Audience, body)
	if err != nil {
		return nil, err
	}
	stored, ok, err = loadPDF(ctx, db, key)
	if err != nil || !ok {
		return body, err
	}
	return stored, nil
}

func loadPDF(ctx context.Context, db storeDB, key SnapshotKey) ([]byte, bool, error) {
	var stored []byte
	err := db.QueryRow(ctx, `
		SELECT pdf
		FROM core.conspect_pdf
		WHERE lesson_id = $1 AND catalog_version = $2 AND audience = $3
	`, key.LessonID, key.CatalogVersion, key.Audience).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return stored, true, nil
}
