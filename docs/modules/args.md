# `args` - command-line argument parser

Import with `import "args.j" as args;`. A declarative CLI parser at the common
surface of Python's `argparse`, plus the features an argparse user misses
immediately - typed flags (long + short), defaults, `required`, `choices`,
`count` / `append` actions, positionals with `nargs`, subcommands, and
`--version`. It is the structured layer over `os.ARGS` (`os.hasFlag` / `os.flag`
are primitive lookups). Pure Jennifer over `strings` + `convert` + `lists` +
`maps`, so it runs on **both binaries**.

```jennifer
use io;
use os;
import "args.j" as args;

def p as args.Parser init args.parser("greet", "Say hello");
$p = args.flag($p, "name", "n", "world", "who to greet");
$p = args.countFlag($p, "verbose", "v", "verbosity (repeatable)");
$p = args.positionalList($p, "extra", "extra names");

def r as args.Result init args.parse($p, os.ARGS);
if ($r.done) { io.printf("%s\n", $r.helpText); exit 0; }   # --help / --version
io.printf("hello %s (v=%d)\n", args.asString($r, "name"), args.count($r, "verbose"));
```

Runnable: [`examples/modules/args_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/args_demo.j)

## The parse model

A `Parser` is a **value-semantic** spec built with copy-returning builders (the
same pattern as `rest.Client`): each builder returns an updated `Parser`, so you
chain assignments (`$p = args.flag($p, ...)`). `args.parse($p, argv)` walks the
argument vector and returns a `Result`.

- `argv` is the full `os.ARGS`; its first element is the program name and is
  **skipped** (like `sys.argv[1:]` / `os.Args[1:]`).
- **Errors are catchable**, not a process `exit(2)`: an unknown flag, a missing
  required arg, a bad-type value, or a `choices` violation throws
  `Error{kind: "args"}`, so parsing composes with `try` / `catch`.
- **`-h` / `--help` (and `--version`) do not throw**: they set `$r.done = true`
  and put the text in `$r.helpText`. Print it and stop:
  `if ($r.done) { io.printf("%s\n", $r.helpText); exit 0; }`.

## Building a parser

| Builder                                   | Adds                                                              |
| ----------------------------------------- | ---------------------------------------------------------------- |
| `args.parser(prog, help)`                 | a new empty parser                                               |
| `args.flag(p, long, short, deflt, help)`  | a string flag (`--long` / `-short`) with a default               |
| `args.intFlag(p, long, short, deflt, help)`   | an int flag                                                  |
| `args.floatFlag(p, long, short, deflt, help)` | a float flag                                                |
| `args.boolFlag(p, long, short, help)`     | a boolean flag (presence -> true; `store_true`)                  |
| `args.countFlag(p, long, short, help)`    | a repeatable flag counting occurrences (`-vvv` -> 3)             |
| `args.listFlag(p, long, short, help)`     | a repeatable flag collecting values into a list (`append`)       |
| `args.positional(p, name, help)`          | one required positional                                          |
| `args.positionalOpt(p, name, deflt, help)`| an optional positional (`nargs "?"`)                             |
| `args.positionalList(p, name, help)`      | a variadic positional, 0+ (`nargs "*"`)                          |
| `args.positionalList1(p, name, help)`     | a variadic positional, 1+ (`nargs "+"`)                          |
| `args.positionalN(p, name, n, help)`      | a positional taking exactly `n` values (`nargs N`)               |
| `args.required(p)`                        | mark the **most-recently-added** argument required               |
| `args.choices(p, allowed)`                | constrain the most-recently-added argument to a set              |
| `args.command(p, name, help, sub)`        | add a subcommand with its own `Parser`                           |
| `args.version(p, ver)`                    | enable `--version`, printing `ver`                               |

`required` and `choices` are **post-modifiers** on the last-added argument, so
they read fluently right after the builder:

```jennifer
$p = args.choices(args.required(args.flag($p, "mode", "m", "", "build mode")), ["debug", "release"]);
```

## Reading the result

| Accessor                    | Returns          | For                                             |
| --------------------------- | ---------------- | ----------------------------------------------- |
| `args.asString(r, name)`    | `string`         | a string flag / positional (default if unset)   |
| `args.asInt(r, name)`       | `int`            | an int flag                                     |
| `args.asFloat(r, name)`     | `float`          | a float flag                                    |
| `args.asBool(r, name)`      | `bool`           | a boolean flag                                  |
| `args.asList(r, name)`      | `list of string` | an `append` flag or variadic positional         |
| `args.count(r, name)`       | `int`            | a `count` flag                                  |
| `args.has(r, name)`         | `bool`           | whether the arg was **supplied** (vs defaulted) |
| `args.usage(p)`             | `string`         | the generated help text (what `--help` prints)  |

The `Result` also exposes fields directly: `$r.command` (the chosen subcommand,
`""` if none), `$r.done`, and `$r.helpText`.

## Argument syntax accepted

- `--flag value`, `--flag=value`, and (for a `store_true` / `count` flag) a bare
  `--flag`.
- Short flags `-x value`, `-xvalue` (glued), and `-abc` **bundling** (each
  character a flag; the first value-taking flag consumes the rest of the cluster
  or the next token).
- `--` ends flag parsing: everything after is positional.
- A `-` followed by a digit (`-5`) is a **positional** (a negative number), not a
  flag.

## Subcommands

A subcommand is its own `Parser`. Global flags come **before** the subcommand
word; everything after it is parsed by the subcommand's parser. The chosen name
is read from `$r.command`, and the subcommand's values live in the same `Result`:

```jennifer
def add as args.Parser init args.positional(args.parser("add", "add a remote"), "url", "the URL");
def git as args.Parser init args.parser("git", "a tiny vcs");
$git = args.boolFlag($git, "quiet", "q", "suppress output");
$git = args.command($git, "add", "add a remote", $add);

def r as args.Result init args.parse($git, os.ARGS);      # e.g. git -q add http://x
# $r.command == "add"; args.asString($r, "url") == "http://x"; args.asBool($r, "quiet") == true
```

## Scope (and what's out)

This is **argparse Level B** - the ~95%. Out of scope (argparse Level C,
diminishing returns for a `.j` module): argument groups, mutually-exclusive sets,
parent parsers, prefix-abbreviation matching (`--verb` for `--verbose`), and
`@argfile`.

## See also

- [os.md](../libraries/os.md) - `os.ARGS` (the input) and the primitive
  `os.hasFlag` / `os.flag`.
- [modules/index.md](index.md) - the module catalog and import rules.
