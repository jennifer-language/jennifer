# `strings` - text utilities

Enable with `use strings;`. Namespaced under `strings.`; every function is
called as `strings.NAME(...)`. Fourteen functions for the common things you
do with strings: case conversion, search, trim, replace, repeat,
substring extraction, and split / join.

> **M9 breaking change.** `strings` moved from flat to namespaced in M9.
> Pre-M9 callers wrote `upper(s)`, `contains(s, sub)`, etc.; the M9 form
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

See also: [../user-guide/index.md](../user-guide/index.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
