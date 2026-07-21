# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# `defer` schedules a call to run when the enclosing block exits - on every exit
# path, last-registered-first. This traces acquire / release ordering without
# touching real I/O so the output is deterministic.
use io;

func acquire(name as string) { io.printf("acquire %s\n", $name); }
func release(name as string) { io.printf("release %s\n", $name); }

func process(items as list of int) {
    acquire("db");
    defer release("db");           # released last (LIFO)
    acquire("file");
    defer release("file");         # released first

    for (def n in $items) {
        defer io.printf("  done %d\n", $n);   # runs at the end of each iteration
        io.printf("  work %d\n", $n);
    }
    io.printf("processed %d items\n", len($items));
}

process([1, 2]);
