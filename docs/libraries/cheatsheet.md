# Cheatsheet - all builtins at a glance

Alphabetical index of every standard-library function and constant. Use
it when you know the *name* and want to know *which library* and *how to
call it*; use each library's own page when you want to read about a
topic.

The table covers what ships with the interpreter today. New
entries land here at the same time as the per-library doc - it's a
flat lookup view, not authoritative.

## Functions

| Call                          | Library                         | What it does                                                                  |
|-------------------------------|---------------------------------|-------------------------------------------------------------------------------|
| `abs(x)`                      | [math](math.md)                 | Absolute value of `x` (int→int, float→float).                                 |
| `bool(v)`                     | [convert](convert.md)           | Canonical conversion to `bool` (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`).    |
| `ceil(x)`                     | [math](math.md)                 | Smallest int ≥ `x`. Accepts int (identity) or float.                          |
| `chars(s)`                    | [strings](strings.md)           | Split `s` into a `list of string`, one entry per Unicode code point.          |
| `contains(s, sub)`            | [strings](strings.md)           | True if `s` contains the substring `sub`.                                     |
| `endsWith(s, suffix)`         | [strings](strings.md)           | True if `s` ends with `suffix`.                                               |
| `eof()`                       | [io](io.md)                     | True if and only if the next `readLine()` would error. Pair with `while (not eof()) {...}`. |
| `float(v)`                    | [convert](convert.md)           | Convert to float (int→float, float identity, string parses, bool→1.0/0.0).   |
| `floor(x)`                    | [math](math.md)                 | Largest int ≤ `x`. Accepts int (identity) or float.                           |
| `indexOf(s, sub)`             | [strings](strings.md)           | Rune index of first `sub` in `s`, or `-1` if absent.                          |
| `int(v)`                      | [convert](convert.md)           | Convert to int (float truncates toward zero, string parses, bool→1/0).       |
| `join(parts, sep)`            | [strings](strings.md)           | Concatenate `list of string` `parts` separated by `sep`. Inverse of `split`. |
| `len(v)`                      | [core](core.md) *(auto-loaded)* | Structural length: rune count (string), element count (list), entry count (map). |
| `lists.concat(a, b)`          | [lists](lists.md)               | New list with `a`'s elements followed by `b`'s.                               |
| `lists.contains(xs, item)`    | [lists](lists.md)               | True if `item` appears in `xs` (haystack, needle).                            |
| `lists.first(xs)`             | [lists](lists.md)               | Element at index 0. Empty input errors.                                       |
| `lists.head(xs, n)`           | [lists](lists.md)               | New list of the first `n` elements.                                           |
| `lists.last(xs)`              | [lists](lists.md)               | Element at the last index. Empty input errors.                                |
| `lists.pop(xs)`               | [lists](lists.md)               | New list without the last element. Empty input errors.                        |
| `lists.push(xs, item)`        | [lists](lists.md)               | New list with `item` appended.                                                |
| `lists.reverse(xs)`           | [lists](lists.md)               | New list with elements reversed.                                              |
| `lists.slice(xs, start[, end])` | [lists](lists.md)             | New sublist `[start, end)`; `end` defaults to `len(xs)`.                      |
| `lists.sort(xs)`              | [lists](lists.md)               | New ascending-sorted list. Numeric / string / bool elements; mixed errors.    |
| `lists.tail(xs, n)`           | [lists](lists.md)               | New list of the last `n` elements.                                            |
| `lower(s)`                    | [strings](strings.md)           | Lowercase `s` (Unicode-aware).                                                |
| `maps.delete(m, key)`         | [maps](maps.md)                 | New map without `key`. Missing key errors (strict at boundaries).             |
| `maps.has(m, key)`            | [maps](maps.md)                 | True if map `m` contains `key`. The non-erroring companion to `$m[key]`.     |
| `maps.keys(m)`                | [maps](maps.md)                 | List of keys in insertion order.                                              |
| `maps.merge(a, b)`            | [maps](maps.md)                 | New map; `b`'s entries layered on top of `a`.                                 |
| `maps.values(m)`              | [maps](maps.md)                 | List of values in insertion order.                                            |
| `max(a, b)`                   | [math](math.md)                 | Larger of two numbers; mixed int/float promotes to float.                     |
| `min(a, b)`                   | [math](math.md)                 | Smaller of two numbers; mixed int/float promotes to float.                    |
| `os.getEnv(name)`             | [os](os.md)                     | Read environment variable `name`. Unset → empty string, no error.             |
| `os.platform()`               | [os](os.md)                     | Operating-system name as reported by the runtime (`"linux"` today).            |
| `pow(x, y)`                   | [math](math.md)                 | `x` raised to `y`; always float. Errors on NaN/Inf-producing inputs.          |
| `printf(value)`               | [io](io.md)                     | Write a value's display form to stdout.                                       |
| `printf(format, args...)`     | [io](io.md)                     | Format-string write to stdout. Verbs: `%d %f %s %t %v %%`; per-verb `\|key=value` modifiers (`pad`, `prec`, `base`, `null=*`, ...). |
| `readLine()`                  | [io](io.md)                     | Read one line from stdin (trailing newline stripped). Errors at EOF - check `eof()` first. |
| `readLine(prompt)`            | [io](io.md)                     | Same as `readLine()` but writes `prompt` to stdout first.                     |
| `repeat(s, n)`                | [strings](strings.md)           | `n` non-negative copies of `s` concatenated.                                  |
| `replace(s, old, new)`        | [strings](strings.md)           | Replace **all** occurrences of `old` in `s` with `new`.                       |
| `round(x)`                    | [math](math.md)                 | Round to nearest int (half away from zero).                                   |
| `split(s, sep)`               | [strings](strings.md)           | Split `s` on non-empty `sep`; returns `list of string`.                       |
| `sprintf(value)`              | [io](io.md)                     | Display-form of a value, returned as a string (doesn't write).                |
| `sprintf(format, args...)`    | [io](io.md)                     | Format-string version of `sprintf`. Same verbs and `\|key=value` modifiers as `printf`.   |
| `sqrt(x)`                     | [math](math.md)                 | Square root; always float. Errors on negative input.                          |
| `startsWith(s, prefix)`       | [strings](strings.md)           | True if `s` starts with `prefix`.                                             |
| `string(v)`                   | [convert](convert.md)           | Convert to string (always succeeds; uses the value's display form).           |
| `substring(s, start)`         | [strings](strings.md)           | Rune-indexed slice of `s` from `start` to end.                                |
| `substring(s, start, end)`    | [strings](strings.md)           | Rune-indexed slice; **exclusive** `end`.                                      |
| `trim(s)`                     | [strings](strings.md)           | Strip leading and trailing Unicode whitespace.                                |
| `trimLeft(s)`                 | [strings](strings.md)           | Strip leading whitespace.                                                     |
| `trimRight(s)`                | [strings](strings.md)           | Strip trailing whitespace.                                                    |
| `typeOf(v)`                   | [convert](convert.md)           | Runtime kind as string (`"int"`, `"float"`, `"string"`, `"bool"`, `"null"`, `"list"`, `"map"`). |
| `upper(s)`                    | [strings](strings.md)           | Uppercase `s` (Unicode-aware).                                                |

## Constants

| Name               | Library                         | Type     | Value                                                       |
|--------------------|---------------------------------|----------|-------------------------------------------------------------|
| `E`                | [math](math.md)                 | `float`  | Euler's number, 2.718281828459045.                          |
| `JENNIFER_VERSION` | [core](core.md) *(auto-loaded)* | `string` | The interpreter's build version (e.g. `"0.6.0"`).           |
| `os.JENNIFER_LF`   | [os](os.md)                     | `string` | Platform line ending. `"\n"` on Linux today.                |
| `os.JENNIFER_OS`   | [os](os.md)                     | `string` | OS tag (`"linux"` today); same value as `os.platform()`.    |
| `PI`               | [math](math.md)                 | `float`  | π, 3.141592653589793.                                       |

## Type-conversion calls

`int`, `float`, `string`, `bool` are also type keywords (used in `def x
as int`). The parser allows them in expression position **only** when
immediately followed by `(`, so `def x as int init int("42");` works
but `def x as int init int;` errors. See
[convert.md](convert.md#notes-on-the-type-name-syntax) for the parser
detail.

## See also

- [index.md](index.md) - library catalog with code samples and the
  organizing principles.
- Per-library reference pages: [io.md](io.md), [convert.md](convert.md),
  [math.md](math.md), [strings.md](strings.md), [core.md](core.md).
- [../user-guide/imports.md](../user-guide/imports.md) - how to import a
  library in a Jennifer source file.
