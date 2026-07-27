# CLI (`cmd/jennifer`)

```
jennifer run [flags] <file.j> [args...]  run a Jennifer program
jennifer run -             read source from stdin
jennifer repl              interactive REPL
jennifer tokens <file.j>   dump the lexer's token stream
jennifer ast <file.j>      dump the preprocessed AST as JSON
jennifer fmt [-w] <file.j>...  format source per style-guide.md (-w rewrites in place)
jennifer lint <file.j>     report compile-legal but suspect patterns
jennifer profile <file.j>  profile hit counts and wall-clock per source position
jennifer test <file.j>     discover and run the file's test methods
jennifer version [-v]      print the build version (-v adds module-path layers)
jennifer help              show usage
```

## Subcommand reference

The `jennifer` command-line tool bundles the Jennifer interpreter and its
full development toolchain into a single binary. Beyond running `.j` programs,
it provides an interactive REPL, a source-code formatter, a linter, a
profiler, a test runner, and lexer-token and AST inspection - so a whole
Jennifer workflow (write, run, format, lint, profile, test) needs no extra
tools. Each subcommand is summarised below and documented in depth on its own
page. The development subcommands (`tokens`, `ast`, `fmt`, `lint`, `profile`,
`test`) and `serve` live in the default `jennifer` binary only; `run`,
`repl`, and `version` work on both `jennifer` and `jennifer-tiny`.

