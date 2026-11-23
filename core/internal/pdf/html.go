package pdf

import (
	"html"
	"strings"
)

func sheetHTML(statements []string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body>")
	for _, text := range statements {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(text))
		b.WriteString("</p>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func formHTML(title string) string {
	return "<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body><h1>" +
		html.EscapeString(title) + "</h1></body></html>"
}
