package plan

import "time"

// Config — подключение и часы раскладки. Now не заменяется на time.Now.
type Config struct {
	DSN string
	Now time.Time
}

// Schedule — правило пары (ученик, предмет): дни 1–7, время UTC, длительность.
type Schedule struct {
	Weekdays    []int
	StartTime   string
	DurationMin int
}

// LessonSpec — урок, который Seed кладёт в план.
type LessonSpec struct {
	ID          string
	PlaceNo     int
	Title       string
	Kind        string
	TemplateID  string
	State       string
	Topics      []string
	PlannedWeek string
	StartsAt    string
	Moved       bool
	Added       bool
}

// Fixture — данные одного ученика для Seed. Now совпадает с Config.Now.
type Fixture struct {
	Now             time.Time
	ExamDate        time.Time
	Subject         string
	CatalogVersion  int
	ShiftWeeks      int
	SourceHashes    map[string]string
	Lessons         []LessonSpec
	Schedule        *Schedule
	Prereqs         map[string][]string
	HomeworkPercent *int
	OtherStudent    bool
}

// World — то, что Seed создал и что видят тесты.
type World struct {
	StudentID       string
	OtherStudentID  string
	OtherLessonID   string
	Subject         string
	LessonIDs       []string
	ActorTelegramID string
	ActorRole       string
	CatalogVersion  int
}

// Snapshot — план 0 в порядке мест. Добавленного урока и переноса в нём нет.
type Snapshot struct {
	SourceHashes map[string]string
	LessonIDs    []string
	TemplateIDs  []string
	PlannedWeeks []string
}
