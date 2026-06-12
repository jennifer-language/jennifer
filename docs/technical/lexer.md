# Lexer (`internal/lexer`)

A hand-written, single-pass scanner.

## Token types

| Group                    | Tokens                                                                                                                                                      |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Markers                  | `EOF`, `ILLEGAL`                                                                                                                                            |
| Literal values           | `INT`, `FLOAT`, `STRING`, `TRUE`, `FALSE`, `NULL`                                                                                                           |
| Identifiers              | `IDENT`, `VARREF`                                                                                                                                           |
| Declaration keywords     | `DEFINE` (`def`), `FUNC`, `AS`, `INIT`, `CONST`, `RETURN`                                                                                                   |
| Import keywords          | `USE`, `IMPORT`                                                                                                                                             |
| Control-flow keywords    | `IF`, `ELSEIF`, `ELSE`, `WHILE`, `FOR`                                                                                                                      |
| Type keywords            | `INT_TYPE`, `FLOAT_TYPE`, `STRING_TYPE`, `BOOL_TYPE`, `LIST`, `MAP`                                                                                         |
| Type structure keywords  | `OF`, `TO`                                                                                                                                                  |
| Iteration keyword        | `IN`                                                                                                                                                        |
| Keyword operators        | `AND`, `OR`, `NOT`                                                                                                                                          |
| Arithmetic operators     | `PLUS` (`+`), `MINUS` (`-`), `STAR` (`*`), `SLASH` (`/`), `DIV` (`//`), `PERCENT` (`%`)                                                                     |
| Comparison operators     | `LT` (`<`), `GT` (`>`), `LE` (`<=`), `GE` (`>=`), `EQ` (`==`)                                                                                               |
| Assignment               | `ASSIGN` (`=`)                                                                                                                                              |
| Grouping and punctuation | `LBRACE` (`{`), `RBRACE` (`}`), `LPAREN` (`(`), `RPAREN` (`)`), `LBRACKET` (`[`), `RBRACKET` (`]`), `SEMI` (`;`), `COMMA` (`,`), `COLON` (`:`), `DOT` (`.`) |

`def` introduces a variable or constant binding (TOKEN_DEFINE); `func`
introduces a method (TOKEN_FUNC). `import` (TOKEN_IMPORT) is for **file
imports** (`import "path.j";`); `use` (TOKEN_USE) is for **library imports**
(`use io;`). `DOT` (`.`) no longer appears in import syntax (paths are
strings now) and is reserved for future expression use.
Comparison tokens `LE`, `GE`, `EQ` are two-character (`<=`, `>=`, `==`) and
are recognized by a one-character lookahead from `<`, `>`, `=`. `RETURN`
is the keyword behind `return [EXPR];` (see [grammar.md](grammar.md)).

`VARREF` carries the variable name *without* the leading `$`.
`STRING` carries the value *with* escape sequences already processed and *without*
surrounding quotes.

## Whitespace handling

Spaces, tabs and newlines are discarded between tokens; they only
ever advance `Line` / `Col` for position tracking. There is no
indentation-significant mode and no off-side rule. The user-facing
consequence is documented in
[user-guide/syntax.md > Tokens and whitespace](../user-guide/syntax.md#tokens-and-whitespace);
the rule is load-bearing for `jennifer fmt`, which trusts that
re-emitting the token stream with canonical spacing produces a
semantically identical program.

The only place whitespace is *retained* is inside string literals -
`readString` reads byte-by-byte until the closing quote, so a literal
space, tab, or even a raw `\n` between the quotes becomes part of
the string value. Escape sequences (`\n`, `\t`, ...) are the
conventional spelling; raw multi-line literals work too but aren't
the canonical form `fmt` produces.

Comments are also whitespace-like for parsing purposes: `# ...` to
end of line and `/* ... */` are skipped after they're recognised,
producing no token. See [Comments](#comments) below.

## Position tracking

Every token records `Line` and `Col` (both 1-based) and `File` (the absolute
path supplied to `TokenizeWithFile`, or `""` for unattributed input). The
`advance()` helper bumps `line` on `\n` and otherwise bumps `col`. `File`
flows from the token to the AST node (every node embeds `pos{File, Line,
Col}`), so errors raised inside an imported `.j` still point at the
imported file - see [Interpreter > Errors and positions](interpreter.md#errors-and-positions-cross-file).

## Keywords

The lexer's keyword map covers: `def func as init const import use return if
elseif else while for true false null and or not int float string bool`.
Anything else lexed as a word stays a `TOKEN_IDENT`. `define` is **not** a
keyword and lexes as a plain identifier. `div` was removed when `//` took
over floor division.

## Comments

`# ...` runs to end of line. `/* ... */` is non-nesting and reports an
"unterminated block comment" error if unclosed. `#` was chosen (over the
C/Java `//` style) so the floor-division operator `//` is unambiguous and
a Jennifer file can begin with a Unix shebang
(`#!/usr/bin/env -S jennifer run`).

## Identifier rule

Variable, method, parameter and library names use `[A-Za-z]{1,64}` only -
no digits, no underscores. Constants use a looser form: uppercase chunks
separated by single `_` characters - `[A-Z]+(_[A-Z]+)*`. Every `_` must
be immediately followed by `[A-Z]`, so leading, trailing and consecutive
underscores are all rejected.

The lexer reflects this by accepting `_` as a continuation character for
bare IDENT tokens (so `MAX_RETRIES` is a single token) but rejecting any
identifier that *ends* with `_`. The full per-kind rule is then enforced
by the parser at each def / use site - variables, methods, parameters,
library names and call callees may not contain `_`; constants may, with
the leading-`_` case already excluded by `isIdentStart`. `$var` references
go through a separate lexer path (`readVarRef`) that still uses the
strict letters-only `isIdentPart`, so `$foo_bar` lex-errors directly.
