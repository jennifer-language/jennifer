# `io` - input/output

Enable with `use io;`. Today the library provides `printf` and `sprintf`. Both
share a Go-style format-string mini-language.

## `printf`

Writes formatted output to standard output. Three calling shapes:

```jennifer
printf("hi\n");                              # literal string (no verbs)
printf($x);                                  # single non-string value, displayed
printf("you are %d years old!\n", $age);     # format string + arguments
printf("%s = %d, ok=%t\n", "answer", 42, true);
```

## `sprintf`

Same arguments as `printf` but **returns** the formatted string instead of
writing it.

```jennifer
def msg as string init sprintf("%d + %d = %d", 1, 2, 3);
printf("%s\n", $msg);   # "1 + 2 = 3"
```

## Format verbs

| Verb | Required kind  | Notes                          |
|------|----------------|--------------------------------|
| `%d` | `int`          | decimal                        |
| `%f` | `float`        | shortest round-trip            |
| `%s` | `string`       | raw                            |
| `%t` | `bool`         | `true` / `false`               |
| `%v` | any            | uses the value's display form  |
| `%%` | -              | literal `%`                    |

Mismatches (wrong verb for the value kind, too few or too many args, dangling
`%`, unknown verb) all produce runtime errors.

**Escaping the meta-characters:**

- A literal `%` in any string passed to `printf`/`sprintf` must be
  doubled to `%%`.
