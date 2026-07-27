# `csv` - RFC 4180 comma-separated values

Import with `import "csv.j" as csv;`. Parses CSV text into rows of string
fields and formats rows back into text, with a quoting-aware hand-written
scanner. Pure Jennifer over `strings` and `maps`, so it runs on either
binary. The delimiter is configurable, so the same code reads and writes
TSV and other single-character-separated formats.

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
| `csv.dialect(delim)`              | `csv.Dialect`                    | A dialect with the given delimiter and defaults (quote `"`, no comment, no trim). |
| `csv.parseDialect(text, d)`       | `list of list of string`         | Parse under a `csv.Dialect` (custom quote, comment-line skip, unquoted-field trim). |
| `csv.formatDialect(rows, d)`      | `string`                         | Format under a `csv.Dialect` (custom delimiter and quote).                   |
| `csv.toRecords(rows)`             | `list of map of string to string`| Treat row 0 as a header; map each later row to a header-keyed record.        |
| `csv.fromRecords(header, records)`| `list of list of string`         | Inverse: a header row followed by one row per record, in `header` order.     |
| `csv.reader(file)` / `readerWith(file, delim)` | `csv.Reader`        | Wrap an open read-mode `fs.File` as a streaming reader.                      |
| `csv.readRow(reader)`             | `list of string`                 | Read one complete record (spans physical lines inside a quoted field); `[]` at EOF. |
| `csv.readerEof(reader)`           | `bool`                           | True once no further record remains; loop `while (not readerEof($r))`.       |
| `csv.reader`/`writer` `closeReader`/`closeWriter` | `null`          | Close the underlying file handle.                                            |
| `csv.writer(file)` / `writerWith(file, delim)` | `csv.Writer`        | Wrap an open write/append-mode `fs.File` as a streaming writer.              |
| `csv.writeRow(writer, fields)`    | `null`                           | Append one quoted-as-needed record terminated by LF.                         |

## Parsing (RFC 4180)

