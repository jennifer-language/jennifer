# Grammar and parser

The authoritative grammar for what the parser accepts, plus a quick tour of
the AST node table and the parser's structure.

## Grammar - EBNF

This grammar describes the token stream **after** preprocessing - file
imports (`import STRING ;`) are spliced before the parser runs, so they
don't appear here. Only library imports (`use IDENT ;`) reach the parser.

Terminals in CAPITALS are token classes from the lexer (see
[Lexer > Token types](lexer.md#token-types)); quoted strings are keywords or
punctuation that match the corresponding token's lexeme.

```ebnf
program     = { useStmt | methodDef | structDef | statement } EOF ;
useStmt     = "use" IDENT [ "as" IDENT ] ";" ;      (* library import; the
                                                       optional "as ALIAS"
                                                       renames the namespace
                                                       at the use site *)
methodDef   = "func" IDENT "(" [ paramList ] ")" block ;
paramList   = param { "," param } ;
param       = IDENT "as" type ;
block       = "{" { statement } "}" ;

structDef   = "def" "struct" IDENT "{" structField { "," structField } [ "," ] "}" ";" ;
                                       (* M13.1: top-level only;
                                          IDENT names the struct type;
                                          field names follow the IDENT
                                          rule too. Hoisted before the
                                          first top-level statement runs. *)
structField = IDENT "as" type ;

statement   = defineStmt
            | assignStmt
            | indexAssign
            | fieldAssign
            | appendStmt
            | returnStmt
            | ifStmt
            | whileStmt
            | forStmt
            | forEachStmt
            | exprStmt ;

returnStmt  = "return" [ expr ] ";" ;

defineStmt  = "def" [ "const" ] IDENT "as" type [ "init" expr ] ";" ;
                                       (* constants require "init" and an
                                          uppercase name matching
                                          [A-Z]+(_[A-Z]+)* (uppercase
                                          chunks joined by single `_`;
                                          no leading, trailing or
                                          consecutive `_`); variables may
                                          omit "init" and get zero-value,
                                          and use the letters-only IDENT
                                          form *)

assignStmt  = VARREF "=" expr ";" ;

indexAssign = VARREF lvalueTail { lvalueTail } "[" expr "]" "=" expr ";" ;
                                       (* l-value chain ending in `[index]`;
                                          root is a VARREF. M13.1: tail
                                          steps may freely mix `[index]`
                                          and `.field`. *)

fieldAssign = VARREF lvalueTail { lvalueTail } "." IDENT "=" expr ";" ;
                                       (* l-value chain ending in `.field`
                                          (M13.1). Root is a VARREF; tail
                                          may mix `[index]` and `.field`. *)

lvalueTail  = "[" expr "]" | "." IDENT ;

appendStmt  = VARREF "[" "]" "=" expr ";" ;
                                       (* append sugar: write-only
                                          target meaning "the position
                                          just past the end of the
                                          list"; read use `e[]` is a
                                          parse error. Only one bare
                                          VARREF root - chained forms
                                          like `$xs[0][]` are not supported
                                          (yet). *)

ifStmt      = "if" "(" expr ")" block
              { "elseif" "(" expr ")" block }
              [ "else" block ] ;

whileStmt   = "while" "(" expr ")" block ;

forStmt     = "for" "(" [ defineStmt | assignStmt | ";" ]
                        [ expr ] ";"
                        [ assignNoSemi ]
                  ")" block ;
assignNoSemi = VARREF "=" expr ;       (* same shape as assignStmt without trailing ";" *)

forEachStmt = "for" "(" "def" IDENT "in" expr ")" block ;
                                       (* iterates list elements (in order)
                                          or map keys (insertion order);
                                          the iteration variable is a fresh
                                          binding in the body's scope *)

exprStmt    = expr ";" ;

type        = primType | listType | mapType | structType ;
primType    = "int" | "float" | "string" | "bool" | "null" | "bytes" ;
listType    = "list" "of" type ;
mapType     = "map" "of" type "to" type ;
                                       (* recursive; nesting like
                                          `list of list of int` and
                                          `map of string to list of int`
                                          falls out naturally *)
structType  = IDENT ;                  (* M13.1: user-defined struct type;
                                          resolved at runtime against the
                                          hoisted struct table - unknown
                                          names are positioned errors *)

expr        = orExpr ;
orExpr      = andExpr { "or" andExpr } ;
andExpr     = notExpr { "and" notExpr } ;
notExpr     = "not" notExpr | compExpr ;
compExpr    = addExpr { ("<" | ">" | "<=" | ">=" | "==") addExpr } ;
addExpr     = mulExpr { ("+" | "-") mulExpr } ;
mulExpr     = unaryExpr { ("*" | "/" | "//" | "%") unaryExpr } ;
unaryExpr   = "-" unaryExpr | primary ;
primary     = ( INT | FLOAT | STRING | "true" | "false" | "null"
              | VARREF | qualifiedCall | qualifiedConstRef
              | call | typeCall | structLit | constRef | "(" expr ")"
              | listLit | mapLit )
              { "[" expr "]" | "." IDENT } ;
                                       (* any primary can be index- or
                                          field-chained; M13.1 adds the
                                          `.field` form *)
structLit   = IDENT "{" structLitField { "," structLitField } [ "," ] "}" ;
                                       (* M13.1: struct literal -
                                          IDENT must be a defined struct.
                                          The `{` after IDENT in
                                          expression position is the
                                          tie-breaker against `constRef`;
                                          empty struct literals are
                                          rejected (every field required). *)
structLitField = IDENT ":" expr ;
call        = IDENT "(" [ expr { "," expr } ] ")" ;
qualifiedCall      = IDENT "." IDENT "(" [ expr { "," expr } ] ")" ;
qualifiedConstRef  = IDENT "." IDENT ;
                                       (* qualifiedCall / qualifiedConstRef:
                                          IDENT "." IDENT, then `(` decides which.
                                          Resolved against the namespaced-builtin
                                          / constant registry, gated by `use lib;`
                                          (or alias-aware equivalent). *)
typeCall    = ("int" | "float" | "string" | "bool") "(" [ expr { "," expr } ] ")" ;
                                       (* type keywords usable as calls only when
                                          immediately followed by `(`; resolved as
                                          convert-library builtins at runtime *)
constRef    = IDENT ;                  (* bare-IDENT: constant reference; the
                                          parser disambiguates `call` vs
                                          `qualifiedCall` vs `constRef` by
                                          peeking for "." / "(". *)
listLit     = "[" [ expr { "," expr } [ "," ] ] "]" ;
mapLit      = "{" [ expr ":" expr { "," expr ":" expr } [ "," ] ] "}" ;
                                       (* `{` is also a block opener; only
                                          legal as a map literal in
                                          expression position, where the
                                          parser is unambiguous *)
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
  integer-result division use `//`: `5 / 2 = 2.5`, `5 // 2 = 2`.
  `//` on float operands returns the floor as a float (`5.7 // 2.0 = 2.0`).
  Line comments are `#` (not `//`), which leaves `//` free for the operator
  and lets a Jennifer file start with a shebang
  (`#!/usr/bin/env -S jennifer run`).
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
- **Lists are array-backed sequences, not Lisp-style linked lists**:
  `def xs as list of int init [1, 2, 3]`. Element access is `$xs[i]`,
  0-indexed, in-bounds-checked. Out-of-bounds reads and writes are
  positioned runtime errors.
- **Maps preserve insertion order**: iteration via `for (def k in $m)`
  visits keys in the order they were first inserted; updating an
  existing key does not move it; appending a new key extends. Reads of
  missing keys are runtime errors - use `has($m, key)` to test.
- **Lists and maps are value-typed**: `$ys = $xs;` copies, function
  parameters bind by copy, and `const` is deep (constness extends to
  every nested element). Aliasing is impossible; mutations through
  `$xs[i] = ...` only affect that binding.
- **Index assignment** (`$xs[i][j] = ...`) walks the chain on a copy of
  the root binding's value, applies the write, and stores the result
  back via `env.Assign`. The const-target check fires once against the
  root binding; deep constness falls out of the value-semantics
  invariant.
- **Iteration** (`for (def x in $coll)`) opens a fresh scope per
  iteration. The loop variable is bound to each element (list) or key
  (map). The collection is evaluated once at loop entry; concurrent
  mutation of the original binding during iteration doesn't affect the
  walk because the iterator works against a snapshot.
- **`{` is overloaded**: it opens a block in statement position and a
  map literal in expression position. The parser disambiguates by
  context; the formatter (which doesn't run the parser) tracks the
  classification through a small stack so both forms get the right
  indentation and spacing.

## Parser (`internal/parser`)

Recursive descent with precedence climbing for binary operators. The
grammar the parser implements is the EBNF above.

### AST nodes

| Node          | Kind  | Fields                                       |
|---------------|-------|----------------------------------------------|
| `Program`     | root  | `Imports []*ImportStmt`, `Methods []*MethodDef`, `Structs []*StructDef`, `TopLevel []Stmt` |
| `ImportStmt`  | stmt  | `Name`, `AsName` (empty unless `use NAME as ALIAS;`) |
| `MethodDef`   | stmt  | `Name`, `Params []Param`, `Body *Block`      |
| `Param`       | -     | `Name`, `Type`                               |
| `StructDef`   | stmt  | `Name`, `Fields []StructField` (M13.1; top-level only, hoisted before execution) |
| `StructField` | -     | `Name`, `Type` (each field of a struct definition)        |
| `Block`       | stmt  | `Stmts []Stmt`                               |
| `DefineStmt`  | stmt  | `IsConst`, `VarName`, `VarType Type`, `InitExpr Expr` (nil = uninit) |
| `AssignStmt`  | stmt  | `VarName`, `Value Expr`                      |
| `IndexAssignStmt` | stmt | `Target *IndexExpr`, `Value Expr` - `$xs[i][j] = ...` (M13.1: chain may include `FieldAccessExpr` nodes) |
| `FieldAssignStmt` | stmt | `Target *FieldAccessExpr`, `Value Expr` - `$p.field = ...` (M13.1) |
| `AppendStmt`  | stmt  | `Target *VarExpr`, `Value Expr` - `$xs[] = item;` (M9) |
| `ReturnStmt`  | stmt  | `Value Expr` (nil for bare `return;`)        |
| `IfStmt`      | stmt  | `Cond`, `Then *Block`, `ElseIfs []Expr`, `ElseIfBodies []*Block`, `Else *Block` |
| `WhileStmt`   | stmt  | `Cond`, `Body *Block`                        |
| `ForStmt`     | stmt  | `Init Stmt`, `Cond Expr`, `Step Stmt`, `Body *Block` (any may be nil) |
| `ForEachStmt` | stmt  | `VarName`, `Coll Expr`, `Body *Block`        |
| `ExprStmt`    | stmt  | `Expr`                                       |
| `IntLit`      | expr  | `Value int64`                                |
| `FloatLit`    | expr  | `Value float64`                              |
| `StringLit`   | expr  | `Value string`                               |
| `BoolLit`     | expr  | `Value bool`                                 |
| `NullLit`     | expr  | -                                            |
| `VarExpr`     | expr  | `Name` (no `$`) - mutable-variable reference  |
| `ConstRefExpr`| expr  | `Name` - bare-IDENT reference; interpreter expects it to resolve to a constant |
| `CallExpr`    | expr  | `Callee`, `Args []Expr`                      |
| `QualifiedCallExpr`     | expr | `Prefix`, `Callee`, `Args []Expr` |
| `QualifiedConstRefExpr` | expr | `Prefix`, `Name` |
| `BinaryExpr`  | expr  | `Op BinaryOp`, `Left`, `Right` (comparison/logical ops return bool; `and`/`or` short-circuit at eval time) |
| `UnaryExpr`   | expr  | `Op UnaryOp` (`OpNeg`/`OpNot`), `Operand` |
| `ListLit`     | expr  | `Elements []Expr` - `[1, 2, 3]`               |
| `MapLit`      | expr  | `Keys []Expr`, `Values []Expr` (parallel) - `{"a": 1}` |
| `IndexExpr`   | expr  | `Target Expr`, `Index Expr` - `$xs[i]`, chained |
| `StructLit`   | expr  | `Name`, `Fields []StructLitField` - `Point{x: 1, y: 2}` (M13.1) |
| `StructLitField` | - | `Name`, `Expr` (one named field in a struct literal) |
| `FieldAccessExpr` | expr | `Target Expr`, `Field` - `$p.field`, chainable with `IndexExpr` (M13.1) |

Every node embeds a `pos{File, Line, Col}` for error reporting and exposes
it via `Node.Pos()` (line/col) and `Node.Filename()` (file path). The file
is populated from the originating token so cross-file diagnostics work.

`Sprint(node)` produces a stable textual representation used by tests.
