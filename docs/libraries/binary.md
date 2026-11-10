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

def a as bytes init convert.bytesFromString("utf-8", "Hello, ");
def b as bytes init convert.bytesFromString("utf-8", "World");
def c as bytes init binary.concat($a, $b);

io.printf("%s\n", convert.stringFromBytes("utf-8", $c));         # "Hello, World"
io.printf("%d\n", binary.indexOf($c, $b));                       # 7
io.printf("%s\n", convert.stringFromBytes("utf-8", binary.slice($c, 7, 12))); # "World"
io.printf("%t\n", binary.startsWith($c, $a));                    # true
```

## Functions

| Call                             | Returns        | Notes                                                                              |
| -------------------------------- | -------------- | ---------------------------------------------------------------------------------- |
| `binary.make(n)`                 | bytes          | Allocate `n` zero bytes - the fixed-size buffer allocator (`bytes` has no literal). `n` in `[0, 256 MiB]` (32 MiB on `jennifer-tiny`). |
| `binary.make(n, fill)`           | bytes          | As above, every byte set to `fill` (a byte value in `[0, 255]`).                    |
| `binary.concat(a, b)`            | bytes          | Join two byte sequences. See [Building buffers](#building-buffers) for the loop caveat. |
| `binary.join(parts)`             | bytes          | Concatenate every `bytes` in a `list of bytes` into one, in `O(n)`. The byte-data `strings.join`. The result is at most 256 MiB (32 MiB on `jennifer-tiny`). |
| `binary.join(parts, sep)`        | bytes          | As above, with `sep` (a `bytes`) placed between adjacent pieces.                    |
| `binary.slice(b, start)`         | bytes          | From `start` to the end of `b`.                                                    |
| `binary.slice(b, start, end)`    | bytes          | The half-open range `[start, end)`; **exclusive end**.                             |
| `binary.indexOf(haystack, needle)`  | int            | Byte index of the first occurrence of `needle`; `-1` if absent. An empty `needle` returns `0`. Same shape as `strings.indexOf`. |
| `binary.indexOf(haystack, needle, from)` | int       | As above but searching from byte offset `from` (in `[0, len]`); returns the absolute index, `-1` if none, `from` for an empty `needle`. `from = idx + 1` each step makes find-all one O(n) pass. |
| `binary.indexOf(haystack, needle, from, limit)` | int | As above but over the half-open window `[from, limit)` (`from <= limit <= len`): a match is reported only when it lies wholly inside it, and the scan never reads past `limit`. Same match as `indexOf(slice(haystack, from, limit), needle)` without the copy, reported as an absolute offset - so a bounded probe does not overscan the rest of the buffer when the needle is absent. |
| `binary.contains(haystack, needle)` | bool           | Whether `needle` occurs in `haystack` (the boolean sibling of `indexOf`, like `strings.contains`). |
| `binary.split(b, sep)`           | list of bytes  | Split on every occurrence of a non-empty `sep`; empty pieces are kept, so `n` separators give `n + 1` parts (an empty `b` gives one empty part). |
| `binary.startsWith(b, prefix)`   | bool           | True iff `b` begins with `prefix`.                                                 |
| `binary.endsWith(b, suffix)`     | bool           | True iff `b` ends with `suffix`.                                                   |
| `binary.bitXor(data, key)`       | bytes          | `data` XOR `key`, the key repeated over the data; the result has `data`'s length. An equal-length key is an element-wise XOR, a short one a repeating mask (a WebSocket frame mask). `key` must not be empty; key bytes past `data`'s length are ignored. |
| `binary.bitAnd(data, key)`       | bytes          | `data` AND `key`, with the same key rules as `bitXor` (clear bits with a mask). |
| `binary.bitOr(data, key)`        | bytes          | `data` OR `key`, with the same key rules as `bitXor` (set bits with a mask). |
| `binary.bitNot(data)`            | bytes          | Every bit of `data` inverted. |
| `binary.readInt(order, b, offset, size)` | int    | The signed integer of `size` bytes (1, 2, 4 or 8) at `offset`; `order` is `"le"` or `"be"`. |
| `binary.readUint(order, b, offset, size)` | int   | As `readInt`, unsigned. An 8-byte value above the int range is an error. |
| `binary.readFloat(order, b, offset, size)` | float | The IEEE 754 float of `size` bytes (4 or 8) at `offset`. A NaN or infinity is an error. |
| `binary.readInts(order, b, offset, count, size[, stride])` | list of int | `count` signed integers, `stride` bytes apart (default `size`). |
| `binary.readFloats(order, b, offset, count, size[, stride])` | list of float | `count` floats, `stride` bytes apart (default `size`). |

`binary.split` cuts a container at a delimiter in one pass, e.g. a MIME
multipart body on its boundary, or a length-unprefixed record stream on its
separator:

```jennifer
def parts as list of bytes init binary.split($body, $boundary);
```

## Numbers

The `read*` functions decode fixed-width numbers of a binary format at a byte
offset, in Go. Rebuilding a number from single bytes in `.j` costs a dozen
interpreted operations per value; one call does it at native speed. The bulk
forms read a column: with `stride` set to the record length, `readFloats`
reads one field of every record in a row-major block.

A `.j` loop of single `read*` calls costs ~1 us per value (1M `readUint`
~0.8 s); one `readInts` / `readFloats` call reads 1M values in ~30 ms.

```jennifer
use binary;

