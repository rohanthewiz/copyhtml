package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Table is the format-independent result of parsing. Rows holds every row,
// including the header row when there is one. Whether row 0 is a header is
// decided at render time, so the parsers never need to know about -no-header.
type Table struct {
	Rows [][]string
	// Align holds explicit per-column alignment ("left", "right", "center")
	// taken from a Markdown separator row such as |:---|---:|. An empty
	// entry, or a nil slice, means "infer from the data" (see columnAlign).
	Align []string
	// Markdown is set when the input was a GFM table. Cells may then carry
	// inline `code` and **bold**, which are converted to HTML when rendering.
	Markdown bool
	// Format names the parser that produced Rows. It is only used in the
	// summary line, so the user can see what auto-detection decided.
	Format string
}

// Cols returns the column count. After normalize every row has this width.
func (t *Table) Cols() int {
	if len(t.Rows) == 0 {
		return 0
	}
	return len(t.Rows[0])
}

// Parse converts text into a Table. A non-zero delim forces delimited parsing
// with that separator. Otherwise format selects a parser, and "auto" runs
// detection.
func Parse(text, format string, delim rune) (*Table, error) {
	lines := contentLines(text)
	if len(lines) == 0 {
		return nil, errors.New("no input text")
	}

	var (
		t   *Table
		err error
	)
	if delim != 0 {
		t, err = parseDelimited(text, delim, fmt.Sprintf("delimited %q", delim))
	} else {
		switch strings.ToLower(format) {
		case "auto", "":
			t, err = parseAuto(text, lines)
		case "csv":
			t, err = parseDelimited(text, ',', "csv")
		case "tsv":
			t, err = parseDelimited(text, '\t', "tsv")
		case "pipe", "md", "markdown", "psql", "box", "mysql":
			t = parsePipe(lines)
		case "ws", "aligned", "space", "fixed":
			t = &Table{Rows: parseAligned(lines), Format: "space-aligned"}
		default:
			return nil, fmt.Errorf("unknown format %q (want auto, csv, tsv, pipe, ws)", format)
		}
	}
	if err != nil {
		return nil, err
	}

	t.normalize()
	if len(t.Rows) == 0 {
		return nil, errors.New("no tabular data found")
	}
	return t, nil
}

// parseAuto picks a parser by looking at the shape of the text. The order
// matters, because the signals overlap. CSV values can contain pipes,
// aligned output can contain commas, and Markdown cells can contain both.
// Each rule therefore runs only after the more specific rules above it:
//
//	rule  condition                                   → parser
//	1     every line has a tab                        → TSV (spreadsheet copy)
//	2     every content line has a vertical bar       → pipe (md/psql/box)
//	3     CSV parse with identical field count ≥2     → CSV (',' or ';')
//	3b    CSV parse, header ≥2 fields and most        → CSV, ragged rows
//	      records ≥2 fields                             padded by normalize
//	4     at least half the content lines have a bar  → pipe, ignoring prose
//	                                                    or captions around it
//	5     otherwise                                   → whitespace-aligned
//
// Rule 3 runs before rule 4 so a CSV whose values sometimes contain '|'
// still parses as CSV. Its strict "identical count" test keeps aligned
// output with commas in only some lines (e.g. "1,234" under a header that
// has no comma) from being misread as CSV.
//
// Rule 3b accepts ragged CSV, such as exports that drop trailing empty
// fields. It only runs when no delimiter passes rule 3, so a clean match
// for one delimiter always beats a loose match for the other. See
// relaxedDelimiter for why requiring a delimited header keeps aligned
// output out.
func parseAuto(text string, lines []string) (*Table, error) {
	content := filter(lines, func(s string) bool { return !isBorder(s) })
	bars := count(content, hasBar)

	switch {
	case count(lines, func(s string) bool { return strings.ContainsRune(s, '\t') }) == len(lines):
		return parseDelimited(text, '\t', "tsv")
	case len(content) > 0 && bars == len(content):
		return parsePipe(lines), nil
	}
	d, ok := strictDelimiter(text)
	if !ok {
		d, ok = relaxedDelimiter(text)
	}
	if ok {
		name := "csv"
		if d == ';' {
			name = "semicolon csv"
		}
		return parseDelimited(text, d, name)
	}
	if bars > 0 && 2*bars >= len(content) {
		return parsePipe(lines), nil
	}
	return &Table{Rows: parseAligned(lines), Format: "space-aligned"}, nil
}

// ---------------------------------------------------------------------------
// Delimited (CSV / TSV / custom)
// ---------------------------------------------------------------------------

