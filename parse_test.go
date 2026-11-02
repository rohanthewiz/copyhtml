package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestParseAuto(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		format string // expected Table.Format
		rows   [][]string
	}{
		{
			name: "csv with empty fields",
			in: "op_id,seq,kind,reason,state\n" +
				"op-1,1,root,,done\n" +
				"op-1,2,child,,done\n",
			format: "csv",
			rows: [][]string{
				{"op_id", "seq", "kind", "reason", "state"},
				{"op-1", "1", "root", "", "done"},
				{"op-1", "2", "child", "", "done"},
			},
		},
		{
			name:   "csv quoting: embedded comma, quote, newline",
			in:     "name,note\n\"Smith, J\",\"said \"\"hi\"\"\"\nx,\"line1\nline2\"\n",
			format: "csv",
			rows:   [][]string{{"name", "note"}, {"Smith, J", `said "hi"`}, {"x", "line1\nline2"}},
		},
		{
			name:   "csv whose values contain pipes stays csv",
			in:     "id,tags\n1,a|b\n2,c\n",
			format: "csv",
			rows:   [][]string{{"id", "tags"}, {"1", "a|b"}, {"2", "c"}},
		},
		{
			name:   "semicolon csv",
			in:     "a;b;c\n1,5;2;3\n",
			format: "semicolon csv",
			rows:   [][]string{{"a", "b", "c"}, {"1,5", "2", "3"}},
		},
		{
			name:   "tsv keeps empty middle cell",
			in:     "a\tb\tc\n1\t\t3\n",
			format: "tsv",
			rows:   [][]string{{"a", "b", "c"}, {"1", "", "3"}},
		},
		{
			name: "markdown with surrounding prose",
			in: "Here is the summary:\n\n" +
				"| File | Items |\n" +
				"|:---|---:|\n" +
				"| `a.csv` | 61 |\n" +
				"| **Total** | **61** |\n\n" +
				"Let me know.\n",
			format: "markdown",
			rows:   [][]string{{"File", "Items"}, {"`a.csv`", "61"}, {"**Total**", "**61**"}},
		},
		{
			name: "psql with footer and empty first cell",
			in: " id | name  | note\n" +
				"----+-------+------\n" +
				"  1 | alpha | x\n" +
				"    | beta  |\n" +
				"(2 rows)\n",
			format: "pipe table",
			rows:   [][]string{{"id", "name", "note"}, {"1", "alpha", "x"}, {"", "beta", ""}},
		},
		{
			name: "mysql grid with footer",
			in: "+----+-------+\n" +
				"| id | name  |\n" +
				"+----+-------+\n" +
				"|  1 | alpha |\n" +
				"+----+-------+\n" +
				"1 row in set (0.00 sec)\n",
			format: "pipe table",
			rows:   [][]string{{"id", "name"}, {"1", "alpha"}},
		},
		{
			name: "unicode box drawing",
			in: "┌──────┬───────┐\n" +
				"│ File │ Items │\n" +
				"├──────┼───────┤\n" +
				"│ a    │ 61    │\n" +
				"│ b    │       │\n" +
				"└──────┴───────┘\n",
			format: "pipe table",
			rows:   [][]string{{"File", "Items"}, {"a", "61"}, {"b", ""}},
		},
		{
			name: "space aligned keeps multi-word values",
			in: "NAME   STATUS     AGE\n" +
				"web    Running    2 days ago\n" +
				"db     Completed  5h\n",
			format: "space-aligned",
			rows:   [][]string{{"NAME", "STATUS", "AGE"}, {"web", "Running", "2 days ago"}, {"db", "Completed", "5h"}},
		},
		{
			name:   "uniq -c single-space gutter",
			in:     "   3 child\n  58 root\n",
			format: "space-aligned",
			rows:   [][]string{{"3", "child"}, {"58", "root"}},
		},
		{
			name: "aligned with commas in some lines is not csv",
			in: "NAME    COUNT\n" +
				"foo     1,234\n" +
				"bar     5,678\n",
			format: "space-aligned",
			rows:   [][]string{{"NAME", "COUNT"}, {"foo", "1,234"}, {"bar", "5,678"}},
		},
		{
			name: "aligned with overrun column falls back to 2+ space split",
			in: "KEY   VALUE\n" +
				"a     short\n" +
				"averyveryverylongkey  long value here\n",
			format: "space-aligned",
			rows:   [][]string{{"KEY", "VALUE"}, {"a", "short"}, {"averyveryverylongkey", "long value here"}},
		},
		{
			name:   "trailing delimiter column dropped",
			in:     "a,b,\n1,2,\n",
			format: "csv",
			rows:   [][]string{{"a", "b"}, {"1", "2"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(normalizeText(tc.in), "auto", 0)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if got.Format != tc.format {
				t.Errorf("format = %q, want %q", got.Format, tc.format)
			}
			if !reflect.DeepEqual(got.Rows, tc.rows) {
				t.Errorf("rows =\n%q\nwant\n%q", got.Rows, tc.rows)
			}
		})
	}
}

func TestMarkdownAlignment(t *testing.T) {
	tb, err := Parse("| a | b | c | d |\n|:--|--:|:-:|---|\n| x | 1 | y | z |\n", "auto", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"left", "right", "center", ""}
	if !reflect.DeepEqual(tb.Align, want) {
		t.Errorf("Align = %q, want %q", tb.Align, want)
	}
}

func TestForcedDelimiterAndFormat(t *testing.T) {
	tb, err := Parse("a:b\n1:2\n", "auto", ':')
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"a", "b"}, {"1", "2"}}; !reflect.DeepEqual(tb.Rows, want) {
		t.Errorf("rows = %q, want %q", tb.Rows, want)
	}
	if _, err := Parse("a\n", "bogus", 0); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestRenderHTML(t *testing.T) {
	tb, _ := Parse("| File | Items |\n|---|---|\n| `<a.csv>` | 61 |\n| **Total** | **61** |\n", "auto", 0)
	got := RenderHTML(tb, true)
	for _, want := range []string{
		`<th style="border:1px solid #999;padding:4px 8px;text-align:left;">File</th>`,
		`<th style="border:1px solid #999;padding:4px 8px;text-align:right;">Items</th>`, // numeric column
		`<code>&lt;a.csv&gt;</code>`,        // escaped before markdown
		`<b>Total</b>`,                      // bold converted
		`text-align:right;"><b>61</b></td>`, // **61** still counts as numeric
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %s\n got: %s", want, got)
		}
	}
}

func TestRenderEscapesNonMarkdown(t *testing.T) {
	tb, _ := Parse("a,b\n<script>,**x**\n", "auto", 0)
	got := RenderHTML(tb, true)
	if !strings.Contains(got, "&lt;script&gt;") || !strings.Contains(got, "**x**") {
		t.Errorf("CSV cells must be escaped and not markdown-converted: %s", got)
	}
}

func TestRunStdout(t *testing.T) {
	var out, errOut bytes.Buffer
	err := run([]string{"-stdout"}, strings.NewReader("\uFEFFa,b\r\n1,2\r\n"), &out, &errOut)
	if err != nil {
		t.Fatalf("run: %v (stderr: %s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), ">a</th>") || !strings.Contains(out.String(), ">2</td>") {
		t.Errorf("unexpected output: %s", out.String())
	}
}
