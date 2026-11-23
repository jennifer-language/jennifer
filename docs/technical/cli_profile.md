# Profiler (`cmd/jennifer/profile.go`, `internal/profile`)

`jennifer profile <prog.j>` runs the program with the evaluator instrumented and
attributes work back to Jennifer source positions (`file:line:col`) and the
functions holding them - the gap `go tool pprof` leaves, since it profiles the
*interpreter binary*, not the `.j` program inside it. The program's own output is
redirected to **stderr** so the profile owns **stdout** cleanly, even in the
binary form; with `--out FILE` the report goes to the file and the program keeps
its stdout:

```sh
jennifer profile prog.j                       # table to stdout, program output to stderr
jennifer profile --by=function prog.j         # one row per function
jennifer profile --top 20 --focus web prog.j  # the 20 hottest rows in web code
jennifer profile --allocs prog.j              # value-semantics (copy) profile
jennifer profile --format=pprof p.j > p.pb.gz # gzipped protobuf for go tool pprof / speedscope
jennifer profile --out prof.txt server.j      # report to a file; Ctrl-C writes it
```

## Statement profile

The default profile records, per source position, how many times a statement
ran and how long it took - **self** (this statement alone), **wait** (time it
spent blocked in a builtin that waits) and **cumulative** (this statement plus
everything it called), highest cumulative time first:

```text
$ jennifer profile --top 6 examples/profile.j
Jennifer statement profile: 1.34ms wall, 9 tasks (main + 8 spawned)
SELF excludes nested statements and waits; WAIT is time blocked in a builtin that
waits (time.sleep, channel.recv, task.wait, httpd.accept, net reads); SELF, WAIT
and CUM are summed over tasks, so with spawned tasks they can exceed the wall time.

  HITS       SELF  WAIT         CUM  FUNCTION     POSITION                 SOURCE
     1  101.232µs    0s  1.243507ms  <top level>  examples/profile.j:71:1  io.printf("eager=%d spawn=%d base=%d\n", eagerCopies(), s...
     1  205.271µs    0s   542.177µs  eagerCopies  examples/profile.j:29:5  while ($i < 200) {
     1  131.742µs    0s   329.594µs  aliasMutate  examples/profile.j:63:5  while ($i < 50) {
   200  202.484µs    0s   284.545µs  eagerCopies  examples/profile.j:31:9  $acc = $acc + sink($ys);
     1   38.181µs    0s   121.141µs  spawnCopies  examples/profile.j:50:5  for (def t in $handles) {
    50  117.061µs    0s   117.061µs  aliasMutate  examples/profile.j:64:9  def alias as list of int init $base;
  (showing 6 of 28 positions)

28 positions, 999 statement executions, 1.242817ms total self time, 43.81µs total wait (top-of-tree cum 1.243507ms)
```

The header gives the run's wall time and the number of tasks profiled (the main
one plus every `spawn`).

