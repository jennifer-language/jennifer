# Interpreter (`internal/interpreter`)

A tree-walking evaluator.

## Runtime values

`Value` is a tagged union (single concrete struct) rather than a Go interface
hierarchy. This avoids `reflect` and method-table indirection, which keeps the
binary small and predictable under TinyGo.

```go
type Value struct {
    Kind    ValueKind  # KindNull | KindInt | KindFloat | KindString |
                       #  KindBool | KindList | KindMap | KindBytes |
                       #  KindStruct (M13.1)
    Int     int64
    Float   float64
    Str     string
    Bool    bool
    List    []Value      # KindList:   element data
    Map     []MapEntry   # KindMap:    insertion-ordered entries
    Bytes   []byte       # KindBytes:  raw bytes
    Fields  []StructField # KindStruct: ordered (Name, Value) per definition
    StructName string    # KindStruct: matches the StructDef name
    ElemTyp *parser.Type # KindList: declared element type (stamped)
    KeyTyp  *parser.Type # KindMap:  declared key type   (stamped)
    ValTyp  *parser.Type # KindMap:  declared value type (stamped)
}
```

`ZeroFor(t parser.Type)` returns the zero value for each declared type and is
used when a `def` omits its `init` clause: `0`, `0.0`, `""`, `false`,
`null`, an empty `[]` list (typed by the declaration), or an empty `{}` map.
`Value.AsFloat()` handles int->float promotion for arithmetic and
comparison; `Value.Equal()` implements `==` (same-kind comparison, plus
the numeric-promotion rule across `int` and `float`; deep-equal for
lists; order-insensitive key→value equality for maps).

### Parameterized Type

`parser.Type` is a recursive struct: `{ Kind TypeKind; Element,
KeyType, ValType *Type }`. Compound types nest naturally
(`list of list of int`). Equality (`Type.Equal`) is structural.

### Value semantics

Lists and maps are **value-typed** in Jennifer: `$ys = $xs;` copies,
function parameters bind by copy, and `const` is deep. No aliasing.
The interpreter enforces this at every binding boundary:

- **`execDefine`** / **`execAssign`**: take an independent copy of the
  right-hand-side value via `Value.Copy()`.
- **`evalCall`** parameter binding: same `Value.Copy()` before
  `callFrame.Define`.
- **`execForEach`**: each iteration copies the bound value.

Cost is O(n) per binding; copy-on-write is a future optimization that
preserves the user-visible semantics.

### Type stamping

After a literal like `[1, 2, 3]` evaluates, the resulting `Value` has
no `ElemTyp`. The declared type lives only on the receiving binding's
`DeclType`. To make subsequent operations - index-writes, parameter
checks, iteration types - have inner-type info without re-consulting
the declaration, the interpreter calls `stampDeclaredType(v, declType)`
at every binding boundary. The helper writes `ElemTyp` / `KeyTyp` /
`ValTyp` onto the value and recurses into nested compound elements so
deep type tracking is preserved for nested `$xs[i][j] = ...` writes.

### Index access

- **Reads** (`evalIndex`): out-of-bounds list indices and missing map
  keys are positioned runtime errors. Reads return the slot value by
  copy via Go's struct semantics, but the inner slice headers still
  alias - which is fine because reads can't mutate anything.
- **Writes** (`execIndexAssign` / `execFieldAssign`, M13.1): both
  route through the unified `collectLvalueSteps` +
  `applyLvalueWrite` walker. The walker descends through the
  chain (any mix of `[index]` and `.field` nodes) on a fresh copy
  of the root binding, writes via `writeIndexedSlot` (index leaf)
  or the per-struct-field type check (field leaf), then commits
  back through `env.Assign`. Const enforcement fires once against
  the root binding (deep constness). Map writes to a missing key
  extend the map (insertion order preserved); writes to an
  existing key update in place. Struct field writes are
  type-checked against the declared field type stored in the
  `StructDef`.

### Structs (M13.1)

User-defined struct types live in `Interpreter.structs`
(`map[string]*parser.StructDef`), populated at `Run` time by hoisting
every top-level `def struct` before any other statement executes.
This mirrors the method-hoisting pass, and the same duplicate-name
check applies (`Run` rejects two structs with the same name; the
REPL silently redefines).

