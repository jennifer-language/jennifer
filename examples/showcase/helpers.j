# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Imported by ../showcase.j to demonstrate file imports.
 * Two small methods that the showcase calls. Lives in a subdirectory so
 * the examples_test.go walker (which only scans top-level *.j files)
 * doesn't try to run it as a standalone program.
 * @module helpers
 */

func fact(n as int) {
    if ($n <= 1) {
        return 1;
    }
    return $n * fact($n - 1);
}

func greet(who as string) {
    return "Hi, " + $who + "!";
}