| Subcommand         | What it does                                             | Details                                                   |
| ------------------ | ------------------------------------------------------- | --------------------------------------------------------- |
| `run <file.j>`     | Run a Jennifer program (`-` reads source from stdin).   | [Module resolution flags](#module-resolution-flags-run)   |
| `repl`             | Interactive read-eval-print loop, with a line editor and history. | [REPL](cli_repl.md)                             |
| `tokens <file.j>`  | Dump the lexer's token stream.                          | [Inspection](cli_inspect.md)                              |
| `ast <file.j>`     | Dump the preprocessed AST as JSON.                      | [Inspection](cli_inspect.md)                              |
| `fmt [-w] <file.j>...` | Format source per the style guide (stdout; `-w`/`--write` rewrites in place). | [Formatter](cli_fmt.md)                    |
| `lint <file.j>`    | Report compile-legal but suspect patterns.              | [Linter](cli_lint.md)                                     |
| `profile <file.j>` | Per-position hit counts and wall-clock timings.         | [Profiler](cli_profile.md)                                |
| `test <file.j>`    | Discover and run the file's test methods.               | [Test runner](cli_test.md)                                |
| `serve <file.j>`  | Run a program; `--watch` re-runs it on change (a web-app reloader, or an autorun loop for any script). | [The `serve` command](#the-serve-command)                 |
| `version [-v]`     | Print the build version (`-v` adds module-path layers). | [Version injection](#version-injection)                   |
| `help`             | Show usage.                                             | -                                                         |

## Module resolution flags (`run`)

`jennifer run` accepts interpreter flags before the source file; anything
after the file is the program's own `os.ARGS`.

- `--sysmoddir DIR` (or `--sysmoddir=DIR`) - the system module directory
  for bare `import "name.j";`. Overrides `JENNIFER_SYSMODDIR`, which
  overrides the compile-time default. A named (CLI or env) dir that is
  missing or not a directory refuses to start; the compile-time default is
  best-effort. The resolved value is `meta.SYSMODDIR`.
- `-I DIR` (or `-I=DIR`, repeatable) - add a directory to the module
  search path after the system dir. A `-I` dir *adds* names; a module name
  appearing in two search dirs is a hard error at load. (Resolution lives
  in `internal/module`; the `import` statement consumes it via the loader
  wired in `main.go`'s `runFile` - `in.EnableModules(baseDir, searchDirs,
  loadModuleProgram, installLibraries)`, where `searchDirs` is the system
  dir followed by each `-I` dir.)
- `--vendor DIR` (or `--vendor=DIR`) - the vendor root for `@scope/package`
  deck imports. Overrides `JENNIFER_VENDOR`, which overrides the upward walk
  to the nearest `vendor/` directory above the program. `@scope/package/`
  expands to `<vendorRoot>/scope/package/package.j` (see the import spec);
  wired via `in.SetVendorRoot(module.FindVendorRoot(vendorFlag, baseDir))`.
  `repl` / `test` use the upward walk (no flag).

`jennifer version -v` reports every system directory the resolver uses - the
system module dir and the vendor root - each with the layers (compile default /
`JENNIFER_SYSMODDIR`, and env / `vendor/`-walk) behind it.

- Verifies the `.j` extension
- Reads the file, parses, runs
- On error: prints the message and a source-context caret on stderr, exits `1`
- Bad usage exits `2`
- `jennifer help` includes a `Version:` line so the build is identifiable at a glance
- `tokens`, `ast`, `fmt`, `lint`, `profile`, `test`, and `serve` are
  present only in the default `jennifer` binary (the run-only `jennifer-tiny`
  build stubs them); the first six are development subcommands with their own
  pages below, and `serve` runs a net-backed `web` app so it, too, is
  default-only.

## The `serve` command

`jennifer serve <file.j>` runs a program the same way `run` does - there is
no entry point, so the file's top-level executes in order and serving happens
only because the program itself calls `web.run(...)`. On its own, `serve` adds
just a banner (printed after a clean parse). Its reason to exist is `--watch`:

```sh
jennifer serve app.j            # run the app
jennifer serve app.j --watch    # re-run on every change to the entry file
```

With `--watch`, `serve` runs the program in a **child process** and restarts
it whenever the entry file changes: it polls the file's modification time and,
on a change, kills the child and starts a fresh one. Ctrl-C stops the loop.

Two uses fall out of the same mechanism:

- **A web-app reloader.** A long-running `web.run` server never exits on its
  own; save a handler and `--watch` restarts it against the new code - the
  Hugo-style edit / reload loop.
- **A general autorun loop.** For a program that *finishes* (any script, not
  just a server), the child exits, `serve` prints `app exited; waiting for a
  change to reload...`, and parks until the next edit - so
  `jennifer serve --watch script.j` is an edit-and-rerun harness for anything:
  save, see the output, save again. The watcher deliberately stays alive after
  a clean exit (a loop that quit the moment a server died on a syntax error
  would defeat the point).

Only the **entry file** is watched today - changes to `include`d files or
imported modules do not trigger a reload. `serve` is default-binary-only (the
`httpd` engine is net-backed, and `--watch` uses `os/exec`); see
[the `web` module](../modules/web.md).


## Shell pipelines and aliases

Because `jennifer run -` reads a program from stdin (the `run -` form
above), Jennifer drops into a shell pipeline like any other filter. One
caveat sets the shape: stdin can carry *either* the program *or* the data,
not both. So a one-liner alias pipes the program in and passes the data as
an argument (read back through `os.ARGS`); a reusable filter keeps the
program in a file and leaves stdin free for the data.

**Inline, program piped via `run -`.** A `json-pretty` that reformats a
single JSON argument. The program arrives on stdin, so the JSON is
`os.ARGS[1]` (`ARGS[0]` is the `-`):

```sh
alias json-pretty="printf '%s' 'use json; use os; use io; io.printf(\"%s\\n\", json.encodePretty(json.decode(os.ARGS[1])));' | jennifer run -"

json-pretty '{"b":2,"a":1}'
# {
#   "b": 2,
#   "a": 1
# }
```

Use `printf '%s'` rather than `echo` to pass the program verbatim, so the
`\n` reaches Jennifer as a two-character escape instead of being expanded
by the shell.

**Reusable filter, data on stdin.** For a true pipe (`... | json-pretty`,
`json-pretty < file.json`) keep the program in a file and let stdin carry
the data. Save this as, say, `~/.local/share/jennifer/json-pretty.j`:

```jennifer
use json;
use io;

def src as string init "";
while (not io.eof()) {
    $src = $src + io.readLine() + "\n";
}
io.printf("%s\n", json.encodePretty(json.decode($src)));
```

```sh
alias json-pretty='jennifer run ~/.local/share/jennifer/json-pretty.j'

echo '{"b":2,"a":1}' | json-pretty
curl -s https://api.example.com/thing | json-pretty
```

The same shape extends to any decode / re-encode pair: swap `json` for
`xml` once that library lands ([M20.2](../milestones.md#m20---system-libraries-compacted)) and the
file becomes a `pretty-xml`.


## Version injection

`internal/version.Version` holds the build version as a single `string`.
The default is `"dev"`; the `Makefile` runs `scripts/gen-version.sh` before
each build, which writes `internal/version/version_gen.go` containing a
small `init()` that overwrites `Version` with the output of
`scripts/version.sh` (a `git describe --tags --long` derivative; see
[../libraries/meta.md](../libraries/meta.md) for the format).

This codegen path replaces the more conventional `go build -ldflags
"-X .Version=..."` because TinyGo 0.41 silently ignores the `-X`
directive. Codegen works identically on both toolchains. The generated
file is `.gitignore`d so the repository never carries a stale copy.

Two consumers read `version.Version`:

- `cmd/jennifer/main.go` prints it in the `help` banner and as the body
  of the `version` subcommand.
- `internal/lib/meta/metalib.go` mirrors it into the interpreter as the
  `meta.VERSION` constant. The `meta` library is opt-in like every
  other library: `use meta; io.printf("%s\n", meta.VERSION);`.

`go test ./...` skips codegen and uses the default `"dev"`. The meta-lib
test only checks that the constant matches `version.Version`, not a
specific value, so it stays robust across builds.
