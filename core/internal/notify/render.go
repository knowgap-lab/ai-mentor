package notify

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	listLimit = 20
	textLimit = 4096
)

func render(template string, params map[string]any, displayName string) string {
	var b strings.Builder
	rest := template
	for {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			b.WriteString(rest)
			break
		}
		rel := strings.IndexByte(rest[start+1:], '}')
		if rel < 0 {
			b.WriteString(rest)
			break
		}
		end := start + 1 + rel
		b.WriteString(rest[:start])
		key := rest[start+1 : end]
		if value, ok := lookupParam(key, params, displayName); ok {
			b.WriteString(value)
		} else {
			b.WriteString(rest[start : end+1])
		}
		rest = rest[end+1:]
	}
	return limitRunes(b.String(), textLimit)
}

func lookupParam(key string, params map[string]any, displayName string) (string, bool) {
	switch key {
	case "display_name":
		return displayName, true
	case "items":
		return formatItems(params["items"]), true
	default:
		value, ok := params[key]
		if !ok {
			return "", false
		}
		text, ok := value.(string)
		if !ok {
			return "", false
		}
		return text, true
	}
}

func formatItems(value any) string {
	items, ok := value.([]any)
	if !ok {
		text, ok := value.(string)
		if ok {
			return text
		}
		return ""
	}
	shown := len(items)
	extra := 0
	if shown > listLimit {
		extra = shown - listLimit
		shown = listLimit
	}
	lines := make([]string, 0, shown+1)
	for i := 0; i < shown; i++ {
		lines = append(lines, fmt.Sprint(items[i]))
	}
	if extra > 0 {
		lines = append(lines, fmt.Sprintf("и ещё %d", extra))
	}
	return strings.Join(lines, "\n")
}

func limitRunes(text string, max int) string {
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max])
}

func studentIDOf(params map[string]any) string {
	id, _ := params["student_id"].(string)
	return id
}
