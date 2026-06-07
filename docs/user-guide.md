# Jennifer - User Guide

Jennifer is a small, experimental, interpreted programming language. This guide
covers everything you can do in Jennifer today ([Milestone 3](milestones.md)).

---

## Installing & running

You need a working [TinyGo](https://tinygo.org/) toolchain (or regular Go for
development). From the repository root:

```sh
# Build the interpreter (TinyGo - the shipping toolchain)
make build

# Or build with Go (faster, for development)
make build-go

# Run a Jennifer source file (must have .j extension)
./jennifer run examples/hello.j

# Print the build version
./jennifer version
```

The `make` targets regenerate `internal/version/version_gen.go` from git
state before invoking the toolchain, so `./jennifer version` always
reflects the current commit. See [libraries/core.md](libraries/core.md)
for the `JENNIFER_VERSION` string format. `JENNIFER_VERSION` is
auto-loaded - it's in scope in every program without any `use` statement.

You can also pipe source in on stdin by passing `-` as the filename:

```sh
echo 'use io; printf("hi\n");' | ./jennifer run -
./jennifer run - < program.j
cat program.j | ./jennifer run -
```

When reading from stdin, error messages identify the source as `<stdin>` and
file imports (`import "name.j";`) resolve relative to the current working
directory.

### Interactive REPL

For experimenting with the language, start an interactive session with
`jennifer repl`:

```text
$ ./jennifer repl
jennifer - Jennifer programming language interpreter
type :quit (or Ctrl-D) to exit; :help for help
>>> use io;
>>> def x as int init 21;
>>> $x + $x;
42
>>> printf("hi\n");
hi
>>> func dbl(n as int) {
...   return $n * 2;
... }
>>> dbl(7);
14
>>> :quit
```

A few notes:

- Statements still end with `;`. If a line ends with an unclosed `{` or `(`,
  the prompt switches to `... ` and waits for you to finish the block.
- A bare expression at the end of an input (like `$x + $x;`) prints its
  value. `null` results (including the return value of `printf`) are
  suppressed.
- String results are printed with surrounding double quotes so they're
  distinguishable from numbers (`"hello"`, not `hello`).
- Variables, constants, methods, and library imports persist for the whole
  session. Methods can be redefined freely as you iterate.
- File imports (`import "lib.j";`) work in the REPL and resolve relative to
  the directory you launched `jennifer repl` from.
- `:quit`, `:exit`, or Ctrl-D end the session; `:help` shows a reminder.

### Inspection and formatting

Three commands help you see what Jennifer is doing under the hood and
keep your source in canonical shape:

```sh
# Print the lexer's token stream, one per line
./jennifer tokens examples/hello.j

# Print the parsed (and preprocessed) AST as JSON
./jennifer ast examples/hello.j

# Reformat the source to canonical style (see docs/stylespec.md)
./jennifer fmt examples/hello.j
```

`fmt` writes the formatted source to stdout. To rewrite in place, use
your shell: `./jennifer fmt foo.j > foo.j.new && mv foo.j.new foo.j`.
The formatter is idempotent (`fmt` of `fmt` output equals `fmt` output)
and preserves runtime behavior - every example in this repo is checked
both ways by the test suite. Current v1 limitations: comments are
dropped, and blank lines aren't preserved or inserted automatically.
See [stylespec.md](stylespec.md) for the full style rules.

For local development you can also use the Go toolchain directly:

```sh
go run ./cmd/jennifer run examples/hello.j
go test ./...
```

---

## Your first program

Save the following as `hello.j`:

```jennifer
// hello.j
use io;

def x as int init 21;
printf($x + $x);
```

Run it:

```sh
./jennifer run hello.j
```

You should see `42`.

### What just happened

1. `use io;` makes Jennifer's standard library functions (only `printf`
   today) available.
2. `def x as int init 21;` declares an integer variable named `x` and
   initializes it to `21`. Notice that **using** a variable requires the `$`
   prefix.
3. `printf($x + $x)` calls the standard library function with the result of
   `21 + 21`.

