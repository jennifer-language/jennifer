# `jsonl` - JSON Lines (JSONL / NDJSON)

Import with `import "jsonl.j" as jsonl;`. Read and write **newline-delimited
JSON**: one independent JSON value per line. A thin framing layer over
[`json`](../libraries/json.md) - each record is a `json.Value`, so `encode` /
`decode` compose `json.encode` / `json.decode` with a `\n` split / join, and the
file helpers add `fs`. Pure Jennifer; runs on **both** binaries.

```jennifer
import "jsonl.j" as jsonl;
use json;

def rows as list of json.Value init [json.decode("{\"a\":1}"), json.decode("[2,3]")];
def text as string init jsonl.encode($rows);   # {"a":1}\n[2,3]\n
def back as list of json.Value init jsonl.decode($text);
```

Runnable: [`examples/modules/jsonl_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/jsonl_demo.j).

## In-memory

| Call | Returns | |
| ---- | ------- | - |
| `jsonl.encode(records)` | `string` | one compact JSON value per line, each newline-terminated |
| `jsonl.decode(text)` | `list of json.Value` | one record per non-blank line |

`records` is a `list of json.Value` - build them with `json.decode`,
`json.map()` / `json.set`, or by re-encoding a struct through `json`. Any
top-level JSON type is a valid line (object, array, number, string, `true` /
`false` / `null`). `decode` skips blank and whitespace-only lines and trims a
trailing `\r` (CRLF input), so `decode(encode(records))` round-trips. An empty
list encodes to `""`; `decode("")` is the empty list.

## Whole file

| Call | Returns | |
| ---- | ------- | - |
| `jsonl.readFile(path)` | `list of json.Value` | read and decode a whole JSONL file |
| `jsonl.writeFile(path, records)` | `null` | encode and write (replacing existing content) |
| `jsonl.appendFile(path, records)` | `null` | encode and append (file created if missing) |

`appendFile` is the common JSONL pattern - adding rows to a growing log or event
stream without rewriting the file.

## Streaming large files

For JSONL too large to hold in memory, a `Reader` yields one record at a time.
The wrapped `fs.File` is a handle - it shares its read position across value
copies, so successive `readRecord` calls advance the same stream.

| Call | Returns | |
| ---- | ------- | - |
| `jsonl.openReader(path)` | `Reader` | open a file for streaming |
| `jsonl.hasMore(reader)` | `bool` | whether the file has unread bytes (coarse; see below) |
| `jsonl.readRecord(reader)` | `Record` | the next `{value, done}` (skips blank lines) |
| `jsonl.closeReader(reader)` | `null` | close the reader |

`readRecord` returns a `Record` `{value, done}`: `done` is the reliable
end-of-stream signal. Loop until `done` rather than guarding with `hasMore` -
`hasMore` is a coarse `not eof` check that still reports `true` when only
trailing blank lines remain (which carry no record), so a `hasMore`-guarded
loop would over-run the last record.

```jennifer
def r as jsonl.Reader init jsonl.openReader("events.jsonl");
while (true) {
    def rec as jsonl.Record init jsonl.readRecord($r);
    if ($rec.done) {
        break;
    }
    # process $rec.value without loading the whole file
}
jsonl.closeReader($r);
```

### Streaming over a caller-owned file handle

When the caller already holds an open `fs.File` (or wants to stream both a
reader and a writer), the file-handle surface wraps an open handle instead of a
path. `reader` / `writer` take an `fs.File`; `writeValue` appends one compact
JSON value plus a newline; `readValue` returns the next record as a `json.Value`
directly.

| Call | Returns | |
| ---- | ------- | - |
| `jsonl.reader(file)` | `Reader` | wrap an open read-mode `fs.File` |
| `jsonl.readValue(reader)` | `json.Value` | the next record (skips blank lines); JSON `null` at end |
| `jsonl.writer(file)` | `Writer` | wrap an open write/append-mode `fs.File` |
| `jsonl.writeValue(writer, value)` | `null` | append one value terminated by a newline |
| `jsonl.closeWriter(writer)` | `null` | close the writer |

```jennifer
use fs;
import "jsonl.j" as jsonl;

def wf as fs.File init fs.open("events.jsonl", "append");
def w as jsonl.Writer init jsonl.writer($wf);
jsonl.writeValue($w, json.decode("{\"event\":\"login\"}"));
jsonl.closeWriter($w);

def rf as fs.File init fs.open("events.jsonl", "read");
def r as jsonl.Reader init jsonl.reader($rf);
while (not fs.eof($rf)) {
    def v as json.Value init jsonl.readValue($r);
    # process $v
}
jsonl.closeReader($r);
```

`readValue` returns a JSON `null` once the stream is exhausted. JSONL written by
`writeValue` has exactly one value per line and no trailing blanks, so `not
fs.eof(...)` is a reliable loop guard; when you need to tell end-of-stream from a
genuine `null` record, use `readRecord` and its explicit `done` flag instead.

## Scope

- **Records are `json.Value`.** JSONL is a framing convention, not a new
  encoder - the actual JSON work stays in the `json` library, and rebuilding a
  typed target from a decoded record is the same explicit step it is there (no
  map-to-struct coercion).
- **`\n`-separated.** Records are separated by line feed; a trailing `\r` is
  tolerated on read. `encode` writes `\n` and terminates the last line too.

## See also

- [json.md](../libraries/json.md) - the encoder / decoder and the `json.Value`
  accessors each record is built from.
- [fs.md](../libraries/fs.md) - the file surface the read / write / stream
  helpers build on.
- [modules/index.md](index.md) - the module catalog and import rules.
