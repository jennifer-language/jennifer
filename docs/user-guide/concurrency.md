# Concurrency

Jennifer's concurrency surface is small on purpose. A program can
launch a computation in the background with `spawn`, get back a
handle of type `task of T`, and read the result later with the
[`task`](../libraries/task.md) library.

```jennifer
use io;
use task;

def t as task of int init spawn { return slowComputation(); };
# ... other work happens here, in parallel with the spawn body ...
def n as int init task.wait($t);
io.printf("answer: %d\n", $n);
```

That's the whole story. No channels, no shared memory, no locks,
no cancellation tokens, no goroutine-leak gotchas. The four pieces
- the `spawn` keyword, the `task of T` type, the `task` library,
and the exit-time loud-fail contract - are what M16.0 ships.

## The model

A `spawn { ... }` block evaluates its body **concurrently with the
rest of the program**. It returns immediately with a `task of T`,
where `T` is the body's declared return type at the use site. The
body runs to its own `return EXPR;`; that becomes the task's
result. When the body raises a runtime error instead, that becomes
the task's error.

Reading the result is a separate, explicit step:

| Want                              | Use                                |
| --------------------------------- | ---------------------------------- |
| The value (block until ready)     | `task.wait($t)`                    |
| Check whether it's done yet       | `task.poll($t)`                    |
| Fire and forget                   | `task.discard($t)`                 |
| Wait for a list of tasks          | `task.waitAll($ts)`                |
| First-to-finish from a list       | `task.waitAny($ts)`                |

See the [`task`](../libraries/task.md) library reference for the
full surface and worked examples per builtin.

## Why value-semantics capture matters

The biggest single design choice in M16.0 is that **a `spawn`
block captures its surrounding scope by deep copy, not by
reference**. Variables visible at the `spawn` site are copied into
the spawn frame at the moment of launch; mutations on either side
afterwards are independent.

```jennifer
use io;
use task;

def xs as list of int init [1, 2, 3];
def t as task of int init spawn {
    return $xs[0] + $xs[1] + $xs[2];   # sees [1, 2, 3]
};

$xs[] = 99;                            # parent mutates after spawn

def total as int init task.wait($t);   # still 6
io.printf("%d\n", $total);
```

This is the same value-semantics discipline Jennifer applies
everywhere - lists, maps, structs, bytes all copy on assignment
and on function-parameter binding. Concurrency reuses the rule, so
**data races are impossible by construction**. There is no shared
mutable state to race on.

The cost is the obvious one: deep-copying a large structure into a
spawn frame is O(N). For a 10 000-element list that's measurable.
The benefit is that you never need a lock, a mutex, an atomic, or
a channel to coordinate; concurrent code reads like sequential
code with `spawn` markers.

## Patterns

### Parallel compute

```jennifer
use io;
use task;

func work(n as int) {
    # ... CPU-bound computation ...
    return $n * $n;
}

def tasks as list of task of int init [];
def i as int init 1;
while ($i <= 4) {
    $tasks[] = spawn { return work($i); };
    $i = $i + 1;
}

def results as list of int init task.waitAll($tasks);
# results are in list order, regardless of completion order
io.printf("%a\n", $results);
```

`task.waitAll` returns results in the same order as the input
tasks, not the order in which they completed. That's the property
most parallel-compute code wants.

### Fire and forget

For background work whose result you genuinely don't care about,
launch with `spawn` and mark the handle with `task.discard`:

```jennifer
use task;

def alive as task of null init spawn {
    # ... log a metric, send a notification, prefetch a cache ...
    return null;
};
task.discard($alive);
```

`task.discard` is required even for happy-path background work
because of the loud-fail contract (next section). It declares to
the runtime that no result is expected, so a later failure won't
crash the program.

### First-to-finish

`task.waitAny` returns the index of whichever task finished first;
the caller follows up with `task.wait($ts[$idx])` to read its
value. The other tasks keep running and need explicit observation:

