#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The ringbuffer module (modules/ringbuffer.j): a fixed-capacity FIFO that keeps
 * only the most recent entries. Feed it a stream of log lines with a small
 * window and watch the oldest fall off, then drain it in order. Pure `.j`; runs
 * on both binaries.
 * Run: jennifer run examples/modules/ringbuffer_demo.j
 * @module ringbuffer_demo
 */
use io;
use strings;
import "../../modules/ringbuffer.j" as ringbuffer;

# Keep only the last 4 log lines of a longer stream.
def stream as list of string init ["boot", "connect", "auth", "query", "query", "close", "reconnect"];
def recent as ringbuffer.RingBuffer init ringbuffer.new(4);
for (def line in $stream) {
    $recent = ringbuffer.push($recent, $line);
    io.printf("+ %s|pad=10 window=[%s]\n", $line, strings.join(ringbuffer.toList($recent), ", "));
}

io.printf("\ndraining %d most-recent lines oldest-first:\n", ringbuffer.size($recent));
def more as bool init true;
repeat {
    if (ringbuffer.isEmpty($recent)) {
        $more = false;
    } else {
        io.printf("  %s\n", ringbuffer.first($recent));
        $recent = ringbuffer.pop($recent);
    }
} until (not $more);
