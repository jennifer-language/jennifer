# `os` - operating-system glue

Enable with `use os;`. Every name lives behind the `os.` prefix
(`os.PLATFORM`, `os.getEnv`). Nothing here is reachable as a bare
identifier.

```jennifer
use io;
use os;

io.printf("platform:     %s\n", os.PLATFORM);
io.printf("architecture: %s\n", os.ARCH);
io.printf("dir sep:      %s\n", os.DIRSEP);
io.printf("home:         %s\n", os.getEnv("HOME"));
io.printf("args:         %d arguments\n", len(os.ARGS));
```

The library splits cleanly: **immutable per-run host facts are
uppercase constants** (no arguments to take, no reason to be a
function); **operations that take arguments are functions**.

Process exit is the language statement `exit EXPR;`, not an `os`
function - see
[rejected.md > `os.exit(n)`](../technical/rejected.md#osexitn).

## Functions

| Call               | Returns      | Notes                                                                               |
| ------------------ | ------------ | ----------------------------------------------------------------------------------- |
| `os.getEnv(name)`  | string       | Reads an environment variable. Unset variables return `""`, no error.               |
| `os.hasFlag(name)` | bool         | True if `name` is an exact-match element of `os.ARGS`. See "Flag inspection" below. |
| `os.flag(name)`    | string       | The element immediately after `name` in `os.ARGS`, or `""` if absent or at end.     |
| `os.run(argv)`     | `os.Result`  | **Blocking.** Run `argv` to completion; capture stdout/stderr. See "External programs" below. |
| `os.spawn(argv)`   | `os.Process` | **Non-blocking.** Start `argv`, return a handle.                                    |
| `os.wait(p)`       | `os.Result`  | Block until `$p` terminates; return captured streams + exit code. Idempotent.       |
| `os.poll(p)`       | bool         | Non-blocking: true once `$p` has exited (a following `os.wait` returns immediately). |
| `os.kill(p)`       | null         | Send SIGTERM to `$p`.                                                               |
| `os.isTerminal(stream)` | bool    | Is `stream` (`"stdout"` / `"stderr"` / `"stdin"`) an interactive terminal? See "Terminal detection". |

### Terminal detection

`os.isTerminal(stream)` answers "is this standard stream an interactive
terminal?" - the usual gate for deciding whether to emit ANSI colour or a
progress spinner. `stream` is `"stdout"`, `"stderr"`, or `"stdin"`; any
other string, or a non-string, is an error.

```jennifer
use os;
def coloured as bool init os.isTerminal("stdout");   # false when piped or redirected
```

It reports `true` for a terminal and `false` for a pipe or a file
redirect. Detection uses the character-device mode bit (no external
dependency), so `/dev/null` - also a character device - reads `true`;
that is harmless, since escapes written there are discarded. A stream
that can't be inspected reports `false` (the conservative answer: when in
doubt, don't emit escapes). On `jennifer-tiny` the minimal runtime may
not introspect terminals, in which case it reports `false`.

### Flag inspection

`os.hasFlag` and `os.flag` are convenience helpers for the most common
"did the user pass `--verbose`?" and "what value follows `--port`?"
patterns. They are **exact-match only**:

- `os.hasFlag("--port")` is true if `"--port"` appears as a standalone
  element of `os.ARGS`. It is **false** if `"--port=8080"` appears
  (different element value).
- `os.flag("--port")` returns the element immediately after `"--port"`
  if there is one, else `""`. The `--foo=bar` form is not parsed.

This is deliberately minimal. Real CLI parsing (combined short flags
like `-rf`, `--foo=bar`, repeated flags, subcommands) belongs to a
future `cli` library; if you need any of it now, walk `os.ARGS`
yourself.

```jennifer
use io;
use os;

if (os.hasFlag("--help")) {
    io.printf("Usage: %s [options]\n", os.ARGS[0]);
    exit 0;
}
def port as string init os.flag("--port");
if ($port == "") {
    $port = "8080";
}
io.printf("listening on %s\n", $port);
```

### External programs

`os.run` and the `os.spawn` / `os.wait` / `os.poll` / `os.kill` quartet
let Jennifer programs execute other programs. Two struct types are
introduced for this:

```jennifer
def struct os.Result {                  # not actually written by users -
    exitCode as int,                    # the library registers it under
    stdout   as string,                 # the `os.` prefix.
    stderr   as string
};

def struct os.Process { pid as int };   # opaque handle for a spawned child.
```

`os.run(argv)` is the synchronous form. `argv` is a `list of string`
in the conventional argv shape - program name first, arguments
following. Stdout and stderr are captured into the returned
`os.Result`:

```jennifer
use io;
use os;

def r as os.Result init os.run(["echo", "hello, world"]);
io.printf("%s", $r.stdout);
io.printf("exit: %d\n", $r.exitCode);
```

`os.spawn(argv)` is the asynchronous form. It returns immediately
with an `os.Process` handle. Drain the streams with `os.wait`
(blocking) or check completion with `os.poll` (non-blocking):

```jennifer
def p as os.Process init os.spawn(["my-long-task", "--input", "data"]);
while (not os.poll($p)) {
    # do other work
}
def r as os.Result init os.wait($p);
io.printf("done: exit=%d\n", $r.exitCode);
```

`os.wait` is **idempotent** - calling it again on the same handle
returns the same `os.Result` immediately. `os.kill($p)` sends
SIGTERM; a subsequent `os.wait` returns whatever the OS reports for
the terminated child.

**No shell parsing.** `argv` is always a list - Jennifer never
concatenates a command string and hands it to a shell. If you
genuinely want shell parsing, pass `["sh", "-c", $cmd]` explicitly so
the shell hop is visible at the call site. This avoids the
shell-injection footguns that plague languages where the easy form is
the unsafe form.

**Non-zero exit codes are not errors.** A failed exit (`exit 1` from
the child) populates `$r.exitCode` with the value; the caller
branches on it. Only *boundary* failures - program not found, not
executable, permission denied, fork/exec failure - are positioned
runtime errors at the call site.

**Stream buffering.** Both stdout and stderr are buffered in memory.
A child that produces gigabytes of output will exhaust the
interpreter's memory; for streaming workloads, redirect to a file
via `["sh", "-c", "cmd > /tmp/out"]` or wait for a future streaming
variant.

**TinyGo limitation.** The `jennifer-tiny` binary (TinyGo build)
does not support `os.run`, `os.spawn`, `os.wait`, `os.poll`, or
`os.kill` - TinyGo's runtime hasn't implemented the underlying
`os/exec` syscalls. Calling these functions on `jennifer-tiny`
returns a friendly runtime error pointing at the default `jennifer`
binary. The default `jennifer` (`make build` produces both, or
`make build-go` for just it) supports the full exec surface. This
was the first user-visible gap in Jennifer's two-binary story;
`net` hit the same boundary and adopted the same
friendly-error pattern.

## Constants

### Host facts

| Name          | Kind   | Value                                                                                     |
| ------------- | ------ | ----------------------------------------------------------------------------------------- |
| `os.PLATFORM` | string | Operating-system name as reported by the runtime: `"linux"`, `"darwin"`, `"windows"`, ... |
| `os.ARCH`     | string | CPU architecture: `"amd64"`, `"arm64"`, `"wasm"`, ...                                     |
| `os.EOL`      | string | Platform line ending. `"\n"` on Unix-likes, `"\r\n"` on Windows.                          |
| `os.DIRSEP`   | string | Path-component separator: `"/"` on Unix-likes, `"\\"` on Windows.                         |
| `os.PATHSEP`  | string | PATH-list separator (between entries in `$PATH`): `":"` on Unix-likes, `";"` on Windows.  |

### Process

| Name      | Kind           | Value                                                                                              |
| --------- | -------------- | -------------------------------------------------------------------------------------------------- |
| `os.ARGS` | list of string | Command-line arguments passed to the running program. Index 0 is the script path, the rest follow. |

Interpreter-self-identity constants (`VERSION`, `BUILD`) live in
[`meta`](meta.md) since they describe the interpreter binary itself
rather than the host environment.

See also: [meta.md](meta.md), [../user-guide/index.md](../user-guide/index.md), [../user-guide/imports.md](../user-guide/imports.md), [../user-guide/style-guide.md](../user-guide/style-guide.md), [index.md](index.md).
