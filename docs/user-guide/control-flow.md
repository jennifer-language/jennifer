# Control flow

## Operators

| Operator             | Meaning                                                  |
| -------------------- | -------------------------------------------------------- |
| `+`                  | addition (`int`/`float`); also concatenation on `string` |
| `-`, `*`             | subtraction, multiplication (`int`/`float`)              |
| `/`                  | **true division - always returns `float`**               |
| `//`                 | floor (integer) division; `int // int -> int`            |
| `%`                  | modulo (`int` only)                                      |
| unary `-`            | numeric negation (`int`/`float`)                         |
| `<`, `>`, `<=`, `>=` | numeric comparison; result is `bool`                     |
| `==`                 | equality; same-kind plus `int`/`float` promotion; `bool` |
| `and`, `or`          | logical; both operands `bool`; short-circuit             |
| `not`                | unary logical negation; operand `bool`                   |
| `&`, `|`, `^`        | bitwise AND / OR / XOR on `int`                          |
| `<<`, `>>`           | left / arithmetic right shift on `int`                   |
| unary `~`            | bitwise NOT on `int` (`~x == -x - 1`)                    |

**Division has two operators.** `/` always returns a `float` (Python 3
style). `//` returns the floor, keeping the type when both operands are
ints:

```jennifer
5 / 2          # 2.5 (float)
5 // 2         # 2   (int)
5.0 / 2.0      # 2.5 (float)
5.7 // 2.0     # 2.0 (float - floor of a float division)
```

So `def x as int init 5 / 2;` is rejected (right side is float). Use
`5 // 2` for an int result, or `def x as float init 5 / 2;`.

(Line comments are `#`, freeing `//` for the Python-3 floor-division
operator. The `#` choice also lets Jennifer files start with a shebang:
`#!/usr/bin/env -S jennifer run`.)

Precedence (low to high): `or`, `and`, `not`, comparison, bitwise `|`,
bitwise `^`, bitwise `&`, shifts `<< >>`, additive (`+`, `-`),
multiplicative (`*`, `/`, `//`, `%`), unary `-` / `~`. Use parentheses
to override: `(1 + 2) * 3`. The bit-op rungs sit between
comparison and additive following Python's precedence, so
`$x & 0xff == 0` parses as `($x & 0xff) == 0` (the intuitive
interpretation), not the C/Go shape `$x & (0xff == 0)`. Examples that
follow the rules:

```jennifer
not 1 == 2                  # not (1 == 2) -> true
1 > 0 and 2 > 1             # true
true or false and false     # true or (false and false) -> true
-3 + 10                     # (-3) + 10 -> 7
-3 * 2                      # (-3) * 2 -> -6
```

**`and` and `or` short-circuit.** The right operand is only evaluated when
the left doesn't already decide the result. That matters when the right side
has side effects:

```jennifer
def gate as bool init false;
def result as bool init $gate and expensive();   # expensive() not called
```

Mixed `int`/`float` arithmetic promotes the int to float and the result is a
float (`3 + 0.5` -> `3.5`). **`/` always returns `float`**, even with two
int operands (`5 / 2` is `2.5`, not `2`). Use `//` when you want an
integer quotient: `5 // 2` is `2`. This is Python-3 division, not C/Java
division.

### Bitwise operators

The bit operators take `int` operands only - float is rejected with a
positioned error. The shifts are arithmetic (sign-extending `>>`); a
negative shift count is rejected, and a count >= 64 saturates to 0 or
-1 the way hardware does. Non-decimal literals (`0xff`, `0o755`,
`0b1010_0110`) and the `_` digit separator (`1_000_000`,
`0xDEAD_BEEF`) make bit-twiddling code much easier to read.

```jennifer
def mask as int init 0xff;
def x as int init 0xDEAD_BEEF;

io.printf("low byte:  %d|base=16\n", $x & $mask);   # ef
io.printf("flip last: %d|base=16\n", $x ^ 1);       # dead_beee
io.printf("shift 4:   %d|base=16\n", $x >> 4);      # dead_beef >> 4
```

## Conditionals and loops

```jennifer
if ($n == 0) {
    io.printf("zero");
} elseif ($n < 10) {
    io.printf("small");
} else {
    io.printf("large");
}

while ($i < 5) {
    $i = $i + 1;
}

for (def i as int init 0; $i < 10; $i = $i + 1) {
    io.printf($i);
}

# for-each over a list or map.
for (def x in $xs) {
    io.printf("%d ", $x);
}
for (def k in $m) {
    io.printf("%s=%d ", $k, $m[$k]);
}
```

Conditions in `if`, `elseif`, `while`, and `for` **must be `bool`** - there
is no implicit truthiness. Use a comparison (`$x == 0`) to get a bool.
For-each (`for (def x in $coll)`) doesn't take a condition - it walks
the whole collection.

### Loop variable scope

C-style `for` opens its own scope. **Where you `def` the iterator
variable decides whether you can still see it after the loop.**

```jennifer
# Loop-local: declare inside the for-init. The iterator lives only for
# the duration of the loop.
for (def i as int init 0; $i < 10; $i = $i + 1) {
    io.printf("%d\n", $i);
}
io.printf("%d\n", $i);   # ERROR: `i` not in scope here
```

```jennifer
# Outer-scope: declare in the surrounding scope, assign in the for-init.
# The variable survives past the loop and holds the value that made the
# condition false (10 here).
def i as int;
for ($i = 0; $i < 10; $i = $i + 1) {
    io.printf("%d\n", $i);
}
io.printf("%d\n", $i);   # ok - prints 10
```

