# Grammar - EBNF

The declarative view of the grammar: what shapes the language contains, in
ISO-style EBNF. For how the parser *decides* between alternatives (ordered
choice, lookahead), see the [PEG grammar](grammar_peg.md) - the two describe
the same language and must be kept in sync; the parser
(`internal/parser/parser.go`) is the source of truth for both. Semantic
notes that no grammar expresses (precedence prose, scoping, value semantics)
stay in [grammar.md](grammar.md).

This grammar describes the token stream **after** preprocessing - file
splices (`include STRING ;`) are expanded before the parser runs, so
they don't appear here. Library imports (`use IDENT ;`) and module
imports (`import STRING [ "as" IDENT ] ;`) do reach the parser: `use`
becomes an `ImportStmt`, `import` a `ModuleImportStmt`.

Terminals in CAPITALS are token classes from the lexer (see
[Lexer > Token types](lexer.md#token-types)); quoted strings are keywords or
punctuation that match the corresponding token's lexeme.

```ebnf
program     = { useStmt | moduleImport | exported | methodDef | structDef | enumDef | statement } EOF ;
exported    = "export" ( methodDef | structDef | enumDef | constDefine ) ;
                                       (* `export` publishes a name from a
                                          module; it may only precede a
                                          `func`, `def struct`, `def enum`, or
                                          `def const`.
                                          Whether a program may contain
                                          `export` at all (module vs script)
                                          is decided at load time, not by the
                                          grammar. *)
useStmt     = "use" IDENT [ "as" IDENT ] ";" ;      (* library import; the
                                                       optional "as ALIAS"
                                                       renames the namespace
                                                       at the use site *)
moduleImport = "import" STRING [ "as" IDENT ] ";" ;  (* module import; the
                                                       STRING path must end
                                                       in ".j". Top-level
                                                       only - a module is a
                                                       declaration, so an
                                                       import inside a block
                                                       is a parse error *)
methodDef   = "func" IDENT "(" [ paramList ] ")" block ;
paramList   = param { "," param } ;
param       = IDENT "as" type ;
block       = "{" { statement } "}" ;

structDef   = "def" "struct" IDENT "{" [ structField { "," structField } [ "," ] ] "}" ";" ;
                                       (* top-level only;
                                          IDENT names the struct type;
                                          zero fields parse. Hoisted
                                          before the first top-level
                                          statement runs. *)
enumDef     = "def" "enum" IDENT "{" enumVariant { "," enumVariant } [ "," ] "}" ";" ;
                                       (* top-level only; a sum type. IDENT
                                          names the enum; at least one variant.
                                          Hoisted like structDef. A value of the
                                          type is exactly one variant. *)
enumVariant = IDENT [ "{" structField { "," structField } [ "," ] "}" ] ;
                                       (* a payload-less tag, or a variant with
                                          named fields (a mini-struct). *)
structField = wordName "as" type ;     (* a field name may be any
                                          identifier-shaped word (see
                                          wordName below) - `from` / `to`
                                          are fine; camelCase (no `_`)
                                          is enforced as a check *)

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
            | repeatStmt
            | matchStmt
            | breakStmt
            | continueStmt
            | exitStmt
            | tryStmt
            | throwStmt
            | deferStmt
            | exprStmt ;

tryStmt     = "try" block "catch" "(" IDENT ")" block ;
                                       (* IDENT is the catch
                                          binding, follows the
                                          iteration-variable name rule
                                          (letters only). No `finally`
                                          in v1. *)
throwStmt   = "throw" expr ";" ;
                                       (* expr may produce any
                                          value; convention is an
                                          `Error` struct. *)
deferStmt   = ( "defer" | "errdefer" ) deferCall ";" ;
deferCall   = "(" deferCall ")" | qualifiedCall | call ;
                                       (* exactly a call - a plain call
                                          (`cleanup()`) or a namespaced /
                                          module call (`fs.close($f)`);
                                          grouping parens are transparent
                                          (`defer (f());` is accepted),
                                          any other expr is a parse
                                          error. Args evaluate at the
                                          defer site; the call runs
                                          when the enclosing block
                                          exits, LIFO. `defer` runs on
                                          every exit path; `errdefer`
                                          only when the block exits
                                          with a propagating error (a
                                          throw / runtime error - not
                                          return, break, continue, or
                                          exit). Block-scoped. *)

returnStmt  = "return" [ expr ] ";" ;
breakStmt   = "break" ";" ;            (* exits the innermost loop *)
continueStmt = "continue" ";" ;        (* skips to the next iteration *)
exitStmt    = "exit" [ expr ] ";" ;    (* terminates the program; the
                                          optional int expr is the exit
                                          code (0 when omitted) *)

constDefine = "def" "const" IDENT "as" type "init" expr ";" ; (* the const
                                          form of defineStmt - the only `def`
                                          an `export` may mark *)
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
                                          root is a VARREF. Tail
                                          steps may freely mix `[index]`
                                          and `.field`. *)

fieldAssign = VARREF lvalueTail { lvalueTail } "." wordName "=" expr ";" ;
                                       (* l-value chain ending in `.field`.
                                          Root is a VARREF; tail
                                          may mix `[index]` and `.field`. *)

lvalueTail  = "[" expr "]" | "." wordName ;

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

repeatStmt  = "repeat" block "until" "(" expr ")" ";" ;
                                       (* post-test loop: the body runs at
                                          least once; exits when the
                                          condition is true *)

matchStmt   = "match" "(" expr ")" "{"
                  { "when" ( valueArm | patternArm ) block }
                  [ "else" block ]
              "}" ;
valueArm    = expr { "," expr } ;      (* one or more values compared to the
                                          subject by `==` *)
patternArm  = IDENT [ "(" IDENT ")" ] ; (* an enum-variant pattern: a variant
                                          name, optionally binding the payload
                                          into a fresh name *)
                                       (* Two arm forms, disambiguated at resolve
                                          time by the subject's type. When the
                                          subject is a variable / parameter of a
                                          known enum type, every arm is a
                                          patternArm (variant + optional binder),
                                          checked for exhaustiveness (cover every
                                          variant or add `else`). Otherwise arms
                                          are valueArms: the subject is compared
                                          to each value by `==`, first match wins,
                                          `else` optional and last, no
                                          fall-through. A bare `Name` value
                                          followed by `{` reads the `{` as the arm
                                          block - parenthesize a struct-literal
                                          value. *)

exprStmt    = expr ";" ;

type        = primType | listType | mapType | taskType | channelType | funcType | structType ;
primType    = "int" | "float" | "string" | "bool" | "null" | "bytes" ;
funcType    = "func" ;                 (* a first-class function value; the
                                          type carries no signature - arity
                                          and argument kinds are checked at
                                          the call site *)
channelType = "channel" "of" type ;    (* a CSP channel. `channel` is a
                                          CONTEXTUAL keyword - a type only in
                                          this `channel of T` position; the `of`
                                          disambiguates it from a struct name, so
                                          `channel` stays an ordinary identifier
                                          everywhere else *)
listType    = "list" "of" type ;
mapType     = "map" "of" type "to" type ;
taskType    = "task" "of" type ;       (* `task of T` - handle to
                                          a `spawn`ed computation. Same
                                          shape as `list of T`; recurses
                                          the same way (`task of list of
                                          int` is legal). *)
                                       (* recursive; nesting like
                                          `list of list of int` and
                                          `map of string to list of int`
                                          falls out naturally *)
structType  = IDENT [ "." IDENT ] ;    (* User-defined struct OR enum type
                                          (bare IDENT), or a library-/module-
                                          provided namespaced type
                                          (`IDENT.IDENT`). The parser cannot tell
                                          a struct name from an enum name here;
                                          both are resolved at runtime against
                                          the struct / enum / NSStructs tables.
                                          Unknown names are positioned errors. *)

expr        = rangeExpr ;
rangeExpr   = orExpr [ ".." orExpr ] ;
                                       (* half-open range `lo..hi` -> `[lo, hi)`.
                                          Non-associative (`a..b..c` is an
                                          error) and looser than every binary
                                          operator, so `1+1..2*3` parses as
                                          `(1+1)..(2*3)`. Both bounds int; a
                                          range materialises a `list of int`,
                                          or - as a for-each source - iterates
                                          lazily. `lo > hi` is a runtime error;
                                          `lo == hi` is empty. *)
orExpr      = andExpr { "or" andExpr } ;
andExpr     = notExpr { "and" notExpr } ;
notExpr     = "not" notExpr | compExpr ;
compExpr    = bitOrExpr { ("<" | ">" | "<=" | ">=" | "==" | "!=") bitOrExpr } ;
bitOrExpr   = bitXorExpr { "|" bitXorExpr } ;
bitXorExpr  = bitAndExpr { "^" bitAndExpr } ;
bitAndExpr  = shiftExpr { "&" shiftExpr } ;
shiftExpr   = addExpr { ("<<" | ">>") addExpr } ;
                                       (* the bitwise family (`|` `^` `&`
                                          `<<` `>>`, unary `~`) operates on
                                          int only; precedence sits between
                                          comparison and additive, tightest
                                          first: shift, then `&`, `^`, `|` *)
addExpr     = mulExpr { ("+" | "-") mulExpr } ;
mulExpr     = unaryExpr { ("*" | "/" | "//" | "%") unaryExpr } ;
unaryExpr   = ("-" | "~") unaryExpr | primary ;
primary     = ( INT | FLOAT | STRING | interpString | "true" | "false" | "null"
              | VARREF | qualifiedCall | qualifiedConstRef | taskCall
              | call | structLit | constRef | "(" expr ")"
              | listLit | mapLit | lenExpr | spawnExpr )
              { "[" ( expr | sliceTail ) "]" | "." wordName | callArgs } ;

interpString = STRING_INTERP ;         (* a cooked "..." string carrying one or
                                          more `{expr}` interpolation slots. The
                                          lexer emits it as one TOKEN_STRING_INTERP
                                          whose Parts alternate literal chunks and
                                          expression-source slots; the parser
                                          sub-parses each slot to a single `expr`
                                          and builds an InterpStringExpr (lowered
                                          to concat + convert.toString). A slot is
                                          one expr, not a statement; `\{`/`\}` are
                                          literal braces; a bare `}` is a lex
                                          error; a raw '...' string never
                                          interpolates. STRING is a cooked or raw
                                          literal with no slots. *)
                                       (* any primary can be index-,
                                          slice-, field-, or call-chained. A
                                          `[...]` holds either an index `expr`
                                          or a `sliceTail` (contains a `..`);
                                          a trailing `callArgs` is a call
                                          through a function value
                                          (CallValueExpr): `$f(x)`,
                                          `$fns[0](x)`, `makeAdder(1)(2)`. *)
callArgs    = "(" [ expr { "," expr } ] ")" ;
sliceTail   = orExpr ".." [ orExpr ]   (* `[a..]`, `[a..b]` *)
            | ".." [ orExpr ] ;        (* `[..]`, `[..b]` - endpoints parse at
                                          orExpr so a bool-keyed comparison
                                          index `$m[$a == $b]` stays an index,
                                          not a slice. Slice yields a fresh,
                                          value-semantic copy (never a view);
                                          open ends default to 0 / len; strict
                                          `0 <= lo <= hi <= len` bounds check;
                                          works on list / bytes / string
                                          (rune-indexed). Read-only:
                                          `$xs[a..b] = ...` is a parse error. *)
spawnExpr   = "spawn" block ;          (* launches the block as a
                                          goroutine and evaluates
                                          immediately to a `task of T`
                                          where T is the body's return
                                          type at the use site. Bare
                                          `return;` produces `task of
                                          null`. Value-semantics
                                          capture: every binding visible
                                          at the spawn site is
                                          deep-copied into a fresh frame
                                          at launch. *)
lenExpr     = "len" "(" expr ")" ;     (* polymorphic
                                          structural-length built-in
                                          (string / list / map /
                                          bytes). Reserved keyword,
                                          not a library function; the
                                          `core` library that once
                                          hosted it no longer exists. *)
structLit   = IDENT [ "." wordName [ "." wordName ] ] "{" [ structLitField { "," structLitField } [ "," ] ] "}" ;
                                       (* struct OR enum literal (same node).
                                          Bare IDENT names a user struct;
                                          `IDENT.IDENT` is a namespaced struct
                                          (`os.Result{...}`) OR a local enum
                                          variant (`Shape.Circle{...}`);
                                          `IDENT.IDENT.IDENT` is a cross-module
                                          enum variant (`mod.Shape.Circle{...}`).
                                          The enum-vs-struct decision is made at
                                          eval from the tables, not from
                                          capitalisation. A payload-less enum
                                          variant (`Shape.Empty`) has no `{...}`
                                          and reaches eval as a constRef the
                                          interpreter resolves against the enum.
                                          The `{` after IDENT in expression
                                          position is the tie-breaker against
                                          `constRef`. `P{}` parses; "every field
                                          present exactly once" is a post-parse
                                          check. *)
structLitField = wordName ":" expr ;
call        = IDENT "(" [ expr { "," expr } ] ")" ;
qualifiedCall      = IDENT "." wordName "(" [ expr { "," expr } ] ")" ;
qualifiedConstRef  = IDENT "." wordName ;
                                       (* qualifiedCall / qualifiedConstRef:
                                          IDENT "." wordName, then `(` decides
                                          which. Resolved against the
                                          namespaced-builtin / constant
                                          registry, gated by `use lib;`
                                          (or alias-aware equivalent). *)
taskCall    = "task" "." wordName "(" [ expr { "," expr } ] ")" ;
                                       (* the `task` library's namespace. `task`
                                          is a type keyword, not an IDENT, so
                                          `qualifiedCall` (IDENT-led) cannot
                                          match it - hence its own production.
                                          Expression position only, as the
                                          namespace prefix (`task.wait($t)`). *)
constRef    = IDENT ;                  (* bare-IDENT: constant reference; the
                                          parser disambiguates `call` vs
                                          `qualifiedCall` vs `constRef` by
                                          peeking for "." / "(". *)
wordName    = IDENT | KEYWORD ;        (* any identifier-shaped token: a plain
                                          IDENT or a keyword spelled like one
                                          (every Jennifer keyword is). Name
                                          positions - after a ".", and field
                                          names in struct definitions and
                                          literals - are contextually
                                          unambiguous, so reserved words are
                                          valid names there:
                                          `strings.repeat(...)`,
                                          `def struct Route { from as Point,
                                          to as Point };`, `$r.to`. *)
listLit     = "[" [ expr { "," expr } [ "," ] ] "]" ;
mapLit      = "{" [ expr ":" expr { "," expr ":" expr } [ "," ] ] "}" ;
                                       (* `{` is also a block opener; only
                                          legal as a map literal in
                                          expression position, where the
                                          parser is unambiguous *)
```

A type keyword (`int`, `float`, `string`, `bool`, `bytes`) has **no
expression-position meaning**: the parser reports a positioned error pointing
at the `convert` library (`convert.toInt(v)`, ...; `convert.bytesFromString(s,
codec)` for bytes). `task` is the one type keyword with an expression role -
solely as the `task.` namespace prefix (`task.wait($t)`).
