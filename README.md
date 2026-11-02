# copyhtml

Reads text containing a table and puts it on the macOS clipboard as an HTML
table. Pasting into Teams, Outlook, Slack or Confluence then gives a real
bordered table, not raw text or literal tags.

## Install

```sh
cd copyhtml
go install        # installs to ~/go/bin/copyhtml
# or
go build -o copyhtml .
```

Pure Go, standard library only.

## Usage

```sh
copyhtml data.csv             # from a file
kubectl get pods | copyhtml   # from a pipe
copyhtml                      # nothing piped in: reads the clipboard
copyhtml -stdout data.csv     # print the HTML instead of copying it
```

Run with no arguments and nothing piped in, it reads the clipboard. So you can
copy table text from anywhere, run `copyhtml`, and paste into Teams.

### Flags

| Flag | Meaning |
|---|---|
| `-f` | Input format: `auto` (default), `csv`, `tsv`, `pipe` (Markdown/psql/box), `ws` (space-aligned columns) |
| `-d` | Explicit single-character delimiter, e.g. `';'` or `'\t'`. Overrides `-f` |
| `-no-header` | Treat the first row as data rather than a header |
| `-stdout` | Write the HTML to stdout instead of the clipboard |
| `-q` | Suppress the summary line on stderr |

## Input it recognises

- **CSV**, including quoted cells, embedded commas and multi-line cells
- **Semicolon-separated** files
- **TSV**, which is what Excel and Google Sheets put on the clipboard
- **Pipe tables**: Markdown, psql, MySQL `+---+` grids, and Unicode box drawing
  (`┌─┬─┐`), like the tables Claude Code draws in the terminal
- **Columns aligned with spaces**, like `kubectl` or `docker ps` output.
  Values containing spaces, such as "2 days ago", stay in one cell.

If the format is guessed wrong, force it with `-f csv|tsv|pipe|ws`, or set the
separator with `-d ';'`.

Also handled:

- A UTF-8 BOM (from Excel CSV exports) is stripped.
- CRLF and lone-CR line endings are normalized.
- Row-count footers from psql and MySQL (`(3 rows)`, `1 row in set`) are dropped.
- Lines with no bars around a pipe table, such as captions or notes, are skipped.

### How auto-detection decides

The rules are tried in order, and the first match wins:

| # | Condition | Parser |
|---|---|---|
| 1 | Every line has a tab | TSV |
| 2 | Every content line has a vertical bar | Pipe table |
| 3 | Parses as CSV with the same field count (2 or more) on every record | CSV (`,` or `;`) |
| 4 | At least half the content lines have a bar | Pipe table (ignores surrounding prose) |
| 5 | Otherwise | Space-aligned columns |

Space-aligned columns are split at *gutters*: character positions that are a
space on every line. Values that contain single spaces therefore don't get
split. If no gutter is consistent across all lines, for example because one
long value overruns its column, each line is split on runs of two or more
spaces instead.

## What the output looks like

- Borders are inline styles, because Teams drops `<style>` blocks and class
  attributes when you paste.
- There are no background colours, so the header stays readable in Teams'
  dark theme.
- The header row goes in `<thead>` as `<th>`, so it renders bold everywhere.
- Columns where every value is a number (`125`, `1,234`, `$5.00`, `12%`) are
  right-aligned. Markdown alignment rows (`|:--|--:|:-:|`) take precedence.
- In Markdown input, `` `code` `` and `**bold**` become formatting. Italics
  are not converted, since underscores are common in identifiers.
- All cell text is HTML-escaped before any formatting is applied.
- A tab-separated plain-text copy goes on the clipboard too, for apps that
  won't take HTML.
- A one-line summary on stderr says what was found, e.g.
  `copied 25 rows × 14 columns (+ header, csv, from data.csv) to the clipboard as HTML`.

## Platform notes

- Clipboard access is macOS-only. It goes through `osascript` (JavaScript for
  Automation, which calls `NSPasteboard`), setting both `public.html` and
  `public.utf8-plain-text`. Data is passed over stdin as JSON, so large
  tables don't hit argument-length limits.
- On other platforms, use `-stdout`.
- Column positions are counted in characters, so East Asian wide characters
  can misalign space-aligned input.

## Layout

| File | Contents |
|---|---|
| `main.go` | Flags, input selection (file / stdin / clipboard), orchestration |
| `parse.go` | Format detection and the CSV, pipe and space-aligned parsers |
| `render.go` | HTML and TSV rendering, numeric alignment, inline Markdown |
| `clipboard_darwin.go` | macOS clipboard read/write via `osascript` |
| `clipboard_other.go` | Stub for other platforms (`-stdout` only) |
| `parse_test.go` | Tests for detection, parsing, rendering and the CLI path |

```sh
go test ./...
```
