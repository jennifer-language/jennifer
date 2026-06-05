# Jennifer Interpreter - Technical Documentation

Internals of the Jennifer interpreter as of Milestone 1.

## Pipeline

```
   source (string)
       │
       ▼
   ┌────────┐
   │ lexer  │   internal/lexer
   └────────┘
       │ []Token
       ▼
   ┌──────────────┐
   │ preprocessor │   internal/preproc   (splices file imports)
   └──────────────┘
       │ []Token
       ▼
   ┌────────┐
   │ parser │   internal/parser
   └────────┘
       │ *Program (AST)
       ▼
   ┌─────────────┐
   │ interpreter │   internal/interpreter + internal/lib/io (and other libs)
   └─────────────┘
       │
       ▼
     stdout / runtime error
```

The CLI lives in `cmd/jennifer/main.go` and orchestrates these stages.

---

## Lexer (`internal/lexer`)

A hand-written, single-pass scanner.

### Token types

```
EOF       INT          DEFINE        LBRACE  PLUS    LT
ILLEGAL   FLOAT        FUNC          RBRACE  MINUS   GT
          STRING       AS            LPAREN  STAR    LE
          IDENT        INIT          RPAREN  SLASH   GE
          VARREF       CONST         SEMI    PERCENT EQ
                       IMPORT        COMMA
                       USE           ASSIGN
                       RETURN        DOT
                       IF
                       ELSEIF
                       ELSE
                       WHILE
                       FOR
                       TRUE
                       FALSE
                       NULL
                       AND
                       OR
                       NOT
                       DIV
                       INT_TYPE
                       FLOAT_TYPE
                       STRING_TYPE
                       BOOL_TYPE
```

`def` introduces a variable or constant binding (TOKEN_DEFINE); `func`
introduces a method (TOKEN_FUNC). `import` (TOKEN_IMPORT) is for **file
imports** (`import "path.j";`); `use` (TOKEN_USE) is for **library imports**
(`use io;`). `DOT` (`.`) no longer appears in import syntax (paths are
strings now) and is reserved for future expression use.
Comparison tokens `LE`, `GE`, `EQ` are two-character (`<=`, `>=`, `==`) and
are recognized by a one-character lookahead from `<`, `>`, `=`. `RETURN` is
already reserved for M3.

`VARREF` carries the variable name *without* the leading `$`.
`STRING` carries the value *with* escape sequences already processed and *without*
surrounding quotes.

### Position tracking

Every token records `Line` and `Col` (both 1-based) and `File` (the absolute
path supplied to `TokenizeWithFile`, or `""` for unattributed input). The
`advance()` helper bumps `line` on `\n` and otherwise bumps `col`. `File`
flows from the token to the AST node (every node embeds `pos{File, Line,
Col}`), so errors raised inside an imported `.j` still point at the
imported file - see "Errors and positions" below.

### Keywords

The lexer's keyword map covers: `def func as init const import use return if
elseif else while for true false null and or not div int float string bool`.
Anything else lexed as a word stays a `TOKEN_IDENT`. `define` is **not** a
keyword and lexes as a plain identifier.

### Comments

`// ...` runs to end of line. `/* ... */` is non-nesting and reports an
"unterminated block comment" error if unclosed.

### Identifier rule

Per the spec, names use `[A-Za-z]` only. Digits and underscores are explicitly
**not** part of identifiers; encountering one mid-token ends the identifier.

---

## Grammar (M3) - EBNF

The authoritative grammar for what the parser accepts. This grammar describes
the token stream **after** preprocessing - file imports (`import STRING ;`)
are spliced before the parser runs, so they don't appear here. Only library
imports (`use IDENT ;`) reach the parser.

