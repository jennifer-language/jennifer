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

```text
5 / 2          # 2.5 (float)
5 // 2         # 2   (int)
5.0 / 2.0      # 2.5 (float)
5.7 // 2.0     # 2.0 (float - floor of a float division)
```

So `def x as int init 5 / 2;` is rejected (right side is float). Use
`5 // 2` for an int result, or `def x as float init 5 / 2;`.

**`%` is floored**, consistent with `//`, so the identity
`(a // b) * b + (a % b) == a` holds for negative operands:
`-7 // 3 == -3` and `-7 % 3 == 2`; `7 % -3 == -2`. This is Python's
convention, not C/Go truncation toward zero.

**Integer overflow errors.** Integer arithmetic whose result does not
fit in a 64-bit int (`9223372036854775807 + 1`) is a positioned runtime
error, not a silent wrap. A mixed `int`/`float` comparison is
**exact** - the int is not promoted to a lossy float - so a 64-bit int
never spuriously compares equal to a nearby float.

Line comments are `#`, which frees `//` for floor division and lets a
Jennifer file start with a shebang: `#!/usr/bin/env -S jennifer run`.

Precedence (low to high): `or`, `and`, `not`, comparison, bitwise `|`,
bitwise `^`, bitwise `&`, shifts `<< >>`, additive (`+`, `-`),
multiplicative (`*`, `/`, `//`, `%`), unary `-` / `~`. Use parentheses
to override: `(1 + 2) * 3`. As in Python, the bit operators sit between
comparison and additive, so `$x & 0xff == 0` parses as `($x & 0xff) == 0`,
not the C/Go shape `$x & (0xff == 0)`:

```text
not 1 == 2                  # not (1 == 2) -> true
1 > 0 and 2 > 1             # true
true or false and false     # true or (false and false) -> true
-3 + 10                     # (-3) + 10 -> 7
-3 * 2                      # (-3) * 2 -> -6
```

**`and` and `or` short-circuit.** The right operand is evaluated only when
the left does not decide the result:

```jennifer
def gate as bool init false;
def result as bool init $gate and expensive();   # expensive() not called
```

Mixed `int`/`float` arithmetic promotes the int to float and the result is a
float (`3 + 0.5` -> `3.5`).

### Bitwise operators

The bit operators take `int` operands only - float is rejected with a
positioned error. `>>` is an arithmetic shift: it fills from the left
with the sign bit, so `-8 >> 1` is `-4`; there is no logical right
shift. `<<` drops the bits it shifts out, the sign bit included, so
`3 << 62` is negative and `1 << 63` is the smallest `int`: shifts are
bit operations, and unlike `+` and `*` they never raise an overflow
error. A negative shift count is rejected, and a count of 64 or more
gives `0` (`-1` for `>>` of a negative number), as in Go (x86 hardware
would use only the count's low six bits). Non-decimal literals (`0xff`,
`0o755`, `0b1010_0110`) and the `_` digit separator (`1_000_000`,
`0xDEAD_BEEF`) keep bit-twiddling code readable.

```jennifer
def mask as int init 0xff;
def x as int init 0xDEAD_BEEF;

io.printf("low byte:  %d|base=16\n", $x & $mask);   # ef
io.printf("flip last: %d|base=16\n", $x ^ 1);       # deadbeee
io.printf("shift 4:   %d|base=16\n", $x >> 4);      # deadbee
```

## Conditionals and loops

```jennifer
def n as int init 7;
def xs as list of int init [3, 1, 2];
def m as map of string to int init {"a": 1, "b": 2};
def data as bytes init encoding.encode("utf-8", "hi");

if ($n == 0) {
    io.printf("zero");
} elseif ($n < 10) {
    io.printf("small");
} else {
    io.printf("large");
}

def count as int init 0;
while ($count < 5) {
    $count = $count + 1;
}

for (def i as int init 0; $i < 10; $i = $i + 1) {
    io.printf("%d\n", $i);
}

# for-each over a list or map.
for (def x in $xs) {
    io.printf("%d ", $x);
}
for (def k in $m) {
    io.printf("%s=%d ", $k, $m[$k]);
}

# for-each over bytes: each byte as an int, 0 to 255.
for (def b in $data) {
    io.printf("%d ", $b);
}

# for-each over a half-open range: 0..n yields 0, 1, ..., n-1.
# No list is built - the range is iterated lazily.
for (def i in 0..10) {
    io.printf("%d ", $i);
}
```

