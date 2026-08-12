# Profiler (`cmd/jennifer/profile.go`, `internal/profile`)

`jennifer profile <prog.j>` runs the program with the evaluator instrumented and
attributes work back to Jennifer source positions (`file:line:col`) - the gap
`go tool pprof` leaves, since it profiles the *interpreter binary*, not the `.j`
program inside it. The program's own output is redirected to **stderr** so the
profile owns **stdout** cleanly, even in the binary form:

```sh
jennifer profile prog.j                       # table to stdout, program output to stderr
jennifer profile --allocs prog.j              # value-semantics (copy) profile
jennifer profile --format=pprof p.j > p.pb.gz # gzipped protobuf for go tool pprof / speedscope
```

## Statement profile

The default profile records, per source position, how many times a statement
ran and how long it took - **self** (this statement alone) versus **cumulative**
(this statement plus everything it called), highest self-time first:

```text
$ jennifer profile examples/profile.j
Jennifer statement profile (wall-clock, self = excluding nested statements)

  HITS        SELF          CUM  POSITION
     1  2.208054ms  16.438373ms  examples/profile.j:76:1
     1   321.116µs  10.776339ms  examples/profile.j:36:5
   200  5.292611ms   5.553693ms  examples/profile.j:38:9
   200  4.755398ms   4.755398ms  examples/profile.j:37:9
     8  1.438971ms   1.438971ms  examples/profile.j:51:30
     ...
```

| Column     | Meaning                                                                          |
| ---------- | -------------------------------------------------------------------------------- |
| `HITS`     | How many times the statement at this position executed.                          |
| `SELF`     | Wall-clock spent *in this statement*, excluding nested statements it called.      |
| `CUM`      | Cumulative wall-clock: this statement plus everything it called.                 |
| `POSITION` | The `file:line:col` of the statement. Rows are sorted by `SELF` descending.       |

A high `SELF` is a hot line; a high `CUM` with low `SELF` is a line that mostly
waits on the work it dispatches.

Below the table the statement profile also reports the **maximum call depth** -
the deepest chain of nested method calls the run reached, overall and per deepest
call site (top 10):

```text
Max call depth (deepest chain of nested method calls): 12
  DEPTH  CALL SITE
     12  examples/profile.j:2:62
      1  examples/profile.j:3:19
```

It shares the interpreter's call-depth counter with the catchable recursion guard
(`limits.MaxCallDepth`, build-tag split - 10000 on the default binary, 24 on
`jennifer-tiny`), so a program that trips a stack-limit / deep-recursion error can
be re-run under `profile` to see exactly which call site drove the depth. The
section is silent for a program that made no method call.

## Modes and formats

| Flag              | Effect                                                                                     |
| ----------------- | ------------------------------------------------------------------------------------------ |
| *(default)*       | Statement profile: hit counts + self / cumulative time per position.                       |
| `--allocs`        | Value-semantics profile instead: where compound values are copied (see below).             |
| `--format=table`  | Human-readable text (default).                                                             |
| `--format=pprof`  | Gzipped protobuf, hand-encoded to keep the zero-dependency stance; `go tool pprof` and speedscope.app read it. |
| `--format=trace`  | Chrome-trace JSON of the method-call timeline (open in `chrome://tracing` / Perfetto).     |
| `--sysmoddir D`   | System module directory for bare imports (defaults to the built-in path).                  |
| `-I DIR`          | Add `DIR` to the module search path (repeatable), same as `run`.                           |
| `--vendor DIR`    | Vendor root for `@scope/package` deck imports.                                              |

Unknown `--format` and the unsupported `--allocs --format=trace` combination
(allocation events have no timeline) are rejected at argument parse, not deferred
to output.

## Programs that use modules

