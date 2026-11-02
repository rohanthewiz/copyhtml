// Command copyhtml reads text containing tabular data and places it on the
// macOS clipboard as an HTML table, so it pastes as a real, bordered table
// into rich-text targets such as Teams, Outlook, Slack, or Confluence.
//
// Supported input shapes (auto-detected, or forced with -f / -d):
//
//   - CSV / semicolon-separated (RFC 4180 quoting via encoding/csv)
//   - TSV (what Excel / Google Sheets put on the clipboard)
//   - Pipe tables: Markdown, psql, MySQL "+---+" grids, Unicode box drawing
//   - Whitespace-aligned columns (kubectl, docker ps, `uniq -c`, column -t)
//
// Data flow:
//
//	file arg │ stdin │ clipboard (when stdin is a terminal)
//	                 │
//	                 ▼
//	       Parse: detect format ──► [][]string ──► normalize
//	                 │                              (trim, pad, drop empty)
//	                 ▼
//	  RenderHTML (inline styles)   +   RenderTSV (plain-text fallback)
//	                 │
//	                 ▼
//	clipboard: public.html + public.utf8-plain-text   (or -stdout)
//
// Usage:
//
//	copyhtml [flags] [file|-]
//	pbpaste | copyhtml            # explicit pipe
//	copyhtml                      # no input piped: reads the clipboard itself
//	copyhtml -stdout data.csv     # print HTML instead of copying
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "copyhtml:", err)
		os.Exit(1)
	}
}

// run is main minus process concerns (exit codes, global streams), so the
// whole CLI path can be driven from tests.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("copyhtml", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("f", "auto", "input format: auto, csv, tsv, pipe (markdown/psql/box), ws (space-aligned columns)")
	delim := fs.String("d", "", `explicit single-character field delimiter, e.g. ';' or '\t' (overrides -f)`)
	noHeader := fs.Bool("no-header", false, "treat the first row as data rather than a header")
	toStdout := fs.Bool("stdout", false, "write the HTML to stdout instead of the clipboard")
	quiet := fs.Bool("q", false, "suppress the summary line on stderr")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: copyhtml [flags] [file|-]")
		fmt.Fprintln(stderr, "Reads tabular text (file, stdin, or the clipboard when nothing is piped in)")
		fmt.Fprintln(stderr, "and puts it on the clipboard as an HTML table.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	delimRune, err := parseDelimFlag(*delim)
	if err != nil {
		return err
	}

	text, source, err := readInput(fs.Args(), stdin)
	if err != nil {
		return err
	}

	t, err := Parse(normalizeText(text), *format, delimRune)
	if err != nil {
		return err
	}

	// A lone row is rendered as data: a table that is nothing but a header
	// looks like a mistake when pasted.
	hasHeader := !*noHeader && len(t.Rows) > 1
	html := RenderHTML(t, hasHeader)

	if *toStdout {
		_, err := fmt.Fprintln(stdout, html)
		return err
	}
	if err := writeClipboard(html, RenderTSV(t)); err != nil {
		return err
	}

	if !*quiet {
		dataRows := len(t.Rows)
		headerNote := "no header"
		if hasHeader {
			dataRows--
			headerNote = "+ header"
		}
		fmt.Fprintf(stderr, "copyhtml: copied %d rows × %d columns (%s, %s, from %s) to the clipboard as HTML\n",
			dataRows, t.Cols(), headerNote, t.Format, source)
	}
	return nil
}

// readInput resolves where the table text comes from. When no file is given
// and stdin is an interactive terminal (nothing piped in), it reads the
// clipboard instead of blocking on the keyboard. That makes the common
// workflow "copy table text → run copyhtml → paste in Teams" a single
// command, with the clipboard serving as both input and output.
func readInput(args []string, stdin io.Reader) (text, source string, err error) {
	switch {
	case len(args) > 1:
		return "", "", fmt.Errorf("expected at most one input file, got %d", len(args))
	case len(args) == 1 && args[0] != "-":
		b, err := os.ReadFile(args[0])
		if err != nil {
			return "", "", err
		}
		return string(b), args[0], nil
	case len(args) == 0 && isTerminal(stdin):
		s, err := readClipboard()
		if err != nil {
			return "", "", fmt.Errorf("reading clipboard: %w", err)
		}
		return s, "clipboard", nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", "", err
	}
	return string(b), "stdin", nil
}

// isTerminal reports whether r is a character device (a TTY) rather than a
// pipe or file. Checking ModeCharDevice avoids a golang.org/x/term
// dependency for the one bit of information we need.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// normalizeText strips a UTF-8 BOM (Excel CSV exports start with one, and it
// would otherwise end up glued to the first header cell) and converts CRLF
// and lone CR line endings to LF. Lone CR still shows up in clipboard text
// from some older Mac apps.
func normalizeText(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// parseDelimFlag accepts a single character, or the escape spellings people
// naturally type for tab, since a literal tab is awkward to enter in a shell.
func parseDelimFlag(s string) (rune, error) {
	switch s {
	case "":
		return 0, nil
	case `\t`, "tab", "TAB":
		return '\t', nil
	}
	if utf8.RuneCountInString(s) != 1 {
		return 0, fmt.Errorf("-d must be a single character, got %q", s)
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == '"' || r == '\n' {
		return 0, fmt.Errorf("-d cannot be %q", r)
	}
	return r, nil
}
