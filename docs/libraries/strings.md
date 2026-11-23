# `strings` - text utilities

Enable with `use strings;`; every function is called as
`strings.NAME(...)`. Case conversion, search, trim, replace, replicate,
split / join, and accent folding. Substrings are slices (`$s[a..b]`), part of
the language.

`len(s)` is a language built-in (no `use` needed), covering strings, lists,
maps, and bytes.

**String positions are 0-relative.** The first character is at index `0`,
not `1`: `strings.indexOf("hello", "h")` returns `0`, the slice
`"hello"[0..1]` is `"h"`, and `len("hello")` is the index just past the last
character (`5`), as in Python, JavaScript, Go and C (unlike Lua, MATLAB,
Pascal).

**All indices and lengths are rune-based** (Unicode code points), not
bytes. `len("héllo")` is `5`, not `6`. `strings.indexOf` and slicing
(`$s[a..b]`) agree.

Every function uses the same units (0-relative, exclusive end, runes), so
`$s[strings.indexOf($s, $x)..]` is the rest of `$s` from the first `$x` on.

```jennifer
use io;
use strings;

io.printf("%d\n", len("hello"));                       # 5  (built-in, no use needed)
io.printf("%s\n", strings.upper("hello"));             # "HELLO"
io.printf("%t\n", strings.contains("hello", "ell"));   # true
io.printf("%t\n", strings.startsWith("hello", "he"));  # true
io.printf("%d\n", strings.indexOf("hello", "l"));      # 2
io.printf("[%s]\n", strings.trim("  hi  "));           # "[hi]"
io.printf("%s\n", strings.trimRightSet("1.500", "0"));  # "1.5"
io.printf("%s\n", strings.replace("a-b-c", "-", "/")); # "a/b/c"
io.printf("%s\n", strings.replicate("ab", 3));         # "ababab"
io.printf("%s\n", "hello"[1..4]);                      # "ell" (a slice)
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
| `strings.indexOf(s, sub, from)`               | int            | As above but searching from rune offset `from` (in `[0, len]`); on non-ASCII `s` each call counts runes from the start |
| `strings.lastIndexOf(s, sub)`                 | int            | Rune index of the *last* occurrence; `-1` if not found    |
| `strings.count(s, sub)`                       | int            | Number of non-overlapping occurrences, scanning left to right (`count("aaaa", "aa")` is `2`); an empty `sub` is an error |
| `strings.trim(s)`                             | string         | Strip leading and trailing whitespace                     |
| `strings.trimLeft(s)`, `strings.trimRight(s)` | string         | One-sided trim                                            |
| `strings.trimPrefix(s, prefix)`               | string         | `s` without `prefix` when it starts with it (removed once), else `s` |
| `strings.trimSuffix(s, suffix)`               | string         | `s` without `suffix` when it ends with it (removed once), else `s` |
| `strings.trimSet(s, cutset)`                  | string         | Strip leading and trailing runes that occur in `cutset` (a set of runes, not a substring) |
| `strings.trimLeftSet(s, cutset)`, `strings.trimRightSet(s, cutset)` | string | One-sided cutset trim                     |
| `strings.replace(s, old, new)`                | string         | Replace **all** occurrences of `old` with `new`           |
| `strings.replicate(s, n)`                     | string         | `n` copies concatenated; `n` must be non-negative         |
| `strings.split(s, sep)`                       | list of string | Split on a non-empty separator; preserves empty segments. More than 4194304 parts (1048576 on `jennifer-tiny`) is an error |
| `strings.chars(s)`                            | list of string | One single-rune string per Unicode code point; same part limit as `split` |
| `strings.join(parts[, sep])`                  | string         | Concatenate a `list of string` with `sep` between entries; Without `sep` the parts are joined with `""`, as `binary.join` does. A result over 256 MiB (32 MiB on `jennifer-tiny`) is an error |

`strings.split` and `strings.chars` complement each other: use
`strings.chars(s)` to break a string into runes (one entry per code
point), `strings.split(s, sep)` to break on a delimiter substring.
`strings.join` is the inverse of `strings.split` for any non-empty
separator: `strings.join(strings.split(s, sep), sep) == s`.

## Indexing rules

Slices, `strings.indexOf`, and `len` all agree on rune indices. So given
`$s = "héllo"`:
- `len($s)` = `5`
- `strings.indexOf($s, "l")` = `2`
- `$s[0..2]` = `"hé"`
- `$s[2..5]` = `"llo"`
- `$s[2..]` = `"llo"` (an open end defaults to `len($s)`)

`strings.substring` was removed in 0.28.0; use a slice.

## Errors

| Case | Example |
| --- | --- |
| Slice with negative start, end past the length, or end before start (a language rule) | `$s[-1..3]`, `$s[0..99]`, `$s[4..2]` |
| Negative count | `strings.replicate(s, -1)` |
| `replicate` / `replace` / `join` result over 256 MiB (32 MiB on `jennifer-tiny`), kind `"limit"`; the same cap applies to every result one call builds, bytes included (`binary.make`, `binary.join`) | `replace(): result exceeds the 268435456-byte limit` |
| `split` / `chars` with more than 4194304 parts (1048576 on `jennifer-tiny`), kind `"limit"` | `chars(): 5000000 runes exceed the limit of 4194304; ...` |
| Wrong argument type | `len(42)`, `strings.replicate("x", "3")` |
| Wrong number of arguments | `strings.upper()` |

## Whitespace

`strings.trim` / `strings.trimLeft` / `strings.trimRight` use Unicode
whitespace (Go's `unicode.IsSpace`): ASCII spaces, tabs, newlines, plus
characters like non-breaking space (`U+00A0`) and Unicode line
separators.

To strip something else, use the affix or the cutset family; neither
touches whitespace unless you name it:

- `strings.trimPrefix(s, prefix)` / `strings.trimSuffix(s, suffix)` remove one
  exact affix, once: `trimSuffix("a\r\r", "\r")` is `"a\r"`, and a line keeps
  its significant spaces when only the `\r` of a CRLF ending goes
  (`trimSuffix($line, "\r")`). An empty affix, or one `s` does not carry,
  returns `s` unchanged.
- `strings.trimSet(s, cutset)` / `trimLeftSet` / `trimRightSet` strip any run
  of runes that occur in `cutset`, in any order: `trimRightSet("1.500", "0")`
  is `"1.5"`, `trimRightSet("100", "0.")` is `"1"`, `trimSet("--a-b--", "-")`
  is `"a-b"`. The cutset is a set of runes (`"äö"` is two), not a substring;
  an empty cutset strips nothing. Use `trimPrefix` to remove a known string.

## Performance

For short text (a title, a filename, a line of input) speed does not
matter. Two patterns get slow when strings grow to thousands of
characters and are touched in a loop, because work gets *repeated*
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

### Avoid re-copying a big value

Two things cost O(n) in the size of a value:

- **Passing a list to a function that writes its parameter.** Under value
  semantics a function gets its own copy of an argument it modifies, so
  calling such a `helper($bigList)` inside a loop copies the entire list
  *every time*, turning an O(n) loop into O(n²). A parameter the function
  only reads is passed without a copy (see
  [Performance](../user-guide/performance.md)).
- **Slicing `$s[a..b]` and `strings.indexOf(s, sub, from)` on non-ASCII
  text count runes from the start of the string on every call**, so slicing
  many pieces out of one long string is O(n²). On ASCII text (every rune one
  byte) they locate the offset in O(1). For non-ASCII
  text, keep offsets relative to a shorter piece.

Keep the big value in one place and pass helpers only small things (an
index, a short piece).

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
