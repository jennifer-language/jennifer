# Grammar and parser

The authoritative grammar for what the parser accepts, plus a quick tour of
the AST node table and the parser's structure.

## The grammar, in two notations

The grammar lives in two companion files, one per notation:

- **[EBNF](grammar_ebnf.md)** - the declarative view: what shapes the
  language contains. Start here to learn or look up the syntax.
- **[PEG](grammar_peg.md)** - the operational view: ordered choice and
  predicates encoding exactly the decisions the recursive-descent parser
  makes (which alternative wins, and which orderings are load-bearing).
  Start here to understand or modify the parser.

Both describe the token stream **after** preprocessing (`include` splices
are already expanded; `use` / `import` do reach the parser), with terminals
written as lexer token classes ([Lexer > Token types](lexer.md#token-types)).
They describe the same language and must be kept in sync - when the syntax
changes, update **both**, in the same commit that changes the parser. The
parser (`internal/parser/parser.go`) is the source of truth.

The semantic rules no grammar can express follow here.

**Semantic notes that aren't expressed in the grammar:**

- Two separate keywords: `def` introduces a binding (variable or constant);
  `func` introduces a method. There's no longer any lookahead disambiguation
  in this area - the parser dispatches purely on the keyword.
- The name in `defineStmt` is a bare `IDENT`. Writing `def $x as int`
  produces a parse error with a hint to drop the `$` (it's reserved for
  use-site references).
- Operator precedence (lowest to highest): `or`, `and`, unary `not`,
  comparison `< > <= >= == !=`, bitwise or `|`, bitwise xor `^`, bitwise and
  `&`, shifts `<< >>`, additive `+ -`, multiplicative `* / // %`, unary `-`
  and `~`. Binary operators are left-associative; `not` and the prefix
  operators are right-associative (`not not x`, `--x`, and `-~x` are all
  valid). The bitwise family (`| ^ & << >> ~`) operates on `int` only.
- `and` and `or` **short-circuit**: the right operand is not evaluated when
  the left already decides the result. Both operands must be `bool`.
- Unary `not` requires `bool`; unary `-` requires `int` or `float`.
- Comparison operators produce `bool`; `if`/`while`/`for` conditions **must**
  be `bool` (no implicit truthiness). The ordering operators (`< > <= >=`)
  compare two numbers or two strings (strings lexicographically by UTF-8 byte);
  a string/number mix is a type error.
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
  the latter resolves to a constant in scope, else to a **first-class
  function value** if the name is a top-level method (the `func` type), else
  a name that resolves to a variable produces a helpful error ("use
  `$name`"); anything else is an undefined-name error.
- A postfix `(...)` on any value expression (not a bare method-name call,
  which parseCallTail consumes) is a **`CallValueExpr`** - a call through a
  function value: `$f(x)`, `$fns[0](x)`, `makeAdder(1)(2)`. The callee must
  evaluate to a non-null `func`; arity and argument kinds are checked at the
  call site exactly as for a named call.
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
grammar the parser implements is documented twice: as
[EBNF](grammar_ebnf.md) (the shapes) and as [PEG](grammar_peg.md) (the
decision order); the PEG is the closer mirror of the code, down to which
`case` order and peeks are load-bearing.

The exported entry points (`Parse`, `ParseTokens`) return a raw
`*Program` without running the scope-analysis pass. Callers
that intend to execute the program must invoke `parser.Resolve(prog)`
themselves (`Interpreter.Run` does this automatically). Splitting the
two lets grammar tests focus on parse trees without wiring up scope
context for every fragment; see [scope analysis](#scope-analysis)
below.

### AST nodes

| Node                    | Kind | Fields                                                                                                     |
| ----------------------- | ---- | ---------------------------------------------------------------------------------------------------------- |
| `Program`               | root | `Imports []*ImportStmt`, `Methods []*MethodDef`, `Structs []*StructDef`, `Enums []*EnumDef`, `TopLevel []Stmt`, `NumGlobals int`                 |
| `ImportStmt`            | stmt | `Name`, `AsName` (empty unless `use NAME as ALIAS;`)                                                       |
| `MethodDef`             | stmt | `Name`, `Params []Param`, `Body *Block`                                                                    |
| `Param`                 | -    | `Name`, `Type`                                                                                             |
| `StructDef`             | stmt | `Name`, `Fields []StructField` (top-level only, hoisted before execution)                           |
| `StructField`           | -    | `Name`, `Type` (each field of a struct definition)                                                         |
| `EnumDef`               | stmt | `Name`, `Variants []EnumVariant` (top-level only, hoisted; a sum type)                              |
| `EnumVariant`           | -    | `Name`, `Fields []StructField` (payload-less when empty)                                            |
| `Block`                 | stmt | `Stmts []Stmt`, `NumSlots int` (hint used by `NewEnvironmentSized`)                                |
| `DefineStmt`            | stmt | `IsConst`, `VarName`, `VarType Type`, `InitExpr Expr` (nil = uninit), `Slot int` (-1 = unresolved) |
| `AssignStmt`            | stmt | `VarName`, `Value Expr`, `Depth`, `Slot` (both -1 = unresolved)                                   |
| `IndexAssignStmt`       | stmt | `Target *IndexExpr`, `Value Expr` - `$xs[i][j] = ...` (chain may include `FieldAccessExpr` nodes)   |
| `FieldAssignStmt`       | stmt | `Target *FieldAccessExpr`, `Value Expr` - `$p.field = ...`                                         |
| `TryStmt`               | stmt | `Body *Block`, `CatchName`, `CatchBody *Block`, `CatchSlot` (slot for `CatchName` in the handler frame) - `try { ... } catch (NAME) { ... }`                |
| `ThrowStmt`             | stmt | `Value Expr` - `throw EXPR;`                                                                       |
| `AppendStmt`            | stmt | `Target *VarExpr`, `Value Expr` - `$xs[] = item;`                                                     |
| `ReturnStmt`            | stmt | `Value Expr` (nil for bare `return;`)                                                                      |
| `IfStmt`                | stmt | `Cond`, `Then *Block`, `ElseIfs []Expr`, `ElseIfBodies []*Block`, `Else *Block`                            |
| `MatchStmt`             | stmt | `Subject Expr`, `Arms []MatchArm`, `Else *Block` (nil if absent) - `match (EXPR) { when ... else ... }`     |
| `MatchArm`              | node | `Values []Expr` (value arm, compared by `==`) OR `Variant`/`Bind`/`BindSlot` (enum-pattern arm, set by the resolver), `Body *Block`         |
| `WhileStmt`             | stmt | `Cond`, `Body *Block`                                                                                      |
| `ForStmt`               | stmt | `Init Stmt`, `Cond Expr`, `Step Stmt`, `Body *Block` (any may be nil)                                      |
| `ForEachStmt`           | stmt | `VarName`, `Coll Expr`, `Body *Block`, `IterSlot` (slot for the iterator in each iteration frame)  |
| `ExprStmt`              | stmt | `Expr`                                                                                                     |
| `IntLit`                | expr | `Value int64`                                                                                              |
| `FloatLit`              | expr | `Value float64`                                                                                            |
| `StringLit`             | expr | `Value string`                                                                                             |
| `BoolLit`               | expr | `Value bool`                                                                                               |
| `NullLit`               | expr | -                                                                                                          |
| `VarExpr`               | expr | `Name` (no `$`), `Depth`, `Slot` (both -1 = unresolved, use name lookup) - mutable-variable reference |
| `ConstRefExpr`          | expr | `Name`, `Depth`, `Slot` (-1 = unresolved) - bare-IDENT reference; interpreter expects it to resolve to a constant |
| `CallExpr`              | expr | `Callee`, `Args []Expr`, `Method *MethodDef` (pre-resolved pointer for hoisted user methods; nil for builtins and resolver-less paths) |
| `CallValueExpr`         | expr | `Callee Expr`, `Args []Expr` (a call through a function value: `$f(x)`; the callee evaluates to a `func` at run time)                |
| `LenExpr`               | expr | `Operand Expr` - `len(EXPR)` language built-in                                                     |
| `QualifiedCallExpr`     | expr | `Prefix`, `Callee`, `Args []Expr`, `Fn any` (pre-resolved `Builtin`; nil for resolver-less paths)     |
| `QualifiedConstRefExpr` | expr | `Prefix`, `Name`, `Const any` (pre-resolved `Value`; nil for resolver-less paths)                    |
| `BinaryExpr`            | expr | `Op BinaryOp`, `Left`, `Right`, `Folded Expr` (pre-computed fold result; nil for runtime-only exprs) |
| `UnaryExpr`             | expr | `Op UnaryOp` (`OpNeg`/`OpNot`/`OpBitNot`), `Operand`, `Folded Expr`                              |
| `ListLit`               | expr | `Elements []Expr` - `[1, 2, 3]`                                                                            |
| `MapLit`                | expr | `Keys []Expr`, `Values []Expr` (parallel) - `{"a": 1}`                                                     |
| `IndexExpr`             | expr | `Target Expr`, `Index Expr` - `$xs[i]`, chained                                                            |
| `RangeExpr`             | expr | `Lo Expr`, `Hi Expr` - half-open `lo..hi`; materialises `list of int` (or iterates lazily as a for-each source) |
| `SliceExpr`             | expr | `Target Expr`, `Lo Expr`, `Hi Expr` (either endpoint nil for an open end) - `$xs[a..b]`, `$xs[a..]`, `$xs[..b]`, `$xs[..]` |
| `StructLit`             | expr | `NS`, `Enum`, `Name`, `Bare`, `Fields []StructLitField` - `Point{...}` / `lib.Point{...}`, reused for enum construction `Shape.Circle{...}` (`NS`=enum) and cross-module `mod.Shape.Circle{...}` (`Enum` set); `Bare` marks a payload-less form |
| `StructLitField`        | -    | `Name`, `Expr` (one named field in a struct literal)                                                       |
| `FieldAccessExpr`       | expr | `Target Expr`, `Field` - `$p.field`, chainable with `IndexExpr`                                    |

Every node embeds a `pos{File, Line, Col}` for error reporting and exposes
it via `Node.Pos()` (line/col) and `Node.Filename()` (file path). The file
is populated from the originating token so cross-file diagnostics work.

`Sprint(node)` produces a stable textual representation used by tests.

### Scope analysis

`internal/parser/resolver.go` is a post-parse pass that walks the AST
and fills in the slot fields (`Depth`, `Slot`, `NumSlots`,
`Program.NumGlobals`, `Block.NumSlots`, etc.). It also promotes two
classes of error from first-execution runtime errors to positioned
parse-time diagnostics:

- **Undefined variables** - `Resolve` walks its own scope stack in
  parallel with the AST and reports any `VarExpr` / `AssignStmt`
  whose name isn't in scope.
- **Shadowing** - a `def` (variable or constant) whose name is
  already visible in an enclosing scope. Same rule the runtime's
  name-based `Define` used to enforce; now caught earlier.

The resolver is idempotent: running twice on the same AST produces
the same annotations. `Interpreter.Run` calls it before any structural
check; `EvalInteractive` (REPL) does not (each REPL turn lacks the
accumulated global context that would let resolution succeed). The
runtime handles the resolver-less path by leaving all slot fields at
the `-1` sentinel and using name-based Environment methods.

**Scope-frame model.** The resolver tracks scopes as a stack. Each
frame carries a name -> slot map and a `count` allocator. A frame is
`isRoot=true` at the boundaries where the runtime chain jumps
directly to globals (the globals frame itself, and a method's
callFrame). Reference lookup walks innermost-out, respects those
root boundaries, and terminates at globals.

Three scope-shape carve-outs where the resolver deliberately deviates
from "one AST scope = one runtime frame" to stay aligned with the
interpreter:

- **`try` body** runs in the enclosing env at runtime; the resolver
  walks its stmts inline in the current scope rather than pushing a
  fresh frame.
- **For-header init** lands in `forEnv` (a frame the resolver pushes
  for the header), body lands in a nested body-frame (pushed by
  `resolveBlock`).
- **Spawn body** is skipped entirely. The runtime's two-frame spawn
  snapshot doesn't line up with a static single-frame view of the
  enclosing scope, so references inside a spawn body stay at
  `(Depth=-1, Slot=-1)` and the interpreter falls back to name-based
  lookup at runtime.

**Method-call pre-resolution.** The resolver also pre-fills
`CallExpr.Method` whenever the callee names a hoisted top-level user
method. The interpreter's `evalCall` consults the pointer first and
skips the `i.methods` hash lookup on every recursive call. Builtins
keep `Method = nil` because the namespaced / global registries need
the runtime `use`-activation check.

**Namespaced-call pre-resolution.** For
`QualifiedCallExpr.Fn` and `QualifiedConstRefExpr.Const` the pre-fill
happens on the interpreter side, not in the parser resolver, because
the namespace / import tables don't exist until `processImports` has
run. `Interpreter.resolveQualifiedRefs` is a second pass invoked from
`Interpreter.Run` after `processImports` that walks the same AST and
stamps the exact `Builtin` / `Value` a call would otherwise look up.

**Constant folding.** `internal/parser/fold.go` runs from
inside `Resolve` as a post-step on `BinaryExpr` / `UnaryExpr`. When
both operands are literal (checked transitively through their own
`Folded` fields via `asLit`), the operator is applied at parse time
and the result stamped on `Folded` as a fresh literal node. Chains
collapse in a single pass - `((1+2)*3)+4` folds to `IntLit(11)`.
Operations that would error at runtime (division by zero, negative
shift count, unknown op) leave `Folded` nil so the runtime hits the
error at its actual source position.

See [interpreter.md > Environment](interpreter.md#environment) for
the runtime side.
