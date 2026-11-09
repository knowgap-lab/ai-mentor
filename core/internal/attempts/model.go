package attempts

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Config — параметры сервиса. В тестах задан только DSN.
type Config struct {
	DSN string
}

// World — идентификаторы, которые Seed подготовил для сценария.
type World struct {
	AssignmentID    string
	ItemID          string
	AttemptID       string
	LessonID        string
	StudentID       string
	ActorTelegramID string
	ActorRole       string
}

// ItemResult — итог элемента (AT-R11).
// Баллы первой попытки и итог хранятся раздельно.
type ItemResult struct {
	Verdict            string
	FirstAttemptPoints int
	TotalPoints        int
	EndsAtAttempt      int
}

type queryer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type scored struct {
	no      int
	verdict string
	points  int
}

func pointsFor(verdict string) int {
	switch verdict {
	case "correct":
		return 2
	case "partial":
		return 1
	default:
		return 0
	}
}

func verdictRank(verdict string) int {
	switch verdict {
	case "correct":
		return 3
	case "partial":
		return 2
	case "wrong":
		return 1
	default:
		return 0
	}
}

// summarize применяет AT-R11: элемент кончается на первой попытке,
// чей текущий вердикт correct. Если верных нет, берётся лучший вердикт.
func summarize(attempts []scored) (ItemResult, bool) {
	if len(attempts) == 0 {
		return ItemResult{}, false
	}
	first := attempts[0].points
	for _, attempt := range attempts {
		if attempt.verdict == "correct" {
			return ItemResult{
				Verdict:            "correct",
				FirstAttemptPoints: first,
				TotalPoints:        attempt.points,
				EndsAtAttempt:      attempt.no,
			}, true
		}
	}
	best := attempts[0]
	for _, attempt := range attempts[1:] {
		if attempt.points > best.points || (attempt.points == best.points && verdictRank(attempt.verdict) > verdictRank(best.verdict)) {
			best = attempt
		}
	}
	return ItemResult{
		Verdict:            best.verdict,
		FirstAttemptPoints: first,
		TotalPoints:        best.points,
		EndsAtAttempt:      best.no,
	}, true
}

func sourcePlace(kind string) (string, string) {
	switch kind {
	case "checkpoint":
		return "checkpoint", "lesson"
	case "mock":
		return "mock", "home"
	case "diagnostics":
		return "diagnostics", "home"
	case "rt":
		return "rt", "home"
	default:
		return "homework", "home"
	}
}

func isTestKind(kind string) bool {
	switch kind {
	case "checkpoint", "mock", "diagnostics":
		return true
	default:
		return false
	}
}
