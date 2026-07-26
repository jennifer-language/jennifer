# `json` - JSON encode / decode

Enable with `use json;`. RFC 8259 JSON, mapped onto Jennifer's value
model. Hand-rolled (no host `encoding/json`) so it works identically on
both binaries.

## Surface

| Call                     | Returns      | Notes                                                          |
| ------------------------ | ------------ | -------------------------------------------------------------- |
| `json.encode(v)`         | `string`     | Compact JSON for any encodable value (including a `json.Value`). |
| `json.encodePretty(v)`   | `string`     | Same, 2-space indented; empty lists / maps stay `[]`/`{}`.     |
| `json.decode(s)`         | `json.Value` | Parse JSON text into an opaque handle (see below).             |

`json.decode` rejects container nesting past a fixed depth with a normal
(catchable) decode error, so hostile deeply-nested input can't exhaust the
interpreter's stack. The limit is shared by every parser in the toolchain and
is set per binary: 1000 levels on the default `jennifer` (Go's growable stack),
64 on `jennifer-tiny` (its fixed 4 MB stack). Both are far past any real
document.

Accessors over a decoded `json.Value`. Every one takes an optional
trailing **JSON Pointer** (RFC 6901) string, relative to the passed node
(`""` or omitted = the node itself):

| Call                        | Returns          | Notes                                                       |
| --------------------------- | ---------------- | ----------------------------------------------------------- |
| `json.typeOf(v[, ptr])`     | `string`         | The node's type: `null` `bool` `int` `float` `string` `list` `map`. |
| `json.get(v[, ptr])`        | `json.Value`     | The addressed sub-node, as a `json.Value` (walk stays opaque). |
| `json.has(v, ptr)`          | `bool`           | Whether the pointer resolves to an existing node.           |
| `json.keys(v[, ptr])`       | `list of string` | Keys of the addressed map, in document order.               |
| `json.length(v[, ptr])`     | `int`            | Element count of a list, or entry count of a map.           |
| `json.asInt(v[, ptr])`      | `int`            | A number node with no fractional part. A `float` node errors. |
| `json.asFloat(v[, ptr])`    | `float`          | A number node; an integral one promotes (JSON has one number type). |
| `json.asString(v[, ptr])`   | `string`         | A string node.                                              |
| `json.asBool(v[, ptr])`     | `bool`           | A `true` / `false` node.                                    |
| `json.isNull(v[, ptr])`     | `bool`           | Whether the addressed node is JSON `null`.                  |

Write surface - every verb is **non-mutating**, returning a fresh
`json.Value`; the idiom is `$v = json.set($v, ...)`:

| Call                        | Returns          | Notes                                                       |
| --------------------------- | ---------------- | ----------------------------------------------------------- |
| `json.map()`                | `json.Value`     | A fresh empty JSON map (object) - the start of a document.  |
| `json.list()`               | `json.Value`     | A fresh empty JSON list (array).                            |
| `json.set(v, ptr, val)`     | `json.Value`     | Upsert a map key, or replace an in-range list index, at `ptr`. |
| `json.insert(v, ptr, val)`  | `json.Value`     | Insert `val` into a list before index `ptr` (or `-` = at end). |
| `json.append(v, ptr, val)`  | `json.Value`     | Push `val` onto the list addressed by `ptr` (sugar for insert at `/.../-`). |
| `json.remove(v, ptr)`       | `json.Value`     | Drop the map key or list element at `ptr`.                  |
| `json.move(v, from, to)`    | `json.Value`     | Relocate the subtree at `from` to `to` (read, remove, then `set`). |

## Encoding

`json.encode` walks the value and writes its JSON image:

| Jennifer                | JSON                                             |
| ----------------------- | ------------------------------------------------ |
| `null`                  | `null`                                           |
| `bool`                  | `true` / `false`                                 |
| `int`                   | integer number                                   |
| `float`                 | number (always with a `.` or exponent, so it decodes back as `float`) |
| `string`                | string (escaped)                                 |
| `bytes`                 | base64 string                                    |
| `list of T`             | array                                            |
| `map of string to V`    | object (in insertion order)                      |
| struct                  | object, keys in field-declaration order          |
| `json.Value`            | the document it wraps (a decode / encode round-trip) |

Encode errors (positioned at the call): a `map` with a non-string key, a
`task` value, or a non-finite `float` (`NaN` / infinity have no JSON
image).

**HTML-safe strings.** In string values the characters `<`, `>`, and `&`, and
the line / paragraph separators U+2028 / U+2029, are escaped as `\u003c`,
`\u003e`, `\u0026`, `\u2028`, and `\u2029` (matching Go's default
`SetEscapeHTML(true)`). This keeps encoded JSON safe to embed directly in an
HTML `<script>` block or a JSONP response - a value of `</script>` cannot break
out of the element, and the two separators cannot terminate a JavaScript string
literal. The output is still valid JSON and decodes back to the exact same
string, so a decode / encode round-trip is unaffected.

```jennifer
use io;
use json;

io.printf("%s\n", json.encode({"id": 1, "tags": ["a", "b"], "ok": true}));
# {"id":1,"tags":["a","b"],"ok":true}
```

## Decoding

`json.decode` returns an opaque **`json.Value`** - a handle onto the
parsed tree. It is deliberately opaque: operators, `[index]`, and
`.field` all reject it (with a hint to the accessors), so you never mix
a still-generic JSON node into typed code by accident. You reach inside
with the accessors, addressing nodes by **JSON Pointer** - the same paths
the (planned) write surface uses, so reads and writes are mirror images.

```jennifer
use io;
use json;

def doc as json.Value init json.decode("{\"x\": 7, \"y\": 8}");
io.printf("%d\n", json.asInt($doc, "/x") + json.asInt($doc, "/y"));   # 15
```

Under the hood a JSON object is a `map of string to V`, an array a
`list of V`, and a number an `int` when it has no fractional or exponent
part (else a `float`) - which is why `json.typeOf` reports `map` / `list`
/ `int` / `float`, Jennifer's own vocabulary, not "object" / "array" /
"number". `convert.typeOf($doc)` is the generic `"object"`;
`convert.objectType($doc)` is the specific `"json.Value"`.

A `def r as json.Value;` with no initializer is a JSON `null` node
(`json.isNull($r)` is `true`).

A `json.Value` **displays as its compact JSON**, so echoing `$doc` at the
REPL, `io.printf("%v", $doc)`, and `convert.toString($doc)` all show the
document (not an opaque `<json.Value>`); `json.encodePretty($doc)` is the
multi-line form.

### JSON Pointer (RFC 6901)

A pointer is a slash-separated list of keys and list indices, addressing
**exactly one** node:

- `""` (or an omitted argument) is the node itself.
- `/user/name` walks map key `user`, then key `name`.
- `/user/roles/0` walks to a list and takes index `0`. Indices are `0`
  or `[1-9][0-9]*` - no leading zeros, no negative indices. The `-`
  end-marker is a write-only position (`insert` / `append`); reads reject
  it.
- Escapes: a literal `/` in a key is written `~1`, a literal `~` is `~0`
  (so key `a/b` is `/a~1b`, key `m~n` is `/m~0n`).

Pointers are relative to the node you pass, so `json.get` to a subtree
and short relative pointers compose:

```jennifer
def user as json.Value init json.get($doc, "/user");
io.printf("%s\n", json.asString($user, "/name"));   # relative to $user
```

A missing key, an out-of-range or malformed index, or descending into a
scalar is a positioned error (catchable with `try`/`catch`); use
`json.has` to test a path without raising. There is no wildcard,
recursive-descent, or predicate syntax - a pointer names one node, which
is what lets the write verbs target it. (Query-style selection over many
nodes is a separate, deliberately unbuilt concern.)

To read the **last** element of a list, compute the index from
`json.length` - a plain pointer, no special syntax:

```jennifer
use convert;
def last as int init json.length($doc, "/qux") - 1;
io.printf("%d\n", json.asInt($doc, "/qux/" + convert.toString($last)));
```

### Walking a nested document

```jennifer
use lists;

def doc as json.Value init json.decode(
    "{\"user\": {\"name\": \"ada\", \"roles\": [\"admin\", \"dev\"]}}");

io.printf("name = %s\n", json.asString($doc, "/user/name"));
for (def i in lists.range(0, json.length($doc, "/user/roles"))) {
    io.printf("role = %s\n", json.asString($doc, "/user/roles/" + convert.toString($i)));
}
```

`json.typeOf` and `json.has` let you branch on shape before extracting -
handy when a field may be absent or polymorphic:

```jennifer
if (json.has($doc, "/count") and json.typeOf($doc, "/count") == "int") {
    def n as int init json.asInt($doc, "/count");
}
```

### Building and editing

The write verbs mirror the reads: same JSON Pointer addressing, but they
**return a new `json.Value`** rather than mutating in place (like
`lists`/`maps`), so you rebind the result:

```jennifer
def v as json.Value init json.map();          # {}
$v = json.set($v, "/name", "ada");            # {"name":"ada"}
$v = json.set($v, "/tags", json.list());      # {"name":"ada","tags":[]}
$v = json.append($v, "/tags", "admin");       # ...,"tags":["admin"]
$v = json.insert($v, "/tags/0", "root");      # ...,"tags":["root","admin"]
$v = json.set($v, "/tags/1", "dev");          # replace index 1
$v = json.remove($v, "/name");                # drop a key
$v = json.move($v, "/tags", "/roles");        # rename a branch
```

The value you store may be a scalar, a Jennifer `list` / `map` / struct
(a struct normalizes to a map, `bytes` to a base64 string), or another
`json.Value` (its tree is spliced in). Since writes never mutate, an
earlier handle keeps its old value:

```jennifer
def a as json.Value init json.decode("{\"n\":1}");
def b as json.Value init json.set($a, "/n", 2);
# json.asInt($a, "/n") is 1; json.asInt($b, "/n") is 2
```

Writes are **strict - no auto-vivification.** `set` creates only the
final pointer segment; a missing intermediate is an error, as is a
`set`/`insert` on a scalar or a bare `null` root:

```jennifer
def v as json.Value;                          # a null node
# json.set($v, "/a", 1)          -> error: cannot set a member of null
# json.set(json.map(), "/a/b", 1) -> error: no key "a" (build /a first)
```

So you build a nested document a level at a time, each parent created
before its child - the same discipline XML forces (you make an element
before you hang things on it), which is why the model ports cleanly. `set`
grows a map (new key) but only *replaces* an in-range list slot; grow a
list with `append` / `insert`. There is no deep-path creation and no
negative indexing - both deliberately, to keep pointers exactly RFC 6901
and unambiguous about list-vs-map intent.

The `-` end-of-array marker (RFC 6901's "position after the last element")
is honoured by **`insert` / `append` only**, where it means *at the end* -
so `json.insert($v, "/xs/-", x)` and `json.append($v, "/xs", x)` are the
same push. It is a write-position, not an element: `set` rejects `/xs/-`
(it never grows a list), and reads reject it too (there is no node there -
the last element is `length - 1`).

### Heterogeneous data

A JSON array or object whose values are of mixed types has no single
Jennifer type - a `list of T` / `map of string to V` is homogeneous. The
opaque `json.Value` sidesteps that: each leaf is extracted one at a time
with a type-checked `as*`, so a document like
`["Vienna", 2026, true, null, {...}, [1,2,3]]` is walked freely, element
by element. This is the main practical win of the handle over decoding
straight into a typed collection.

### No map-to-struct coercion

Jennifer does no coercion at binding boundaries (you write `5.0`, not
`5`, for a float), and JSON decode is no exception: there is no automatic
map-to-struct landing. To put JSON in a typed struct, rebuild it
explicitly from the decoded handle - the schema is right there:

```jennifer
def struct Point { x as int, y as int };
def d as json.Value init json.decode("{\"x\": 7, \"y\": 8}");
def p as Point init Point{ x: json.asInt($d, "/x"), y: json.asInt($d, "/y") };
```

The encode direction has no such restriction - `json.encode($p)` serializes
a struct to an object directly, and `json.encode($d)` re-serializes the
decoded handle. See [technical/rejected.md](../technical/rejected.md) for
why the decode coercion was declined.

## See also

- [convert.md](convert.md) - `convert.typeOf` (`"object"`) and
  `convert.objectType` (`"json.Value"`) for identifying a handle.
- [encoding.md](encoding.md) - hex / base64 and character-set codecs
  (`json` reuses base64 for `bytes`).
- [cheatsheet.md](cheatsheet.md) - every builtin at a glance.