| Column     | Meaning                                                                          |
| ---------- | -------------------------------------------------------------------------------- |
| `HITS`     | How many times the statement at this position executed.                          |
| `SELF`     | Wall-clock spent *in this statement*, excluding nested statements it called and its `WAIT`. |
| `WAIT`     | Wall-clock this statement spent blocked in a builtin that waits (see below).     |
| `CUM`      | Cumulative wall-clock: this statement plus everything it called, waits included, counted once per stack (a recursive statement's nested activations add no `CUM` of their own). |
| `FUNCTION` | The function holding the statement, or `<top level>`.                            |
| `POSITION` | The `file:line:col` of the statement. Rows are sorted by `CUM` descending, then by `SELF`. |
| `SOURCE`   | The statement's source line, trimmed, cut at 60 characters.                      |

A path under the working directory is shown relative to it, a module from the
system module directory as `<sys>/web.j`, anything else in full.

A high `SELF` is a hot line; a high `CUM` with low `SELF` and `WAIT` is a line
that mostly waits on the work it dispatches. A high `WAIT` is a line that sat in
a blocking builtin: `time.sleep`, `channel.send` / `recv` / `recvTimeout` /
`select`, `task.wait` / `waitAll` / `waitAny` / `waitTimeout` /
`waitAnyTimeout`, `httpd.accept`, `net.accept` / `readBytes` / `readAll` /
`readN` / `recvFrom`, `serial.read` (the builtins that watch task cancellation
while they block, plus the two `accept`s). Other builtins count as self time,
including a `meta.callMain` waiting for the host lock and the network wait inside
a module such as `http` (which shows as the self time of the module's own lines).

Below the table the statement profile also reports the **maximum call depth** -
the deepest chain of nested function calls the run reached, overall and per deepest
call site (top 10):

```text
Max call depth (deepest chain of nested function calls): 5
  DEPTH  FUNCTION     CALL SITE
      5  eagerCopies  examples/profile.j:31:23
      1  <top level>  examples/profile.j:71:42
```

It shares the interpreter's call-depth counter with the catchable recursion guard
(`limits.MaxCallDepth`, build-tag split - 10000 on the default binary, 120 on
`jennifer-tiny`), so a program that trips a stack-limit / deep-recursion error can
be re-run under `profile` to see which call site drove the depth. The
section is silent for a program that made no function call.

### Per function (`--by=function`)

`--by=function` rolls the statement rows up into one row per function, the view
most questions start from:

```text
  CALLS       SELF     WAIT         CUM  FUNCTION     POSITION
      1    54.97µs       0s  1.379418ms  <top level>  examples/profile.j
      1  568.658µs       0s   568.658µs  aliasMutate  examples/profile.j:60:1
      1  438.886µs       0s   530.237µs  eagerCopies  examples/profile.j:25:1
      1  206.803µs  64.26µs   225.553µs  spawnCopies  examples/profile.j:39:1
    200   91.351µs       0s    91.351µs  sink         examples/profile.j:19:1
```

`CALLS` is how often the function ran (the hits of its first statement); `SELF`
and `WAIT` sum every statement inside it, a `spawn` body included; `CUM` sums
the function's own statements (a nested statement's time is already in the
statement holding it), so it is the time spent in calls to it. A file's
top-level statements make one `<top level>` row, positioned at the file.
`POSITION` is where the function is declared.

### Fewer rows (`--top`, `--focus`)

`--top N` keeps the first `N` rows of each table; `--focus RE` keeps the rows
whose path (as shown, or in full) or function matches the regular expression
`RE` (Go syntax). A cut table ends with `(showing N of M positions)`; the totals
below it still cover the whole run. Both apply to the `--allocs` tables too, and
only to the table format: `go tool pprof` has its own `-top` and `-focus`.

## Modes and formats

| Flag              | Effect                                                                                     |
| ----------------- | ------------------------------------------------------------------------------------------ |
| *(default)*       | Statement profile: hit counts + self / wait / cumulative time per position.                |
| `--allocs`        | Value-semantics profile instead: where compound values are copied or rebuilt (see below).  |
| `--format=table`  | Human-readable text (default).                                                             |
| `--format=pprof`  | Gzipped protobuf, hand-encoded to keep the zero-dependency stance; `go tool pprof` and speedscope.app read it. |
| `--format=trace`  | Chrome-trace JSON of the function-call timeline (open in `chrome://tracing` / Perfetto).     |
| `--out FILE`      | Write the report to `FILE` instead of stdout; the program keeps its stdout. Enables `SIGUSR1` snapshots. |
| `--by=function`   | One row per function instead of per statement (`--by=statement`, the default). Table only; not with `--allocs`. |
| `--top N`         | Keep the first `N` rows of each table. Table only.                                         |
| `--focus RE`      | Keep the rows whose path or function matches `RE`. Table only.                             |
| `--sysmoddir D`   | System module directory for bare imports (defaults to the built-in path).                  |
| `-I DIR`          | Add `DIR` to the module search path (repeatable), same as `run`.                           |
| `--vendor DIR`    | Vendor root for `@scope/package` deck imports.                                              |

Unknown `--format` or `--by`, the unsupported `--allocs --format=trace`
combination (allocation events have no timeline), and the table-only flags with
another format are rejected at argument parse, not deferred to output.

The pprof form names each location's function as the table does (`<top level>`
outside one), one location per statement, so `go tool pprof -top` lists functions
and `-lines` statements. Its sample types are `hits`, `self`, `wait` and `cum`,
with `self` the default: `go tool pprof -top` then ranks functions by the `SELF`
that `--by=function` shows. (`-sample_index=cum` sums the cumulative time of every
statement of a function, nested ones included, so per function it overstates;
read cumulative time per statement with `-lines`.) File names stay absolute so
`-list` finds the source.

## Programs that do not exit

`jennifer profile` writes its report when the program ends. A server never ends
on its own, so the profiler handles the signals that stop one (Unix):

- **SIGINT / SIGTERM** stop the program: it exits as if it ran `exit 130` /
  `exit 143` at its next loop checkpoint, so statements still running finish
  their records and deferred calls run; the report is written and the process
  exits with that code. A program that reaches no checkpoint within a second
  (its main task blocked in `httpd.accept`, say) gets the report written as it
  stands then, without the rows of the statements still running.
