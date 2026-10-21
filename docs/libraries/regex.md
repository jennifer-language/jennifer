# `regex` - regular expressions

Enable with `use regex;`. Six verbs over `string`, one match
struct. RE2 syntax (Go's `regexp` package), a subset of PCRE
without backreferences or lookahead/lookbehind.

```jennifer
use io;
use regex;

if (regex.matches("^[A-Z][a-z]+$", "Hello")) {
    io.printf("looks capitalised\n");
}
```

## Surface

| Call                                       | Returns              | Notes                                                                    |
| ------------------------------------------ | -------------------- | ------------------------------------------------------------------------ |
| `regex.matches(pattern, s)`                | `bool`               | Does `pattern` match anywhere in `s`?                                    |
| `regex.find(pattern, s)`                   | `regex.Match`        | First match, or a sentinel with `start=-1` on no match.                  |
| `regex.findAll(pattern, s)`                | `list of regex.Match` | Every non-overlapping match, left to right.                             |
| `regex.replace(pattern, s, replacement)`   | `string`             | Replace every match. `$1`, `${name}` expand to captured groups.          |
| `regex.split(pattern, s)`                  | `list of string`     | Split `s` at every match of `pattern`.                                   |
| `regex.escape(s)`                          | `string`             | Escape metacharacters so `s` is treated as a literal pattern.            |

## The `regex.Match` struct

```jennifer
def struct regex.Match {
    text as string,                            # the full matched substring
    start as int,                              # rune index where the match starts
    end as int,                                # rune index where the match ends (exclusive)
    groups as list of string,                  # positional captures (index 0 = group 1)
    groupsNamed as map of string to string     # named captures (see below)
};
```

`start` and `end` are **rune indices**, consistent with
`strings.substring` and friends. Multi-byte characters advance
the count by one per rune, not per byte.

### No-match sentinel

`regex.find` on no match returns a Match with `start=-1`,
`end=-1`, `text=""`, empty `groups`, empty `groupsNamed`:

```jennifer
def m as regex.Match init regex.find("[0-9]+", "no digits here");
if ($m.start == -1) {
    io.printf("no match\n");
}
```

## Worked examples

**Write patterns as raw single-quoted literals** (`'\d+'`, not `"\\d+"`). In a
raw string backslashes are literal, so the pattern reads exactly as its RE2 form.
The cooked double-quoted form also works with every `\` doubled.

### Match a whole string

```jennifer
if (regex.matches("^[A-Z][a-zA-Z]*$", $name)) {
    # ... $name starts with a capital and has no digits ...
}
```

### Extract with positional groups

```jennifer
def m as regex.Match init regex.find('(\d+):(\d+)', "PORT=8080:9090");
if ($m.start >= 0) {
    io.printf("first=%s second=%s\n", $m.groups[0], $m.groups[1]);
}
```

### Extract with named groups

Named groups are addressed by name in `groupsNamed` and also
appear in positional `groups` (same order as they appear in
the pattern):

```jennifer
use regex;
use maps;

def m as regex.Match init regex.find(
    "(?P<key>[a-z]+)=(?P<value>[0-9]+)", "port=8080");
if ($m.start >= 0) {
    io.printf("key=%s value=%s\n",
        $m.groupsNamed["key"], $m.groupsNamed["value"]);
}
```

`maps.has($m.groupsNamed, "some_name")` returns whether a
named group is present without erroring on missing keys.

### Iterate every match

```jennifer
def all as list of regex.Match init regex.findAll('\d+', "a1 b22 c333");
for (def m in $all) {
    io.printf("%s at %d..%d\n", $m.text, $m.start, $m.end);
}
```

### Replace with group substitution

`$1` in the replacement string expands to positional group 1;
`${name}` expands to a named group. Doubled `$$` produces a
literal `$`.

```jennifer
def r as string init regex.replace('(\d+)', "port 8080", "[$1]");
# $r is "port [8080]"

def r2 as string init regex.replace(
    '(?P<host>\w+):(?P<port>\d+)', "example.com:80",
    "host=${host} port=${port}");
# $r2 is "host=example.com port=80"
```

### Split on a pattern

```jennifer
def parts as list of string init regex.split('\s+', "one   two  three");
# $parts is ["one", "two", "three"]
```

### Escape a literal

`regex.escape` returns a pattern that matches its input
verbatim, for building patterns from user input or literals
containing metacharacters:

```jennifer
def userInput as string init "1+2=(3)";
def pat as string init regex.escape($userInput);
# $pat is "1\\+2=\\(3\\)"
if (regex.matches($pat, $someHaystack)) { ... }
```

## Syntax

Jennifer uses **RE2 syntax** exactly as Go's `regexp` package
does; full reference: <https://github.com/google/re2/wiki/Syntax>.
The most-used pieces:

| Pattern         | Meaning                                                       |
| --------------- | ------------------------------------------------------------- |
| `.`             | Any character except newline (add `(?s)` for dotall).         |
| `^` / `$`       | Start / end of line (with `(?m)`) or of input (without).      |
| `\d` `\w` `\s`  | Digit / word char / whitespace.                               |
| `\D` `\W` `\S`  | Their complements.                                            |
| `[abc]` `[a-z]` | Character class.                                              |
| `[^abc]`        | Negated character class.                                      |
| `a?` `a*` `a+`  | 0-or-1, 0-or-more, 1-or-more (greedy).                        |
| `a??` `a*?`     | Lazy variants.                                                |
| `a{n,m}`        | Bounded repetition.                                           |
| `a|b`           | Alternation.                                                  |
| `(...)`         | Grouping and positional capture.                              |
| `(?:...)`       | Grouping without capture.                                     |
| `(?P<name>...)` | Named capture.                                                |
| `(?i)`          | Case-insensitive flag.                                        |
| `(?m)`          | Multiline (`^` / `$` match at line boundaries).               |
| `(?s)`          | Dotall (`.` matches newline).                                 |

**Not supported by RE2 (compile errors):**

- Backreferences: `\1`, `\k<name>`.
- Lookahead / lookbehind: `(?=...)`, `(?!...)`, `(?<=...)`,
  `(?<!...)`.
- Possessive quantifiers: `a++`.

RE2 omits these so matching stays linear in the input, which
makes it safe for untrusted input.

## Errors

- **Invalid pattern.** Positioned at the call site with the
  pattern quoted and the RE2 error message: `regex.find:
  invalid pattern "[unterminated": error parsing regexp:
  missing closing ]: `[unterminated``.
- **Wrong argument type.** Boundary error: `regex.matches:
  pattern must be string, got int`.
- **Wrong argument count.** Boundary error: `regex.replace
  expects 3 arguments (pattern, s, replacement), got 2`.

Every error is catchable with `try` / `catch`.

## Pattern caching

The library keeps a 128-entry LRU cache of compiled patterns,
so a hot loop reusing a pattern string compiles it once. Beyond
128 distinct patterns the oldest is evicted silently.

A pattern that compiles to an excessively large program (a short pattern can
expand to millions of instructions through nested bounded repeats) is a
catchable error, so the cache stays bounded in memory even under
attacker-supplied patterns.

A future `regex.compile` verb would expose explicit control if a
benchmark showed the implicit cache was not enough.

## What's left out

- **`regex.compile` + `regex.Pattern` handle.** The implicit
  LRU cache handles the common case.
- **Non-string operations** (regex over `bytes`).
- **Streaming iterator** (`for (def m in regex.iter(pat, s))`).
- **Global-flag arguments.** Case-insensitive as a boolean
  parameter would leak an option that already lives in the
  pattern (`(?i)`).
- **`regex.count`.** Write `len(regex.findAll(pat, s))`.
- **Backreferences, lookahead, lookbehind.** RE2 doesn't
  support them (the cost of linear-time matching).

## See also

- [`strings`](strings.md) - non-regex string helpers
  (`contains`, `split`, `indexOf`).
- [`convert`](convert.md) - `toString` for building patterns
  from mixed values.
- [../milestones.md](../milestones.md) - design spec.
- <https://github.com/google/re2/wiki/Syntax> - full RE2
  syntax reference.
