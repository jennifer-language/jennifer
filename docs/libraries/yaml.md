# `yaml` - YAML decode / encode

Enable with `use yaml;`. [YAML 1.2](https://yaml.org) decode / encode onto
the same opaque, library-owned value that [`json`](json.md) and
[`toml`](toml.md) use - the **same read / walk / write surface, name for
name**, so a program that reads, walks, and builds JSON or TOML does the same
with YAML. Like TOML, YAML has a timestamp scalar JSON lacks, surfaced through
`yaml.asDatetime` (backed by [`time`](time.md)).

`yaml.decode(text)` returns an opaque `yaml.Value` (a `KindObject`, the sibling
of `json.Value` / `toml.Value`); operators, `[index]`, and `.field` all reject
it, so the accessors below are the only way inside. `convert.typeOf` reports
`"object"`; `convert.objectType` reports `"yaml.Value"`.

Unlike `json` / `toml` / `xml`, which are hand-rolled, `yaml` is backed by a Go
dependency ([`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3)): full
YAML - anchors and aliases, flow and block styles, implicit typing, and
multi-document streams - is impractical to hand-roll and has no Go standard
library. The dependency is verified TinyGo-clean, so `yaml` builds on **both
binaries** (`jennifer` and `jennifer-tiny`).

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `yaml.decode(text)` | `yaml.Value` | Parse a single-document YAML string into an opaque handle. A multi-document stream errors (use `decodeAll`); an empty string decodes to `null`. |
| `yaml.decodeAll(text)` | `list of yaml.Value` | Parse every document of a `---`-separated stream. |
| `yaml.encode(v)` | `string` | Render to **compact flow style** (`{a: 1, b: [x, y]}`). |
| `yaml.encodePretty(v)` | `string` | Render to **readable block style** (indented, one entry per line). |
| `yaml.typeOf(v[, ptr])` | `string` | Node type: `null` / `bool` / `int` / `float` / `string` / `bytes` / `list` / `map` / `datetime`. |
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
| `yaml.set(v, ptr, val)` | `yaml.Value` | Upsert a map key / replace an in-range list index. |
| `yaml.insert(v, ptr, val)` | `yaml.Value` | Insert into a list at an index or `-` (end). |
| `yaml.append(v, ptr, val)` | `yaml.Value` | Push onto the list at `ptr`. |
| `yaml.remove(v, ptr)` | `yaml.Value` | Remove the addressed key / element. |
| `yaml.move(v, from, to)` | `yaml.Value` | Relocate a subtree. |

The `[, ptr]` argument is a **JSON Pointer** (see below); omit it (or pass
`""`) to address the node itself. Every write verb is **non-mutating** - it
returns a fresh handle, so the idiom is `$v = yaml.set($v, ...)`, the same
shape lists and maps use.

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
included), `true` / `false` a `bool`, `~` / `null` a `null`, a timestamp a
`datetime`, and everything else a `string`. A quoted scalar keeps its string
type (`"42"` stays a string). Mappings become maps in document order; sequences
become lists; a `!!binary` scalar decodes to `bytes`. A non-scalar mapping key
(a `? [a, b]` complex key) is rejected.

`yaml.decode` guards against resource-exhaustion input with normal (catchable)
decode errors: a raw-text pre-scan rejects structural nesting deeper than 128
levels **before** the parse runs (the underlying parser is recursive, and a
deeply-nested document would otherwise overflow the interpreter's stack - fatal
on the fixed-stack `jennifer-tiny`), and a five-million-node budget stops an
alias bomb (anchors that each reference the previous one twice) from exhausting
memory.

### Anchors, aliases, and merge keys

An anchor (`&name`) and its aliases (`*name`) are resolved **by value** during
decode - each alias expands to an independent copy, so the resulting tree has
no shared state. A `<<` merge key pulls another mapping's entries in; an
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

YAML has no document-pointer syntax of its own, so addressing reuses `json`'s
**JSON Pointer** - identical `/`-separated syntax, so a program that walks JSON
walks YAML unchanged:

```jennifer
yaml.get($doc, "/server/ports/0")   # first port
yaml.has($doc, "/server/tls")       # false if absent
```

A pointer is `""` (the whole document) or a `/`-led sequence of tokens; `~1`
escapes a literal `/` inside a key and `~0` a literal `~` (apply `~1` first).
List tokens are `0` or `[1-9][0-9]*`.

### Timestamps

A YAML timestamp scalar is the one place YAML is richer than JSON.
`yaml.typeOf` reports `datetime`; `yaml.asDatetime` returns a `time.Time`:

```jennifer
use time;
def doc init yaml.decode("created: 2001-12-15T02:59:43Z");
def t as time.Time init yaml.asDatetime($doc, "/created");
io.printf("%s\n", time.iso($t));    # 2001-12-15T02:59:43Z
```

## Encoding

`yaml.encode` renders **flow style** (compact, `{a: 1, b: [x, y]}`);
`yaml.encodePretty` renders **block style** (the readable, indented form).
Flow / block is YAML's own analogue of `json`'s compact / pretty pair. A native
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