Top-level statements run in source order - there is no required entry-point
method. You can still group reusable code into methods (`def`) and call them
explicitly.

---

## Language reference

### Tokens and whitespace

Whitespace (spaces, tabs, newlines) is **not** significant. Statements are
terminated by `;`.

### Comments

```jennifer
// line comment - runs to end of line

/* block comment -
   can span multiple lines */
```

### Identifiers

- Variable, method, parameter and library names are letters only: `[A-Za-z]`,
  up to 64 characters. No digits or underscores.
- **Constants** are uppercase chunks joined by single `_` separators:
  `[A-Z]+(_[A-Z]+)*`, up to 64 characters. Every `_` must be immediately
  followed by another uppercase letter. `MAX`, `MAX_RETRIES`, `HTTP_OK`,
  and `A_B_C` are all legal; `_MAX`, `MAX_`, and `MAX__INT` are not.
- **Variable references** use a leading `$`: define `name`, refer to it as
  `$name`.

### Types

| Type     | Example literals                | Notes                                     |
|----------|---------------------------------|-------------------------------------------|
| `int`    | `0`, `42`, `9001`               | 64-bit signed                             |
| `float`  | `3.14`, `0.5`                   | 64-bit; promoted from int in mixed math   |
| `string` | `"hello"`, `'single quotes'`    | Supports escape sequences                 |
| `bool`   | `true`, `false`                 | Produced by comparison operators          |
| `null`   | `null`                          | A type with a single value (the unit)     |

#### String escape sequences

Both `"..."` and `'...'` are valid string delimiters. The following escapes
are recognized:

| Escape | Meaning            |
|--------|--------------------|
| `\n`   | newline            |
| `\r`   | carriage return    |
| `\t`   | tab                |
| `\\`   | backslash          |
| `\"`   | double quote       |
| `\'`   | single quote       |
| `\0`   | null character     |

### Variables and constants

```jennifer
def name as int init 5;            // declare and initialize
def count as int;                  // declare with the zero value of int (0)
def const MAX as int init 100;     // constant: uppercase name, init required
```

Uninitialized variables get the **zero value** of their declared type:
`0`, `0.0`, `""`, `false`, or `null`.

**At the def site, names are bare identifiers (no `$`).** The `$` sigil is
reserved for use-site references that read or assign a variable. So:

```jennifer
def x as int init 5;     // def site - bare name
printf($x);              // use site - $ prefix
$x = 42;                 // assignment - $ prefix

def $x as int init 5;    // ERROR: drop the $ here
```

Constants don't use `$` anywhere (they're not mutable, so the sigil would have
no meaning):

```jennifer
def const MAX as int init 100;
printf(MAX);             // use site - bare name
MAX = 200;               // ERROR: cannot assign to constant
```

Assignment uses `=`:

```jennifer
def x as int init 0;
$x = 42;          // ok
$x = "string";    // error: cannot assign string to int variable
```

#### Scoping

- Each `{...}` block introduces a new scope.
- A binding is visible from its `def` to the end of the enclosing block, and
  is inherited by inner blocks.
- Inner scopes can **read** outer bindings but **cannot redefine** a name
  already in scope (no shadowing). The interpreter rejects shadowing at
  runtime.
- A `for` loop opens a private scope wrapping `init`/`cond`/`step`/body, so
  the loop variable does not leak out.

### Methods

```jennifer
func greet(name as string) {
    printf("hello, %s\n", $name);
}

greet("Jennifer");   // call it from top level
```

Two keywords, two jobs:

- `def [const] NAME ...` introduces a **binding** (variable or constant).
- `func NAME(p as TYPE, q as TYPE) { ... }` introduces a **method**.

**Parameters** use bare identifiers (same rule as `def`) and each has a
declared type. Inside the body, parameters are referenced as `$p` like any
other variable. At the call site, the interpreter checks the number of
arguments and the kind of each one - mismatches produce a positioned error.

