# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * A fixed-capacity ring buffer of strings: a bounded FIFO that overwrites the
 * oldest entry when full. `push` appends (dropping the oldest once capacity is
 * exceeded); `pop` removes the oldest; `first` / `last` peek without removing.
 * Entries are ordered oldest-to-newest. Useful for a sliding window of recent
 * events, log lines, or samples.
 *
 * Value-semantic: `push` / `pop` return the updated buffer (chain them,
 * `$rb = ringbuffer.push($rb, x)`, which updates in place - a push writes one
 * slot, whatever the capacity). Since a value-semantic `pop` cannot return both
 * the item and the new buffer, read the oldest with `first` before you `pop`
 * it. Pure `.j`; runs on both binaries. Stores strings - serialize other values
 * with `convert.toString` or `json`.
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

/**
 * A ring buffer. `items` holds the slots in storage order, which is rotated once
 * the buffer has wrapped - read the entries oldest-first with `toList`.
 * @field items {list of string} the slots (storage order, not oldest-first)
 * @field capacity {int} the maximum number of entries
 * @field head {int} the slot index of the oldest entry
 * @field count {int} the number of entries held
 */
export def struct RingBuffer {
    items as list of string,
    capacity as int,
    head as int,
    count as int
};

func fail(msg as string) {
    throw Error{kind: "ringbuffer", message: "ringbuffer: " + $msg};
}

/**
 * Create an empty ring buffer of the given capacity.
 * @param capacity {int} the maximum number of entries (>= 1)
 * @return {RingBuffer} the empty buffer
 * @throws {Error} kind "ringbuffer" if capacity is < 1
 */
export func new(capacity as int) {
    if ($capacity < 1) {
        fail("capacity must be >= 1");
    }
    def items as list of string init [];
    return RingBuffer{items: $items, capacity: $capacity, head: 0, count: 0};
}

/**
 * Append an item, dropping the oldest if the buffer is already full.
 * @param rb {RingBuffer} the buffer
 * @param item {string} the item to append
 * @return {RingBuffer} the updated buffer
 */
export func push(rb as RingBuffer, item as string) {
    if ($rb.count >= $rb.capacity) {
        # Full: the oldest slot takes the new item, and the next slot becomes
        # the oldest.
        $rb.items[$rb.head] = $item;
        $rb.head = ($rb.head + 1) % $rb.capacity;
        return $rb;
    }
    def slot as int init ($rb.head + $rb.count) % $rb.capacity;
    if ($slot == len($rb.items)) {
        $rb.items[] = $item;
    } else {
        $rb.items[$slot] = $item;
    }
    $rb.count = $rb.count + 1;
    return $rb;
}

/**
 * Remove the oldest item. Read it with `first` before popping it.
 * @param rb {RingBuffer} the buffer
 * @return {RingBuffer} the updated buffer
 * @throws {Error} kind "ringbuffer" if the buffer is empty
 */
export func pop(rb as RingBuffer) {
    if ($rb.count == 0) {
        fail("pop from an empty ring buffer");
    }
    $rb.items[$rb.head] = ""; # drop the reference to the popped entry
    $rb.head = ($rb.head + 1) % $rb.capacity;
    $rb.count = $rb.count - 1;
    return $rb;
}

/**
 * The oldest item, without removing it.
 * @param rb {RingBuffer} the buffer
 * @return {string} the oldest item
 * @throws {Error} kind "ringbuffer" if the buffer is empty
 */
export func first(rb as RingBuffer) {
    if ($rb.count == 0) {
        fail("first of an empty ring buffer");
    }
    return $rb.items[$rb.head];
}

/**
 * The newest item, without removing it.
 * @param rb {RingBuffer} the buffer
 * @return {string} the newest item
 * @throws {Error} kind "ringbuffer" if the buffer is empty
 */
export func last(rb as RingBuffer) {
    if ($rb.count == 0) {
        fail("last of an empty ring buffer");
    }
    return $rb.items[($rb.head + $rb.count - 1) % $rb.capacity];
}

/**
 * The number of entries currently held.
 * @param rb {RingBuffer} the buffer
 * @return {int} the entry count
 */
export func size(rb as RingBuffer) {
    return $rb.count;
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
 * @return {bool} true when empty
 */
export func isEmpty(rb as RingBuffer) {
    return $rb.count == 0;
}

/**
 * Whether the buffer is at capacity.
 * @param rb {RingBuffer} the buffer
 * @return {bool} true when full
 */
export func isFull(rb as RingBuffer) {
    return $rb.count >= $rb.capacity;
}

/**
 * A copy of the entries, oldest to newest.
 * @param rb {RingBuffer} the buffer
 * @return {list of string} the entries in order
 */
export func toList(rb as RingBuffer) {
    def out as list of string init [];
    for (def k in 0..$rb.count) {
        $out[] = $rb.items[($rb.head + $k) % $rb.capacity];
    }
    return $out;
}
