# `maps` - map manipulation

Enable with `use maps;`; every function is called as `maps.NAME(...)`.
Each function **returns a new map (or list)** and never mutates the input:

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

Reports whether the map contains the key. Check before an indexed read
to avoid the missing-key runtime error:

```jennifer
def m as map of string to int init {"a": 1};
if (maps.has($m, "a")) {
    io.printf("%d\n", $m["a"]);   # safe - we just checked
}
```

The list counterpart is [`lists.contains`](lists.md), which checks
values rather than keys; the different names keep that distinction
visible at the call site.

### `maps.delete` is strict

Deleting a key that isn't in the map raises a positioned runtime
error, matching the read-side rule that `$m[missing]` is an error.
For "delete if present", guard with `maps.has`:

```jennifer
if (maps.has($m, "stale")) {
    $m = maps.delete($m, "stale");
}
```

A silent no-op would let a misspelt key go unnoticed.

### `maps.merge` ordering

`merge(a, b)` returns `a`'s entries in `a`'s insertion order, with
`b`'s values overwriting where keys collide. New keys from `b` are
appended in `b`'s insertion order. So merging
`{"x": 1, "y": 2}` with `{"y": 99, "z": 3}` yields
`{"x": 1, "y": 99, "z": 3}`, as Python's `{**a, **b}` does.

### Value semantics

The library never modifies its inputs: `maps.delete` and `maps.merge`
return a new, independent map and leave the sources intact.

See also: [lists.md](lists.md), [index.md](index.md). `len(m)` is a
language built-in (no import needed).
