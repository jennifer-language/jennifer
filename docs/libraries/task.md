# `task` - operations on spawned computations

Enable with `use task;`. Five builtins for observing and joining
`task of T` values produced by `spawn { ... }` blocks. The library
ships alongside the `spawn` keyword and the `task of T`
type kind; together they form Jennifer's concurrency surface.

For the broader story (when to use `spawn`, what value-semantics
capture buys you, the exit-time loud-fail contract for unwaited
errors), see
[../user-guide/concurrency.md](../user-guide/concurrency.md).

```jennifer
use io;
use task;

def t as task of int init spawn { return 1 + 1; };
def n as int init task.wait($t);
io.printf("%d\n", $n);                      # 2
```

## Surface

| Call                            | Returns         | Notes                                                                                          |
| ------------------------------- | --------------- | ---------------------------------------------------------------------------------------------- |
| `task.wait($t)`                 | `T`             | Block until `$t` finishes; return its value, or re-raise its error.                            |
| `task.poll($t)`                 | `bool`          | Non-blocking: true once `$t` has completed (value or error available).                         |
| `task.discard($t)`              | `null`          | Mark `$t` fire-and-forget so the exit-time loud-fail skips it. Does not block.                 |
| `task.waitAll($ts)`             | `list of T`     | Wait for every task in `$ts`; results in list order; re-raises the first error if any.         |
| `task.waitAny($ts)`             | `int`           | Block until any task in `$ts` is done; return its index. Caller follows up with `task.wait`.   |
| `task.cancel($t)`               | `null`          | Request cooperative cancellation of `$t` (see "Cancellation"). Does not block.                  |
| `task.cancelled()`              | `bool`          | Non-raising poll: true if the **current** spawn body has been cancelled (always false on main). |
| `task.waitTimeout($t, ms)`      | `T`             | Like `task.wait`, but throws a catchable "timed out" error if `$t` doesn't finish in `ms` (`ms` up to ~9.2e12; a larger value is a catchable error, not a silent instant timeout). |
| `task.waitAnyTimeout($ts, ms)`  | `int`           | Like `task.waitAny`, but throws a catchable "timed out" error if none finishes in `ms`.         |

`task.wait` is the workhorse - everything else is a convenience or
a non-blocking variant. The full surface is intentionally small;
patterns more complex than "wait for the result", "wait for them
all", "wait for whichever is first" compose by hand on top of
these five.

## Error propagation

A `task of T` carries either a value or an error after its body
finishes. `task.wait` returns the value when there is one and
re-raises the error otherwise - the rethrow surfaces as a
positioned runtime error at the wait site, so an enclosing
`try`/`catch` catches it the same way it catches any
runtime error:

```jennifer
use task;

def t as task of int init spawn {
    def xs as list of int init [];
    return $xs[5];                          # out-of-bounds inside the spawn
};

try {
    def n as int init task.wait($t);        # rethrown here
} catch (e) {
    io.printf("caught: %s\n", $e.message);  # "list index 5 out of bounds (len 0)"
}
```

A successful `task.wait` and a `task.wait` that re-raises *both*
mark the task observed - the parent saw the outcome either way.
`task.discard($t)` is the third way to mark a task observed; use
it for fire-and-forget where you genuinely don't care about the
result.

## Exit-time loud-fail

The contract: a task that ends in an error and is never
observed (never `task.wait`'d, never `task.discard`'d) has its
error printed to stderr at program exit, and the process exits
non-zero. No spawn error can silently disappear from the run.

Default to `task.wait` when you need the result; default to
`task.discard($t);` when you genuinely don't. Both make the
intent visible at the call site. Doing neither is the "no
footguns" wake-up call - the loud-fail will surface it.

```jennifer
use task;

def alive as task of null init spawn {
    # ... long-running background work ...
    return null;
};
task.discard($alive);                       # explicit fire-and-forget
```

Note: the loud-fail scan blocks on every unobserved task to wait
for completion before deciding. A `spawn { while (true) { ... } }`
without `task.discard` will hang the program at exit since the
goroutine never finishes. Spawned bodies that may not terminate
should be paired with an explicit `task.discard` at the top of
the scope.

## Working with collections of tasks

### `task.waitAll($ts)`

Common pattern: spawn N units of work, wait for all results in
order.

```jennifer
use task;

func worker(n as int) {
    return $n * $n;
}

def tasks as list of task of int init [];
def i as int init 1;
while ($i <= 4) {
    $tasks[] = spawn { return worker($i); };
    $i = $i + 1;
}

def squares as list of int init task.waitAll($tasks);
# $squares is [1, 4, 9, 16]
```

