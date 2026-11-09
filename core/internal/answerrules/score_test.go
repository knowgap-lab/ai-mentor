package answerrules

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

// example — одна строка docs/catalog/answer-rules.examples.yaml.
type example struct {
	ID     string            `yaml:"id"`
	Rule   string            `yaml:"rule"`
	Format string            `yaml:"format"`
	Key    any               `yaml:"key"`
	Input  any               `yaml:"input"`
	Traps  map[string]string `yaml:"traps"`
	Expect map[string]any    `yaml:"expect"`
}

// TestScore сверяет Score с каждой строкой таблицы примеров.
// Имя подтеста содержит id и номер правила: ar-08/AR-R7.
func TestScore(t *testing.T) {
	examples := loadExamples(t)
	seen := make(map[string]struct{}, len(examples))
	for _, ex := range examples {
		if ex.ID == "" || ex.Rule == "" {
			t.Fatalf("строка без id или rule: %#v", ex)
		}
		name := ex.ID + "/" + ex.Rule
		if _, ok := seen[name]; ok {
			t.Fatalf("повтор подтеста %s", name)
		}
		seen[name] = struct{}{}
		t.Run(name, func(t *testing.T) {
			key := coerceKey(t, ex.Format, ex.Key)
			raw := coerceRaw(t, ex.Format, ex.Input)
			got := Score(ex.Format, key, raw, ex.Traps)
			again := Score(ex.Format, key, raw, ex.Traps)
			if !reflect.DeepEqual(got, again) {
				t.Fatalf("повторный вызов дал другой результат: %+v и %+v", got, again)
			}
			want := expectResult(t, ex.Expect)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Score() = %+v, таблица %+v", got, want)
			}
		})
	}
}

func loadExamples(t *testing.T) []example {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("не удалось определить путь к тесту")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "catalog", "answer-rules.examples.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение %s: %v", path, err)
	}
	var rows []example
	if err := yaml.Unmarshal(body, &rows); err != nil {
		t.Fatalf("разбор %s: %v", path, err)
	}
	if len(rows) == 0 {
		t.Fatalf("в %s нет строк", path)
	}
	return rows
}

// coerceKey приводит ключ таблицы к типу Score: choice — []int,
// integer — int, digits и match — string.
func coerceKey(t *testing.T, format string, v any) any {
	t.Helper()
	switch format {
	case "ct.choice1", "ct.choiceN":
		return asInts(t, v)
	case "ct.integer":
		return asInt(t, v)
	case "ct.digits", "ct.match":
		return asString(t, v)
	default:
		t.Fatalf("неизвестный format %q", format)
		return nil
	}
}

// coerceRaw приводит ввод ученика к типу Score: choice — []int, иначе string.
func coerceRaw(t *testing.T, format string, v any) any {
	t.Helper()
	switch format {
	case "ct.choice1", "ct.choiceN":
		return asInts(t, v)
	case "ct.integer", "ct.digits", "ct.match":
		return asString(t, v)
	default:
		t.Fatalf("неизвестный format %q", format)
		return nil
	}
}

// expectResult собирает Result из expect.
// Поля, которых в строке нет, остаются нулевыми: пустой TrapID означает,
// что ловушка не сработала и поля в примере нет.
func expectResult(t *testing.T, m map[string]any) Result {
	t.Helper()
	for k := range m {
		switch k {
		case "status", "normalized", "verdict", "points", "trap_id":
		default:
			t.Fatalf("неизвестное поле expect %q", k)
		}
	}
	status, ok := m["status"]
	if !ok {
		t.Fatal("в expect нет status")
	}
	r := Result{Status: asString(t, status)}
	if v, ok := m["normalized"]; ok {
		r.Normalized = asString(t, v)
	}
	if v, ok := m["verdict"]; ok {
		r.Verdict = asString(t, v)
	}
	if v, ok := m["points"]; ok {
		r.Points = asInt(t, v)
	}
	if v, ok := m["trap_id"]; ok {
		r.TrapID = asString(t, v)
	}
	return r
}

func asInt(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	default:
		t.Fatalf("ожидалось целое, получен %T (%#v)", v, v)
		return 0
	}
}

func asInts(t *testing.T, v any) []int {
	t.Helper()
	xs, ok := v.([]any)
	if !ok {
		t.Fatalf("ожидался список целых, получен %T (%#v)", v, v)
	}
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = asInt(t, x)
	}
	return out
}

func asString(t *testing.T, v any) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("ожидалась строка, получен %T (%#v)", v, v)
	}
	return s
}
