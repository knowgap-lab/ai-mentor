package events_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/knowgap-lab/ai-mentor/core/internal/dbtest"
	"github.com/knowgap-lab/ai-mentor/core/internal/events"
	"github.com/pressly/goose/v3"
)

// Документы собраны только из полей events.schema.json. Идентификаторы выдуманы.

func TestS7_ValidateAcceptsEachType(t *testing.T) {
	for eventType, doc := range s7ValidDocs(t) {
		t.Run(eventType, func(t *testing.T) {
			if err := events.Validate(doc); err != nil {
				t.Fatalf("Validate: %v\n%s", err, doc)
			}
		})
	}
}

func TestS7_ValidateRejectsMissingSchemaVersion(t *testing.T) {
	doc := s7WithoutKey(t, s7ValidDocs(t)["assignment_issued"], "schema_version")
	if err := events.Validate(doc); err == nil {
		t.Fatal("документ без schema_version принят")
	}
}

func TestS7_ValidateRejectsUnknownEventType(t *testing.T) {
	doc := s7Replace(t, s7ValidDocs(t)["assignment_issued"], "event_type", "s7_unknown")
	if err := events.Validate(doc); err == nil {
		t.Fatal("неизвестный event_type принят")
	}
}

func TestS7_ValidateRejectsForeignPayload(t *testing.T) {
	doc := s7Replace(t, s7ValidDocs(t)["assignment_issued"], "payload", map[string]any{
		"assignment_id": "s7-asg",
		"answered":      1,
		"skipped":       0,
	})
	if err := events.Validate(doc); err == nil {
		t.Fatal("assignment_issued с нагрузкой assignment_submitted принят")
	}
}

func TestS7_ValidateRejectsExtraField(t *testing.T) {
	t.Run("envelope", func(t *testing.T) {
		doc := s7Replace(t, s7ValidDocs(t)["assignment_withdrawn"], "s7_extra", "no")
		if err := events.Validate(doc); err == nil {
			t.Fatal("лишнее поле конверта принято")
		}
	})
	t.Run("payload", func(t *testing.T) {
		payload := map[string]any{
			"assignment_id": "s7-asg",
			"reason":        "ошибка в условии",
			"s7_extra":      "no",
		}
		doc := s7Replace(t, s7ValidDocs(t)["assignment_withdrawn"], "payload", payload)
		if err := events.Validate(doc); err == nil {
			t.Fatal("лишнее поле нагрузки принято")
		}
	})
}

func TestAT_R1_S7_NoUpdateDelete(t *testing.T) {
	dsn := dbtest.Start(t)
	s7Migrate(t, dsn)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	for _, priv := range []string{"SELECT", "INSERT"} {
		if !s7Privilege(t, conn, priv) {
			t.Fatalf("у core_app нет %s на core.journal_event", priv)
		}
	}
	for _, priv := range []string{"UPDATE", "DELETE"} {
		if s7Privilege(t, conn, priv) {
			t.Fatalf("у core_app есть %s на core.journal_event", priv)
		}
	}
	s7ExpectDenied(t, conn, `UPDATE journal_event SET kind = 'x'`)
	s7ExpectDenied(t, conn, `DELETE FROM journal_event`)
}