// parseDelimited uses encoding/csv so quoted fields, embedded delimiters,
// doubled quotes, and multi-line cells all behave as RFC 4180 says.
// LazyQuotes tolerates the stray '"' common in hand-made or TSV data, such as
// 15" monitor. Ragged rows are allowed here and padded later by normalize.
func parseDelimited(text string, delim rune, name string) (*Table, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	// TrimLeadingSpace is deliberately left off. With a tab delimiter it
	// would swallow consecutive tabs and silently merge empty cells.
	// normalize trims cells after splitting instead.
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	return &Table{Rows: rows, Format: name}, nil
}

// strictDelimiter reports whether text parses cleanly as CSV with ',' or ';'
// and the same number of fields (at least 2) on every record. FieldsPerRecord
// = 0 makes encoding/csv enforce that, using the first record's width. When
// both delimiters qualify, the one giving more columns wins. For example,
// "a;b;c,d" is 3 columns of semicolon CSV, not 2 columns of comma CSV.
func strictDelimiter(text string) (rune, bool) {
	best, bestCols := rune(0), 1
	for _, d := range []rune{',', ';'} {
		r := csv.NewReader(strings.NewReader(text))
		r.Comma = d
		r.FieldsPerRecord = 0
		r.LazyQuotes = true
		rows, err := r.ReadAll()
		if err != nil || len(rows) == 0 {
			continue
		}
		if n := len(rows[0]); n > bestCols {
			best, bestCols = d, n
		}
	}
	return best, best != 0
}

// relaxedDelimiter is the fallback for CSV whose records have different
// widths. A delimiter qualifies when:
//
//  1. the first record (the header) has at least 2 fields, and
//  2. more than half of all records have at least 2 fields.
//
// Condition 1 does most of the work against false positives. In aligned
// CLI output the commas live in the data ("0.12, 0.30", "1,234"), while
// the header is plain words separated by spaces, so it parses as a single
// field and the delimiter is rejected. Condition 2 lets through a CSV with
// a few one-field lines, such as a trailing note or a row with only its
// first column filled, while rejecting text where the header line is the
// only one that happens to contain a comma.
//
// The record widths themselves are not compared, so these all pass
// (normalize later pads every row to the widest):
//
//	a,b,c        header 3
//	1,2          short row
//	3,4,5,6      long row
//
// When both delimiters qualify, the wider header wins, matching
// strictDelimiter.
func relaxedDelimiter(text string) (rune, bool) {
	best, bestCols := rune(0), 1
	for _, d := range []rune{',', ';'} {
		r := csv.NewReader(strings.NewReader(text))
		r.Comma = d
		r.FieldsPerRecord = -1 // allow ragged records; widths checked below
		r.LazyQuotes = true
		rows, err := r.ReadAll()
		if err != nil || len(rows) == 0 || len(rows[0]) < 2 {
			continue
		}
		multi := 0
		for _, row := range rows {
			if len(row) >= 2 {
				multi++
			}
		}
		if 2*multi <= len(rows) {
			continue
		}
		if n := len(rows[0]); n > bestCols {
			best, bestCols = d, n
		}
	}
	return best, best != 0
}

// ---------------------------------------------------------------------------
// Pipe tables: Markdown, psql, MySQL, Unicode box drawing
// ---------------------------------------------------------------------------
//
// All four share one idea: cells are separated by a vertical bar, and some
// lines are pure decoration. Examples of what is accepted:
//
//	Markdown            psql                 MySQL / box drawing
//	| a | b |            a | b               +---+---+    ┌───┬───┐
//	|:--|--:|           ---+---              | a | b |    │ a │ b │
//	| 1 | 2 |            1 | 2               +---+---+    ├───┼───┤
//	                    (1 row)              | 1 | 2 |    │ 1 │ 2 │
//	                                         +---+---+    └───┴───┘
//
// Decoration lines (isBorder) are skipped, except that a Markdown separator
// donates its column alignment. Lines with no bar at all, such as footers
// and captions, are also skipped. Bar-like box characters are folded to '|'
// so a single splitter handles every style.

// barRunes are the characters treated as column separators. These are the
// ASCII pipe and the light, heavy, and double box-drawing verticals.
const barRunes = "|│┃║"

var barReplacer = strings.NewReplacer("│", "|", "┃", "|", "║", "|")

func hasBar(s string) bool { return strings.ContainsAny(s, barRunes) }

// isBorder reports whether a line is pure table decoration: only rule
// characters (-=+:| and the Unicode box-drawing block U+2500–U+257F) plus
// spaces, with at least one horizontal stroke. The horizontal-stroke
// requirement keeps a content row whose cells are all empty, such as "│ │",
// from being discarded.
func isBorder(s string) bool {
	horizontal := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == ':' || r == '|' || r == '+':
		case r == '-' || r == '=':
			horizontal = true
		case r >= 0x2500 && r <= 0x257F:
			// ─ ━ ═ ┄ ┅ ┈ ┉ ╌ ╍ and their kin are the horizontal strokes.
			// Junctions (┼ ┬ ├ …) and verticals are decoration but not
			// enough on their own.
			if strings.ContainsRune("─━═┄┅┈┉╌╍", r) {
				horizontal = true
			}
		default:
			return false
		}
	}
	return horizontal
}

