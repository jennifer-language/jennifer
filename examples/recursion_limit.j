# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# Runaway recursion is a catchable error, not a crash. The interpreter caps the
# depth of nested calls, blocks, and expressions below the point where its own
# stack would overflow, and raises a "limit" error there - so even a recursion
# with a heavy frame (nested blocks plus a deep operand expression) is caught.
# The cap differs between the standard and the TinyGo build, so this prints the
# error kind, never the limit itself. It doubles as a release check that the
# binary's stack size and its depth cap still agree: a stack too small for the
# cap segfaults here instead of printing the catch.

use io;
use strings;

func dive(n as int) {
    if ($n >= 0) {
        while ($n >= 0) {
            if ($n % 2 >= 0) {
                return ((((($n + 1) * 2) - 1) + 3) // 2) + dive($n + 1);
            }
        }
    }
    return 0;
}

try {
    dive(0);
    io.printf("recursion returned (unexpected)\n");
} catch (e) {
    io.printf("caught a %s error\n", $e.kind);
    io.printf("mentions the call stack: %t\n", strings.contains($e.message, "call stack too deep"));
}
io.printf("still running after the catch\n");
