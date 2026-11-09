// Пакет scenario гоняет сценарии given → steps → expect против http.Handler.
package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Bindings — значения, которые загрузчик подставляет в путь и заголовки запроса.
type Bindings struct {
	Path   map[string]string // имя параметра пути → значение
	Header map[string]string // заголовки запроса, например Cookie
}

// Loader заполняет привязки по блоку given одного сценария.
type Loader func(given map[string]any) (Bindings, error)

// Options — вход прогонщика. Журнал nil значит, что снимка нет.
type Options struct {
	Spec      []byte // OpenAPI YAML
	Scenarios []byte // список сценариев, как в *.examples.yaml
	Handler   http.Handler
	Load      Loader
	Journal   func() map[string]any // снимок журнала после шага; nil значит журнала нет
}

type scenarioDoc struct {
	ID    string         `yaml:"id"`
	Given map[string]any `yaml:"given"`
	Steps []stepDoc      `yaml:"steps"`
}

type stepDoc struct {
	Op       string         `yaml:"op"`
	Request  any            `yaml:"request"`
	Expect   map[string]any `yaml:"expect"`
	Parallel []stepDoc      `yaml:"parallel"`
}

type operation struct {
	method string
	path   string
}

var pathParamRE = regexp.MustCompile(`\{([^{}]+)\}`)

// Run выполняет каждый сценарий: Load, шаги, сравнение ответа.
func Run(t *testing.T, opts Options) {
	t.Helper()
	idx := parseSpec(t, opts.Spec)
	scenarios := parseScenarios(t, opts.Scenarios)
	srv := httptest.NewServer(opts.Handler)
	t.Cleanup(srv.Close)

	for _, sc := range scenarios {
		if opts.Load == nil {
			t.Fatalf("сценарий %s: Load не задан", sc.ID)
		}
		bindings, err := opts.Load(sc.Given)
		if err != nil {
			t.Fatalf("сценарий %s: Load: %v", sc.ID, err)
		}
		for i, st := range sc.Steps {
			where := fmt.Sprintf("сценарий %s шаг %d", sc.ID, i+1)
			runStep(t, where, srv, idx, bindings, st, opts.Journal)
		}
	}
}

func parseScenarios(t *testing.T, raw []byte) []scenarioDoc {
	t.Helper()
	var scenarios []scenarioDoc
	if err := yaml.Unmarshal(raw, &scenarios); err != nil {
		t.Fatalf("сценарии: %v", err)
	}
	return scenarios
}

func parseSpec(t *testing.T, raw []byte) map[string][]operation {
	t.Helper()
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("openapi: %v", err)
	}
	idx := make(map[string][]operation)
	for path, item := range doc.Paths {
		for key, raw := range item {
			fields, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id, _ := fields["operationId"].(string)
			if id == "" {
				continue
			}
			idx[id] = append(idx[id], operation{
				method: strings.ToUpper(key),
				path:   path,
			})
		}
	}
	return idx
}

