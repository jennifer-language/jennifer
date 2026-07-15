# `ringbuffer` - fixed-capacity ring buffer

Import with `import "ringbuffer.j" as ringbuffer;`. A fixed-capacity FIFO of
strings that **overwrites the oldest entry when full** - a sliding window of the
most recent items (log lines, events, samples). Pure `.j` over `lists`; runs on
both binaries.

```jennifer
import "ringbuffer.j" as ringbuffer;

def rb as ringbuffer.RingBuffer init ringbuffer.new(3);
$rb = ringbuffer.push($rb, "a");
$rb = ringbuffer.push($rb, "b");
$rb = ringbuffer.push($rb, "c");
$rb = ringbuffer.push($rb, "d");   # overwrites "a"; buffer is now [b, c, d]
ringbuffer.first($rb);             # "b" (oldest)
$rb = ringbuffer.pop($rb);         # buffer is now [c, d]
```

Runnable: [`examples/modules/ringbuffer_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/ringbuffer_demo.j).

## Surface

```jennifer
def struct ringbuffer.RingBuffer { items as list of string, capacity as int };
```

| Call | Returns | |
| ---- | ------- | - |
| `ringbuffer.new(capacity)` | `RingBuffer` | an empty buffer of the given capacity (>= 1) |
| `ringbuffer.push(rb, item)` | `RingBuffer` | append, dropping the oldest if already full |
| `ringbuffer.pop(rb)` | `RingBuffer` | remove the oldest (throws if empty) |
| `ringbuffer.first(rb)` | `string` | peek the oldest (throws if empty) |
| `ringbuffer.last(rb)` | `string` | peek the newest (throws if empty) |
| `ringbuffer.size(rb)` | `int` | the number of entries held |
| `ringbuffer.capacity(rb)` | `int` | the capacity |
| `ringbuffer.isEmpty(rb)` | `bool` | whether it holds no entries |
| `ringbuffer.isFull(rb)` | `bool` | whether it is at capacity |
| `ringbuffer.toList(rb)` | `list of string` | a copy of the entries, oldest to newest |

## Reading while removing

Because the buffer is **value-semantic**, a `pop` cannot return both the removed
item and the new buffer. Read the oldest with `first` *before* you `pop` it:

```jennifer
repeat {
    if (ringbuffer.isEmpty($rb)) {
        # done
    } else {
        def oldest as string init ringbuffer.first($rb);
        # ... use $oldest ...
        $rb = ringbuffer.pop($rb);
    }
} until (ringbuffer.isEmpty($rb));
```

## Scope

- **Strings only.** Store other values through `convert.toString` or a `json` /
  `encoding` representation.
- **Value-semantic.** `push` / `pop` return a fresh buffer; assign the result
  back (`$rb = ringbuffer.push($rb, x)`). The original is unchanged.
- **Overwrite on full is silent.** `push` drops the oldest when at capacity with
  no signal - check `isFull` first if you need to know.

## See also

- [lists.md](../libraries/lists.md) - the list operations underneath.
- [bloom.md](bloom.md) - the sibling data-structure module.
- [modules/index.md](index.md) - the module catalog and import rules.