- A program that **catches the signal itself** (`os.catchSignal("int")`) keeps
  it: the profiler stays out of the way and writes the report at the program's
  own exit.
- **SIGUSR1** prints the interpreter's diagnostics dump as under `run` (see
  [`os`](../libraries/os.md)) and, with `--out FILE`, writes the profile so far
  to `FILE.1`, `FILE.2`, ... while the program keeps running, so a server can be
  profiled per load phase. Each snapshot holds everything the one before it held.
  The statements still running at a snapshot (the accept loop) are not in it.
  Without `--out` there is no snapshot, since stdout may be the program's.

```sh
jennifer profile --out prof.txt server.j &
# warm up, then:
kill -USR1 %1    # prof.txt.1: the warm-up
# run the load, then:
kill -USR1 %1    # prof.txt.2: warm-up and load
kill -INT %1     # prof.txt: the whole run
```

## Programs that use modules

`profile` enables `import` exactly as `run` does, so a program that reaches for a
shipped module (`markdown`, `http`, ...) profiles normally. The module's own
statements are instrumented too and attributed to their own file positions and
functions, so a hot line inside `<sys>/markdown.j` appears in the table beside the
caller's lines. The self / cumulative split is corrected at the module-call
boundary: a call into a module counts toward the caller's *cumulative* time but
not its *self* time (the module's own lines carry that), so nothing is
double-counted. (`test --coverage` reuses the profiler but leaves module
instrumentation off, so a coverage report still measures only the entry program.)

After an error that executed nothing (a failed import, a bad path), no profile is
written - the stderr error is the whole result, and in particular
`--format=pprof` does not leave a valid-but-empty gzip behind. A run that executed
some statements before erroring still emits its partial profile.

## Allocation profile (`--allocs`)

Because Jennifer is value-semantic, copies are where hidden cost hides.
`--allocs` reports three copy paths per source position:

```text
$ jennifer profile --allocs examples/profile.j
Jennifer allocation profile (value-semantics copies): 935µs wall, 9 tasks (main + 8 spawned)

Eager copies - a store that deep-copied a compound value (def, assignment, parameter,
literal element, field / index / append write, slice, throw, catch, defer argument):
  COUNT     BYTES  TYPE         FUNCTION     POSITION                 SOURCE
     50  39.8 KiB  list of int  aliasMutate  examples/profile.j:64:9  def alias as list of int init $base;
  50 copies across 1 sites, ~39.8 KiB copied (sorted by bytes; sizes are estimates)

Spawn-frame deep copies - a scope snapshot captured at spawn launch:
  COUNT     TOTAL  FUNCTION     POSITION                  SOURCE
      8  88.621µs  spawnCopies  examples/profile.j:44:22  $handles[] = spawn {

Module-boundary rebuilds - a value holding a module's own structs, rebuilt to cross a module call:
  (none)
```

The eager-copy table is **sorted by bytes**, not by count: a site that copies a
one-byte buffer thousands of times costs less than one that copies a large
struct a few hundred times, and the bytes column puts the second one first.
`BYTES` is the estimated total a site copied - one value cell per list element
and struct field, two per map entry, plus every `bytes` buffer, recursively;
strings are shared by a copy, so they count nothing. `TYPE` names what was
copied (a struct name, `list of T`, `map of K to V`, `bytes`), or `mixed` when
one site copied several types. The pprof form carries both measures as the
`alloc_objects` (count) and `alloc_space` (bytes) sample types, plus a `type`
label (`go tool pprof -sample_index=alloc_space`).

The module-boundary table has the same columns plus `DIR`. A call into an
imported module hands each argument to the module and the result back through
`retagStructs`, which rebuilds every struct, list and map holding a struct that
module declares, so each struct carries the identity its side of the boundary
expects. That is a copy in all but name: it is paid whether or not the callee
touches the value, and it grows with the value. `in` rows are arguments entering
the module, `out` rows are results and constants leaving it; both sit at the call
site (or the `alias.NAME` read), and `TYPE` is the caller's view of the value
(`m.Tree`, `list of m.Node`). A value holding no struct of the called module is
passed as is and never appears. In the pprof form these samples carry
`kind=boundary-in` / `boundary-out`.

| Copy path             | What it is                                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------------------------------- |
| **Eager copies**      | Every store that deep-copies a compound value (`Value.Copy()`), at the position where it happens: a `def` / assignment / parameter binding; an element of a list, map, struct or enum literal (`Cur{names: $c.names}` copies the list, reported at `$c.names`); a field, index, map or append write (`$p.xs = $xs`, `$ys[0] = $xs`, `$ys[] = $xs`); a list or `bytes` slice; a thrown value, the catch binding and a `defer` argument. Where the real allocation cost lives. A fresh list / map / struct literal or slice is already private, so storing it is **not** counted again (no redundant copy); neither is a borrowed read-only parameter / local, a moved variable or an owned call result, which skip the copy too. A parameter bound by a call from another module (or through `meta.call` / `meta.callMain`, or a func value called across a module boundary) is reported at the **parameter's declaration**, since that path has no call-site position. |
| **Spawn-frame copies**| The scope snapshot taken when a `spawn` launches (`snapshotForSpawn`, the environment copy), with its time.        |
| **Module-boundary rebuilds** | The struct-identity rebuild of an argument or result crossing into or out of an imported module (`retagStructs`): module method calls, func values called across a module boundary, and module constant reads. A `meta.callMain` re-entry into the host is not reported (that path has no call site). |

`examples/profile.j` exercises both - read it to see where value semantics turn
a store into real allocation.

## Reading a parallel profile

`spawn` bodies are profiled too (each on its own goroutine, onto the shared,
mutex-guarded collector with a per-goroutine self/cumulative accumulator). Things
to keep in mind:

- **Times are summed over tasks**, so total `SELF` and a statement's `CUM` can
  exceed the wall time in the header. Four workers each spending 5s at one
  position report ~20s of self time there though only ~5s of wall-clock passed -
  time-at-position summed over all tasks, like `pprof`'s CPU time exceeding wall
  time. The header's task count says how many tasks contributed.
- **Waiting is apart from working.** A worker blocked in `channel.recv`, a
  `task.waitAll` on in-flight workers, an accept loop or a `time.sleep` puts that
  time in `WAIT`, not `SELF`, so the hot lines are the ones that compute. A
  `spawn` body's statements belong to the function that holds the `spawn`.
- A statement that blocks in something not listed as waiting (a `meta.callMain`
  waiting for the host lock, a module's network read) still shows it as `SELF`.

## Instrumentation (implementation)

The interpreter carries an optional `Profiler` interface
(`internal/interpreter/profiling.go`) and three gate flags; `nil` means no
profiling, the only hot-path cost being a nil check. The concrete collector
lives in `internal/profile` and is injected only by this subcommand, so no
profiling machinery compiles into either binary's run path. Hook points:

- **`Run`** declares each function's position and its body's statement positions,
  and each top-level statement's (`DeclareFunc`), before the program starts. The
  collector maps a row to the function whose declaration is the last one at or
  before it in the same file.
- **`execStmt`** wraps `execStmtRaw`, timing each statement. An accumulator
  splits self from cumulative time (the standard nested-timing subtraction). It
  lives on the frame's **`CallChain`** (`env.depth.prof`, beside the call-depth
  counter), not on the shared `Interpreter` or a root `Environment`. The chain is
  goroutine-local and is threaded across every dispatch boundary (module call,
  func value, callback, `meta.call` / `meta.callMain`), so a module's statements
  and a host handler reached back through `meta.callMain` bubble their time into
  the calling statement exactly once, and parallel `spawn` bodies or concurrent
  `web` workers (whose handlers all re-enter the one host global) never mix
  accumulators. The chain also counts the activations of each statement in
  progress (`CallChain.active`), so a recursive statement records cumulative time
  only in its outermost activation. The collector's maps are mutex-guarded.
- **`waitCall`** times a call to one of the blocking builtins
  (`blockingBuiltins`) and adds it to `CallChain.wait`, which `execStmt` reports
  as the statement's wait and leaves out of its self time.
- **`evalCall`** times each function-call body for the trace timeline.
- **`eagerCopy`** records an eager deep copy at each value-storage site when the
  value is a compound; `recordCopy` does the same after a helper that copies on
  its own (`writeIndexedSlot`, `appendInto`, a slice).
- **`evalSpawn`** times the `snapshotForSpawn` deep copy and counts the task.
- **`loopCheckpoint`** turns a `RequestExit` (SIGINT / SIGTERM under `profile`)
  into an `exit` on the main task.

`evalExpr` is deliberately *not* timed: a `time.Now()` around every literal read
would swamp the profile with its own overhead.

**TinyGo.** Build-tag split like the linter: `profile.go`
(`!tinygo || tinygo_devtools`) is the only importer of `internal/profile`;
`devtools_tinygo.go` stubs the subcommand in the run-only `jennifer-tiny`
binary. `jennifer-tiny-dev` (the `tinygo_devtools` build) carries it, to profile
a TinyGo-only stack or performance issue on the constrained runtime itself.

Part of the [CLI reference](cli.md).