```jennifer
use io;
use task;

def fast as task of int init spawn { return 1; };
def slow as task of int init spawn { return 2; };
def candidates as list of task of int init [$fast, $slow];

def winner as int init task.waitAny($candidates);
def value as int init task.wait($candidates[$winner]);
task.discard($candidates[1 - $winner]);

io.printf("winner=%d val=%d\n", $winner, $value);
```

## The loud-fail contract

If a spawn body raises a runtime error and the program never
observes the resulting task (never `task.wait`s it, never
`task.waitAll`s it, never `task.discard`s it), Jennifer prints the
error to stderr at program exit and exits non-zero.

This is deliberate. The default of every other concurrency model
is some flavour of "errors in spawned work get silently dropped
unless you go out of your way to handle them." That's a footgun
- bugs hide inside unobserved tasks until something else breaks
much later. M16.0's contract is the inverse: an unobserved spawn
error is **always** loud. The only way to silence it is to say so
explicitly with `task.discard`.

```jennifer
use task;

def bad as task of int init spawn {
    def xs as list of int init [];
    return $xs[5];                          # error inside the body
};
# the program ends here without touching $bad
```

```
$ jennifer run example.j
spawn error (unwaited): list index 5 out of bounds (len 0)
$ echo $?
1
```

Three ways to make a spawn quiet:

1. `task.wait($t)` - read the result. The error rethrows at the
   wait site; an enclosing `try`/`catch` can suppress it.
2. `task.waitAll($ts)` / `task.waitAny($ts)` - same idea, with
   `waitAll` observing every survivor on the way out.
3. `task.discard($t)` - the explicit fire-and-forget marker.

Doing nothing is not a fourth option.

> **Beware infinite spawns.** The loud-fail scan blocks on every
> unobserved task to determine whether it produced an error.
> `spawn { while (true) { ... } }` without `task.discard` will
> hang the program at exit, since the goroutine never finishes.
> Long-running or potentially-non-terminating spawns should be
> paired with an explicit `task.discard` at the top of the scope.

## What's deliberately not in v1

The M16.0 surface stops short of several features common to other
languages' concurrency stories. Each was considered and deferred,
not rejected:

- **Channels.** Inter-task communication beyond "wait for a
  return value" is not in v1. The chosen surface (`task of T` +
  `wait/waitAll/waitAny`) handles the most common cases without
  the bookkeeping channels require. A channel primitive would
  arrive in a later M16.x milestone.
- **Cancellation tokens.** No way to signal a running task to
  stop. The spawn body runs to completion or to an unhandled
  error. Cancellation is an open design question (cooperative
  flag? hard abort? structured-concurrency tree?) and stays
  deferred.
- **Timeouts.** No built-in `task.waitWithTimeout`. Build one with
  a sentinel spawn that returns after `time.sleep` and a
  `task.waitAny` over the pair. A higher-level helper may ship
  later.
- **Structured concurrency.** No automatic scope-bounded
  termination of unwaited tasks. The loud-fail contract is
  Jennifer's lighter-weight answer: not as strong a guarantee as
  Trio / async-await scopes, but enough to keep spawned errors
  visible.
- **Parallel `for` / `map`.** No `for parallel (...)` syntax. If
  you need it, write the explicit `spawn` / `waitAll` pair; that's
  what a parallel-map combinator would compile to anyway.
- **Atomics, mutexes, shared mutable state.** Value-semantics
  capture removes the need.

The single most likely follow-on in this area is a `time`-aware
helper for timeouts; channels are second; structured-concurrency
scopes a distant third (and might never land, since the loud-fail
contract already addresses the main pain point).

## See also

- [`task` library reference](../libraries/task.md) - the five
  builtins, error propagation, worked examples.
- [Control flow](control-flow.md) - `try`/`catch` works the same
  way around `task.wait` as around any other runtime error.
- [Methods](methods.md) - calling methods from inside a spawn body
  works normally; the body inherits the global env the same way
  any other call frame does.
- [`docs/milestones.md`](../milestones.md) - the M16.0 entry has
  the design rationale (data-race-freedom by construction, the
  loud-fail decision, what's deferred to later M16.x).
