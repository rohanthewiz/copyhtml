package main

import (
	"html"
	"regexp"
	"strings"
)

// Styling is entirely inline. Teams, Outlook, and most chat editors drop
// <style> blocks and class attributes on paste but keep style="" on
// elements. Background colors are deliberately avoided: in Teams' dark theme
// a light header fill is kept while the text turns white, which makes the
// header unreadable. A mid-gray border reads well in both themes.
const (
	tableStyle = "border-collapse:collapse;"
	cellStyle  = "border:1px solid #999;padding:4px 8px;"
)

// RenderHTML produces a self-contained <table> fragment. When hasHeader is
// true, row 0 goes in <thead> as <th> cells, so it renders bold in every
// client.
func RenderHTML(t *Table, hasHeader bool) string {
	align := columnAlign(t, hasHeader)
	var b strings.Builder
	b.WriteString(`<table style="` + tableStyle + `">`)
	body := t.Rows
	if hasHeader && len(body) > 0 {
		b.WriteString("<thead>")
		writeRow(&b, "th", body[0], align, t.Markdown)
		b.WriteString("</thead>")
		body = body[1:]
	}
	b.WriteString("<tbody>")
	for _, row := range body {
		writeRow(&b, "td", row, align, t.Markdown)
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

// writeRow emits one row. Header cells take the column's alignment too, so
// a right-aligned numeric column has a right-aligned title over it.
func writeRow(b *strings.Builder, tag string, row, align []string, md bool) {
	b.WriteString("<tr>")
	for j, cell := range row {
		b.WriteString("<" + tag + ` style="` + cellStyle + "text-align:" + align[j] + `;">`)
		b.WriteString(cellHTML(cell, md))
		b.WriteString("</" + tag + ">")
	}
	b.WriteString("</tr>")
}

// cellHTML escapes the cell, converts embedded newlines (multi-line CSV
// cells) into <br>, and applies inline Markdown for Markdown sources only.
// Escaping happens first so cell text can never inject markup. The Markdown
// pass then only adds tags around already-escaped text.
func cellHTML(cell string, md bool) string {
	s := html.EscapeString(cell)
	if md {
		s = inlineMarkdown(s)
	}
	return strings.ReplaceAll(s, "\n", "<br>")
}

var (
	codeSpan = regexp.MustCompile("`([^`]+)`")
	boldSpan = regexp.MustCompile(`\*\*(.+?)\*\*`)
)

// inlineMarkdown converts `code` and **bold** in an already-escaped string.
// Code spans are cut out first so "**" inside backticks stays literal, as
// in CommonMark. Italics (*x* / _x_) are intentionally not supported:
// underscores are everywhere in identifiers like job_name, and converting
// them would mangle exactly the data people paste most.
func inlineMarkdown(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range codeSpan.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(boldSpan.ReplaceAllString(s[last:m[0]], "<b>$1</b>"))
		b.WriteString("<code>" + s[m[2]:m[3]] + "</code>")
		last = m[1]
	}
	b.WriteString(boldSpan.ReplaceAllString(s[last:], "<b>$1</b>"))
	return b.String()
}

// numeric matches typical numeric cell values: integers, decimals,
// thousands separators, an optional sign, currency prefix, or percent
// suffix. Examples: 125, -3.5, 1,234, $1,000.00, 12%, .5
var numeric = regexp.MustCompile(`^[-+]?[$€£¥]?(\d[\d,]*(\.\d+)?|\.\d+)%?$`)

// columnAlign resolves each column's text-align. An explicit Markdown
// alignment wins. Otherwise a column whose non-empty body cells are all
// numeric is right-aligned, so digits line up, and anything else is left
// aligned. The header row is excluded from the test, since titles are
// never numeric.
func columnAlign(t *Table, hasHeader bool) []string {
	cols := t.Cols()
	align := make([]string, cols)
	body := t.Rows
	if hasHeader && len(body) > 0 {
		body = body[1:]
	}
	for j := 0; j < cols; j++ {
		if j < len(t.Align) && t.Align[j] != "" {
			align[j] = t.Align[j]
			continue
		}
		align[j] = "left"
		seen := false
		allNum := true
		for _, r := range body {
			v := strings.TrimSpace(strings.ReplaceAll(r[j], "**", "")) // **125** in Markdown totals
			if v == "" {
				continue
			}
			seen = true
			if !numeric.MatchString(v) {
				allNum = false
				break
			}
		}
		if seen && allNum {
			align[j] = "right"
		}
	}
	return align
}

// RenderTSV is the plain-text flavor placed on the clipboard alongside the
// HTML. Targets that refuse HTML (terminals, plain editors) get clean TSV,
// which spreadsheets also paste into separate cells. Tabs and newlines
// inside cells are flattened to spaces so they cannot break the grid.
func RenderTSV(t *Table) string {
	flatten := strings.NewReplacer("\t", " ", "\n", " ")
	var b strings.Builder
	for _, row := range t.Rows {
		for j, cell := range row {
			if j > 0 {
				b.WriteByte('\t')
			}
			b.WriteString(flatten.Replace(cell))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
