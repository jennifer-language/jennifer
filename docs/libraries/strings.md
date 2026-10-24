# `strings` - text utilities

Enable with `use strings;`; every function is called as
`strings.NAME(...)`. Case conversion, search, trim, replace, repeat,
substring extraction, split / join, and accent folding.

> **Breaking change.** `strings` moved from flat to namespaced: `upper(s)`,
> `contains(s, sub)` are now `strings.upper(s)`, `strings.contains(s, sub)`.
> Collision-prone verbs (`contains`, `split`, `replace`, ...) belong in their
> domain library, as with `lists` / `maps`.

> **`len(s)` is a language built-in** (no `use` needed), covering strings,
> lists, maps, and bytes.

**String positions are 0-relative.** The first character is at index `0`,
not `1`. So `strings.indexOf("hello", "h")` returns `0`,
`strings.substring("hello", 0, 1)` returns `"h"`, and `len("hello")` is the
same as the index just past the last character (`5`), as in Python,
JavaScript, Go, Rust, Java, C and most other languages (unlike Lua, MATLAB,
Pascal).

**All indices and lengths are rune-based** (Unicode code points), not
bytes. `len("héllo")` is `5`, not `6`. `strings.indexOf` and
`strings.substring` agree.

Because every function uses the same units (0-relative, exclusive end,
runes), `strings.substring(s, strings.indexOf(s, x), len(s))` does what
you'd expect.

```jennifer
use io;
use strings;

io.printf("%d\n", len("hello"));                       # 5  (built-in, no use needed)
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
| `strings.fold(s)`                             | string         | Remove common Latin diacritics for a sort / search key (`Österreich` -> `Osterreich`, `café` -> `cafe`, `ß` -> `ss`); case preserved, NFD combining accents dropped. Not full Unicode collation. |
| `strings.contains(s, sub)`                    | bool           | Substring search                                          |
| `strings.startsWith(s, prefix)`               | bool           |                                                           |
| `strings.endsWith(s, suffix)`                 | bool           |                                                           |
| `strings.indexOf(s, sub)`                     | int            | Rune index of first occurrence; `-1` if not found         |
| `strings.indexOf(s, sub, from)`               | int            | As above but searching from rune offset `from` (in `[0, len]`); `from = idx + 1` walks every occurrence in one pass |
| `strings.lastIndexOf(s, sub)`                 | int            | Rune index of the *last* occurrence; `-1` if not found    |
| `strings.count(s, sub)`                       | int            | Number of non-overlapping occurrences, scanning left to right (`count("aaaa", "aa")` is `2`); an empty `sub` is an error |
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
`strings.substring(s, start, len(s))`.

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

Trimming specific characters instead of whitespace is not supported
(a possible future `strings.trimChars(s, chars)`).

## Performance

For short text (a title, a filename, a line of input) speed does not
matter. Two patterns get slow when strings grow to thousands of
characters and you touch them in a loop, because work gets *repeated*
([Big O notation](https://en.wikipedia.org/wiki/Big_O_notation): **O(n)**
grows with the input, **O(n²)** with its square - ten times the text, a
hundred times the work).

### Build strings with `join`, not `+` in a loop

Every `+` on strings makes a new string and copies both sides into it.
In a loop, each step re-copies everything built so far, so the total
work is O(n²):

```jennifer
# Slow: O(n^2). Each step recopies the whole result so far.
def out as string init "";
for (def piece in $pieces) {
    $out = $out + $piece;
}
```

Instead, collect the pieces in a list and join them once; `strings.join`
visits each piece once, so the whole thing is O(n):

```jennifer
# Fast: O(n). Collect the parts, then join once.
def parts as list of string init [];
for (def piece in $pieces) {
    $parts[] = $piece;
}
def out as string init strings.join($parts, "");
```

The same applies to assembling an HTML page, a CSV file, or a log line
from many pieces.

### Avoid re-copying a big value

Two things cost O(n) in the size of a value:

- **Passing a list to a function that writes its parameter.** Under value
  semantics a function gets its own copy of an argument it modifies, so
  calling such a `helper($bigList)` inside a loop copies the entire list
  *every time*, turning an O(n) loop into O(n²). A parameter the function
  only reads is passed without a copy (see
  [Performance](../user-guide/performance.md)).
- **`strings.substring` counts from the start.** Because indices are
  rune-based (see [Indexing rules](#indexing-rules)), `substring` walks
  the string from the beginning to reach `start`. One call is fine;
  slicing thousands of small pieces out of one long string adds up to
  O(n²).

The fix for both: don't hand over the whole big value repeatedly. Convert
the string to a rune list *once* with `strings.chars`, read single
positions with `$cs[i]` (O(1)), and when you need a slice back as a string
use `lists.slice` plus `strings.join`, which copies only the slice (needs
`use lists;`):

```jennifer
# Slow: substring re-scans from the start on every call.
def piece as string init strings.substring($big, $start, $end);

# Fast: index into a rune list built once; copy only the slice.
def cs as list of string init strings.chars($big);              # once
# ...then, cheaply, as many times as you like:
def piece as string init strings.join(lists.slice($cs, $start, $end), "");
```

Likewise, keep the big value in one place and pass helpers only small
things (an index, a short piece).

## Accent-folding for sorting and search

`strings.fold(s)` strips common Latin diacritics to build a locale-ish sort or
search key, case preserved: `strings.fold("Österreich")` is `"Osterreich"`,
`strings.fold("café")` is `"cafe"`, `strings.fold("Straße")` is `"Strasse"`.
Ordering operators (`<` / `>` / `<=` / `>=`) and `lists.sort` compare strings by
raw UTF-8 byte, so an accented word sorts *after* all of ASCII - `"Österreich"`
lands after `"Zebra"`, not near `"Oslo"`. Fold a key to fix that:

```jennifer
use strings;
use lists;

func key(s as string) { return strings.lower(strings.fold($s)); }
def ordered as list of string init lists.sortBy($names, key);   # Österreich sorts near O
```

It covers the common Western / Central European letters (Latin-1 Supplement plus
common Latin Extended-A) and a few expansions (`ß` / `ẞ` -> `ss` / `SS`,
`Æ` -> `AE`); runes outside that table pass through unchanged. A stray combining
accent (an NFD-decomposed diaeresis, acute, etc. in the U+0300-U+036F range) is
dropped, so the decomposed spelling of a word folds to the same key as its
precomposed form. It is **not** full Unicode collation: no per-locale ordering
(German phonebook `ö` = `oe`, Swedish `ö` after `z`), no non-Latin scripts. For
those, sort by a key that suits your locale; a dedicated collation library is a
horizon idea.

See also: [../user-guide/index.md](../user-guide/index.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
