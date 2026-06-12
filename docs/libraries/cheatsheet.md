# Cheatsheet - all builtins at a glance

Alphabetical index of every standard-library function and constant. Use
it when you know the *name* and want to know *which library* and *how to
call it*; use each library's own page when you want to read about a
topic.

The table covers what ships with the interpreter today. New
entries land here at the same time as the per-library doc - it's a
flat lookup view, not authoritative.

## Functions

| Call                               | Library                         | What it does                                                                                                                        |
| ---------------------------------- | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `math.abs(x)`                      | [math](math.md)                 | Absolute value of `x` (int→int, float→float).                                                                                       |
| `convert.toBool(v)`                | [convert](convert.md)           | Canonical conversion to `bool` (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`).                                                          |
| `math.ceil(x)`                     | [math](math.md)                 | Smallest int ≥ `x`. Accepts int (identity) or float.                                                                                |
| `strings.chars(s)`                 | [strings](strings.md)           | Split `s` into a `list of string`, one entry per Unicode code point.                                                                |
| `strings.contains(s, sub)`         | [strings](strings.md)           | True if `s` contains the substring `sub`.                                                                                           |
| `strings.endsWith(s, suffix)`      | [strings](strings.md)           | True if `s` ends with `suffix`.                                                                                                     |
| `io.eof()`                         | [io](io.md)                     | True if and only if the next `io.readLine()` would error. Pair with `while (not io.eof()) {...}`.                                   |
| `convert.toFloat(v)`               | [convert](convert.md)           | Convert to float (int→float, float identity, string parses, bool→1.0/0.0).                                                          |
| `math.floor(x)`                    | [math](math.md)                 | Largest int ≤ `x`. Accepts int (identity) or float.                                                                                 |
| `strings.indexOf(s, sub)`          | [strings](strings.md)           | Rune index of first `sub` in `s`, or `-1` if absent.                                                                                |
| `convert.toInt(v)`                 | [convert](convert.md)           | Convert to int (float truncates toward zero, string parses, bool→1/0).                                                              |
| `strings.join(parts, sep)`         | [strings](strings.md)           | Concatenate `list of string` `parts` separated by `sep`. Inverse of `strings.split`.                                                |
| `len(v)`                           | [core](core.md) *(auto-loaded)* | Structural length: rune count (string), element count (list), entry count (map).                                                    |
| `lists.concat(a, b)`               | [lists](lists.md)               | New list with `a`'s elements followed by `b`'s.                                                                                     |
| `lists.contains(xs, item)`         | [lists](lists.md)               | True if `item` appears in `xs` (haystack, needle).                                                                                  |
| `lists.first(xs)`                  | [lists](lists.md)               | Element at index 0. Empty input errors.                                                                                             |
| `lists.head(xs, n)`                | [lists](lists.md)               | New list of the first `n` elements.                                                                                                 |
| `lists.last(xs)`                   | [lists](lists.md)               | Element at the last index. Empty input errors.                                                                                      |
| `lists.pop(xs)`                    | [lists](lists.md)               | New list without the last element. Empty input errors.                                                                              |
| `lists.push(xs, item)`             | [lists](lists.md)               | New list with `item` appended.                                                                                                      |
| `lists.reverse(xs)`                | [lists](lists.md)               | New list with elements reversed.                                                                                                    |
| `lists.slice(xs, start[, end])`    | [lists](lists.md)               | New sublist `[start, end)`; `end` defaults to `len(xs)`.                                                                            |
| `lists.sort(xs)`                   | [lists](lists.md)               | New ascending-sorted list. Numeric / string / bool elements; mixed errors.                                                          |
| `lists.tail(xs, n)`                | [lists](lists.md)               | New list of the last `n` elements.                                                                                                  |
| `strings.lower(s)`                 | [strings](strings.md)           | Lowercase `s` (Unicode-aware).                                                                                                      |
| `maps.delete(m, key)`              | [maps](maps.md)                 | New map without `key`. Missing key errors (strict at boundaries).                                                                   |
| `maps.has(m, key)`                 | [maps](maps.md)                 | True if map `m` contains `key`. The non-erroring companion to `$m[key]`.                                                            |
| `maps.keys(m)`                     | [maps](maps.md)                 | List of keys in insertion order.                                                                                                    |
| `maps.merge(a, b)`                 | [maps](maps.md)                 | New map; `b`'s entries layered on top of `a`.                                                                                       |
| `maps.values(m)`                   | [maps](maps.md)                 | List of values in insertion order.                                                                                                  |
| `math.max(a, b)`                   | [math](math.md)                 | Larger of two numbers; mixed int/float promotes to float.                                                                           |
| `math.min(a, b)`                   | [math](math.md)                 | Smaller of two numbers; mixed int/float promotes to float.                                                                          |
| `os.getEnv(name)`                  | [os](os.md)                     | Read environment variable `name`. Unset → empty string, no error.                                                                   |
| `os.platform()`                    | [os](os.md)                     | Operating-system name as reported by the runtime (`"linux"` today).                                                                 |
| `math.pow(x, y)`                   | [math](math.md)                 | `x` raised to `y`; always float. Errors on NaN/Inf-producing inputs.                                                                |
| `io.printf(value)`                 | [io](io.md)                     | Write a value's display form to stdout.                                                                                             |
| `io.printf(format, args...)`       | [io](io.md)                     | Format-string write to stdout. Verbs: `%d %f %s %t %v %%`; per-verb `\|key=value` modifiers (`pad`, `prec`, `base`, `null=*`, ...). |
| `io.readLine()`                    | [io](io.md)                     | Read one line from stdin (trailing newline stripped). Errors at EOF - check `io.eof()` first.                                       |
| `io.readLine(prompt)`              | [io](io.md)                     | Same as `io.readLine()` but writes `prompt` to stdout first.                                                                        |
| `strings.repeat(s, n)`             | [strings](strings.md)           | `n` non-negative copies of `s` concatenated.                                                                                        |
| `strings.replace(s, old, new)`     | [strings](strings.md)           | Replace **all** occurrences of `old` in `s` with `new`.                                                                             |
| `math.round(x)`                    | [math](math.md)                 | Round to nearest int (half away from zero).                                                                                         |
| `strings.split(s, sep)`            | [strings](strings.md)           | Split `s` on non-empty `sep`; returns `list of string`.                                                                             |
| `io.sprintf(value)`                | [io](io.md)                     | Display-form of a value, returned as a string (doesn't write).                                                                      |
| `io.sprintf(format, args...)`      | [io](io.md)                     | Format-string version of `sprintf`. Same verbs and `\|key=value` modifiers as `printf`.                                             |
| `math.sqrt(x)`                     | [math](math.md)                 | Square root; always float. Errors on negative input.                                                                                |
| `strings.startsWith(s, prefix)`    | [strings](strings.md)           | True if `s` starts with `prefix`.                                                                                                   |
| `convert.toString(v)`              | [convert](convert.md)           | Convert to string (always succeeds; uses the value's display form).                                                                 |
| `strings.substring(s, start)`      | [strings](strings.md)           | Rune-indexed slice of `s` from `start` to end.                                                                                      |
| `strings.substring(s, start, end)` | [strings](strings.md)           | Rune-indexed slice; **exclusive** `end`.                                                                                            |
| `strings.trim(s)`                  | [strings](strings.md)           | Strip leading and trailing Unicode whitespace.                                                                                      |
| `strings.trimLeft(s)`              | [strings](strings.md)           | Strip leading whitespace.                                                                                                           |
| `strings.trimRight(s)`             | [strings](strings.md)           | Strip trailing whitespace.                                                                                                          |
| `convert.typeOf(v)`                | [convert](convert.md)           | Runtime kind as string (`"int"`, `"float"`, `"string"`, `"bool"`, `"null"`, `"list"`, `"map"`).                                     |
| `strings.upper(s)`                 | [strings](strings.md)           | Uppercase `s` (Unicode-aware).                                                                                                      |

## Constants

| Name               | Library                         | Type     | Value                                                    |
| ------------------ | ------------------------------- | -------- | -------------------------------------------------------- |
| `E`                | [math](math.md)                 | `float`  | Euler's number, 2.718281828459045.                       |
| `JENNIFER_VERSION` | [core](core.md) *(auto-loaded)* | `string` | The interpreter's build version (e.g. `"0.6.0"`).        |
| `os.JENNIFER_LF`   | [os](os.md)                     | `string` | Platform line ending. `"\n"` on Linux today.             |
| `os.JENNIFER_OS`   | [os](os.md)                     | `string` | OS tag (`"linux"` today); same value as `os.platform()`. |
| `PI`               | [math](math.md)                 | `float`  | π, 3.141592653589793.                                    |

## Type-conversion calls

`int`, `float`, `string`, `bool` are also type keywords (used in `def x
as int`). The parser allows them in expression position **only** when
immediately followed by `(`, so `def x as int init convert.toInt("42");` works
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
