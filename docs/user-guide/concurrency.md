# Concurrency

Jennifer's concurrency surface is deliberately small. `spawn` launches a
computation in the background and returns a handle of type `task of T`;
the [`task`](../libraries/task.md) library reads the result later.

```jennifer
use io;
use task;

def t as task of int init spawn { return slowComputation(); };
# ... other work happens here, in parallel with the spawn body ...
def n as int init task.wait($t);
io.printf("answer: %d\n", $n);
```

The pieces are the `spawn` keyword, the `task of T` type, the `task`
library (waiting, cooperative cancellation, bounded waits),
[`channel of T`](../libraries/channel.md) for streaming values between
tasks, and the exit-time loud-fail contract. There is no shared mutable
memory and no lock: a task works on its own copy of the data, and values
travel between tasks only by being copied.

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
| Stop a task                       | `task.cancel($t)` + `task.discard($t)` |
| Wait with a deadline              | `task.waitTimeout($t, ms)`         |

The [`task`](../libraries/task.md) reference covers every builtin with
examples.

**Cancellation is cooperative.** `task.cancel($t)` sets a flag the spawned body
observes at its next loop checkpoint, where the runtime raises a catchable "task
cancelled" (kind `"cancelled"`), so a runaway `spawn` can be stopped, and a body
that catches it inside a `try` / `catch` exits cleanly with a partial result. A body blocked in
`time.sleep`, a channel operation, a `task` wait, `os.waitTimeout` or a socket read (TCP
`net.readBytes` / `readN` / `readAll`, UDP `net.recvFrom`, and so the `http`
module's requests) wakes at once and raises the same error. A task blocked in
another system call - `net.accept`, `term.readByte` - does not see the flag
until that call returns; close its socket from another task (the blocked call
then raises a catchable `Error{kind: "closed"}`) or `task.discard` it.
`task.waitTimeout` / `waitAnyTimeout` / `waitAllTimeout`,
`channel.recvTimeout` / `selectTimeout` and `os.waitTimeout` bound a wait,
throwing a catchable `Error{kind: "timeout"}`; `0` polls. Details are in the
[`task` reference's Cancellation and Timeouts sections](../libraries/task.md#cancellation).

**Channels stream values.** Where a `task` observes one result, a
[`channel of T`](../libraries/channel.md) carries many values between goroutines:
`channel.make(capacity)`, `channel.send` (which copies the value in, so the
receiver gets its own copy), `channel.recv`, `channel.close`, and a fan-in
`channel.select`. A channel is shared, but the data through it is copied, so the
value-semantics guarantee below still holds. Producer / consumer, worker pools
and pipelines are the usual shapes.

## Why value-semantics capture matters

**A `spawn` block captures its surrounding scope by deep copy, not
by reference.** Variables visible at the `spawn` site are copied into
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

Lists, maps, structs and bytes copy on assignment and on parameter binding
everywhere; concurrency reuses the rule, so **data races are impossible by
construction**.

The cost: deep-copying a large structure into a spawn frame is O(N),
measurable for a 10 000-element list. In exchange no data needs a lock, a
mutex or an atomic; a channel passes values, it does not guard them.

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
tasks, not the order in which they completed.

### Many live tasks on `jennifer-tiny`

Under `jennifer-tiny` every task runs on one OS thread (concurrency, no
multi-core speedup) and holds a fixed 4 MB stack while it is alive. Starting
tasks that are all alive at once also gets slower the more there are: under
TinyGo's default collector a few dozen start in well under a second, 100 take
about 2 s and 400 about 35 s, with roughly 4 MB of memory each. The standard
`jennifer` binary starts the same 400 in milliseconds.

So on `jennifer-tiny`, keep the number of live tasks small: wait for each task
(or each small batch) before spawning the next, or start a few worker tasks
that take their jobs from a [channel](../libraries/channel.md) instead of
spawning one task per job up front.

### Fire and forget

For background work whose result you do not need, mark the handle with
`task.discard`:

```jennifer
use task;

def alive as task of null init spawn {
    # ... log a metric, send a notification, prefetch a cache ...
    return null;
};
task.discard($alive);
```

`task.discard` is required even for background work that succeeds, because
of the loud-fail contract (next section): it declares that no result is
expected, so a later failure does not fail the program.

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
error to stderr at program exit and exits non-zero. An explicit
`exit N;` keeps its code `N`: the error is still printed, but the code
you chose wins.

An unobserved spawn error is **always** loud, so a bug cannot hide in a task
nobody reads; the only way to silence it is an explicit `task.discard`.

```jennifer
use task;

def bad as task of int init spawn {
    def xs as list of int init [];
    return $xs[5];                          # error inside the body
};
# the program ends here without touching $bad
```

```text
$ jennifer run example.j
example.j: unwaited spawn error: runtime error at /path/to/example.j:5:15: list index 5 out of bounds (len 0)
      return $xs[5];                          # error inside the body
                ^
$ echo $?
1
```

Three ways to make a spawn quiet:

1. `task.wait($t)` - read the result. The error rethrows at the
   wait site; an enclosing `try`/`catch` can suppress it.
2. `task.waitAll($ts)` / `task.waitAny($ts)` - same idea, with
   `waitAll` observing every survivor on the way out.
3. `task.discard($t)` - the explicit fire-and-forget marker.

> **Beware infinite spawns.** When a program runs to its end, the
> loud-fail scan waits for every unobserved task to finish, to learn
> whether it failed. `spawn { while (true) { ... } }` without
> `task.discard` therefore keeps the program from ending, since the
> goroutine never finishes. Pair a long-running or
> potentially-non-terminating spawn with an explicit `task.discard`.
>
> A program that ends **early** - by `exit`, or by an uncaught error -
> does not wait: it reports the unobserved tasks that have already
> failed and abandons the ones still running. `exit` ends the whole
> program, so a server's accept loop blocked in `net.accept` cannot
> keep it alive.

## What Jennifer leaves out

Features common in other languages that Jennifer does not have:

- **Hard abort.** `task.cancel` is cooperative: it takes effect at
  the body's next loop checkpoint, never in the middle of a
  statement. Blocking calls that wake on it (sleeps, channel
  operations, task waits, socket reads) raise at once; a task blocked
  in another system call (`net.accept`) is stopped by closing what it
  waits on instead.
- **Structured concurrency.** No automatic scope-bounded termination of
  unwaited tasks. The loud-fail contract is a lighter answer: weaker than
  Trio / async-await scopes, but it keeps spawned errors visible.
- **Parallel `for` / `map`.** No `for parallel (...)` syntax. Write the
  explicit `spawn` / `waitAll` pair, or a worker pool over a channel.
- **Atomics, mutexes, shared mutable variables.** Value-semantics
  capture removes the need. State that several tasks must see lives
  behind a library handle instead - a channel, or a
  [`kv`](../libraries/kv.md) store (whose `incr` is atomic).

## See also

- [`task` library reference](../libraries/task.md) - every
  builtin, error propagation, cancellation, timeouts, worked
  examples.
- [`channel` library reference](../libraries/channel.md) - sending,
  receiving, closing, and `select` over several channels.
- [Control flow](control-flow.md) - `try`/`catch` works the same
  way around `task.wait` as around any other runtime error.
- [Functions](functions.md) - a spawn body calls functions like any other
  frame.
- [`docs/milestones.md`](../milestones.md) - the concurrency entry has
  the design rationale (data-race-freedom by construction, the
  loud-fail decision).