**Return values** use `return EXPR;` to return a value or `return;` to return
`null`. A body that runs to the end without `return` also yields `null`.
Methods don't declare a return type; the caller's type check (e.g.
`def x as int init mymethod();`) is what enforces the value's kind at the
use site.

**Recursion** works out of the box - methods are hoisted, so any method can
call any other (or itself) by name.

```jennifer
func fact(n as int) {
    if ($n == 0) { return 1; }
    return $n * fact($n - 1);
}

printf("%d\n", fact(5));    // 120
```

Methods are **hoisted**: all `func NAME() { ... }` declarations are collected
before any top-level statement runs, so a method can be called from anywhere
in the file regardless of where it's defined. There is no required entry
point - top-level statements execute in source order.

Methods can only be defined at the top level (not inside another method's
body). Method bodies inherit the global scope, so top-level variables are
visible inside methods (subject to the no-shadowing rule).

**Methods cannot shadow imported builtins.** If you write `use io;` and
then `func printf() { ... }`, the program is rejected:

```
runtime error at 2:1: method "printf" shadows a builtin from `io`;
rename it or remove `use io;`
```

Without the `use io;`, the name is yours to define. This is the same
no-shadowing discipline Jennifer applies to variables.

### Imports

Two keywords, two mechanisms:

```jennifer
use io;                  // library import - enables `io` library (printf, sprintf)
import "helpers.j";      // file import - splices helpers.j here
```

**Library imports** (`use NAME;`) enable a built-in module. The libraries
that ship today are `io`, `convert`, `math`, `strings`, and `meta`; see
[libraries/index.md](libraries/index.md) for the catalog.

**File imports** (`import "PATH.j";`) textually include another `.j` source
file at the point of import. The path is a **string literal** that must end
in `.j`. Relative paths resolve from the directory of the file containing
the import; absolute paths and subdirectories work:

```jennifer
import "helpers.j";          // sibling file
import "subdir/utils.j";     // subdirectory
import "../shared/util.j";   // parent dir
import "/abs/path/lib.j";    // absolute path
```

File imports may appear anywhere a statement is allowed, including inside a
block:

```jennifer
use io;
import "helpers.j";          // ← spliced here; whatever helpers.j contains lands here
printf($helper_value);
```

Circular imports (file A imports file B, B imports A) are detected and
rejected with an error.

Mixing the keywords produces a helpful error:

```
import io;           → error: use `use io;` for system libraries
use foo.j;           → error: use `import "foo.j";` for files
import foo.j;        → error: file imports take a string literal: `import "foo.j";`
```

Notes:
- The imported file's contents must be valid where the import appears. A file
  containing a top-level `def` cannot be imported inside a block (since
  definitions are only allowed at the top level).

### Operators

| Operator             | Meaning                                                  |
|----------------------|----------------------------------------------------------|
| `+`                  | addition (`int`/`float`); also concatenation on `string` |
| `-`, `*`            | subtraction, multiplication (`int`/`float`)               |
| `/`                  | **true division - always returns `float`**               |
| `div`                | floor (integer) division; `int div int → int`            |
| `%`                  | modulo (`int` only)                                      |
| unary `-`            | numeric negation (`int`/`float`)                         |
| `<`, `>`, `<=`, `>=` | numeric comparison; result is `bool`                     |
| `==`                 | equality; same-kind plus `int`/`float` promotion; `bool` |
| `and`, `or`          | logical; both operands `bool`; short-circuit             |
| `not`                | unary logical negation; operand `bool`                   |

**Division has two operators.** `/` always returns a `float` (Python 3
style). `div` returns the floor, keeping the type when both operands are
ints:

```jennifer
5 / 2          // 2.5 (float)
5 div 2        // 2   (int)
5.0 / 2.0      // 2.5 (float)
5.7 div 2.0    // 2.0 (float - floor of a float division)
```

So `def x as int init 5 / 2;` is rejected (right side is float). Use
`5 div 2` for an int result, or `def x as float init 5 / 2;`.

