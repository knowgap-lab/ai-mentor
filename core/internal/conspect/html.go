package conspect

import (
	"html"
	"strconv"
	"strings"
)

func conspectHTML(subject, title string, place int, doc conspectOut) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body>")
	b.WriteString("<p>")
	b.WriteString(html.EscapeString(subject))
	b.WriteString("</p><p>")
	b.WriteString(strconv.Itoa(place))
	b.WriteString("</p><h1>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h1>")
	writeBlocks(&b, doc.CarriedOver)
	writeBlocks(&b, doc.Blocks)
	b.WriteString("</body></html>")
	return b.String()
}

func writeBlocks(b *strings.Builder, blocks []blockOut) {
	for _, block := range blocks {
		b.WriteString("<section><h2>")
		b.WriteString(html.EscapeString(block.Title))
		b.WriteString("</h2><p>")
		b.WriteString(html.EscapeString(block.Markdown))
		b.WriteString("</p>")
		if block.Widget != nil {
			b.WriteString("<img alt=\"")
			b.WriteString(html.EscapeString(block.Widget.Widget))
			b.WriteString("\" src=\"data:image/svg+xml,%3Csvg%3E%3C/svg%3E\">")
		}
		b.WriteString("</section>")
	}
}
