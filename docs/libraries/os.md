# `os` - operating-system glue

Enable with `use os;`. Every name lives behind the `os.` prefix
(`os.PLATFORM`, `os.getEnv`).

```jennifer
use io;
use os;

io.printf("platform:     %s\n", os.PLATFORM);
io.printf("architecture: %s\n", os.ARCH);
io.printf("dir sep:      %s\n", os.DIRSEP);
io.printf("home:         %s\n", os.getEnv("HOME"));
io.printf("args:         %d arguments\n", len(os.ARGS));
```

**Immutable per-run host facts are uppercase constants**;
**operations that take arguments are functions**.

Process exit is the language statement `exit EXPR;`, not an `os`
function - see
[rejected.md > `os.exit(n)`](../technical/rejected.md#osexitn).

## Functions

| Call               | Returns      | Notes                                                                               |
| ------------------ | ------------ | ----------------------------------------------------------------------------------- |
| `os.getEnv(name)`  | string       | Reads an environment variable. Unset variables return `""`, no error.               |
| `os.setEnv(name, value)` | null   | Sets an environment variable for this process (and any child it later spawns). An invalid name (empty, or containing `=` / NUL) is a positioned error. |
| `os.unsetEnv(name)` | null | Removes an environment variable from this process and any child it later spawns; a no-op when it is not set. Not the same as `setEnv(name, "")`, which leaves it set to the empty string (`GIT_DIR=` still changes what `git` does). Same name rule as `setEnv`. |
| `os.hasEnv(name)` | bool | Whether the variable is set, even to `""` - the case `getEnv` cannot tell from unset. Save `hasEnv` + `getEnv` before changing a variable, then restore with `setEnv` or `unsetEnv`. |
| `os.hasFlag(name)` | bool         | True if `name` is an exact-match element of `os.ARGS`. See "Flag inspection" below. |
| `os.flag(name)`    | string       | The element immediately after `name` in `os.ARGS`, or `""` if absent or at end.     |
| `os.run(argv[, stdin])` | `os.Result` | **Blocking.** Run `argv` to completion; capture stdout/stderr. An optional `stdin` (`string` or `bytes`) is fed to the child then closed. A non-zero exit is a value in `exitCode`; a program that cannot be started (not found, not executable) **throws** a catchable `Error{kind: "runtime"}`. See "External programs" below. |
| `os.spawn(argv[, options])` | `os.Process` | **Non-blocking.** Start `argv`, return a handle. `options` is a `map of string to string` with optional `"stdout"` / `"stderr"` keys. See "Spawned child stdio" below. |
| `os.wait(p)`       | `os.Result`  | Block until `$p` terminates; return captured streams + exit code. Idempotent.       |
| `os.poll(p)`       | bool         | Non-blocking: true once `$p` has exited (a following `os.wait` returns immediately). |
| `os.kill(p)`       | null         | Send SIGTERM to `$p`.                                                               |
| `os.release(p)`    | bool         | Drop a **finished** handle from the registry (frees its captured output); errors if `$p` is still running. Returns whether the handle existed. |
| `os.isTerminal(stream)` | bool    | Is `stream` (`"stdout"` / `"stderr"` / `"stdin"`) an interactive terminal? See "Terminal detection". |
| `os.cwd()`         | string       | Absolute path of the current working directory. Errors only if it can't be determined. |
| `os.homeDir()`     | string       | The current user's home directory (`$HOME` on Unix, `%USERPROFILE%` on Windows). Errors if unresolved. |
| `os.tempDir()`     | string       | Directory for temporary files (`$TMPDIR` or `/tmp` on Unix; the `%TMP%`/`%TEMP%` location on Windows). Never errors; the directory is not created. |
| `os.catchSignal(name)` | null     | Start trapping a Unix signal so the program can react to it (see "Signals"). |
| `os.gotSignal(name)` | bool      | Whether `name` has arrived since the last poll, clearing the flag. |

### Signals

`os.catchSignal(name)` opts into trapping a signal, and `os.gotSignal(name)` polls
whether it has arrived (clearing the flag). The model is **cooperative**: a
delivered signal only sets a flag and the program reacts where it polls, so no
Jennifer code runs in a signal context and value semantics hold. `name` is one of `"int"`, `"term"`, `"hup"`,
`"usr2"` (letters-only, lowercased, no `SIG` prefix). The main use is a
**graceful shutdown**:

```jennifer
use os;

os.catchSignal("term");                  # opt in (also "int" / "hup" / "usr2")
def running as bool init true;
while ($running) {
    serveOneRequest();
    if (os.gotSignal("term")) { $running = false; }   # poll at a safe point
}
shutdownCleanly();                       # close connections, remove temp files, ...
```

**Trapping is opt-in per signal**, so defaults survive: until you call
`os.catchSignal("int")`, Ctrl-C still terminates the program as usual (a script
that never traps SIGINT is stopped by Ctrl-C; one that traps it must poll). A
signal never caught is never pending, so a stray `os.gotSignal` is harmless. A
program that catches a signal and never polls will not see it.

**`"usr1"` is reserved** for interpreter diagnostics and cannot be caught:
`kill -USR1 <pid>` makes the interpreter print a one-shot snapshot to stderr and
keep running, showing where a stuck program is. Use
`"usr2"` for a program-defined signal. The snapshot is a fixed, labeled block
(delivered at the next loop / function-call boundary):

```
=== jennifer diagnostics (SIGUSR1) ===
time:       2026-01-02T15:04:05+01:00
executing:  path/to/program.j:42:5
tasks:      3 spawned, 2 live
goroutines: 6
memory:     heap 12.4 MiB (sys 68.0 MiB), 14 GCs
=======================================
```

- **time** - when the snapshot was taken (RFC 3339), to line it up with logs.
- **executing** - the `.j` source position at the current loop / call checkpoint.
- **tasks** - `spawn`ed tasks this run: total spawned and how many are still live.
- **goroutines** - the process goroutine count (tracks the spawn workers plus the
  interpreter).
- **memory** - runtime heap in use, memory obtained from the OS, and GC count -
  the "is it growing while stuck?" read.

A function-name **call stack** and loop depth are a planned addition (they need
call-frame tracking in the interpreter). The dump rides the Unix `SIGUSR1`
wiring, so it is Unix-only - but it works on **both** binaries there, `jennifer`
and `jennifer-tiny` (subject to the scheduler note below); `kill -USR1` never
terminates either binary.

Signals are **Unix-only** and work on both binaries on a Unix host (`SIGUSR1` /
`SIGUSR2` / `SIGHUP` do not exist on Windows, where `os.catchSignal` is a
positioned error). Aborting a script is unaffected: Ctrl-C on
Windows terminates through the OS, not through this library.

**`jennifer-tiny` scheduler note.** The TinyGo binary runs a **cooperative**
single-thread scheduler (`-scheduler=tasks`), so a delivered signal - both an
`os.gotSignal` flag and the `SIGUSR1` dump - is observed only when the program
reaches a scheduler yield point (a `time.sleep`, blocking I/O, a `spawn`
handoff). The trap itself is installed at the OS level, so a signal never
terminates the program; but a **pure CPU-bound loop** with no yield can defer the
observation indefinitely. Programs that block or sleep (servers, poll loops with
`time.sleep`) see signals normally. The default `jennifer` binary uses a
preemptive scheduler and has no such latency.

### Terminal detection

`os.isTerminal(stream)` reports whether a standard stream is an interactive
terminal, the usual gate for ANSI colour or a progress spinner. `stream` is `"stdout"`, `"stderr"`, or `"stdin"`; any
other string, or a non-string, is an error.

```jennifer
use os;
def coloured as bool init os.isTerminal("stdout");   # false when piped or redirected
```

It reports `true` for a terminal and `false` for a pipe, a file redirect, or
a redirect from a non-terminal character device such as `/dev/null` or
`/dev/zero`, so it also works as a "may I prompt?" gate. The default binary
uses a real terminal probe (`golang.org/x/term`); a stream that can't be
inspected reports `false`. On
`jennifer-tiny`, which excludes `x/term`, detection falls back to the
character-device mode bit - correct for a pty, but a `/dev/null` redirect reads
`true` there; use `io.eof()` to gate a read on the tiny build.

### Flag inspection

`os.hasFlag` and `os.flag` cover "did the user pass `--verbose`?" and
"what value follows `--port`?". They are **exact-match only**:

- `os.hasFlag("--port")` is true if `"--port"` appears as a standalone
  element of `os.ARGS`. It is **false** if `"--port=8080"` appears
  (different element value).
- `os.flag("--port")` returns the element immediately after `"--port"`
  if there is one, else `""`. The `--foo=bar` form is not parsed.

Real CLI parsing (combined short flags like `-rf`, `--foo=bar`,
repeated flags, subcommands) belongs to a future `cli` library; for
now, walk `os.ARGS` yourself.

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
execute other programs. The library registers two structs:

```jennifer
def struct os.Result {                  # not actually written by users -
    exitCode as int,                    # the library registers it under
    stdout   as string,                 # the `os.` prefix.
    stderr   as string
};

def struct os.Process {                 # a spawned child
    handle as int,                      #   the registry handle (pass it to
    pid    as int                       #   os.wait / poll / kill / release)
};                                      #   `pid` is the real OS pid (ps, /proc,
                                        #   logging); `handle` is a monotonic
                                        #   internal id, distinct from the pid.
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

**Feeding stdin.** An optional second argument - a `string` or `bytes` - is
written to the child's standard input, which is then closed (the child sees
EOF). This drives a filter (`sort`, `jq`, `gzip`) in one call:

```jennifer
def up as os.Result init os.run(["tr", "a-z", "A-Z"], "shout");
io.printf("%s", $up.stdout);                 # SHOUT

def sorted as os.Result init os.run(["sort"], "banana\napple\ncherry\n");
io.printf("%s", $sorted.stdout);             # apple / banana / cherry
```

It is **deadlock-free**: the whole input is buffered up front and the output is
drained as the child runs, so neither side blocks on a full pipe. The captured
output is still capped at 16 MiB per stream (a large input can produce a larger
output, which truncates with the usual marker). This is a one-shot form; a
persistent, interleaved stdin/stdout session on a spawned process is future
work.

`os.spawn(argv)` is the asynchronous form and returns an `os.Process`
handle immediately. Drain the streams with `os.wait`
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
returns the same `os.Result` immediately. If the child did not
terminate cleanly (an I/O failure draining its streams, not a non-zero
exit), `os.wait` raises a catchable error instead of reporting exit
code 0. `os.kill($p)` sends SIGTERM (falling back to a hard kill on
Windows, which has no SIGTERM); a subsequent `os.wait` returns whatever
the OS reports for the terminated child.

**Captured output is capped** at 16 MiB per stream for both `os.run`
and `os.spawn`: a child that writes more has the excess dropped and a
`\n[output truncated at 16 MiB]` marker appended, so a runaway child
can't grow the interpreter heap without bound.

**Spawned child stdio.** By default `os.spawn` **captures** both streams (so
`os.wait` returns them, like `os.run`). A child you never `os.wait` then
produces no visible output, and a child that fails at startup leaves its
diagnostics only in the capture buffer. The optional
`options` map redirects each stream independently; each value is one of:

| `"stdout"` / `"stderr"` value | Effect |
| ----------------------------- | ------ |
| `"capture"` (default)         | Buffered and returned on `os.wait`'s `os.Result`. |
| `"inherit"`                   | Connected to the parent's own stdout / stderr, so the child's output appears **live** (on the terminal, or wherever the parent's stream is redirected). `os.wait`'s field for that stream is then empty. |
| `"discard"`                   | Dropped. |
| any other string              | A **file path**; the stream is written to that file (created, truncated). |

```jennifer
# See a child's diagnostics as it runs (e.g. a browser that may fail at startup):
def p as os.Process init os.spawn(["chromium", "--headless", $url], {"stderr": "inherit"});

# Or send them to a log file without building a shell line:
def q as os.Process init os.spawn(["worker", "--job", $id], {"stderr": "/var/log/worker.err"});
```

**The handle vs the pid.** `os.wait` / `os.poll` / `os.kill` / `os.release` take
the whole `os.Process` and use its `handle` (a monotonic registry id). `$p.pid`
is the **real OS pid**, for logs, `ps`, `/proc`, or an out-of-band signal. The
OS recycles a pid once a child is reaped, so the registry keys on the stable
handle instead.

Because a handle (and its captured output) stays live for idempotent
`os.wait`, a long-running program that spawns many children should
`os.release($p)` each handle once it has read the result - it drops the
entry from the process registry so it does not grow without bound.
Releasing a still-running process is an error (`os.wait` or `os.kill`
it first); releasing an already-released handle returns `false`.

```jennifer
def r as os.Result init os.wait($p);
# ... use $r ...
os.release($p);   # free the handle in a per-job server loop
```

**No shell parsing.** `argv` is always a list; Jennifer never hands a
command string to a shell. For shell parsing, pass `["sh", "-c", $cmd]`
so the shell hop is visible at the call site. This avoids shell
injection by default.

**Non-zero exit codes are not errors.** A failed exit (`exit 1` from
the child) populates `$r.exitCode` with the value; the caller
branches on it. Only *boundary* failures - program not found, not
executable, permission denied, fork/exec failure - are positioned
runtime errors at the call site (`Error{kind: "runtime"}`, catchable). An
"is this tool installed?" probe wraps the call:

```jennifer
use os;
func hasGit() {
    try {
        return os.run(["git", "--version"]).exitCode == 0;
    } catch (e) {
        return false; # not on PATH, or not executable
    }
}
```

**Stream buffering.** Both stdout and stderr are buffered in memory.
A child that produces gigabytes of output will exhaust the
interpreter's memory; for streaming workloads, redirect to a file
via `["sh", "-c", "cmd > /tmp/out"]` or wait for a future streaming
variant.

**TinyGo limitation.** `jennifer-tiny` does not support `os.run`,
`os.spawn`, `os.wait`, `os.poll`, or `os.kill`: TinyGo's runtime does not
implement the underlying `os/exec` syscalls. On `jennifer-tiny` these
return a friendly runtime error pointing at the default `jennifer`
binary (`make build` produces both, `make build-go` only the default).
`net` uses the same friendly-error pattern.

## Constants

### Host facts

| Name          | Kind   | Value                                                                                     |
| ------------- | ------ | ----------------------------------------------------------------------------------------- |
| `os.PLATFORM` | string | Operating-system name as reported by the runtime: `"linux"`, `"darwin"`, `"windows"`, ... |
| `os.ARCH`     | string | CPU architecture: `"amd64"`, `"arm64"`, `"wasm"`, ...                                     |
| `os.NCPU`     | int    | Number of logical CPUs usable by the running process (`runtime.NumCPU`): usable parallelism, not a raw core count, so `1` on `jennifer-tiny` (cooperative single-thread scheduler). For CPU model, RAM etc., read the OS's own files (e.g. `/proc/cpuinfo` via `fs` on Linux). |
| `os.EOL`      | string | Platform line ending. `"\n"` on Unix-likes, `"\r\n"` on Windows.                          |
| `os.DIRSEP`   | string | Path-component separator: `"/"` on Unix-likes, `"\\"` on Windows.                         |
| `os.PATHSEP`  | string | PATH-list separator (between entries in `$PATH`): `":"` on Unix-likes, `";"` on Windows.  |

### Process

| Name      | Kind           | Value                                                                                              |
| --------- | -------------- | -------------------------------------------------------------------------------------------------- |
| `os.ARGS` | list of string | Command-line arguments passed to the running program. Index 0 is the script path, the rest follow. |

Interpreter identity constants (`VERSION`, `BUILD`) live in
[`meta`](meta.md).

See also: [meta.md](meta.md), [../user-guide/index.md](../user-guide/index.md), [../user-guide/imports.md](../user-guide/imports.md), [../user-guide/style-guide.md](../user-guide/style-guide.md), [index.md](index.md).