Per-instance values use `KindStruct` and carry the struct name plus
a `[]StructField` (each entry is `{Name, Value}`). The field slice is
stored in declaration order so `%v` rendering and `Equal` checks are
deterministic. `Copy()` deep-copies every field so value semantics
hold; `MatchesDeclared` matches by name (`p` typed `Point` matches
`Point`-typed declarations only).

The `def x as Name;` no-init path is handled by `zeroStructFor`,
which recurses through nested struct-typed fields so every leaf
field gets its declared zero. Unknown struct names are rejected here
and at `execDefine` time before the init expression is evaluated, so
the user sees `"unknown struct type"` rather than a misleading
type-mismatch error.

### Library-provided namespaced structs (M15.2)

Libraries register their own struct types via
`Interpreter.RegisterNamespacedStruct(libName, structName, fields)`.
The definition lands in `i.NSStructs` keyed by `nsKey{NS, Name}`,
parallel to `NSBuiltins` and `NSConstants`. Users write
`def x as os.Result;` for the type and `os.Result{ ... }` for the
literal; both forms resolve at use time via `resolveNamespacePrefix`
so aliases (`use os as o; def x as o.Result;`) work the same way as
they do for namespaced function calls and constants.

At runtime the value carries both `StructName` and an optional
`StructNS` tag. `MatchesDeclared` and `Equal` compare on the
`(NS, Name)` pair so a library `os.Result` is a distinct type from a
user-defined `Result`; `Display` prefixes the namespace
(`os.Result{exitCode: 0, ...}`). Field access, chained lvalues
(`$r.exitCode`, `$line.from.x = 5;`), value semantics, and deep
`const` all reuse the M13.1 user-struct machinery - only the
type-resolution path differs.

User code may not register namespaced structs; the API is Go-side
only. Programs that want to declare their own structs keep using
the M13.1 `def struct Name { ... };` bare form.

### Iteration

`execForEach` opens a fresh per-iteration scope so the loop variable
binding doesn't leak out and `def`-rebindings don't accumulate. For
lists it walks elements in order; for maps it walks keys in
insertion order. The underlying map representation is a parallel
slice (`[]MapEntry`) rather than a Go `map[K]V` precisely to make
this iteration deterministic and testable.

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
- [libraries/strings.md](../libraries/strings.md) - `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`/`trimLeft`/`trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`
- [libraries/os.md](../libraries/os.md) - `os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`, `os.ARGS`, `os.getEnv`, `os.hasFlag`, `os.flag`, `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill`
- [libraries/meta.md](../libraries/meta.md) - `meta.VERSION` (build version), `meta.BUILD` (toolchain)
- [libraries/index.md](../libraries/index.md) - catalog and organizing principles
- `len(EXPR)` is a language built-in primary (M15.4+), not a library function. See [grammar.md](grammar.md).

What follows is the implementation contract, not the user-facing API.

Library functions are Go closures registered with the interpreter:

```go
type BuiltinCtx struct {
    Out    io.Writer  // stdout-like effects write here
    In     io.Reader  // stdin-consuming builtins read here
    InREPL bool       // true when the call originates from the REPL
}
type Builtin func(ctx BuiltinCtx, args []Value) (Value, error)

# In a library package:
func Install(in *interpreter.Interpreter) {
    in.Register("io", "printf", printf)
    in.Register("io", "sprintf", sprintf)
    in.Register("io", "readLine", readLine)
    in.Register("io", "eof", eofFn)
}
```

`BuiltinCtx` replaces an earlier `(out io.Writer, args)` signature at
M7 to give input-consuming builtins symmetric access to stdin and the
REPL flag. `Interpreter.In` defaults to `os.Stdin`; the REPL sets
`Interpreter.InREPL = true` so `readLine` / `eof` refuse rather than
racing the line editor for input.

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

### Namespaced libraries

Domain libraries register through the namespaced API:

```go
in.RegisterNamespaced("os", "platform", platformFn)
in.RegisterNamespacedConst("os", "JENNIFER_OS", interpreter.StringVal("linux"))
```

Both entries are keyed by `(namespace, name)` in `NSBuiltins` /
`NSConstants`. The library's name doubles as the namespace prefix
(future libraries may decouple them, but today they always match).
Registering through the namespaced API also flags the lib in
`knownNamespaces`.

