# `channel` - CSP channels between goroutines

The `channel` library provides CSP-style channels: typed conduits that carry
values between a `spawn`ed body and the rest of the program (or between several
spawns). Channels are the coordination primitive that pairs with
[`spawn` / `task`](task.md) - where `task` observes a single result, a channel
streams many values and lets producers and consumers rendez-vous.

```jennifer
use channel;
use task;

def ch as channel of int init channel.make(0);   # unbuffered
def producer as task of int init spawn {
    def i as int init 0;
    while ($i < 5) {
        channel.send($ch, $i * 10);
        $i = $i + 1;
    }
    channel.close($ch);
    return 0;
};
# consumer: drain until the producer closes the channel
def sum as int init 0;
try {
    while (true) {
        $sum = $sum + channel.recv($ch);
    }
} catch (e) {
}
task.wait($producer);
```

## Why channels (not locks)

A channel is a **shared handle**: copies - including the deep copy a `spawn` makes
of its captured scope - refer to the one underlying conduit, which is what lets a
spawn body and its parent talk. But the **values sent through it are copied** at
the send site, so channels carry copies and Jennifer's no-shared-mutable-state
guarantee still holds. That is precisely why channels, not mutexes or atomics, are
the right primitive: a shared lock would expose shared mutable state and stays
rejected. A channel shares the *conduit*, never the *data*.

`channel` is a **contextual keyword**: `channel of T` is a type, but `channel`
stays an ordinary identifier everywhere else (a field or parameter named
`channel` keeps working).

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `channel.make(capacity)` | `channel of T` | A new channel. `capacity` 0 is unbuffered (send blocks for a receiver); `n` buffers up to `n` values (a capacity beyond a generous ceiling is a catchable error, not an OOM crash). `T` comes from the binding. |
| `channel.send(ch, value)` | `null` | Checks `value` against the channel's `T`, deep-copies it in, then sends (blocking per capacity). Wrong type / send on a closed channel are catchable errors. |
| `channel.recv(ch)` | `T` | Blocks and returns the next value. On a closed **and** drained channel it throws a catchable "receive on a closed channel". |
| `channel.close(ch)` | `null` | Closes the channel so a draining receiver sees the end. Double-close is a catchable error. |
| `channel.select(chs)` | `T` | Fan-in: the next value from any of the channels; drops closed ones and throws when all are closed and drained. |
| `channel.len(ch)` | `int` | Number of values currently buffered. |
| `channel.capacity(ch)` | `int` | The channel's buffer capacity (0 = unbuffered). |

## Value semantics

`channel.send` copies the value in, so a later mutation by the sender never
reaches the receiver:

```jennifer
def ch as channel of list of int init channel.make(1);
def xs as list of int init [1, 2, 3];
channel.send($ch, $xs);
$xs[0] = 999;                                  # mutate after send
def got as list of int init channel.recv($ch);  # still [1, 2, 3]
```

The element type is enforced at **both** ends. `channel.send` validates the value
against the channel's declared `T` at the send site (`channel.send($intCh, "x")`
is a catchable "value must be int, got string"), and the receiving binding checks
it again (`def n as int init channel.recv($ch);` rejects a non-int) - so a
mismatch is caught where it happens, and a `channel of int` still cannot be
assigned to a `channel of string`. A channel not yet bound to a `channel of T`
(a bare `channel.make` result) carries no recorded type, so its first send is
unchecked - like a fresh generic list literal - until a typed binding stamps `T`.

## Closing and draining

`channel.close` signals "no more values". A receiver draining with
`channel.recv` sees a catchable "receive on a closed channel" once the buffer is
empty, so the idiomatic consumer loop is a `try` / `catch`:

```jennifer
try {
    while (true) {
        process(channel.recv($ch));
    }
} catch (e) {
    # channel drained and closed - done
}
```

Only the producer should close, and only once. After close, buffered values still
receive until drained; further sends are catchable errors.

## `select` (fan-in)

`channel.select(chs)` blocks until any channel in the list has a value and returns
it - a merge of several producers into one consumer. A closed channel drops out of
the wait set; when every channel is closed and drained, `select` throws a catchable
"all channels are closed".

```jennifer
try {
    while (true) {
        handle(channel.select([$a, $b, $c]));
    }
} catch (e) {
    # every input closed
}
```

`select` returns the received **value**, not an index of which channel fired (a
channel receive is destructive and Jennifer has no multiple-return, so an
index-plus-value form would need a different shape). It is the merge / fan-in
primitive; per-channel dispatch is a possible later addition.

## Blocking and cancellation

Channel operations block like Go's: an unbuffered `send` waits for a receiver, a
`recv` waits for a value. A channel with no counterpart (an unbuffered `send`
nothing will receive, a `recv` on a channel nothing will send to or close)
**hangs** the program - the same sharp edge as a non-terminating `spawn`. Note
this does *not* trigger Go's "all goroutines asleep" deadlock abort: the
interpreter keeps a signal-handler goroutine alive (for `SIGUSR1` diagnostics), so
the runtime never sees every goroutine blocked, and the program waits until it is
killed rather than aborting. A spawn body blocked in `channel.send` / `recv` is
also **not** at a loop checkpoint, so [`task.cancel`](task.md#cancellation) cannot
interrupt it. Unblock a waiting body by `channel.close`-ing the channel it waits
on (a drained `recv` then raises), or size a buffered channel so a send does not
block. Cancellable channel operations are a possible later addition.

## Patterns

- **Producer / consumer**: one spawn sends, the main goroutine drains until close
  (the opening example).
- **Fan-out (worker pool)**: several spawned workers `recv` from one job channel;
  close it to signal shutdown, each worker's `recv` then ends.
- **Fan-in**: several spawned producers `send` into one channel (or use
  `channel.select` over several channels) that one consumer drains.
- **Pipeline**: stage N reads its input channel and sends into stage N+1's; each
  stage closes its output when its input drains.

## See also

- [`task`](task.md) - observe a single spawned result; cancellation and timeouts.
- [Concurrency](../user-guide/concurrency.md) - the `spawn` model and
  value-semantics capture.
