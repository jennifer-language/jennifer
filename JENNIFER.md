# JENNIFER.md - the Jennifer language, for coding assistants

Drop this file into a project where you write **Jennifer** (`.j` files) and
point your AI coding assistant at it ("we code in Jennifer, see JENNIFER.md").
It is a self-contained reference to the language so an assistant with no prior
knowledge of Jennifer can write correct code. It describes the *language*, not
the interpreter's internals.

Jennifer is a small, interpreted language (tree-walking interpreter written in
Go/TinyGo). Source files use the `.j` extension. Run a program with
`jennifer run program.j`, start a REPL with `jennifer repl`.

**Full documentation** - guides, the complete library reference, and an
alphabetical cheatsheet of every builtin - is hosted at
<https://mplx.github.io/jennifer-lang/>. If an assistant has web access, fetch
the exact signature of any function there. Source and issues:
<https://github.com/mplx/jennifer-lang>.

> This file mirrors the authoritative spec. If something here conflicts with the
> [hosted docs](https://mplx.github.io/jennifer-lang/), the docs win - tell the
> maintainer.

---

## The 10 rules that trip people up

Read these first; they are where Jennifer differs from Python/JS/Go and where
an assistant usually guesses wrong:

1. **Variables are referenced with a `$` sigil: `$x`.** But the *declaration*
   uses a bare name: `def x as int init 5;` then use `$x`. Writing `def $x` is
   an error; using bare `x` in an expression is an error.
2. **Constants are referenced bare (no `$`): `MAX`.** They are `UPPER_CASE`.
3. **Method calls are bare and take `()`: `greet()`.** The parser tells a call
   from a constant by the `(`.
4. **`/` is true division and always returns `float`** (like Python 3).
   `5 / 2 == 2.5`. Use `//` for integer/floor division: `5 // 2 == 2`.
5. **Identifiers are letters only, <= 64 chars.** No digits, no underscores in
   variable/method/parameter/library names. `myVar`, not `my_var` or `var2`.
   (Constants are the *only* names that take `_`: `MAX_RETRIES`.)
6. **Statements end with `;`.** Whitespace (including newlines) is
   insignificant everywhere.
7. **Comments are `#` (line) and `/* */` (block, nests).** Not `//` - that is
   the floor-division operator.
8. **No `++`, `--`, `+=`, or any compound assignment.** Only `$x = EXPR;`.
9. **Value semantics: assignment and argument passing copy.** `$b = $a;` then
   mutating `$b` never touches `$a`. Same for lists, maps, structs, bytes.
10. **Logical operators are words: `and`, `or`, `not`** (not `&&`/`||`/`!`).
    `&` `|` `^` `~` are the *bitwise* operators.

---

## Lexical basics

- **Identifiers** (variables, methods, parameters, library names): `[A-Za-z]`,
  <= 64 chars. No digits, no underscores.
- **Constant names**: uppercase chunks joined by single `_`:
  `[A-Z]+(_[A-Z]+)*`. Legal: `MAX`, `MAX_RETRIES`, `HTTP_OK`. Illegal: `_MAX`,
  `MAX_`, `MAX__INT`, `maxInt`.
- **`.j` import paths** are strings and may contain digits, `_`, `/`.
- A leading `#!` line is allowed (shebang): `#!/usr/bin/env -S jennifer run`.

## Types

Primitive: `null`, `int`, `float`, `string`, `bool`, `bytes`.
Compound: `list of T`, `map of K to V`, user `struct`s, `task of T` (a handle
to a `spawn`ed computation).

- **int** literals: `42`, `0xff`, `0o755`, `0b1010`, with `_` digit separators
  (`1_000_000`, `0xDEAD_BEEF`).
- **float** literals: need a `.`: `3.14`, `0.5` (and `_` separators).
- **string** literals: `"..."` or `'...'`; both parse escapes
  `\n \r \t \\ \" \' \0`.
- **bool**: `true`, `false`. **null**: `null`.
- **bytes** has no literal: build with `convert.bytesFromString(s, "utf-8")`
  or append into `def b as bytes;` with `$b[] = 65;`.
- **list** literals: `[1, 2, 3]`, `[]`. Lists are homogeneous (one element
  type).
- **map** literals: `{"a": 1, "b": 2}`, `{}`. Insertion-ordered.
- **struct** literals: `Point{ x: 1, y: 2 }` after
  `def struct Point { x as int, y as int };`. Every field must be named.

## Variables and constants

```jennifer
def x as int;                 # declare, zero value (0)
def y as int init 5;          # declare + initialize
def const MAX as int init 10; # constant, must be initialized, never reassigned
$x = 7;                       # assignment uses the $ sigil
```

- The name at the `def` site is bare (`def x`), never `def $x`.
- `const` is deep: a const list/map/struct rejects mutation at any depth.

## Operators

- Arithmetic: `+  -  *  /  //  %`. `/` is float division; `//` is floor.
- Unary `-` (negation). `+` also concatenates two strings.
- Comparison: `<  >  <=  >=  ==` -> `bool`.
- Logical (words, short-circuit): `and`, `or`, `not`. Operands must be `bool`.
- Bitwise (int only): `&  |  ^  ~  <<  >>`.
- Mixed int/float arithmetic promotes to `float`.
- Precedence, low to high: `or` < `and` < `not` < comparison < `|` < `^` < `&`
  < shifts < `+ -` < `* / // %` < unary `- ~`. So `$x & 0xff == 0` parses as
  `($x & 0xff) == 0`.

## Control flow

```jennifer
if ($n > 0) { ... } elseif ($n < 0) { ... } else { ... }

while ($i < 10) { ... }

for (def i as int init 0; $i < 10; $i = $i + 1) { ... }   # C-style

for (def x in $xs) { ... }     # for-each over a list (elements)
for (def k in $m) { ... }      # for-each over a map (keys, insertion order)

repeat { ... } until ($done);  # post-test loop; body runs at least once

break;      # exit innermost loop
continue;   # next iteration
exit;       # terminate the whole program (exit 0); exit EXPR sets the code
```

Conditions must be `bool` (there is no truthiness). `break`/`continue` do not
cross a method-call or `spawn` boundary.

### Errors

```jennifer
use io;
try {
    throw Error{ kind: "bad", message: "nope", file: "", line: 0, col: 0 };
} catch (e) {
    io.printf("%s\n", $e.message);
}
```

`throw EXPR;` raises any value; convention is the auto-provided `Error` struct
`{kind, message, file, line, col}`. `catch` also catches the runtime errors
builtins raise (out-of-range, missing key, etc.), wrapped into `Error`.
`exit`/`return`/`break`/`continue` are control flow, not catchable.

## Methods

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
- Methods are **top-level only** (not nested). Recursion works.
- Method bodies see global variables/constants. A method may not shadow a
  global name, nor share a name with a builtin from an imported library.
- A program has **no required entry point**: top-level statements run in order.

## Compound types: indexing and iteration

```jennifer
def xs as list of int init [1, 2, 3];
$xs[0];            # read -> 1
$xs[0] = 9;        # write
$xs[] = 4;         # append (write-only; lists and bytes only)

def m as map of string to int init {"a": 1};
$m["a"];           # read (missing key is an error - test with maps.has)
$m["b"] = 2;       # write

def p as Point init Point{ x: 1, y: 2 };
$p.x;              # field read
$p.x = 5;          # field write

$grid[i][j] = v;   # chains nest and mix [index] and .field
```

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

## Imports

```jennifer
use io;                 # enable a system library, addressed io.printf(...)
use strings as s;       # alias: only s.upper(...) works after this
include "helpers.j";    # textual splice of another .j file (preprocessor)
```

- `use NAME [as ALIAS];` - system library. Nothing auto-loads; every program
  states its imports. Aliasing is a rename (the canonical prefix stops working).
- `include "path.j";` - textual file splice (path is a string literal ending in
  `.j`, resolved relative to the including file).
- `import "x.j" as x;` - the module system, **designed but not yet
  implemented** (errors today with a hint).

## Standard library (all namespaced, all opt-in via `use`)

Call as `LIB.name(...)`. Enable with `use LIB;` first. Highlights:

- **`io`** - `printf` / `sprintf` with verbs `%d %f %s %t %v %a` and
  `%verb[|key=value]` modifiers (`pad`, `align`, `base`, `prec`, `sign`,
  `group`, `case`, ...); `readLine`, `eof`, `readBytes`.
- **`convert`** - `toInt toFloat toString toBool`, `typeOf`, `objectType`,
  `bytesFromString` / `stringFromBytes` (utf-8). Note: the callees are
  `toInt` etc. because `int`/`float`/`string`/`bool`/`bytes` are reserved type
  keywords (they appear only after `as`).
- **`math`** - `abs min max sqrt pow floor ceil round rand randInt randSeed`;
  constants `PI`, `E`. Undefined results error (no NaN).
- **`strings`** - `upper lower contains startsWith endsWith indexOf trim
  replace repeat substring split chars join`. Rune-indexed.
- **`lists`** - `push pop first last head tail reverse sort contains concat
  slice shuffle range`. Non-mutating (they return new lists).
- **`maps`** - `keys values has delete merge`. `has` before a missing-key read.
- **`os`** - `getEnv`, `hasFlag`/`flag`, `isTerminal`, `run`/`spawn`;
  constants `PLATFORM ARCH EOL DIRSEP PATHSEP ARGS`.
- **`json`** - `encode`/`encodePretty`/`decode`. `decode` returns an opaque
  `json.Value` walked by JSON Pointer accessors (`get`/`asInt`/`asString`/
  `typeOf`/`has`/`keys`/`length`/...) and edited by non-mutating writers
  (`set`/`insert`/`append`/`remove`/`move`, `map()`/`list()`).
- **`time`**, **`fs`**, **`net`**, **`regex`**, **`hash`**, **`crc`**,
  **`compress`**, **`archive`**, **`encoding`**, **`uuid`**, **`meta`**,
  **`testing`** - clock, files, sockets, RE2 regex, digests, checksums,
  byte-stream + container compression, text/character codecs, UUIDs,
  interpreter identity, and test primitives.

For the exact signature of any function, see the hosted library reference -
the [cheatsheet](https://mplx.github.io/jennifer-lang/libraries/cheatsheet.html)
(every builtin in one table) or the
[per-library pages](https://mplx.github.io/jennifer-lang/libraries/index.html)
(e.g. `.../libraries/json.html`).

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

def struct User { name as string, age as int };

def users as list of User init [
    User{ name: "ada", age: 36 },
    User{ name: "bob", age: 41 },
];

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
- Used a digit or `_` in a variable name? -> not allowed (only constants take
  `_`).
- Used `x += 1` or `x++`? -> write `$x = $x + 1;`.
- Called `int(x)` / `string(x)`? -> use `convert.toInt(x)` / `toString`.
- Read a map key that might be absent? -> guard with `maps.has($m, key)`.
- Forgot `use io;` before `io.printf`? -> every library must be imported.
- Expected a mutated copy to change the original? -> value semantics; it will
  not.
