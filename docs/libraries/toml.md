# `toml` - TOML encode / decode

Enable with `use toml;`. RFC-conformant [TOML 1.0.0](https://toml.io)
decode / encode onto the same opaque, library-owned value that
[`json`](json.md) uses - the **same read / walk / write surface, name for
name**, so a program that reads, walks, and builds JSON does the same with
TOML. The one thing TOML has that JSON does not, its four date-time forms,
is surfaced through `toml.asDatetime` (backed by [`time`](time.md)).

`toml.decode(text)` returns an opaque `toml.Value` (a `KindObject`, the
sibling of `json.Value`); operators, `[index]`, and `.field` all reject it,
so the accessors below are the only way inside. `convert.typeOf` reports
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
`""`) to address the node itself. Every write verb is **non-mutating** -
it returns a fresh handle, so the idiom is `$v = toml.set($v, ...)`, the
same shape lists and maps use.

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
io.printf("%s\n", toml.asString($doc, "/title"));         # Jennifer
io.printf("%d\n", toml.asInt($doc, "/server/ports/0"));    # 8000
io.printf("%s\n", toml.asString($doc, "/fruit/1/name"));   # banana
```

The full TOML 1.0 value grammar decodes: basic / literal / multiline
strings, integers (decimal, `0x` hex, `0o` octal, `0b` binary, `_` digit
separators), floats (including `inf` / `nan`), booleans, `[table]`,
`[[array of tables]]`, dotted keys (`a.b.c = 1`), and inline tables
(`{ x = 1, y = 2 }`). Tables become maps in document order; arrays become
lists.

### JSON Pointer (RFC 6901)

TOML has no document-pointer syntax of its own, and a dotted path would be
ambiguous the moment a key itself contains a `.` (the quoted key
`"a.b"`). So addressing reuses `json`'s **JSON Pointer** - identical
`/`-separated syntax, so a program that walks JSON walks TOML unchanged:

```jennifer
toml.get($doc, "/server/ports/0")   # first port
toml.has($doc, "/server/tls")       # false if absent
```

A pointer is `""` (the whole document) or a `/`-led sequence of tokens;
`~1` escapes a literal `/` inside a key and `~0` a literal `~` (apply `~1`
first). List tokens are `0` or `[1-9][0-9]*`.

### Date-times

The date-time forms are the one place TOML is richer than JSON.
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

Jennifer ships **one** structured config format, and it is TOML. INI - the
`[section]` / `key=value` shape people reach for first - is deliberately
**not** supported, for three concrete reasons:

- **No real standard.** "INI" is a family of mutually-incompatible dialects,
  not a spec. Parsers disagree on comment characters (`;` vs `#`), quoting,
  escapes, whether `[a.b]` nests, duplicate keys, and case sensitivity. There
  is nothing to conform *to*, so "reads INI" would mean "reads our INI."
- **Flat.** INI has one level of `[section]`. It has no arrays of tables, no
  nested tables, no arrays at all in any agreed form - exactly the structure
  a real configuration needs.
- **Untyped.** Every INI value is a bare string. `port = 8080`, `debug =
  true`, and `ratio = 0.5` are all just text; the program re-parses each by
  hand and guesses the type. There is no `int` / `float` / `bool` / date-time
  distinction.

TOML fixes all three - a real (versioned) standard, nested tables and arrays
of tables, and typed scalars including native date-times - which is why it,
not INI, is the format Jennifer decodes into typed values. INI stays out on
purpose (documented here rather than silently missing); a tiny `.ini` cousin
is a candidate only if concrete demand appears.

## See also

- [`json`](json.md) - the sibling library `toml` mirrors name for name.
- [`time`](time.md) - the `time.Time` `toml.asDatetime` returns.
- [milestones.md](../milestones.md) - the `toml` system-library design and
  the `httpd` config dependency it was sequenced for.
