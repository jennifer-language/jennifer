# Functions

```jennifer
use io;

func greet(name as string) {
    io.printf("hello, %s\n", $name);
}

greet("Jennifer");   # call it from top level
```

Two keywords, two jobs:

- `def [const] NAME ...` introduces a **binding** (variable or constant).
- `func NAME(p as TYPE, q as TYPE) { ... }` introduces a **function**.

**Parameters** use bare identifiers (same rule as `def`) and each has a
declared type. Inside the body, parameters are referenced as `$p` like any
other variable. A call with the wrong number of arguments, or an argument of
the wrong kind, is a positioned error.

**Return values** use `return EXPR;` to return a value or `return;` to return
`null`. A body that runs to the end without `return` also yields `null`.
Functions declare no return type; the type check at the use site (e.g.
`def x as int init myFunction();`) enforces the value's kind.

Functions are **hoisted**: all `func NAME() { ... }` declarations are collected
before any top-level statement runs, so a function can be called from anywhere
in the file, and any function can call any other, or itself:

```jennifer
use io;

func fact(n as int) {
    if ($n == 0) { return 1; }
    return $n * fact($n - 1);
}

io.printf("%d\n", fact(5));    # 120
```

Functions can only be defined at the top level (not inside another function's
body). Function bodies inherit the global scope, so top-level variables are
visible inside functions (subject to the no-shadowing rule).

**A function cannot take the name of an imported namespace.** After `use io;`,
`func io() { ... }` is rejected:

```text
runtime error at app.j:2:1: function "io" shadows imported namespace "io"
```

A library's members live behind its prefix, so `func printf()` next to
`use io;` is allowed: `printf()` and `io.printf()` are different names. A
function also may not share a name with a top-level variable or constant, and
`len` is a reserved word.

## Functions as values

A function name is also a **function value**: written bare (no `(`), it evaluates
to a `func` you can store, pass, and call later: `def f as func init greet;`
then `$f(args)`. The higher-order `lists` helpers (`lists.map` / `filter` /
`reduce` / ...) take one. See
[Function values](types-and-values.md#function-values) for the rules.
