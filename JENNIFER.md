# JENNIFER.md - the Jennifer language, for coding assistants

Drop this file into a project where you write **Jennifer** (`.j` files) and
point your AI coding assistant at it ("we code in Jennifer, see JENNIFER.md").
It is a self-contained language reference for an assistant with no prior
knowledge of Jennifer. It describes the *language*, not the interpreter's
internals.

Jennifer is a batteries-included, interpreted language (tree-walking interpreter
written in Go/TinyGo). Source files use the `.j` extension. Run a program with
`jennifer run program.j`, start a REPL with `jennifer repl`.

**Full documentation** - guides, the complete library reference, and an
alphabetical cheatsheet of every builtin - is hosted at
<https://jennifer-lang.dev/>; with web access, fetch exact function
signatures there. Source and issues:
<https://github.com/jennifer-language/jennifer>.

> This file mirrors the authoritative spec. If something here conflicts with the
> [hosted docs](https://jennifer-lang.dev/), the docs win - tell the
> maintainer.

> **Keep this file in sync with the installed interpreter - it is your job.**
> This copy goes stale as the local `jennifer` is upgraded (or the installed
> `jennifer` may already be older than it). A mismatched JENNIFER.md is the most common reason an
> assistant writes `.j` code the interpreter rejects. **You, the assistant, keep
> it aligned with the installed release** - before you rely on it, and again
> whenever code you wrote fails with an "unknown"/"not defined"/pragma-version
> error.
>
> **Cheap self-check first.** After a sync, the last line of this file is a marker
> recording the exact `jennifer version` string it was synced for
> (`<!-- jennifer-md-synced-for: ... -->`). Run `jennifer version` and compare:
> **equal -> current, do nothing**; **different, or no marker yet (a pristine
> copy) -> run the sync below**. One command and a string compare, so do it at the
> start of any `.j` work.
>
> **Sync:**
>
> 1. Read the installed version: `jennifer version` (e.g. `0.24.0`, or
>    `0.24.0-dev+N.<sha>`).
> 2. **Prefer the copy installed with the interpreter.** The Linux packages
>    (Arch, Debian, the Docker image) put one at
>    `/usr/share/doc/jennifer/JENNIFER.md`, Homebrew (once its tap is published)
>    under `$(brew --prefix)/share/doc/jennifer/`, the Windows installer beside
>    `jennifer.exe`. It was built from the same commit as the installed
>    `jennifer`, so it matches exactly (even a dev build, where `main` may be
>    ahead) and needs no network. Otherwise fetch the matching JENNIFER.md from
>    the source repo:
>    - a **development build** - any version containing `-dev` (e.g.
>      `0.24.0-dev+N.<sha>`) or the bare string `dev` - is ahead of the last
>      release, so **always** fetch the `main` branch, regardless of the `X.Y.Z`
>      it prints:
>      `https://raw.githubusercontent.com/jennifer-language/jennifer/main/JENNIFER.md`;
>    - a **clean release** `X.Y.Z` (no `-dev` suffix) fetches its own tag:
>      `https://raw.githubusercontent.com/jennifer-language/jennifer/X.Y.Z/JENNIFER.md`.
> 3. If the installed or fetched file differs from this local copy, **replace
>    this local copy with it**.
> 4. **Stamp the marker.** Whether or not step 3 replaced anything, set the last
>    line of the local copy to the full `jennifer version` string you read in
>    step 1, so the next self-check is a cheap compare:
>    `<!-- jennifer-md-synced-for: 0.24.0-dev+N.<sha> -->`. Record the whole
>    string verbatim - for a dev build it carries the commit (`+N.<sha>`), so the
>    marker changes the moment the installed interpreter is rebuilt and your check
>    catches it. (Without an installed copy, a dev build re-fetches `main` per
>    step 2, since `main` advances even when the local commit has not; the marker
>    still tells you the interpreter itself moved.)
>
> Match the **installed interpreter**, not the newest release: if the local
> `jennifer` is `0.24.0` while `0.25.0` is out, fetch the `0.24.0` JENNIFER.md. (No installed copy and no web
> access? Then note in your reply that you could not verify JENNIFER.md against
> the installed `jennifer version`, so some guidance here may not match it.)

---

## The 10 rules that trip people up

Where Jennifer differs from Python/JS/Go and an assistant usually guesses wrong:

1. **Variables are referenced with a `$` sigil: `$x`.** But the *declaration*
   uses a bare name: `def x as int init 5;` then use `$x`. Writing `def $x` is
   an error; using bare `x` in an expression is an error.
2. **Constants are referenced bare (no `$`): `MAX`.** They are `UPPER_CASE`.
   Reading one *with* `$` (`$MAX`) is a parse error - the sigil is for
   mutable variables only. A function may not share a name with a top-level
   variable or constant either (`def foo ...; func foo() {}` is rejected).
3. **Function calls are bare and take `()`: `greet()`.** The parser tells a call
   from a constant by the `(`.
4. **`/` is true division and always returns `float`** (like Python 3).
   `5 / 2 == 2.5`. Use `//` for integer/floor division: `5 // 2 == 2`. `%`
   is **floored** to match `//` (`-7 % 3 == 2`, `7 % -3 == -2`). Integer
   arithmetic that overflows `int64` is a runtime error (no silent wrap,
   including `-(MinInt64)`), and **float arithmetic that overflows to a
   non-finite value is an error too** (`1e308 * 10.0` raises, never
   `+Inf`/`NaN`); a mixed `int`/`float` comparison is exact (no lossy
   promotion).
5. **Identifiers are letter-initial, then letters + digits, <= 64 chars.** No
   underscores in variable/function/parameter/library names. `myVar`, `var2`,
   `sha256` are fine; `my_var` is not, and a name cannot start with a digit.
   (Constants are the *only* names that take `_`: `MAX_RETRIES`.)
6. **Statements end with `;`.** Whitespace (including newlines) is
   insignificant everywhere.
7. **Comments are `#` (line) and `/* */` (block, nests).** Not `//` - that is
   the floor-division operator.
8. **No `++`, `--`, `+=`, or any compound assignment.** Only `$x = EXPR;`.
9. **Value semantics: assignment and argument passing copy.** `$b = $a;` then
   mutating `$b` never touches `$a`. Same for lists, maps, structs, bytes. The
   copy is a semantic guarantee, not always a cost: a compound parameter the
   function never writes is passed without a copy where that cannot be observed,
   so read-only struct / list / map parameters (a recursive evaluator's context)
   are cheap and need no workaround. Three exceptions copy on every call: a
   nested-collection parameter (`list of list`, `map of K to list`, ...; wrap it
   in a struct to get the borrow); any compound parameter of a function that can
   reach a callback builtin (`lists.sortBy`, `map`, `filter`, ...) or a func-value
   call, when the entry file has a mutable top-level `def` (prefer `lists.sort` on
   keys, or `def const`); and every argument or result of a call into a module
   that holds one of that module's own structs (it is rebuilt both ways).
   `jennifer profile --allocs` shows each place a copy or rebuild still happens.
10. **Logical operators are words: `and`, `or`, `not`** (not `&&`/`||`/`!`).
    `&` `|` `^` `~` are the *bitwise* operators.

---

## Lexical basics

- **Identifiers** (variables, functions, parameters, library names):
  `[A-Za-z][A-Za-z0-9]*`, <= 64 chars. Letter-initial (a digit-initial token is a
  number), then letters and digits; no underscores. Legal: `myVar`, `sha256`,
  `x2`, `toUtf8`.
- **Constant names**: uppercase chunks joined by single `_`, with in-chunk
  digits: `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*` (each chunk starts with a letter).
  Legal: `MAX`, `MAX_RETRIES`, `HTTP_OK`, `SHA256`, `HTTP2`, `SCRAM_SHA256`.
  Illegal: `_MAX`, `MAX_`, `MAX__INT`, `maxInt`, `AES_256` (write `AES256`).
- **`.j` import paths** are strings and may contain digits, `_`, `/`.
- **Reserved words** (cannot be a variable, function, parameter, or library name):
  `and` `as` `break` `catch` `const` `continue` `def` `defer` `else` `elseif`
  `enum` `errdefer` `exit` `export` `false` `for` `func` `if` `import` `in`
  `include` `init` `len` `list` `map` `match` `not` `null` `of` `or` `repeat`
  `return` `spawn` `struct` `task` `throw` `to` `true` `try` `until` `use` `when`
  `while`, plus the type keywords `int` `float` `string` `bool` `bytes` (legal only
  after `as`). These look contextual but are **not**:
  `list` / `map` (unlike `channel`, they are hard keywords even outside `list of T`
  / `map of K to V`), and `to` / `of` / `in` (keywords everywhere, not just in a
  type or `for`-each). `channel` is the one contextual word - a type only
  in `channel of T`, a valid identifier elsewhere. So `from`/`to` as a parameter
  pair fails on `to`; use `to2` / `hi` / `end` instead.
- A leading `#!` line is allowed (shebang): `#!/usr/bin/env -S jennifer run`. A
  file with a shebang may also be run **without a `.j` extension** (as an
  executable installed under a bare command name); `import` / `include` targets
  still require `.j`.

## Types

Primitive: `null`, `int`, `float`, `string`, `bool`, `bytes`.
Compound: `list of T`, `map of K to V`, user `struct`s, user `enum`s (sum
types), `task of T` (a handle to a `spawn`ed computation), `func` (a first-class
function value), `channel of T` (a CSP channel between goroutines).

- **int** literals: `42`, `0xff`, `0o755`, `0b1010`, with `_` digit separators
  (`1_000_000`, `0xDEAD_BEEF`).
- **float** literals: a `.` (`3.14`, `0.5`) or an `[eE][+-]?` exponent
  (`6.022e23`, `1.6e-19`, `1e10` - the exponent alone makes it a float); `_`
  separators in the mantissa only. Overflow (`1e400`) is an error, not `Infinity`;
  underflow (`1e-400`) rounds to `0.0` (a finite value; only the non-finite is banned).
- **string** literals: `"..."` is **cooked** -
  escape sequences `\n \r \t \\ \" \' \0 \{ \}`, plus Unicode `\uXXXX` (exactly 4
  hex, the BMP) and `\UXXXXXXXX` (exactly 8 hex, any plane, e.g. `\U0001F600`), are
  processed; a surrogate, an out-of-range code point, or the wrong digit count is
  a lex error. A cooked string also **interpolates**: an unescaped `{expr}` is a
  slot (see below). `'...'` is **raw** - no
  escape processing at all and no interpolation: every byte to the next `'` is
  literal (backslashes, braces, and newlines included), so `'\d+\.\d+'` is an
  8-char string, `'{"port": 8080}'` is literal JSON, and a multi-line block is
  a `'...'` that spans newlines. To put a `'` inside a string, use the cooked
  form: `"it's"`. There is no `r"..."` prefix.
  **Reach for raw `'...'` by default when the text carries backslashes or
  braces** - regex patterns (`'\d+\.\d+'`, `'\bword\b'`), Windows paths
  (`'C:\Users\me'`), JSON / template / brace-heavy literals (`'{"a": 1}'`,
  `'${VAR}'`), and any escape-dense blob: raw needs no escaping, unlike the
  cooked equivalent (`"\\d+\\.\\d+"`, `"C:\\Users\\me"`, `"\{\"a\": 1\}"`).
  Pick cooked `"..."` only when you need one of its three features: an escape
  sequence (`\n`, `\t`, `\uXXXX`), a `'` inside the string, or `{expr}`
  interpolation. Rule of thumb: **if you find yourself typing `\\`,
  `\{`, or `\}`, switch the whole literal to raw `'...'`.**
- **string interpolation**: inside a cooked `"..."` string, each unescaped
  `{expr}` is a slot - one Jennifer **expression** evaluated in the current scope
  and stringified in place (the `convert.toString` form; no `use convert` needed).
  `"total: {$sum}, next {$n + 1}, up {strings.upper($s)}"`. A slot holds a single
  expression (a variable, constant, field / index access, arithmetic, a call), not
  a statement - a `;`, `def`, `if`, or assignment in a slot is a parse error, and
  an empty `{}` is an error. Write a literal brace as `\{` / `\}`; a bare unescaped
  `}` is a lex error. A **raw** `'...'` string never interpolates (it is the "no
  interpolation" form). No `f"..."` prefix - the cooked / raw split is the opt-in.
  Style: keep slots to variables, field / index access, and arithmetic; a
  side-effecting or expensive **call** in a slot is flagged by `lint` (L204) -
  compute it into a variable first. An undefined variable in a slot is a
  parse-time error, like any other reference.
- **bool**: `true`, `false`. **null**: `null`.
- **bytes** has no literal: build with `convert.bytesFromString("utf-8", s)`
  or append into `def b as bytes;` with `$b[] = 65;`.
- **list** literals: `[1, 2, 3]`, `[]`. Lists are homogeneous (one element
  type).
- **map** literals: `{"a": 1, "b": 2}`, `{}`. Insertion-ordered.
- **struct** literals: `Point{x: 1, y: 2}` after
  `def struct Point { x as int, y as int };`. Every field must be named, with two
  exceptions: an `Error` literal may omit `file` / `line` / `col`, and a literal
  of a **library** struct (`net.Datagram{peer: $p}`, `os.Result{...}`) or of a
  struct from a module that ships with Jennifer (`jwt.Policy{iss: $i}`) may omit
  any field, which then takes its zero value - so a field added later does not
  break it. An unknown or missing field is an error when the program
  loads, not when the literal runs.
- **enum** (sum type): `def enum Shape { Circle { r as float }, Empty };` at top
  level. A value is one variant: construct with `Shape.Circle{r: 2.0}` or the
  payload-less `Shape.Empty` (cross-module: `alias.Shape.Circle{...}`). Value
  semantics + equality like structs. `def s as Shape;` (no init) zeroes to the
  **first** variant, payload zeroed. Read the payload only through `match` (no
  `$enum.field`). Names may be any case (an all-`UPPERCASE` name is a constant,
  so don't name a type all-caps).
- **func** (first-class function value): a bare function name in expression
  position **is** the value; a name followed by `(` is a call.
  `def f as func init greet;` binds the function `greet` into a value; call it
  through a variable or any function-valued expression: `$f(args)`,
  `$fns[0](x)`, `makeAdder(1)(2)`. Pass and return them like any value
  (`func apply(fn as func, x as int) { return $fn($x); }`). Arity + argument
  types are checked at the call site (the `func` type has no signature).
  Immutable (copies share the function; value semantics hold). Zero value
  (`def f as func;`) is a **null** function - calling it errors. No `&NAME`
  sigil and no anonymous-function / closure literal (yet). Powers the
  higher-order `lists` layer (`map` / `filter` / `reduce` / `find` / `any` /
  `all` / `sortBy`).

  ```jennifer
  use lists;
  func dbl(n as int) { return $n * 2; }
  func isEven(n as int) { return $n % 2 == 0; }
  def xs as list of int init [1, 2, 3, 4];
  def doubled as list of int init lists.map($xs, dbl);      # [2, 4, 6, 8]
  def evens as list of int init lists.filter($xs, isEven);  # [2, 4]
  ```

## Variables and constants

```jennifer
def x as int;                 # declare, zero value (0)
def y as int init 5;          # declare + initialize
def const MAX as int init 10; # constant, must be initialized, never reassigned
$x = 7;                       # assignment uses the $ sigil
```

- The name at the `def` site is bare (`def x`), never `def $x`.
- `const` is deep: a const list/map/struct rejects mutation at any depth.
- Top-level code runs in source order: define a constant **above** the code that
  first reads it. A top-level read before its `def const` is a load-time error; a
  function may read a constant defined lower down, but not when it is called
  before that definition has run.
- A function body may not `def` a name that a top-level `def` already uses (no
  shadowing, globals included): a helper's loop variable cannot reuse the name
  of a top-level loop variable.

## Operators

- Arithmetic: `+  -  *  /  //  %`. `/` is float division; `//` is floor.
- Unary `-` (negation). `+` also concatenates two strings.
- Comparison: `<  >  <=  >=  ==  !=` -> `bool`. `!=` is `not (a == b)`. The
  ordering operators (`<  >  <=  >=`) work on two numbers or two strings
  (strings compare lexicographically by UTF-8 bytes); a string/number mix is a
  type error. There is no bare `!` (logical negation is the word `not`); a lone
  `!` is a lex error.
- Logical (words, short-circuit): `and`, `or`, `not`. Operands must be `bool`.
- Bitwise (int only): `&  |  ^  ~  <<  >>`.
- Mixed int/float arithmetic promotes to `float`.
- Range `..`: `lo..hi` is a half-open range `[lo, hi)`, int bounds only,
  non-associative, looser than every other operator. Builds a `list of int`
  (`def r as list of int init 0..n;`), or iterates lazily as a for-each source
  (`for (def i in 0..n)`), or slices (`$xs[a..b]`). `lo > hi` errors; `lo == hi`
  is empty. Always a fresh copy, never a view.
- Precedence, low to high: `..` < `or` < `and` < `not` < comparison < `|` < `^`
  < `&` < shifts < `+ -` < `* / // %` < unary `- ~`. So `$x & 0xff == 0` parses
  as `($x & 0xff) == 0`, and `1+1..2*3` as `(1+1)..(2*3)`.

## Control flow

```jennifer
if ($n > 0) { ... } elseif ($n < 0) { ... } else { ... }

while ($i < 10) { ... }

for (def i as int init 0; $i < 10; $i = $i + 1) { ... }   # C-style

for (def x in $xs) { ... }     # for-each over a list (elements)
for (def k in $m) { ... }      # for-each over a map (keys, insertion order)
for (def i in 0..10) { ... }   # for-each over a half-open range (lazy)

repeat { ... } until ($done);  # post-test loop; body runs at least once

match ($cmd) {                 # multi-way value dispatch (subject evaluated once)
    when "start" {
        start();
    }
    when "stop", "halt" {      # several values per arm (an OR of equality)
        stop();
    }
    else {                     # optional default, must be last
        unknown();
    }
}

# Over an enum subject, `match` dispatches on the variant and binds its payload.
# It must be exhaustive (cover every variant) or carry an `else`.
match ($shape) {
    when Circle(c) { area($c.r); }   # $c is the variant's payload (a mini-struct)
    when Rect(rc)  { area2($rc.w, $rc.h); }
    when Empty     { }               # payload-less variant: no binder
}

break;      # exit innermost loop
continue;   # next iteration
exit;       # terminate the whole program (exit 0); exit EXPR sets the code (Unix: 0..255, masked to 8 bits)
```

Conditions must be `bool` (there is no truthiness). `and` / `or` **short-circuit**
(`true or f()` never calls `f`). `break`/`continue` do not cross a function-call or
`spawn` boundary.

`match` compares the subject to each `when` value by strict `==`; the first
matching arm runs and its values evaluate left-to-right only until a match. There
is **no fall-through**, and `match` is **not** a `break` target - `break` /
`continue` in an arm act on the enclosing loop. A bare `when Name { }` reads
`Name` as a value and `{` as the block, so a struct-literal value needs parens:
`when (Point{x: 1}) { ... }`. No matching arm and no `else` is a no-op.

### Errors

```jennifer
use io;
try {
    throw Error{kind: "bad", message: "nope"};
} catch (e) {
    io.printf("%s at line %d\n", $e.message, $e.line);
}
```

`throw EXPR;` raises any value; convention is the auto-provided `Error` struct
`{kind, message, file, line, col}`. An `Error` literal may omit `file` / `line` /
`col` (only `kind` and `message` are required), and `throw` stamps a missing
location with the throw site. `catch` also catches the runtime errors builtins
raise (out-of-range, missing key, etc.), wrapped into `Error` with `kind:
"runtime"` - or `"limit"` (a resource cap), `"timeout"` (a `net` deadline) or
`"closed"` (a `net` socket closed before or under the call).
Runaway recursion raises a catchable `"limit"` too ("call stack too deep"). The
depth budget (10000 on the default binary, 120 on `jennifer-tiny`) counts nested
blocks and operand evaluations as well as calls, so the reachable recursion depth
is lower than 10000 - a few hundred to a few thousand calls, depending on how
much nesting each call adds. A library that promises callers a specific error
kind should expect `"limit"` from deep input (a deeply nested document) and
translate it.
`exit`/`return`/`break`/`continue` are control flow, not catchable.

### Cleanup with `defer`

```jennifer
use fs;
func write(path as string) {
    def f as fs.File init fs.open($path, "write");
    defer fs.close($f);         # runs when the block exits, however it exits
    fs.writeString($f, "data\n");
}
```

`defer CALL(args);` schedules a **call** (function / namespaced / module call - a
non-call is a parse error) to run when the **enclosing block** exits, on every
path (`return`/`break`/`continue`/`throw`/`exit`/fall-through), **LIFO**.
Arguments are evaluated at the `defer` line; the call runs at block exit.
Block-scoped (a `defer` in a loop body runs each iteration); does not cross a
function or `spawn` boundary. A deferred throw propagates and supersedes a pending
error (never an `exit`). There is no `finally`.

`errdefer CALL(args);` is the error-path variant: same form, same LIFO stack,
but the call runs **only when the block exits with a propagating error** (a
`throw` or a runtime error) - skipped on fall-through, `return`, `break`,
`continue`, and `exit`. It is the undo half of an acquire whose resource must
survive on success:

```jennifer
func connect(addr as string) {
    def c as net.Conn init net.connect($addr);
    errdefer net.close($c);     # a failed handshake closes; success keeps it open
    handshake($c);
    return Session{conn: $c};
}
```

## Functions

```jennifer
use io;

func greet(name as string) {
    io.printf("hi %s\n", $name);   # parameters referenced as $name
    return;                        # bare return -> null; or return EXPR;
}

greet("ada");
```

- Bare parameter names (`name as string`), referenced inside as `$name`.
- No return type is declared; the caller's `def x as T init f();` checks it.
- Functions are **top-level only** (not nested). Recursion works.
- Function bodies see global variables/constants. A function may not shadow a
  global name, nor share a name with a builtin from an imported library.
- A program has **no required entry point**: top-level statements run in order.

## Compound types: indexing and iteration

```jennifer
def xs as list of int init [1, 2, 3];
$xs[0];            # read -> 1
$xs[0] = 9;        # write
$xs[] = 4;         # append (write-only; lists and bytes only)
$box.items[] = 5;  # append through a chain: a struct field ...
$grid[0][] = 6;    # ... or a nested list - in place, like the bare form
# Never `$box.items = lists.push($box.items, v)` in a loop: that copies the whole
# list every pass (O(n^2)). See the accumulator note under "Prefer $xs[]".

def m as map of string to int init {"a": 1};
$m["a"];           # read (missing key is an error - test with maps.has)
$m["b"] = 2;       # write

def p as Point init Point{x: 1, y: 2};
$p.x;              # field read
$p.x = 5;          # field write

$grid[i][j] = v;   # chains nest and mix [index] and .field

def mid as list of int init $xs[1..3];   # slice: half-open [1, 3) copy
$xs[2..];  $xs[..3];  $xs[..];           # open ends default to 0 / len
```

**Slicing (`$xs[a..b]`)** returns a fresh, value-semantic copy of a half-open
`[a, b)` sub-range of a `list`, `bytes`, or `string` (rune-indexed). Open ends
default to the extremes (`$xs[a..]`, `$xs[..b]`, `$xs[..]`). Bounds are strict
(`0 <= a <= b <= len`). A slice is **read-only**: `$xs[a..b] = ...` is a parse
error (it is a copy, so a write through it would do nothing).

**Prefer `$xs[]` over `lists.push` in loops.** The `$xs[] = item;` append sugar
(lists and bytes) mutates in place - amortized O(N) to append
N items. `$xs = lists.push($xs, item)` returns a *new* list each pass and copies
the whole list, so the same loop is O(N^2). Use `$xs[]` to build a list element
by element (a raster, a buffer, a big result set); use `lists.push` only when you
want a fresh list and keep the original.

**Thread a growing accumulator through a call only as `$acc = f($acc, ...)` in
a function.** A `list` / `bytes` / struct argument is deep-copied on a call (value
semantics), so passing a growing accumulator into a helper each iteration is
O(N^2) - the tell is that it scales *backwards*: smaller batches run slower,
because each call re-copies a larger accumulator. The one exception: inside a
function, `$acc = addTo($acc, v);` on a local or parameter, outside a `try`, moves
the old value in (it is dead after the call) and takes the mutated parameter back
without a copy - O(1) per call. Top-level code (its variables are globals), a
global, a `try`, or another target copies. Simplest is to keep the
accumulator in the caller and append with `$xs[]`:

```jennifer
# O(1) per call when called as `$acc = addTo($acc, v);` inside a function;
# O(N) per call (so O(N^2) in a loop) from top-level code or inside a `try`
func addTo(acc as list of int, v as int) {
    $acc[] = $v;
    return $acc;
}
# still quadratic anywhere: a local copy of the parameter copies every element
#     def out as list of int init $acc;
# always linear: the helper returns just the piece; the caller owns the accumulator
for (def v in $items) { $results[] = compute($v); }
```

`jennifer profile --allocs` names the offending line when a copy like this shows up,
ranked by bytes copied (with the copied type), so a big accumulator outranks a
small value copied more often. `jennifer lint` flags both accumulator shapes
before you profile (`L205`): a parameter mutated and returned, and a parameter
copied into a local that is grown with `[] =` and returned.

`len(EXPR)` is a language built-in (not a library): rune count of a string,
element count of a list, entry count of a map, byte count of bytes.

## Concurrency

```jennifer
use task;
def t as task of int init spawn { return expensiveThing(); };
def result as int init task.wait($t);   # also poll / discard / waitAll / waitAny
```

`spawn { ... }` runs concurrently and evaluates to a `task of T`. It deep-copies
its enclosing scope at launch, so there are no shared-memory data races.

**Cancellation + timeouts.** `task.cancel($t)` requests cooperative cancellation:
the body observes it at its next loop checkpoint, where the runtime raises a
catchable "task cancelled" (so a runaway `spawn` can be stopped). `time.sleep`,
`channel.send` / `recv` / `select` and the `task` waits wake and raise the same.
A task blocked in a system call (`net.accept`, a socket read, `term.readByte`)
only sees it when the call returns: close its socket from another task (the call
then throws `Error{kind: "closed"}`), or `task.discard` it.
Catch it inside the body for a clean partial result; `task.cancel` plus
`task.discard` is stop-and-forget. A program that runs to its end waits for
every undiscarded task; one that ends by `exit` or an uncaught error does not.
`task.cancelled()` is a non-raising poll.
`task.waitTimeout($t, ms)` / `task.waitAnyTimeout($ts, ms)` are bounded waits that
throw a catchable `Error{kind: "timeout"}`.

**Channels.** `channel of T` streams values between goroutines (the counterpart to
`task`'s single result). `channel.make(capacity)` (0 = unbuffered), `send`
(deep-copies the value in - the receiver gets its own copy), `recv` (blocks; throws
a catchable error on a closed+drained channel - drain with `try { while (true) {
process(channel.recv($ch)); } } catch (e) { }`), `close`, `select([...])` (fan-in:
next value from any open channel), `len` / `capacity`. A channel is a shared
handle but the values through it are copied, so no-shared-mutable-state holds.
`channel` is a contextual keyword (a type only in `channel of T`; a valid
identifier elsewhere).

## Imports

```jennifer
use io;                 # enable a system library, addressed io.printf(...)
use strings as s;       # alias: only s.upper(...) works after this
include "helpers.j";    # textual splice of another .j file (preprocessor)
import "./util.j" as u; # load util.j as a module, addressed u.fn(...) / u.CONST
```

- `use NAME [as ALIAS];` - system library. Nothing auto-loads; every program
  states its imports. Aliasing is a rename (the canonical prefix stops working).
- `include "path.j";` - textual file splice (path is a string literal ending in
  `.j`, resolved relative to the including file). A file only ever `include`d is
  named `NAME.inc.j`; `jennifer lint` then skips the names it takes from its
  includer (lint the including file).
- `import "PATH.j" [as NAME];` - **module** import (a real boundary, not a
  splice). Path forms: `./x.j` / `../x.j` local, `/x.j` absolute, bare `x.j`
  from the module search path. Loads once (run-once, cached), depth-first
  post-order; cycles error. Re-importing the same module under the same alias
  is a no-op. Reach the module's surface as `NAME.fn(args)`,
  `NAME.CONST`, and `NAME.Struct` / `NAME.Struct{...}` (`NAME` is the `as`
  alias, else the file stem). A **module top level is declarations-only**:
  `def const`, `def struct`, `func`, `use`, `import` - no mutable `def`, no
  free-standing statements. `use` is not transitive across the boundary.
- `export` publishes a top-level `def const` / `def struct` / `func` from a
  module; unmarked names are private (reaching one from outside errors). A
  module struct type keeps its identity `(module, name)` at the consumer, so
  `def p as NAME.Struct init NAME.make();` type-checks and `a.Point` /
  `b.Point` are distinct. An exported struct/func may not expose a private
  struct. `export` is only valid in a module (a parse error in a `run`
  script). A co-located `MODULE_test.j` white-box overlay runs under
  `jennifer test`.
- **Requirement header** - a file may declare what it needs with typed comment
  pragmas in its header block, checked at read time (a program, a module, and each
  `include`d file self-checks): `# pragma-jennifer-version: >=0.25.0` (a minimum
  interpreter floor; any `-dev` build bypasses, only a release tag is compared - one
  floor per file) and `# pragma-jennifer-capability: net` (a host facility the build
  must have: `net`, `exec` or `sql`, none on `jennifer-tiny`; multiple accumulate). A
  mismatch aborts with a clear message; a malformed directive is a hard error.
  Query capabilities at runtime with `meta.hasCapability("net")` / `meta.CAPABILITIES`.

### Decks (packages)

A **deck** is a third-party package with a scoped name: `@jennifer/NAME`
(official) or `@deckyard/NAME` (community), among others. Find decks, their
versions and manuals in the registry at <https://registry.jennifer-lang.dev/>
(search, `/browse`, a page per deck at `/deck/<scope>/<deck>`). Bare names
(`import "http.j";`) are the bundled modules below, never decks.

```sh
jvc add "@jennifer/slack" "^0.1.0"   # record the requirement in deck.toml
jvc install                          # resolve, vendor into ./vendor/, pin camcorder.lock
jvc update                           # move to the newest versions the ranges allow
```

`jvc` (the package manager, <https://jvc.jennifer-lang.dev/>) keeps the
requirements in the project's `deck.toml`, next to the interpreter floor:

```toml
[engines]
jennifer = ">=0.25.0"

[decks]
"@jennifer/slack" = "^0.1.0"
```

```jennifer
import "@jennifer/slack/" as slack;           # the deck's entry module (slack.j)
import "@deckyard/forensicgenetics/lineage.j" as lineage;  # another module in it
```

- A trailing `/` imports the deck's entry module `<deck>.j`; the default alias is
  the deck name. `@scope/deck/file.j` reaches another module in the deck.
- `@` resolves only under a vendored tree: `--vendor DIR`, else
  `JENNIFER_VENDOR`, else the nearest `vendor/` above the program. A sibling
  checkout of the deck is not found; run `jvc install` (`vendor/` is
  gitignored, `camcorder.lock` is committed). No `.` / `..` in the path.
- A deck's files carry its floor and capabilities as pragmas, so a deck needing
  `net` / `exec` / `sql` refuses to load on `jennifer-tiny`.

## Standard library (all namespaced, all opt-in via `use`)

Call as `LIB.name(...)` after `use LIB;`. Argument order everywhere (also for
your own modules): a handle first, then an algorithm / format / codec name, then
the subject, then other parameters, optional ones last. **`[full]`** marks a library (or part)
that needs the default `jennifer` binary; `jennifer-tiny` stubs it. Reads that
return outside data as a `string` (`io` / `fs` text reads, `os.run` / `wait`
output) error on invalid UTF-8, naming the byte offset; read `bytes` for
binary data (`os.runBytes` / `waitBytes`). `httpd` answers such a path / query / header 400 itself.

- **`io`** - `printf` (stdout) / `eprintf` (stderr) / `sprintf` (to a string)
  with verbs `%d %f %s %t %v %a` and `%verb|key=value` modifiers (`pad`,
  `align`, `base`, `prec`, `sign`, `group`, `case`, ...); `%a` (list / map) also
  takes `prec trim sci base sign mode case` per element (`%a|prec=2`). A bare
  modifier value ends at the first non-`[A-Za-z0-9_]` character (`"%d|base=16,
  %d"` prints the comma; quote others: `%a|sep=" | "`). Input `readLine` /
  `readLines` / `eof`, binary `readBytes` / `readChars`.
- **`convert`** - `toInt toFloat toString toBool` (the type names are keywords),
  `typeOf`, `objectType`, `fromCodepoint` / `toCodepoint`, `bytesFromString` /
  `stringFromBytes` (`"utf-8"` only). `toInt(s, radix)` takes bare digits in base
  2 / 8 / 10 / 16 (strip `0x` first); `toString(n, radix)` is the inverse.
- **`math`** - `abs min max sqrt pow floor ceil round trunc sign cbrt hypot`;
  `sin cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh`; `exp
  expm1 ln log10 log2 log1p log`(x, base); `factorial comb perm gcd lcm`; `rand
  randInt randSeed` (not crypto-grade); `erf erfc gamma lgamma beta lbeta
  regGammaP regGammaQ regBetaI`; `PI E TAU`. Radians; `floor` / `ceil` /
  `round` / `trunc` return int; undefined results throw (no NaN).
- **`decimal`** - exact base-10 numbers in an opaque `decimal.Decimal`;
  operands are a Decimal, an `int` or a decimal string (`"12.34"`), never a
  float (`from(f)` converts one). `add` / `sub` / `mul` / `sum` / `pow(d, n)`;
  `div(a, b)` is exact or throws, `div(a, b, scale[, mode])` rounds;
  `round(d, scale[, mode])`. Modes `"half-even"` (default), `"half-up"`,
  `"half-down"`, `"up"`, `"down"`, `"ceiling"`, `"floor"`. `==` is numeric
  (`1.0 == 1.00`), `%v` keeps the scale; order with `compare(a, b)` (`<`
  throws); `sign` / `scale` / `isInteger` / `normalize` / `neg` / `abs`;
  `toString` / `toInt` (exact only) / `toFloat`. Big integers are scale-0
  Decimals; over 100,000 digits is kind `"limit"`.
- **`stats`** - over `list of int` / `float`: `mean median mode modes
  geometricMean harmonicMean weightedMean variance stddev sampleVariance
  sampleStddev range iqr mad skewness kurtosis percentile quartiles min max sum
  zscore correlation covariance sampleCovariance describe` (population unless
  `sample*`; `kurtosis` is excess; `min` / `max` / `mode` / `modes` / `range` /
  `sum` keep the input kind, `describe -> Summary`). Distributions:
  `normalPdf/Cdf/Quantile/Sample`, `tPdf/tCdf/tQuantile`,
  `chiSquareCdf/Quantile`, `fCdf/fQuantile`, `binomialPmf/Cdf`,
  `poissonPmf/Cdf`. Inference: `linearRegression` / `multipleRegression`,
  `confidenceInterval`, `proportionCi` (wald / wilson / clopper-pearson),
  `tTest` / `tTest2`, `chiSquareTest`, `fTest`, `anova`, `histogram` ->
  `Regression` / `Interval` / `Test` structs. Undefined results throw. There
  is no `prob` library: distributions live here.
- **`ml`** - classical ML on tabular data: a fit returns an opaque `ml.Model`
  used with `predict` / `transform` / `predictProba`. `linearRegression`
  `ridge` `lasso` `kNNRegressor` `decisionTreeRegressor`
  `randomForestRegressor`; `kNN` `naiveBayes` `logisticRegression`
  `decisionTree` `randomForest`; `kMeans` `pca` `standardScaler`
  `minMaxScaler`. `coefficients` `intercept` `centroids` `components`
  `explainedVariance` `featureImportances`; `trainTestSplit -> Split`, `kFold
  -> list of Fold`, `polynomialFeatures`; `accuracy` `precision` `recall` `f1`
  `confusionMatrix` `rocAuc` `logLoss` `rmse` `mse` `mae` `r2`. X is rows
  (`list of list of float/int`), y a list; `math.randSeed` makes it repeatable.
- **`linalg`** - vectors (`list of float`): `dot distance cross normalize`;
  matrices (`list of list of float`): `transpose trace determinant inverse
  solve identity zeros shape`; `norm` / `scale` / `add` / `sub` take either;
  `matmul` does matrix*matrix, matrix*vector, vector*matrix (use `dot` for two
  vectors). Mismatched shapes, singular matrices, a zero vector to
  `normalize` and overflow throw.
- **`strings`** - `upper lower fold contains startsWith endsWith indexOf
  lastIndexOf count trim trimLeft trimRight replace repeat substring split chars
  join`, rune-indexed (`indexOf(s, sub[, from])`). `fold` strips Latin
  diacritics / ligatures but keeps case: compare `lower(fold(a)) ==
  lower(fold(b))`.
- **`lists`** - `push pop first last head tail reverse sort contains indexOf
  mismatch concat slice shuffle range`, higher-order `map filter reduce find any
  all sortBy` (take `func` values). Non-mutating. `mismatch(a, b[, from])` is
  the first differing index or `-1`.
- **`binary`** - bulk `bytes` operations in Go (`bytes` is a keyword, hence the
  name): `make(n[, fill])` (`n` <= 256 MiB, 32 MiB tiny; the only way to get a sized
  buffer), `concat`, `join(parts[, sep])`, `slice`, `indexOf(haystack, needle[, from[, limit]])`
  (byte offsets; `limit` bounds the scan), `contains`, `split`, `startsWith`,
  `endsWith`, `bitXor` / `bitAnd` / `bitOr(data, key)` (key repeated over the
  data, extra key bytes ignored: a WebSocket mask, an equal-length XOR) and
  `bitNot(data)`.
  `readInt` / `readUint(order, b, offset, size)` (size 1/2/4/8, order
  `"le"`/`"be"`), `readFloat` (4/8; NaN/Inf throw), `readInts` /
  `readFloats(order, b, offset, count, size[, stride])`. Build a
  buffer with `net.readAll` / `readN` or one `join` over a `list of bytes`;
  `concat` in a loop is O(n^2), `$out[] = byte` linear (~0.35 us per byte).
- **`maps`** - `keys values has delete merge`, `without(m, keys)` (several
  keys, one copy). `has` before reading a key that may be missing.
- **`os`** - `getEnv` / `setEnv` / `unsetEnv` / `hasEnv` (true even when set to
  `""`), `hasFlag` / `flag`, `isTerminal`, `cwd` / `homeDir` / `tempDir`.
  Processes: `run(argv[, stdin]) -> Result{exitCode, stdout, stderr}`
  (blocking) and `spawn(argv[, options]) -> Process{handle, pid}` with `wait` /
  `poll` / `kill` / `release`; `runBytes` / `waitBytes` return
  `BytesResult{exitCode, stdout, stderr as bytes, truncated}`; `options` maps `"stdout"` / `"stderr"` to
  `"capture"` (default), `"inherit"`, `"discard"` or a path containing `/`.
  Signals: `catchSignal(name)` / `gotSignal(name)` for `"int"` / `"term"` / `"hup"` /
  `"usr2"` (`"usr1"` is reserved). Constants `PLATFORM ARCH NCPU EOL DIRSEP
  PATHSEP ARGS`.
- **`path`** - path strings, no I/O: `base dir ext stem join clean isAbs`,
  `split(p) -> [dir, file]`, with the host separator. Not a filename sanitizer.
- **`json`** - `encode` / `encodePretty` / `decode` into an opaque `json.Value`,
  read by JSON Pointer (`get` / `asInt` / `asString` / `typeOf` / `has` / `keys`
  / `length` / ...) and edited by non-mutating `set` / `insert` / `append` /
  `remove` / `move`. `typeOf(v, ptr)` is `"missing"` for an absent pointer. A
  repeated key is a decode error. `json.list(xs)` / `json.map(m)` build a
  document in one step: collect first, then convert, instead of a `set` per
  item.
- **`asn1`** - BER decode / DER encode, shaped like `json`: `decode(bytes) ->
  asn1.Value` with child-index pointers (`typeOf` / `tagClass` / `tagNumber` /
  `isConstructed` / `get` / `has` / `length` / `asInt` / `asBool` / `asString`
  / `asBytes` / `asOid` / `isNull`); build with `integer` / `enumerated` /
  `boolean` / `null` / `octetString` / `utf8String` / `printableString` /
  `ia5String` / `oid` / `sequence` / `set`, `tagged` (explicit) / `retag`
  (implicit), then `encode(v)`.
- **`toml`** - TOML 1.0 `encode` / `encodePretty` / `decode`, the `json` API
  name for name, plus `asDatetime`. Jennifer's config format.
- **`xml`** - `decode` / `encode` / `encodePretty` over an `xml.Value` element
  tree: `tag` / `text` / `attr` / `hasAttr` / `attrs` / `children` / `typeOf`;
  paths `name`, `name[k]` (1-based), `*`, `*:local` in `get` / `findAll` /
  `has`; build with `element` / `setAttr` / `setText` / `append`. Namespaces:
  `localName` / `namespace` / `lookupPrefix` / `attrNS` / `hasAttrNS`; match by
  URI with a prefix map as the third path argument (`xml.findAll($r,
  "k:Document", {"k": KML_URI})`); `xml.decode($frag, {"p": URI})` decodes a
  fragment with bindings; `elementNS` / `setAttrNS` build. A C0 control
  other than tab / LF / CR, U+FFFE or U+FFFF is an error in decode and build.
- **`yaml`** - YAML 1.2 `decode` (one document) / `decodeAll` / `encode` (flow)
  / `encodePretty` (block), the `json` API name for name, plus `asDatetime` /
  `isDatetime` / `isNull`. Anchors, aliases and `<<` merges resolve.
- **`intl`** - message catalogs: `load(lang, catalog)` (first is the default),
  `setLocale` / `locale`, `tr(key[, params])` with `%name%` placeholders (`%%`
  for `%`) and fallback locale -> base language -> default -> the key. Not
  `i18n`; there is no `_()`.
- **`httpd`** `[full]` - HTTP/1.1 server engine, a pull loop: `listen(addr) -> Server` /
  `listenTLS(addr, cert, key)` (+ HTTP/2), or `listenWith` / `listenTLSWith`
  with `Options{maxBodyBytes, maxInFlight, respondTimeoutMs, bodyTimeoutMs,
  writeTimeoutMs, maxHeaderBytes, dirListing}` (0 = default: 10 MiB body, 256
  in flight, 60 s respond -> `503`, 60 s body -> `408`). `maxInFlight x
  maxBodyBytes` must fit a process budget (4 GiB; `setMaxBufferBudget(bytes)` /
  `setMaxBufferBudgetFromRAM(fraction)`). Loop `accept($srv) -> Request` and
  `respond($req, status, body)`; `method` / `path` / `query` / `rawQuery` /
  `header` / `body` / `remoteAddr`, `setHeader`, `serveFile` / `serveDir`
  (`serveFileEtag` / `serveDirEtag` add a content-hash `ETag`), `etag($req,
  tag)` (true = 304 sent, stop), `isOpen` / `timedOut`, per-request
  `setRequestValue` / `requestValue`, `shutdown`. Paths never contain `.` /
  `..`, and path / query / headers are valid UTF-8 (else 400); `serveDir` hides dotfiles except `.well-known/`. `spawn` several
  accept loops for a pool.
- **`term`** `[full]` - `makeRaw(stream) -> State` / `restore(state)`,
  `size(stream) -> Size{rows, cols}` (throws off a terminal), `readByte()`
  (`-1` at EOF), `readByteTimeout(ms)` (`-2` on timeout, `0` polls). Not in the
  REPL. Output-only TUIs need just `ansi` + `os.isTerminal`.
- **`serial`**, **`spi`**, **`i2c`**, **`gpio`** `[full]`, Linux only -
  `serial.open(path, baud) -> Port` (`read` / `write` / `flush` / `close`,
  `openWith` for termios); `spi.open(path) -> Device`, `configure(dev, mode,
  speedHz)`, `transfer(dev, bytes)`; `i2c.open(path, addr) -> Bus`, `read` /
  `write` / `readReg(bus, reg, n)` / `writeReg`; `gpio` over
  `/dev/gpiochipN` (`setup` / `read` / `write` / `release`, `IN` / `OUT`).
- **`sql`** `[full]` - MySQL / MariaDB / PostgreSQL (no SQLite):
  `open(driver, dsn) -> Connection`, `query` / `exec` on a Connection or Tx,
  cursor `next` + `asInt` / `asFloat` / `asString` / `asBool` / `asBytes` /
  `isNull`, `begin` / `commit` / `rollback`, prepared statements. Values bind
  only through placeholders.
- **`time`** - `Time` / `Duration` / `Zone`. `now`; `unix` / `unixMillis` /
  `unixNanos` and `fromUnix` / `fromUnixMillis` / `fromUnixNanos`; `year`
  `month` `day` `hour` `minute` `second` `nanosecond` `weekday`; durations
  `fromSeconds` / `fromMilliseconds` / `fromMinutes` / `fromHours` and
  `seconds` / `milliseconds` / `minutes` / `hours` (not `toMillis`); `add` /
  `sub` / `before` / `after` / `equal`; `zone` / `inZone` / `local` / `utc`;
  `sleep`; strftime `format` / `parse`; `iso` / `fromIso`; `UTC`,
  `PROGRAM_START`. Fixed-offset zones only (no IANA / DST).
- **`fs`** - `read` / `write` / `append` (String / Bytes forms),
  `writeNew(path, content)` (fails if it exists: the lock primitive), `exists` /
  `isFile` / `isDir` / `stat` / `realpath` / `readlink` / `symlink`, `mkfifo`,
  `chmod` / `chown`, `mkdir` / `mkdirAll` / `remove` / `removeAll` / `rename`
  (atomic replace: write a temp file, then rename) / `list` / `walk`,
  `makeTempFile` / `makeTempDir`, `fs.File` handles (`open` / `readLine` / ... /
  `sync` / `close`), polling `watch -> Watcher` (`next` / `hasEvent` /
  `close`).
- **`net`** `[full]` - TCP `connect(addr[, timeoutMs])` / `listen` / `accept` /
  `readBytes` (empty once the peer closed) / `writeBytes` / `readAll` / `readN`;
  `setReadDeadline` / `setWriteDeadline` (ms, `0` clears; arm both to bound an
  exchange). Errors: kind `"timeout"`, `"closed"`, else `"runtime"` (test the
  kind, not the message). TLS `connectTLS` / `startTLS` (+ `TLSOptions{
  skipVerify, caCert}`); UDP `listenUDP` / `sendTo` / `recvFrom -> Datagram{data,
  peer, truncated}` / `setBroadcast` / `bindToDevice` (Linux); DNS
  `lookup` / `reverseLookup`; `close` / `address`.
- **`regex`** - RE2: `matches` / `find` / `findAll` / `replace` / `split` /
  `escape`; `Match{text, start, end, groups, groupsNamed}` (rune indices,
  `start == -1` = no match). Patterns are cached.
- **`hash`** - `compute(algo, b)` (`"md5"` / `"sha1"` / `"sha256"` / ...), `hmac(algo,
  message, key)`, constant-time `equal`, streaming `stream` / `update` /
  `finalize`.
- **`crc`** - CRC-32 / CRC-64 over `bytes`, shaped like `hash`.
- **`crypto`** - `randBytes(n)` / `randInt(lo, hi)` (crypto-grade),
  `hkdf` / `pbkdf2`, AES-256-GCM `encrypt` / `decrypt`, Ed25519
  `signKeypair` / `sign` / `verify`, X25519 `x25519Keypair` / `x25519(priv,
  peerPub)` (run through `hkdf`) / `x25519Public`; `[full]`: RSA / ECDSA
  `rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify`, `rsaGenerateKey` /
  `ecGenerateKey` / `jwkPublic` / `jwkToPem` / `csr`, ML-KEM-768
  `mlkemKeypair` / `mlkemEncapsulate` / `mlkemDecapsulate`.
- **`compress`** - `pack` / `unpack` for `"gzip"` / `"zlib"` / `"deflate"`
  (level `"fast"` / `"default"` / `"best"`), streaming `compress.Stream`.
  `unpack(algo, b[, maxBytes])` caps output (256 MiB default, kind `"limit"`):
  pass `maxBytes` for untrusted input.
- **`images`** - opaque immutable 8-bit RGBA `images.Image`. `decode(bytes[,
  maxPixels])` (PNG / JPEG / GIF; 64 Mpx default, kind `"limit"`), `info(bytes)
  -> Info{format, width, height}`, `encode("png" | "jpeg" | "gif", img[,
  quality])`; `create(w, h[, color])` / `fromPixels(w, h, rgba)`; `width` /
  `height` / `pixel(img, x, y) -> Color{r, g, b, a}` / `pixels(img)`; `rgb` /
  `rgba` / `hex("#rrggbb[aa]")` (a `Color{}` literal's omitted `a` is 0);
  `crop` / `resize(img, w, h[, "catmullrom" | "bilinear" | "nearest"])` (0 keeps
  the aspect) / `fit` / `rotate` / `flip` / `grayscale` / `threshold` / `dither`
  / `fill` / `draw(dst, src, x, y)`. No `setPixel`: edit `pixels` and use
  `fromPixels`.
- **`archive`** - tar / zip / tar.gz over `bytes`: `pack` / `unpack` of `list
  of Entry{name, data, mode, mtime}`. `unpack` refuses absolute names, `..`
  and special files (`unpackWith(format, b, UnpackOptions{skipSpecial: true})` skips
  them).
- **`encoding`** - `isAscii` / `lenBytes` / `lenRunes`; `toText` / `fromText`
  with `hex` / `base32` / `base64` / `base64-url` / `ascii85` / `z85` /
  `quoted-printable` / `uri-percent` / `uri-form`; charsets `encode` / `decode`
  (`ascii`, `iso-8859-*`, `windows-*`, `ebcdic`). Exact codec names.
- **`uuid`** - `v4()` (122 random bits, crypto-grade) / `v7()` (time-ordered,
  not a secret), `parse` / `isValid` / `version`, `NIL`.
- **`meta`** - `VERSION` / `BUILD` / `SYSMODDIR` / `CAPABILITIES`,
  `hasCapability(name)`; `call(name, args...)` / `defined(name)` and
  `callMain` / `definedMain` (resolve in the entry program, so a module can
  call its host's handlers).
- **`testing`** - `run(name)` (catches `exit`), `assertEqual` / ... /
  `assertThrows` / `assertThrowsWith(name, args, kind)` (kind `"assertion"`),
  `results` / `reset`, `report` (`text` / `tap` / `junit`). `jennifer test`
  builds on it.
- **`kv`** - in-process key/value store with TTL: `open()` / `openFile(path)` ->
  `kv.Store`; `set(store, k, v, ttl)` / `add` / `get` / `has` / `keys(store[,
  prefix])` / `delete` / `touch` / `incr(store, k, delta)` (signed delta, `-1`
  when absent, never creates) / `close`; `setValue` / `getValue` share an opaque
  value (a `json.Value`) without copying (in-memory only). A `Store` is a handle:
  copies, `spawn`ed tasks and modules share one map (thread-safe, `incr` atomic),
  the shared mutable state a `.j` module cannot hold.

For exact signatures see the hosted
[cheatsheet](https://jennifer-lang.dev/libraries/cheatsheet.html) or the
[per-library pages](https://jennifer-lang.dev/libraries/index.html).

## Module library (Jennifer-coded, brought in with `import`)

Distributable `.j` modules that ship with Jennifer: ordinary Jennifer source,
distinct from the Go libraries above. They live in the system module dir, so
`import "NAME.j";` resolves with no path (`import "./NAME.j" as NAME;` for a
local copy); address them as `NAME.fn(...)` / `NAME.Struct`. Conventions for
every entry below:

- **`[full]`** marks a module (or part) that needs the default (full)
  `jennifer` binary: networking, `term`, `sql`, RSA / ECDSA. Everything else
  also runs on the constrained `jennifer-tiny` build.
- **Builders are value-semantic:** they return the updated value, so thread it
  (`$f = feed.add($f, $e);`). Inside a function that is an in-place update;
  at top level or in a `try` each call copies the whole value, so prefer the
  bulk form (`addAll`, `setMany`, ...) where one exists.
- **Errors** are `Error{kind: "<module>"}` unless an entry says otherwise.
- Names that are keywords are avoided: `listObjects` / `readDir`, not `list`.

- **`acme`** `[full]` - ACME (RFC 8555) client for Let's Encrypt and compatible
  CAs: `connect(directoryUrl, accountKey)` -> `register(client, email)` ->
  `order(client, domains)` -> `authorization` + `challenge(kind, authz)` ->
  HTTP-01 `keyAuthorization(client, token)` / DNS-01 `dnsRecord(client, token)`
  -> `accept` + `pollAuthorization` -> `finalize(client, order, csr, ...)` (a
  `crypto.csr`) -> `downloadCertificate`. JWS `RS256` / `ES256`. Test against a
  staging endpoint first.
- **`amqp`** `[full]` - AMQP 0-9-1 (RabbitMQ). `connect(amqp.options(host, user,
  password))` (`withPort` / `withVhost`) -> `Conn`; `declareQueue(c, name,
  durable) -> QueueInfo{name, messageCount, consumerCount}` /
  `declareQuorumQueue` / `declareExchange` / `bindQueue`; `publish(c, exchange,
  routingKey, body)` / `publishText` (+ `Properties`); `get(c, queue, autoAck) ->
  Message{empty, deliveryTag, exchange, routingKey, body}` (loop until `empty`);
  consumer via blocking `receiveDelivery`; `ack` / `nack`; confirms
  `confirmSelect` / `waitConfirm`; `close`. One channel, SASL PLAIN,
  `Options.security` a `transport.Security` (no `Starttls`).
- **`ansi`** - terminal styling as string wrappers: `color(s, name)` /
  `bgColor` / `style(s, name)` (bold / dim / italic / underline / reverse) /
  `color256` / `bgColor256` / `rgbToColor256(r, g, b)` / `rgb` / `strip`, plus
  shortcuts (`ansi.red(s)`, `ansi.bold(s)`); wrappers nest. Off unless a TTY;
  `NO_COLOR` disables, `FORCE_COLOR` forces.
- **`args`** - declarative CLI parser over `os.ARGS`: `parser(prog, help) -> Parser` then
  `flag` / `intFlag` / `floatFlag` / `boolFlag` (long, short, default, help),
  `countFlag` (`-vvv`), `listFlag` (repeatable), `positional` /
  `positionalOpt` / `positionalList` / `positionalList1` / `positionalN`,
  `required(p)` / `choices(p, allowed)`, `command(p, name, help, sub)`,
  `version(p, ver)`. `parse($p, os.ARGS) -> Result`, read with `asString` /
  `asInt` / `asFloat` / `asBool` / `asList` / `count` / `has` (+ `$r.command`).
  `-h` / `--help` / `--version` set `$r.done` with `$r.helpText` to print; bad
  input throws. `dispatch($r, handlers)` runs the chosen subcommand's `func(r as
  Result)` from a `map of string to func`.
- **`barcode`** - barcodes as images (`label` emits printer commands instead).
  `encode(symbology, data, opts) -> Symbol`: `"qr"` (`opts.ecLevel` L/M/Q/H),
  `"datamatrix"`, `"code128"` / `"code93"` / `"ean13"` / `"ean8"` / `"upca"` /
  `"upce"` / `"itf"` / `"code39"` / `"gs1-128"`. Render `svg(sym, opts)` /
  `png(sym, opts) -> bytes` / `terminal(sym)` (2D) / `matrix(sym) -> list of list
  of bool`; `defaults()` -> `Options` (scale / height / quiet / ecLevel / colours
  / humanReadable).
- **`bloom`** - Bloom filter of strings: `new(size, hashes)` /
  `optimal(n, fpr)`, `add` / `addAll`, `mightContain(f, item)` (no false
  negatives), `serialize` / `deserialize`, `union` / `merge` (same shape).
- **`cron`** - cron expressions, a pure calculator (no clock): `parse(expr) ->
  Schedule`, `matches(schedule, t)`, `next(schedule, after) -> time.Time`. Five
  fields with `*` `,` `-` `/n`, month / day names, `@daily` / `@hourly` / ...
  macros. Scheduling is your own `spawn` + `time.sleep` loop.
- **`csv`** - RFC 4180, strict: `parse(s)` / `format(rows)`, `parseWith` /
  `formatWith` (any one-character delimiter), `toRecords` / `fromRecords`
  (header-keyed maps), `formatSafe(rows)` (defuses spreadsheet formulas), a
  `Dialect` (`dialect(delim)` -> `parseDialect` / `formatDialect`: delimiter,
  quote, comment, trim, `width` 0 = as the first record / n / -1 = any),
  streaming `reader` / `readRow` / `writer` / `writeRow` over an `fs.File`.
  Malformed quoting, ragged records and mismatched record keys throw `csv`
  errors with line and column; `format` joins records with CRLF.
- **`docblock`** - the `/** ... */` doc-comment format (summary, description,
  `@param` / `@field` / `@return` / `@throws` / `@since` / `@deprecated` /
  `@see` / `@example` / `@internal` / `@module`, types in `{...}`):
  `parse(source) -> FileDoc` with `Diagnostic`s for drift / orphans.
- **`doctree`** - the document tree `markdown` / `rst` / `myst` build, and its
  renderers. A `Node` (`kind`, `children`, `attrs`) is walked with `typeOf` /
  `children` / `text` / `level` / `attr` / `get` / `findAll` / `has`.
  `render("html" | "xhtml" | "ansi", doc)`, `renderHtml(doc, HtmlOptions{
  allowRawHtml, xhtml})`; `renderPdf(doc, opts) -> bytes` / `renderPdfDoc ->
  pdf.Document` with `pdfDefaults()` -> `PdfOptions` (page size, fonts,
  metadata, `bookmarkLevel`, `headingStyles`, `admonitionLabels`, `images`,
  `imageDpi`); `pageBreak()`. Kinds beyond Markdown: `line_block` / `line`,
  `footnote` / `citation` / `footnote_ref` / `citation_ref`, `def_desc`, `figure` / `caption` / `legend`, `topic`,
  `rubric`, `container`, `span` / `cite` / `abbr` / `line_break` /
  `html_inline`; `id` / `class` attrs reach the HTML, lists take `start` /
  `enumtype`, tables `headerRows`, cells `colspan` / `rowspan`. PDF images: put
  `pdf.loadImage` results in `PdfOptions.images` keyed by URL (a lone-image
  paragraph is drawn; otherwise `[alt]` text).
- **`dot`** - Graphviz DOT text: `digraph(name)` / `graph(name)`, `node` /
  `nodeWith(g, id, attrs)`, `edge(g, src, dst)` / `edgeWith`, `graphAttr` /
  `nodeAttr` / `edgeAttr`, `render(g)`. Layout is Graphviz's job.
- **`dotenv`** - `.env` files: `parse(text)` / `read(path) -> map`, `load(path)`
  (also `os.setEnv`). Cascade `.env` -> `.env.local` -> `.env.<profile>` ->
  `.env.<profile>.local`, a real env var always winning: `readCascade(dir,
  profile)` / `resolve` / `loadCascade` (sets only unset keys) / `autoload(dir)`
  (profile from `JENNIFER_ENV`). `#` comments, `export`, single / double
  quotes (double: escapes, multi-line), backward `${VAR}` interpolation.
- **`feed`** - RSS 2.0 and Atom 1.0. `Feed{title, link, updated, entries,
  author, categories}` of `Entry` (+ `Enclosure{url, length, type}` for
  podcasts); `feed(title, link)` / `entry(title, link)` / `add` / `addAll` /
  `feedUpdated` / `entryId` / `entryPublished` / `entrySummary` /
  `entryContent` / `entryAuthor` / `entryCategory` / `entryEnclosure` /
  `hasEnclosure`; `build("rss" | "atom", f)` / `parse(text)` / `kind(text)`.
  Safe on untrusted feeds.
- **`flatdb`** - a crash-atomic JSON file store (small data, not a database).
  `open(path) -> DB` (empty if absent) / `openString(text)` (read-only); JSON
  Pointer `get` / `has` / `keys` / `length` / `set` / `append` / `remove`;
  `$db = flatdb.save($db)` (temp + fsync + rename; throws on a conflicting
  writer) / `saveAs(db, path)`. Several writers: `update(path, transform)` runs a
  locked read-modify-write (`func(db as DB) -> DB`); raw `lock` (5 s) /
  `lockWith(path, waitMs)` / `unlock`.
- **`font`** - TrueType / OpenType parser: `parse(b)` / `open(path) -> Font`;
  `unitsPerEm` / `name` / `advance(f, cp)` / `kern(f, l, r)` / `ascender` /
  `descender` / `lineGap` / `capHeight` / `xHeight`; `glyphPath(f, cp)` (SVG
  `d`) / `glyph(f, cp) -> Glyph`. `glyf` and CFF outlines (incl. CID-keyed);
  no shaping, `CFF2`, variable axes or hinting.
- **`gpio`** - Linux sysfs GPIO, pin-keyed: `setup(pin, "in" | "out")` /
  `write(pin, 0 | 1)` / `read(pin)` / `release(pin)`. Root `/sys/class/gpio`,
  overridable with `JENNIFER_GPIO_BASE`.
- **`graphql`** `[full]` - `client(endpoint)`, `bearer` / `basic` / `header` /
  `withCA` / `insecure` / `withTimeout` / `withRetries` / `withBackoff`;
  `query(c, query, variables) -> json.Value` (data under `/data`; an `errors`
  array, even on HTTP 200, throws). `tryQuery` returns the envelope for partial
  data (`hasErrors` / `errorMessages` / `errorCodes`); `queryNamed` /
  `tryQueryNamed` add `operationName`. Retries apply to queries, never
  mutations.
- **`html`** - build and render escaped HTML5: `element(tag, attrs, children)` /
  `text(s)` / `raw(s)` / `attr(n, v)` / `boolAttr(name)`, `render` /
  `renderAll` / `renderXhtml` / `renderAllXhtml`; `escape(s)` (text and quoted
  attributes); `safeUrl(url)` (http / https / mailto, else `"#"`: gate every
  data-driven `href` / `src`). Tolerant `parse(src)` into the same `Node`,
  queried with `get(node, sel)` / `findAll` / `has` (`/`-paths, `*`,
  `name[k]`) + `attrOf` / `hasAttr`.
- **`http`** `[full]` - HTTP/1.1 client; only `http://` / `https://` URLs.
  `request(method, url, headers, body)` / `requestWith(..., timeoutMs)` and
  `get` / `post(url, contentType, body, headers)` / `put` / `patch` / `delete`
  / `head` / `options` -> `Response{status, statusText, headers, body}`
  (headers lowercased; `header(resp, name)`). Bytes: `getBytes` /
  `requestBytes` -> `BytesResponse`, upload `requestRawBody` /
  `requestRawBodyTls`. Policy: `send(method, url, headers, body, Options{
  timeoutMs, maxBytes, maxRedirects, maxRetries, backoffMs, tls,
  allowCrossOriginRedirect, allowInsecureRedirect})` follows redirects, retries
  429 / 5xx (`Retry-After`), keeps cookies; `maxBytes` caps the body.
  Keep-alive: `connect(url, options) -> Session`, `exchange(s, method, path,
  headers, body) -> Exchange{response, session}` (thread `$s = $x.session;`),
  `close`. TLS: `requestTls` / `requestWithTls` + `TlsOptions{skipVerify,
  caCert}`; `basic(user, pass)`; Digest (RFC 7616, SHA-256 / MD5):
  `sendDigest(method, url, headers, body, options, user, password)` answers
  a 401 challenge, `digestAuthorization(challenge, method, url, user,
  password, body)` builds the header for any other call.
- **`ical`** - iCalendar (RFC 5545): a `Calendar` of `Event` / `Todo`.
  `calendar()`, `event(uid, start, end, summary)`, `describe` / `locate` / `withAllDay` / `withZone(ev, tzid)` /
  `recur(ev, ical.rule(...))` / `addRdate` / `addExdate` / `withOrganizer` /
  `addAttendee(ev, attendee(...))` / `addAlarm(ev, alarm(...))` / `add` /
  `addTodo`; `occurrences(ev, max)` expands FREQ / INTERVAL / COUNT / UNTIL +
  RDATE - EXDATE; `encode(cal)` / `parse(text)` round-trip.
- **`idna`** - `toAscii(domain)` / `toUnicode` (Punycode, `münchen.de` <->
  `xn--mnchen-3ya.de`) / `isAscii`; enforces label and domain lengths.
- **`imap`** `[full]` - IMAP4rev1. `connect(opts) -> Session`; `folders(s,
  pattern)`, `status(s, folder)`, `selectFolder`, `search(s, criteria)` ->
  UIDs, `fetch(s, uid)` (raw) / `fetchMessage` (`mime.Part`) / `fetchPartial`;
  `addFlags` / `removeFlags` / `flags` / `copy` / `move` / `append` /
  `createFolder` / expunge; `logout`, `fetchAll(opts, folder)`. Messages are
  addressed by stable UID. `criteria() -> Criteria` filters server-side (`subject` / `from`
  / `to` / `text`, `since` / `before`, flags, size) and client-side
  (`subjectRegex` / `hasAttachments`). IDLE: `idle(s)` -> `receiveNotification`
  / `pollNotification` (`EXISTS` / `EXPUNGE` / `RECENT`) -> `done`.
- **`ipnet`** - IPv4 / IPv6: `parseAddress(s) -> Address` (v4-mapped folds to
  v4), `toString` (RFC 5952), `version` / `equal` / `unmap` / `next` / `prev` /
  `compare`; `parse(cidr) -> Network`, `contains`, `netmask` / `broadcast`,
  `networkString`, `hostCount`, `firstUsable` / `lastUsable`, `hosts` /
  `split(net, prefix)` (capped 65536), `aggregate`, `overlaps` / `subnetOf`;
  `scope(addr) -> Scope` enum (`Global` / `Private` / `Loopback` /
  `LinkLocal` / `Multicast` / `Unspecified` / `Reserved`) + `isGlobal` /
  `isPrivate` / ... predicates.
- **`jsonl`** - JSON Lines: `encode(records)` / `decode(text) -> list of
  json.Value` (errors name the line), `readFile` / `writeFile` / `appendFile`,
  streaming `openReader` / `hasMore` / `readRecord` / `closeReader` and
  `writer` / `writeRecord` / `closeWriter` over an `fs.File`.
- **`jsonrpc`** `[full]` - JSON-RPC 2.0. `client(endpoint)` /
  `clientWith(endpoint, headers)`, `call(c, method, params) -> json.Value`,
  `notify`. `handle(requestBody) -> replyBody` dispatches to top-level `func
  NAME(params as json.Value)` (batches capped at 1000, notifications, reserved
  error codes); transport-agnostic.
- **`jwt`** - JWT (RFC 7519). `sign(alg, claims, key)` / `signWithKid`,
  `verify(alg, token, key)`, `decode` / `header` (unverified: never authorize on
  them). Claims a `json.Value` object, `key` always `bytes`. `HS256/384/512`
  (key >= 32 / 48 / 64 bytes: `crypto.randBytes(32)`), `RS*`, `ES*`, `EdDSA`. `verify` pins the expected
  algorithm, checks `exp` / `nbf`, compares constant-time; rejections are kind
  `"value"`. `verifyWith(alg, token, key, Policy{iss, aud, requireExp, leeway})` /
  `verifyLeeway`; key by `kid`: `verifyWithKeys(alg, token, keysByKid)` /
  `verifyJwks(alg, token, jwksJson)` / `jwksKeys` (convert once), with
  `verifyWithKeysWith` / `verifyJwksWith` taking a `Policy`. For OIDC / CI
  tokens use a `Policy`: without one `iss` / `aud` are unchecked and a missing
  `exp` passes. `RS*` / `ES*` are `[full]`.
- **`kvstore`** - one key/value API over selectable backends with per-key TTL:
  `Store` enum from `memcacheStore(mc)` / `redisStore(rc)` `[full]` /
  `inProcessStore()` / `fileStore(path)`; `set(store, key, value, ttl)` / `get`
  / `delete` / `touch` / `incrWindow`. `ttl <= 0` means no expiry. Backs
  `session` and `ratelimit`.
- **`label`** - label printing. `new(w, h)` in mm, then `text(label, x, y,
  TextOptions{height, points, rotation, bold}, content)` / `barcode(label, type,
  x, y, BarcodeOptions{height, moduleWidth, ratio, checkDigit, errorLevel,
  hideText}, data)` (code128 / ean13 / ean8 / itf / code39 /
  gs1-128 / datamatrix / qr) / `box` / `image(label, x, y, name)` / `quantity`.
  `render(label, device)` with `zpl(dpi)` (Zebra) or `cab()` / `cabWith(
  CabSetup)`; `send(host, port, rendered)` `[full]` writes to port 9100.
- **`ldap`** `[full]` - LDAP v3 client and server. `connect(address, security)
  -> Conn`, `bind(c, dn, password)` / `bindSasl(c, algo, user, password) ->
  Result{code, matchedDn, message}`, `search(c, baseDn, scope, filter, attrs)`
  (`SCOPE_BASE` / `SCOPE_ONE` / `SCOPE_SUB`) / `searchPaged` -> `list of Entry`
  (`values` / `firstValue`). Filters: `parseFilter("(&(a=b)(c=*))")` or
  `equals` / `present` / `substrings` / `allOf` / `anyOf` / `negate` / ...;
  escape values with `escapeFilterValue`. Writes `add` / `modify` (`change`,
  `MOD_ADD` / `MOD_DELETE` / `MOD_REPLACE`) / `delete` / `modifyDn` /
  `passwordModify`; `unbind` / `close`. Server: `directory(entries)` /
  `openDirectory(path)`, `entry` / `group` / `password`, `addEntry` /
  `modifyEntry` / `getEntry`, `serve(dir, address)`.
- **`log`** - leveled structured logging. `new(format, level) -> Logger` (stdout) /
  `toStderr` / `toFile(format, level, path)` / `toSyslog(level, address, app)` `[full]`;
  levels `debug` < `info` < `warn` < `error`, formats `text` / `logfmt` /
  `json`. `debug(logger, message, fields)` / `info` / `warn` / `error` /
  `at(logger, level, ...)`; `with(logger, fields)` adds persistent fields;
  `fatal` logs and exits 1.
- **`mactelnet`** `[full]` - MikroTik MAC-Telnet: a RouterOS console by MAC
  address, no IP needed (bootstrap a fresh router). `connect(iface, mac, user,
  password) -> Session` (MD5 or EC-SRP login, auto-detected); `send(s, text)`
  (end lines with `\r\n`) / `recv(s, timeoutMs)` / `close`; `parseMac` /
  `formatMac`. Linux only; binding to `iface` needs root (skipped otherwise).
  Cleartext.
- **`markdown`** - CommonMark 0.31.2 reader (644 / 652 spec examples) plus GFM
  tables, task lists, `~~strike~~`, footnotes, `~sub~` / `^sup^` /
  `==mark==`, definition lists, GitHub alerts (`admonition` nodes), `{#id
  .class}` attributes. `parse(md) -> doctree.Node` (walk and render with
  `doctree.*`; `markdown.render` / `renderPdf` do not exist). Shortcuts:
  `toHtml(md)` (raw HTML escaped; `toHtmlWith(md, opts)` with `allowRawHtml`
  for trusted input), `toXhtml`, `toAnsi`, `toPdf` / `toPdfWith(md,
  doctree.PdfOptions)`. Authoring: `header` / `style` / `link` / `bullets` /
  `numbered` / `codeBlock` / `table`, `tablePretty`.
- **`mcp`** `[full]` - Model Context Protocol (stateless JSON-RPC). Server:
  `server(name, version)`, `addTool(s, name, desc, inputSchema, handler)` /
  `addResource` / `addPrompt` (handlers are top-level `func NAME(arg as
  json.Value)`), schemas via `schema()` + `property(...)`; `handle(server,
  body) -> reply` (only registered handlers run), `serveStdio(server)`. Client:
  `connect(endpoint)` / `connectWith`, `initialize` / `listTools` /
  `callTool(c, name, args)` / `listResources` / `readResource` / `listPrompts`
  / `getPrompt`. No SSE / sessions.
- **`memcache`** `[full]` - memcached text protocol. `connect(opts) -> Session`;
  `set(s, key, value, exptime)` / `add` / `get` (`""` if absent) / `delete` /
  `incr` / `decr` (`-1` if absent) / `touch` / `quit`; `setBytes` /
  `getBytes`; `getMulti(s, keys)`; `gets -> Item{value, cas, found}` + `cas(...,
  casId)` -> `"stored"` / `"exists"` / `"not_found"`.
- **`mikrotik`** `[full]` - RouterOS binary API. `connect(options(host, user,
  password))` or `optionsTLS` (port 8729, certificate verified; `withCA(opts,
  pem)` to pin, `insecure(opts)` as a last resort) -> `Session`. `talk(s,
  command, attrs) -> list of map` (one map per `!re`), `print(s, path)`,
  `run(s, command, attrs) -> string` (`=ret=`), `talkQuery` / `printWhere`
  (`?` query words), `.tag`-correlated commands, `/listen` streams (`listen` /
  `receiveReply` / `cancel`).
- **`mime`** - MIME messages. Build `text(contentType, body)` / `attachment` /
  `attachmentBytes(filename, contentType, data)` / `multipart(subtype,
  boundary, parts)` / `withHeader`, then `encode(part)`; `parse(text)`, read with
  `headerValue` / `body` (charset-decoded) / `parts` / `contentType` /
  `address`, take apart with `walk` / `attachments` / `textBodies` /
  `findParts(part, mediaType)` + `data` / `filename` / `disposition` /
  `isAttachment`. RFC 2231 filenames and RFC 2047 words (`encodeWord` /
  `decodeWord`) round-trip.
- **`mqtt`** `[full]` - MQTT 3.1.1 (`mqtts` = TLS). `connect(opts) -> Client`, `subscribe` /
  `publish(c, topic, message)` / `publishBytes` (QoS 0), `publishQos1` /
  `subscribeQos1`, retained messages, Last-Will via `connectWith`, blocking
  `receive(c) -> Message{topic, payload}`, `poll(c, timeoutMs)` (0 or 1),
  `ping` / `disconnect` / `reconnect`.
- **`multipart`** - `multipart/form-data`: `field(name, value)` / `file(name,
  filename, contentType, data)` -> `Part`; `build(parts)` / `buildWith(parts,
  boundary)` -> `Built{contentType, body}`; `parse(contentType, body) -> list of
  Part`, `text(part)` / `isFile(part)`. Bodies are `bytes`.
- **`myst`** - MyST Markdown (Sphinx / Jupyter Book) on top of `markdown`:
  directives in ```` ```{name} ```` or `:::{name}` fences (options as `:key:`
  lines or a `---` block); roles (`{sub}`, `{abbr}`, `{kbd}`, `{math}`,
  `{pep}`, `{rfc}`); `(label)=` targets with `{ref}` / `{numref}` / `{eq}` /
  `{doc}`; `%` comments; `+++`; `frontMatter(src)`; `$x$` / `$$...$$ (label)`.
  Directives: admonitions, `code-block`, `math`, `image`, `figure`, `topic` /
  `sidebar`, `rubric`, quotes, `container`, `table`, `list-table`,
  `csv-table`, `raw` (escaped unless `allowRawHtml`), `contents`, `eval-rst`.
  `parse` / `parseWith(src, Options{strict, allowRawHtml, noDollarMath,
  noColonFence})`, `toHtml` / `toHtmlWith` / `toXhtml` / `toAnsi`, `check(src) ->
  list of Diagnostic`. Write MyST as a raw `'...'` string: cooked strings
  interpolate `{...}`.
- **`ntp`** `[full]` - SNTP client: `query(host)` (port 123, 5 s) /
  `queryWith(address, timeoutMs)` -> `Result{serverTime, offset, delay}`.
  Measures only; does not set the clock.
- **`oauth`** `[full]` - OAuth2 token client: `clientCredentials(cfg)` /
  `refresh(cfg, refreshToken)` / device flow `deviceStart(cfg)` ->
  `deviceWait(cfg, dev)` -> `Token`; JWT bearer (RFC 7523)
  `jwtAssertion(alg, issuer, subject, audience, lifetime, extra, key, kid)` +
  `jwtBearer(cfg, assertion)`. `Config` presets `google` / `microsoft`, `isExpired`,
  `save` / `load` (file mode 0600). No auth-code + PKCE. Tokens feed
  `sasl.bearer`.
- **`orm`** `[full]` - data mapper over `sql` (structs have no methods). Schema:
  `schema(table, pk, dialect)` + `column(s, name, kind)` (`Dialect.Mysql` /
  `Postgres`; `ColumnKind.Int` / `String` / `Float` / `Bool` / `Bytes`), with
  `notNull` / `unique` / `autoIncrement` / `withDefault`. CRUD on a `Session`
  (`session(conn)` / `transaction(tx)`): `insert` / `find` / `update` /
  `delete` / `all`; records are `map of string to string`. Queries:
  `from($schema)` -> `select` / `count` / `aggregate` -> `where` / `orWhere` /
  `whereIn` / `whereNotIn` / `whereNull` / `whereBetween` -> `join` /
  `leftJoin` / `rightJoin` -> `groupBy` + `having` -> `orderBy` / `limit` /
  `offset` / `page` -> `toSql($q) -> Rendered{sql, params}`; identifiers and
  operators are allowlisted. DDL `createTable` / `dropTable` / `addColumn` /
  `createIndex` / `addForeignKey`; relations `belongsTo` / `hasOne` / `hasMany`
  / `manyToMany` + `joinRelation`, eager `with` + `load -> Result` (`rows` /
  `related` / `relatedOne`); `upsert` / `insertMany` / `insertReturning` /
  `updateWhere` / `deleteWhere` / `save`, `first` / `exists` / `findBy` /
  `pluck`.
- **`password`** - `schema()` (16 chars, four classes) with `withLength(s, lo,
  hi)` / `withClasses` / `withMinimums` / `withSymbolSet` /
  `withoutAmbiguous`; `generate(schema)` (crypto-grade, length <= 4096),
  `validate(schema, pw) -> Report{valid, reasons}`, `complexity(pw) ->
  Strength{length, classes, poolSize, entropy, label}`.
- **`pdf`** - PDF generation. `document()`, `page(w, h)`, `text(pg, x, y, font,
  size, str)` / `line` / `rect(pg, x, y, w, h, filled)` / `color(pg, r, g, b)`,
  `addPage`, `render(doc) -> bytes` (byte-identical, no timestamp); `info(doc,
  key, value)` + `pdfDate(t)`, `bookmark(doc, page, y, title, level)`.
  Standard-14 fonts; points, origin bottom-left. Non-Latin text: `loadFont` /
  `addFont` / `textUnicode` (TrueType only). Images: `loadImage` / `addImage` /
  `drawImage` (PNG, JPEG, GIF), `imageFrom(name, images.Image)`. Layout:
  `measureText` / `measureTextUnicode`, `wrapText`, `textBlock(pg, x, y, width,
  font, size, leading, str, align)` / `textBlockUnicode` (left / right /
  center / justify).
- **`plot`** - SVG charts. `chart(series, opts)` over `series(name, xs, ys)`
  (`mark` line / points / both / area, `dash`, `yErr`, `shape`); wrappers `line`
  / `scatter` / `bar(labels, values, opts)` / `histogram(data, bins, opts)` /
  `bars` (grouped / stacked). `defaults() -> Options`: size, title, `xLabel` / `yLabel`,
  colours, fonts, margins, `legendPos`, `xLog` / `yLog`, `xDate` /
  `dateFormat`, tooltips; `hline` / `vline` reference lines; `floats(ints)`;
  `save($svg, path)`.
- **`pop`** `[full]` - POP3: `connect(opts) -> Session`, `stat` / `count` /
  `sizes` / `retrieve(s, n)` / `deleteMessage` / `top(s, n, lines)` / `reset` /
  `noop` / `quit`, `uidl` / `uidlOne` (stable ids), `fetchAll(opts)`. Auth per
  `Options.auth`: USER/PASS, APOP, XOAUTH2, CRAM-MD5, SCRAM-*, `"auto"`.
- **`ratelimit`** - `fixedWindow(store, limit, window)` / `slidingWindow` ->
  `Limiter` over a `kvstore.Store`; `check(l, key) -> Result{allowed,
  remaining, retryAfter, resetSeconds}` (records) / `peek` (does not).
- **`redis`** `[full]` - RESP2. `connect(opts) -> Session`; `get` / `set` / `del`
  / `exists` / `incr` / `decr` / `ping`, `setBytes` / `getBytes`, hashes
  `hset` / `hget` / `hgetAll` / `hdel`, lists `lpush` / `rpush` / `lrange` /
  `llen` / `lpop`, sets `sadd` / `srem` / `smembers` / `sismember` / `scard`;
  `command(s, args) -> Reply{kind, str, num, items}`; pub/sub `subscribe` /
  `psubscribe` / `publish` / `receiveMessage`; `pipeline`, `multi` / `exec` /
  `discard`; `scan` (use it, not `keys`, in production). `-ERR` throws.
- **`resque`** `[full]` - Resque-compatible jobs on Redis: `enqueue(s, queue,
  class, args)`, `reserve(s, queues) -> Job{queue, class, args}` (empty `class`
  when drained) / `reserveBlocking(s, queues, timeoutSec)`, `queueLength` /
  `queues` / `size` / `fail`. The dispatch loop is yours.
- **`rest`** `[full]` - REST over `http` + `json`: `client(baseUrl)` with
  `bearer` / `basic` / `withHeader` / `withTimeout` / `withRedirects` /
  `withRetries` / `withBackoff` / `withCA` / `insecure`; `get(c, path, query)`
  / `post(c, path, contentType, body)` / `put` / `patch` / `delete` ->
  `Response`; `getJson` / `postJson` / `putJson` / `patchJson`; `paginate(c,
  path, query, maxPages)` (Link `rel="next"`, same origin only) /
  `paginateCursor`. Path values are not escaped: `uri.encode` each segment.
- **`ringbuffer`** - bounded FIFO of strings that overwrites the oldest:
  `new(capacity)`, `push` / `pop` (O(1)), `first` / `last` / `size` /
  `capacity` / `isEmpty` / `isFull` / `toList` (oldest first). Read `first`
  before `pop`.
- **`rst`** - reStructuredText into a `doctree.Node`: sections, all list kinds,
  literal / doctest blocks, quotes, line blocks, transitions, docinfo, grid and
  simple tables (spans), roles, references and targets, footnotes, citations,
  substitutions. Directives: admonitions, `code` / `code-block`, `math`,
  `image`, `figure`, `topic` / `sidebar`, `rubric`, quotes, `container`,
  `class`, `raw`, `contents`, `list-table`, `csv-table` (`:class:` / `:name:`
  on each). `parse` / `parseWith(src, Options{...})`, `toHtml` / `toHtmlWith` /
  `toXhtml` / `toAnsi`, `title(doc)`, `docinfo(doc)`; lenient: `check(src) ->
  list of Diagnostic{severity, line, message}`, `Options{strict: true}` throws the first. No file I/O
  (`include` refused).
- **`s3`** - S3-compatible storage (AWS / MinIO / R2 / B2), SigV4, path-style.
  `connect(endpoint, region, accessKey, secretKey)`; `get` / `put` / `delete` /
  `head` / `copy` / `listObjects` (+ `objectKeys(xml)`, `isTruncated` /
  `nextContinuationToken` / `listObjectsFrom`) -> `http.Response`; `getBytes` /
  `putBytes`, `putWith` / `putBytesWith(..., contentType, metadata)`;
  multipart `createMultipartUpload` / `uploadPart` /
  `completeMultipartUpload` / `abortMultipartUpload`. Requests are `[full]`;
  `presign(client, method, bucket, key, expiresSeconds)` is not. Bucket names
  are validated and an empty key is refused.
- **`sasl`** - mail auth mechanisms: `plain(user, pass)` / `loginUser` /
  `loginPass` / `bearer(user, token)` (XOAUTH2), `cram(user, pass, challenge)`,
  SCRAM-SHA-1 / -256 `scramStart(algo, user)` -> `scramClientFirst` ->
  `scramClientFinal` -> `scramFinalToken` -> `scramVerify`;
  `negotiate(advertised)` picks the strongest.
- **`screen`** - terminal UIs. Output (both binaries): a cell `Buffer`
  (`newScreen(rows, cols)`) drawn with `text` / `textColor` / `box` / `fill` /
  `hline` / `vline` / `set`, batch with `setMany(buf, list of Plot{x, y,
  cell})` / `textMany(buf, list of Span{x, y, text, color})`; ANSI builders
  `clear` / `moveTo(x, y)` / `hideCursor` / `enterAlt` / ...; `render(buf)` /
  `diff(old, new)`. Input `[full]` (`term`): `decodeKey(seq) -> Key{name,
  char}`, `nextKey` / `begin` / `end` / `size`; non-blocking
  `startInput() -> Input` + `hasKey` / `pollKey` / `waitKey`. 0-based, clipped;
  single-width cell text only; control characters are written as spaces.
- **`semver`** - SemVer 2.0.0: `parse` / `isValid` / `toString`, `compare` /
  `lt` / `eq` / `gt` / ... / `diff`, `isStable` / `isPrerelease`, `incMajor` /
  `incMinor` / `incPatch`, `sort` / `rsort`, `coerce` / `clean`; ranges
  `satisfies(version, range)` (`^`, `~`, comparators, `||`, hyphen, x-ranges),
  `maxSatisfying` / `minSatisfying`, `minVersion`, `validRange`, `intersects` /
  `subset` / `gtr` / `ltr` / `outside` / `simplifyRange`.
- **`session`** - server-side sessions over a `kvstore.Store`: a `json.Value`
  under `sess:ID` with a sliding TTL. `create(store, ttl) -> id`, `load` (empty
  object if absent), `save(store, id, data, ttl)`, `touch`, `destroy`,
  `exists`, `rotate(store, id, ttl)` (call on login / logout). `ttl` >= 1.
  Check `exists` before `save` on a client-sent id (fixation).
- **`smtp`** `[full]` - `send(opts, from, recipients, message)` (EHLO, STARTTLS /
  TLS via `Options.security`, auth per `Options.auth`: PLAIN / LOGIN / XOAUTH2 /
  CRAM-MD5 / SCRAM-*, MAIL / RCPT / DATA); `message` from `mime`. Cleartext auth
  refused unless `allowInsecureAuth`. Several messages: `open(opts) ->
  Session`, `sendOn(s, from, recipients, message)`, `close`.
- **`snmp`** `[full]` - SNMP v1 / v2c client and agent. `client(host,
  community)` / `clientWith(address, community, version, timeoutMs, retries)`;
  `get(c, oids)` / `getNext` / `set(c, varbinds)` / `walk(c, rootOid)` -> `list
  of Varbind{oid, type, value, number}` (`integer` / `octetString` / `oid` /
  `counter32` / `gauge32` / `timeTicks` / `counter64` / ...). Agent: `agent(community, version,
  bindings)`, `serve(a, address)` / `serveOn(a, socket, stop)`; `intVar` /
  `stringVar` / `oidVar` / `varbind`; `VERSION1` / `VERSION2C`. No v3 / traps.
- **`soap`** `[full]` - SOAP 1.1 / 1.2 over HTTP. `connect(endpoint)` ->
  `Client` (`withVersion(c, "1.2")` / `withBasicAuth` / `withToken` /
  `withDigestAuth` / `withHeader` / `withSoapHeader(c, block)` / `withCaCert` /
  `withMaxBytes`);
  `call(c, action, body as xml.Value) -> xml.Value` (first Body child; a fault
  throws) / `send -> Reply{status, envelope, isFault, fault}`; `operation(ns,
  name, params, qualified)` builds, `values(node)` reads. Server: `handle(body,
  action, handler)` / `handleWith(..., understood)` -> `Response{status,
  contentType, body}`, `handler(req as Request{version, action, headers,
  body})` returns the result element, `senderFault(reason)` for a Client
  fault; `requestAction(contentType, soapAction)`. Codec: `envelope(version,
  headers, body)` / `parse -> Envelope` / `parseFault -> Fault{code, subcode,
  reason, actor, detail}` / `faultEnvelope`. No WSDL.
- **`sqlmigrate`** `[full]` - schema migrations over `sql`: `Migration{version,
  description, up, down}` (DDL strings); `migrate(conn, migrations)` (lexical
  version order, one transaction each, tracked in `schema_migrations`),
  `rollbackMigrations(conn, migrations, steps)`, `migrationStatus`. The list is
  validated before anything runs.
- **`tengine`** - Go `text/template` subset over a `json.Value`: `newSet()`,
  `add(set, name, src)`, `render(set, entry, data)`. `.a.b` / `$` / `$var`;
  `if` / `else if` / `else` (`eq` / `ne` / `lt` / `and` / `or` / `not`), `range`
  / `with` (+ `else`), `{{ $x := ... }}`, `template` / `block`, comments, `{{-
  -}}`; pipes `upper` / `lower` / `title` / `trim` / `html` / `urlize` /
  `default` / `truncate` / `join` / `len` / `printf`. Not auto-escaped: pipe
  HTML output through `html`. Nesting and output (10 MiB) are capped.
- **`totp`** - RFC 6238 codes: `generate(secret, opts)` / `verify(secret,
  code, opts)` (+/-1 step) / `generateAt` / `verifyAt` / `verifyWindow`;
  `uri(issuer, account, secret, opts)` (`otpauth://`); `generateSecret()` /
  `generateSecretN(n)`; `hotp(secret, counter)`. Zero `Options` = 6 digits, 30
  s, SHA-1; else set `digits` / `period` / `algorithm`. Secrets must decode to >= 10 bytes. Replay protection and rate
  limiting are the caller's job.
- **`transport`** - `Security { None, Tls, Starttls }`, the `security` field
  of every socket client's `Options` (`smtp.Options{security:
  transport.Security.Starttls, ...}`); `encrypted(s)`. `redis` / `amqp` /
  `mqtt` reject `Starttls`.
- **`uri`** - RFC 3986: `parse(raw) -> Uri{scheme, user, host, port, path,
  query, fragment}` (splits, does not validate) / `build(u)`; `encode` /
  `decode` (`%20`), `encodeForm` / `decodeForm` (`+`); `buildQuery(map)` /
  `parseQuery(q)`; `resolve(base, ref)`. Bad `%` escapes throw.
- **`validate`** - rules over a `map of string to string`: `required` /
  `isInt` / `isFloat` / `isBool` / `min(n)` / `max(n)` / `minLen` / `maxLen` /
  `pattern(re)` (anchor it yourself) / `email` / `url` / `datetime(format)` /
  `oneOf` / `noneOf` / `password(schema)` / `custom(fn, msg)` /
  `withMessage(r, m)`, in a `map of string to list of Rule`. `check(data,
  rules) -> list of Failure{field, rule, param, message}`, `ok`, `messages` /
  `byField`, `localize(errs, templates)`, `checkValue(field, value, rules)`;
  `ensure` / `ensureValue` throw kind `"validation"`. Absent / blank fields
  pass every rule but `required`.
- **`vcard`** - vCard 4.0: `card(formattedName)`, `withName(c, family, given)`
  / `withFullName` / `withNickname` / `withOrg(c, org, title)` / `addEmail` /
  `addEmailTyped` / `addPhoneTyped` / `addAddress(c, address(...))` /
  `withBday` / `withPhoto` / `addCategory` / `withUrl` / `withNote`;
  `encode(c)` / `encodeAll(cards)` / `parse(text) -> list of Card`.
- **`web`** `[full]` - HTTP framework over `httpd`. Routes take handler `func`
  values: `get($app, "/users/:id", showUser)` / `post` / `put` / `patch` /
  `delete` / `route` (`:param`, trailing `*rest`); `before` middleware,
  `notFound`, `onError` (a throw becomes a logged 500); HEAD is served by GET.
  Handlers are `func name(ctx as web.Context)` and run concurrently: never let
  two write one top-level `def`. Context: `param` / `query` / `rawQuery` /
  `method` / `path` / `header` / `body` / `bodyJson` / `form` / `formValue` /
  `remoteAddr` / `remoteHost`; replies `text` / `html` / `sendJson` /
  `redirect` / `respond` / `setHeader` / `serveFile` / `serveDir` / `sendGzip`;
  `isOpen(ctx)`. Cookies `cookie` / `setCookie` + `CookieOptions`; sessions
  `sessionId` / `renewSession`; `cors` + `CorsOptions`; `etag`; `basicAuth -> BasicCredentials` /
  `bearerToken`; CSRF `csrfToken` / `csrfCheck` (non-empty secret; one token
  per request); `mount($app, prefix, sub)` / `joinRoute`. `run($app, addr)` /
  `serveOn`; `jennifer serve app.j [--watch]`.
- **`webapi`** `[full]` - JSON-API layer over `web`. Builder `new() -> Api` / `mount(a,
  version, path)` / `alias` / `deprecate` / `authenticator(a, fn)` /
  `limiter(a, fn)` / `limitKey(a, fn)` / `get` / `post` / ... with a `Spec{auth
  (Auth.None / Auth.Bearer), scopes, rules (validate), rateLimit, produces
  (Produces.Json / Html / Negotiate)}` (`public()` = zero) / `install(a, app,
  guard)`. Wire the guard with a shim: `func apiGuard(ctx) { return
  webapi.guard($api, $ctx); }`. It authenticates (401), checks scopes (403),
  validates (422) and rate-limits (429 + `Retry-After`); `evaluate(spec,
  identity, data)` is the pure core. Read checked input only through
  `validated(a, ctx)`. Also `fail` / `notFound` / `denied` / `unauthorized`,
  `queryData` / `jsonData` / `identity`, `wants` / `negotiate(ctx, offered)`,
  `page` / `sendPage`, `discovery`.
- **`webdav`** `[full]` - WebDAV client (Nextcloud, ownCloud, Apache, nginx,
  NAS). `connect(url)` (paths relative to it) + `withBasicAuth` / `withToken` /
  `withHeader` / `withCaCert`; `readDir` / `walk` / `stat` / `exists` ->
  `Resource{href, path, name, isDir, size, modified, etag, contentType,
  quotaUsed, quotaAvailable, props}`; `get(c, path, maxBytes) -> bytes`,
  `put(c, path, data) -> etag`, `putWith(..., PutOptions{contentType, ifMatch,
  onlyIfNew})`, `mkdir` / `mkdirAll` / `delete` / `move(c, src, dst,
  overwrite)` / `copy`; any property by Clark name (`clark(ns, local)`) via
  `propfind(c, path, depth, props)` / `proppatch`; codec `parseMultistatus` /
  `multistatus`.
- **`webhook`** - HMAC webhooks: `sign(payload, secret)` (`sha256=` hex, the
  GitHub header) / `verify(payload, signature, secret)`; timestamped
  `stripeSign` / `stripeVerify`, `slackSign` / `slackVerify`,
  `timestampedSign` / `timestampedVerify` (reject stale timestamps). Sign the
  raw body. An empty secret throws. `send(url, payload, secret)` `[full]`.
- **`websocket`** `[full]` - RFC 6455 client: `connect(url)` /
  `connectWith(url, timeoutMs)` / `connectOptions(url, Options{timeoutMs,
  headers, protocols, caCert, skipVerify})` -> `Conn` (`Conn.protocol`);
  `send(c, text)` / `sendBytes`, `receive(c) -> Message{kind, text, data}` (kind
  text / binary / close / pong; pings answered, fragments joined), `ping` /
  `close`.
- **`zmq`** `[full]` - ZeroMQ (ZMTP 3.1) without libzmq; one socket is one TCP
  connection to one peer. `connect(kind, "host:port")` / `connectWith(kind,
  address, Options{mechanism, username, password, identity, timeoutMs,
  maxMessageBytes})` (`defaultOptions()`), or `listen` / `listenWith` ->
  `Listener` + `accept(l)` per peer. Kinds `PAIR`, `PUB` / `SUB`, `REQ` /
  `REP`, `DEALER` / `ROUTER`, `PUSH` / `PULL`. `send(sock, frames)` /
  `sendText`, `receive -> Message{frames, envelope}`, `poll(sock, timeoutMs)`,
  `text(msg)`, `reply(sock, request, frames)`; `$s = zmq.subscribe($s,
  prefix)` (SUB gets nothing until subscribed). NULL or PLAIN (needs a
  password); no CURVE, ipc, inproc or reconnect. Oversize is kind `"limit"`.
- **Deprecated, removed in 0.28.0** - `discord`, `gotify`, `influxdb`,
  `prometheus`, `slack`, `statsd` and `telegram` moved to decks (see
  [Decks](#decks-packages)): `jvc add "@jennifer/NAME" ...`, then `import
  "@jennifer/NAME/" as NAME;`.

Full per-module reference: the hosted
[module docs](https://jennifer-lang.dev/modules/index.html).

## Two complete programs

Hello, with a helper and a loop:

```jennifer
use io;

func fib(n as int) {
    if ($n < 2) { return $n; }
    return fib($n - 1) + fib($n - 2);
}

for (def i as int init 0; $i < 10; $i = $i + 1) {
    io.printf("fib(%d) = %d\n", $i, fib($i));
}
```

Structs, a list, and JSON:

```jennifer
use io;
use json;

def struct User {
    name as string,
    age as int
};

def users as list of User init [User{name: "ada", age: 36}, User{name: "bob", age: 41}];

for (def u in $users) {
    io.printf("%s is %d\n", $u.name, $u.age);
}

io.printf("%s\n", json.encode($users));   # [{"name":"ada","age":36},...]
```

## Common mistakes checklist (for the assistant)

- Referenced a variable without `$`? -> add it (`$x`, not `x`).
- Put `$` on a constant or a `def` name? -> remove it.
- Used `//` for a comment? -> that is floor division; use `#`.
- Expected `5 / 2` to be `2`? -> it is `2.5`; use `//`.
- Used `&&`/`||`/`!`? -> use `and`/`or`/`not`.
- Started a variable name with a digit, or used `_` in it? -> not allowed
  (interior digits are fine; only constants take `_`).
- Used `x += 1` or `x++`? -> write `$x = $x + 1;`.
- Called `int(x)` / `string(x)`? -> use `convert.toInt(x)` / `toString`.
- Read a map key that might be absent? -> guard with `maps.has($m, key)`.
- Forgot `use io;` before `io.printf`? -> every library must be imported.
- Expected a mutated copy to change the original? -> value semantics; it will
  not.
