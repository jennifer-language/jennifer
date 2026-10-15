# Test runner (`cmd/jennifer/test.go`, `internal/lib/testing`)

`jennifer test <file.j>` discovers a file's `test*` methods, runs each with
optional `setUp` / `tearDown` hooks, and reports pass / fail. It is the
`testing` library's assertion vocabulary and runner primitives wired into a
subcommand - "primitives in Go, orchestration as subcommand", the same shape as
`fmt` / `lint` / `profile`.

## A test file

```jennifer
use testing;

def state as int init 0;

func setUp() {              # runs before every test*
    $state = 10;
}

func testAddsUp() {
    testing.assertEqual(2 + 3, 5);
    testing.assertEqual($state, 10);
}

func testGreeting() {
    testing.assertContains("hello world", "world");
    testing.assertTrue(len("abc") == 3);
}

func testDeliberateFail() {
    testing.assertEqual(2 + 2, 5);      # oops
}
```

Running it prints one line per test and a summary; a failure names the
assertion, its source position, and the mismatch:

```text
$ jennifer test mathtest.j
PASS testAddsUp (0 ms)
FAIL testDeliberateFail (0 ms)
     [assertion] mathtest.j:20:5 assertEqual: 4 != 5
PASS testGreeting (0 ms)

2 passed, 1 failed, 3 total
```

## Discovery and hooks

- A **test** is any top-level `func` whose name begins with `test`
  (`testAddsUp`, `testGreeting`, ...). The file's top level runs once first, so
  `def`s and `use` / `import` are in place before any test.
- **`setUp`** (if defined) runs before each test; **`tearDown`** after each.
  Both are optional, looked up by name, and absent by default - the shape of an
  xUnit fixture without the class.

## Flags

| Flag              | Effect                                                                                       |
| ----------------- | -------------------------------------------------------------------------------------------- |
| `--filter=REGEX`  | Run only tests whose name matches `REGEX` (e.g. `--filter=testGreeting`).                     |
| `--format=text`   | Human-readable, one line per test (default).                                                 |
| `--format=tap`    | TAP version 14 - CI-friendly, with a YAML diagnostic block per failure.                       |
| `--format=junit`  | JUnit XML - for CI dashboards that consume it.                                                |
| `--isolated`      | Run each test in a fresh interpreter subprocess: clean state per test, coarser reporting (a pass/fail line, no in-process kind / position). |
| `--allow-empty`   | Exit `0` when no test method is found (none by the `test*` convention, or none matching `--filter`). Without it an empty suite exits `2`. |
| `--coverage[=FMT]`| Also report statement coverage: which executable positions in the tested file(s) ran. `text` (default) prints a per-file and total percentage plus the never-executed positions; `json` emits a machine-readable form. Runs in-process (so it overrides `--isolated`). |

## Coverage

`--coverage` reuses the [profiler](cli_profile.md)'s per-position hit data (no
second counting path): the tests run with statement profiling live, and the
report intersects the recorded hits with every executable statement position
statically walked from the AST. Coverage is scoped to the tested program's
files, so an imported module that merely ran does not skew it. A module overlay
(`MODULE_test.j`) reports the module file and the test file separately.

```text
$ jennifer test --coverage mathlib_test.j
... test report ...

Coverage (statements):
  .../mathlib.j       12/14  (85.7%)
    uncovered: 33:5, 41:9
  .../mathlib_test.j  8/8  (100.0%)
  total: 20/22 (90.9%)
```

`--coverage=json` puts the machine-readable report on **stdout** (files, per-file
`covered` / `total` / `percent` / `uncovered` positions, and the grand total);
the human test report moves to **stderr** so a tool can parse stdout directly -
the same "machine format owns stdout" rule `jennifer profile --format=pprof`
uses.

The same run in TAP:

```text
$ jennifer test --format=tap mathtest.j
TAP version 14
1..3
ok 1 - testAddsUp
not ok 2 - testDeliberateFail
  ---
  kind: assertion
  message: assertEqual: 4 != 5
  file: mathtest.j
  line: 20
  col: 5
  ...
ok 3 - testGreeting
```

## Exit codes

| Code | Meaning                                       |
| ---- | --------------------------------------------- |
| `0`  | All tests passed.                             |
| `1`  | At least one test failed.                     |
| `2`  | Runner error (parse / lex / IO), or no test methods found - no tests ran. |

The `0` / `1` / `2` split matches `jennifer lint`, so a CI step can treat
"failures" and "the tool broke" differently. An empty suite counts as a runner
error, so a gate that only checks the exit status cannot pass an overlay with
nothing in it; `--allow-empty` accepts one that is empty on purpose.

## Assertions

Six builtins from the `testing` library (`internal/lib/testing/assertions.go`,
built into **both** binaries). On failure each throws the canonical
`Error{kind: "assertion"}` positioned at the call site, which the runner catches
and classifies exactly like a user `throw`:

| Assertion                              | Passes when                                              |
| -------------------------------------- | -------------------------------------------------------- |
| `testing.assertEqual(a, b)`            | `a` equals `b` (value **and** kind).                     |
| `testing.assertNotEqual(a, b)`         | `a` differs from `b`.                                    |
| `testing.assertTrue(cond)`             | `cond` (a `bool`) is `true`.                             |
| `testing.assertFalse(cond)`            | `cond` is `false`.                                       |
| `testing.assertContains(container, x)` | `x` occurs in the string / list `container`.             |
| `testing.assertThrows(name, kind)`     | calling the zero-arg method `name` throws an `Error` of that `kind`. |

`assertThrows` takes the method **name** as a string (Jennifer has no function
references - `myTest` is a name, not a value), and the interpreter's
`CallByName` invokes it. `testing.runWith(name, args)` /
`CallByNameWith` bind arguments for framework dispatchers; the zero-arg
`CallByName` is the entry point the runner itself uses.

## Flow (implementation)

The subcommand parses, preprocesses, and runs the file (methods hoist, top level
executes for setup), then:

- **Discovery** - `Interpreter.MethodNames()` filtered to `test*`, overridable
  with `--filter`.
- **Per test** - `setUp` if present, run the test through `CallByName` (timing +
  `ClassifyError` into a record), `tearDown` if present.
- **Report** - `--format=text|tap|junit` via `testinglib.RenderReport`; the
  exit-code split above.
- **`--isolated`** - `os.Executable()` re-invokes the binary as
  `jennifer test --testing-single METHOD FILE.j` (runs exactly one method,
  prints a one-line result); the parent records the child's exit code and line.

**Enabling interpreter change.** A Go builtin can now raise a *catchable*
Jennifer error. `evalCall` / `evalQualifiedCall` previously flattened any
builtin error into a fresh `runtimeError` (losing a custom `kind`); now
`builtinError` passes an `*ErrorSignal` / `*ExitSignal` through unwrapped - so
`interpreter.RaiseError(kind, msg, ...)` throws a real Jennifer error - and
`BuiltinCtx` carries the call-site `file/line/col` so it anchors at the call. No
existing builtin returned a signal, so the change is additive.

**TinyGo.** The subcommand is build-tag split like the other dev tools:
`test.go` (`!tinygo`) has it, `devtools_tinygo.go` stubs it. The assertion
vocabulary and runner primitives live in the always-built `testing` library, so
a hand-written TinyGo suite can still call `testing.assertEqual` /
`testing.run` directly.

Part of the [CLI reference](cli.md).
