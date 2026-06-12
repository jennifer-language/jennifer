# `lists` - list manipulation (M9)

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
shorthand for `$xs = lists.push($xs, item);`.

## Functions

| Call                          | Returns      | Notes                                                              |
| ----------------------------- | ------------ | ------------------------------------------------------------------ |
| `lists.push(xs, item)`        | list         | New list with `item` appended.                                     |
| `lists.pop(xs)`               | list         | New list without the last element. Empty input errors.             |
| `lists.first(xs)`             | element kind | Element at index `0`. Empty input errors.                          |
| `lists.last(xs)`              | element kind | Element at the last index. Empty input errors.                     |
| `lists.head(xs, n)`           | list         | New list of the first `n` elements. `n` must be in `[0, len(xs)]`. |
| `lists.tail(xs, n)`           | list         | New list of the last `n` elements. Same range constraint.          |
| `lists.reverse(xs)`           | list         | New list, elements in reverse order.                               |
| `lists.sort(xs)`              | list         | New list sorted ascending. See "Sort" below.                       |
| `lists.contains(xs, item)`    | bool         | True iff `item` appears in `xs` under structural equality.         |
| `lists.concat(a, b)`          | list         | `a`'s elements followed by `b`'s.                                  |
| `lists.slice(xs, start)`      | list         | Elements from `start` to end (exclusive `end` = `len(xs)`).        |
| `lists.slice(xs, start, end)` | list         | Elements `[start, end)`. Out-of-range bounds error.                |

### Sort

`lists.sort` works on lists whose elements share a comparable kind:

- All elements `int` or `float` (mixed allowed - the comparison
  promotes int to float, same rule as `+`).
- All elements `string` - lexicographic order on the underlying
  rune sequence.
- All elements `bool` - `false < true`.

A list mixing strings with numbers, or containing `null`, `list`, or
`map` elements, errors at runtime rather than silently producing
nonsense. Comparator-based sort (`lists.sortBy`) is deferred until
methods are first-class values.

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

See also: [maps.md](maps.md), [core.md](core.md) (`len`), [index.md](index.md).
