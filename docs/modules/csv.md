# `csv` - RFC 4180 comma-separated values

Import with `import "csv.j" as csv;`. Parses CSV text into rows of string
fields and formats rows back into text. Pure Jennifer over `strings` and
`maps`, so it runs on either binary. The delimiter is configurable, so the same
code reads and writes TSV and other single-character-separated formats.
Parsing is strict: malformed quoting and records of different widths are
errors, never silently repaired. Records are split natively
(`strings.split`); only irregular ones (a quote mid-field, a bare `CR`) go
through a character scanner, which accepts the same input or names the fault.

```jennifer
use io;
import "csv.j" as csv;

def rows as list of list of string init csv.parse("name,note\n\"Smith, J\",hi");
io.printf("%s | %s\n", $rows[1][0], $rows[1][1]);        # Smith, J | hi

def recs as list of map of string to string init csv.toRecords($rows);
io.printf("%s\n", $recs[0]["note"]);                      # hi
```

Runnable: [`examples/modules/csv_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/csv_demo.j).

## Surface

| Call                              | Returns                          | Notes                                                                        |
| --------------------------------- | -------------------------------- | ---------------------------------------------------------------------------- |
| `csv.parse(s)`                    | `list of list of string`         | Parse standard comma-delimited CSV into rows of fields.                      |
| `csv.parseWith(s, delim)`         | `list of list of string`         | Same, with a single-character delimiter (`"\t"` for TSV).                    |
| `csv.format(rows)`                | `string`                         | Encode rows as comma-delimited CSV; quotes fields that need it. **Unsafe for spreadsheets** - see below. |
| `csv.formatWith(rows, delim)`     | `string`                         | Same, with a single-character delimiter.                                     |
| `csv.formatSafe(rows)`            | `string`                         | Like `format`, but neutralises spreadsheet formula injection. The export-to-spreadsheet path. |
| `csv.formatSafeWith(rows, delim)` | `string`                         | Like `formatWith`, injection-neutralised.                                    |
| `csv.dialect(delim)`              | `csv.Dialect`                    | A dialect with the given delimiter and defaults (quote `"`, no comment, no trim, width 0). |
| `csv.parseDialect(text, d)`       | `list of list of string`         | Parse under a `csv.Dialect` (custom quote, comment-line skip, trim, record width). |
| `csv.formatDialect(rows, d)`      | `string`                         | Format under a `csv.Dialect` (custom delimiter, quote and width).            |
| `csv.toRecords(rows)`             | `list of map of string to string`| Treat row 0 as a header; map each later row to a header-keyed record.        |
| `csv.fromRecords(header, records)`| `list of list of string`         | Inverse: a header row followed by one row per record, in `header` order.     |
| `csv.reader(file)` / `readerWith(file, delim)` | `csv.Reader`        | Wrap an open read-mode `fs.File` as a streaming reader.                      |
| `csv.readRow(reader)`             | `list of string`                 | Read the next record, as `parse` would; `[]` once none is left.              |
| `csv.readerEof(reader)`           | `bool`                           | True once no further record remains; loop `while (not readerEof($r))`.       |
| `csv.reader`/`writer` `closeReader`/`closeWriter` | `null`          | Close the underlying file handle.                                            |
| `csv.writer(file)` / `writerWith(file, delim)` | `csv.Writer`        | Wrap an open write/append-mode `fs.File` as a streaming writer.              |
| `csv.writeRow(writer, fields)`    | `null`                           | Append one quoted-as-needed record terminated by CRLF.                       |

## Parsing (RFC 4180)

`parse` and `parseWith` implement the [RFC 4180](https://www.rfc-editor.org/rfc/rfc4180)
rules:

- Fields are separated by the delimiter (a comma by default); records by
  `CRLF`, `LF` or a bare `CR` outside quotes (RFC 4180 names only `CRLF`; the
  other two are what real files use).
- A field wrapped in `"` may contain the delimiter, line breaks, and
  quotes; an embedded quote is written doubled (`""`) and decodes to one.
  Spaces are part of a field.
- An empty input yields no rows; a trailing record separator does **not**
  add an empty trailing row. A separator with nothing after it *within* a
  record is a real empty field (`a,` is two fields, the second empty), and a
  blank line is a record with one empty field, `[""]`.
- Every record must have as many fields as the first (RFC 4180 section 2.4);
  `Dialect.width` changes the rule.
- Any other use of the quote is an error with its position: a quote inside an
  unquoted field (`5" disk`), text after a closing quote (`"ab"c`), and a quote
  that is never closed.
- The delimiter and the quote must each be one character, not a line break,
  and differ from each other.

Every fault is an `Error{kind: "csv"}`:

```text
csv: a quote inside an unquoted field at line 2, column 4
csv: text after a closing quote at line 1, column 5
csv: the quoted field opened at line 2, column 3 is not closed
csv: record 2 has 1 fields, expected 2
```

Lines count from 1 and include those inside quoted fields; columns count
characters. Changed in 0.26.0: the parser used to accept all of these, opening
a quoted section at a stray quote and padding nothing.

```jennifer
# Embedded comma, doubled quote, and newline all survive.
def rows as list of list of string init csv.parse("\"Smith, J\",\"said \"\"hi\"\"\",\"two\nlines\"");
# rows[0] == ["Smith, J", "said \"hi\"", "two\nlines"]
```

## Formatting

`format` / `formatWith` are the inverse. A field is quoted only when it
carries the delimiter, a quote, or a line break; embedded quotes double.
Records are joined with `CRLF` (RFC 4180) and **no trailing line break**, so
`parse(format(rows))` returns the rows:

```jennifer
def rows as list of list of string init [];
$rows[] = ["plain", "has,comma", "q\"uote"];
io.printf("%s\n", csv.format($rows));
# plain,"has,comma","q""uote"
```

- A row that is one empty field is written `""`: unquoted it would be an
  empty line, which at the end of the text reads back as no record.
- A row with no fields cannot be written (a CSV record has at least one) and
  is an error, as are rows of different widths.
- Only the **record separators** normalise: an `LF`- or `CR`-terminated input
  re-emits with `CRLF` between records. Line breaks *inside* a quoted field
  are field content and pass through verbatim.

Changed in 0.26.0: records were joined with `LF`, a lone empty field was
written as an empty line, and empty or ragged rows were written as they
came.

## Spreadsheet formula injection (use `formatSafe`)

**`format` / `formatWith` are UNSAFE for a CSV that a spreadsheet will
open.** RFC 4180 quoting does not stop CSV formula injection
([CWE-1236](https://cwe.mitre.org/data/definitions/1236.html)): a field
whose first character is `=`, `+`, `-`, `@`, a tab (`0x09`), or a CR
(`0x0D`) is interpreted by Excel and Google Sheets as a **formula** on
import, so a value like `=SUM(...)`, or a payload like
`=cmd|'/c calc'!A1`, executes in the victim's spreadsheet.

`formatSafe` / `formatSafeWith` behave like `format` / `formatWith` but,
before quoting, prefix any such field with a single apostrophe (`'`), which
every major spreadsheet treats as "this cell is literal text":

```jennifer
def rows as list of list of string init [];
$rows[] = ["=SUM(A1:A9)", "normal"];
io.printf("%s\n", csv.formatSafe($rows));
# '=SUM(A1:A9),normal      <- the formula is now inert text on import
```

Only a field's **first** character is inspected (so `a=b` is untouched),
and a normal field is left exactly as `format` would write it. Use plain
`format` only when the target is another program that parses CSV as data,
never a spreadsheet.

## Dialects

`csv.Dialect` groups the non-standard knobs so a caller can read a
`;`-delimited, `#`-commented, whitespace-padded file in one call:

```jennifer
def struct csv.Dialect {
    delimiter as string,   # the field delimiter
    quote as string,       # the quote character (default ")
    comment as string,     # a comment-line prefix, or "" for none
    trim as bool,          # trim whitespace around fields on parse
    width as int           # fields per record: 0 = as the first, n, or -1 = any
};
```

`csv.dialect(delim)` builds one with the defaults (quote `"`, no comment, no
trim, width 0); set the other fields to customise. `parseDialect` reads under
it, `formatDialect` writes under it so that `parseDialect` with the same
dialect returns the rows:

```jennifer
def d as csv.Dialect init csv.dialect(";");
$d.comment = "#";
$d.trim = true;
def rows as list of list of string init csv.parseDialect("# note\n a ; b ;c", $d);
# rows == [["a", "b", "c"]]
```

- **`comment`** skips a physical line only when the line's **record** starts
  with the prefix (outside any quoted field), so a `#` inside data is safe.
- **`trim`** trims **unquoted** fields, and allows spaces and tabs before an
  opening and after a closing quote (`a, "b"`); a quoted field keeps its
  content verbatim (`"  keep  "` stays `  keep  `). Without `trim`, a space
  before a quote is a quote inside an unquoted field.
- **`width`** is the number of fields every record must have: `0` (default)
  as many as the first record, a count, or `-1` for records of any width.
  `formatDialect` checks rows the same way.
- On write, `formatDialect` quotes a field with surrounding whitespace when
  `trim` is set, and a record's first field starting with the `comment`
  prefix, so neither changes on the way back.
- `parse` / `parseWith` / `format` / `formatWith` use `dialect(delim)`.

## Streaming a large file

For a table too large to hold in memory, `csv.reader` / `csv.writer` wrap an
open `fs.File` handle and process one record at a time. The file and the
reader's buffer are handles shared by every copy of the `Reader`, so
successive `readRow` calls advance the same stream. `readRow` returns what
`parse` would: a quoted field may span lines, `CRLF`, `LF` and a bare `CR`
end a record, line breaks inside quotes are kept verbatim, and the same
quoting and width rules apply. It reads the file in 4096-character chunks:

```jennifer
use fs;
import "csv.j" as csv;

def rf as fs.File init fs.open("big.csv", "read");
def r as csv.Reader init csv.reader($rf);
while (not csv.readerEof($r)) {
    def row as list of string init csv.readRow($r);
    # process one record without loading the whole file
}
csv.closeReader($r);

def wf as fs.File init fs.open("out.csv", "write");
def w as csv.Writer init csv.writer($wf);
csv.writeRow($w, ["name", "note"]);
csv.writeRow($w, ["Smith, J", "hi"]);   # embedded comma auto-quoted
csv.closeWriter($w);
```

- `readerEof` is the loop guard; `readRow` returns `[]` once no record is
  left (a blank line is `[""]`).
- A malformed record is consumed before its error is thrown, so a caller that
  catches the error can read on. A stray quote ends the record at its line
  break; it does not pull in the rest of the file.
- `writeRow` ends each record with `CRLF`, writes a lone empty field as `""`,
  and rejects a row with no fields or not as many as the first; nothing is
  written then.
- `readerWith` / `writerWith` take a custom delimiter for streaming TSV.
- The `state` field of `Reader` and `Writer` is internal: build them with
  `reader` / `readerWith` / `writer` / `writerWith`.

Changed in 0.26.0: `readRow` read with `fs.readLine`, so it returned only the
first record of a line holding bare-`CR` separators, turned `CRLF` inside a
quoted field into `LF`, and returned `[]` for a blank line.

## Header-keyed records

`toRecords` pairs the header row with the data rows, giving
one `map of string to string` per record keyed by column name; `fromRecords`
rebuilds rows from records and an explicit header:

```jennifer
def rows as list of list of string init csv.parse("name,age\nAda,36\nGrace,45");
def recs as list of map of string to string init csv.toRecords($rows);
# recs[0] == {"name": "Ada", "age": "36"}

def back as list of list of string init csv.fromRecords(["name", "age"], $recs);
# back == [["name","age"], ["Ada","36"], ["Grace","45"]]
```

- `toRecords` requires one field per header name in every row, and a header
  that names each column once; anything else is a `csv` error.
- `fromRecords` takes the header **explicitly** rather than reading it off
  the records, because map iteration order is insertion order per record
  and would not give a stable column order across records. Every record must
  have exactly the header's keys: a missing key and a key the header does not
  name are `csv` errors.
- `toRecords([])` is `[]`; a header-only input yields an empty record list.

Changed in 0.26.0: `toRecords` padded short rows with `""`, dropped fields
past the header and let a duplicate column overwrite the earlier one;
`fromRecords` wrote `""` for a missing key and ignored keys outside the
header.

## Out of scope

No type inference (numbers, booleans, dates): every field is a `string`,
and the caller converts with `convert.toInt` / `convert.toFloat`. For
whole-file work, read with `fs.readString` (or slurp stdin) and hand the
text to `parse`.

## See also

- [strings.md](../libraries/strings.md) - `split` / `join` / `replace`,
  which `csv` builds the scanner and encoder on.
- [maps.md](../libraries/maps.md) - `has` / `keys`, used by the record
  helpers.
- [fs.md](../libraries/fs.md) - `readString` to load a CSV file to hand to
  `parse`.
- [modules/index.md](index.md) - the module catalog and import rules.
