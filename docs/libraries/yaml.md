# `yaml` - YAML decode / encode

Enable with `use yaml;`. [YAML 1.2](https://yaml.org) decode / encode onto
the same opaque, library-owned value that [`json`](json.md) and
[`toml`](toml.md) use, with the **same read / walk / write surface, name for
name**. YAML's timestamp scalar, which JSON lacks, is surfaced through
`yaml.asDatetime` (backed by [`time`](time.md)).

`yaml.decode(text)` returns an opaque `yaml.Value` (a `KindObject`, the sibling
of `json.Value` / `toml.Value`); `[index]` and `.field` reject it (and `==` /
`!=` compare two handles by value), so use the accessors below.
`convert.typeOf` reports
`"object"`; `convert.objectType` reports `"yaml.Value"`.

Unlike the hand-rolled `json` / `toml` / `xml`, `yaml` is backed by a Go
dependency ([`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3)): full
YAML (anchors and aliases, flow and block styles, implicit typing,
multi-document streams) is impractical to hand-roll and has no Go standard
library. The dependency is verified TinyGo-clean, so `yaml` builds on **both
binaries**.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `yaml.decode(text)` | `yaml.Value` | Parse a single-document YAML string into an opaque handle. A multi-document stream errors (use `decodeAll`); an empty string decodes to `null`. |
| `yaml.decodeAll(text)` | `list of yaml.Value` | Parse every document of a `---`-separated stream. |
| `yaml.encode(v)` | `string` | Render to **compact flow style** (`{a: 1, b: [x, y]}`). |
| `yaml.encodePretty(v)` | `string` | Render to **readable block style** (indented, one entry per line). |
| `yaml.typeOf(v[, ptr])` | `string` | Node type: `null` / `bool` / `int` / `float` / `string` / `bytes` / `list` / `map` / `datetime`, or `missing` when `ptr` does not resolve (where `has` is false). |
| `yaml.get(v[, ptr])` | `yaml.Value` | The addressed sub-node, re-wrapped so a walk stays opaque. |
| `yaml.has(v, ptr)` | `bool` | Whether the pointer resolves. |
| `yaml.keys(v[, ptr])` | `list of string` | Keys of the addressed mapping, in document order. |
| `yaml.length(v[, ptr])` | `int` | Element count of a list, entry count of a map. |
| `yaml.asInt(v[, ptr])` | `int` | Strict: a float node errors. |
| `yaml.asFloat(v[, ptr])` | `float` | An integer node promotes to float. |
| `yaml.asString(v[, ptr])` | `string` | |
| `yaml.asBool(v[, ptr])` | `bool` | |
| `yaml.isNull(v[, ptr])` | `bool` | Whether the addressed node is `null`. |
| `yaml.asDatetime(v[, ptr])` | `time.Time` | A YAML timestamp scalar as a `time.Time` (needs `use time;`). |
| `yaml.isDatetime(v[, ptr])` | `bool` | Whether the addressed node is a timestamp. |
| `yaml.map()` | `yaml.Value` | A fresh empty mapping, to build a document from scratch. |
| `yaml.list()` | `yaml.Value` | A fresh empty sequence. |
| `yaml.map(m)` / `yaml.list(xs)` | `yaml.Value` | A mapping / sequence built from a Jennifer map / list in one step (see json's "Building a large list or map"). |
| `yaml.set(v, ptr, val)` | `yaml.Value` | Upsert a map key / replace an in-range list index. |
| `yaml.insert(v, ptr, val)` | `yaml.Value` | Insert into a list at an index or `-` (end). |
| `yaml.append(v, ptr, val)` | `yaml.Value` | Push onto the list at `ptr`. |
| `yaml.remove(v, ptr)` | `yaml.Value` | Remove the addressed key / element. |
| `yaml.move(v, from, to)` | `yaml.Value` | Relocate a subtree. |

The `[, ptr]` argument is a **JSON Pointer** (see below); omit it (or pass
`""`) to address the node itself. Every write verb is **non-mutating** and
returns a fresh handle: `$v = yaml.set($v, ...)`, as with lists and maps.

## Decoding

```jennifer
use io;
use yaml;

def src as string init "title: Jennifer\n"
    + "server:\n"
    + "  host: localhost\n"
    + "  ports: [8000, 8001]\n";

def doc as yaml.Value init yaml.decode($src);
io.printf("%s\n", yaml.asString($doc, "/title"));           # Jennifer
io.printf("%d\n", yaml.asInt($doc, "/server/ports/0"));     # 8000
io.printf("%s\n", yaml.asString($doc, "/server/host"));     # localhost
```

Scalars are typed by YAML's implicit resolution: `42` is an `int` (decimal,
`0x` hex, and `0o` octal all resolve), `3.14` a `float` (`.inf` / `.nan`
included, though `yaml.asFloat` refuses those: Jennifer has no non-finite
float), `true` / `false` a `bool`, `~` / `null` a `null`, a timestamp a
`datetime`, and everything else a `string`. A quoted scalar keeps its string
type (`"42"` stays a string), and `yaml.encode` re-quotes a string that reads
like another type. An integer too large for a 64-bit `int` is kept as its
exact-digit `string`, not a lossy `float`. Mappings become maps in document order; sequences become lists;
a `!!binary` scalar decodes to `bytes`. A non-scalar mapping key (a `? [a, b]`
complex key) and a **duplicate mapping key** are both rejected.

`yaml.decode` guards against resource-exhaustion input with normal (catchable)
decode errors: a raw-text pre-scan rejects structural nesting deeper than 128
levels **before** the parse runs (the underlying parser is recursive, so a
deeply nested document would otherwise overflow the stack, fatally on the
fixed-stack `jennifer-tiny`); an input-byte cap (4 MiB on `jennifer`,
512 KiB on `jennifer-tiny`) rejects an oversized document before the backing
parser materialises its whole node tree; and a node budget (shared with the
other decoders) stops an alias bomb (anchors that each reference the previous
one twice) from exhausting memory.

The decoded tree is the same compact, immutable one `json` and `toml` use, so
binding, passing or `yaml.get`-ing a `yaml.Value` shares the tree instead of
copying it. While decoding, the backing parser's own node tree also takes
memory, briefly, for the whole document.

### Anchors, aliases, and merge keys

An anchor (`&name`) and its aliases (`*name`) are resolved **by value** during
decode: each alias expands to an independent copy. A `<<` merge key pulls another mapping's entries in; an
explicit key always wins over a merged one, and an earlier merge source wins
over a later one:

```jennifer
def src as string init "defaults: &def\n"
    + "  retries: 3\n"
    + "  timeout: 30\n"
    + "service:\n"
    + "  <<: *def\n"
    + "  timeout: 60\n";
def doc init yaml.decode($src);
yaml.asInt($doc, "/service/retries")   # 3  (merged in)
yaml.asInt($doc, "/service/timeout")   # 60 (own key wins)
```

### Multi-document streams

`yaml.decode` handles a single document and errors on a `---`-separated
stream, pointing at `yaml.decodeAll`, which returns one `yaml.Value` per
document:

```jennifer
def docs as list of yaml.Value init yaml.decodeAll($stream);
for (def d in $docs) { io.printf("%v\n", yaml.keys($d)); }
```

### JSON Pointer (RFC 6901)

YAML has no pointer syntax of its own, so addressing reuses `json`'s **JSON
Pointer**; code that walks JSON walks YAML unchanged:

```jennifer
yaml.get($doc, "/server/ports/0")   # first port
yaml.has($doc, "/server/tls")       # false if absent
```

A pointer is `""` (the whole document) or a `/`-led sequence of tokens; `~1`
escapes a literal `/` inside a key and `~0` a literal `~` (apply `~1` first).
List tokens are `0` or `[1-9][0-9]*`.

### Timestamps

`yaml.typeOf` reports `datetime` for a timestamp scalar; `yaml.asDatetime`
returns a `time.Time`:

```jennifer
use time;
def doc init yaml.decode("created: 2001-12-15T02:59:43Z");
def t as time.Time init yaml.asDatetime($doc, "/created");
io.printf("%s\n", time.iso($t));    # 2001-12-15T02:59:43Z
```

## Encoding

`yaml.encode` renders **flow style** (compact, `{a: 1, b: [x, y]}`);
`yaml.encodePretty` renders **block style** (the readable, indented form).
(YAML's analogue of `json`'s compact / pretty pair.) A native
`map` / `list` / struct / scalar encodes directly, so a `yaml.Value` handle is
not required; a `bytes` value encodes as a `!!binary` scalar and a `time.Time`
as a timestamp.

```jennifer
def doc init yaml.decode($src);
io.printf("%s", yaml.encodePretty($doc));
```

Block style preserves a timestamp's implicit type on round-trip; flow style
quotes a timestamp (so it round-trips as a string) - prefer `encodePretty` when
a document carries timestamps.

## Building and editing

Start from `yaml.map()` / `yaml.list()` and grow a level at a time. Writes are
**strict** (no auto-vivification): `set` creates only the final pointer
segment, so build intermediate maps explicitly.

```jennifer
def cfg as yaml.Value init yaml.map();
$cfg = yaml.set($cfg, "/name", "demo");
$cfg = yaml.set($cfg, "/ports", yaml.list());
$cfg = yaml.append($cfg, "/ports", 8000);
$cfg = yaml.append($cfg, "/ports", 8001);
io.printf("%s", yaml.encodePretty($cfg));
# name: demo
# ports:
#     - 8000
#     - 8001
```

## See also

- [`json`](json.md) / [`toml`](toml.md) - the sibling config libraries `yaml`
  mirrors name for name.
- [`time`](time.md) - the `time.Time` `yaml.asDatetime` returns.
- [milestones.md](../milestones.md) - the `yaml` system-library design and the
  one place a config parser earns a Go dependency.
