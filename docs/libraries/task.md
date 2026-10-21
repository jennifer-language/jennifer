# `task` - operations on spawned computations

Enable with `use task;`. Builtins for observing, joining, cancelling
and bounding `task of T` values produced by `spawn { ... }` blocks.

For when to use `spawn`, value-semantics capture, and the exit-time
loud-fail contract, see
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
| `task.waitTimeout($t, ms)`      | `T`             | Like `task.wait`, but throws a catchable `Error{kind: "timeout"}` if `$t` doesn't finish in `ms` (`ms` up to ~9.2e12; a larger value is a catchable error, not a silent instant timeout). |
| `task.waitAnyTimeout($ts, ms)`  | `int`           | Like `task.waitAny`, but throws a catchable `Error{kind: "timeout"}` if none finishes in `ms`. |

`task.wait` is the core call; the rest are conveniences or
non-blocking variants. More complex patterns compose from these.

## Error propagation

A `task of T` carries either a value or an error after its body
finishes. `task.wait` returns the value or re-raises the error at the wait call,
where an enclosing `try`/`catch` catches it like any runtime error. The
error keeps the position where it happened in the spawn body:

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

A `task.wait` marks the task observed whether it returns or
re-raises. `task.discard($t)` also marks a task observed; use it
for fire-and-forget.

## Exit-time loud-fail

A task that ends in an error and is never observed (never
`task.wait`'d, never `task.discard`'d) has its error printed to
stderr at program exit, and the process exits non-zero, so no spawn
error disappears silently.

Use `task.wait` when you need the result and `task.discard($t);`
when you don't; both make the intent visible at the call site.

```jennifer
use task;

def alive as task of null init spawn {
    # ... long-running background work ...
    return null;
};
task.discard($alive);                       # explicit fire-and-forget
```

When the program runs to its end, the loud-fail scan waits for
every unobserved task to complete. A `spawn { while (true) { ... } }`
without `task.discard` therefore hangs the program at exit; pair a
body that may not terminate with an explicit `task.discard`.

## Working with collections of tasks

### `task.waitAll($ts)`

Spawn N units of work, wait for all results in order.

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
first error in list order. The other errors are observed but not
surfaced; to get them, wait on each task individually with your
own try/catch.

### `task.waitAny($ts)`

First to finish wins. Returns the *index* of the first completed
task (the lowest index when several are already done); follow up
with `task.wait($ts[$idx])` to read the result and mark it observed.
The losing tasks keep running; observe or `task.discard` them so the
loud-fail doesn't report them.

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

`task.waitAny([])` is a positioned runtime error.

## Cancellation

`task.cancel($t)` requests **cooperative** cancellation: it sets a flag the
spawned body observes at its next **loop checkpoint** (the start of any `while` /
`for` / for-each / `repeat` iteration), where the runtime raises a **catchable
"task cancelled"** error, stopping the loop and the spawn at a safe point.
The tree-walker cannot preempt a goroutine; a non-loop computation is instead
bounded by the call-depth guard.

A non-terminating `spawn` can thus be cancelled. Cancellation reaches a body
only at a loop checkpoint: a task blocked in a system call - `net.accept`, a socket read,
`term.readByte` - does not see the flag until that call returns. Unblock it by
closing its socket from another task (the blocked call raises a catchable
`Error{kind: "closed"}`), or `task.discard` it. A program that ends by `exit` or an uncaught error
does not wait for still-running tasks at all; one that runs to its end waits for
every task nobody discarded.

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

`task.cancelled()` is a **non-raising** poll of the current body's cancel flag,
for use at an arbitrary point (e.g. before an expensive step). It does **not**
suppress the loop-checkpoint raise; use the `try` / `catch` idiom above to exit
a loop cleanly.

Notes:
- `task.cancel` does **not** wait and does **not** mark the task observed. The
  idiom to stop-and-forget is `task.cancel($t); task.discard($t);`. If you instead
  `task.wait` a cancelled task whose body didn't catch the error, the "task
  cancelled" error is re-raised by the wait (catchable).
- Cancelling an already-finished task is a harmless no-op.

## Timeouts

`task.waitTimeout($t, ms)` is `task.wait` bounded by a millisecond deadline: it
returns the result (re-raising a body error, exactly like `wait`) if `$t`
completes within `ms`, and otherwise throws a **catchable**
`Error{kind: "timeout"}` (message `task.waitTimeout: timed out after N ms`) - the
same kind `net` uses for its deadlines.
`task.waitAnyTimeout($ts, ms)` is the multi-task form, returning the first
finisher's index or throwing on timeout. On a timeout the task(s) stay live and
unobserved, so follow up with a retry, or `task.cancel` + `task.discard`. A body
error re-raised by `waitTimeout` keeps its own kind, so check `kind` to tell the
two apart:

```jennifer
try {
    def v as int init task.waitTimeout($t, 500);   # up to 500 ms
} catch (e) {
    if ($e.kind != "timeout") { throw $e; }   # the body failed
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
`task.wait` carry the position from the spawn body, so the report
shows the actual fault location.

## See also

- [../user-guide/concurrency.md](../user-guide/concurrency.md) -
  worked-example tour: when to spawn, what value-semantics capture
  buys you, the loud-fail contract, what Jennifer leaves out.
- [../technical/interpreter.md > Concurrency](../technical/interpreter.md#concurrency) -
  internals: goroutine mapping, frame snapshot, error routing,
  registry, exit-time scan.
- [../milestones.md](../milestones.md) - the design history of `spawn` +
  `task of T` + the `task` library; later milestones use
  them to build `fs`, `net`, `httpd`.
