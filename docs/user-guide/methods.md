# Methods

```jennifer
func greet(name as string) {
    io.printf("hello, %s\n", $name);
}

greet("Jennifer");   # call it from top level
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

io.printf("%d\n", fact(5));    # 120
```

Methods are **hoisted**: all `func NAME() { ... }` declarations are collected
before any top-level statement runs, so a method can be called from anywhere
in the file regardless of where it's defined. There is no required entry
point - top-level statements execute in source order.

Methods can only be defined at the top level (not inside another method's
body). Method bodies inherit the global scope, so top-level variables are
visible inside methods (subject to the no-shadowing rule).

**Methods cannot shadow imported builtins.** If you write `use io;` and
then `func io.printf() { ... }`, the program is rejected:

```
runtime error at 2:1: method "printf" shadows a builtin from `io`;
rename it or remove `use io;`
```

Without the `use io;`, the name is yours to define. This is the same
no-shadowing discipline Jennifer applies to variables.

## Methods as values

A method name is also a **function value**: written bare (no `(`), it evaluates
to a `func` you can store, pass, and call later - `def f as func init greet;`
then `$f(args)`. This is what powers the higher-order `lists` helpers
(`lists.map` / `filter` / `reduce` / ...). See
[Function values](types-and-values.md#function-values) for the full rules.