// parsePipe implements the pipe-table parsing described above.
func parsePipe(lines []string) *Table {
	t := &Table{Format: "pipe table"}
	var body []string
	for _, ln := range lines {
		if isBorder(ln) {
			// Only the first Markdown separator sets alignment. Later rule
			// lines in box/MySQL grids carry no alignment information.
			if t.Align == nil {
				if a, ok := markdownAlign(ln); ok {
					t.Align, t.Markdown, t.Format = a, true, "markdown"
				}
			}
			continue
		}
		if !hasBar(ln) {
			continue
		}
		body = append(body, barReplacer.Replace(strings.TrimSpace(ln)))
	}

	// "Framed" tables (Markdown, MySQL, box) put a bar at both ends of each
	// row, and those outer bars must be stripped or every row gains an empty
	// first and last cell. psql does not frame its rows, and stripping
	// there would destroy a genuinely empty first or last cell. The style
	// is decided by majority across rows instead of per row, so one psql
	// row that happens to start with an empty cell ("| x |" after trimming)
	// is not mistaken for framing.
	framed := 2*count(body, func(s string) bool {
		return strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") && len(s) > 1
	}) > len(body)

	for _, ln := range body {
		t.Rows = append(t.Rows, splitPipeRow(ln, framed))
	}
	return t
}

// splitPipeRow splits one bar-normalized row. A GFM-escaped "\|" is a
// literal pipe inside a cell, not a separator.
func splitPipeRow(ln string, framed bool) []string {
	if framed {
		ln = strings.TrimPrefix(ln, "|")
		if strings.HasSuffix(ln, "|") && !strings.HasSuffix(ln, `\|`) {
			ln = strings.TrimSuffix(ln, "|")
		}
	}
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(ln); i++ {
		switch {
		case ln[i] == '\\' && i+1 < len(ln) && ln[i+1] == '|':
			cur.WriteByte('|')
			i++
		case ln[i] == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(ln[i])
		}
	}
	return append(cells, cur.String())
}

// markdownAlign recognizes a GFM delimiter row (only '|', ':', '-', and
// spaces, containing both '|' and '-'). It returns per-column alignment:
// ":--" left, "--:" right, ":-:" center, "---" unspecified (""). psql
// ("---+---") and MySQL ("+---+") rules contain '+', so they fall through
// to false.
func markdownAlign(ln string) ([]string, bool) {
	s := strings.TrimSpace(ln)
	if strings.Trim(s, "|:- \t") != "" || !strings.Contains(s, "|") || !strings.Contains(s, "-") {
		return nil, false
	}
	var align []string
	for _, c := range splitPipeRow(s, strings.HasPrefix(s, "|")) {
		c = strings.TrimSpace(c)
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		switch {
		case left && right:
			align = append(align, "center")
		case right:
			align = append(align, "right")
		case left:
			align = append(align, "left")
		default:
			align = append(align, "")
		}
	}
	return align, true
}

// ---------------------------------------------------------------------------
// Whitespace-aligned columns
// ---------------------------------------------------------------------------
//
// Splitting on whitespace alone breaks on values that contain spaces, such as
// "all done" or "2 days ago". Instead, gutters are found: character
// positions that hold a space on every line. Content between gutters is a
// column, so multi-word values stay intact.
//
//	NAME    STATUS     AGE           gutter map (^ = space on all lines)
//	web     Running    2 days ago    ....^^^^.......^^^^...........
//	db      Completed  5h            → 3 columns: [0,4) [8,17) [21,…)
//
// Gutters narrower than minGap are ignored, so single spaces inside values
// never split a column. minGap starts at 2, which is what column -t,
// kubectl, docker, and psql emit. It falls back to 1 for output like
// `uniq -c` ("  58 root"), whose only gutter is a single space.
//
// Limitation: positions are counted in runes, so East Asian wide characters,
// which occupy two terminal cells, can shift later columns.

var multiSpace = regexp.MustCompile(`\s{2,}`)

