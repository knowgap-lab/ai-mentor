// Пакет conspect отдаёт конспект урока из снимка. PDF не собирается.
package conspect

// Config — подключение. В тестах задан только DSN.
type Config struct {
	DSN string
}

// WidgetSpec — виджет ревизии и его JSON Schema.
type WidgetSpec struct {
	Widget string
	Params map[string]any
	Schema map[string]any
}

// BlockRev — ревизия блока каталога на одной версии.
type BlockRev struct {
	ID       string
	Kind     string
	Title    string
	Markdown string
	Version  int
	Widget   *WidgetSpec
}

// StageSpec — этап снимка: блоки в заданном порядке.
type StageSpec struct {
	ID     string
	Blocks []string
}

// LessonSpec — урок, который Seed кладёт в план.
type LessonSpec struct {
	ID         string
	PlaceNo    int
	Title      string
	State      string
	Stages     []StageSpec
	NoBlocks   bool
	LessonFile string
	Mark       string
	Practice   []string
}

// CarrySpec — перенос блоков на урок.
type CarrySpec struct {
	ToLessonID   string
	FromLessonID string
	Blocks       []string
}

// Fixture — данные одного репетитора и ученика для Seed.
type Fixture struct {
	Subject         string
	CatalogVersion  int
	SnapshotVersion int
	Blocks          []BlockRev
	Lessons         []LessonSpec
	Carry           []CarrySpec
	OtherStudent    bool
	Peer            bool
}

// World — то, что Seed создал и что видят тесты.
type World struct {
	TutorTelegramID   string
	StudentTelegramID string
	ActorRole         string
	OtherLessonID     string
	PeerLessonID      string
}

type storedWidget struct {
	Widget string         `json:"widget"`
	Params map[string]any `json:"params"`
}

type storedRev struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	Body         string         `json:"body"`
	Widget       *storedWidget  `json:"widget,omitempty"`
	WidgetSchema map[string]any `json:"widget_schema,omitempty"`
}

type snapSlot struct {
	VariantID string `json:"variant_id"`
}

type snapStage struct {
	ID       string     `json:"id"`
	Blocks   []string   `json:"blocks"`
	Practice []snapSlot `json:"practice,omitempty"`
}
