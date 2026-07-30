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
| `os.setEnv(name, value)` | null   | Sets an environment variable for this process (and any child it later spawns). An invalid name (empty, or containing `=` / NUL) is a positioned error. |
| `os.hasFlag(name)` | bool         | True if `name` is an exact-match element of `os.ARGS`. See "Flag inspection" below. |
| `os.flag(name)`    | string       | The element immediately after `name` in `os.ARGS`, or `""` if absent or at end.     |
| `os.run(argv[, stdin])` | `os.Result` | **Blocking.** Run `argv` to completion; capture stdout/stderr. An optional `stdin` (`string` or `bytes`) is fed to the child then closed. See "External programs" below. |
| `os.spawn(argv)`   | `os.Process` | **Non-blocking.** Start `argv`, return a handle.                                    |
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
whether it has arrived (clearing the flag). The model is **cooperative, never
preemptive**: a delivered signal only sets a flag; your program reacts at a point
of its own choosing, so no Jennifer code runs in a signal context and the
value-semantics guarantees hold. `name` is one of `"int"`, `"term"`, `"hup"`,
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
signal never caught is never pending, so a stray `os.gotSignal` is harmless. Note
the trade-off: a program that catches a signal and then never polls will not see
it (polling latency is real and explicit).

**`"usr1"` is reserved** for interpreter diagnostics and cannot be caught:
`kill -USR1 <pid>` makes the interpreter print a one-shot snapshot to stderr and
keep running - the answer to "my program is stuck in a loop, *where*?". Use
`"usr2"` for a program-defined signal. The snapshot is a fixed, labeled block
(delivered at the next loop / method-call boundary):

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

A method-name **call stack** and loop depth are a planned addition (they need
call-frame tracking in the interpreter). The dump rides the Unix `SIGUSR1`
wiring, so it is Unix-only - but it works on **both** binaries there, `jennifer`
and `jennifer-tiny` (subject to the scheduler note below); `kill -USR1` never
terminates either binary.

Signals are **Unix-only**, and work on both binaries on a Unix host (`SIGUSR1` /
`SIGUSR2` / `SIGHUP` do not exist on Windows, where `os.catchSignal` is a
positioned error rather than a crash). Aborting a script is unaffected: Ctrl-C on
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

def struct os.Process { pid as int };   # opaque handle for a spawned child
                                        # (a monotonic internal id, not the OS pid).
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
EOF). This drives a filter (`sort`, `jq`, `gzip`) or any program that reads
stdin, in one call:

```jennifer
def up as os.Result init os.run(["tr", "a-z", "A-Z"], "shout");
io.printf("%s", $up.stdout);                 # SHOUT

def sorted as os.Result init os.run(["sort"], "banana\napple\ncherry\n");
io.printf("%s", $sorted.stdout);             # apple / banana / cherry
```

It is **deadlock-free**: the whole input is buffered up front and the output is
drained as the child runs, so neither side blocks on a full pipe. The captured
output is still capped at 16 MiB per stream (a large input can produce a larger
output, which truncates with the usual marker). This is the "feed all input, read
all output" one-shot form; a persistent, interleaved stdin/stdout session on a
spawned process is a separate future capability.

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
returns the same `os.Result` immediately. If the child did not
terminate cleanly (an I/O failure draining its streams, not a non-zero
exit), `os.wait` raises a catchable error rather than reporting a false
exit code 0. `os.kill($p)` sends SIGTERM (falling back to a hard kill on
Windows, which has no SIGTERM); a subsequent `os.wait` returns whatever
the OS reports for the terminated child.

**Captured output is capped** at 16 MiB per stream for both `os.run`
and `os.spawn`: a child that writes more has the excess dropped and a
`\n[output truncated at 16 MiB]` marker appended, so a runaway child
can't grow the interpreter heap without bound.

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
| `os.NCPU`     | int    | Number of logical CPUs usable by the running process (`runtime.NumCPU`). Portable stdlib, so it stays OS-independent - it reports usable parallelism, not a raw core count: on `jennifer-tiny` (cooperative single-thread scheduler) it is `1`. For richer host details (CPU model, RAM), read the OS's own files from Jennifer, e.g. `/proc/cpuinfo` via `fs` on Linux, so the interpreter stays portable. |
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
