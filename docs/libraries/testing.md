# `testing` - assertions and test-runner primitives

Enable with `use testing;`. An assertion vocabulary plus the
system-side runner surface: name-based method invocation, a
per-process result accumulator, and a format dispatcher for
human / TAP / JUnit output. The `jennifer test` subcommand
(discovery, `setUp` / `tearDown`, `--format`, `--isolated`)
orchestrates on top; see
[technical/cli_test.md](../technical/cli_test.md).

```jennifer
use io;
use testing;

func addPasses() {
    testing.assertEqual(1 + 1, 2);
}

testing.run("addPasses");
io.printf("%s", testing.report(testing.results(), "text"));
```

Under `jennifer test`, the `testing.run` / `report` boilerplate is
handled for you; a test file just defines `test*` methods with
assertions in the body.

## Quick start

The everyday path is the `jennifer test` subcommand: write `func test*`
methods with assertions, and the runner discovers and runs them - no
`testing.run` / `report` boilerplate.

```jennifer
use testing;

func testMath() {
    testing.assertEqual(2 + 3, 5);
    testing.assertTrue(len("abc") == 3);
}
```

```text
$ jennifer test math_test.j
PASS testMath (0 ms)

1 passed, 0 failed, 1 total
```

`setUp` / `tearDown` methods (run before / after each test) and the flags
(`--filter`, `--format=text|tap|junit`, `--isolated`) are documented in
[technical/cli_test.md](../technical/cli_test.md). The rest of this page
documents the `testing` **primitives** the runner is built from - reach for
them directly only when building your own harness.

## Why this is a system library

A pure `.j` test runner would have to take *the test body* as
a value. Jennifer has no function references / first-class
methods - you can't say `testing.run(myTest)` because
`myTest` is a name, not a value. The interpreter already does
name-based method lookup at every call site; `testing.run`
exposes that as a builtin so a Jennifer-coded module can
dispatch user methods by name without inventing its own
indirection.

