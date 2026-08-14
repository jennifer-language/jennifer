# Performance

Most Jennifer code never needs performance tuning, and the interpreter already
handles the parts that would otherwise be traps. Values are copied eagerly at
every store site - assignment, parameter binding, a `spawn` snapshot - which is
what makes aliasing bugs impossible. But the interpreter elides the copy wherever
it can prove doing so is invisible, and it does this for you automatically:

- **Read-only parameters are borrowed, not copied.** A parameter a method never
  writes is bound by alias, so passing a large `list` or `map` into a helper that
  only reads it does not deep-copy it. You get this with no annotation and no
  special syntax - just write the read-only helper.
- **Call frames are pooled and slot-bound.** A deeply recursive program allocates
  a near-constant handful of objects rather than one allocation per call, so
  recursion and tight call loops do not generate garbage-collector pressure on
  their own.

So this page is not "hand-optimize everything." It is the short list of patterns
where the *shape* of your code, not the interpreter, decides whether a hot path is
linear or quadratic - plus how to find the hot path in the first place.

**Measure before you change anything.** [`jennifer profile`](tooling.md) reports
hits and wall-clock time per source position; `jennifer profile --allocs` reports
value-semantics copies. Optimize the line the profiler names, not the line you
suspect. A hot path in a Jennifer program is almost always a single loop; the rest
of the program does not repay the readability you would spend tuning it.

## Build strings, lists, and bytes by appending, not by re-concatenating

Because values are copied rather than shared, `$s = $s + piece` copies the
*entire* string accumulated so far on every step. Growing a string with `+` inside
a loop is therefore **quadratic** - the total work grows with the square of the
number of pieces, and a few thousand appends spend most of their time copying
intermediate strings they immediately throw away:

```jennifer
# O(N^2): each `+` copies the whole accumulated string
def out as string init "";
for (def row in $rows) {
    $out = $out + $row + "\n";
}
```

Collect the pieces in a `list of string` with the `$xs[] = item;` append form -
which is amortised **O(N)**, because a list grows its own backing in place with no
whole-value copy - and join once at the end:

```jennifer
# O(N): append is amortised constant, a single join builds the result
def lines as list of string init [];
for (def row in $rows) {
    $lines[] = $row;
}
def out as string init strings.join($lines, "\n");
```

Building up `bytes` works the same way: append with `$b[] = byte;` and use the
buffer directly (there is no `+` for `bytes`). And it is not only strings - any
accumulator you rebuild with a whole-value operation each iteration has this shape.
Prefer the in-place append (`$xs[] = item;`) over `$xs = lists.push($xs, item);`
inside a tight loop for the same reason: the append grows the binding's own backing,
the `push` builds a fresh copy.

Reach for `+` only to join a small, fixed number of pieces (`$name + ": " + $value`).
The moment a concatenation lives inside a loop, switch to append-and-join.

**When you must concatenate in a loop, group the small piece.** A long
left-associative chain re-copies the growing accumulator at every `+`:
`$s + a + b + c + d` copies `$s` four times. Parenthesising the small part builds it
once and touches the large accumulator a single time:

```jennifer
$out = $out + $field + ": " + $value + "\n";      # copies the growing $out at every +
$out = $out + ($field + ": " + $value + "\n");    # builds the chunk once, one copy of $out
```

Both produce the same string (concatenation is associative), so this is a free
rewrite - do not touch the large accumulator more often than you must.

## Push byte-buffer work into the `binary` library

Everything above is about not re-copying an accumulator. `bytes` has a second trap
on top of that: every individual byte operation from `.j` - reading a byte,
comparing it, appending one - pays the tree-walker's per-operation cost. On a
kilobyte that is invisible; on a megabyte buffer it dominates. The
[`binary`](../libraries/binary.md) library exists for exactly this - each function
runs its inner loop once in Go, at native speed - so the two byte-buffer hot shapes
both have a one-call answer.

**Assembling many chunks - `binary.join`, never concat in a loop.** `bytes` has no
`+`, but `binary.concat` is the same quadratic trap: it copies both inputs into a
fresh result, so growing a buffer with it re-copies the whole accumulator every
iteration - **O(N^2)**.

```jennifer
# O(N^2): each concat re-copies the whole accumulated buffer
def out as bytes;
for (def chunk in $chunks) {
    $out = binary.concat($out, $chunk);
}
```

`binary.join` is the byte-data counterpart of `strings.join`: append the pieces to a
`list of bytes`, then join once into a single allocation sized to the total - **O(N)**.