- A literal `|` *immediately after a verb* must be doubled to `||`,
  because `|` otherwise starts a modifier list (see [Format
  modifiers](#format-modifiers)). The escape consumes one of the two
  `|`s; the other appears in the output. Pipes that don't touch a
  verb are normal characters and need no escaping:
  `printf("a|b %s||c|d\n", "X")` prints `a|b X|c|d` - the `||` after
  `%s` is the escape, while the `|`s in `a|b` and `c|d` sit between
  non-verb characters and pass through unchanged.

## Format modifiers

Each verb (except `%v`) accepts an optional pipe-separated modifier list:

```
%verb[|key=value]*
```

Modifiers are **order-independent flags** - `%d|pad=5|fill=0|align=right`
and `%d|align=right|fill=0|pad=5` produce the same output. The list runs
left-to-right until it hits a byte that isn't part of a key or value.
To put a literal `|` immediately after a verb, double it: `||` writes
one `|` and ends the modifier list (same shape as `%%`). Unknown keys,
bad values, missing companions (e.g. `group=` without `sep=`), and the
same key set twice are all runtime errors.

Evaluation order within one verb:

1. **Null check.** If the value is `null` *and* the spec includes a
   `null=` modifier, the verb-specific render is skipped and the
   configured replacement is used.
2. **Verb-specific render.** `mode`, `base`, `prec`, `sci`, `sign`,
   `group`/`sep`, `case` apply here.
3. **Layout.** `max` truncates (rune-aware), then `pad`+`fill`+`align`
   extends. Layout still applies to the null replacement, so columns
   line up.

### `null=` (shared by `%s`, `%d`, `%f`, `%t`)

| Form                | Output when value is `null`                                |
|---------------------|------------------------------------------------------------|
| `null=empty`        | `""`                                                       |
| `null=null`         | `"null"`                                                   |
| `null=literal("X")` | `X` - the quoted text, with Jennifer string escapes parsed |

Without a `null=` modifier, a `null` value is still a type-mismatch error
against any verb except `%v`. `null=` wins over every other modifier on
its verb: `%s|mode=quote|null=literal("X")` on a null prints `X`, not
`"X"`.

### `%s` modifiers

| Key      | Values                 | Default | Effect                                        |
|----------|------------------------|---------|-----------------------------------------------|
| `pad`    | non-negative integer   | -       | minimum rune width                            |
| `max`    | non-negative integer   | -       | truncate to N runes                           |
| `align`  | `left`, `right`        | `left`  | which side gets the pad spaces                |
| `mode`   | `raw`, `quote`, `escape` | `raw`   | wrap in `"..."` (`quote`) / show escapes (`escape`) |
| `null`   | see above              | -       | substitute when value is `null`               |

`mode=quote` wraps the string in double quotes and escapes interior
`\`, `"`, and control bytes. `mode=escape` does the same escaping
without the wrapping - useful for showing a string's structure in
debug output.

```jennifer
printf("[%s|pad=8|align=right]\n", "hi");        # [      hi]
printf("[%s|max=3]\n", "abcdef");                # [abc]
printf("%s|mode=quote\n", "a\nb");               # "a\nb"
```

### `%d` modifiers

| Key      | Values                            | Default    | Effect                                              |
|----------|-----------------------------------|------------|-----------------------------------------------------|
| `pad`    | non-negative integer              | -          | minimum width                                       |
| `fill`   | `0`                               | space      | zero-pad between sign and digits; requires `align=right` (the default) |
| `align`  | `left`, `right`                   | `right`    | which side gets the pad                             |
| `base`   | `2`, `8`, `10`, `16`              | `10`       | digit base; hex uses lowercase                      |
| `sign`   | `negative`, `always`, `space`     | `negative` | sign for non-negative values                        |
| `group`  | positive integer                  | -          | digit-group size, reading right-to-left             |
| `sep`    | one of `_`, `,`, `.`, `-`, `:`    | -          | group separator; required with `group=` and vice versa |
| `null`   | see above                         | -          | substitute when value is `null`                     |

```jennifer
printf("%d|base=2\n", 5);                                # 101
printf("%d|base=16|group=4|sep=_\n", 3735928559);        # dead_beef
printf("%d|pad=5|fill=0|sign=always\n", 42);             # +0042
```

### `%f` modifiers

| Key      | Values                        | Default    | Effect                                                       |
|----------|-------------------------------|------------|--------------------------------------------------------------|
| `prec`   | non-negative integer          | shortest   | fraction digits (or mantissa fraction digits when `sci=true`) |
| `trim`   | `true`, `false`               | `false`    | strip trailing fraction zeros and the `.` if all zero        |
| `sci`    | `true`, `false`               | `false`    | force scientific notation (`1.23e+03`)                       |
| `pad`    | non-negative integer          | -          | minimum width                                                |
| `align`  | `left`, `right`               | `right`    | which side gets the pad                                      |
| `sign`   | `negative`, `always`, `space` | `negative` | sign for non-negative values                                 |
| `null`   | see above                     | -          | substitute when value is `null`                              |

```jennifer
printf("%f|prec=2\n", 3.14159);              # 3.14
printf("%f|prec=4|trim=true\n", 3.0);        # 3
printf("%f|sci=true|prec=2\n", 0.00123);     # 1.23e-03
```

### `%t` modifiers

| Key      | Values                       | Default | Effect                                  |
|----------|------------------------------|---------|-----------------------------------------|
| `case`   | `lower`, `upper`, `title`    | `lower` | `true`/`false`, `TRUE`/`FALSE`, `True`/`False` |
| `null`   | see above                    | -       | substitute when value is `null`         |

### `%v` modifiers

`%v` takes no modifiers - it is deliberately the "I don't care, just
print it" verb. Use a typed verb plus modifiers when you want to shape
the output.

## Input from stdin

Three builtins for reading lines from standard input. They are
**output-symmetric** with `printf` / `sprintf` and intentionally
minimal - line at a time, with an explicit end-of-input predicate so
nothing happens implicitly.

### `readLine() -> string`

Read one line from stdin. The trailing `\r\n` or `\n` is stripped; the
returned string never carries a newline. Calling at end-of-input is a
positioned runtime error (`readLine: end of input`), so the caller
must check `eof()` first.

A final line that has no trailing newline is returned normally on the
call that reaches it; the subsequent call errors.

### `readLine(prompt) -> string`

Same as `readLine()` but writes `prompt` to stdout (no newline added)
before reading. The prompt is written **unconditionally**, even when
stdin is piped - explicit beats silently skipping the prompt off a
non-TTY.

```jennifer
def name as string init readLine("name: ");
printf("hi, %s\n", $name);
```

### `eof() -> bool`

True if and only if the next `readLine()` would error. Implemented by peeking one
byte through a buffered reader; the byte stays in the buffer for the
next read. Once true, `eof()` stays true for the rest of the run.

### Canonical loop

```jennifer
use io;
while (not eof()) {
    def line as string init readLine();
    printf("[%s]\n", $line);
}
```

This is the only pattern the language asks you to learn. There is no
`for line in stdin` shortcut, no `lines()` that slurps the whole
stream, and `readLine()` does not return a sentinel value at EOF -
they were considered and rejected because the existing trio is
already complete and adding parallels would violate Jennifer's "one
way per thing" stance.

### REPL limitation

The interactive REPL owns stdin via its line editor, so `readLine`
and `eof` both refuse inside the REPL with a clear error:

```
readLine: stdin is owned by the REPL editor
```

A proper side-channel for REPL input is a future milestone. To play
with the input functions today, put your program in a `.j` file and
run it with stdin piped or redirected: `jennifer run prog.j < input.txt`.

## Float display

Floats always display with a decimal point so the value's type stays visible:
`5.0` prints as `"5.0"`, not `"5"`. That matters most after the Python 3
division change - `4 / 2` is the float `2.0`, and you can tell at a glance
rather than wondering whether it's an `int`.

See also: [../user-guide/index.md](../user-guide/index.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
