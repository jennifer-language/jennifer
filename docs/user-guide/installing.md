# Installing & running

You need a working [TinyGo](https://tinygo.org/) toolchain (or regular Go for
development). From the repository root:

```sh
# Build the interpreter (TinyGo - the shipping toolchain)
make build

# Or build with Go (faster, for development)
make build-go

# Run a Jennifer source file (must have .j extension)
./jennifer run examples/hello.j

# Print the build version
./jennifer version
```

The `make` targets regenerate `internal/version/version_gen.go` from git
state before invoking the toolchain, so `./jennifer version` always
reflects the current commit. See [../libraries/core.md](../libraries/core.md)
for the `JENNIFER_VERSION` string format. `JENNIFER_VERSION` is
auto-loaded - it's in scope in every program without any `use` statement.

You can also pipe source in on stdin by passing `-` as the filename:

```sh
echo 'use io; io.printf("hi\n");' | ./jennifer run -
./jennifer run - < program.j
cat program.j | ./jennifer run -
```

When reading from stdin, error messages identify the source as `<stdin>` and
file imports (`import "name.j";`) resolve relative to the current working
directory.

## Interactive REPL

For experimenting with the language, start an interactive session with
`jennifer repl`:

```text
$ ./jennifer repl
jennifer - Jennifer programming language interpreter
type :quit (or Ctrl-D) to exit; :help for help
>>> use io;
>>> def x as int init 21;
>>> $x + $x;
42
>>> io.printf("hi\n");
hi
>>> func dbl(n as int) {
...   return $n * 2;
... }
>>> dbl(7);
14
>>> :quit
```

A few notes:

- Statements still end with `;`. If a line ends with an unclosed `{` or `(`,
  the prompt switches to `... ` and waits for you to finish the block.
- A bare expression at the end of an input (like `$x + $x;`) prints its
  value. `null` results (including the return value of `printf`) are
  suppressed.
- String results are printed with surrounding double quotes so they're
  distinguishable from numbers (`"hello"`, not `hello`).
- Variables, constants, methods, and library imports persist for the whole
  session. Methods can be redefined freely as you iterate.
- File imports (`import "lib.j";`) work in the REPL and resolve relative to
  the directory you launched `jennifer repl` from.
- `:quit`, `:exit`, or Ctrl-D end the session; `:help` shows a reminder.

The prompt supports the standard line-editing keys you'd expect from
a modern shell:

| Key                    | Action                   |
| ---------------------- | ------------------------ |
| Left / Right           | Move cursor              |
| Home / End             | Jump to line start / end |
| Ctrl+A / Ctrl+E        | Same as Home / End       |
| Ctrl+Left / Ctrl+Right | Move by word             |
| Backspace, Delete      | Delete character         |
| Ctrl+W, Ctrl+Backspace | Delete word backward     |
| Ctrl+U / Ctrl+K        | Kill to line start / end |
| Up / Down              | Browse history           |
| Ctrl+C                 | Cancel the current line  |

History is in-memory only (no on-disk persistence yet) and holds up to
100 entries. When stdin is piped (e.g. `echo ... | jennifer repl` in
a test harness) the editor is bypassed and the REPL reads lines
normally, so non-interactive uses keep working.

## Inspection and formatting

Three commands help you see what Jennifer is doing under the hood and
keep your source in canonical shape:

```sh
# Print the lexer's token stream, one per line
./jennifer tokens examples/hello.j

# Print the parsed (and preprocessed) AST as JSON
./jennifer ast examples/hello.j

# Reformat the source to canonical style (see docs/user-guide/style-guide.md)
./jennifer fmt examples/hello.j
```

`fmt` writes the formatted source to stdout. To rewrite in place, use
your shell: `./jennifer fmt foo.j > foo.j.new && mv foo.j.new foo.j`.
The formatter is idempotent (`fmt` of `fmt` output equals `fmt` output)
and preserves runtime behavior - every example in this repo is checked
both ways by the test suite. Current v1 limitations: comments are
dropped, and blank lines aren't preserved or inserted automatically.
See [style-guide.md](style-guide.md) for the full style rules.

For local development you can also use the Go toolchain directly:

```sh
go run ./cmd/jennifer run examples/hello.j
go test ./...
```
