package homework

// Config — подключение к базе уроков и черновика.
// Пустой AppOrigin в проверке Origin заменяется на http://localhost.
type Config struct {
	DSN       string
	AppOrigin string
}

// World — выдуманные участники, которых Seed положил в базу.
type World struct {
	StudentID       string
	OtherStudentID  string
	OtherLessonID   string
	Subject         string
	LessonIDs       []string
	ActorTelegramID string
	ActorRole       string
}

// Fixture — вход сборки одного ученика. Варианты и прошлое ДЗ общие для его уроков.
type Fixture struct {
	Subject        string
	CatalogVersion int
	OtherStudent   bool
	Lessons        []Lesson
	Variants       []Variant
	Past           []PastItem
}

// Lesson — урок фикстуры. StartsAt — RFC3339 или пустая строка.
type Lesson struct {
	ID        string
	PlaceNo   int
	State     string
	Cancelled bool
	NoBlocks  bool
	Coverage  string
	StartsAt  string
	Blocks    []string
	Tasks     []Task
	Recipe    []Series
	Mark      *Mark
}

// Task — практическая задача урока.
type Task struct {
	Position    int
	VariantID   string
	TypeID      string
	Subvariant  string
	TheoryBlock string
}

// Series — серия рецепта ДЗ.
type Series struct {
	Source string
	TypeID string
	Count  int
	Topic  string
}

// Mark — две отметки урока. PracticePosition 0 при PracticeSet — отметка практики «нет».
type Mark struct {
	TheorySet        bool
	Theory           string
	PracticeSet      bool
	PracticePosition int
}

// Variant — вариант каталога, доступный сборке.
type Variant struct {
	ID                string
	TypeID            string
	Subvariant        string
	Topic             string
	Difficulty        int
	TheoryBlock       string
	Statement         string
	Minutes           int
	AnswerFormat      string
	CheckpointReserve bool
	Seen              bool
}

// PastItem — задача последнего сданного ДЗ.
type PastItem struct {
	VariantID   string
	Topic       string
	MaxPoints   int
	FirstPoints int
}
