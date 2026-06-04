# Jennifer - User Guide

Jennifer is a small, experimental, interpreted programming language. This guide
covers everything you can do in Jennifer today ([Milestone 3](milestones.md)).

---

## Installing & running

You need a working [TinyGo](https://tinygo.org/) toolchain (or regular Go for
development). From the repository root:

```sh
# Build the interpreter
tinygo build -o jennifer ./cmd/jennifer

# Run a Jennifer source file (must have .j extension)
./jennifer run examples/hello.j
```

You can also pipe source in on stdin by passing `-` as the filename:

```sh
echo 'use io; printf("hi\n");' | ./jennifer run -
./jennifer run - < program.j
cat program.j | ./jennifer run -
```

When reading from stdin, error messages identify the source as `<stdin>` and
file imports (`import "name.j";`) resolve relative to the current working
directory.

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

## Language reference (M3)

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

- Variable and method names are letters only: `[A-Za-z]`, up to 64 characters.
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

**Library imports** (`use NAME;`) enable a built-in module. Today only
`io` exists; M4 will add `math`, `strings`, and `convert`.

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
is auto-loaded.

| Library    | Enable with  | Functions                                    |
|------------|--------------|----------------------------------------------|
| `io`       | `use io;`    | `printf`, `sprintf`                          |
| `convert`  | `use convert;` | `int`, `float`, `string`, `bool`, `typeof` |

#### `io` library

`printf` and `sprintf` share a Go-style format-string mini-language.

```jennifer
printf("hi\n");                              // literal string
printf($x);                                  // single value, displayed
printf("you are %d years old!\n", $age);     // format + arguments
printf("%s = %d, ok=%t\n", "answer", 42, true);
```

`sprintf` takes the same arguments and **returns** the formatted string instead
of writing to stdout:

```jennifer
def msg as string init sprintf("%d + %d = %d", 1, 2, 3);
printf("%s\n", $msg);   // "1 + 2 = 3"
```

Format verbs:

| Verb | Required kind  | Notes                                  |
|------|----------------|----------------------------------------|
| `%d` | `int`          | decimal                                |
| `%f` | `float`        | shortest round-trip                    |
| `%s` | `string`       | raw                                    |
| `%t` | `bool`         | `true` / `false`                       |
| `%v` | any            | uses the value's display form          |
| `%%` | -              | literal `%`                            |

Mismatches (wrong verb for the value kind, too few or too many args, dangling
`%`, unknown verb) all produce runtime errors. A literal `%` in any string
passed to `printf`/`sprintf` must be doubled to `%%`.

Floats always display with a decimal point so the value's type stays visible:
`5.0` prints as `"5.0"`, not `"5"`. That's particularly important after the
Python 3 division change - `4 / 2` is the float `2.0`, and you can tell at
a glance.

#### `convert` library

Explicit value-kind conversion. Each function takes one argument and either
returns the converted value or errors at runtime.

```jennifer
use io;
use convert;

def n as int init int("42");       // parse string -> 42
def f as float init float(5);      // int -> 5.0
def s as string init string(3.14); // any -> "3.14"
def b as bool init bool(0);        // 0 -> false, nonzero -> true

printf("%s\n", typeof(5 / 2));     // "float" (after Python 3 / change)
printf("%s\n", typeof(5 div 2));   // "int"
```

| Call          | Source kinds                       | Behavior                                                |
|---------------|------------------------------------|---------------------------------------------------------|
| `int(v)`      | int / float / string / bool        | identity / truncate / parse / `true`=1, `false`=0       |
| `float(v)`    | int / float / string / bool        | convert / identity / parse / `true`=1.0, `false`=0.0    |
| `string(v)`   | any                                | always succeeds; uses the value's display form          |
| `bool(v)`     | bool / int / float / string        | identity / canonical only (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`) |
| `typeof(v)`   | any                                | returns kind as string: `"int"`, `"float"`, etc.        |

Errors:
- `int("abc")` - parse failure
- `int(null)` - no conversion defined
- `bool("maybe")` - strings: only `"true"` and `"false"` accepted
- `bool(123)`, `bool(-1)` - ints: only `0` and `1` accepted
- `bool(1.5)` - floats: only `0.0` and `1.0` accepted
- arity errors (too many/few arguments)

For "any nonzero counts as true" semantics, write the comparison explicitly:

```jennifer
def b as bool init $count != 0;     // not bool($count)
```

This matches the strict-conditions rule everywhere else in Jennifer - if
you want to project an arbitrary value into a bool, you state the criterion.

The type-name calls (`int`, `float`, `string`, `bool`) only work in expression
position when immediately followed by `(`. Writing `def x as int init int;`
errors with a hint to either use it as a conversion call or use a value.

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
