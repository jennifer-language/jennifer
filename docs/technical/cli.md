# CLI (`cmd/jennifer`)

```
jennifer run <file.j>     run a Jennifer program
jennifer run -            read source from stdin
jennifer repl             interactive REPL
jennifer tokens <file.j>  dump the lexer's token stream
jennifer ast <file.j>     dump the preprocessed AST as JSON
jennifer fmt <file.j>     format source per docs/stylespec.md
jennifer version          print the build version and exit
jennifer help             show usage
```

- Verifies the `.j` extension
- Reads the file, parses, runs
- On error: prints the message and a source-context caret on stderr, exits `1`
- Bad usage exits `2`
- `jennifer help` includes a `Version:` line so the build is identifiable at a glance

## REPL (`cmd/jennifer/repl.go`)

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

## Inspection: `tokens` and `ast`

`cmd/jennifer/dump.go` and `cmd/jennifer/astjson.go` implement two
read-only inspection subcommands. `tokens` runs only the lexer and
prints one token per line in column-aligned `LINE:COL TYPE [lexeme]`
form - useful for tracing scanning issues and as a teaching tool.
`ast` runs lex + preproc + parse and writes the resulting AST as
two-space-indented JSON; every node carries `type`, `file`, `line`,
`col`, plus its node-specific fields.

The JSON emitter is hand-rolled in `astjson.go`'s `emitNode` (a switch
over every concrete AST type). We avoid `encoding/json` because its
reflect-based marshaling is fragile under TinyGo and at odds with the
tagged-union `Value` discipline used elsewhere; a switch over ~20 node
kinds is small enough to keep readable. Each field-emitter
(`emitStringField`, `emitBoolField`, `emitNodeListField`, etc.) writes
`"key": value,` and the closing `endObj` trims the trailing comma so
the output is valid JSON.

## Formatter (`cmd/jennifer/fmt.go`)

`jennifer fmt` formats source per [../stylespec.md](../stylespec.md). It
operates on the lexer's token stream rather than the AST, for two
reasons:

1. **`import "file.j";` survives.** The preprocessor consumes file
   imports before the parser sees them; an AST-based formatter would
   inline every import, which is the opposite of what a developer
   wants from `fmt`. The token-level formatter sees IMPORT tokens
   unchanged and re-emits them.
2. **User-written parens survive.** The AST records grouping only
   through nesting structure; redundant parens are erased. A token
   walker preserves LPAREN/RPAREN exactly as written, so
   `($a + $b) * $c` stays parenthesized after a round trip.

`formatTokens(tokens)` drives a small state machine (`fmtState`): for
each token it computes the separator (`writeSeparator`) - none, a
space, or a newline-plus-indent - and then writes the token's canonical
spelling (`writeToken`). Key state fields:

- `indent` bumps on `{` and drops on `}` (the closing brace dedents
  *before* it's written so it lands at the outer indent).
- `prevIsOperand` answers "is the next `-` binary or unary?" - flipped
  by `isOperandToken` after every emit.
- `prevIsUnaryMinus` suppresses the right-side space after a `-` that
  was determined to be unary.
- `insideForHeader` is a small backward scan that lets the two `;`s
  inside `for (...; ...; ...)` stay on the same line.

Strings are re-quoted with `quoteJenniferString` (double quotes plus
standard escapes), mirroring the lexer's `readString` on the way in.

Known v1 limitations are documented in stylespec.md: comments are
stripped by the lexer and not preserved; blank lines aren't preserved
or auto-inserted between logical groups.

## Version injection

`internal/version.Version` holds the build version as a single `string`.
The default is `"dev"`; the `Makefile` runs `scripts/gen-version.sh` before
each build, which writes `internal/version/version_gen.go` containing a
small `init()` that overwrites `Version` with the output of
`scripts/version.sh` (a `git describe --tags --long` derivative; see
[../libraries/core.md](../libraries/core.md) for the format).

This codegen path replaces the more conventional `go build -ldflags
"-X .Version=..."` because TinyGo 0.41 silently ignores the `-X`
directive. Codegen works identically on both toolchains. The generated
file is `.gitignore`d so the repository never carries a stale copy.

Two consumers read `version.Version`:

- `cmd/jennifer/main.go` prints it in the `help` banner and as the body
  of the `version` subcommand.
- `internal/lib/core/corelib.go` mirrors it into the interpreter as the
  `JENNIFER_VERSION` constant. `core` is auto-loaded, so Jennifer
  programs read it directly: `printf("%s\n", JENNIFER_VERSION);` (no
  `use` statement).

`go test ./...` skips codegen and uses the default `"dev"`. The core-lib
test only checks that the constant matches `version.Version`, not a
specific value, so it stays robust across builds.
