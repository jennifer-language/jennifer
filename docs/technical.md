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
   │ interpreter │   internal/interpreter + internal/stdlib
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
EOF       INT          DEFINE        LBRACE  PLUS
ILLEGAL   STRING       AS            RBRACE  MINUS
          IDENT        INIT          LPAREN  STAR
          VARREF       IMPORT        RPAREN  SLASH
                       INT_TYPE      SEMI    PERCENT
                       STRING_TYPE   COMMA
                                     ASSIGN
                                     DOT
```

The keywords `def` and `define` both produce a single `TOKEN_DEFINE` -
they are synonyms. `DOT` (`.`) is only used in file-import paths (`name.j`).

`VARREF` carries the variable name *without* the leading `$`.
`STRING` carries the value *with* escape sequences already processed and *without*
surrounding quotes.

### Position tracking

Every token records `Line` and `Col` (both 1-based). The `advance()` helper bumps
`line` on `\n` and otherwise bumps `col`.

### Keywords

`define def as init import int string` are looked up in a map after reading an
identifier; `def` and `define` map to the same token type. Anything else stays a
`TOKEN_IDENT`.

### Comments

`// ...` runs to end of line. `/* ... */` is non-nesting and reports an
"unterminated block comment" error if unclosed.

### Identifier rule

Per the spec, names use `[A-Za-z]` only. Digits and underscores are explicitly
**not** part of identifiers; encountering one mid-token ends the identifier.

---

## Grammar (M1) - EBNF

The authoritative grammar for what the parser accepts. This grammar describes
the token stream **after** preprocessing - file imports (`import IDENT . IDENT ;`)
are spliced before the parser runs, so they don't appear here.

Terminals in CAPITALS are token classes from the lexer (see [Token types](#token-types));
quoted strings are keywords or punctuation that match the corresponding token's
lexeme.

```ebnf
program     = { importStmt | methodDef } EOF ;
importStmt  = "import" IDENT ";" ;                  (* library import *)
methodDef   = ("def" | "define") IDENT "(" ")" block ;
block       = "{" { statement } "}" ;
statement   = defineStmt | exprStmt ;
defineStmt  = ("def" | "define") VARREF "as" type "init" expr ";" ;
exprStmt    = expr ";" ;
type        = "int" | "string" ;
expr        = addExpr ;
addExpr     = mulExpr { ("+" | "-") mulExpr } ;
mulExpr     = primary { ("*" | "/" | "%") primary } ;
primary     = INT | STRING | VARREF | call | "(" expr ")" ;
call        = IDENT "(" [ expr { "," expr } ] ")" ;
```

**Semantic notes that aren't expressed in the grammar:**

- Both `def` and `define` produce `TOKEN_DEFINE`. The parser disambiguates a
  `methodDef` from a `defineStmt` by lookahead: `IDENT` follows for a method,
  `VARREF` for a variable definition.
- `+` and `-` are left-associative; `*`, `/`, `%` bind tighter and are also
  left-associative.
- Variable definitions are only allowed inside a `block` (M1 has no top-level
  variables).
- `app()` must be among the program's `methodDef`s; it's the entry point.

---

## Parser (`internal/parser`)

Recursive descent with precedence climbing for binary operators. The grammar
the parser implements is the one in [Grammar (M1)](#grammar-m1---ebnf) above.

### AST nodes (M1)

| Node          | Kind  | Fields                                       |
|---------------|-------|----------------------------------------------|
| `Program`     | root  | `Imports []*ImportStmt`, `Methods []*MethodDef` |
| `ImportStmt`  | stmt  | `Name`                                       |
| `MethodDef`   | stmt  | `Name`, `Body *Block`                        |
| `Block`       | stmt  | `Stmts []Stmt`                               |
| `DefineStmt`  | stmt  | `VarName`, `VarType Type`, `InitExpr Expr`   |
| `ExprStmt`    | stmt  | `Expr`                                       |
| `IntLit`      | expr  | `Value int64`                                |
| `StringLit`   | expr  | `Value string`                               |
| `VarExpr`     | expr  | `Name` (no `$`)                              |
| `CallExpr`    | expr  | `Callee`, `Args []Expr`                      |
| `BinaryExpr`  | expr  | `Op BinaryOp`, `Left`, `Right`               |

Every node embeds a `pos{Line, Col}` for error reporting and exposes it via
`Node.Pos()`.

`Sprint(node)` produces a stable textual representation used by tests.

---

## Preprocessor (`internal/preproc`)

Sits between the lexer and the parser. Its only job is to expand file imports.

### Algorithm

1. Walk the token stream.
2. When the sequence `IMPORT IDENT DOT IDENT(=="j") SEMI` is found:
   - Resolve `IDENT.j` relative to the current file's directory.
   - Reject if the path was already visited up the import chain (circular import).
   - Read the file, lex it (with file-tagged tokens), recursively preprocess it.
   - Splice the result (dropping the trailing `EOF`) at this point.
3. Any other `import` (including the canonical `import stdlib;`) is left alone
   and reaches the parser as an `ImportStmt` node.

### Edge cases

- The file extension must literally be `.j` - `import foo.go;` is rejected.
- Filenames follow the identifier rule (`[A-Za-z]`) since they're carried by
  `TOKEN_IDENT`. Future expansion: accept quoted strings to allow underscores,
  hyphens, and subdirectories.
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
    Kind ValueKind  // KindNull | KindInt | KindString
    Int  int64
    Str  string
}
```

### Environment

`Environment` is a parent-linked map of names → `Value`, plus a per-frame
`consts` set.

- **`Define(name, val, isConst)`** - adds to the current frame; errors if the
  name exists *anywhere in the chain* (the spec forbids shadowing).
- **`Assign(name, val)`** - walks up the chain to find the binding; errors if
  the binding is a constant or doesn't exist.
- **`Get(name)`** - walks up the chain.

In M1 only one global frame is used (no nested blocks yet beyond method bodies).
A method call creates its own root frame - methods do not see outer variables.
M2/M3 introduce block-scope nesting and the formal scoping rules from the spec.

### Execution model

1. `Interpreter.Run(prog)` records `Imports` and collects every `MethodDef`
   into `i.methods`.
2. It looks up `app` and errors if missing.
3. It calls `app()` in a fresh `Environment`.
4. Statements are evaluated sequentially; expressions return a `Value`.
5. Method calls (M1: zero-arg) execute the body in a fresh frame and propagate
   any returned `Value` (M1: always `null`, since `return` doesn't exist yet).

### Builtins / stdlib

Stdlib functions are Go closures registered in `Interpreter.Builtins`:

```go
type Builtin func(out io.Writer, args []Value) (Value, error)
```

A call to `foo(...)` resolves in this order:

1. User-defined method `foo` in `i.methods`.
2. Builtin `foo` - **but only if the library that registered it was imported**.
   In M1, all builtins gate on `import stdlib;`.

`stdlib.Install(in)` registers `printf`. M1 `printf` takes exactly one argument
and writes its `Display()` form to the interpreter's writer.

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
| `internal/stdlib`      | `Install` registers `printf`; arity errors     |
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
internal/stdlib/stdlib.go        Builtin Jennifer functions
internal/stdlib/stdlib_test.go   Stdlib unit tests
examples/*.j                     Example programs
examples/expected/*.txt          Expected stdout per example
examples/with_import/            Subdirectory demonstrating file imports
```

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

If a feature added in M2/M3 conflicts with TinyGo (e.g. `reflect`), the fix is
in the interpreter, not the build target.
