# `toml` - TOML encode / decode

Enable with `use toml;`. [TOML 1.0.0](https://toml.io)
decode / encode (passes the toml-test 1.0.0 suite) onto the same opaque, library-owned value that
[`json`](json.md) uses, with the **same read / walk / write surface, name for
name**. TOML's four date-time forms, which JSON lacks, are surfaced through
`toml.asDatetime` (the offset form, backed by [`time`](time.md)) and
`toml.asLocal` (the three local forms).

`toml.decode(text)` returns an opaque `toml.Value` (a `KindObject`, the
sibling of `json.Value`); `[index]` and `.field` reject it (and `==` / `!=`
compare two handles by value), so use the accessors below.
`convert.typeOf` reports `"object"`; `convert.objectType` reports
`"toml.Value"`.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `toml.decode(text)` | `toml.Value` | Parse a TOML document into an opaque handle. |
| `toml.decodeWith(text, o)` | `toml.Value` | Same, under the lower caps of a `toml.DecodeOptions{maxDepth, maxNodes}` (see [Decode caps](#decode-caps)). |
| `toml.encode(v)` | `string` | Render a `toml.Value` (or a native `map` / `list` / scalar) to TOML text. |
| `toml.encodePretty(v)` | `string` | Same, with a blank line separating `[table]` / `[[array]]` sections. |
| `toml.typeOf(v[, ptr])` | `string` | Node type: `null` / `bool` / `int` / `float` / `string` / `list` / `map` / `datetime` (an offset date-time) / `local-datetime` / `local-date` / `local-time`, or `missing` when `ptr` does not resolve (where `has` is false). |
| `toml.get(v[, ptr])` | `toml.Value` | The addressed sub-node, re-wrapped so a walk stays opaque. |
| `toml.has(v, ptr)` | `bool` | Whether the pointer resolves. |
| `toml.keys(v[, ptr])` | `list of string` | Keys of the addressed table, in document order. |
| `toml.length(v[, ptr])` | `int` | Element count of a list, entry count of a table. |
| `toml.asInt(v[, ptr])` | `int` | Strict: a float node errors. |
| `toml.asFloat(v[, ptr])` | `float` | An integer node promotes to float when the float holds it exactly, else kind `"toml"`. |
| `toml.asDecimal(v[, ptr])` | `decimal.Decimal` | A number node as a decimal ([json](json.md) has the details). |
| `toml.asString(v[, ptr])` | `string` | |
| `toml.asFloats(v[, ptr])` / `asInts` / `asStrings` | `list of float` / `int` / `string` | A whole list in one call, each element read as `asFloat` / `asInt` / `asString` reads it ([json](json.md) has the details). |
| `toml.asBool(v[, ptr])` | `bool` | |
| `toml.asDatetime(v[, ptr])` | `time.Time` | An offset date-time as a `time.Time` (needs `use time;`). A local form is a `"runtime"` error naming `asLocal`. |
| `toml.isDatetime(v[, ptr])` | `bool` | Whether the addressed node is an offset date-time (what `asDatetime` reads). |
| `toml.asLocal(v[, ptr])` | `toml.Local` | A local date-time, date or time as `toml.Local{year, month, day, hour, minute, second, nanosecond}`; the parts the form lacks are 0. Another node is a `"runtime"` error. |
| `toml.map()` | `toml.Value` | A fresh empty table, to build a document from scratch. |
| `toml.list()` | `toml.Value` | A fresh empty array. |
| `toml.map(m)` / `toml.list(xs)` | `toml.Value` | A table / array built from a Jennifer map / list in one step (see json's "Building a large list or map"). |
| `toml.set(v, ptr, val)` | `toml.Value` | Upsert a table key / replace an in-range list index. |
| `toml.insert(v, ptr, val)` | `toml.Value` | Insert into a list at an index or `-` (end). |
| `toml.append(v, ptr, val)` | `toml.Value` | Push onto the list at `ptr`. |
| `toml.remove(v, ptr)` | `toml.Value` | Remove the addressed key / element. |
| `toml.move(v, from, to)` | `toml.Value` | Relocate a subtree. |

The `[, ptr]` argument is a **JSON Pointer** (see below); omit it (or pass
`""`) to address the node itself. Every write verb is **non-mutating** and
returns a fresh handle: `$v = toml.set($v, ...)`, as with lists and maps.

### Decode caps

| Cap | Default (`jennifer` / `jennifer-tiny`) | `DecodeOptions` field |
|---|---|---|
| Nesting depth (containers, header and dotted-key segments) | 1000 / 64 | `maxDepth` |
| Nodes: values, and the tables and arrays of tables that headers and dotted keys create | 2,097,152 / 262,144 | `maxNodes` |

A reached cap is kind `"limit"`. `toml.decodeWith` lowers the caps per call,
as `json.decodeWith` does: 0 keeps the default; a value above it, or a
negative one, is a `"runtime"` error naming the allowed range.

```jennifer
def doc as toml.Value init toml.decodeWith($text, toml.DecodeOptions{maxDepth: 16, maxNodes: 10000});
```

### Errors

A decode error reads `toml.decode: <what> at line L, column C` (the column in
bytes, as in `json`); a document that ends where a key, value or closing
bracket is due reads `unexpected end of input`.

| Kind | Raised for |
|---|---|
| `"toml"` | malformed text in `decode` / `decodeWith`: a syntax error, a duplicate key or table, an invalid escape, number or date-time, invalid UTF-8, an integer outside int64 (TOML 1.0 requires the error; quote it as a string); an integer `asFloat` cannot return exactly |
| `"limit"` | a reached cap (above) |
| `"runtime"` | misuse: a wrong argument, `DecodeOptions` out of range, an unencodable value (`null`, a non-table root), an accessor on a node of another type (`asDatetime` on a local form, `asLocal` on an offset date-time), a malformed pointer |

The decoded tree is the same compact, immutable one `json` uses: a document
takes from about its text size to roughly 15 times it in memory, and binding,
passing or `toml.get`-ing a `toml.Value` shares the tree instead of copying it.

## Decoding

```jennifer
use io;
use toml;

def src as string init "title = \"Jennifer\"
[server]
host = \"localhost\"
ports = [8000, 8001]

[[fruit]]
name = \"apple\"
[[fruit]]
name = \"banana\"
";

def doc as toml.Value init toml.decode($src);
io.printf("%s\n", toml.asString($doc, "/title"));          # Jennifer
io.printf("%d\n", toml.asInt($doc, "/server/ports/0"));    # 8000
io.printf("%s\n", toml.asString($doc, "/fruit/1/name"));   # banana
```

The full TOML 1.0 value grammar decodes: basic / literal / multiline
strings, integers (decimal, `0x` hex, `0o` octal, `0b` binary, `_` digit
separators), floats (including `inf` / `nan`, which `typeOf` reports as
`float` and `encode` keeps, but `toml.asFloat` refuses: Jennifer has no
non-finite float), booleans, `[table]`, `[[array of tables]]`, dotted keys (`a.b.c = 1`), and inline tables
(`{ x = 1, y = 2 }`). Tables become maps in document order; arrays become
lists.

### JSON Pointer (RFC 6901)

TOML has no pointer syntax of its own, and a dotted path would be ambiguous
for a key containing `.` (the quoted key `"a.b"`). Addressing therefore reuses
`json`'s **JSON Pointer**, so code that walks JSON walks TOML unchanged:

```jennifer
toml.get($doc, "/server/ports/0");   # first port
toml.has($doc, "/server/tls");       # false if absent
```

A pointer is `""` (the whole document) or a `/`-led sequence of tokens;
`~1` escapes a literal `/` inside a key and `~0` a literal `~` (apply `~1`
first); any other `~` is a malformed pointer. List tokens are `0` or `[1-9][0-9]*`.

### Date-times

| Form | Example | `typeOf` | Read with |
|---|---|---|---|
| Offset date-time | `1979-05-27T07:32:00Z`, `...-07:00` | `datetime` | `asDatetime` -> `time.Time` |
| Local date-time | `1979-05-27T07:32:00` | `local-datetime` | `asLocal` -> `toml.Local` |
| Local date | `1979-05-27` | `local-date` | `asLocal` (time parts 0) |
| Local time | `07:32:00.5` | `local-time` | `asLocal` (date parts 0) |

A local form names no instant (it has no offset), so `asDatetime` refuses it
instead of guessing UTC; the caller decides the zone.

```jennifer
use time;
def doc as toml.Value init toml.decode("created = 1979-05-27T07:32:00Z\nday = 1979-05-27");
def t as time.Time init toml.asDatetime($doc, "/created");
io.printf("%s\n", time.iso($t));    # 1979-05-27T07:32:00Z
def d as toml.Local init toml.asLocal($doc, "/day");
io.printf("%d %d %d\n", $d.year, $d.month, $d.day);    # 1979 5 27
```

A space separator (`1979-05-27 07:32:00Z`) is accepted and normalised to
`T`. Every form keeps its lexical form, so `encode` and a `set` of the node
write it back unchanged. An offset date-time outside 1677-09-21 ..
2262-04-11 (the `time.Time` range, e.g. `9999-12-31`) makes `asDatetime`
throw instead of returning a wrapped instant; `asLocal` reads any year.

## Encoding

`toml.encode` renders a document back to text; `toml.encodePretty` adds a
blank line before each section header. The document **root must be a
table** (TOML has no top-level array or scalar form), and TOML has **no
null type** - encoding a `null` value is an error. A `bytes` value encodes
as a base64 string, a `time.Time` as an offset date-time.

```jennifer
def doc as toml.Value init toml.decode($src);
io.printf("%s", toml.encode($doc));
# leaf keys first, then [server], then the [[fruit]] sections -
# so keys always attach to the right table
```

## Building and editing

Start from `toml.map()` / `toml.list()` and grow a level at a time. Writes
are **strict** (no auto-vivification): `set` creates only the final pointer
segment, so build intermediate tables explicitly.

```jennifer
def cfg as toml.Value init toml.map();
$cfg = toml.set($cfg, "/name", "demo");
$cfg = toml.set($cfg, "/server", toml.map());
$cfg = toml.set($cfg, "/server/host", "localhost");
$cfg = toml.set($cfg, "/ports", toml.list());
$cfg = toml.append($cfg, "/ports", 8000);
$cfg = toml.append($cfg, "/ports", 8001);
io.printf("%s", toml.encode($cfg));
# name = "demo"
# ports = [8000, 8001]
# [server]
# host = "localhost"
```

## Why TOML and not INI

Jennifer ships one structured config format, TOML. INI (`[section]` /
`key=value`) is **not** supported:

- **No standard.** "INI" is a family of incompatible dialects. Parsers
  disagree on comment characters (`;` vs `#`), quoting, escapes, whether
  `[a.b]` nests, duplicate keys, and case sensitivity, so "reads INI" would
  mean "reads our INI".
- **Flat.** One level of `[section]`; no nested tables, arrays of tables, or
  arrays in any agreed form.
- **Untyped.** Every value is a string (`port = 8080`, `debug = true`,
  `ratio = 0.5` are all text), so the program re-parses and guesses types.

TOML is a versioned standard with nested tables, arrays of tables, and typed
scalars including date-times.

## See also

- [`json`](json.md) - the sibling library `toml` mirrors name for name.
- [`time`](time.md) - the `time.Time` `toml.asDatetime` returns.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Infrastructure as code for MikroTik routers](https://academy.jennifer-lang.dev/talks/mplx/mikrotik-iac/) | desired router state read from TOML files |
| [YAML, TOML, JSON and XML](https://academy.jennifer-lang.dev/talks/mplx/data-formats/) | one config with dates and tables, decoded and written |
| [Your own small blog with Jennifer](https://academy.jennifer-lang.dev/talks/mplx/blog/) | front matter of Markdown posts decoded into typed fields |
