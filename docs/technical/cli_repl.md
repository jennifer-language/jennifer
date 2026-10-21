# REPL (`cmd/jennifer/repl.go`)

The REPL drives a read-eval-print loop on top of the standard pipeline. Each
input is lexed, preprocessed, parsed, and fed to `Interpreter.EvalInteractive`
(not `Run`). `EvalInteractive` differs from `Run` in three ways:
the global env is lazy-initialized and preserved across calls, library
imports and function definitions are idempotent / re-assignable so the user
can iterate, and the value of a trailing `ExprStmt` is returned so the loop
can print it.

One safety restriction: input that defines functions or structs, or adds
`use` / `import` statements, is rejected while a spawned task is still
running. Spawn bodies resolve function calls, struct lookups, and namespace
prefixes by name from their own goroutines, so mutating those shared
tables mid-flight would be a data race (Go's fatal "concurrent map read
and map write"). Observe the task first - `task.wait($t)` or
`task.discard($t)` - or let it finish; plain statements evaluate
normally in the meantime (spawn snapshots are isolated from the live
global frame).

Both import kinds work at the prompt: `use LIB;` activates a library
namespace, and `import "PATH.j";` loads a module (`runRepl` calls
`EnableModules` with the current directory as the local-import base and the
system module dir as the search path, so `./mod.j` resolves against the cwd and
a bare `mod.j` through the search path). `EvalInteractive` calls
`loadModuleImports` in REPL mode, which no-ops a re-submitted `import` of the
same module under the same alias (a module is run-once / cached) while still
rejecting an alias bound to a different module. Because caching is by resolved
path, editing a module file and re-importing it in the same session serves the
cached version - restart the REPL to pick up edits.

## Echoing a value

Because `EvalInteractive` returns the trailing `ExprStmt`'s value, you can
inspect any variable by typing **its bare reference followed by `;`** - the
REPL prints the value's `Display()` form:

```jennifer
>>> def x as int init 41;
>>> $x;
41
>>> def doc as json.Value init json.decode("{\"a\":[1,2]}");
>>> $doc;
{"a":[1,2]}
```

This is a REPL-only convenience: `Run` (the batch path) evaluates an
expression statement but discards its value, so a bare `$x;` in a `.j`
**script prints nothing**. To show a value from a script, format it
explicitly - `io.printf("%v\n", $x);` (or `io.sprintf($x)` /
`convert.toString($x)`). Opaque values render through their registered
displayer, so `$doc;` shows a `json.Value` as its JSON rather than
`<json.Value>`.

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

## Line editor (`cmd/jennifer/lineedit.go`, `cmd/jennifer/history.go`)

When stdin is a terminal the REPL installs raw mode via
`golang.org/x/term` and reads lines through a small built-in editor.
The editor is a single state machine over a rune buffer plus a cursor
index; each keystroke updates the state and triggers a `redraw()`.

Supported input:

| Key                          | Action                                                                       |
| ---------------------------- | ---------------------------------------------------------------------------- |
| Printable rune               | Insert at cursor                                                             |
| Backspace, Ctrl+H            | Delete char before cursor                                                    |
| Delete (CSI `3~`)            | Delete char at cursor                                                        |
| Left / Right (CSI `D` / `C`) | Move cursor by one char                                                      |
| Home / End (CSI `H` / `F`)   | Jump to line start / end                                                     |
| Ctrl+A / Ctrl+E              | Same as Home / End                                                           |
| Ctrl+Left / Ctrl+Right       | Move by word                                                                 |
| Alt+B / Alt+F                | Same as Ctrl+Left / Ctrl+Right (macOS terminals send these for option-arrow) |
| Ctrl+W, Ctrl+Backspace       | Delete word backward                                                         |
| Ctrl+U                       | Kill from line start to cursor                                               |
| Ctrl+K                       | Kill from cursor to line end                                                 |
| Up / Down                    | History navigation                                                           |
| Ctrl+C                       | Cancel current line (fresh prompt)                                           |
| Ctrl+D on empty buffer       | EOF (exits the REPL)                                                         |
| Ctrl+D on non-empty buffer   | Forward-delete                                                               |

Word boundaries use a small punctuation + whitespace ruleset that's
predictable for source-code editing without needing a full Unicode
word-break implementation. History is an in-memory ring (`replHistory`,
100 entries by default, adjacent duplicates collapsed); on-disk
persistence is a future enhancement.

Non-TTY stdin falls back to the original `bufio` line reader. This
keeps `echo ... | jennifer repl` and integration tests working
unchanged - the editor would do nothing useful on a non-interactive
stream anyway.

Raw mode disables the kernel's `OPOST` flag, so `\n` written to stdout
no longer auto-translates to `\r\n`. The REPL works around this with
a `crlfWriter` wrapper that performs the translation in user
space for error/help/result prints. Cooked-mode output (the banner
printed before raw mode is entered, and anything after raw mode is
restored) goes to `os.Stderr` / `os.Stdout` directly.

The editor only handles single-line editing. Multi-line input via the
continuation prompt (`... `) is still driven by the surrounding REPL
loop's `inputComplete()` check, so unclosed `{` / `(` accumulate
across calls to `editor.readLine`.

### Syntax highlighting on commit (`cmd/jennifer/highlight.go`)

While you type, the line is drawn plain. On Enter, if colour is enabled,
the editor redraws the committed line one last time with syntax
highlighting (`redrawCommitted` -> `highlightLine`) before the newline,
so the source shows coloured just above its output. Highlighting only at
commit (not per keystroke) keeps the edit path cheap and sidesteps
recolouring half-typed, unlexable input.

`highlightLine` lexes the line and wraps each token's source span in an
ANSI SGR colour (keyword, type, string, number, `$var`, comment; other
tokens stay default). It slices by each token's 1-based **rune column**
rather than its lexeme, so the user's exact spacing is preserved and a
processed `TOKEN_STRING` lexeme (quotes stripped, escapes resolved)
can't desync the offsets. A span runs to the next token's start, so
trailing whitespace inherits the token's colour - invisible for
foreground-only codes. The colours are zero-width escapes, so the
editor's cursor-column arithmetic is unaffected; the commit redraw skips
the cursor-back step because a newline follows immediately. On any lex
error (e.g. an unterminated string mid-edit) `highlightLine` returns the
input unchanged, so the line always echoes verbatim.

Colour is gated by `colorEnabled()`: stdout must be a TTY and `NO_COLOR`
(https://no-color.org) must be unset. The editor already requires a TTY
stdin, so the two together mean colour appears only in an
interactive session; piped or redirected output stays plain.


Part of the [CLI reference](cli.md).
