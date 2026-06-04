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

Every token records `Line` and `Col` (both 1-based). The `advance()` helper bumps
`line` on `\n` and otherwise bumps `col`.

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

Every node embeds a `pos{Line, Col}` for error reporting and exposes it via
`Node.Pos()`.

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

**`internal/lib/io`** registers `printf` and `sprintf` under the Jennifer
library name `io`. Both share a `formatArgs` helper with three shapes:
- 0 args -> error.
- First arg is a string -> treat as a Go-style format string and substitute
  the remaining args. Verbs: `%d` (int), `%f` (float), `%s` (string),
  `%t` (bool), `%v` (any/display), `%%` (literal `%`). Mismatches between
  verb and arg kind produce a runtime error.
- Exactly 1 non-string arg -> write its `Display()` form (preserves the M2
  ergonomics of `printf($int)`).

`printf` writes to the interpreter's `Out`; `sprintf` returns a `KindString`
value and ignores the writer.

**`internal/lib/convert`** registers `int`, `float`, `string`, `bool`, and
`typeof` under the Jennifer library name `convert`. Each takes exactly one
argument:

- `int(v)`: int identity; float truncates toward zero; string parses (errors
  on bad input); bool → 1/0; null errors.
- `float(v)`: int→float; float identity; string parses; bool → 1.0/0.0;
  null errors.
- `string(v)`: always succeeds; returns `Display()`.
- `bool(v)`: bool identity; canonical-only otherwise - `0`/`1` (int), `0.0`/`1.0`
  (float), `"true"`/`"false"` (string). Non-canonical values like `-1`, `123`,
  `0.5`, `"x"`, and `null` all error. For "nonzero is true" semantics write
  the comparison explicitly (`$x != 0`).
- `typeof(v)`: returns the kind name (`"int"`/`"float"`/`"string"`/`"bool"`/`"null"`)
  as a string. Useful when the result of an arithmetic expression isn't
  obviously typed.

The parser allows `int(...)`, `float(...)`, `string(...)`, `bool(...)` even
though those tokens are type keywords - see the `typeCall` production. `typeof`
is a normal IDENT call.

### Runtime errors

`*runtimeError` carries optional `Line`/`Col`. Errors render as
`runtime error at L:C: <msg>` so the CLI's `extractPos` can find the position
and print a caret under the offending source line.

---

## CLI (`cmd/jennifer`)

```
jennifer run <file.j>
```

- Verifies the `.j` extension
- Reads the file, parses, runs
- On error: prints the message and a source-context caret on stderr, exits `1`
- Bad usage exits `2`

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
cmd/jennifer/examples_test.go    Golden-file integration test
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