Conditions in `if`, `elseif`, `while`, and `for` **must be `bool`** - there
is no implicit truthiness. Use a comparison (`$x == 0`) to get a bool.
For-each (`for (def x in $coll)`) takes no condition: it walks the whole
collection (or range), a copy taken when the loop starts, so writes to the
collection in the body do not change later iterations. A string is not a for-each source: say what to walk -
`strings.chars($s)` (code points) or `encoding.encode("utf-8", $s)`
(bytes).

**A condition short-circuits** (see `and` / `or` above), so a call on the right
may never run:

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

The loop-local form is the [recommended style](style-guide.md#loops); use
the outer-scope form only to inspect the iterator after the loop. For-each
(`for (def x in $coll)`) is always loop-local: the iteration variable lives
in a fresh scope each pass and is gone once the loop exits.

## `repeat ... until` (post-test loop)

A loop that runs **at least once**, then repeats until a condition is true:

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

It is the post-test counterpart to `while`. `repeat`/`until` instead of
`do { } while ...` makes the condition read as English ("loop until done"),
like the word operators `and`, `or`, `not`. `cond` must be `bool`.

## `match` (multi-way value dispatch)

When a chain of `if` / `elseif` compares **one subject** against several values,
`match` says it once:

```jennifer
func dispatch(cmd as string) {
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
  **loop**, never the match, which avoids C's "break breaks the switch" and
  "forgot the break" traps.

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

`jennifer fmt` lays a `match` out as a flat list of arms, like a `switch` /
`case`: the subject on the opening line, then each `when` (and the `else`)
on its **own** line at the arm indent, its body on indented lines below. Arms do **not** cuddle the previous arm's `}` (unlike an
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
still runs before the condition is re-checked, as in C, Go and Java:

```jennifer
for (def i as int init 0; $i < 5; $i = $i + 1) {
    if ($i % 2 == 0) { continue; }
    io.printf("%d ", $i);
}
# prints "1 3 "
```

Both work in `while`, C-style `for`, for-each (`for (def x in $coll)`),
and `repeat ... until`. In `repeat`, `continue` jumps to the `until`
check, skipping the rest of the body.

Misuse:

- `break` and `continue` **only exist inside a loop**. One at the top level
  or in a function body with no enclosing loop is a positioned runtime error.
- They do **not** cross the function-call boundary. A `break` inside a
  function body looks for a loop in **that function**, not in the caller.
  If the called function has no loop, the `break` errors.
- They only catch the **innermost** loop. To exit several levels at
  once, use a flag variable that the outer loop checks, or refactor
  the inner work into a function that `return`s when done.

### Matching an enum (variant patterns)

When the subject of a `match` is a variable or parameter whose type is an
[enum](types-and-values.md#enums-sum-types), the arms are **variant patterns**
instead of values. Each arm names a variant and, optionally, binds its payload:

```jennifer
def enum Shape { Circle{r as float}, Rect{w as float, h as float}, Empty };

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
to revisit. A `match` over an ordinary value like an `int` or `string` need
not be exhaustive. `break` / `continue` in an arm still act on the
enclosing loop, never the `match`.

The check applies wherever the subject's enum type is known: in the same file,
inside a `spawn` body, and for an enum imported from a module. A same-file match
is checked when the file is parsed (so `jennifer lint` reports it); one over an
imported enum is checked when the program loads, since the module has to be read
first.

