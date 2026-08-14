# `binary` - bulk operations on `bytes`

Enable with `use binary;`. Namespaced under `binary.`; every function is
called as `binary.NAME(...)`. It is to `bytes` what [`strings`](strings.md)
is to `string` and [`lists`](lists.md) is to `list`: concatenate, slice,
search, and split byte buffers.

The library is named `binary` because `bytes` is a reserved type keyword
(it appears after `as`, as in `def b as bytes;`) and so cannot be a library
namespace.

**All indices and lengths are byte offsets**, 0-relative. `binary.slice(b, 0, 2)`
returns the first two bytes; `len(b)` is the byte count. (Contrast
[`strings`](strings.md), which counts Unicode code points.)

**Every operation is non-mutating and value-semantic.** The inputs are never
aliased or written; each result is a freshly-allocated `bytes` (or `list of
bytes` / `int` / `bool`). Mutating a result never touches the source.

```jennifer
use io;
use convert;
use binary;

def a as bytes init convert.bytesFromString("Hello, ", "utf-8");
def b as bytes init convert.bytesFromString("World", "utf-8");
def c as bytes init binary.concat($a, $b);

io.printf("%s\n", convert.stringFromBytes($c, "utf-8"));         # "Hello, World"
io.printf("%d\n", binary.indexOf($c, $b));                       # 7
io.printf("%s\n", convert.stringFromBytes(binary.slice($c, 7, 12), "utf-8")); # "World"
io.printf("%t\n", binary.startsWith($c, $a));                    # true
```

## Functions

| Call                             | Returns        | Notes                                                                              |
| -------------------------------- | -------------- | ---------------------------------------------------------------------------------- |
| `binary.concat(a, b)`            | bytes          | Join two byte sequences. See [Building buffers](#building-buffers) for the loop caveat. |
| `binary.join(parts)`             | bytes          | Concatenate every `bytes` in a `list of bytes` into one, in `O(n)`. The byte-data `strings.join`. |
| `binary.join(parts, sep)`        | bytes          | As above, with `sep` (a `bytes`) placed between adjacent pieces.                    |
| `binary.slice(b, start)`         | bytes          | From `start` to the end of `b`.                                                    |
| `binary.slice(b, start, end)`    | bytes          | The half-open range `[start, end)`; **exclusive end**.                             |
| `binary.indexOf(haystack, needle)`  | int            | Byte index of the first occurrence of `needle`; `-1` if absent. An empty `needle` returns `0`. Same shape as `strings.indexOf`. |
| `binary.contains(haystack, needle)` | bool           | Whether `needle` occurs in `haystack` (the boolean sibling of `indexOf`, like `strings.contains`). |
| `binary.split(b, sep)`           | list of bytes  | Split on every occurrence of a non-empty `sep`; preserves empty segments.          |
| `binary.startsWith(b, prefix)`   | bool           | True iff `b` begins with `prefix`.                                                 |
| `binary.endsWith(b, suffix)`     | bool           | True iff `b` ends with `suffix`.                                                   |

`binary.split` is the natural way to cut a container at a delimiter in one
pass - for example splitting a MIME multipart body on its boundary, or a
length-unprefixed record stream on its separator:

```jennifer
def parts as list of bytes init binary.split($body, $boundary);
```

## Errors

- `binary.slice(b, -1, 3)` - negative start.
- `binary.slice(b, 0, 99)` - end past the byte length.
- `binary.slice(b, 4, 2)` - end before start.
- `binary.split(b, sep)` with an empty `sep`.
- Non-bytes arguments where `bytes` are required, or non-int where int is
  required (`binary.slice(b, "3")`).
- Arity errors (too many or too few arguments).

All are positioned, catchable runtime errors.

## Why this library exists: throughput

The reason to reach for `binary` over a hand-written loop is speed. A `.j`
program that concatenates, slices, searches, or splits a byte buffer *one byte
at a time* pays the tree-walking interpreter's per-operation cost on every byte
(an index read, a comparison, an append, a loop step). For kilobyte-sized data
that is invisible; for a megabyte-scale buffer it dominates. Each `binary`
function pushes the inner loop into Go and runs it once at native speed
(`binary.indexOf` / `binary.split` are Go's `bytes.Index` / `bytes.Split`), so
scanning a large buffer for a delimiter is one call, not a per-byte scan.

```jennifer
# Slow: a per-byte scan pays the interpreter's cost on every byte.
def idx as int init -1;
def i as int init 0;
while ($i + len($needle) <= len($buf) and $idx < 0) {
    # ... byte-by-byte comparison ...
    $i = $i + 1;
}

# Fast: one Go-speed scan.
def idx as int init binary.indexOf($buf, $needle);
```

### Building buffers

`binary.concat` copies both inputs into a fresh result, so it is
`O(len(a) + len(b))` per call. Using it to accumulate a growing buffer in a
loop (`$buf = binary.concat($buf, $chunk)` each iteration) re-copies the whole
buffer every time, which is `O(n^2)` overall. To assemble a buffer from a
stream, read it in one call instead: [`net.readAll`](net.md) reads a whole
response body to EOF as one `bytes`, and [`net.readN`](net.md) reads an exact
length-prefixed frame - both grow a single Go slice rather than concatenating in
a `.j` loop. Reserve `binary.concat` for joining a small, fixed number of
pieces.

When you already hold the pieces - you built them in a loop rather than read
them off a stream - collect them in a `list of bytes` and make one
`binary.join` call. That is `O(n)` (a single allocation sized to the total),
where the concat-in-a-loop it replaces is `O(n^2)`. It is the exact byte-data
analogue of building a string with list-append plus `strings.join`:

```jennifer
# Slow: each concat re-copies the whole accumulator - O(n^2).
def out as bytes;
for (def chunk in $chunks) {
    $out = binary.concat($out, $chunk);
}

# Fast: append the pieces, join once - O(n).
def parts as list of bytes init [];
for (def chunk in $chunks) {
    $parts[] = $chunk;
}
def out as bytes init binary.join($parts);
```

## TinyGo

`binary` is pure byte manipulation with no host dependencies, so it runs in
full on both binaries - the default `jennifer` and the constrained
`jennifer-tiny`.

See also: [`strings`](strings.md), [`net`](net.md) (`readAll` / `readN`),
[index.md](index.md).