func s7ValidDocs(t *testing.T) map[string][]byte {
	t.Helper()
	item := map[string]any{
		"item_id":       "s7-item",
		"variant_id":    "s7-variant",
		"answer_format": "ct.integer",
		"max_points":    2,
		"source":        "new_topic",
	}
	payloads := map[string]any{
		"assignment_issued": map[string]any{
			"assignment_id":   "s7-asg",
			"kind":            "homework",
			"subject":         "math",
			"catalog_version": 1,
			"rules_version":   "s7-rules",
			"deadline":        nil,
			"place":           "home",
			"items":           []any{item},
		},
		"assignment_withdrawn": map[string]any{
			"assignment_id": "s7-asg",
			"reason":        "выдумано",
		},
		"assignment_submitted": map[string]any{
			"assignment_id": "s7-asg",
			"answered":      1,
			"skipped":       0,
		},
		"deadline_changed": map[string]any{
			"assignment_id": "s7-asg",
			"deadline":      "2026-11-20T00:00:00Z",
		},
		"attempt_recorded": map[string]any{
			"attempt_id":       "s7-attempt",
			"assignment_id":    "s7-asg",
			"item_id":          "s7-item",
			"variant_id":       "s7-variant",
			"catalog_version":  1,
			"raw_input":        "14",
			"normalized_input": "14",
			"attempt_no":       1,
			"verdict":          "wrong",
			"points":           0,
			"source":           "homework",
			"place":            "home",
			"rules_version":    "s7-rules",
			"idempotency_key":  "s7-key",
		},
		"attempt_verdict": map[string]any{
			"attempt_id": "s7-attempt",
			"origin":     "scoring",
			"verdict":    "wrong",
			"points":     0,
			"max_points": 2,
		},
		"manual_verdict": map[string]any{
			"attempt_id": "s7-attempt",
			"verdict":    "partial",
			"points":     1,
			"reason":     "зачтено",
		},
		"did_not_understand": map[string]any{
			"assignment_id": "s7-asg",
			"item_id":       "s7-item",
		},
		"dispute_opened": map[string]any{
			"assignment_id": "s7-asg",
			"item_id":       "s7-item",
			"reason":        "key_wrong",
			"dispute_id":    "s7-dispute",
		},
		"observation": map[string]any{
			"observation_id":       "s7-obs",
			"item_id":              "s7-item",
			"revision":             1,
			"type_id":              "s7-type",
			"subvariant_id":        "s7-sub",
			"variant_id":           "s7-variant",
			"source":               "homework",
			"place":                "home",
			"first_attempt_points": 0,
			"total_points":         0,
			"max_points":           2,
			"trap_ids":             []any{},
			"skipped":              false,
			"late":                 false,
			"catalog_version":      1,
			"rules_version":        "s7-rules",
		},
		"recheck_completed": map[string]any{
			"correction_id": 8,
			"changed_items": []any{"s7-item"},
		},
		"catalog_version_loaded": map[string]any{
			"catalog_version": 1,
		},
		"lesson_frozen": map[string]any{
			"lesson_id":       "s7-lesson",
			"template_id":     "s7-template",
			"catalog_version": 1,
			"blocks":          []any{},
			"stages":          []any{},
			"practice":        []any{},
		},
		"lesson_marked": map[string]any{
			"lesson_id": "s7-lesson",
			"coverage":  "all",
		},
		"lesson_moved": map[string]any{
			"lesson_id": "s7-lesson",
			"to":        "2026-11-17T18:00:00Z",
		},
		"lesson_cancelled": map[string]any{
			"lesson_id": "s7-lesson",
			"date":      "2026-11-14T18:00:00Z",
		},
		"lesson_skipped": map[string]any{
			"lesson_id": "s7-lesson",
		},
		"lesson_carried_over": map[string]any{
			"from_lesson_id": "s7-lesson",
			"to_lesson_id":   "s7-lesson-2",
			"blocks":         []any{},
			"practice":       []any{},
		},
		"consent_given": map[string]any{
			"consent_id":   "s7-consent",
			"kind":         "parent",
			"text_version": "s7-text",
		},
		"consent_revoked": map[string]any{
			"consent_id":   "s7-consent",
			"anonymise_at": "2026-11-21T12:00:00Z",
		},
		"student_archived":   map[string]any{},
		"student_unarchived": map[string]any{},
		"student_anonymised": map[string]any{
			"source": "schedule",
		},
		"account_bound": map[string]any{
			"user_id": "s7-student",
			"role":    "student",
		},
		"session_revoked": map[string]any{
			"user_id": "s7-student",
			"reason":  "logout",
		},
		"device_confirmed": map[string]any{
			"tutor_id": "s7-tutor",
			"method":   "first_login",
		},
		"dispute_resolved": map[string]any{
			"dispute_id": "s7-dispute",
			"resolution": "rejected",
		},
		"lesson_added": map[string]any{
			"lesson_id": "s7-lesson",
			"subject":   "math",
			"starts_at": "2026-11-16T18:00:00Z",
		},
		"plan_reordered": map[string]any{
			"subject":    "math",
			"lesson_ids": []any{"s7-lesson", "s7-lesson-2"},
		},
		"schedule_rule_set": map[string]any{
			"subject":      "math",
			"weekdays":     []any{1, 4},
			"start_time":   "18:00",
			"duration_min": 60,
		},
		"tutor_rebound": map[string]any{
			"tutor_id": "s7-tutor",
		},
		"consent_revocation_cancelled": map[string]any{
			"consent_id": "s7-consent",
		},
	}
	noStudent := map[string]bool{
		"catalog_version_loaded": true,
		"device_confirmed":       true,
		"tutor_rebound":          true,
	}
	out := make(map[string][]byte, len(payloads))
	for eventType, payload := range payloads {
		out[eventType] = s7Envelope(t, eventType, payload, !noStudent[eventType])
	}
	if len(out) != 32 {
		t.Fatalf("типов %d, в oneOf их 32", len(out))
	}
	return out
}

func s7Envelope(t *testing.T, eventType string, payload any, withStudent bool) []byte {
	t.Helper()
	doc := map[string]any{
		"schema_version": 1,
		"journal_seq":    1,
		"occurred_at":    "2026-11-14T12:00:00Z",
		"actor":          map[string]any{"role": "tutor", "id": "s7-tutor"},
		"event_type":     eventType,
		"payload":        payload,
	}
	if withStudent {
		doc["student_id"] = "s7-student"
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func s7WithoutKey(t *testing.T, doc []byte, key string) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(doc, &value); err != nil {
		t.Fatal(err)
	}
	delete(value, key)
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func s7Replace(t *testing.T, doc []byte, key string, value any) []byte {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(doc, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded[key] = value
	raw, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func s7Privilege(t *testing.T, conn *pgx.Conn, priv string) bool {
	t.Helper()
	var ok bool
	err := conn.QueryRow(context.Background(),
		`SELECT has_table_privilege('core_app', 'core.journal_event', $1)`, priv).Scan(&ok)
	if err != nil {
		t.Fatalf("права %s: %v", priv, err)
	}
	return ok
}

func s7ExpectDenied(t *testing.T, conn *pgx.Conn, statement string) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO core`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE core_app`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, statement)
	if err == nil {
		t.Fatalf("%s выполнился", statement)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("%s: %v", statement, err)
	}
}

func s7Migrate(t *testing.T, dsn string) {
	t.Helper()
	dir, err := s7MigrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("db close: %v", closeErr)
		}
	}()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			t.Errorf("goose close: %v", closeErr)
		}
	}()
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("миграции: %v", err)
	}
}

func s7MigrationsDir() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		dir := start
		for {
			candidate := filepath.Join(dir, "migrations", "00001_schemas.sql")
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return filepath.Join(dir, "migrations"), nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("каталог migrations не найден")
}
