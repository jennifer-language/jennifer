# Grammar - PEG

The operational view of the grammar: a Parsing Expression Grammar whose
**ordered choice** and **predicates** encode exactly the decisions
`internal/parser/parser.go` makes (a recursive-descent parser with fixed
lookahead *is* a PEG evaluator, minus the backtracking it never needs).
Where the [EBNF](grammar_ebnf.md) says *what shapes exist*, this grammar says
*which alternative wins and why*. The two describe the same language and must
be kept in sync; the parser is the source of truth for both.

Like the EBNF, this describes the token stream **after** preprocessing
(`include` splices are already expanded). Terminals in CAPITALS are token
classes from the lexer ([Lexer > Token types](lexer.md#token-types)); quoted
strings are keywords or punctuation matching one token. Because the terminals
are whole tokens, the classic PEG longest-match ordering concern (`"<="`
before `"<"`) does not arise here - the lexer already decided; ordering below
matters only where two alternatives share a *token* prefix.

Notation: `<-` defines, `/` is ordered choice (first match wins, no
reconsidering), `?` `*` `+` repeat greedily, `&e` succeeds iff `e` matches
(consuming nothing), `!e` succeeds iff `e` does not match (consuming
nothing).

```peg
# --- program structure -------------------------------------------------------

Program      <- (UseStmt / ModuleImport / Exported / MethodDef / StructDef
               / Statement)* EOF
Exported     <- "export" (MethodDef / StructDef / ConstDefine)
UseStmt      <- "use" IDENT ("as" IDENT)? ";"
ModuleImport <- "import" STRING ("as" IDENT)? ";"
MethodDef    <- "func" IDENT "(" ParamList? ")" Block
ParamList    <- Param ("," Param)*
Param        <- IDENT "as" Type
Block        <- "{" Statement* "}"

StructDef    <- "def" "struct" IDENT "{" (StructField ("," StructField)* ","?)? "}" ";"
StructField  <- WordName "as" Type
                 # zero fields parse (`def struct E { };`); a field name may be
                 # any identifier-shaped word (WordName, below) - `from` / `to`
                 # are fine - with camelCase (no `_`) enforced as a check

# --- statements --------------------------------------------------------------
# The parser dispatches on the leading token; the keyword alternatives are
# mutually exclusive, so their relative order is free. Within the VARREF-
# rooted group and against ExprStmt the order IS load-bearing: ExprStmt must
# come last (`$x = 1;` would otherwise never be an assignment), and
# ForEachStmt must precede ForStmt (both start `for` "(" - the parser peeks
# for `def IDENT in`, which ordered choice reproduces).

Statement    <- DefineStmt
              / IfStmt / WhileStmt / ForEachStmt / ForStmt / RepeatStmt / MatchStmt
              / BreakStmt / ContinueStmt / ReturnStmt / ExitStmt
              / TryStmt / ThrowStmt / DeferStmt
              / AssignStmt / AppendStmt / IndexAssign / FieldAssign
              / ExprStmt

ConstDefine  <- "def" "const" IDENT "as" Type "init" Expr ";"
DefineStmt   <- "def" "const"? IDENT "as" Type ("init" Expr)? ";"
                 # constants require "init" and the UPPER_CASE name shape;
                 # both are checked by the parser, not the grammar

IfStmt       <- "if" "(" Expr ")" Block
                ("elseif" "(" Expr ")" Block)*
                ("else" Block)?
WhileStmt    <- "while" "(" Expr ")" Block
ForEachStmt  <- "for" "(" "def" IDENT "in" Expr ")" Block
ForStmt      <- "for" "(" ForInit Expr? ";" ForStep? ")" Block
ForInit      <- DefineStmt / AssignStmt / ";"
                 # define / assign consume their own ";"; a lone ";" is the
                 # empty init
ForStep      <- VARREF "=" Expr        # no trailing ";" - the ")" ends it
RepeatStmt   <- "repeat" Block "until" "(" Expr ")" ";"
MatchStmt    <- "match" "(" Expr ")" "{"
                  ("when" Expr ("," Expr)* Block)*
                  ("else" Block)?
                "}"
                 # the subject and each `when` value parse with struct-literal
                 # suppression on (a bare `Name` followed by `{` is a value +
                 # the block, not `Name{...}`); the flag is re-enabled inside
                 # any "(" / "[" / call / list / map, mirroring Go's exprLev.
                 # `else` must be last (enforced by the parser, not the grammar).
BreakStmt    <- "break" ";"
ContinueStmt <- "continue" ";"
ReturnStmt   <- "return" Expr? ";"
ExitStmt     <- "exit" Expr? ";"
TryStmt      <- "try" Block "catch" "(" IDENT ")" Block
ThrowStmt    <- "throw" Expr ";"
DeferStmt    <- ("defer" / "errdefer") DeferCall ";"
DeferCall    <- "(" DeferCall ")" / QualifiedCall / PlainCall
                 # exactly a call - the parser parses a full expression and
                 # rejects any node that is not a call. Grouping parens
                 # evaluate to their inner node, so `defer (f());` is accepted
                 # (hence the recursive first alternative); a postfix chain
                 # (`f()[0]`, `f().g`) is not a call node and fails.

# The four VARREF-rooted statements. AssignStmt first mirrors the parser's
# `$x =` peek; AppendStmt's "[" "]" prefix cannot be an IndexAssign (an index
# expression never starts at "]"), but trying it first keeps the intent
# obvious. In `(LvalueStep !"=")*` the not-predicate stops the greedy star
# one step early, leaving the FINAL `[index]` / `.field` - the one directly
# before "=" - for the explicit tail. Without the predicate a greedy star
# would swallow the last step and the alternative could never match.

AssignStmt   <- VARREF "=" Expr ";"
AppendStmt   <- VARREF "[" "]" "=" Expr ";"
IndexAssign  <- VARREF (LvalueStep !"=")* "[" Expr "]" "=" Expr ";"
FieldAssign  <- VARREF (LvalueStep !"=")* "." WordName "=" Expr ";"
LvalueStep   <- "[" Expr "]" / "." WordName

ExprStmt     <- Expr ";"

# --- types -------------------------------------------------------------------
# The compound heads (`list` / `map` / `task`) and the primitive keywords are
# distinct tokens, so this choice never backtracks; StructType last catches
# the bare / namespaced IDENT forms.

Type         <- ListType / MapType / TaskType / PrimType / StructType
ListType     <- "list" "of" Type
MapType      <- "map" "of" Type "to" Type
TaskType     <- "task" "of" Type
PrimType     <- "int" / "float" / "string" / "bool" / "null" / "bytes"
StructType   <- IDENT ("." IDENT)?

# --- expressions -------------------------------------------------------------
# One rule per precedence level, loosest first (the same ladder the parser
# climbs): or, and, not, comparison, |, ^, &, shifts, additive,
# multiplicative, unary. `A (op A)*` yields left association; NotExpr and
# Unary recurse right.

Expr         <- RangeExpr
RangeExpr    <- OrExpr (".." OrExpr)?   # half-open `lo..hi` -> `[lo, hi)`;
                                        # non-associative (a second `..` is an
                                        # error), looser than every binary op.
                                        # Materialises `list of int`, or - as a
                                        # for-each source - iterates lazily.
OrExpr       <- AndExpr ("or" AndExpr)*
AndExpr      <- NotExpr ("and" NotExpr)*
NotExpr      <- "not" NotExpr / CompExpr
CompExpr     <- BitOrExpr (("<" / ">" / "<=" / ">=" / "==" / "!=") BitOrExpr)*
BitOrExpr    <- BitXorExpr ("|" BitXorExpr)*
BitXorExpr   <- BitAndExpr ("^" BitAndExpr)*
BitAndExpr   <- ShiftExpr ("&" ShiftExpr)*
ShiftExpr    <- AddExpr (("<<" / ">>") AddExpr)*
AddExpr      <- MulExpr (("+" / "-") MulExpr)*
MulExpr      <- Unary (("*" / "/" / "//" / "%") Unary)*
Unary        <- ("-" / "~") Unary / Primary

# Any primary can be postfix-chained with indexing and field access. The
# chain belongs to Primary, not Atom, so `f()[0].x` parses as expected.

Primary      <- Atom Postfix*
Postfix      <- "[" (SliceTail / Expr) "]" / "." WordName
SliceTail    <- OrExpr ".." OrExpr? / ".." OrExpr?
                                        # `$xs[a..b]`, `$xs[a..]`, `$xs[..b]`,
                                        # `$xs[..]`. Endpoints parse at OrExpr so
                                        # a bool-keyed comparison index
                                        # `$m[$a == $b]` stays an index. A slice
                                        # is a fresh value-semantic copy (never a
                                        # view); read-only (`$xs[a..b] = ...` is a
                                        # parse error).

# Atom is where the parser's peek-based disambiguation lives; here it is
# ordered choice over alternatives that share prefixes. IdentExpr and
# TaskExpr expand below.

Atom         <- INT / FLOAT / STRING / "true" / "false" / "null"
              / LenExpr / SpawnExpr / TaskExpr
              / VARREF / IdentExpr
              / "(" Expr ")" / ListLit / MapLit
                 # a type keyword (int / float / string / bool / bytes) is NOT
                 # an atom: expression position rejects it with a positioned
                 # error pointing at convert.toInt / convert.toFloat / ...

LenExpr      <- "len" "(" Expr ")"
SpawnExpr    <- "spawn" Block

# After a bare IDENT the next token decides, in this order: "." starts a
# qualified form, "{" a struct literal, "(" a plain call, anything else a
# constant reference. The order is load-bearing: QualifiedSuffix must be
# tried before StructBody / CallArgs (a "." wins over everything), and the
# bare-IDENT constant reference must come last or it would always win.
# After the "." the same trick repeats: "(" makes it a call, "{" a
# namespaced struct literal, else it is a qualified constant.

IdentExpr    <- IDENT IdentSuffix?
IdentSuffix  <- QualifiedSuffix / StructBody / CallArgs
QualifiedSuffix <- "." WordName (CallArgs / StructBody)?

# WordName: any identifier-shaped token - a plain IDENT or a keyword spelled
# like one (every Jennifer keyword is). Name positions (after a ".", field
# names in struct definitions and literals) are contextually unambiguous, so
# reserved words are valid names there: `strings.repeat(...)`,
# `def struct Route { from as Point, to as Point };`, `$r.to`.

WordName     <- IDENT / KEYWORD

# `task` is a type keyword, not an IDENT, so the IDENT-led call rules
# (PlainCall / QualifiedCall) never match it; TaskExpr below is the one
# production that consumes `task` in expression position - and only as the
# `task` library's namespace prefix.

TaskExpr     <- "task" QualifiedSuffix

# Named the same as their EBNF counterparts:

PlainCall     <- IDENT CallArgs
QualifiedCall <- IDENT "." WordName CallArgs
CallArgs      <- "(" (Expr ("," Expr)*)? ")"
StructBody    <- "{" (StructLitField ("," StructLitField)* ","?)? "}"
StructLitField <- WordName ":" Expr
                 # `P{}` parses; "every field present exactly once" is a
                 # post-parse check (duplicates at parse time, missing fields
                 # at evaluation)

ListLit      <- "[" (Expr ("," Expr)* ","?)? "]"
MapLit       <- "{" (MapEntry ("," MapEntry)* ","?)? "}"
MapEntry     <- Expr ":" Expr
                 # "{" is a block opener in statement position and a map
                 # literal in expression position; the split between
                 # Statement and Expr contexts keeps the two from meeting
```

## Reading the load-bearing orderings

Three places where PEG's first-match-wins is doing real work, and what the
parser does at the same juncture:

- **`Statement`: `ExprStmt` last.** The parser peeks: `$x` followed by `=`
  is an assignment, `$xs[...]...` ending in `= expr ;` an index/field
  assignment, and only a `$x` chain *not* followed by `=` falls through to
  an expression statement. Ordered choice reproduces that priority; putting
  `ExprStmt` earlier would shadow every assignment form.
- **`IdentSuffix` / `QualifiedSuffix`: call before constant.** The parser
  decides `call` vs `constRef` (and `qualifiedCall` vs `qualifiedConstRef`)
  by peeking for `(`. In PEG the bare-name alternative simply comes last: if
  `CallArgs` matches, it was a call; only when nothing longer matches does
  the name stand alone.
- **`IndexAssign` / `FieldAssign`: `(LvalueStep !"=")*`.** PEG repetition is
  greedy and never gives back, so `LvalueStep* "[" Expr "]"` could not work -
  the star would consume the final step. The `!"="` predicate stops the star
  exactly one step before the `=`, which is precisely the parser's approach
  of scanning the l-value chain while watching for the assignment that
  terminates it.
