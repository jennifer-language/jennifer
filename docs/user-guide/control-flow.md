# Control flow

## Operators

| Operator             | Meaning                                                  |
| -------------------- | -------------------------------------------------------- |
| `+`                  | addition (`int`/`float`); also concatenation on `string` |
| `-`, `*`             | subtraction, multiplication (`int`/`float`)              |
| `/`                  | **true division - always returns `float`**               |
| `//`                 | floor (integer) division; `int // int -> int`            |
| `%`                  | modulo (`int` only); **floored**, matching `//`          |
| unary `-`            | numeric negation (`int`/`float`)                         |
| `<`, `>`, `<=`, `>=` | order two numbers (exact across `int`/`float`) or two strings (lexicographic); `bool`  |
| `==`, `!=`           | equality / inequality; same-kind plus exact `int`/`float`; `bool` |
| `and`, `or`          | logical; both operands `bool`; short-circuit             |
| `not`                | unary logical negation; operand `bool` (there is no `!`) |
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

**`%` is floored**, consistent with `//`, so the identity
`(a // b) * b + (a % b) == a` holds for negative operands:
`-7 // 3 == -3` and `-7 % 3 == 2`; `7 % -3 == -2`. (This is Python's
convention, not C/Go truncation toward zero.)

**Integer overflow errors.** Integer arithmetic whose result does not
fit in a 64-bit int (`9223372036854775807 + 1`) is a positioned runtime
error rather than a silent wrap. A mixed `int`/`float` comparison is
**exact** - the int is not promoted to a lossy float - so a 64-bit int
never spuriously compares equal to a nearby float.

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