`testing.run` is also the one place in the language where
`exit` is intercepted. Language-level `try`/`catch`
deliberately does **not** catch `exit` (see
[control-flow.md](../user-guide/control-flow.md#try-catch-throw));
the testing runner catches it at the Go level so a runaway
`exit` in a test body stays scoped to the runner. This
carve-out is why the primitive can't live in `.j`.

## Surface

| Call                                        | Returns                  | Notes                                                                             |
| ------------------------------------------- | ------------------------ | --------------------------------------------------------------------------------- |
| `testing.run(name)`                         | `testing.Result`         | Look up a zero-arg method by name, call it, catch every failure mode, append.     |
| `testing.runWith(name, args)`               | `testing.Result`         | Like `run`, but binds the `args` list to the method's parameters (arity + declared-type checked). For framework dispatchers, not the zero-arg tests the runner discovers. |
| `testing.results()`                         | `list of testing.Result` | Snapshot of the accumulator. Value semantics - safe to modify.                    |
| `testing.reset()`                           | `null`                   | Clear the accumulator between independent runs.                                   |
| `testing.report(results, format)`           | `string`                 | Render `results` to `"text"`, `"tap"`, or `"junit"`.                              |

## Assertions

Six builtins for test bodies. Each reduces to a `Value.Equal` / Kind
comparison in Go - native speed, no per-call interpreter overhead - and,
on failure, throws `Error{kind: "assertion"}` positioned at the
assertion call, which `testing.run` catches and records.

| Call                                       | Fails (throws) when                                                                     |
| ------------------------------------------ | --------------------------------------------------------------------------------------- |
| `testing.assertEqual(actual, expected)`    | `actual != expected` (deep structural equality: lists / maps / structs compare by value). |
| `testing.assertNotEqual(actual, expected)` | `actual == expected`.                                                                   |
| `testing.assertTrue(cond)`                 | `cond` is `false` (`cond` must be `bool`).                                               |
| `testing.assertFalse(cond)`                | `cond` is `true` (`cond` must be `bool`).                                                |
| `testing.assertContains(haystack, needle)` | `needle` is absent: substring for a string, element for a list, key for a map (by haystack kind). |
| `testing.assertThrows(name, kind)`         | the named zero-arg method doesn't throw, or throws an `Error` whose `kind` differs.      |
| `testing.assertThrowsWith(name, args, kind)` | as `assertThrows`, but binds the list `args` to the method's parameters - no per-case zero-arg wrapper method needed. |

```jennifer
use testing;

func add(a as int, b as int) { return $a + $b; }

func testAdd() {
    testing.assertEqual(add(2, 3), 5);
    testing.assertContains([1, 2, 3], 2);
    testing.assertThrows("mustFail", "boom");
    testing.assertThrowsWith("parse", ["1 ~ 2"], "syntax");
}
```

`assertThrowsWith` is the argument-passing form: where `assertThrows` needs a
zero-arg wrapper method per failing input, `assertThrowsWith("parse", [input],
kind)` calls `parse(input)` directly, so one helper covers every case.

### Table-driven tests

There is no `testing.subtest` primitive; drive a set of cases with a plain loop
inside one test method - the idiomatic Jennifer shape:

```jennifer
use testing;

def struct Case { input as int, want as int };

func testDoubles() {
    def cases as list of Case init [
        Case{input: 0, want: 0},
        Case{input: 3, want: 6},
        Case{input: -2, want: -4}];
    for (def c in $cases) {
        testing.assertEqual($c.input * 2, $c.want);
    }
}
```

An assertion `throw`s, so the **first** failing case stops that test method
(later iterations don't run) and the reported position points at the
`assertEqual` line. Put a distinguishing value in the case (or a per-case
`assertContains` message) when you need to tell which row failed.

## The `testing.Result` struct

```jennifer
def struct testing.Result {
    name as string,
    ms as int,               # elapsed wall time in milliseconds
    passed as bool,
    errorKind as string,     # "" on pass; "runtime" / "error" / "exit" / "unknown" on fail
    errorMessage as string,
    file as string,          # position where the failure originated (if known)
    line as int,
    col as int
};
```

`errorKind` mirrors the strings surfaced by
[`try`/`catch`](../user-guide/control-flow.md#try-catch-throw)
plus a new `"exit"` value for the exit-intercept case:

| Value       | Meaning                                                                         |
| ----------- | ------------------------------------------------------------------------------- |
| `""`        | The test passed.                                                                |
| `"runtime"` | An interpreter runtime error (out-of-bounds, missing key, type mismatch, ...).  |
| `"error"`   | A `throw` whose thrown value wasn't an `Error` struct.                          |
| `"assertion"` etc. | A `throw Error{kind: "assertion", ...}` - `errorKind` mirrors the struct's `kind` field. |
| `"exit"`    | The test body called `exit`. `errorMessage` is `"exit code N"`.                 |
| `"unknown"` | Anything else (method not found, wrong parameter count, ...).                   |

## How `testing.run` handles each failure mode

```jennifer
# Pass path
func passing() {
    return;                     # any normal return counts as a pass
}

# Failure via user throw
func failing() {
    throw Error{
        kind: "assertion",
        message: "expected 42, got 41",
        file: "", line: 0, col: 0
    };
}

# Failure via runtime error
func indexing() {
    def xs as list of int init [];
    def x as int init $xs[5];   # out-of-bounds; kind=runtime
}

# Exit inside a test - captured, doesn't propagate
func earlyExit() {
    exit 1;                     # kind=exit; program keeps running
}
```

Every call to `testing.run` appends exactly one `testing.Result`
to the accumulator. The result is also returned, so the caller
can inspect it immediately.

## Reports

`testing.report(results, format)` takes any list of
`testing.Result` and returns a rendered string. Three formats
ship in v1; format strings are case-sensitive to match the
codec-table shape used by `hash.compute`, `encoding.toText`,
`fs.open`.

### `"text"` - human-readable

```
PASS addPasses (0 ms)
FAIL addFails (1 ms)
     [assertion] 1 + 1 != 2
FAIL earlyExit (0 ms)
     [exit] exit code 1

1 passed, 2 failed, 3 total
```

### `"tap"` - Test Anything Protocol v14

```
TAP version 14
1..3
ok 1 - addPasses
not ok 2 - addFails
  ---
  kind: assertion
  message: 1 + 1 != 2
  ...
not ok 3 - earlyExit
  ---
  kind: exit
  message: exit code 1
  ...
```

Works with the `prove` command and most CI harnesses.

### `"junit"` - JUnit XML

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="jennifer" tests="3" failures="2">
  <testcase name="addPasses" time="0.000"></testcase>
  <testcase name="addFails" time="0.001">
    <failure type="assertion" message="1 + 1 != 2">1 + 1 != 2</failure>
  </testcase>
  <testcase name="earlyExit" time="0.000">
    <failure type="exit" message="exit code 1">exit code 1</failure>
  </testcase>
</testsuite>
```

The ubiquitous CI input format.

Unknown format strings error at the boundary:
`testing.report: unknown format "html"; known: "text", "tap",
"junit"`.

## Errors

- **Wrong argument count.** Boundary error:
  `testing.run expects 1 argument (name), got 0`.
- **Wrong argument type.** Boundary error:
  `testing.run: name must be string, got int`.
- **Method with parameters.** `testing.run` in v1 only invokes
  zero-argument methods. Calling it with a method that takes
  parameters records a failing Result with `errorKind="unknown"`
  and the reason in `errorMessage`.
- **Unknown format.** `testing.report: unknown format "html";
  known: "text", "tap", "junit"`.

## Concurrency

The accumulator is guarded by a mutex, so `spawn { testing.run(...) }`
from multiple tasks doesn't race. Ordering is by completion time,
not spawn time. A test runner that wants stable ordering should
run tests sequentially or sort the results before rendering.

## Running with `jennifer test`

These builtins are the substrate; the `jennifer test` subcommand is
the orchestration layer on top. It discovers `test*` methods (or
`--filter=REGEX`), runs `setUp` / `tearDown` around each, renders
`--format=text|tap|junit`, and with `--isolated` runs each test in a
fresh interpreter subprocess so one test's crash, `exit`, or leaked
global state can't affect the others. Its flags and exit codes are
documented in
[technical/cli_test.md > Test runner](../technical/cli_test.md#test-runner-cmdjennifertestgo-internallibtesting).
`testing.runWith` (and `Interpreter.CallByNameWith` beneath it) supplies
the arg-taking dispatch that parameterised drivers use.

Still deferred:

- **Per-test timeouts.** A non-terminating test still hangs its
  subprocess; `--isolated` isolates state, not runtime.
- **Skip / xfail.** Runner-level policy, not a primitive.
- **First-class subtests.** A body loop
  (`for (def c in $cases) { testing.assertEqual(...); }`) covers the
  observed table-driven cases; a `testing.subtest(name)` primitive would
  need new language surface.

## See also

- [technical/cli_test.md > Test runner](../technical/cli_test.md#test-runner-cmdjennifertestgo-internallibtesting)
  - the `jennifer test` subcommand: discovery, `--filter`,
  `--format`, `--isolated`, and the enabling interpreter change.
- [milestones.md](../milestones.md) - design spec and the
  follow-on.
- [control-flow.md](../user-guide/control-flow.md#try-catch-throw)
  - the `try`/`catch` machinery `testing.run` builds on.
- [concurrency.md](../user-guide/concurrency.md) - `spawn` under
  the mutex-guarded accumulator.
