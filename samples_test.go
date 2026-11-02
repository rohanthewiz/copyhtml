package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSamples runs every file in testdata/ through the same path the CLI
// uses (normalizeText → Parse) and checks what came out.
//
// Design choices:
//
//   - The samples are real-world shaped files (Excel exports, kubectl,
//     psql, ...) that complement the small inline strings in parse_test.go.
//     Keeping them as files means they can also be fed to the binary by hand
//     (`copyhtml -stdout testdata/x`) when debugging detection.
//   - Each case asserts the detected format and the table shape, then spot
//     checks a few cells rather than the full grid. The cells chosen are the
//     ones that exercise the edge case named in the file (a value containing
//     spaces, an escaped quote, an empty cell, ...). Full-grid comparisons
//     for these sizes would mostly restate the input and make the test noisy
//     to update.
//   - TestSamplesCoverAllFiles guards against a sample being added to
//     testdata/ without a matching case here.
//
// Row counts include the header row: Parse does not know about headers,
// that is decided at render time.
func TestSamples(t *testing.T) {
	// cell addresses a single [row, col] position in Table.Rows.
	type cell struct{ row, col int }

	tests := []struct {
		file   string
		format string // -f value passed to Parse; "" means auto
		// wantErr, when set, is a substring of the expected Parse error.
		// The remaining fields are ignored in that case.
		wantErr    string
		wantFormat string // expected Table.Format
		rows, cols int
		cells      map[cell]string
		markdown   bool     // expected Table.Markdown
		align      []string // expected Table.Align; nil skips the check
	}{
		{
			file: "csv_basic.csv", wantFormat: "csv", rows: 5, cols: 4,
			cells: map[cell]string{{0, 3}: "Start Date", {4, 2}: "76000.50"},
		},
		{
			// Quoted comma, doubled-quote escape, a cell spanning two
			// lines, and HTML that must survive parsing untouched (it is
			// escaped later, by the renderer).
			file: "csv_quoted.csv", wantFormat: "csv", rows: 4, cols: 4,
			cells: map[cell]string{
				{1, 1}: "Widget, Large",
				{1, 2}: `A big widget, with "extra" features`,
				{2, 2}: "Multi-line\ndescription here",
				{3, 2}: "Contains <b>html</b> & ampersands",
			},
		},
		{
			// The BOM must not end up glued to "Region", and CRLF must not
			// leave a trailing '\r' on the last column.
			file: "csv_excel_bom_crlf.csv", wantFormat: "csv", rows: 4, cols: 5,
			cells: map[cell]string{{0, 0}: "Region", {0, 4}: "Q4", {3, 4}: "1700"},
		},
		{
			// '.' thousands separators and non-ASCII headers; ',' never
			// appears so only ';' gives a consistent field count.
			file: "semicolon.csv", wantFormat: "semicolon csv", rows: 4, cols: 4,
			cells: map[cell]string{{0, 3}: "Fläche km²", {2, 0}: "Österreich", {1, 2}: "83.200.000"},
		},
		{
			file: "tsv_spreadsheet.tsv", wantFormat: "tsv", rows: 5, cols: 4,
			cells: map[cell]string{{1, 1}: "Apple Inc.", {4, 2}: "", {4, 3}: "-1.05%"},
		},
		{
			file: "unicode_wide.tsv", wantFormat: "tsv", rows: 4, cols: 3,
			cells: map[cell]string{{0, 0}: "名前", {1, 2}: "🍣", {2, 1}: "São Paulo", {3, 1}: "København"},
		},
		{
			// The quoted "38.00" and the empty Qty/Unit Price cells on the
			// totals row.
			file: "numbers_align.csv", wantFormat: "csv", rows: 5, cols: 5,
			cells: map[cell]string{{4, 1}: "", {4, 2}: "", {4, 3}: "38.00", {2, 4}: "75%"},
		},
		{
			// Markdown inline syntax stays raw in the parsed cells; it is
			// converted to <b>/<code> by RenderHTML when Markdown is set.
			file: "markdown.md", wantFormat: "markdown", rows: 4, cols: 4,
			markdown: true, align: []string{"left", "center", "right", ""},
			cells: map[cell]string{{1, 1}: "**Done**", {1, 3}: "Uses `OAuth2`", {3, 3}: "<needs design>"},
		},
		{
			// The "(3 rows)" footer has no bar and must be dropped.
			file: "psql.txt", wantFormat: "pipe table", rows: 4, cols: 4,
			cells: map[cell]string{{0, 2}: "email", {3, 1}: "mlee", {3, 3}: "t"},
		},
		{
			// "+---+" borders skipped; "3 rows in set" footer dropped.
			file: "mysql.txt", wantFormat: "pipe table", rows: 4, cols: 4,
			cells: map[cell]string{{3, 2}: "NULL", {1, 3}: "2026-10-01 09:15:00"},
		},
		{
			// "degraded" touches the next │ with no padding.
			file: "box_unicode.txt", wantFormat: "pipe table", rows: 4, cols: 3,
			cells: map[cell]string{{0, 0}: "Service", {2, 1}: "degraded", {2, 2}: "310ms"},
		},
		{
			// Values with single spaces must stay in one cell.
			file: "kubectl.txt", wantFormat: "space-aligned", rows: 5, cols: 5,
			cells: map[cell]string{{2, 3}: "1 (3h ago)", {1, 4}: "2 days ago", {3, 2}: "CrashLoopBackOff"},
		},
		{
			// Two-word header, quoted COMMAND values, and an empty PORTS
			// cell on the last row that must not shift NAMES left.
			file: "docker_ps.txt", wantFormat: "space-aligned", rows: 4, cols: 7,
			cells: map[cell]string{
				{0, 0}: "CONTAINER ID",
				{1, 2}: `"/docker-entrypoint.…"`,
				{1, 5}: "0.0.0.0:8080->80/tcp",
				{3, 5}: "",
				{3, 6}: "cache",
			},
		},
		{
			// No header line; the first row is still row 0 (the CLI
			// would render it as a header unless -no-header is given).
			file: "uniq_c.txt", wantFormat: "space-aligned", rows: 4, cols: 2,
			cells: map[cell]string{{0, 0}: "142", {0, 1}: "GET /api/users", {3, 1}: "PUT /api/users/42"},
		},
		{
			// Commas appear in only some lines, so the strict CSV rule
			// rejects it and "0.12, 0.30" stays one cell.
			file: "ambiguous_commas_in_ws.txt", wantFormat: "space-aligned", rows: 4, cols: 3,
			cells: map[cell]string{{1, 1}: "0.12, 0.30", {2, 2}: "12"},
		},
		{
			// Known limitation, pinned so a change to detection is a
			// deliberate decision: auto requires an identical field count
			// on every row before choosing CSV, so a ragged CSV falls
			// through to space-aligned and each line becomes one cell.
			file: "ragged.csv", wantFormat: "space-aligned", rows: 4, cols: 1,
			cells: map[cell]string{{0, 0}: "a,b,c", {2, 0}: "3,4,5,6"},
		},
		{
			// Forcing CSV parses it, with short rows padded to the widest.
			file: "ragged.csv", format: "csv", wantFormat: "csv", rows: 4, cols: 4,
			cells: map[cell]string{{0, 3}: "", {1, 1}: "2", {1, 2}: "", {2, 3}: "6", {3, 1}: ""},
		},
		{
			// Blank lines between and after rows are dropped.
			file: "blank_lines.csv", wantFormat: "csv", rows: 2, cols: 2,
			cells: map[cell]string{{0, 0}: "col1", {1, 1}: "y"},
		},
		{
			file: "single_row.csv", wantFormat: "csv", rows: 1, cols: 3,
			cells: map[cell]string{{0, 2}: "row"},
		},
		{
			file: "empty.txt", wantErr: "no input text",
		},
	}

	for _, tc := range tests {
		name := tc.file
		if tc.format != "" {
			name += "/-f=" + tc.format
		}
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			format := tc.format
			if format == "" {
				format = "auto"
			}
			got, err := Parse(normalizeText(string(b)), format, 0)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if got.Format != tc.wantFormat {
				t.Errorf("Format = %q, want %q", got.Format, tc.wantFormat)
			}
			if len(got.Rows) != tc.rows || got.Cols() != tc.cols {
				// Shape is wrong, so cell addresses below are meaningless;
				// dump the rows to make the mismatch easy to diagnose.
				t.Fatalf("shape = %d×%d, want %d×%d\nrows: %q",
					len(got.Rows), got.Cols(), tc.rows, tc.cols, got.Rows)
			}
			// normalize promises a rectangular table; check every row so a
			// padding bug shows up even in cells not spot-checked below.
			for i, r := range got.Rows {
				if len(r) != tc.cols {
					t.Errorf("row %d has %d cells, want %d: %q", i, len(r), tc.cols, r)
				}
			}
			for c, want := range tc.cells {
				if g := got.Rows[c.row][c.col]; g != want {
					t.Errorf("cell[%d][%d] = %q, want %q", c.row, c.col, g, want)
				}
			}
			if got.Markdown != tc.markdown {
				t.Errorf("Markdown = %v, want %v", got.Markdown, tc.markdown)
			}
			if tc.align != nil {
				for i, want := range tc.align {
					var g string
					if i < len(got.Align) {
						g = got.Align[i]
					}
					if g != want {
						t.Errorf("Align[%d] = %q, want %q (Align = %q)", i, g, want, got.Align)
					}
				}
			}
		})
	}
}

