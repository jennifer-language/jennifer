# `lists` - list manipulation

Enable with `use lists;`. Namespaced under `lists.`; every function is
called as `lists.NAME(...)`. Each function **returns a new list** -
nothing mutates the input. Commit the result with the usual
assignment:

```jennifer
use io;
use lists;

def xs as list of int init [3, 1, 4, 1, 5];
$xs = lists.push($xs, 9);          # append item
$xs = lists.pop($xs);              # drop last
$xs = lists.sort($xs);             # sort ascending
io.printf("first=%d last=%d\n", lists.first($xs), lists.last($xs));
```

For the common "append to a list as you build it" pattern, the
language ships the `$xs[] = item;` sugar (see
[user-guide/types-and-values.md](../user-guide/types-and-values.md#the-xs-append-sugar)). It's
shorthand for `$xs = lists.push($xs, item);` **and, in a loop, much
faster**: `$xs[]` mutates in place through the copy-on-write protocol
(amortized O(N) for N appends), whereas `$xs = lists.push($xs, item)`
returns a new list each pass and copies the whole thing - O(N^2)
overall. Prefer `$xs[]` when building a list element by element; use
`lists.push` when you specifically want a fresh list and keep the
original.

## Functions

| Call                            | Returns      | Notes                                                              |
| ------------------------------- | ------------ | ------------------------------------------------------------------ |
| `lists.push(xs, item)`          | list         | New list with `item` appended. In a build-a-list loop prefer the `$xs[]` sugar (O(N) vs this call's O(N^2)). |
| `lists.pop(xs)`                 | list         | New list without the last element. Empty input errors.             |
| `lists.first(xs)`               | element kind | Element at index `0`. Empty input errors.                          |
| `lists.last(xs)`                | element kind | Element at the last index. Empty input errors.                     |
| `lists.head(xs, n)`             | list         | New list of the first `n` elements. `n` must be in `[0, len(xs)]`. |
| `lists.tail(xs, n)`             | list         | New list of the last `n` elements. Same range constraint.          |
| `lists.reverse(xs)`             | list         | New list, elements in reverse order.                               |
| `lists.sort(xs)`                | list         | New list sorted ascending. See "Sort" below.                       |
| `lists.contains(xs, item)`      | bool         | True iff `item` appears in `xs` under structural equality.         |
| `lists.concat(a, b)`            | list         | `a`'s elements followed by `b`'s.                                  |
| `lists.slice(xs, start)`        | list         | Elements from `start` to end (exclusive `end` = `len(xs)`).        |
| `lists.slice(xs, start, end)`   | list         | Elements `[start, end)`. Out-of-range bounds error.                |
| `lists.shuffle(xs)`             | list         | New list, elements in uniformly random order. See "Shuffle" below. |
| `lists.range(start, end)`       | list of int  | Half-open: `[start, start+1, ..., end-1]`. See "Range" below.      |
| `lists.range(start, end, step)` | list of int  | Walks by `step` while staying strictly before `end`. See "Range".  |
| `lists.map(xs, fn)`             | list         | New list of `fn(x)` for each `x`. See "Higher-order functions".     |
| `lists.filter(xs, fn)`          | list         | The elements where `fn(x)` returns `true`.                          |
| `lists.reduce(xs, fn, init)`    | element kind | Fold left: `acc = fn(acc, x)`, starting from `init`.                |
| `lists.find(xs, fn)`            | element kind | The first `x` where `fn(x)` is `true`; errors (catchable) if none.  |
| `lists.any(xs, fn)`             | bool         | True if `fn(x)` is `true` for any element (short-circuits).         |
| `lists.all(xs, fn)`             | bool         | True if `fn(x)` is `true` for every element (true for empty).       |
| `lists.sortBy(xs, keyFn)`       | list         | New list sorted ascending by the key `keyFn(x)` returns.           |

### Higher-order functions

`map` / `filter` / `reduce` / `find` / `any` / `all` / `sortBy` each take a
first-class **`func`** value (see the language guide) and call it back per
element. A bare method name in expression position is the function value, so you
pass a method by name:

```jennifer
use lists;
func dbl(n as int) { return $n * 2; }
func isEven(n as int) { return $n % 2 == 0; }

def xs as list of int init [1, 2, 3, 4, 5];
lists.map($xs, dbl);            # [2, 4, 6, 8, 10]
lists.filter($xs, isEven);     # [2, 4]
lists.reduce($xs, add, 0);     # sum, with func add(a as int, b as int) {...}
lists.any($xs, isEven);        # true
lists.all($xs, isEven);        # false
```

- **`map`** returns a **generic** list - its element type is whatever the
  callback returns, checked against the declared type at the binding site, so
  `def names as list of string init lists.map($people, nameOf);` type-checks only
  when every result is a string.
- **`filter`** / **`find`** / **`any`** / **`all`**: the callback must return a
  `bool`, else a positioned error. `find` errors (catchably) when nothing matches
  - guard with `lists.any` first, or wrap it in `try` / `catch`.
- **`sortBy`** takes a **key extractor** `keyFn(x) -> K` (a comparable scalar:
  `int` / `float` / `string` / `bool`) and sorts ascending by that key, stably.
  The key is extracted once per element, so `keyFn` runs O(n) times, not
  O(n log n). Sort descending by negating a numeric key (`func neg(n as int) {
  return 0 - $n; }`) or reversing the result. Sort structs by a field with a
  one-line accessor (`func ageOf(p as Person) { return $p.age; }`).

### Sort

`lists.sort` works on lists whose elements share a comparable kind:

- All elements `int` or `float` (mixed allowed - the comparison
  promotes int to float, same rule as `+`).
- All elements `string` - lexicographic order on the underlying
  rune sequence.
- All elements `bool` - `false < true`.

A list mixing strings with numbers, or containing `null`, `list`, or
`map` elements, errors at runtime rather than silently producing
nonsense. For a custom order, `lists.sortBy(xs, keyFn)` sorts by the key a
`func` value extracts (see "Higher-order functions" above).

### `first`/`last` versus `head`/`tail`

`first` and `last` return *elements* (the value at index 0 or
`len-1`). `head` and `tail` return *sublists* of length `n`, modelled
on the Unix `head` / `tail` commands. They are not aliases; pick the
one that matches what you actually want:

```jennifer
def xs as list of int init [10, 20, 30, 40, 50];
lists.first($xs);          # 10              (an int)
lists.last($xs);           # 50              (an int)
lists.head($xs, 2);        # [10, 20]        (a list of int)
lists.tail($xs, 2);        # [40, 50]        (a list of int)
```

For "everything except the first/last element", use `slice`:

```jennifer
lists.slice($xs, 1);                # [20, 30, 40, 50]
lists.slice($xs, 0, len($xs) - 1);  # [10, 20, 30, 40]
```

### Argument order

`lists.contains` puts the *haystack* first and the *needle* second
(`lists.contains($xs, item)`). Mirrors `strings.contains($s, $sub)`.
PHP's `in_array($needle, $haystack)` order is deliberately not
adopted - it's famously confusing.

### Shuffle

`lists.shuffle(xs)` returns a uniformly-random permutation of `xs` -
non-mutating, like every other helper in this library. The algorithm
is Durstenfeld's variant of the Fisher-Yates shuffle (O(n), uniform
across the n! permutations). The random source is the same one
`math.rand` / `math.randInt` / `math.randSeed` use, so calling
`math.randSeed(N)` before a shuffle makes the result deterministic
across runs:

```jennifer
use lists;
use math;

math.randSeed(1);
def a as list of int init lists.shuffle([1, 2, 3, 4, 5]);
math.randSeed(1);
def b as list of int init lists.shuffle([1, 2, 3, 4, 5]);
# $a and $b are byte-identical permutations.
```

Empty and single-element inputs are returned (still copied per the
non-mutating convention). The function does NOT require `use math;`
in the calling program - the shared source is a Go-side
implementation detail.

### Range

`lists.range(start, end)` returns a **half-open** list of consecutive
integers: `[start, start+1, ..., end-1]` for ascending, descending
implied when `start > end`. **End is exclusive.** Same semantic as
`lists.slice` and `strings.substring`, and the same shape every
half-open range function in the wider ecosystem has (Python
`range`, Go slice indexing, Rust `..`, etc.). The full design
rationale (including why we deviated from Jennifer's
English-reading default) is in
[design-decisions.md > Half-open ranges](../technical/design-decisions.md#half-open-ranges).

```jennifer
use lists;

lists.range(0, 5);          # [0, 1, 2, 3, 4]    - 5 elements
lists.range(1, 5);          # [1, 2, 3, 4]       - 4 elements (5 excluded)
lists.range(5, 5);          # []                 - coincident bounds: empty
lists.range(3, 0);          # [3, 2, 1]          - descending implied
```

Two properties fall out:

- **Index alignment.** `lists.range(0, len($xs))` gives exactly the
  valid indices for a list of `len($xs)` elements, matching how
  `$xs[i]` indexing works.
- **Composability.** `lists.concat(lists.range(a, b), lists.range(b, c))`
  is exactly `lists.range(a, c)`. Partitioning a range never
  duplicates or skips the boundary.

For the "count from 1 to N inclusive" idiom, write
`lists.range(1, N + 1)`.

`lists.range(start, end, step)` walks by `step`, always stopping
strictly before `end`. Positive step requires `start <= end`;
negative step requires `start >= end`; step must be non-zero
(positional error).

```jennifer
lists.range(0, 9, 3);       # [0, 3, 6]          - 9 excluded
lists.range(1, 9, 3);       # [1, 4, 7]          - 10 past 9, stop at 7
lists.range(10, 1, -3);     # [10, 7, 4]         - 1 excluded
lists.range(10, 0, -3);     # [10, 7, 4, 1]      - -2 past 0, stop at 1
```

There's no "did the step land?" question - the rule is uniformly
"emit while inside the open end."

### Value semantics

Every function copies its inputs; the original list is never modified.
Callers always re-bind the result:

```jennifer
def xs as list of int init [1, 2, 3];
def ys as list of int init lists.push($xs, 4);
# $xs is still [1, 2, 3]; $ys is [1, 2, 3, 4]
```

This matches the rest of Jennifer's value-semantics design - the same
rule that makes `$dst = $src;` a copy, not an alias.

See also: [maps.md](maps.md), [index.md](index.md). `len(xs)` is a
language built-in (no import needed).