Terminals in CAPITALS are token classes from the lexer (see [Token types](#token-types));
quoted strings are keywords or punctuation that match the corresponding token's
lexeme.

```ebnf
program     = { useStmt | methodDef | statement } EOF ;
useStmt     = "use" IDENT ";" ;                     (* library import *)
methodDef   = "func" IDENT "(" [ paramList ] ")" block ;
paramList   = param { "," param } ;
param       = IDENT "as" type ;
block       = "{" { statement } "}" ;

statement   = defineStmt
            | assignStmt
            | returnStmt
            | ifStmt
            | whileStmt
            | forStmt
            | exprStmt ;

returnStmt  = "return" [ expr ] ";" ;

defineStmt  = "def" [ "const" ] IDENT "as" type [ "init" expr ] ";" ;
                                       (* constants require "init" and an UPPERCASE name;
                                          variables may omit "init" and get zero-value *)

assignStmt  = VARREF "=" expr ";" ;

ifStmt      = "if" "(" expr ")" block
              { "elseif" "(" expr ")" block }
              [ "else" block ] ;

whileStmt   = "while" "(" expr ")" block ;

forStmt     = "for" "(" [ defineStmt | assignStmt | ";" ]
                        [ expr ] ";"
                        [ assignNoSemi ]
                  ")" block ;
assignNoSemi = VARREF "=" expr ;       (* same shape as assignStmt without trailing ";" *)

exprStmt    = expr ";" ;

type        = "int" | "float" | "string" | "bool" | "null" ;

expr        = orExpr ;
orExpr      = andExpr { "or" andExpr } ;
andExpr     = notExpr { "and" notExpr } ;
notExpr     = "not" notExpr | compExpr ;
compExpr    = addExpr { ("<" | ">" | "<=" | ">=" | "==") addExpr } ;
addExpr     = mulExpr { ("+" | "-") mulExpr } ;
mulExpr     = unaryExpr { ("*" | "/" | "div" | "%") unaryExpr } ;
unaryExpr   = "-" unaryExpr | primary ;
primary     = INT | FLOAT | STRING | "true" | "false" | "null"
            | VARREF | call | typeCall | constRef | "(" expr ")" ;
call        = IDENT "(" [ expr { "," expr } ] ")" ;
typeCall    = ("int" | "float" | "string" | "bool") "(" [ expr { "," expr } ] ")" ;
                                       (* type keywords usable as calls only when
                                          immediately followed by `(`; resolved as
                                          convert-library builtins at runtime *)
constRef    = IDENT ;                  (* bare-IDENT: constant reference; the
                                          parser disambiguates `call` vs
                                          `constRef` by peeking for "(". *)
```

**Semantic notes that aren't expressed in the grammar:**

- Two separate keywords: `def` introduces a binding (variable or constant);
  `func` introduces a method. There's no longer any lookahead disambiguation
  in this area - the parser dispatches purely on the keyword.
- The name in `defineStmt` is a bare `IDENT`. Writing `def $x as int`
  produces a parse error with a hint to drop the `$` (it's reserved for
  use-site references).
- Operator precedence (lowest to highest): `or`, `and`, unary `not`,
  comparison `< > <= >= ==`, additive `+ -`, multiplicative `* / %`, unary
  `-`. Binary operators are left-associative; `not` and unary `-` are
  right-associative (`not not x` and `--x` are both valid).
- `and` and `or` **short-circuit**: the right operand is not evaluated when
  the left already decides the result. Both operands must be `bool`.
- Unary `not` requires `bool`; unary `-` requires `int` or `float`.
- Comparison operators produce `bool`; `if`/`while`/`for` conditions **must**
  be `bool` (no implicit truthiness).
- Mixed `int`/`float` arithmetic promotes `int` to `float`; the result is
  `float`. `%` requires int operands. `+` on two `string` values concatenates.
- **`/` (true division) always returns `float`** (Python 3 semantics). For
  integer-result division use the `div` keyword: `5 / 2 = 2.5`, `5 div 2 = 2`.
  `div` on float operands returns the floor as a float (`5.7 div 2.0 = 2.0`).
- Floats always display with a `.` so the type stays visible: `5.0` prints as
  `"5.0"`, not `"5"`. See `interpreter.DisplayFloat`.
- Methods may only be defined at the top level. Variable definitions, assignments,
  control flow, and expression statements may appear at the top level or inside
  a block.
- Each `block` (`{...}`) introduces a new lexical scope. A binding is visible
  from its `def` to the end of the enclosing block, and is inherited by
  inner blocks; inner scopes **cannot redeclare** a name already visible.
- `for` opens a private scope for its `init`, `cond`, `step`, and body so the
  init variable does not leak out of the loop.
- There is **no required entry point**. Top-level statements execute in source
  order. Methods are hoisted (collected before any top-level statement runs)
  so they can be called regardless of textual order.
- Method bodies inherit the global scope as their outer scope, so top-level
  variables are visible inside methods (subject to the no-shadowing rule).
- Method parameters use bare `IDENT` (no `$`), same as variable definitions.
  Writing `func f($x as int)` errors with "parameter name has no `$`".
- Call sites type-check arguments against the declared parameter types at
  runtime; both arity and per-argument kind are checked.
- Method return values are dynamically typed - methods don't declare a
  return type, and callers receive whatever value the body returns (or
  `null` for a bare `return;` or a body that falls off the end).
- A bare `IDENT` in expression position is parsed as a `CallExpr` if
  immediately followed by `(`, otherwise as a `ConstRefExpr`. At runtime
  the latter must resolve to a constant in scope; a name that resolves to
  a variable produces a helpful error ("use `$name`").

---

## Parser (`internal/parser`)

Recursive descent with precedence climbing for binary operators. The grammar
the parser implements is the one in [Grammar (M3)](#grammar-m3---ebnf) above.

### AST nodes (M3)

| Node          | Kind  | Fields                                       |
|---------------|-------|----------------------------------------------|
| `Program`     | root  | `Imports []*ImportStmt`, `Methods []*MethodDef`, `TopLevel []Stmt` |
| `ImportStmt`  | stmt  | `Name`                                       |
| `MethodDef`   | stmt  | `Name`, `Params []Param`, `Body *Block`      |
| `Param`       | -     | `Name`, `Type`                               |
| `Block`       | stmt  | `Stmts []Stmt`                               |
| `DefineStmt`  | stmt  | `IsConst`, `VarName`, `VarType Type`, `InitExpr Expr` (nil = uninit) |
| `AssignStmt`  | stmt  | `VarName`, `Value Expr`                      |
| `ReturnStmt`  | stmt  | `Value Expr` (nil for bare `return;`)        |
| `IfStmt`      | stmt  | `Cond`, `Then *Block`, `ElseIfs []Expr`, `ElseIfBodies []*Block`, `Else *Block` |
| `WhileStmt`   | stmt  | `Cond`, `Body *Block`                        |
| `ForStmt`     | stmt  | `Init Stmt`, `Cond Expr`, `Step Stmt`, `Body *Block` (any may be nil) |
| `ExprStmt`    | stmt  | `Expr`                                       |
| `IntLit`      | expr  | `Value int64`                                |
| `FloatLit`    | expr  | `Value float64`                              |
| `StringLit`   | expr  | `Value string`                               |
| `BoolLit`     | expr  | `Value bool`                                 |
| `NullLit`     | expr  | -                                            |
| `VarExpr`     | expr  | `Name` (no `$`) - mutable-variable reference  |
| `ConstRefExpr`| expr  | `Name` - bare-IDENT reference; interpreter expects it to resolve to a constant |
| `CallExpr`    | expr  | `Callee`, `Args []Expr`                      |
| `BinaryExpr`  | expr  | `Op BinaryOp`, `Left`, `Right` (comparison/logical ops return bool; `and`/`or` short-circuit at eval time) |
| `UnaryExpr`   | expr  | `Op UnaryOp` (`OpNeg`/`OpNot`), `Operand` |

Every node embeds a `pos{File, Line, Col}` for error reporting and exposes it
via `Node.Pos()` (line/col) and `Node.Filename()` (file path). The file is
populated from the originating token so cross-file diagnostics work.

`Sprint(node)` produces a stable textual representation used by tests.

---

## Preprocessor (`internal/preproc`)

Sits between the lexer and the parser. Its only job is to expand file imports
and pass library imports through.

### Algorithm

1. Walk the token stream.
2. When `IMPORT STRING SEMI` is found:
   - Verify the string ends in `.j`.
   - Resolve the path (relative to the current file's directory, or absolute
     if it starts with `/`).
   - Reject if the path was already visited up the import chain (circular import).
   - Read the file, lex it (with file-tagged tokens), recursively preprocess it.
   - Splice the result (dropping the trailing `EOF`) at this point.
3. When `USE IDENT SEMI` is found: pass through unchanged. The parser turns
   it into an `ImportStmt` node.
4. Helpful errors for common mistakes:
   - `import IDENT;` (old library form) -> "use `use NAME;` for system libraries".
   - `import IDENT.j;` (old unquoted file form) -> "file imports take a string literal".
   - `use IDENT.j;` (file form with `use`) -> "use `import \"name.j\";` for files".

### Edge cases

- The path string must literally end in `.j`. `import "foo.go";` is rejected.
- Paths may contain `/` for subdirectories. Absolute paths are accepted as-is;
  relative paths resolve from the importing file's directory.
- Circular imports are detected by tracking absolute paths visited along the
  current chain.

---

## Interpreter (`internal/interpreter`)

A tree-walking evaluator.

### Runtime values

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

### Environment

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

### Execution model

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
   the no-shadowing rule). Returned values propagate (M2: always `null`, since
   `return` doesn't exist yet).

There is no required entry point. A program with only imports and method
defs is valid and runs to completion immediately (those methods are simply
never called).

### Builtins and libraries

Each library lives in its own Go package under `internal/lib/<name>/` and
registers its functions (and constants) on the interpreter. User-facing
reference docs are split per library:

- [lib_io.md](lib_io.md) - `printf`, `sprintf`, format verbs
- [lib_convert.md](lib_convert.md) - `int`, `float`, `string`, `bool`, `typeOf`
- [lib_math.md](lib_math.md) - `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `PI`, `E`
- [lib_strings.md](lib_strings.md) - `len`, `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`/`trimLeft`/`trimRight`, `replace`, `repeat`, `substring`
- [lib_meta.md](lib_meta.md) - `VERSION` (interpreter build version)

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
substitution; single non-string arg writes `Display()` (the M2 ergonomic
preserved). `printf` writes to `Interpreter.Out`; `sprintf` returns a
`KindString` value and ignores the writer.

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

### Runtime errors

`*runtimeError` carries optional `File`/`Line`/`Col`. Errors render as
`runtime error at FILE:L:C: <msg>` (or `runtime error at L:C: <msg>` when
the file is unknown). All four Jennifer error types - `*lexer.LexError`,
`*preproc.PreprocessError`, `*parser.ParseError`, and `*runtimeError` -
implement a small `Position() (file string, line, col int)` interface. The
CLI uses that interface (no string parsing) to look up the right file and
print a caret under the offending source line.

### Errors and positions (cross-file)

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

---

## CLI (`cmd/jennifer`)

```
jennifer run <file.j>     run a Jennifer program
jennifer run -            read source from stdin
jennifer repl             interactive REPL
jennifer version          print the build version and exit
jennifer help             show usage
```

- Verifies the `.j` extension
- Reads the file, parses, runs
- On error: prints the message and a source-context caret on stderr, exits `1`
- Bad usage exits `2`
- `jennifer help` includes a `Version:` line so the build is identifiable at a glance

### REPL (`cmd/jennifer/repl.go`)

The REPL drives a read-eval-print loop on top of the standard pipeline. Each
input is lexed, preprocessed, parsed, and fed to `Interpreter.EvalInteractive`
(not `Run`). `EvalInteractive` differs from `Run` in three documented ways:
the global env is lazy-initialized and preserved across calls, library
imports and method definitions are idempotent / re-assignable so the user
can iterate, and the value of a trailing `ExprStmt` is returned so the loop
can print it.

Multi-line input is handled by a small `inputComplete(tokens)` helper that
balances `{`/`(` against `}`/`)` (using the lexer's tokens so string and
comment contents are ignored) and requires the input to end in `;` or `}`.
Anything else triggers a `... ` continuation prompt. Unbalanced *closing*
delimiters intentionally fall through to the parser for diagnosis since no
amount of additional input would fix them.

REPL input is tagged with the synthetic file label `<repl>`. The
cross-file-error snippet loader in `printErrorContext` treats `<repl>` like
`<stdin>`: no external file lookup is attempted, and the current input
buffer is used as the snippet source. Lex errors discard the buffer (since
they cannot become valid by reading more); parse and runtime errors print
and the loop continues.

`:quit` / `:exit` / EOF terminate cleanly; `:help` prints a short reminder.
Directives are only recognized at a fresh prompt so a literal `:quit` inside
a block doesn't short-circuit.

### Version injection

`internal/version.Version` holds the build version as a single `string`.
The default is `"dev"`; the `Makefile` runs `scripts/gen-version.sh` before
each build, which writes `internal/version/version_gen.go` containing a
small `init()` that overwrites `Version` with the output of
`scripts/version.sh` (a `git describe --tags --long` derivative; see
[lib_meta.md](lib_meta.md) for the format).

This codegen path replaces the more conventional `go build -ldflags
"-X .Version=..."` because TinyGo 0.41 silently ignores the `-X`
directive. Codegen works identically on both toolchains. The generated
file is `.gitignore`d so the repository never carries a stale copy.

Two consumers read `version.Version`:

- `cmd/jennifer/main.go` prints it in the `help` banner and as the body
  of the `version` subcommand.
- `internal/lib/meta/metalib.go` mirrors it into the interpreter as the
  `VERSION` constant, so Jennifer programs can read it via
  `use meta; printf("%s\n", VERSION);`.

`go test ./...` skips codegen and uses the default `"dev"`. The meta-lib
test only checks that the constant matches `version.Version`, not a
specific value, so it stays robust across builds.

---

## Testing

| Package                | What it tests                                  |
|------------------------|------------------------------------------------|
| `internal/lexer`       | Token-by-token output for fixed inputs; error cases |
| `internal/parser`      | AST shape via `Sprint`; precedence; error cases |
| `internal/interpreter` | Full programs in-memory; stdout captured       |
| `internal/lib/io`       | `Install` registers `printf`/`sprintf`; arity and format errors |
| `cmd/jennifer`         | Golden test that runs every `examples/*.j` and compares stdout to `examples/expected/*.txt` |

Run everything with `go test ./...`.

---

## File map

```
cmd/jennifer/main.go             CLI + source-context error formatting
cmd/jennifer/repl.go             Interactive REPL loop
cmd/jennifer/examples_test.go    Golden-file integration test
cmd/jennifer/repl_test.go        REPL inputComplete unit tests
cmd/jennifer/cross_file_error_test.go  Cross-file error reporting tests
internal/lexer/token.go          Token type definitions
internal/lexer/lexer.go          Scanner (with optional file tagging)
internal/lexer/lexer_test.go     Lexer tests
internal/preproc/preproc.go      File-import preprocessor
internal/preproc/preproc_test.go Preprocessor tests
internal/parser/ast.go           AST node types + Sprint
internal/parser/parser.go        Recursive-descent parser
internal/parser/parser_test.go   Parser tests
internal/interpreter/value.go         Runtime Value tagged union
internal/interpreter/environment.go   Scoped symbol table
internal/interpreter/interpreter.go   Tree-walking evaluator
internal/interpreter/interpreter_test.go End-to-end interpreter tests
internal/lib/io/iolib.go          `io` library: printf, sprintf
internal/lib/io/iolib_test.go     io library unit tests
internal/lib/convert/convert.go   `convert` library: int, float, string, bool, typeof
internal/lib/math/mathlib.go      `math` library: abs/min/max/sqrt/pow/floor/ceil/round; PI/E
internal/lib/strings/stringslib.go  `strings` library: len/upper/lower/contains/startsWith/endsWith/indexOf/trim*/replace/repeat/substring
internal/lib/meta/metalib.go      `meta` library: VERSION constant
internal/lib/meta/metalib_test.go meta library unit tests
internal/version/version.go       Default Version = "dev"
internal/version/version_gen.go   GENERATED by scripts/gen-version.sh (gitignored)
scripts/version.sh                Computes the version string from git
scripts/gen-version.sh            Writes version_gen.go before each build
Makefile                          build / build-go / test / clean / version targets
examples/*.j                     Example programs
examples/expected/*.txt          Expected stdout per example
examples/with_import/            Subdirectory demonstrating file imports
```

---

## Rejected features

Proposals that were considered and explicitly turned down. Recorded here so
the same ideas don't come back as fresh suggestions next session.

### Increment / decrement (`++`/`--`)

Considered: postfix `$i++` and prefix `++$i`.

Rejected because:

- The pre/post distinction is a real footgun - the two forms differ only
  in expression context, which is exactly where bugs hide. Swift removed
  `++`/`--` in version 3 (2016) for this exact reason.
- The savings are tiny (three characters) and only apply to `+1` / `-1`.
- Python rejected them from the start and the language hasn't suffered.
- `$i = $i + 1;` is verbose but unambiguous; the readability cost is small.

### Compound assignment (`+=`, `-=`, `*=`, `/=`, `div=`, `%=`)

Considered as an alternative to `++`/`--`.

Rejected because:

- `div=` reads particularly badly - mashing a word-operator into the
  assignment-operator family stands out.
- Several operators to add and remember for marginal ergonomic gain over
  `$x = $x + E;`.
- Slippery slope: would we also need a string-concat `+=`? An `and=`?
  Where does the family end?
- Keeping a single assignment shape (`$x = EXPR;`) makes source code uniform
  and matches Jennifer's "one way to do each thing" stance.

---

## TinyGo notes

The interpreter is built with TinyGo: `tinygo build -o jennifer ./cmd/jennifer`.

A few constraints shape the implementation:

- **No `reflect`-heavy code.** Tagged-union `Value` instead of interfaces with
  type assertions in hot paths.
- **No `text/template`, no goroutines in the interpreter core.** Not needed
  yet, but worth not introducing accidentally.
- **`testing` runs under regular `go test`.** TinyGo's `testing` support is
  partial; we develop and verify with `go test ./...`.