def magic as int init binary.readUint("be", $head, 0, 4);
def count as int init binary.readInt("le", $head, 4, 2);
# the 8-byte float at offset 16 of each 40-byte record:
def col as list of float init binary.readFloats("le", $data, 16, $rows, 8, 40);
```

Jennifer has no NaN or infinity, so `readFloat` and `readFloats` throw on a
non-finite bit pattern. A format that encodes missing values as NaN payloads
(SAS) reads the bits with `readUint(order, b, offset, 8)` and decides itself.
`readUint` with size 8 throws above 2^63 - 1; read such fields as two 4-byte
halves.

## Errors

- `binary.slice(b, -1, 3)` - negative start.
- `binary.slice(b, 0, 99)` - end past the byte length.
- `binary.slice(b, 4, 2)` - end before start.
- `binary.make(n)` with `n` < 0 or above 256 MiB; a `fill` outside `[0, 255]`.
- `binary.indexOf` with `from` outside `[0, len]` or `limit` outside `[from, len]`.
- `binary.join` with a non-`bytes` element, or a result above 256 MiB.
- `binary.split(b, sep)` with an empty `sep`, or more than 4,194,304 parts
  (1,048,576 on `jennifer-tiny`).
- `binary.bitXor` / `bitAnd` / `bitOr` with an empty `key`.
- Non-bytes arguments where `bytes` are required, or non-int where int is
  required (`binary.slice(b, "3")`).
- `read*`: a span past the end of `b`, a negative offset, a size outside the
  allowed set, an order other than `"le"` / `"be"`, a non-finite float, an
  8-byte unsigned value above the int range, a `count` outside `[0, len(b)]`
  or above 4,194,304 (1,048,576 on `jennifer-tiny`), a `stride` below 1. The offset is checked
  even when `count` is 0.
- Arity errors (too many or too few arguments).

All are positioned, catchable runtime errors.

## Performance

A `.j` loop pays the interpreter's per-operation cost on every byte; each
`binary` call runs its loop once in Go (`indexOf` and `split` use
`bytes.Index`), so scanning a large buffer for a delimiter is one call.

```jennifer
# Slow: a per-byte scan pays the interpreter's cost on every byte.
def idx as int init -1;
def i as int init 0;
while ($i + len($needle) <= len($buf) and $idx < 0) {
    # ... byte-by-byte comparison ...
    $i = $i + 1;
}

# Fast: one Go-speed scan.
def found as int init binary.indexOf($buf, $needle);
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

Pieces built in a loop: append them to a `list of bytes` and call
`binary.join` once (one allocation, `O(n)`):

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
def joined as bytes init binary.join($parts);
```

Appending single bytes with `$out[] = byte` is linear too (~0.35 us per byte,
loop included).

## TinyGo

`binary` has no host dependencies, so it runs in full on both `jennifer` and
`jennifer-tiny`.


See also: [`strings`](strings.md), [`net`](net.md) (`readAll` / `readN`),
[index.md](index.md).