The subject may also reach the enum through declared types: a field of a local
struct (`$doc.root`), an element of a `list of Shape` (`$queue[$head]`), a value
of a `map of K to Shape` (`$byName["x"]`), or a chain of these
(`$grid[$i][$j]`, `$byName["x"].tree`). It is evaluated once. A call result
(`next()[0]`) or a field of another module's struct has no type the parser can
see, so a pattern arm over one is refused; assign it to a typed variable first
(`def s as Shape init next()[0];` then `match ($s)`).

## `exit`

`exit;` ends the program immediately, skipping the rest of the current
function, every caller frame and every remaining top-level statement. The
bare form yields exit code 0:

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
caller can read back.

`exit` is **distinct from `return`**: `return` ends the current function
and yields a value to the caller; `exit` ends the program.

## `try`, `catch`, `throw`

Catchable errors. `throw EXPR;` signals an error from any reachable
point; `try { body } catch (NAME) { handler }` runs the body and, if
anything inside it throws (user code or a runtime failure like
out-of-bounds), runs the handler with `NAME` bound to the thrown
value:

```jennifer
use convert;
use io;

def input as string init "42";
try {
    def n as int init convert.toInt($input);
    process($n);
} catch (err) {
    io.printf("not a number: %s\n", $err.message);
}
```

### What can be thrown

Any value. The **convention** is an `Error` struct, which the runtime
predefines (no `def struct Error { ... };` needed):

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
catch sites dispatch on `$err.kind`. An `Error` literal may leave out its
location (`file`, `line`, `col`) - the one struct whose literal need not name
every field - and `throw` fills a missing location in with the throw site's, so a
caught error says where it was raised. `kind` and `message` stay required. A
location given explicitly, and a caught error rethrown with `throw $err;`, keep
their own.

```jennifer
func parseConfig(src as string) {
    if (not strings.contains($src, "=")) {
        throw Error{kind: "parse_error", message: "missing `=`"};
    }
    # ... happy path ...
}

def cfg as string init "port 8080";
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

An error that carries more - an HTTP status, a vendor code - is a struct of
your own with `kind` and `message` string fields plus its data. Handlers
dispatch on `kind` as for any error, and read the extra fields only under a
kind the struct alone uses: `convert.typeOf` says `"struct"` for every
struct, so it cannot tell yours from `Error`. Lint (L104) accepts such a
struct like `Error`, and `testing.assertThrows` and the test report read its
`kind` and `message`. Bundled protocol clients do the same: `rest.HttpError`,
`graphql.GraphqlError`, `jsonrpc.RpcError`, `mcp.McpError`, `oauth.OAuthError`,
`smtp.SmtpError`, `ldap.LdapError`, `amqp.AmqpError`, `acme.AcmeError`,
`soap.SoapError`, `webdav.WebdavError`, `mqtt.MqttError`, `snmp.SnmpError`,
`imap.ImapError`, `redis.RedisError`, `s3.S3Error`, `pop.PopError`,
`ical.IcalError`, `vcard.VcardError`, `memcache.MemcacheError`,
`ntp.NtpError` and `zmq.ZmqError` are every error of their module's kind
(the `@jennifer` decks' clients too, such as `mikrotik.MikrotikError` or
`telegram.TelegramError`). A catch binding takes whatever was thrown, so a helper
that receives one declares the module's struct type, not `Error`:
`func retryLater(e as smtp.SmtpError)`.

```jennifer
def struct HttpError { kind as string, message as string, status as int };