```jennifer
# O(N): append the pieces, one join builds the result
def parts as list of bytes init [];
for (def chunk in $chunks) {
    $parts[] = $chunk;
}
def out as bytes init binary.join($parts);   # optional separator: binary.join($parts, $sep)
```

This is the same distinction as the string case, split by what you hold: appending
*single bytes* one at a time (`$b[] = byte;`) is already the amortised-O(N) in-place
form; joining *chunks* you already have is `binary.join`. Only `binary.concat` in a
loop is quadratic. (When the chunks arrive off a *stream* rather than a list in hand,
[`net.readN`](../libraries/net.md) / `net.readAll` grow one Go slice for you - reach
for those over a read-and-concat loop.)

**Searching and slicing - one native call, not a hand-rolled per-byte scan.**
Walking a buffer byte by byte to find a delimiter, or copying out a range with an
index loop, pays the interpreter on every byte. `binary.indexOf` / `binary.slice` /
`binary.split` / `binary.contains` carry that loop into Go (`bytes.Index`, a
`copy`), so locating a CRLF or a MIME boundary in a large body is one call:

```jennifer
# Slow: a per-byte comparison loop pays the interpreter's cost on every byte
def idx as int init -1;
def i as int init 0;
while ($i + len($needle) <= len($buf) and $idx < 0) {
    # ... byte-by-byte comparison ...
    $i = $i + 1;
}

# Fast: one native-speed scan
def idx as int init binary.indexOf($buf, $needle);
```

The rule mirrors the string one: build a `bytes` result by appending single bytes
or by joining chunks, never by concatenating a growing buffer in a loop - and when
you search or slice a whole buffer, let one `binary` call do the walking.

### Caution: byte offsets are not character offsets - do not cut a rune

Dropping from rune-indexed `strings` operations to byte-indexed `binary` ones buys
speed, but hands you one responsibility the string layer handled for you: **a
multi-byte UTF-8 character occupies several bytes, and a byte-position cut can land
in the middle of one.** Decoding such a fragment back with
`convert.stringFromBytes($b, "utf-8")` is a **runtime error** - the decoder is
strict and rejects a truncated sequence - so a buffer that merely happens to carry
an accented letter or an emoji across your cut point turns a working program into a
throwing one.

What makes byte-scanning safe in the first place is that ASCII is transparent to
UTF-8: every byte of a multi-byte character is `>= 0x80`, so an ASCII byte - a `\n`,
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
# Back the cut to a character boundary so the slice never ends mid-rune
while ($end > $start and $end < len($buf) and $buf[$end] >= 128 and $buf[$end] < 192) {
    $end = $end - 1;
}
def s as string init convert.stringFromBytes(binary.slice($buf, $start, $end), "utf-8");
```

(Counting bytes, searching for an ASCII delimiter, or moving raw `bytes` around
untouched never has this problem - it bites only when you slice at a computed
position and then decode the result back to a `string`.)

## Get hot calls out of the innermost loop

A method call costs two separable things: the **copy** of each argument into the
callee, and the **call itself** - borrowing a frame, binding the parameters, the
call-depth check, and walking the method body. Read-only-parameter borrow (above)
makes the copy free when the callee only reads its argument, but it cannot make the
call itself free. So a helper invoked *once per character or per element* in a hot
loop still pays the call overhead once per element, even after the argument copy is
gone:

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

The win is real, and it is the last one available at the source level: one real
workload - a syntax highlighter that called a predicate once per character - ran
**1.37x** faster after this rewrite, with byte-identical output and no interpreter
change.

**Do this surgically.** This is a hot-inner-loop transformation, the kind a profiler
flags with a five- or six-digit hit count on one line. Everywhere else, keep
factoring code into small helpers: the call overhead is invisible outside the
hottest loops, and readable structure is worth far more than a call you make a few
hundred times. Inlining is a readability cost you pay only where the profile earns
it.

## See also

- [Editor & AI support / tooling](tooling.md) - running `jennifer profile`.
- [Types and values](types-and-values.md) - value semantics, the copy-on-store
  model these patterns work around.
- [`binary` library](../libraries/binary.md) - the bulk-`bytes` functions
  (`join` / `concat` / `slice` / `indexOf` / `split`) and their throughput rationale.
- [`strings` library](../libraries/strings.md) - `join`, the string counterpart of
  the append-and-join pattern.
- [Best practices](best-practices.md) - the stylistic companion to this page.
