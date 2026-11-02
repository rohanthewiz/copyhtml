# Sample inputs

Run any of them with `copyhtml -stdout testdata/<file>`. "Detected as" is what
auto-detection picks today.

| File | Exercises | Detected as |
|---|---|---|
| `csv_basic.csv` | Plain CSV | CSV, 4 cols |
| `csv_quoted.csv` | Quoted commas, `""` escapes, multi-line cell (→ `<br>`), HTML that must be escaped | CSV, 4 cols |
| `csv_excel_bom_crlf.csv` | UTF-8 BOM + CRLF line endings, as Excel exports them | CSV, 5 cols |
| `semicolon.csv` | European `;` separator, non-ASCII headers | semicolon, 4 cols |
| `tsv_spreadsheet.tsv` | Excel/Sheets clipboard shape, one empty cell | TSV, 4 cols |
| `unicode_wide.tsv` | CJK, accents, emoji | TSV, 3 cols |
| `numbers_align.csv` | Numeric/percent columns (right-align inference), empty cells | CSV, 5 cols |
| `markdown.md` | GFM table: `:--`/`:-:`/`--:` alignment, `**bold**`, `` `code` ``, `<`/`&` | pipe, 4 cols |
| `psql.txt` | psql output with `(3 rows)` footer | pipe, 4 cols |
| `mysql.txt` | MySQL `+---+` grid with footer | pipe, 4 cols |
| `box_unicode.txt` | Unicode box drawing, a cell flush against `│` | pipe, 3 cols |
| `kubectl.txt` | Space-aligned, values with spaces ("2 days ago", "1 (3h ago)") | ws, 5 cols |
| `docker_ps.txt` | Space-aligned, multi-word header ("CONTAINER ID"), an empty PORTS cell | ws, 7 cols |
| `uniq_c.txt` | `uniq -c` output: leading spaces, no header line | ws, 2 cols |
| `ambiguous_commas_in_ws.txt` | Aligned columns whose values contain commas (must not become CSV) | ws, 3 cols |
| `ragged.csv` | Rows with differing field counts | **ws, 1 col** (needs `-f csv` → 4 cols, padded) |
| `blank_lines.csv` | Blank lines between and after rows | CSV, 2 cols |
| `single_row.csv` | One row only, rendered as data, not header | CSV, 3 cols |
| `empty.txt` | Empty input | error: `no input text` |
