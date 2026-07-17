# `strings` - text utilities

Enable with `use strings;`. Namespaced under `strings.`; every function is
called as `strings.NAME(...)`. Fourteen functions for the common things you
do with strings: case conversion, search, trim, replace, repeat,
substring extraction, and split / join.

> **Breaking change.** `strings` moved from flat to namespaced.
> Callers used to write `upper(s)`, `contains(s, sub)`, etc.; the namespaced form
> is `strings.upper(s)`, `strings.contains(s, sub)`. Same library, just
> the call-site prefix is mandatory now. The rationale matches the
> lists/maps move: collision-prone verbs (`contains`, `split`,
> `replace`, ...) belong in their domain library to keep the bare-name
> pool clean.

> **Looking for `len(s)`?** It lives in the auto-loaded
> [`core`](core.md) library, so it's available everywhere without
> any `use` statement. The same `len` covers strings, lists, and maps
> with one polymorphic dispatch.

**String positions are 0-relative.** The first character is at index `0`,
not `1`. So `strings.indexOf("hello", "h")` returns `0`,
`strings.substring("hello", 0, 1)` returns `"h"`, and `len("hello")` is the
same as the index just past the last character (`5`). This matches Python,
JavaScript, Go, Rust, Java, C, C++, C#, Swift, Ruby. Lua/MATLAB/Pascal are
1-relative; Jennifer is not.

**All indices and lengths are rune-based** (Unicode code points), not
bytes. `len("héllo")` is `5`, not `6`. `strings.indexOf` and
`strings.substring` agree.

The combination of "0-relative" plus "exclusive end" plus "rune-based"
means `strings.substring(s, strings.indexOf(s, x), len(s))` always does
what you'd guess - the same units come out of every function.

```jennifer
use io;
use strings;

io.printf("%d\n", len("hello"));                       # 5  (core, auto-loaded)
io.printf("%s\n", strings.upper("hello"));             # "HELLO"
io.printf("%t\n", strings.contains("hello", "ell"));   # true
io.printf("%t\n", strings.startsWith("hello", "he"));  # true
io.printf("%d\n", strings.indexOf("hello", "l"));      # 2
io.printf("[%s]\n", strings.trim("  hi  "));           # "[hi]"
io.printf("%s\n", strings.replace("a-b-c", "-", "/")); # "a/b/c"
io.printf("%s\n", strings.repeat("ab", 3));            # "ababab"
io.printf("%s\n", strings.substring("hello", 1, 4));   # "ell"
```

## Functions

| Call                                          | Returns        | Notes                                                     |
| --------------------------------------------- | -------------- | --------------------------------------------------------- |
| `strings.upper(s)`, `strings.lower(s)`        | string         | Case conversion (Unicode-aware)                           |
| `strings.contains(s, sub)`                    | bool           | Substring search                                          |
| `strings.startsWith(s, prefix)`               | bool           |                                                           |
| `strings.endsWith(s, suffix)`                 | bool           |                                                           |
| `strings.indexOf(s, sub)`                     | int            | Rune index of first occurrence; `-1` if not found         |
| `strings.trim(s)`                             | string         | Strip leading and trailing whitespace                     |
| `strings.trimLeft(s)`, `strings.trimRight(s)` | string         | One-sided trim                                            |
| `strings.replace(s, old, new)`                | string         | Replace **all** occurrences of `old` with `new`           |
| `strings.repeat(s, n)`                        | string         | `n` copies concatenated; `n` must be non-negative         |
| `strings.substring(s, start)`                 | string         | Rune-indexed; from `start` to the end of the string       |
| `strings.substring(s, start, end)`            | string         | Rune-indexed; **exclusive end**                           |
| `strings.split(s, sep)`                       | list of string | Split on a non-empty separator; preserves empty segments  |
| `strings.chars(s)`                            | list of string | One single-rune string per Unicode code point             |
| `strings.join(parts, sep)`                    | string         | Concatenate a `list of string` with `sep` between entries |

`strings.split` and `strings.chars` complement each other: use
`strings.chars(s)` to break a string into runes (one entry per code
point), `strings.split(s, sep)` to break on a delimiter substring.
`strings.join` is the inverse of `strings.split` for any non-empty
separator: `strings.join(strings.split(s, sep), sep) == s`.

## Indexing rules

