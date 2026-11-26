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
function; see [rejected.md > `os.exit(n)`](../technical/rejected.md#osexitn).

## Functions

| Call               | Returns      | Notes                                                                               |
| ------------------ | ------------ | ----------------------------------------------------------------------------------- |
| `os.getEnv(name)`  | string       | Reads an environment variable. Unset variables return `""`, no error. A value that is not valid UTF-8 throws `"os"`, naming the variable and the byte offset (`os.getEnv: LANG: invalid UTF-8 at byte 3; use os.getEnvBytes`). |
| `os.getEnvBytes(name)` | bytes    | The variable's raw value; unset is empty `bytes`, as for `getEnv` (`hasEnv` tells unset from empty). |
| `os.argsBytes()`   | list of bytes | The arguments of `os.ARGS` as raw `bytes`, for a program that must take any argument. |
| `os.setEnv(name, value)` | null   | Sets an environment variable for this process (and any child it later spawns). An invalid name (empty, or containing `=` / NUL) is a positioned error. |
| `os.unsetEnv(name)` | null | Removes an environment variable from this process and any child it later spawns; a no-op when it is not set. Not the same as `setEnv(name, "")`, which leaves it set to the empty string (`GIT_DIR=` still changes what `git` does). Same name rule as `setEnv`. |
| `os.hasEnv(name)` | bool | Whether the variable is set, even to `""` - the case `getEnv` cannot tell from unset. Save `hasEnv` + `getEnv` before changing a variable, then restore with `setEnv` or `unsetEnv`. |
| `os.run(argv[, stdin])` | `os.Result` | **Blocking.** Run `argv` to completion; capture stdout/stderr (must be valid UTF-8, else a catchable error; see "External programs"). An optional `stdin` (`string` or `bytes`) is fed to the child then closed. A non-zero exit is a value in `exitCode`; a program that cannot be started (not found, not executable) **throws** a catchable `Error{kind: "runtime"}`. See "External programs" below. |
| `os.runBytes(argv[, stdin])` | `os.BytesResult` | As `os.run`, with both streams returned as unchecked `bytes`. See "Binary output" below. |
| `os.spawn(argv)` | `os.Process` | **Non-blocking.** Start `argv`, return a handle; both streams captured. |
| `os.spawnWith(argv, os.SpawnOptions)` | `os.Process` | As `os.spawn`, with each stream routed by `SpawnOptions{stdout, stderr}`, and the working directory, environment and process group set by `dir`, `env`, `cleanEnv`, `group`. See "Spawned child stdio" and "Directory, environment and process group" below. |
| `os.wait(p)`       | `os.Result`  | Block until `$p` terminates; return captured streams + exit code. Idempotent. Non-UTF-8 output errors, like `os.run`. |
| `os.waitBytes(p)`  | `os.BytesResult` | As `os.wait`, with both streams as `bytes`. |
| `os.waitTimeout(p, ms)` | `os.Result` | As `os.wait`, waiting at most `ms` milliseconds; throws `"timeout"` and leaves `$p` running. `0` polls. |
| `os.waitBytesTimeout(p, ms)` | `os.BytesResult` | As `os.waitBytes`, bounded like `os.waitTimeout`. |
| `os.poll(p)`       | bool         | Non-blocking: true once `$p` has exited (a following `os.wait` returns immediately). |
| `os.kill(p)`       | null         | Send SIGTERM to `$p` alone, also when it leads a group: `os.signal($p, "term")` for a child spawned without `group`. |
| `os.signal(p, name)` | null       | Send signal `name` (`"term"`, `"kill"`, `"int"`, `"hup"`, `"quit"`, `"usr1"`, `"usr2"`) to `$p`, or to its whole process group when it was spawned with `group: true`. An exited child is not signalled (no error). See "Stopping a child and its children". |
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
Jennifer code runs in a signal context and value semantics hold. `name` is one
of `"int"`, `"term"`, `"hup"`, `"usr2"` (lowercase, no `SIG` prefix). The main
use is a **graceful shutdown**:

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

**Trapping is opt-in per signal**, so defaults survive: until the program calls
`os.catchSignal("int")`, Ctrl-C terminates it as usual; once it traps SIGINT it
must poll. A signal never caught is never pending, so a stray `os.gotSignal` is
harmless. A program that catches a signal and never polls will not see it.

**`"usr1"` is reserved** for interpreter diagnostics and cannot be caught:
`kill -USR1 <pid>` makes the interpreter print a one-shot snapshot to stderr and
keep running, showing where a stuck program is. Use `"usr2"` for a
program-defined signal. The snapshot is a fixed, labeled block (delivered at the
next loop / function-call boundary):

```text
=== jennifer diagnostics (SIGUSR1) ===
time:       2026-01-02T15:04:05+01:00
executing:  path/to/program.j:42:5
tasks:      3 spawned, 2 live
goroutines: 6
memory:     heap 12.4 MiB (sys 68.0 MiB), 14 GCs
=======================================
```

| Line | Content |
| --- | --- |
| `time` | When the snapshot was taken (RFC 3339), to line it up with logs |
| `executing` | The `.j` source position at the current loop / call checkpoint |
| `tasks` | `spawn`ed tasks this run: total spawned and how many are still live |
| `goroutines` | The process goroutine count (the spawn workers plus the interpreter) |
| `memory` | Runtime heap in use, memory obtained from the OS, and GC count: the "is it growing while stuck?" read |

The dump has no call stack. It is Unix-only, and works on **both** binaries
there (subject to the scheduler note below); `kill -USR1` never terminates
either.

Under `jennifer profile --out FILE`, `kill -USR1` also writes the profile so far
to `FILE.1`, `FILE.2`, ... and the program keeps running; SIGINT / SIGTERM there
write the report before the program stops, unless the program caught the signal
itself. See [the profiler](../technical/cli_profile.md#programs-that-do-not-exit).

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
terminal, the usual gate for ANSI colour or a progress spinner. `stream` is
`"stdout"`, `"stderr"`, or `"stdin"`; any other string, or a non-string, is an
error.

```jennifer
use os;
def coloured as bool init os.isTerminal("stdout");   # false when piped or redirected
```

It reports `true` for a terminal and `false` for a pipe, a file redirect, or
a redirect from a non-terminal character device such as `/dev/null` or
`/dev/zero`, so it also works as a "may I prompt?" gate. The default binary
uses a real terminal probe (`golang.org/x/term`); a stream that can't be
inspected reports `false`. On `jennifer-tiny`, which excludes `x/term`,
detection falls back to the
character-device mode bit - correct for a pty, but a `/dev/null` redirect reads
`true` there; use `io.eof()` to gate a read on the tiny build.

### Flag inspection

`os.hasFlag` and `os.flag` are deprecated (see [Deprecated](#deprecated)): parse
`os.ARGS` with the [`args`](../modules/args.md) module.

| Deprecated | With `args` |
| --- | --- |
| `os.hasFlag("--verbose")` | `$p = args.boolFlag($p, "verbose", "v", "...");`, then `args.asBool($r, "verbose")` |
| `os.flag("--port")` (`""` when absent) | `$p = args.flag($p, "port", "p", "8080", "...");`, then `args.asString($r, "port")` |

`args` also reads `--port=8080` (which `os.flag` returned as `""`), combined
short flags, repeated flags and subcommands, and throws `"args"` for an
unknown flag, where `os.hasFlag` ignored it.

```jennifer
use io;
use os;
import "args.j" as args;

def p as args.Parser init args.parser("serve", "Serve files");
$p = args.flag($p, "port", "p", "8080", "port to listen on");
def r as args.Result init args.parse($p, os.ARGS);
if ($r.done) {
    io.printf("%s\n", $r.helpText);
    exit 0;
}
io.printf("listening on %s\n", args.asString($r, "port"));
```

### External programs

`os.run` and the `os.spawn` / `os.wait` / `os.poll` / `os.kill` quartet
execute other programs; `os.runBytes` / `os.waitBytes` return the output as
`bytes`. The library registers three structs:

```text
def struct os.Result {                  # os.run / os.wait
    exitCode as int,
    stdout   as string,
    stderr   as string
};

def struct os.BytesResult {             # os.runBytes / os.waitBytes
    exitCode  as int,
    stdout    as bytes,
    stderr    as bytes,
    truncated as bool                   # the 16 MiB cap cut stdout or stderr
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
output, which truncates with the usual marker). This is a one-shot form; there
is no persistent, interleaved stdin/stdout session on a spawned process.

`os.spawn(argv)` is the asynchronous form and returns an `os.Process`
handle immediately. Drain the streams with `os.wait` (blocking) or check
completion with `os.poll` (non-blocking):

```jennifer
def p as os.Process init os.spawn(["my-long-task", "--input", "data"]);
while (not os.poll($p)) {
    # do other work
}
def r as os.Result init os.wait($p);
io.printf("done: exit=%d\n", $r.exitCode);
```

`os.waitTimeout($p, ms)` (and `os.waitBytesTimeout`) bounds the wait: when
`$p` has not exited within `ms` milliseconds it throws `"timeout"` and the
child keeps running, so the caller can `os.kill` it or wait again. `ms` `0`
polls; a negative `ms`, or one above 9,223,372,036,854, is a `"runtime"`
error. `task.cancel` wakes a task blocked in it with `"cancelled"`.

```jennifer
try {
    def r as os.Result init os.waitTimeout($p, 5000);
} catch (e) {
    if ($e.kind != "timeout") {
        throw $e;
    }
    os.kill($p);
    def killed as os.Result init os.wait($p);
}
```

`os.wait` is **idempotent** - calling it again on the same handle
returns the same `os.Result` immediately. If the child did not
terminate cleanly (an I/O failure draining its streams, not a non-zero
exit), `os.wait` raises a catchable error instead of reporting exit
code 0. `os.kill($p)` sends SIGTERM (falling back to a hard kill on
Windows, which has no SIGTERM); a subsequent `os.wait` returns whatever
the OS reports for the terminated child. `os.signal` sends other signals, and
to a whole process group (see "Stopping a child and its children").

**Captured output is capped** at 16 MiB per stream for both `os.run`
and `os.spawn`: a child that writes more has the excess dropped and a
`\n[output truncated at 16 MiB]` marker appended, so a runaway child
can't grow the interpreter heap without bound. A rune split by the cap is
dropped.

**Captured output must be UTF-8.** `os.Result.stdout` and `.stderr` are
strings, so `os.run` and `os.wait` raise a catchable error of kind `"os"` when a captured
stream is not valid UTF-8, naming the stream and the offset of the first bad
byte (`os.run: stdout: invalid UTF-8 at byte 2`).

**Binary output.** `os.runBytes(argv[, stdin])` and `os.waitBytes(p)` take the
same arguments as `os.run` / `os.wait` and return an `os.BytesResult`: both
streams as `bytes`, unchecked. The 16 MiB cap applies; instead of the text
marker, `truncated` is `true` when it cut either stream. The capture mode is
chosen when reading, so `os.wait` and `os.waitBytes` may read the same handle.

```jennifer
def png as os.BytesResult init os.runBytes(["convert", "in.svg", "png:-"]);
if ($png.exitCode == 0 and not $png.truncated) {
    fs.writeBytes("out.png", $png.stdout);
}
```

**Spawned child stdio.** By default `os.spawn` **captures** both streams (so
`os.wait` returns them, like `os.run`). A child never `os.wait`ed then
produces no visible output, and a child that fails at startup leaves its
diagnostics only in the capture buffer. `os.spawnWith(argv,
os.SpawnOptions{stdout, stderr})` routes each stream independently; each field
is one of:

| `stdout` / `stderr` value     | Effect |
| ----------------------------- | ------ |
| `""` (the zero value) or `"capture"` | Buffered and returned by `os.wait` (`os.Result`) or `os.waitBytes` (`os.BytesResult`). |
| `"inherit"`                   | Connected to the parent's own stdout / stderr, so the child's output appears **live** (on the terminal, or wherever the parent's stream is redirected). `os.wait`'s field for that stream is then empty. |
| `"discard"`                   | Dropped. |
| a path containing `/`         | A **file**; the stream is written to it (created, truncated). Write `./out.log` for a file in the working directory. If `"stdout"` and `"stderr"` name the same file, both streams share one descriptor and interleave in write order. |

Any other value is an error, so a typo such as `"inhert"` does not create a
file. `os.spawnWith(argv, os.SpawnOptions{})` is `os.spawn(argv)`.

```jennifer
# See a child's diagnostics as it runs (e.g. a browser that may fail at startup):
def p as os.Process init os.spawnWith(["chromium", "--headless", $url],
    os.SpawnOptions{stderr: "inherit"});

# Or send them to a log file without building a shell line:
def q as os.Process init os.spawnWith(["worker", "--job", $id],
    os.SpawnOptions{stderr: "/var/log/worker.err"});
```

**Directory, environment and process group.** Four more `os.SpawnOptions`
fields shape the child; each zero value keeps `os.spawn`'s behaviour, so a
literal names only those it sets:

```text
def struct os.SpawnOptions {
    stdout   as string,                  # see the table above
    stderr   as string,
    dir      as string,                  # working directory; "" = the parent's
    env      as map of string to string, # set on top of the inherited environment
    cleanEnv as bool,                    # true: start from an empty environment
    group    as bool                     # true: lead a new process group (Unix)
};
```

| Field | Effect |
| --- | --- |
| `dir` | The child's working directory. It must exist and be a directory, else `os.spawnWith` throws `"runtime"` before anything starts (no redirect file is created). A relative program path in `argv[0]` (`./tool`) is resolved against `dir`; a redirect file path against the parent's directory. |
| `env` | Variables set for the child, replacing an inherited variable of the same name. A name must be non-empty with no `=` or NUL, a value no NUL (else `"runtime"`). |
| `cleanEnv` | `true` drops the inherited environment: the child sees only `env`. Keeps the parent's secrets (tokens, keys in variables) out of a child. `PATH` is then unset in the child, but `argv[0]` is still looked up on the parent's `PATH`. |
| `group` | `true` starts the child as the leader of a new process group (`setpgid`), whose id is `$p.pid`. `os.signal` then reaches every process in it, e.g. the jobs a shell started in the background. Unix only: on other platforms `group: true` throws `"runtime"`. |

```jennifer
def p as os.Process init os.spawnWith(
    ["sh", "-c", $script],
    os.SpawnOptions{
        dir: $workdir,
        cleanEnv: true,
        env: {"PATH": "/usr/bin:/bin", "LANG": "C.UTF-8"},
        group: true
    });
```

**Stopping a child and its children.** `os.signal($p, name)` sends `"term"`,
`"kill"`, `"int"`, `"hup"`, `"quit"`, `"usr1"` or `"usr2"` (the lowercase,
`SIG`-less names of `os.catchSignal`). For a child spawned with `group: true` it
signals the whole group (`kill(-pgid)`), otherwise the child alone. A child that
has exited is not signalled and that is no error. A group is signalled even
after its leader exited, since the remaining members keep the group alive; once
none is left the call does nothing. Caveat: when every member has gone, the OS
may in time reuse the group id for an unrelated group, which a late
`os.signal` would then reach; signal a group while it can still have members,
not long after the run. A process that moved itself to another group (`setsid`,
a daemon) is out of reach. The usual bounded run:

```jennifer
try {
    def r as os.Result init os.waitTimeout($p, 5000);
} catch (e) {
    if ($e.kind != "timeout") {
        throw $e;
    }
    os.signal($p, "term");                  # ask the whole group to stop
    try {
        os.waitTimeout($p, 1000);
    } catch (e2) {
        if ($e2.kind != "timeout") {
            throw $e2;
        }
        os.signal($p, "kill");              # then force it
    }
    os.wait($p);
}
```

On Windows only `"term"` and `"kill"` exist, both a hard stop of the child
(as `os.kill` there); the other names throw `"runtime"`.

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
injection.

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

**Stream buffering.** A captured stream is held in memory up to the
16 MiB cap. For more output, redirect it to a file:
`os.spawnWith(argv, os.SpawnOptions{stdout: "/tmp/out"})`.

**TinyGo limitation.** `jennifer-tiny` does not support `os.run`,
`os.runBytes`, `os.spawn`, `os.spawnWith`, `os.wait`, `os.waitBytes`, their
timeout forms, `os.poll`, `os.kill`, or `os.signal`:
TinyGo's runtime does not implement the underlying `os/exec` syscalls. On
`jennifer-tiny` these raise a runtime error pointing at the default `jennifer`
binary (`make build` produces both, `make build-go` only the default). `net`
follows the same pattern.

### Errors

| Kind | Raised for |
|---|---|
| `"os"` | text that is not valid UTF-8: a child's captured stdout or stderr in `os.run` / `os.wait`, an environment variable in `os.getEnv`, an argument when `os.ARGS` is read (and the value `os.flag` returns) |
| `"limit"` | too many live child processes from `os.spawn` / `spawnWith` |
| `"timeout"` | `os.waitTimeout` / `waitBytesTimeout`: the process did not exit within `ms` (it keeps running) |
| `"cancelled"` | `task.cancel` of a task blocked in `os.waitTimeout` / `waitBytesTimeout` |
| `"closed"` | a released process handle (or one never spawned) passed to `os.wait`, `waitTimeout`, `poll`, `kill` or `signal`; `os.release` of it returns `false` |
| `"runtime"` | a program that cannot be started, any other operating-system failure, and misuse: a wrong argument, an unknown option, signal or stream name, a `dir` that is missing or not a directory, a bad `env` name or value, `group` or a signal not available on the platform, `ms` negative or above 9,223,372,036,854 |

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

When an argument is not valid UTF-8, every read of `os.ARGS` throws `"os"`,
naming the argument index and the byte offset
(`os.ARGS: argument 2: invalid UTF-8 at byte 1; use os.argsBytes()`); a
program that never reads it is unaffected. `os.argsBytes()` returns the raw
arguments.

Interpreter identity constants (`VERSION`, `BUILD`) live in
[`meta`](meta.md).

## Deprecated

Each still works and is removed in 0.29.0; `jennifer lint` reports a call
(L301).

| Deprecated | Use instead |
| --- | --- |
| `os.hasFlag(name)` | the [`args`](../modules/args.md) module: `args.boolFlag`, then `args.asBool` (see [Flag inspection](#flag-inspection)) |
| `os.flag(name)` | the `args` module: `args.flag`, then `args.asString` |
| `os.spawn(argv, options)` (the options map) | `os.spawnWith(argv, os.SpawnOptions{stdout: ..., stderr: ...})`; the map's `"stdout"` / `"stderr"` values are the fields' |

See also: [meta.md](meta.md), [../user-guide/index.md](../user-guide/index.md), [../user-guide/imports.md](../user-guide/imports.md), [../user-guide/style-guide.md](../user-guide/style-guide.md), [index.md](index.md).
