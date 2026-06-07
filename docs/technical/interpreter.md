# Interpreter (`internal/interpreter`)

A tree-walking evaluator.

## Runtime values

`Value` is a tagged union (single concrete struct) rather than a Go interface
hierarchy. This avoids `reflect` and method-table indirection, which keeps the
binary small and predictable under TinyGo.

```go
type Value struct {
    Kind  ValueKind  // KindNull | KindInt | KindFloat | KindString | KindBool
    Int   int64
    Float float64
    Str   string
    Bool  bool
}
```

`ZeroFor(t parser.Type)` returns the zero value for each declared type and is
used when a `def` omits its `init` clause: `0`, `0.0`, `""`, `false`, or
`null`. `Value.AsFloat()` handles int->float promotion for arithmetic and
comparison; `Value.Equal()` implements `==` (same-kind comparison, plus the
numeric-promotion rule across `int` and `float`).

## Environment

`Environment` is a parent-linked map of names → `Binding{Value, DeclType, IsConst}`.
Storing the declared type lets `Assign` reject type-mismatching writes (you
cannot assign a string to a variable declared as int).

- **`Define(name, val, declType, isConst)`** - adds to the current frame;
  errors if the name exists *anywhere in the chain* (the spec forbids
  shadowing).
- **`Assign(name, val)`** - walks up the chain to find the binding; errors if
  the binding is a constant, the value's kind doesn't match its declared
  type, or the name is undefined.
- **`Get(name)`** - walks up the chain.

`execBlock` opens a fresh child `Environment` for each `{...}` block, so
variables declared inside don't leak out. `for` opens its own scope wrapping
init/cond/step/body so the init variable is visible throughout the loop
without escaping it.

## Execution model

1. `Interpreter.Run(prog)` records `Imports` into `i.imported`.
2. Collects every `MethodDef` into `i.methods` (methods are hoisted: callable
   regardless of source order). During collection it enforces two rules:
   no duplicate method names, and no method name that collides with a
   registered builtin whose owning library has been imported (the no-shadowing
   rule extended to builtins - see `evalCall` below).
3. Creates the global `Environment` (`i.global`) and executes `prog.TopLevel`
   statements in source order in that global scope.
4. Method calls execute the body in a fresh call frame whose parent is
   `i.global`, so top-level variables are visible inside methods (subject to
   the no-shadowing rule). The body's return value (bare `return;` -> `null`;
   `return EXPR;` -> the expression's value; falling off the end -> `null`)
   propagates back to the caller.

There is no required entry point. A program with only imports and method
defs is valid and runs to completion immediately (those methods are simply
never called).

## Builtins and libraries

Each library lives in its own Go package under `internal/lib/<name>/` and
registers its functions (and constants) on the interpreter. User-facing
reference docs are split per library:

- [libraries/io.md](../libraries/io.md) - `printf`, `sprintf`, format verbs
- [libraries/convert.md](../libraries/convert.md) - `int`, `float`, `string`, `bool`, `typeOf`
- [libraries/math.md](../libraries/math.md) - `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `PI`, `E`
- [libraries/strings.md](../libraries/strings.md) - `len`, `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`/`trimLeft`/`trimRight`, `replace`, `repeat`, `substring`
- [libraries/core.md](../libraries/core.md) - auto-loaded; `VERSION` (interpreter build version)
- [libraries/index.md](../libraries/index.md) - catalog and organizing principles

What follows is the implementation contract, not the user-facing API.

Library functions are Go closures registered with the interpreter:

```go
type Builtin func(out io.Writer, args []Value) (Value, error)

// In a library package:
func Install(in *interpreter.Interpreter) {
    in.Register("io", "printf", printf)
    in.Register("io", "sprintf", sprintf)
}
```

`Interpreter.Builtins` stores `builtinEntry{Lib, Fn}` per name. A call to
`foo(...)` resolves in this order:

1. User-defined method `foo` in `i.methods`.
2. Builtin `foo` - **but only if its owning library has been `use`d**. The
   error otherwise quotes the right library name: `` `foo` requires `use <lib>;` ``.

The no-shadowing check at hoist time uses the same lookup: a user method
that collides with an imported library's builtin is rejected.

Library-provided **constants** (like `math.PI`) are registered via
`Interpreter.RegisterConst(lib, name, value)`. Lookup happens in
`evalExpr`'s `ConstRefExpr` case: user env first, then library constants
gated on the same `use`-check.

For the user-facing API of each library, follow the links above. Below are
the implementation-only notes worth knowing as a maintainer.

**`internal/lib/io`**: `printf` and `sprintf` share a `formatArgs` helper
with three shapes - 0 args errors; first-arg-is-string triggers format
substitution; single non-string arg writes the value's `Display()` form
(the "just print this value" shortcut). `printf` writes to
`Interpreter.Out`; `sprintf` returns a `KindString` value and ignores
the writer.

**`internal/lib/math`**: `floor`/`ceil`/`round` accept int (identity) or
float and return `int`. `round` uses Go's `math.Round` (half away from
zero). `PI` and `E` are registered via `RegisterConst`; the `ConstRefExpr`
lookup falls back to library constants when the user env doesn't have the
name, gated on the owning library being `use`d.

**`internal/lib/convert`**: parser side - the `typeCall` production lets
`int(...)`, `float(...)`, `string(...)`, `bool(...)` parse despite their
names being type keywords. `typeOf` is a normal IDENT call. `bool(v)`
implements canonical-only conversion at all source kinds (`0`/`1` for int,
`0.0`/`1.0` for float, `"true"`/`"false"` for string) - non-canonical
values produce a positioned error, not silent coercion.

**`internal/lib/strings`**: all indices and lengths are **rune-based**
(Unicode code points), implemented via `unicode/utf8`. `len` returns the
rune count; `indexOf` returns a rune index (not the byte index Go's
`strings.Index` produces - we translate); `substring` uses a small
`byteOffsetForRune` helper to convert rune-indexed bounds back to byte
slicing on the underlying string. `repeat` guards against multiplication
overflow before calling Go's `strings.Repeat` to avoid the panic in the
standard library. The Go package is named `stringslib` to avoid colliding
with the standard `strings` package, which it depends on heavily.

## Runtime errors

`*runtimeError` carries optional `File`/`Line`/`Col`. Errors render as
`runtime error at FILE:L:C: <msg>` (or `runtime error at L:C: <msg>` when
the file is unknown). All four Jennifer error types - `*lexer.LexError`,
`*preproc.PreprocessError`, `*parser.ParseError`, and `*runtimeError` -
implement a small `Position() (file string, line, col int)` interface. The
CLI uses that interface (no string parsing) to look up the right file and
print a caret under the offending source line.

## Errors and positions (cross-file)

The pipeline plumbs file information through three layers:

1. The lexer attaches the source file path to every token (`Token.File`).
   `TokenizeWithFile(source, file)` is the entry point; the no-arg
   `Tokenize` leaves `File` blank for unattributed input.
2. The preprocessor preserves each spliced token's `File` field when
   resolving `import "path.j";`, so tokens from an imported file keep that
   file's path, line, and column.
3. The parser propagates `File` from tokens to every AST node (each
   `pos` struct carries `File`, `Line`, `Col`). Synthesized nodes (e.g.
   `BinaryExpr`) copy the file from the left operand.

When the interpreter raises a `*runtimeError`, it pulls file/line/col from
the offending node via a small `posFor(node)` helper. The CLI's
`printErrorContext` type-asserts the `positioned` interface, and if the
reported file differs from the program's main file it loads that file from
disk before slicing out the snippet to display.