`strings.substring`, `strings.indexOf`, and `len` all agree on rune
indices. So given `s = "héllo"`:
- `len(s)` = `5`
- `strings.indexOf(s, "l")` = `2`
- `strings.substring(s, 0, 2)` = `"hé"`
- `strings.substring(s, 2, 5)` = `"llo"`
- `strings.substring(s, 2)` = `"llo"` (2-arg form, end defaults to `len(s)`)

The 2-arg `strings.substring(s, start)` is the same as
`strings.substring(s, start, len(s))` - a common case worth shortening.

## Errors

- `strings.substring(s, -1, 3)` - negative start.
- `strings.substring(s, 0, 99)` - end past the string length.
- `strings.substring(s, 4, 2)` - end before start.
- `strings.repeat(s, -1)` - negative count.
- Non-string arguments where strings are required (`len(42)`).
- Non-int arguments where ints are required (`strings.repeat("x", "3")`).
- Arity errors (too many or too few arguments).

## Whitespace

`strings.trim` / `strings.trimLeft` / `strings.trimRight` use Unicode
whitespace (Go's `unicode.IsSpace`): ASCII spaces, tabs, newlines, plus
characters like non-breaking space (`U+00A0`) and Unicode line
separators.

If you need to trim specific characters instead of whitespace, that's
not in v1 - propose `strings.trimChars(s, chars)` as a follow-up if it
comes up.

## Performance

Most `strings` calls are cheap, and for short, human-sized text - a
title, a filename, a line of input - you never need to think about
speed. Two everyday patterns can get slow when strings grow to
thousands of characters and you touch them in a loop. Both come down to
how much work gets *repeated*; the O(1) / O(n) / O(n²) shorthand below is
explained in [Big O notation](https://en.wikipedia.org/wiki/Big_O_notation).
The gist: **O(n)** means the work grows in step with the input, and
**O(n²)** means it grows with the *square* of the input - ten times the
text, a hundred times the work.

### Build strings with `join`, not `+` in a loop

Every `+` on strings makes a new string and copies both sides into it.
Once is fine. In a loop, each step re-copies everything built so far, so
the total work is O(n²):

```jennifer
# Slow: O(n^2). Each step recopies the whole result so far.
def out as string init "";
for (def piece in $pieces) {
    $out = $out + $piece;
}
```

Instead, collect the pieces in a list and join them in one pass.
`strings.join` visits each piece exactly once, so the whole thing is
O(n):

```jennifer
# Fast: O(n). Collect the parts, then join once.
def parts as list of string init [];
for (def piece in $pieces) {
    $parts[] = $piece;
}
def out as string init strings.join($parts, "");
```

Same idea whenever you assemble something big from many small pieces -
an HTML page, a CSV file, a log line built from several fields: gather
the parts, then `strings.join` at the end.

### Avoid re-copying a big value

Two things in Jennifer quietly make a *whole copy* of a value, which
costs O(n) in the size of that value:

- **Passing a list or string to a function.** Jennifer uses value
  semantics: a function gets its own copy of each argument, so it can
  never change the caller's data by surprise. Convenient - but calling
  `helper($bigList)` inside a loop copies the entire list *every time*,
  turning an O(n) loop into O(n²).
- **`strings.substring` counts from the start.** Because indices are
  rune-based (see [Indexing rules](#indexing-rules)), `substring` walks
  the string from the beginning to reach `start`. One call is fine;
  slicing thousands of small pieces out of one long string adds up to
  O(n²).

The workaround for both is the same idea: don't hand the whole big value
over and over. Convert the string to a rune list *once* with
`strings.chars`, then read single positions with `$cs[i]` (that is O(1)
each), and when you need a slice back as a string use `lists.slice` plus
`strings.join` - which copies only the slice, not the whole string
(needs `use lists;`):

```jennifer
# Slow: substring re-scans from the start on every call.
def piece as string init strings.substring($big, $start, $end);

# Fast: index into a rune list built once; copy only the slice.
def cs as list of string init strings.chars($big);              # once
# ...then, cheaply, as many times as you like:
def piece as string init strings.join(lists.slice($cs, $start, $end), "");
```

Keeping the big value in one place and passing only small things into
helpers - an index, a short piece - follows the same rule.

See also: [../user-guide/index.md](../user-guide/index.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