(`//` would have been the Python 3 spelling, but `//` is line-comment syntax
in Jennifer. The Pascal-style `div` keyword fills the same role.)

Precedence (low to high): `or`, `and`, `not`, comparison, additive (`+`, `-`),
multiplicative (`*`, `/`, `div`, `%`), unary `-`. Use parentheses to override:
`(1 + 2) * 3`. Examples that follow the rules:

```jennifer
not 1 == 2                  // not (1 == 2) -> true
1 > 0 and 2 > 1             // true
true or false and false     // true or (false and false) -> true
-3 + 10                     // (-3) + 10 -> 7
-3 * 2                      // (-3) * 2 -> -6
```

**`and` and `or` short-circuit.** The right operand is only evaluated when
the left doesn't already decide the result. That matters when the right side
has side effects:

```jennifer
def gate as bool init false;
def result as bool init $gate and expensive();   // expensive() not called
```

Mixed `int`/`float` arithmetic promotes the int to float and the result is a
float (`3 + 0.5` -> `3.5`). `int / int` is integer division; if either
operand is a float, division is float division.

### Control flow

```jennifer
if ($n == 0) {
    printf("zero");
} elseif ($n < 10) {
    printf("small");
} else {
    printf("large");
}

while ($i < 5) {
    $i = $i + 1;
}

for (def i as int init 0; $i < 10; $i = $i + 1) {
    printf($i);
}
```

Conditions in `if`, `elseif`, `while`, and `for` **must be `bool`** - there
is no implicit truthiness. Use a comparison (`$x == 0`) to get a bool.

### Libraries

Standard functions live in topic-based libraries enabled with `use`. Nothing
is auto-loaded. Each library has its own reference doc; this table is the
index.

| Library   | Enable with    | Contents                                                        | Reference                |
|-----------|----------------|-----------------------------------------------------------------|--------------------------|
| `io`      | `use io;`      | `printf`, `sprintf` and the format-verb mini-language           | [libraries/io.md](libraries/io.md)             |
| `convert` | `use convert;` | `int`, `float`, `string`, `bool`, `typeOf` - explicit casts     | [libraries/convert.md](libraries/convert.md)   |
| `math`    | `use math;`    | `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`; constants `PI`, `E` | [libraries/math.md](libraries/math.md)         |
| `strings` | `use strings;` | `len`, `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring` | [libraries/strings.md](libraries/strings.md)   |
| `core`    | *(auto-loaded)* | `JENNIFER_VERSION` - the interpreter's build version string. No `use` needed; `use core;` is a runtime error. | [libraries/core.md](libraries/core.md)         |

See [libraries/index.md](libraries/index.md) for a catalog with code
samples and the library-organization principles.

Quick orientation - if you're reading top to bottom and just want a flavor:

```jennifer
use io;
use convert;
use math;

printf("%s\n", typeOf(5 / 2));         // "float"      [convert]
printf("%d\n", floor(PI * 2.0));       // 6            [math + io]
printf("%s\n", string(true));          // "true"       [convert]
```

The per-library docs cover every function in detail along with error cases.

---

## Example: strings

```jennifer
// greeting.j
use io;

def name as string init "Jennifer";
printf("hello, " + $name + "!\n");
```

Output:

```
hello, Jennifer!
```

## Example: FizzBuzz

```jennifer
// fizzbuzz.j
use io;

for (def i as int init 1; $i <= 15; $i = $i + 1) {
    if ($i % 15 == 0) {
        printf("FizzBuzz\n");
    } elseif ($i % 3 == 0) {
        printf("Fizz\n");
    } elseif ($i % 5 == 0) {
        printf("Buzz\n");
    } else {
        printf("%d\n", $i);
    }
}
```

## Example: Factorial (recursion + parameters)

```jennifer
// factorial.j
use io;

func fact(n as int) {
    if ($n == 0) { return 1; }
    return $n * fact($n - 1);
}

for (def i as int init 0; $i <= 8; $i = $i + 1) {
    printf("%d! = %d\n", $i, fact($i));
}
```
