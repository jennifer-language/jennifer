# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

# First-class function targets: passed by bare name as `func` values to the
# higher-order `lists` layer and called through a variable.
func dbl(n as int) {
    return $n * 2;
}

func isEven(n as int) {
    return $n % 2 == 0;
}

func addup(a as int, b as int) {
    return $a + $b;
}

# `defer` runs its calls in LIFO order as the block exits, on every path.
func deferDemo() {
    defer io.printf("  runs last (registered first)\n");
    defer io.printf("  runs first (registered last)\n");
    io.printf("  body runs before any defer\n");
}
