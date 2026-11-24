# Performance

Most Jennifer code needs no tuning. Values are copied at every store site
(assignment, parameter binding, a `spawn` snapshot), which makes aliasing bugs
impossible, and the interpreter elides the copy wherever it can prove that is
invisible:

- **Read-only parameters are borrowed, not copied.** A parameter a function never
  writes is bound by alias, so passing a large `list` or `map` into a helper that
  only reads it does not deep-copy it. No annotation is needed. Two cases
  still copy on every call - see
  [When a read-only argument is still copied](#when-a-read-only-argument-is-still-copied).
- **A mutate-and-return helper updates in place.** Inside a function,
  `$grid = put($grid, x, y, v);` hands `$grid` to `put` without a copy (the old
  value is dead once the call returns), and the value `put` mutates and returns
  comes back without one, so the call costs the same for a ten-cell grid as for a
  million-cell one. The variable has to be both passed and assigned, and
  be a local or parameter of the function, outside any `try` there (a `catch` could
  still read the old value). Top-level code (its variables are globals), a global
  inside a function, or a different target (`def next as Grid init put($grid, ...)`)
  copies the value once per call.
- **Call frames are pooled and slot-bound.** A deeply recursive program allocates
  a near-constant handful of objects rather than one per call, so recursion and
  tight call loops put no pressure on the garbage collector by themselves.

This page lists the few patterns where the *shape* of your code decides whether a
hot path is linear or quadratic, and how to find the hot path.

**Measure before you change anything.** [`jennifer profile`](../technical/cli_profile.md) reports
hits and wall-clock time per source position (`--by=function` per function), with
time spent waiting apart from time spent working; `jennifer profile --allocs` reports
value-semantics copies, ranked by the bytes each site copied. Optimize the line the
profiler names, not the line you suspect. A hot path is almost always a single loop;
the rest of the program does not repay the readability spent tuning it.

## Build strings, lists, and bytes by appending, not by re-concatenating

`$s = $s + piece` copies the *entire* string accumulated so far on every step.
Growing a string with `+` inside a loop is therefore **quadratic**: a few thousand
appends spend most of their time copying intermediate strings:

```jennifer
def rows as list of string init ["a", "b", "c"];

# O(N^2): each `+` copies the whole accumulated string
def out as string init "";
for (def row in $rows) {
    $out = $out + $row + "\n";
}
```

Collect the pieces in a `list of string` with the `$xs[] = item;` append form,
amortised **O(N)** because a list grows its own backing in place, and join once at
the end:

```jennifer
def rows as list of string init ["a", "b", "c"];

# O(N): append is amortised constant, a single join builds the result
def lines as list of string init [];
for (def row in $rows) {
    $lines[] = $row;
}
def out as string init strings.join($lines, "\n");
```

Building up `bytes` works the same way: append with `$b[] = byte;` and use the
buffer directly (there is no `+` for `bytes`). Any accumulator rebuilt with a
whole-value operation each iteration has this shape: in a tight loop prefer
`$xs[] = item;` over `$xs = lists.push($xs, item);`, which builds a fresh copy.

Use `+` only to join a small, fixed number of pieces (`$name + ": " + $value`);
in a loop, append and join.

**When you must concatenate in a loop, group the small piece.** A long
left-associative chain re-copies the growing accumulator at every `+`:
`$s + a + b + c + d` copies `$s` four times. Parenthesising the small part builds it
once and touches the large accumulator a single time:

```jennifer
func addField(out as string, field as string, value as string) {
    $out = $out + $field + ": " + $value + "\n";      # copies the growing $out at every +
    $out = $out + ($field + ": " + $value + "\n");    # builds the chunk once, one copy of $out
    return $out;
}
```

Both produce the same string (concatenation is associative).

## Push byte-buffer work into the `binary` library

`bytes` has a second trap: every byte operation in `.j` (reading a byte, comparing
it, appending one) pays the tree-walker's per-operation cost. On a kilobyte that is
invisible; on a megabyte buffer it dominates. The [`binary`](../libraries/binary.md)
library runs the inner loop in Go, so both shapes below have a one-call answer.

**Assembling many chunks - `binary.join`, never concat in a loop.** `bytes` has no
`+`, but `binary.concat` is the same quadratic trap: it copies both inputs into a
fresh result, so growing a buffer with it re-copies the whole accumulator every
iteration - **O(N^2)**.

```jennifer
def chunks as list of bytes init [encoding.encode("utf-8", "ab"), encoding.encode("utf-8", "cd")];

# O(N^2): each concat re-copies the whole accumulated buffer
def out as bytes;
for (def chunk in $chunks) {
    $out = binary.concat($out, $chunk);
}
```

`binary.join` is the byte-data counterpart of `strings.join`: append the pieces to a
`list of bytes`, then join once into a single allocation sized to the total - **O(N)**.

```jennifer
def chunks as list of bytes init [encoding.encode("utf-8", "ab"), encoding.encode("utf-8", "cd")];

# O(N): append the pieces, one join builds the result
def parts as list of bytes init [];
for (def chunk in $chunks) {
    $parts[] = $chunk;
}
def out as bytes init binary.join($parts);   # optional separator: binary.join($parts, $sep)
```

Appending *single bytes* (`$b[] = byte;`) is already amortised O(N); joining
*chunks* you hold is `binary.join`. Only `binary.concat` in a loop is quadratic.
When the chunks arrive off a *stream*, [`net.readN`](../libraries/net.md) /
`net.readAll` grow one buffer for you; use them instead of a read-and-concat loop.

**Searching and slicing - one native call, not a hand-rolled per-byte scan.**
Walking a buffer byte by byte to find a delimiter, or copying out a range with an
index loop, pays the interpreter for every byte. `binary.indexOf` / a slice (`$buf[a..b]`) /
`binary.split` / `binary.contains` carry that loop into Go (`bytes.Index`, a
`copy`), so locating a CRLF or a MIME boundary in a large body is one call:

```jennifer
func findSlow(buf as bytes, needle as bytes) {
    # Slow: a per-byte comparison loop pays the interpreter's cost on every byte
    def idx as int init -1;
    def i as int init 0;
    while ($i + len($needle) <= len($buf) and $idx < 0) {
        # ... byte-by-byte comparison ...
        $i = $i + 1;
    }
    return $idx;
}

func findFast(buf as bytes, needle as bytes) {
    return binary.indexOf($buf, $needle);   # one native-speed scan
}
```

### Caution: byte offsets are not character offsets - do not cut a rune

Byte-indexed `binary` operations are faster than rune-indexed `strings` ones, but
**a multi-byte UTF-8 character occupies several bytes, and a byte-position cut can
land in the middle of one.** Decoding such a fragment with
`encoding.decode("utf-8", $b)` is a **runtime error** (the decoder rejects a
truncated sequence), so an accented letter or an emoji across the cut point makes a
working program throw.

Byte-scanning is safe because ASCII is transparent to UTF-8: every byte of a multi-byte character is `>= 0x80`, so an ASCII byte - a `\n`,
a `,`, a `|`, the `*` in a `/**` - can never appear inside one. Cutting *at an ASCII
delimiter* (the usual reason to scan bytes at all) is therefore always rune-safe:
the boundaries fall between characters by construction. `binary.indexOf` /
`binary.split` on an ASCII needle, and a slice that ends at an ASCII marker, are all
fine.

The trap is a cut at an *arbitrary* byte position - a fixed `offset + 4096`
look-ahead window, a midpoint derived from a byte length. If you must cut there,
back the end off any partial trailing rune before decoding; a UTF-8 continuation
byte is in `0x80..0xBF`:

```jennifer
func decodeRange(buf as bytes, start as int, end as int) {
    # Back the cut to a character boundary so the slice never ends mid-rune
    while ($end > $start and $end < len($buf) and $buf[$end] >= 128 and $buf[$end] < 192) {
        $end = $end - 1;
    }
    return encoding.decode("utf-8", $buf[$start..$end]);
}
```

Counting bytes, searching for an ASCII delimiter, or moving raw `bytes` around
never has this problem; it bites only when you slice at a computed position and
decode the result to a `string`.

## Get hot calls out of the innermost loop

A function call costs two separable things: the **copy** of each argument into the
callee, and the **call itself** - borrowing a frame, binding the parameters, the
call-depth check, and walking the function body. The read-only-parameter borrow
(above) makes the copy free, not the call, so a helper invoked *once per element* in
a hot loop still pays the call overhead once per element:

```jennifer
# classify() is invoked once per element - N call frames
func scan(cs as list of string) {
    def n as int init 0;
    def i as int init 0;
    while ($i < len($cs)) {
        if (classify($cs[$i])) { $n = $n + 1; }
        $i = $i + 1;
    }
    return $n;
}
```

In the hottest loops, get the call off the per-element path. In rough order of
preference:

- **Inline** the helper's body into the loop when it is small. No call at all.
- **Batch** it: call the helper once over the whole collection (a `classifyAll(cs)`
  that returns a `list`), so the per-element work runs inside a single frame rather
  than one frame per element.
- **Lift loop-invariant calls** out of the loop entirely: anything the helper
  recomputes each iteration that does not depend on the current element (a config
  read, a constant) belongs above the loop.

A syntax highlighter that called a predicate once per character ran **1.37x**
faster after this rewrite.

**Do this surgically**, where a profiler shows a five- or six-digit hit count on one
line. Everywhere else, keep factoring code into small helpers: the call overhead is
invisible outside the hottest loops.

## When a read-only argument is still copied

The borrow has two gaps. In each, a call costs time proportional to the size of
its argument, so a helper called in a loop over a large value goes quadratic:

- **Nested collections.** A parameter of type `list of list`, `list of map`,
  `map of K to list` or `map of K to map` is copied on every call, even when the
  function only reads it. Wrap the value in a struct and pass the struct, which
  is borrowed:

  ```jennifer
  def struct Matrix { rows as list of list of float };
  func cell(m as Matrix, r as int, c as int) { return $m.rows[$r][$c]; }   # borrowed
  func cellOf(rows as list of list of float, r as int, c as int) {        # copied per call
      return $rows[$r][$c];
  }
  ```

- **Callbacks.** In a program whose entry file declares any mutable top-level
  `def` - even an unused `int` - a parameter is borrowed only when the
  interpreter can prove the function writes no global, directly or through what it
  calls. A function that reaches a callback builtin (`lists.map`, `filter`,
  `reduce`, `find`, `any`, `all`, `sortBy`, `sortWith`, `meta.call`, `meta.callMain`, the
  `testing` runners) or calls a function value gets no such proof, so it copies
  every compound argument on every call. That holds when the callback is a named
  function that writes nothing, and when the call is reached through a helper.
  Separately, one `spawn` anywhere in the run that can do any of this turns the
  proof off for the whole run. In a hot path, sort with `lists.sort` on computed
  keys instead of `lists.sortBy`, or make the top-level state `def const`.

`jennifer profile --allocs` shows both as eager copies at the call site. The
roadmap plans to close them (M30.3 and M30.8). A call into a module passes its
arguments and result as a local call does.

## See also

- [`jennifer profile`](../technical/cli_profile.md) - flags and output formats.
- [Types and values](types-and-values.md) - value semantics, the copy-on-store
  model these patterns work around.
- [`binary` library](../libraries/binary.md) - the bulk-`bytes` functions
  (`join` / `concat` / `indexOf` / `split` / `contains`).
- [`strings` library](../libraries/strings.md) - `join`, the string counterpart of
  the append-and-join pattern.
- [Best practices](best-practices.md) - the stylistic companion to this page.