`profile` enables `import` exactly as `run` does, so a program that reaches for a
shipped module (`markdown`, `http`, ...) profiles normally. The module's own
statements are instrumented too and attributed to their own file positions, so a
hot line inside `markdown.j` appears in the table beside the caller's lines. The
self / cumulative split is corrected at the module-call boundary: a call into a
module counts toward the caller's *cumulative* time but not its *self* time (the
module's own lines carry that), so nothing is double-counted. (`test --coverage`
reuses the profiler but leaves module instrumentation off, so a coverage report
still measures only the entry program.)

After an error that executed nothing (a failed import, a bad path), no profile is
written to stdout - the stderr error is the whole result, and in particular
`--format=pprof` does not leave a valid-but-empty gzip behind. A run that executed
some statements before erroring still emits its partial profile.

## Allocation profile (`--allocs`)

Because Jennifer is value-semantic, copies are where hidden cost hides.
`--allocs` reports two copy paths per source position:

```text
$ jennifer profile --allocs examples/profile.j
Jennifer allocation profile (value-semantics copies)

Eager copies - a def / assignment / parameter binding that deep-copied a compound value:
  COUNT  POSITION
    200  examples/profile.j:38:28
    200  examples/profile.j:37:9
     50  examples/profile.j:69:9
     ...
  453 copies across 6 sites

Spawn-frame deep copies - a scope snapshot captured at spawn launch:
  (none)
```

| Copy path             | What it is                                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------------------------------- |
| **Eager copies**      | A `def` / assignment / parameter binding that deep-copies a compound value (`Value.Copy()`). Where the real allocation cost lives. A fresh list / map / struct literal RHS is already private, so binding it is **not** counted (no redundant copy). |
| **Spawn-frame copies**| The scope snapshot taken when a `spawn` launches (`snapshotForSpawn`).                                            |

`examples/profile.j` exercises both - read it to see where value semantics turn
a store into real allocation.

## Reading a parallel profile

`spawn` bodies are profiled too (each on its own goroutine, onto the shared,
mutex-guarded collector with a per-goroutine self/cumulative accumulator). Two
things to keep in mind:

- **Self time aggregates across goroutines**, so total self time can exceed
  wall-clock elapsed. Four workers each spending 5s at one position report ~20s
  of self time there though only ~5s of wall-clock passed - time-at-position
  summed over all goroutines, like `pprof`'s CPU time exceeding wall time.
- **Blocking counts as self time.** A statement that waits (`task.wait` /
  `task.waitAll` on in-flight workers, or `time.sleep`) attributes that
  wall-clock wait to itself, so a `waitAll` line can show large self time with a
  hit count of one. It is real elapsed time the statement occupied, not
  computation.

## Instrumentation (implementation)

The interpreter carries an optional `Profiler` interface
(`internal/interpreter/profiling.go`) and three gate flags; `nil` means no
profiling, the only hot-path cost being a nil check. The concrete collector
lives in `internal/profile` and is injected only by this subcommand, so no
profiling machinery compiles into either binary's run path. Hook points:

- **`execStmt`** wraps `execStmtRaw`, timing each statement. A `profChild`
  accumulator splits self from cumulative time (the standard nested-timing
  subtraction). It lives on each goroutine's **root `Environment`**
  (`env.root.profChild`), not the shared `Interpreter`, so parallel `spawn`
  bodies each accumulate into their own snapshot root instead of racing one
  field; the collector's maps are mutex-guarded for the same reason.
- **`evalCall`** times each method-call body for the trace timeline.
- **`eagerCopy`** records an eager deep copy at each value-storage site
  (`def` / assignment / parameter binding) when the value is a compound.
- **`evalSpawn`** times the `snapshotForSpawn` deep copy.

`evalExpr` is deliberately *not* timed: a `time.Now()` around every literal read
would swamp the profile with its own overhead.

**TinyGo.** Build-tag split like the linter: `profile.go` (`!tinygo`) is the only
importer of `internal/profile`; `devtools_tinygo.go` stubs the subcommand in the
run-only `jennifer-tiny` binary.

Part of the [CLI reference](cli.md).