The loop-local form is the [recommended style](style-guide.md#loops);
reach for the outer-scope form only when you actually need to inspect
the iterator after the loop ends. For-each (`for (def x in $coll)`)
is always loop-local - the iteration variable lives in a fresh scope
each pass through the loop and is gone once the loop exits.

## `repeat ... until` (post-test loop)

For loops that should run **at least once**, then keep going until a
condition becomes true:

```jennifer
def n as int init 0;
repeat {
    io.printf("n=%d\n", $n);
    $n = $n + 1;
} until ($n >= 3);
# prints n=0, n=1, n=2 - the body runs three times before until is true.
```

The body runs unconditionally on entry, then `until (cond)` is checked
**after** each iteration. The loop stops when `cond` evaluates true.

This is the post-test counterpart to `while`. The keyword pair
`repeat`/`until` was chosen over `do { } while ...` so the condition
inversion ("loop until done") reads as English and matches the rest of
Jennifer's word-operator style (`and`, `or`, `not`). Like every other
condition slot, `cond` must be `bool`.

## `break` and `continue`

`break;` exits the **innermost** enclosing loop:

```jennifer
for (def i as int init 0; $i < 10; $i = $i + 1) {
    if ($i == 5) { break; }
    io.printf("%d ", $i);
}
# prints "0 1 2 3 4 "
```

`continue;` skips the rest of the current iteration and starts the
next one. In a C-style `for` loop, the step expression (`$i = $i + 1`)
still runs before the condition is re-checked - matching the behaviour
in C, Go, Java, and Python:

```jennifer
for (def i as int init 0; $i < 5; $i = $i + 1) {
    if ($i % 2 == 0) { continue; }
    io.printf("%d ", $i);
}
# prints "1 3 "
```

Both work in `while`, C-style `for`, for-each (`for (def x in $coll)`),
and `repeat ... until`. In `repeat`, `continue` jumps to the `until`
check (skipping the rest of the body); the loop still terminates
normally when `until` becomes true.

Misuse:

- `break` and `continue` **only exist inside a loop**. Using one at
  the top level or as a stray statement in a method body that has no
  enclosing loop is a positioned runtime error.
- They do **not** cross the method-call boundary. A `break` inside a
  method body looks for a loop in **that method**, not in the caller.
  If the called method has no loop, the `break` errors.
- They only catch the **innermost** loop. To exit several levels at
  once, use a flag variable that the outer loop checks, or refactor
  the inner work into a method that `return`s when done.

## `exit`

`exit;` terminates the whole program immediately - it skips the rest
of the current method, every caller frame, and every remaining
top-level statement. The bare form yields exit code 0:

```jennifer
use io;
io.printf("ok\n");
exit;                   # process ends with code 0
io.printf("never\n");   # not reached
```

`exit EXPR;` sets the exit code; `EXPR` must evaluate to `int`:

```jennifer
use io;
io.printf("error: input missing\n");
exit 2;                 # process ends with code 2
```

`exit` is **distinct from `return`**. `return` ends the current
method's body and yields a value to the caller; `exit` ends the
program. Use `return` when a method has done its job; use `exit` when
the whole run is over.

## `try`, `catch`, `throw`

Catchable errors. `throw EXPR;` signals an error from any reachable
point; `try { body } catch (NAME) { handler }` runs the body and, if
anything inside it throws (user code or a runtime failure like
out-of-bounds), runs the handler with `NAME` bound to the thrown
value:

```jennifer
use io;

try {
    def n as int init convert.toInt($input);
    process($n);
} catch (err) {
    io.printf("not a number: %s\n", $err.message);
}
```

### What can be thrown

Any value. The **convention** is an `Error` struct - the runtime
auto-defines that struct shape so user code can rely on it without a
`def struct Error { ... };` of its own:

```jennifer
def struct Error {
    kind    as string,    # short symbolic tag
    message as string,    # human-readable
    file    as string,
    line    as int,
    col     as int,
};
```

User code throws an `Error{...}` to signal expected failure modes;
catch sites dispatch on `$err.kind`:

```jennifer
func parseConfig(src as string) {
    if (not strings.contains($src, "=")) {
        throw Error{
            kind: "parse_error",
            message: "missing `=`",
            file: "", line: 0, col: 0
        };
    }
    # ... happy path ...
}

try {
    parseConfig($cfg);
} catch (err) {
    if ($err.kind == "parse_error") {
        io.printf("config invalid: %s\n", $err.message);
    } else {
        throw $err;     # not our concern; let it propagate
    }
}
```

A bare `throw "boom";` still works (any value); the catch handler just
won't be able to read `.kind` / `.message` off it. Use
`convert.typeOf($err)` if you need to branch on the kind.

### What can be caught

- **User-issued `throw EXPR;`** - whatever the user passed, copied
  into the catch binding (value semantics, like every other binding
  boundary).
- **Runtime errors** - out-of-bounds reads / writes, missing map
  keys, type mismatches, division by zero, undefined names,
  bytes-element range violations, and the rest of today's positioned
  runtime errors. The runtime wraps them into the canonical `Error`
  struct with `kind = "runtime"` (more specific tags will land per
  site over time) and the original file / line / col preserved.

### What can NOT be caught

- **`exit` / `exit EXPR;`** - the program-level escape hatch stays
  escape. `try { exit 1; } catch (e) { ... }` lets the exit through;
  the catch block does not run.
- **`return` / `break` / `continue`** - they're control flow, not
  errors. `try { break; } catch (e) { ... }` breaks the enclosing
  loop; the handler does not run.

### Re-throwing

`throw $err;` inside a catch re-raises - the value propagates past
the current `try`/`catch` to the next enclosing `try`. Same value
unless replaced.

### No `finally` in v1

Jennifer does not have a `finally` clause yet. The pattern is
"do the cleanup explicitly in both branches" until a real cleanup
need surfaces (probably with file handles in a future `fs`
library).