`parse` and `parseWith` implement the [RFC 4180](https://www.rfc-editor.org/rfc/rfc4180)
rules:

- Fields are separated by the delimiter (a comma by default); records by
  `LF` or `CRLF`. A bare `CR` outside quotes also ends a record.
- A field wrapped in `"` may contain the delimiter, line breaks, and
  quotes; an embedded quote is written doubled (`""`) and decodes to one.
- An empty input yields no rows; a trailing record separator does **not**
  add an empty trailing row. A separator with nothing after it *within* a
  record is a real empty field (`a,` is two fields, the second empty).

```jennifer
# Embedded comma, doubled quote, and newline all survive.
def rows as list of list of string init csv.parse("\"Smith, J\",\"said \"\"hi\"\"\",\"two\nlines\"");
# rows[0] == ["Smith, J", "said \"hi\"", "two\nlines"]
```

## Formatting

`format` / `formatWith` are the inverse. A field is quoted only when it
carries the delimiter, a quote, or a line break; embedded quotes double.
Records are joined with `LF` and **no trailing newline**, so
`parse(format(rows))` round-trips the data:

```jennifer
def rows as list of list of string init [];
$rows[] = ["plain", "has,comma", "q\"uote"];
io.printf("%s\n", csv.format($rows));
# plain,"has,comma","q""uote"
```

Only the **record separators** normalise: a `CRLF`- or `CR`-terminated
input re-emits with `LF` between records. Line breaks *inside* a quoted
field are field content and pass through verbatim, so no data is altered.

## Spreadsheet formula injection (use `formatSafe`)

**`format` / `formatWith` are UNSAFE for a CSV that a spreadsheet will
open.** RFC 4180 quoting does not stop CSV formula injection
([CWE-1236](https://cwe.mitre.org/data/definitions/1236.html)): a field
whose first character is `=`, `+`, `-`, `@`, a tab (`0x09`), or a CR
(`0x0D`) is interpreted by Excel and Google Sheets as a **formula** on
import - so a value like `=SUM(...)`, or a payload like
`=cmd|'/c calc'!A1`, executes in the victim's spreadsheet. A field is
still just text in the CSV; the danger is the consuming spreadsheet.

`formatSafe` / `formatSafeWith` are the export-to-spreadsheet path: they
behave exactly like `format` / `formatWith` but, before quoting, prefix
any such field with a single apostrophe (`'`), the standard neutraliser
that every major spreadsheet treats as "this cell is literal text":

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
    trim as bool           # trim whitespace from *unquoted* fields on parse
};
```

`csv.dialect(delim)` builds one with the standard defaults (quote `"`, no
comment, no trim); set the other fields to customise. `parseDialect` reads
under it, `formatDialect` writes under it (the `comment` / `trim` fields are
parse-only):

```jennifer
def d as csv.Dialect init csv.dialect(";");
$d.comment = "#";
$d.trim = true;
def rows as list of list of string init csv.parseDialect("# note\n a ; b ;c", $d);
# rows == [["a", "b", "c"]]
```

- **`comment`** skips a physical line only when the line's **record** starts
  with the prefix (outside any quoted field), so a `#` inside data is safe.
- **`trim`** trims **unquoted** fields only; a quoted field keeps its
  whitespace verbatim (`"  keep  "` stays `  keep  `).
- The plain `parse` / `parseWith` / `format` / `formatWith` are unchanged -
  a dialect is opt-in.

## Streaming a large file

For a table too large to hold in memory, `csv.reader` / `csv.writer` wrap an
open `fs.File` handle and process one record at a time. The wrapped file
shares its read position across value copies (like every `fs` handle), so
successive `readRow` calls advance the same stream. `readRow` reads a
**complete** record even when a quoted field spans several physical lines:

```jennifer
use fs;
import "csv.j" as csv;

def rf as fs.File init fs.open("big.csv", "read");
def r as csv.Reader init csv.reader($rf);
while (not csv.readerEof($r)) {
    def row as list of string init csv.readRow($r);
    if (len($row) > 0) {
        # process one record without loading the whole file
    }
}
csv.closeReader($r);

def wf as fs.File init fs.open("out.csv", "write");
def w as csv.Writer init csv.writer($wf);
csv.writeRow($w, ["name", "note"]);
csv.writeRow($w, ["Smith, J", "hi"]);   # embedded comma auto-quoted
csv.closeWriter($w);
```

`readRow` returns `[]` at end-of-file (and for a blank line), so
`readerEof` is the loop guard. `readerWith` / `writerWith` take a custom
delimiter for streaming TSV. The streaming reader decodes an embedded
newline (a quoted field split across lines) back to `\n`; a `\r` inside a
quoted field is normalised to `\n` on the streaming path (the whole-file
`parse` preserves it).

## Header-keyed records

Most CSV has a header row. `toRecords` pairs it with the data rows, giving
one `map of string to string` per record keyed by column name; `fromRecords`
rebuilds rows from records and an explicit header:

```jennifer
def rows as list of list of string init csv.parse("name,age\nAda,36\nGrace,45");
def recs as list of map of string to string init csv.toRecords($rows);
# recs[0] == {"name": "Ada", "age": "36"}

def back as list of list of string init csv.fromRecords(["name", "age"], $recs);
# back == [["name","age"], ["Ada","36"], ["Grace","45"]]
```

Details worth knowing:

- Every record carries **every** header key. A data row shorter than the
  header fills the missing fields with `""`; fields past the header width
  are dropped (they have no name).
- Duplicate header names collapse - a later column overwrites an earlier
  one of the same name (map keys are unique).
- `fromRecords` takes the header **explicitly** rather than reading it off
  the records, because map iteration order is insertion order per record
  and would not give a stable column order across records. A key absent
  from a record writes `""`.
- `toRecords([])` is `[]`; a header-only input yields an empty record list.

## Out of scope

Type inference (numbers, booleans, dates) is not part of this module -
every field is a `string`, and the caller converts what it needs with
`convert.toInt` / `convert.toFloat`. For whole-file work, read with
`fs.readString` (or slurp stdin) and hand the text to `parse`; for a file
too large to hold in memory, use the streaming `reader` / `writer` above.

## See also

- [strings.md](../libraries/strings.md) - `split` / `join` / `replace`,
  which `csv` builds the scanner and encoder on.
- [maps.md](../libraries/maps.md) - `has` / `keys`, used by the record
  helpers.
- [fs.md](../libraries/fs.md) - `readString` to load a CSV file to hand to
  `parse`.
- [modules/index.md](index.md) - the module catalog and import rules.