**Only libraries flagged in `knownNamespaces` may be aliased.**
`processImports` rejects `use NAME as ALIAS;` for any library that
registered exclusively through `Register` / `RegisterConst` (the
flat API) with the message `library NAME has no namespaced
builtins; ` + "`as ALIAS`" + ` aliasing is meaningless here`. The
flat libraries (`io`, `convert`, `math`, `strings`, `core`) all
fall into this category - they have no prefix to rename, and
silently accepting an `as` clause would create the misleading
impression of an alias-shaped escape hatch.

`processImports` builds two maps from each `use NAME [as ALIAS];`:

- `nsPrefixes[prefix] = canonicalNamespace` - the prefix that's
  active at call sites; `prefix == canonical` for `use os;`,
  `prefix == alias` for `use os as o;`.
- `nsAliasedAway[canonical] = alias` - records that the canonical
  name has been shadowed by an alias, so a later `os.foo()` after
  `use os as o;` errors with a `did you mean ` + "`o`" + `?` hint.

Resolution at a `QualifiedCallExpr` / `QualifiedConstRefExpr` goes
through `resolveNamespacePrefix(prefix)`:

1. If `prefix` is in `nsPrefixes`, use the canonical namespace it
   points at.
2. Else, if `prefix` is in `nsAliasedAway`, emit the "did you mean
   `<alias>`?" hint.
3. Else, if `prefix` is the canonical name of a *known* namespaced
   lib the program forgot to `use`, emit a `requires ` + "`use prefix;`"
   reminder.
4. Else, emit `unknown namespace`.

The no-shadowing rule for top-level methods (`checkMethodNoShadow`)
adds one more clause: a method name that matches an active namespace
prefix is rejected (`func os() {}` errors after `use os;`, but is
fine after `use os as o;` because only `o` is reserved as a prefix).

The five essential flat libraries (`io`, `convert`, `math`,
`strings`, `core`) intentionally do *not* use the namespaced API -
their names stay bare for ergonomics.

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

`*runtimeError` carries optional `File`/`Line`/`Col` and a `Kind` tag
(M13.2; defaults to `"runtime"` when the originating site doesn't
specialise it). Errors render as `runtime error at FILE:L:C: <msg>`
(or `runtime error at L:C: <msg>` when the file is unknown). All
five Jennifer error types - `*lexer.LexError`, `*preproc.PreprocessError`,
`*parser.ParseError`, `*runtimeError`, and `*ErrorSignal` (M13.2) -
implement a small `Position() (file string, line, col int)` interface.
The CLI uses that interface (no string parsing) to look up the right
file and print a caret under the offending source line.

### Catchable errors (M13.2)

`try { body } catch (NAME) { handler }` runs the body and, on an
error, binds the thrown value to `$NAME` in a fresh per-handler
scope. Two sentinel paths can produce the catchable error:

- **`*ErrorSignal`** - raised by `throw EXPR;` (`execThrow`). Carries
  the thrown `Value` plus the throw's source position. Uncaught
  signals reach the CLI through the same `positioned` interface as
  `*runtimeError`.
- **`*runtimeError`** - raised by any builtin or language operation
  (out-of-bounds index, missing map key, type mismatch, etc.). When
  one reaches an enclosing `try`, `execTry` wraps it via
  `runtimeErrorToValue` into an `Error` struct (`kind`, `message`,
  `file`, `line`, `col`) and binds it like any other thrown value.

`*ExitSignal` is **not** routed through this path - the spec puts
process exit outside the recoverable-error scope, so `execTry`
propagates it untouched. `blockResult` flags (`hasReturn`,
`hasBreak`, `hasContinue`) flow through `execTry` unchanged so the
surrounding method / loop sees them.

The canonical `Error` struct is auto-hoisted into `i.structs` by
both `Run` and `EvalInteractive` before any user struct definition
runs (`canonicalErrorStructDef()`). User code may not redefine it -
the existing duplicate-struct check fires with
`struct "Error" is defined more than once`.

`runtimeError.Kind` is the symbolic tag surfaced as `$err.kind` in
the catch block. The current shipping default is `"runtime"`;
specific tags (`"out_of_bounds"`, `"type_mismatch"`, etc.) get
filled in per call site as user code grows demand for finer
dispatch.

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