func runStep(t *testing.T, where string, srv *httptest.Server, idx map[string][]operation, b Bindings, st stepDoc, journal func() map[string]any) {
	t.Helper()
	if len(st.Parallel) > 0 {
		for i, child := range st.Parallel {
			runStep(t, fmt.Sprintf("%s parallel[%d]", where, i), srv, idx, b, child, journal)
		}
		checkJournal(t, where, st.Expect, journal)
		return
	}
	if st.Op == "" {
		t.Fatalf("%s: нет op", where)
	}

	expanded, err := expand(st.Request)
	if err != nil {
		t.Fatalf("%s: %v", where, err)
	}
	op, err := lookup(idx, st.Op)
	if err != nil {
		t.Fatalf("%s: %v", where, err)
	}
	path, err := substPath(op.path, b.Path)
	if err != nil {
		t.Fatalf("%s: %v", where, err)
	}

	var body io.Reader
	if st.Request != nil {
		raw, err := json.Marshal(expanded)
		if err != nil {
			t.Fatalf("%s: json запроса: %v", where, err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(op.method, srv.URL+path, body)
	if err != nil {
		t.Fatalf("%s: запрос: %v", where, err)
	}
	for k, v := range b.Header {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s: %v", where, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s: чтение ответа: %v", where, err)
	}
	checkResponse(t, where, st.Expect, resp, raw)
	checkJournal(t, where, st.Expect, journal)
}

func lookup(idx map[string][]operation, id string) (operation, error) {
	ops := idx[id]
	switch len(ops) {
	case 0:
		return operation{}, fmt.Errorf("операция %s не найдена", id)
	case 1:
		return ops[0], nil
	default:
		return operation{}, fmt.Errorf("операция %s не единственная (%d)", id, len(ops))
	}
}

func substPath(tmpl string, params map[string]string) (string, error) {
	var missing []string
	out := pathParamRE.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := match[1 : len(match)-1]
		v, ok := params[name]
		if !ok {
			missing = append(missing, name)
			return match
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("нет параметров пути: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// expand повторяет строку, только если значение — объект из одного ключа $repeat.
func expand(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 1 {
			if rep, ok := x["$repeat"]; ok {
				return expandRepeat(rep)
			}
		}
		out := make(map[string]any, len(x))
		for k, child := range x {
			ev, err := expand(child)
			if err != nil {
				return nil, err
			}
			out[k] = ev
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, child := range x {
			ev, err := expand(child)
			if err != nil {
				return nil, err
			}
			out[i] = ev
		}
		return out, nil
	default:
		return v, nil
	}
}

func expandRepeat(v any) (any, error) {
	list, ok := v.([]any)
	if !ok || len(list) != 2 {
		return nil, fmt.Errorf("$repeat: нужна пара [строка, число]")
	}
	s, ok := list[0].(string)
	if !ok {
		return nil, fmt.Errorf("$repeat: первый элемент не строка")
	}
	n, ok := asInt(list[1])
	if !ok || n < 0 {
		return nil, fmt.Errorf("$repeat: число повторов %v", list[1])
	}
	return strings.Repeat(s, n), nil
}

func checkResponse(t *testing.T, where string, exp map[string]any, resp *http.Response, raw []byte) {
	t.Helper()
	if exp == nil {
		return
	}
	if want, ok := exp["http"]; ok {
		code, ok := asInt(want)
		if !ok {
			t.Fatalf("%s: http не число: %#v", where, want)
		}
		if resp.StatusCode != code {
			t.Fatalf("%s: http %d, ожидали %d", where, resp.StatusCode, code)
		}
	}

	var obj map[string]any
	if _, body := exp["body"]; body || exp["no_fields"] != nil || exp["error"] != nil {
		parsed, err := decodeObject(raw)
		if err != nil {
			t.Fatalf("%s: тело ответа: %v", where, err)
		}
		obj = parsed
	}
	if body, ok := exp["body"].(map[string]any); ok {
		if err := matchMap(where+".body", body, obj); err != nil {
			t.Fatalf("%s: %v", where, err)
		}
	} else if _, ok := exp["body"]; ok {
		t.Fatalf("%s: expect.body не объект", where)
	}
	if nf, ok := exp["no_fields"]; ok {
		fields, ok := nf.([]any)
		if !ok {
			t.Fatalf("%s: no_fields не список", where)
		}
		for _, f := range fields {
			name, ok := f.(string)
			if !ok {
				t.Fatalf("%s: no_fields: %#v не строка", where, f)
			}
			if _, exists := obj[name]; exists {
				t.Fatalf("%s: поле %s должно отсутствовать", where, name)
			}
		}
	}
	if errExp, ok := exp["error"]; ok {
		em, ok := errExp.(map[string]any)
		if !ok {
			t.Fatalf("%s: expect.error не объект", where)
		}
		wantCode, ok := em["code"]
		if !ok {
			t.Fatalf("%s: expect.error.code отсутствует", where)
		}
		errObj, ok := obj["error"].(map[string]any)
		if !ok {
			t.Fatalf("%s: в теле нет error", where)
		}
		got, exists := errObj["code"]
		if !exists {
			t.Fatalf("%s: в теле нет error.code", where)
		}
		if err := matchValue(where+".error.code", wantCode, got); err != nil {
			t.Fatalf("%s: %v", where, err)
		}
	}
	if hs, ok := exp["headers"]; ok {
		hm, ok := hs.(map[string]any)
		if !ok {
			t.Fatalf("%s: expect.headers не объект", where)
		}
		for name, want := range hm {
			wantStr, ok := want.(string)
			if !ok {
				t.Fatalf("%s: заголовок %s: %#v не строка", where, name, want)
			}
			vals := resp.Header.Values(name)
			if len(vals) == 0 {
				t.Fatalf("%s: нет заголовка %s", where, name)
			}
			if vals[0] != wantStr {
				t.Fatalf("%s: заголовок %s=%q, ожидали %q", where, name, vals[0], wantStr)
			}
		}
	}
}

func checkJournal(t *testing.T, where string, exp map[string]any, journal func() map[string]any) {
	t.Helper()
	if exp == nil {
		return
	}
	raw, ok := exp["journal"]
	if !ok {
		return
	}
	if journal == nil {
		t.Fatalf("%s: журнал ожидается, Journal равен nil", where)
	}
	want, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s: expect.journal не объект", where)
	}
	snap := journal()
	if snap == nil {
		snap = map[string]any{}
	}
	if err := matchMap(where+".journal", want, snap); err != nil {
		t.Fatalf("%s: %v", where, err)
	}
}

func decodeObject(raw []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("пустое тело")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("тело не объект")
	}
	return obj, nil
}

func matchMap(path string, expect, actual map[string]any) error {
	for k, ev := range expect {
		av, ok := actual[k]
		if !ok {
			return fmt.Errorf("%s.%s: нет ключа", path, k)
		}
		if err := matchValue(path+"."+k, ev, av); err != nil {
			return err
		}
	}
	return nil
}

func matchValue(path string, expect, actual any) error {
	if ef, ok := asFloat(expect); ok {
		af, ok := asFloat(actual)
		if !ok || af != ef {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return nil
	}
	switch ev := expect.(type) {
	case map[string]any:
		av, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return matchMap(path, ev, av)
	case []any:
		av, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		if len(ev) != len(av) {
			return fmt.Errorf("%s: длина %d != %d", path, len(av), len(ev))
		}
		for i := range ev {
			if err := matchValue(fmt.Sprintf("%s[%d]", path, i), ev[i], av[i]); err != nil {
				return err
			}
		}
		return nil
	default:
		if !reflect.DeepEqual(expect, actual) {
			return fmt.Errorf("%s: %#v != %#v", path, actual, expect)
		}
		return nil
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func asInt(v any) (int, bool) {
	f, ok := asFloat(v)
	if !ok || f != float64(int(f)) {
		return 0, false
	}
	return int(f), true
}