If any task in the list ended in an error, `waitAll` drains every
other task (so the loud-fail stays quiet) and then re-raises the
first error in list order. The other errors are observed-but-not-
surfaced - if you need them, wait on each task individually with
your own try/catch.

### `task.waitAny($ts)`

"First to finish wins" pattern. Returns the *index* of the first
completed task; you follow up with `task.wait($ts[$idx])` to read
the result (and mark that one observed). The losing tasks keep
running; observe them or `task.discard` them so the loud-fail
doesn't catch them.

```jennifer
use task;

def fast as task of int init spawn { return 1; };
def slow as task of int init spawn {
    # imagine more work here
    return 2;
};
def candidates as list of task of int init [$fast, $slow];

def winner as int init task.waitAny($candidates);
def value as int init task.wait($candidates[$winner]);
task.discard($candidates[1 - $winner]);     # release the loser
```

`task.waitAny([])` is a positioned runtime error - there's nothing
to wait on.

## Cancellation

`task.cancel($t)` requests **cooperative** cancellation: it sets a flag the
spawned body observes at its next **loop checkpoint** (the start of any `while` /
`for` / for-each / `repeat` iteration), where the runtime raises a **catchable
"task cancelled"** error. That stops the loop - and the spawn - at a safe point.
The tree-walker cannot preempt a goroutine, so a body makes progress toward a
checkpoint by looping; a tight non-loop computation is instead bounded by the
call-depth guard.

This directly retires the exit-time hang: a non-terminating `spawn` that would
otherwise block the program at exit can be cancelled and it stops.

```jennifer
def t as task of int init spawn {
    def n as int init 0;
    while (true) { $n = $n + 1; }
    return $n;
};
task.cancel($t);
task.discard($t);          # cancel + discard: stop and forget
```

**Clean partial results** use `try` / `catch`: catch the raised "task cancelled"
inside the body and return what you have.

```jennifer
def t as task of int init spawn {
    def n as int init 0;
    try {
        while (true) { $n = $n + 1; }
    } catch (e) {
        # cancelled - fall through with the partial count
    }
    return $n;
};
task.cancel($t);
def partial as int init task.wait($t);   # a clean value, not an error
```

`task.cancelled()` is a **non-raising** poll of the current body's cancel flag -
useful at an arbitrary point (e.g. before starting an expensive step). It does
**not** suppress the loop-checkpoint raise, so inside a loop the raise still fires;
the `try` / `catch` idiom above is the way to exit a loop cleanly.

Notes:
- `task.cancel` does **not** wait and does **not** mark the task observed. The
  idiom to stop-and-forget is `task.cancel($t); task.discard($t);`. If you instead
  `task.wait` a cancelled task whose body didn't catch the error, the "task
  cancelled" error is re-raised at the wait site (catchable).
- Cancelling an already-finished task is a harmless no-op.

## Timeouts

`task.waitTimeout($t, ms)` is `task.wait` bounded by a millisecond deadline: it
returns the result (re-raising a body error, exactly like `wait`) if `$t`
completes within `ms`, and otherwise throws a **catchable** "timed out" error.
`task.waitAnyTimeout($ts, ms)` is the multi-task form, returning the first
finisher's index or throwing on timeout. On a timeout the task(s) stay live and
unobserved, so follow up with a retry, or `task.cancel` + `task.discard`.

```jennifer
try {
    def v as int init task.waitTimeout($t, 500);   # up to 500 ms
} catch (e) {
    task.cancel($t);
    task.discard($t);
}
```

## Errors

The boundary checks are uniform across the library:

- Wrong argument count: `task.wait expects 1 argument (task), got 2`.
- Wrong scalar / structural type:
  `task.wait: argument must be a task, got int`,
  `task.waitAll: argument must be a list of task, got string`,
  `task.waitAll: element 2: argument must be a task, got int`.
- Empty list to `waitAny`:
  `task.waitAny: list is empty (no tasks to wait on)`.

All errors are positioned at the call site. Errors re-raised by
`task.wait` carry the position from the spawn body, so debuggers
and human readers see the actual fault location, not the wait
site.

## See also

- [../user-guide/concurrency.md](../user-guide/concurrency.md) -
  worked-example tour: when to spawn, what value-semantics capture
  buys you, the loud-fail contract, what's deliberately deferred
  to later milestones.
- [../technical/interpreter.md > Concurrency](../technical/interpreter.md#concurrency-m160) -
  internals: goroutine mapping, frame snapshot, error routing,
  registry, exit-time scan.
- [../milestones.md](../milestones.md) - ships `spawn` +
  `task of T` + the `task` library; later milestones use
  them to build `fs`, `net`, `httpd`.
