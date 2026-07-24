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
| Comparison operators     | `LT` (`<`), `GT` (`>`), `LE` (`<=`), `GE` (`>=`), `EQ` (`==`), `NEQ` (`!=`)                                                                                 |
| Assignment               | `ASSIGN` (`=`)                                                                                                                                              |
| Grouping and punctuation | `LBRACE` (`{`), `RBRACE` (`}`), `LPAREN` (`(`), `RPAREN` (`)`), `LBRACKET` (`[`), `RBRACKET` (`]`), `SEMI` (`;`), `COMMA` (`,`), `COLON` (`:`), `DOT` (`.`), `DOTDOT` (`..`) |

`def` introduces a variable or constant binding (TOKEN_DEFINE); `func`
introduces a method (TOKEN_FUNC). `import` (TOKEN_IMPORT) is for **file
imports** (`import "path.j";`); `use` (TOKEN_USE) is for **library imports**
(`use io;`). `DOT` (`.`) no longer appears in import syntax (paths are
strings now) and is reserved for future expression use.
Comparison tokens `LE`, `GE`, `EQ`, `NEQ` are two-character (`<=`, `>=`, `==`,
`!=`) and are recognized by a one-character lookahead from `<`, `>`, `=`, `!`.
`!` exists **only** as the lead of `!=` (logical negation is the word `not`); a
bare `!` is a positioned lex error whose message points at both `not` and `!=`.
`RETURN` is the keyword behind `return [EXPR];` (see [grammar.md](grammar.md)).

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

Comments and blank lines are emitted as **trivia tokens**
(`TOKEN_COMMENT_LINE`, `TOKEN_COMMENT_BLOCK`,
`TOKEN_COMMENT_SHEBANG`, `TOKEN_BLANK_LINE`) so `jennifer fmt`
can round-trip them. The preprocessor and parser strip these
tokens at entry; the formatter walks the raw lexer stream. See
[Comments](#comments) below.

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

`# ...` runs to end of line and emits `TOKEN_COMMENT_LINE`; the special
case of `#!` on line 1 col 1 emits `TOKEN_COMMENT_SHEBANG` instead so
the formatter can re-emit the shebang verbatim at the file head.
`/* ... */` emits `TOKEN_COMMENT_BLOCK` and **nests** via a depth
counter (increment on `/*`, decrement on `*/`, exit at depth 0).
Unterminated nested comments error positionally at the outermost `/*`
so the message points at where the user meant to start.

Each comment token's `Lexeme` carries the verbatim source text
including the delimiters (`# ...`, `/* ... */`, `#! ...`) so the
formatter round-trips byte-for-byte.

Runs of blank lines collapse to one `TOKEN_BLANK_LINE` per run -
matching the style rule "never more than one consecutive blank line".

`#` was chosen (over the C/Java `//` style) so the floor-division
operator `//` is unambiguous and a Jennifer file can begin with a
Unix shebang (`#!/usr/bin/env -S jennifer run`).

A `.` peeks for a second `.`: two together lex as one `DOTDOT` (the
half-open range operator `a..b`), a single `.` stays `DOT`. Number
scanning already requires a digit after `.` for a fraction, so `1..5`
lexes as `INT DOTDOT INT` (not `1.` `.5`) with no special-casing.

## Identifier rule

Every identifier is **letter-initial** (a token that starts with a digit is
always a number, which keeps lexing unambiguous). Variable, method, parameter
and library names are `[A-Za-z][A-Za-z0-9]{0,63}` - a letter, then letters and
digits, no underscores. Constants use a looser form: uppercase chunks separated
by single `_` characters, with in-chunk digits - `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*`.
Each chunk starts with an uppercase letter (never a digit), and every `_` must be
immediately followed by that letter, so leading, trailing, consecutive, and
underscore-then-digit forms are all rejected (`SHA256` / `SCRAM_SHA256` are legal,
`AES_256` is not - write `AES256`).

The lexer reflects this by accepting digits and `_` as continuation characters
for bare IDENT tokens (so `sha256` and `MAX_RETRIES` are each a single token) but
rejecting any identifier that *ends* with `_`. The full per-kind rule is then
enforced by the parser at each def / use site - variables, methods, parameters,
library names and call callees may not contain `_`; constants may, with the
leading-`_` case already excluded by `isIdentStart`. `$var` references go through
a separate lexer path (`readVarRef`) whose `isIdentPart` allows the leading
letter plus digits but not `_`, so `$x2` lexes and `$foo_bar` lex-errors directly.
