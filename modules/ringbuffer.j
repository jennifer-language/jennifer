# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A fixed-capacity ring buffer of strings: a bounded FIFO that overwrites the
 * oldest entry when full. `push` appends (dropping the oldest once capacity is
 * exceeded); `pop` removes the oldest; `first` / `last` peek without removing.
 * Elements are ordered oldest-to-newest. Useful for a sliding window of recent
 * events, log lines, or samples.
 *
 * Value-semantic: `push` / `pop` return a fresh buffer (chain them,
 * `$rb = ringbuffer.push($rb, x)`). Since a value-semantic `pop` cannot return
 * both the item and the new buffer, read the oldest with `first` before you
 * `pop` it. Over `lists`; runs on both binaries. Stores strings - serialize
 * other values with `convert.toString` or `json`.
 * @module ringbuffer
 * @example
 * import "ringbuffer.j" as ringbuffer;
 * def rb as ringbuffer.RingBuffer init ringbuffer.new(3);
 * $rb = ringbuffer.push($rb, "a");
 * $rb = ringbuffer.push($rb, "b");
 * $rb = ringbuffer.push($rb, "c");
 * $rb = ringbuffer.push($rb, "d");   # overwrites "a"; buffer is now [b, c, d]
 * def oldest as string init ringbuffer.first($rb);   # "b"
 * $rb = ringbuffer.pop($rb);         # buffer is now [c, d]
 */
use lists;

/**
 * A ring buffer.
 * @field items {list of string} the entries, oldest first
 * @field capacity {int} the maximum number of entries
 */
export def struct RingBuffer {
    items as list of string,
    capacity as int
};

func fail(msg as string) {
    throw Error{ kind: "ringbuffer", message: "ringbuffer: " + $msg, file: "", line: 0, col: 0 };
}

/**
 * Create an empty ring buffer of the given capacity.
 * @param capacity {int} the maximum number of entries (must be >= 1)
 * @return {RingBuffer} the empty buffer
 * @throws {Error} kind "ringbuffer" if capacity is < 1
 */
export func new(capacity as int) {
    if ($capacity < 1) {
        fail("capacity must be >= 1");
    }
    def items as list of string init [];
    return RingBuffer{ items: $items, capacity: $capacity };
}

/**
 * Append an item, dropping the oldest if the buffer is already full. Returns a
 * fresh buffer.
 * @param rb {RingBuffer} the buffer
 * @param item {string} the item to append
 * @return {RingBuffer} the updated buffer
 */
export func push(rb as RingBuffer, item as string) {
    def out as RingBuffer init $rb;
    $out.items = lists.push($out.items, $item);
    if (len($out.items) > $out.capacity) {
        $out.items = lists.slice($out.items, 1);   # drop the oldest
    }
    return $out;
}

/**
 * Remove the oldest item. Returns a fresh buffer. Read the item with `first`
 * before popping it.
 * @param rb {RingBuffer} the buffer
 * @return {RingBuffer} the buffer without its oldest item
 * @throws {Error} kind "ringbuffer" if the buffer is empty
 */
export func pop(rb as RingBuffer) {
    if (len($rb.items) == 0) {
        fail("pop from an empty ring buffer");
    }
    def out as RingBuffer init $rb;
    $out.items = lists.slice($out.items, 1);
    return $out;
}

/**
 * The oldest item, without removing it.
 * @param rb {RingBuffer} the buffer
 * @return {string} the oldest item
 * @throws {Error} kind "ringbuffer" if the buffer is empty
 */
export func first(rb as RingBuffer) {
    if (len($rb.items) == 0) {
        fail("first of an empty ring buffer");
    }
    return $rb.items[0];
}

/**
 * The newest item, without removing it.
 * @param rb {RingBuffer} the buffer
 * @return {string} the newest item
 * @throws {Error} kind "ringbuffer" if the buffer is empty
 */
export func last(rb as RingBuffer) {
    if (len($rb.items) == 0) {
        fail("last of an empty ring buffer");
    }
    return $rb.items[len($rb.items) - 1];
}

/**
 * The number of entries currently held.
 * @param rb {RingBuffer} the buffer
 * @return {int} the entry count
 */
export func size(rb as RingBuffer) {
    return len($rb.items);
}

/**
 * The buffer's capacity.
 * @param rb {RingBuffer} the buffer
 * @return {int} the capacity
 */
export func capacity(rb as RingBuffer) {
    return $rb.capacity;
}

/**
 * Whether the buffer holds no entries.
 * @param rb {RingBuffer} the buffer
 * @return {bool} true if empty
 */
export func isEmpty(rb as RingBuffer) {
    return len($rb.items) == 0;
}

/**
 * Whether the buffer is at capacity.
 * @param rb {RingBuffer} the buffer
 * @return {bool} true if full
 */
export func isFull(rb as RingBuffer) {
    return len($rb.items) >= $rb.capacity;
}

/**
 * A copy of the entries, oldest to newest.
 * @param rb {RingBuffer} the buffer
 * @return {list of string} the entries
 */
export func toList(rb as RingBuffer) {
    return $rb.items;
}
