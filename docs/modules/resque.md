# `resque` - background jobs on Redis

Import with `import "resque.j" as resque;`. Schedule background jobs onto named
queues now and process them from a worker later, over the [`redis`](redis.md)
module. Deliberately **Resque wire-compatible**: queues are Redis lists at
`resque:queue:NAME`, the queue registry is a set at `resque:queues`, and a job
is the JSON envelope `{"class":"WorkerName","args":[...]}`. Because that layout
is the de-facto Resque standard, a job Jennifer enqueues can be processed by a
Ruby-resque / php-resque worker and vice versa. Built on `redis` (which uses
`net`), so this module needs the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "resque.j" as resque;
import "redis.j" as redis;

def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1",
    port: 6379, security: "none", user: "", password: "", db: 0});

# producer: schedule a job
resque.enqueue($db, "email", "SendWelcome", ["user@example.com", "en"]);
```

Runnable: [`examples/modules/resque_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/resque_demo.j).

## Surface

The module works over an existing `redis.Session` - it adds no transport of its
own.

| Call / type                                  | Notes                                                             |
| -------------------------------------------- | ----------------------------------------------------------------- |
| `resque.Job`                                 | A reserved job: `queue`, `class`, `args` (`list of string`).      |
| `resque.enqueue(session, queue, class, args)` | Register `queue` and push a job (`class` + string `args`) onto it, in one pipelined round trip (SADD + RPUSH, as Ruby-resque does). |
| `resque.reserve(session, queues)`            | Pop the next job from the first non-empty queue (priority order), non-blocking; an empty `Job` when all are drained. |
| `resque.reserveBlocking(session, queues, timeoutSec)` | Like `reserve`, but **blocks** on the queues (Redis `BLPOP`) until one has work or `timeoutSec` elapses (0 = block forever); an empty `Job` on timeout. |
| `resque.queueLength(session, queue)`         | Pending jobs on one queue.                                        |
| `resque.queues(session)`                     | Registered queue names (`list of string`).                       |
| `resque.size(session)`                       | Total pending jobs across every queue (one pipelined batch of LLENs). |
| `resque.fail(session, job, message)`         | Record a failed job on the failed list.                          |

`args` is a `list of string` - it maps to Ruby-resque's *positional* arguments
(`perform(a, b)`). A job enqueued elsewhere with a numeric or boolean arg still
reserves cleanly (each arg is read back as its string form).

## Producer and worker

The **producer** is one call. The **worker** is your loop: `reserve`, then
dispatch on the `class` string. A Jennifer module can't call a method by a name
computed at runtime, so the module hands you the decoded `Job` and you branch -
the same class-lookup a Ruby worker does under the hood:

```jennifer
use io;
import "resque.j" as resque;

def db as redis.Session init resque... ;   # a redis.Session
while (true) {
    def job as resque.Job init resque.reserve($db, ["high", "email"]);
    if (len($job.class) == 0) {
        # every queue drained; sleep and poll again (blocking BLPOP is a later add)
        break;
    }
    try {
        if ($job.class == "SendWelcome") {
            io.printf("welcome -> %s\n", $job.args[0]);
        } elseif ($job.class == "Ping") {
            io.printf("pong\n");
        } else {
            resque.fail($db, $job, "unknown class");
        }
    } catch (e) {
        resque.fail($db, $job, $e.message);
    }
}
```

`reserve` checks the queues in the order you pass, so put higher-priority queue
names first. Within one queue jobs are FIFO. For a worker that should sleep
rather than spin when the queues are empty, use `reserveBlocking`, which parks on
the socket (Redis `BLPOP`) until a job arrives or `timeoutSec` elapses; it widens
the session's read deadline for the duration of that one call so the client does
not give up before the server does.

## Compatibility notes

Two behaviours are inherent to Resque, not added here:

- **The `class` is resolved on the worker's side.** `enqueue` only ships the
  string; the runtime that pops the job must define a job by that name. So a
  Ruby worker runs a Jennifer-enqueued job only when its codebase has that
  class.
- **The namespace must match.** Keys use the `resque:` prefix (the Resque
  default); both ends must agree.

For the **php-resque** ecosystem, args follow a single-hash convention
(`args: [{...}]`, plus `id` / `queue_time` envelope fields) rather than Ruby's
positional array; this module emits the positional Ruby form.

## Out of scope

Basics first - these are deferred to a later pass:

- **Full Resque failure records.** `fail` writes a simplified entry, not the
  complete `failed_at` / `exception` / `backtrace` / `worker` shape.
- **Scheduled / delayed jobs and retries.**
- **A configurable namespace** (fixed to `resque:`).

## See also

- [redis.md](redis.md) - the client this module runs on.
- [json.md](../libraries/json.md) - the envelope encode / decode.
- [modules/index.md](index.md) - the module catalog and import rules.
