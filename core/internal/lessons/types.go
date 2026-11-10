package lessons

import "time"

// Config — подключение и часы раскладки. Now не заменяется на time.Now.
type Config struct {
	DSN       string
	Now       time.Time
	AppOrigin string
}

// Schedule — правило пары (ученик, предмет): дни 1–7, время UTC, длительность.
type Schedule struct {
	Weekdays    []int  `json:"weekdays"`
	StartTime   string `json:"start_time"`
	DurationMin int    `json:"duration_min"`
}

// PracticeSpec — слот практики шаблона урока.
type PracticeSpec struct {
	VariantID   string
	TypeID      string
	Role        string
	Position    int
	TheoryBlock string
}

// StageSpec — этап шаблона. Minutes — границы [начало, конец] в минутах урока.
type StageSpec struct {
	ID       string
	Title    string
	Kind     string
	Minutes  [2]int
	Blocks   []string
	Practice []PracticeSpec
}

// PracticeRef — отметка последней решённой задачи.
type PracticeRef struct {
	StageID  string
	Position int
}

// MarkSpec — уже сохранённые отметки урока на момент Seed.
type MarkSpec struct {
	Theory   *string
	Practice *PracticeRef
}

// LessonSpec — урок, который Seed кладёт в план.
type LessonSpec struct {
	ID          string
	PlaceNo     int
	Title       string
	Kind        string
	TemplateID  string
	State       string
	Stages      []StageSpec
	PlannedWeek string
	NoBlocks    bool
	Issued      bool
	Mark        *MarkSpec
}

// VariantSpec — кандидат практики. Seen и Reserve в слот не берутся.
type VariantSpec struct {
	ID      string
	TypeID  string
	Seen    bool
	Reserve bool
}

// Fixture — данные одного ученика для Seed. Now совпадает с Config.Now.
type Fixture struct {
	Now            time.Time
	ExamDate       time.Time
	Subject        string
	CatalogVersion int
	Lessons        []LessonSpec
	Schedule       *Schedule
	Pool           []VariantSpec
	ReviseBlocks   []string
	OtherStudent   bool
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
