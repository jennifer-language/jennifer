# `maps` - map manipulation (M9)

Enable with `use maps;`. Namespaced under `maps.`; every function is
called as `maps.NAME(...)`. Each function **returns a new map (or
list)** - the input is never mutated:

```jennifer
use io;
use maps;

def scores as map of string to int init {"alice": 90, "bob": 80};
def names as list of string init maps.keys($scores);   # ["alice", "bob"]
def values as list of int init maps.values($scores);   # [90, 80]
def shrunk as map of string to int init maps.delete($scores, "bob");
def with_dave as map of string to int init maps.merge($scores, {"dave": 75});
```

## Functions

| Call                  | Returns                | Notes                                                                     |
| --------------------- | ---------------------- | ------------------------------------------------------------------------- |
| `maps.keys(m)`        | `list of <key type>`   | The map's keys, in insertion order.                                       |
| `maps.values(m)`      | `list of <value type>` | The map's values, in insertion order.                                     |
| `maps.has(m, key)`    | bool                   | Key membership test. The non-erroring companion to `$m[key]`.             |
| `maps.delete(m, key)` | map                    | New map without `key`. Missing key is an error - see below.               |
| `maps.merge(a, b)`    | map                    | New map: `a`'s entries with `b`'s layered on top (`b` wins on collision). |

### `maps.has`

Reports whether the map contains the given key. Pair it with the
indexed-read to avoid the missing-key runtime error:

```jennifer
def m as map of string to int init {"a": 1};
if (maps.has($m, "a")) {
    io.printf("%d\n", $m["a"]);   # safe - we just checked
}
```

Pre-M9 this lived in core as bare `has(...)`; it moved here because
map-only membership is domain-specific and didn't fit core's
"universally needed structural primitives" charter (`len`, by
contrast, is genuinely polymorphic across string / list / map and
stays). The lists side of "does this contain that?" is
[`lists.contains`](lists.md), which checks values rather than keys -
the differing names keep the role distinction visible at every call
site.

### `maps.delete` is strict

Deleting a key that isn't in the map raises a positioned runtime
error - matching the read-side rule that `$m[missing]` is an error
rather than a silent default. Callers who want a "delete if present"
shape can guard with `maps.has`:

```jennifer
if (maps.has($m, "stale")) {
    $m = maps.delete($m, "stale");
}
```

This is "strict at boundaries" applied to writes: silent no-ops on
missing keys would let typos drift through unnoticed.

### `maps.merge` ordering

`merge(a, b)` returns `a`'s entries in `a`'s insertion order, with
`b`'s values overwriting where keys collide. New keys from `b` are
appended in `b`'s insertion order. So merging
`{"x": 1, "y": 2}` with `{"y": 99, "z": 3}` yields
`{"x": 1, "y": 99, "z": 3}` - same shape Python's `{**a, **b}`
produces.

### Value semantics

The library never modifies its inputs. `maps.delete` and `maps.merge`
both copy, so the source maps stay intact for further use; the new
map is independent of either.

See also: [lists.md](lists.md), [index.md](index.md). `len(m)` is a
language built-in (no import needed).
