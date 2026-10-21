# `toml` - TOML encode / decode

Enable with `use toml;`. RFC-conformant [TOML 1.0.0](https://toml.io)
decode / encode onto the same opaque, library-owned value that
[`json`](json.md) uses, with the **same read / walk / write surface, name for
name**. TOML's four date-time forms, which JSON lacks, are surfaced through
`toml.asDatetime` (backed by [`time`](time.md)).

`toml.decode(text)` returns an opaque `toml.Value` (a `KindObject`, the
sibling of `json.Value`); `[index]` and `.field` reject it (and `==` / `!=`
compare two handles by value), so use the accessors below.
`convert.typeOf` reports
`"object"`; `convert.objectType` reports `"toml.Value"`.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `toml.decode(text)` | `toml.Value` | Parse a TOML document into an opaque handle. |
| `toml.encode(v)` | `string` | Render a `toml.Value` (or a native `map` / `list` / scalar) to TOML text. |
| `toml.encodePretty(v)` | `string` | Same, with a blank line separating `[table]` / `[[array]]` sections. |
| `toml.typeOf(v[, ptr])` | `string` | Node type: `null` / `bool` / `int` / `float` / `string` / `list` / `map` / `datetime`. |
| `toml.get(v[, ptr])` | `toml.Value` | The addressed sub-node, re-wrapped so a walk stays opaque. |
| `toml.has(v, ptr)` | `bool` | Whether the pointer resolves. |
| `toml.keys(v[, ptr])` | `list of string` | Keys of the addressed table, in document order. |
| `toml.length(v[, ptr])` | `int` | Element count of a list, entry count of a table. |
| `toml.asInt(v[, ptr])` | `int` | Strict: a float node errors. |
| `toml.asFloat(v[, ptr])` | `float` | An integer node promotes to float. |
| `toml.asString(v[, ptr])` | `string` | |
| `toml.asBool(v[, ptr])` | `bool` | |
| `toml.asDatetime(v[, ptr])` | `time.Time` | Any of the four date-time forms as a `time.Time` (needs `use time;`). |
| `toml.isDatetime(v[, ptr])` | `bool` | Whether the addressed node is a date-time. |
| `toml.map()` | `toml.Value` | A fresh empty table, to build a document from scratch. |
| `toml.list()` | `toml.Value` | A fresh empty array. |
| `toml.set(v, ptr, val)` | `toml.Value` | Upsert a table key / replace an in-range list index. |
| `toml.insert(v, ptr, val)` | `toml.Value` | Insert into a list at an index or `-` (end). |
| `toml.append(v, ptr, val)` | `toml.Value` | Push onto the list at `ptr`. |
| `toml.remove(v, ptr)` | `toml.Value` | Remove the addressed key / element. |
| `toml.move(v, from, to)` | `toml.Value` | Relocate a subtree. |

The `[, ptr]` argument is a **JSON Pointer** (see below); omit it (or pass
`""`) to address the node itself. Every write verb is **non-mutating** and
returns a fresh handle: `$v = toml.set($v, ...)`, as with lists and maps.

`toml.decode` rejects container nesting (and dotted-key segment counts) past a
fixed depth with a normal (catchable) decode error, so hostile deeply-nested
input can't exhaust the interpreter's stack. The limit is shared by every parser
in the toolchain and is set per binary: 1000 levels on the default `jennifer`
(Go's growable stack), 64 on `jennifer-tiny` (its fixed 4 MB stack). `decode`
also caps the total number of value nodes it will materialise - about two
million on `jennifer`, a quarter million on `jennifer-tiny` - so a hostile flat
document (a huge array) raises a catchable error instead of growing without
bound.

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
non-finite float), booleans, `[table]`,
`[[array of tables]]`, dotted keys (`a.b.c = 1`), and inline tables
(`{ x = 1, y = 2 }`). Tables become maps in document order; arrays become
lists.

### JSON Pointer (RFC 6901)

TOML has no pointer syntax of its own, and a dotted path would be ambiguous
for a key containing `.` (the quoted key `"a.b"`). Addressing therefore reuses
`json`'s **JSON Pointer**, so code that walks JSON walks TOML unchanged:

```jennifer
toml.get($doc, "/server/ports/0")   # first port
toml.has($doc, "/server/tls")       # false if absent
```

A pointer is `""` (the whole document) or a `/`-led sequence of tokens;
`~1` escapes a literal `/` inside a key and `~0` a literal `~` (apply `~1`
first). List tokens are `0` or `[1-9][0-9]*`.

### Date-times

`toml.typeOf` reports `datetime`; `toml.asDatetime` returns a `time.Time`:

```jennifer
use time;
def doc init toml.decode("created = 1979-05-27T07:32:00Z");
def t as time.Time init toml.asDatetime($doc, "/created");
io.printf("%s\n", time.iso($t));    # 1979-05-27T07:32:00Z
```

All four forms parse - offset date-time (`...Z` / `...-07:00`), local
date-time (no offset), local date (`1979-05-27`), and local time
(`07:32:00`); a space separator (`1979-05-27 07:32:00Z`) is accepted and
normalised to `T`. A local date is taken at midnight UTC and a local time
on the zero date when converted to a `time.Time`; the original lexical form
is preserved for round-trip encoding.

## Encoding

`toml.encode` renders a document back to text; `toml.encodePretty` adds a
blank line before each section header. The document **root must be a
table** (TOML has no top-level array or scalar form), and TOML has **no
null type** - encoding a `null` value is an error. A `bytes` value encodes
as a base64 string, a `time.Time` as an offset date-time.

```jennifer
def doc init toml.decode($src);
io.printf("%s", toml.encode($doc));
# leaf keys first, then [server], then the [[fruit]] sections -
# so keys always attach to the right table
```

## Building and editing

Start from `toml.map()` / `toml.list()` and grow a level at a time. Writes
are **strict** (no auto-vivification): `set` creates only the final pointer
segment, so build intermediate tables explicitly.

```jennifer
use time;
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
scalars including date-times. A small `.ini` reader is a candidate only if
concrete demand appears.

## See also

- [`json`](json.md) - the sibling library `toml` mirrors name for name.
- [`time`](time.md) - the `time.Time` `toml.asDatetime` returns.
- [milestones.md](../milestones.md) - the `toml` system-library design and
  the `httpd` config dependency it was sequenced for.
