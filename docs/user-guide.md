# Jennifer - User Guide

Jennifer is a small, experimental, interpreted programming language. This guide
covers everything you can do in Jennifer **today** (Milestone 1). Features
planned but not yet implemented are listed under [Coming next](#coming-next);
for the full roadmap see [milestones.md](milestones.md).

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
import stdlib;

def app() {
    define $x as int init 21;
    printf($x + $x);
}
```

Run it:

```sh
./jennifer run hello.j
```

You should see `42`.

### What just happened

1. `import stdlib;` makes Jennifer's standard library functions (only `printf`
   today) available.
2. `def app() { ... }` defines the entry method. Every Jennifer program needs an
   `app()`.
3. `define $x as int init 21;` declares an integer variable named `x` and
   initializes it to `21`. Notice that **using** a variable requires the `$`
   prefix.
4. `printf($x + $x)` calls the standard library function with the result of
   `21 + 21`.

---

## Language reference (M1)

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

### Types (M1)

Only two of Jennifer's planned types are usable in Milestone 1:

| Type     | Example literals               | Notes                      |
|----------|--------------------------------|----------------------------|
| `int`    | `0`, `42`, `9001`              | 64-bit signed              |
| `string` | `"hello"`, `'single quotes'`   | Supports escape sequences  |

Coming in M2: `float`, `null`, `bool` (added by request - extends the original
spec).

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

### Variables

```jennifer
define $name as int init 5;        // declare and initialize
```

In M1 the `init` clause is **required**. Defining a variable without an
initializer arrives in M2.

#### Scoping rules (full rules apply once blocks/methods land in M2/M3)

- A variable is visible from the point of `define` to the end of the enclosing
  block.
- Inner scopes can read outer variables but **cannot redefine** a name that is
  already in scope (no shadowing). This is enforced by the interpreter.
- Constants (`define const NAME as TYPE init VALUE;`) arrive in M2.

### Methods

```jennifer
def app() {
    // body
}
```

`def` and `define` are **synonyms** - you can use either keyword for both
variable definitions and method definitions:

```jennifer
def app() {              // method using `def`
    def $x as int init 1; // variable using `def`
    define $y as int init 2; // variable using `define`
}
define helper() {}       // method using `define`
```

M1 only supports zero-argument methods, and methods can only be defined at the
top level (not inside another method's body). Parameters, return values, and
calling methods from other methods arrive in M3.

`app()` is the entry point: the interpreter calls it after collecting all
top-level method declarations.

### Imports

Two forms:

```jennifer
import stdlib;          // library import - enables stdlib functions like printf
import helpers.j;       // file import - splices the contents of helpers.j here
```

**Library imports** (`import NAME;`) enable a built-in module. Today only
`stdlib` exists.

**File imports** (`import NAME.j;`) textually include another `.j` source file
at the point of the import. The file is resolved relative to the directory of
the file containing the import. File imports may appear anywhere a statement is
allowed, including inside a block:

```jennifer
def app() {
    import helpers.j;   // ← spliced here; whatever helpers.j contains lands here
    printf($helper_value);
}
```

Circular imports (file A imports file B, B imports A) are detected and
rejected with an error.

Notes:
- File names follow the identifier rule (`[A-Za-z]`), so `myblock.j` is fine
  but `my_block.j` is not (yet).
- The imported file's contents must be valid where the import appears. A file
  containing a top-level `def` cannot be imported inside a block (since method
  definitions are only allowed at the top level in M1).

### Operators

All M1 operators work on `int` operands and produce an `int` result.

| Operator | Meaning           |
|----------|-------------------|
| `+`      | addition          |
| `-`      | subtraction       |
| `*`      | multiplication    |
| `/`      | integer division  |
| `%`      | modulo            |

Precedence: `*`, `/`, `%` bind tighter than `+`, `-`. Use parentheses to
override: `(1 + 2) * 3`.

Comparison operators (`< > <= >= ==`) and `if`/`while`/`for` arrive in M2.

### Standard library

`printf(value)` writes `value` to standard output without a trailing newline.
In M1 it accepts exactly one argument. Use multiple `printf` calls (or `"\n"`)
to format multi-line output. Format specifiers (`%d`, `%s`, `%f`) come in M3
alongside multi-argument calls.

---

## Worked example: strings

```jennifer
// greeting.j
import stdlib;

def app() {
    define $name as string init "Jennifer";
    printf("hello, ");
    printf($name);
    printf("!\n");
}
```

Output:

```
hello, Jennifer!
```