func showUser(id as int) {
    try {
        fetchUser($id);       # throws HttpError{kind: "api.http", ...} on a non-2xx
    } catch (err) {
        if ($err.kind == "api.http" and $err.status == 404) {
            io.printf("no such user\n");
        } else {
            throw $err;
        }
    }
}
```

A bare `throw "boom";` works too (any value), but the handler cannot read `.kind` / `.message` off it, and lint warns (L104). Use
`convert.typeOf($err)` if you need to tell a thrown string from a struct.

### What can be caught

- **User-issued `throw EXPR;`** - the thrown value, copied into the catch
  binding (value semantics).
- **Runtime errors** - out-of-bounds reads / writes, missing map
  keys, type mismatches, division by zero, undefined names,
  bytes-element range violations, and the rest of the positioned
  runtime errors. The runtime wraps them into the canonical `Error`
  struct with `kind = "runtime"` (the sites below use a more specific
  kind) and the original file / line / col preserved.
- **Timeouts** - a `net` read or write past its deadline, a `connect` past its
  `timeoutMs`, a `task.waitTimeout` / `waitAnyTimeout` that runs out - carry
  `kind = "timeout"`. In `net`, a call on a closed socket (one closed before
  the call, or under a call blocked on it) carries `kind = "closed"`, so a poll
  loop can tell "nothing arrived yet" and "we closed it" from a broken socket
  (`"net"`).
- **Resource-exhaustion errors** carry `kind = "limit"` instead of
  `"runtime"`, so a caller can tell "the input was too big / too deep"
  from "something inside me is broken" and a library can re-raise it in
  its own vocabulary. It covers the call-depth cap ("call stack too
  deep"), the recursion memory guard (below), `archive.unpack` /
  `unpackWith` exceeding their size / entry caps, and the receive and
  processing caps of the bundled modules (see the table below).

  The call-depth cap counts function calls, block nesting, and expression
  nesting together, so recursion through several nested `if` blocks or a
  recursive call wrapped in many parentheses raises a catchable `"limit"`
  error rather than crashing. A companion **recursion memory guard** covers
  the heap the depth cap does not: a recursion whose every frame copies a large
  value (a parameter or local holding a big `list` / `map` / `string`) would
  exhaust memory, an uncatchable crash, long before the depth cap fires, so the
  live per-frame copies are bounded too, raising the same catchable `"limit"`
  error. The accounting is crude: it
  counts the value copies made into recursive frames, not every allocation, so
  a single huge value or a non-recursive loop that grows one is still bounded
  only by the host's own memory. The fix for a tripped recursion is the one the
  error names - pass the big value as a read-only parameter (which is borrowed,
  not copied) or index it in place, rather than binding a fresh copy per frame.

### Error kinds

A handler dispatches on `kind`, so the kinds are the error vocabulary. Most
come from one module or library and are named after it (`"json"`, `"smtp"`,
`"webdav"`). A few are **generic**: any library or module raises them for the
same situation, so a handler can treat it the same way whatever raised it.

| Kind | Raised for | Raised by |
|---|---|---|
| `"runtime"` | a fault in the program: an index out of range, a missing map key, a type mismatch, division by zero, a bad argument to a library function | the interpreter and the Go libraries |
| `"value"` | an invalid input value, such as a malformed semver, JWT or colour name | `jwt`, `semver`, `feed`, `markdown`, `screen`, `ansi`, `validate`; most modules use their own kind for this instead |
| `"limit"` | a size, count or depth cap was reached: the input may be valid, just too big for the cap | the call-depth and recursion guards, `archive`, `net.readAll`, the document decoders' nesting and node caps (`json`, `toml`, `yaml`, `xml`, `asn1`, and their `set` / `insert`), and the bundled modules' receive caps (`http`, `imap`, `pop`, `redis`, `ldap`, `mqtt`, `websocket`, `zmq`, `transport`) and processing budgets (`font`, `html`, `pdf`, `tengine`, the `snmp` / `webdav` walks) |
| `"timeout"` | a deadline passed | `net` reads, writes and `connect`, `serial` deadlines, `i2c` bus timeouts, `task.waitTimeout` / `waitAnyTimeout`, `channel.recvTimeout` |
| `"closed"` | the connection or handle is closed: closed before the call, under a blocked call, or by the peer mid-read (`net.readN`, a reply `http` got only part of) | `net`, `serial`, `http`, and the protocol modules over `net` (`amqp`, `imap`, `mqtt`, `pop`, `redis`, `websocket`) |
| `"net"` | the network failed: a connection refused, reset or unreachable, a name that does not resolve, a TLS handshake or certificate that fails, or a peer that closed before any byte of its reply. Always a `net.NetError` (below) | `net` and every module over it (`http`, `smtp`, `redis`, ...), which pass it on |
| `"cancelled"` | the task running the code was cancelled (`task.cancel`); the message is `task cancelled` | the loop checkpoint in a spawned body and every blocking builtin |

A generic kind is a plain `Error`, except `"net"`. A module's own kind may
carry a struct with more fields - `rest.HttpError`, `smtp.SmtpError` and the
others listed above - and is never one of the generic kinds, so a `"limit"`
from `imap` is an `Error`, not an `ImapError`. Match the kind, not the
message: messages are for people and may be reworded.

`"net"` is always a `net.NetError{kind, message, op, reason, sent, replied}`,
so its fields are safe to read once the kind matches (`def ne as net.NetError
init $e;`). `op` is the step that failed (`"connect"`, `"handshake"`,
`"read"`, `"write"`, `"accept"`, `"listen"`, `"lookup"`); `reason` is one of
`"refused"`, `"reset"` (a broken pipe too), `"unreachable"`, `"dns"`,
`"tls"`, `"closed"` (the peer closed before any reply, from a protocol module)
and `"other"`. `sent` (the whole request was written) and `replied` (some of
the answer arrived) are filled in by `http`; `net` itself leaves them false.
They are what a retry decision needs: nothing sent (a `"connect"` or
`"handshake"` failure, or `sent` false) is safe to repeat; a request that was
sent may have been carried out, so repeat it only if it is idempotent. A
module that rewraps the errors it catches into its own kind passes `"net"` on
unchanged, as it does `"limit"`, `"timeout"`, `"closed"` and `"cancelled"`, so
a caller can retry any network client the same way. A malformed address (no
port) is still kind `"runtime"`; a deadline is still `"timeout"`.

```jennifer
func load(url as string) {
    try {
        def body as string init fetchPage($url);
    } catch (e) {
        match ($e.kind) {
            when "limit" { io.printf("the page is too large\n"); }
            when "timeout", "closed" { io.printf("the server did not answer; retry\n"); }
            when "net" { io.printf("cannot reach the server: %s\n", $e.reason); }
            when "cancelled" { throw $e; }   # we are being stopped: let it go
            else { throw $e; }
        }
    }
}
```

### What can NOT be caught

- **`exit` / `exit EXPR;`** - `try { exit 1; } catch (e) { ... }` lets the
  exit through; the catch block does not run.
- **`return` / `break` / `continue`** - control flow, not errors. `try { break; } catch (e) { ... }` breaks the enclosing
  loop; the handler does not run.

### Re-throwing

`throw $err;` inside a catch re-raises the same value to the next enclosing
`try`.

### No `finally`

Jennifer has no `finally` clause; `defer` (below) covers the same need
without `finally`'s traps.

## `defer` (deterministic cleanup)

`defer CALL(args);` schedules a single call to run when the **enclosing block**
exits - on **every** exit path: normal fall-through, `return`, `break`,
`continue`, a `throw` that unwinds through it, and `exit`. It keeps acquiring
a resource and releasing it next to each other:

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

- **The deferred thing must be a call** - a function call (`defer cleanup();`) or a
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
  func processAll(files as list of string) {
      for (def name in $files) {
          def f as fs.File init fs.open($name, "read");
          defer fs.close($f);            # closes at the end of this iteration
          process($f);
      }
  }
  ```

  A top-level `defer` runs at program end.
- **Does not cross the function or `spawn` boundary** (like `break` / `return`): a
  function's defers run when that function returns, not in its caller.
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
  func open(addr as string, useTls as bool) {
      def c as net.Conn init net.connect($addr);
      errdefer net.close($c);        # function-body scope: armed until return
      if ($useTls) {
          $c = net.startTLS($c);     # NOT here - this block exits right away
      }
      handshake($c);
      return Session{conn: $c};
  }
  ```

In the REPL, each input is its own frame (as with `defer`): an input that errors
runs its errdefers, an input that succeeds discards them - they do not carry
over to later inputs.

If cleanup must happen on *every* path (a temp file, a one-shot request's
socket), use `defer`; `errdefer` is only for the acquire-or-undo shape.