func parseAligned(lines []string) [][]string {
	var grid [][]rune
	width := 0
	for _, ln := range lines {
		if isBorder(ln) {
			continue // psql/rst-style "----  ----" rules under the header
		}
		r := []rune(expandTabs(ln))
		grid = append(grid, r)
		width = max(width, len(r))
	}

	for _, minGap := range []int{2, 1} {
		if spans := columnSpans(grid, width, minGap); len(spans) >= 2 {
			if rows, ok := cutSpans(grid, spans); ok {
				return rows
			}
		}
	}

	// No gutter survives across all lines. Usually one long value has
	// overrun its column. Fall back to splitting each line on its own runs
	// of 2+ spaces, which keeps multi-word values together at the cost of
	// possibly ragged rows (normalize pads them).
	var rows [][]string
	multi := 0
	for _, ln := range lines {
		if isBorder(ln) {
			continue
		}
		cells := multiSpace.Split(strings.TrimSpace(ln), -1)
		if len(cells) > 1 {
			multi++
		}
		rows = append(rows, cells)
	}
	if 2*multi > len(rows) {
		return rows
	}

	// Last resort: every whitespace run is a separator.
	rows = rows[:0]
	for _, ln := range lines {
		if !isBorder(ln) {
			rows = append(rows, strings.Fields(ln))
		}
	}
	return rows
}

// columnSpans returns [start, end) rune ranges of the columns. It scans for
// runs of non-gutter positions and merges runs separated by a gutter
// narrower than minGap. Leading indentation and trailing space are never
// part of a span, so they are dropped naturally.
func columnSpans(grid [][]rune, width, minGap int) [][2]int {
	gutter := make([]bool, width)
	for i := range gutter {
		gutter[i] = true
		for _, row := range grid {
			if i < len(row) && row[i] != ' ' {
				gutter[i] = false
				break
			}
		}
	}

	var spans [][2]int
	for i := 0; i < width; {
		if gutter[i] {
			i++
			continue
		}
		start := i
		for i < width && !gutter[i] {
			i++
		}
		// Absorb a narrow gutter: it is a space inside a value, not a
		// column break.
		if n := len(spans); n > 0 && start-spans[n-1][1] < minGap {
			spans[n-1][1] = i
		} else {
			spans = append(spans, [2]int{start, i})
		}
	}
	return spans
}

// cutSpans slices each row by the column spans. Short rows yield empty cells.
//
// The result is rejected (ok=false) if any cell still contains a run of 2+
// spaces. That signals a phantom gutter. Positions past the end of short
// lines count as spaces, so one long row can create a gutter that the
// short rows never actually had:
//
//	KEY   VALUE                             ← short: whole line in span 0
//	averyveryverylongkey  long value here   ← gutter at 20–21 only here
//
// The caller then falls back to splitting each line independently.
func cutSpans(grid [][]rune, spans [][2]int) ([][]string, bool) {
	rows := make([][]string, 0, len(grid))
	for _, row := range grid {
		cells := make([]string, len(spans))
		for j, sp := range spans {
			if sp[0] < len(row) {
				cells[j] = strings.TrimSpace(string(row[sp[0]:min(sp[1], len(row))]))
				if multiSpace.MatchString(cells[j]) {
					return nil, false
				}
			}
		}
		rows = append(rows, cells)
	}
	return rows, true
}

// expandTabs converts tabs to spaces at 8-column stops, matching how a
// terminal displayed the text, so gutter positions line up.
func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Normalization and helpers
// ---------------------------------------------------------------------------

// normalize makes the table rectangular and tidy. It:
//   - trims cell whitespace (HTML would collapse it anyway)
//   - drops rows that are entirely empty
//   - pads ragged rows to the widest row
//   - drops trailing columns that are empty in every row, which typically
//     come from a trailing delimiter on every line
func (t *Table) normalize() {
	rows := t.Rows[:0]
	width := 0
	for _, r := range t.Rows {
		empty := true
		for j := range r {
			r[j] = strings.TrimSpace(r[j])
			if r[j] != "" {
				empty = false
			}
		}
		if !empty {
			rows = append(rows, r)
			width = max(width, len(r))
		}
	}

	// Find the last column holding any content.
	used := 0
	for _, r := range rows {
		for j := len(r) - 1; j >= used; j-- {
			if r[j] != "" {
				used = j + 1
				break
			}
		}
	}
	width = min(width, used)

	for i, r := range rows {
		switch {
		case len(r) < width:
			rows[i] = append(r, make([]string, width-len(r))...)
		case len(r) > width:
			rows[i] = r[:width]
		}
	}
	t.Rows = rows
	if len(t.Align) > width {
		t.Align = t.Align[:width]
	}
}

// footer matches the row-count trailers psql and MySQL print after results.
// It is anchored and specific so that a data line like "3 rows" in a
// one-column table survives.
var footer = regexp.MustCompile(`^\(\d+ rows?\)$|^\d+ rows? in set\b`)

// contentLines returns the non-blank lines of text, minus SQL client footers.
func contentLines(text string) []string {
	var out []string
	for _, ln := range strings.Split(text, "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || footer.MatchString(s) {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func filter(ss []string, keep func(string) bool) []string {
	var out []string
	for _, s := range ss {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

func count(ss []string, pred func(string) bool) int {
	n := 0
	for _, s := range ss {
		if pred(s) {
			n++
		}
	}
	return n
}