// TestSamplesCLI drives a few samples through run() to cover behavior that
// lives above Parse: reading a file argument, header detection, and HTML
// escaping in the rendered output.
func TestSamplesCLI(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string // substrings that must appear in stdout
		notWant []string // substrings that must not appear
	}{
		{
			// A lone row renders as data, never as a header-only table.
			name:    "single row has no header",
			args:    []string{"-stdout", "testdata/single_row.csv"},
			want:    []string{">only</td>"},
			notWant: []string{"<th"},
		},
		{
			name:    "-no-header keeps first uniq -c line as data",
			args:    []string{"-stdout", "-no-header", "testdata/uniq_c.txt"},
			want:    []string{">142</td>"},
			notWant: []string{"<th"},
		},
		{
			// Raw HTML in a CSV cell is escaped, and the two-line cell
			// becomes a <br>.
			name: "csv cells are escaped",
			args: []string{"-stdout", "testdata/csv_quoted.csv"},
			want: []string{
				"Contains &lt;b&gt;html&lt;/b&gt; &amp; ampersands",
				"Multi-line<br>description here",
				">id</th>",
			},
			notWant: []string{"<b>html</b>"},
		},
		{
			// Markdown inline syntax is converted, but a literal <...>
			// in a Markdown cell is still escaped.
			name:    "markdown inline syntax",
			args:    []string{"-stdout", "testdata/markdown.md"},
			want:    []string{"<b>Done</b>", "<code>OAuth2</code>", "&lt;needs design&gt;"},
			notWant: []string{"**Done**", "<needs design>"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := run(tc.args, strings.NewReader(""), &out, &errOut); err != nil {
				t.Fatalf("run: %v (stderr: %s)", err, errOut.String())
			}
			html := out.String()
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("output missing %q\n got: %s", w, html)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("output should not contain %q\n got: %s", w, html)
				}
			}
		})
	}
}

// TestSamplesCoverAllFiles fails when a file is added to testdata/ without
// a case in TestSamples, so new samples cannot silently go untested.
func TestSamplesCoverAllFiles(t *testing.T) {
	covered := map[string]bool{
		"README.md": true, // documentation, not a sample
	}
	for _, f := range []string{
		"csv_basic.csv", "csv_quoted.csv", "csv_excel_bom_crlf.csv", "semicolon.csv",
		"tsv_spreadsheet.tsv", "unicode_wide.tsv", "numbers_align.csv", "markdown.md",
		"psql.txt", "mysql.txt", "box_unicode.txt", "kubectl.txt", "docker_ps.txt",
		"uniq_c.txt", "ambiguous_commas_in_ws.txt", "ragged.csv", "blank_lines.csv",
		"single_row.csv", "empty.txt",
	} {
		covered[f] = true
	}
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && !covered[e.Name()] {
			t.Errorf("testdata/%s has no case in TestSamples", e.Name())
		}
	}
}