# for-each over a half-open range: 0..n yields 0, 1, ..., n-1.
# No list is built - the range is iterated lazily.
for (def i in 0..10) {
    io.printf("%d ", $i);
}
```

Conditions in `if`, `elseif`, `while`, and `for` **must be `bool`** - there
is no implicit truthiness. Use a comparison (`$x == 0`) to get a bool.
For-each (`for (def x in $coll)`) doesn't take a condition - it walks
the whole collection (or the whole range).

**A condition short-circuits.** `and` / `or` only evaluate their right operand
when the result is not already decided, so a call on the right side may never
run:

```jennifer
use io;
func printer() { io.printf("ran "); return true; }
if (true or printer()) { io.printf("a\n"); }    # prints "a" - printer() is skipped
if (false and printer()) { io.printf("b\n"); }  # prints nothing - printer() is skipped
if (true and printer()) { io.printf("c\n"); }   # prints "ran c" - printer() runs
```

Put the cheap or decisive test on the left, and don't rely on a side effect in a
short-circuited operand.

A range `lo..hi` is **half-open** (`[lo, hi)`): it includes `lo` and
excludes `hi`, so `0..n` runs exactly `n` times. Bounds are int; `lo >
hi` is an error and `lo == hi` is empty. The same `..` builds a list
(`def r as list of int init 1..5;` is `[1, 2, 3, 4]`) and slices a
collection (`$xs[a..b]`, covered in
[types-and-values](types-and-values.md)).

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

## `match` (multi-way value dispatch)

When a chain of `if` / `elseif` compares **one subject** against several values,
`match` says it once:

```jennifer
match ($cmd) {
    when "start" {
        start();
    }
    when "stop", "halt" {      # several values in one arm
        stop();
    }
    else {                     # optional default, must be last
        io.printf("unknown: %s\n", $cmd);
    }
}
```

- The **subject** is evaluated **once**, then compared to each `when` value by
  the strict `==` operator (the same exact, type-strict rules - an `int` subject
  never matches a `float` value). The **first** arm with a matching value runs;
  the rest are skipped.
- An arm lists **one or more values** (`when 2, 3, 4`), which is an *OR of
  equality* - the arm runs if the subject equals any of them. The values are
  evaluated left-to-right and **stop at the first match**, so a side-effecting
  value in a later position may not run.
- Values are **any expression**, not just literals - `when MAX`, `when $limit`,
  `when lo(), hi()`. A bare `when Name { ... }` reads `Name` as the value and the
  `{` as the arm block, so **parenthesize a composite-literal value**: a struct
  literal `when (Point{x: 1, y: 2}) { ... }` (required), and a map literal
  `when ({"a": 1}) { ... }` (optional, but the parens let `jennifer fmt` lay the
  arm out cleanly).
- **No fall-through.** Each arm is an independent block; there is no `break` to
  end an arm and nothing falls into the next. Because of that, **`match` is not a
  `break` target**: a `break` or `continue` inside an arm acts on the enclosing
  **loop**, never the match - which sidesteps C's "break breaks the switch" and
  "forgot the break" traps at once.

  ```jennifer
  for (def n in [1, 2, 3, 4]) {
      match ($n) {
          when 3 {
              break;     # breaks the for loop, not the match
          }
      }
      io.printf("%d ", $n);   # 1 2
  }
  ```

- `else` is the **optional** default and must come last. **No matching arm and
  no `else` is a well-defined no-op** (nothing runs), not an error.
- `match` is a **statement, not an expression** - arms act, assign, or `return`,
  the same as `if`. Each arm's block is its own scope.

`jennifer fmt` lays a `match` out as a flat list of arms - like a `switch` /
`case` in other languages: the subject on the opening line, then each `when`
(and the `else`) starting its **own** line at the arm indent with its body on
its own indented lines. Arms do **not** cuddle the previous arm's `}` (unlike an
`if`'s `} else {`), so you can scan the `when` column top to bottom. A long
`when` value list wraps with each continuation value aligned under the first.

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

### Matching an enum (variant patterns)

When the subject of a `match` is a variable or parameter whose type is an
[enum](types-and-values.md#enums-sum-types), the arms are **variant patterns**
instead of values. Each arm names a variant and, optionally, binds its payload:

```jennifer
def enum Shape { Circle { r as float }, Rect { w as float, h as float }, Empty };

func describe(s as Shape) {
    match ($s) {
        when Circle(c) { return "circle radius " + convert.toString($c.r); }
        when Rect(rc)  { return "rectangle"; }   # binder optional
        when Empty     { return "nothing"; }     # payload-less variant
    }
    return "?";
}
```

`when Circle(c)` binds the variant's payload into a fresh `$c` (a mini-struct
with that variant's fields) for that arm only. A pattern `match` must be
**exhaustive** - cover every variant or add an `else` - and a missing variant is
a **compile-time error**, so adding a variant later flags every `match` you need
to revisit. (A `match` over an ordinary value like an `int` or `string` stays
non-exhaustive, as above.) `break` / `continue` in an arm still act on the
enclosing loop, never the `match`.

The check applies wherever the subject's enum type is known: in the same file,
inside a `spawn` body, and for an enum imported from a module. A same-file match
is checked when the file is parsed (so `jennifer lint` reports it); one over an
imported enum is checked when the program loads, since the module has to be read
first.

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

On Unix a process exit status is only 8 bits, so the code a shell or
parent process observes is `EXPR & 0xff`: `exit 256` is seen as `0` and
`exit 300` as `44`. Stick to the `0..255` range for an exit code a
caller can actually read back.

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
  bytes-element range violations, and the rest of the positioned
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

### No `finally`

Jennifer has no `finally` clause, and won't - `defer` (below) covers the same
need without `finally`'s footguns.

## `defer` (deterministic cleanup)

`defer CALL(args);` schedules a single call to run when the **enclosing block**
exits - on **every** exit path: normal fall-through, `return`, `break`,
`continue`, a `throw` that unwinds through it, and `exit`. It is the clean way to
pair "acquire a resource" with "release it", right next to each other:

```jennifer
use fs;
use io;

func dumpFirst(path as string) {
    def f as fs.File init fs.open($path, "read");
    defer fs.close($f);                 # runs however this function exits

    if (fs.eof($f)) { return; }         # <- fs.close($f) runs here
    io.printf("%s\n", fs.readLine($f)); # <- and here, on normal exit
}
```

Rules:

- **The deferred thing must be a call** - a method call (`defer cleanup();`) or a
  namespaced / module call (`defer fs.close($f);`). A non-call
  (`defer 1 + 2;`) is a parse error. Because it is a single call, not a block,
  there is no way to smuggle a `return` or `throw` into the cleanup.
- **Arguments are evaluated at the `defer` line; the call runs at block exit.**
  `defer io.printf("done %s\n", $path);` captures `$path`'s value now.
- **LIFO.** Several defers in one block run last-registered-first, so resources
  release in reverse acquisition order:

  ```jennifer
  def conn as net.Conn init net.connect("db.local:6379");
  defer net.close($conn);                # released last
  def f as fs.File init fs.open("query.log", "write");
  defer fs.close($f);                    # released first
  ```

- **Block-scoped.** A `defer` runs at the end of its enclosing `{ }`, so one
  inside a loop body runs at the end of **each iteration** (no pile-up):

  ```jennifer
  for (def name in $files) {
      def f as fs.File init fs.open($name, "read");
      defer fs.close($f);                # closes at the end of this iteration
      process($f);
  }
  ```

  A top-level `defer` runs at program end.
- **Does not cross the method or `spawn` boundary** (like `break` / `return`): a
  method's defers run when that method returns, not in its caller.
- A deferred call that **throws** propagates and is catchable; if the block was
  already unwinding an error, the deferred error supersedes it. An `exit` is
  never superseded - defers still run, but the exit code stands.

For durability specifically (flushing to disk), `defer` the `close` but call
`fs.sync` explicitly and check it - see [fs](../libraries/fs.md); a durability
failure should surface before you tell the user "done", not from a deferred call
running on the way out.

## `errdefer` (undo on error only)

`errdefer CALL(args);` is `defer`'s error-path sibling: same single-call form,
same argument snapshot at the `errdefer` line, same LIFO stack - but the call
runs **only when the enclosing block exits with a propagating error** (a `throw`
or a runtime error). On fall-through, `return`, `break`, `continue`, and `exit`
it is skipped.

Reach for it when success must **keep** the resource and only failure should
release it - the classic connect-then-handshake, where a plain `defer` would
close the very connection you mean to hand to the caller:

```jennifer
func connect(addr as string) {
    def c as net.Conn init net.connect($addr);
    errdefer net.close($c);        # a failed handshake must not leak the socket
    handshake($c);                 # may throw partway through
    return Session{conn: $c};      # success: the caller owns the open conn
}
```

Rules beyond `defer`'s:

- **One teardown stack.** `defer` and `errdefer` in the same block run in one
  last-registered-first pass; on an error exit both kinds fire, on a normal exit
  the `errdefer` entries are skipped.
- **A failing defer arms the errdefers.** If a plain deferred call throws during
  teardown, the block is now exiting with an error - `errdefer` entries later in
  the teardown (registered earlier) do run.
- **`exit` is not an error.** A deliberate `exit` runs plain defers but skips
  errdefers, and the exit code stands.
- **Keep the undo call unlikely to throw.** An errdefer only ever runs while an
  error is already propagating, and if the undo call *itself* throws, its error
  supersedes the original (the same rule as `defer`) - the catch handler then
  sees the cleanup failure, not the root cause. A close on a handle you own is
  fine; anything that can plausibly fail belongs in explicit error handling.
- **Block-scoped, like `defer` - and unlike Zig.** An `errdefer` registered
  inside an `if` (or any inner block) is resolved when *that block* exits: leave
  the `if` normally and the errdefer is gone, even if the function throws two
  lines later. Register the errdefer in the same block that should own the
  undo - for the connect pattern, the function body, right after the acquire:

  ```jennifer
  def c as net.Conn init net.connect($addr);
  errdefer net.close($c);            # function-body scope: armed until return
  if ($opts.tls) {
      $c = net.startTLS($c);         # NOT here - this block exits right away
  }
  handshake($c);
  ```

In the REPL, each input is its own frame (as with `defer`): an input that errors
runs its errdefers, an input that succeeds discards them - they do not carry
over to later inputs.

If cleanup must happen on *every* path (a temp file, a one-shot request's
socket), use `defer`; `errdefer` is only for the acquire-or-undo shape.

