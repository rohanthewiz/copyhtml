# Sample inputs

Run any of them with `copyhtml -stdout testdata/<file>`. "Detected as" is the
format name auto-detection picks today, as asserted in `samples_test.go`. The
same name appears in the summary line when copying to the clipboard; it is not
printed with `-stdout`.

| File | Exercises | Detected as |
|---|---|---|
| `csv_basic.csv` | Plain CSV | `csv`, 4 cols |
| `csv_quoted.csv` | Quoted commas, `""` escapes, multi-line cell (→ `<br>`), HTML that must be escaped | `csv`, 4 cols |
| `csv_excel_bom_crlf.csv` | UTF-8 BOM + CRLF line endings, as Excel exports them | `csv`, 5 cols |
| `semicolon.csv` | European `;` separator, non-ASCII headers | `semicolon csv`, 4 cols |
| `tsv_spreadsheet.tsv` | Excel/Sheets clipboard shape, one empty cell | `tsv`, 4 cols |
| `unicode_wide.tsv` | CJK, accents, emoji | `tsv`, 3 cols |
| `numbers_align.csv` | Numeric/percent columns (right-align inference), empty cells | `csv`, 5 cols |
| `markdown.md` | GFM table: `:--`/`:-:`/`--:` alignment, `**bold**`, `` `code` ``, `<`/`&` | `markdown`, 4 cols |
| `psql.txt` | psql output with `(3 rows)` footer | `pipe table`, 4 cols |
| `mysql.txt` | MySQL `+---+` grid with footer | `pipe table`, 4 cols |
| `box_unicode.txt` | Unicode box drawing, a cell flush against `│` | `pipe table`, 3 cols |
| `kubectl.txt` | Space-aligned, values with spaces ("2 days ago", "1 (3h ago)") | `space-aligned`, 5 cols |
| `docker_ps.txt` | Space-aligned, multi-word header ("CONTAINER ID"), an empty PORTS cell | `space-aligned`, 7 cols |
| `uniq_c.txt` | `uniq -c` output: leading spaces, no header line | `space-aligned`, 2 cols |
| `ambiguous_commas_in_ws.txt` | Aligned columns whose values contain commas (must not become CSV) | `space-aligned`, 3 cols |
| `ragged.csv` | Rows with differing field counts | `csv`, 4 cols (short rows padded) |
| `blank_lines.csv` | Blank lines between and after rows | `csv`, 2 cols |
| `single_row.csv` | One row only, rendered as data, not header | `csv`, 3 cols |
| `empty.txt` | Empty input | error: `no input text` |
