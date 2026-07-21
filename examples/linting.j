# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * One triggered-then-suppressed case per lint check.
 * `jennifer lint examples/linting.j` reports nothing: every finding below is
 * provoked on purpose and then silenced with a `# lint-disable: ID` comment.
 * The directive sits on the line the finding anchors to. Delete any directive
 * to watch that check fire.
 * @module linting
 */

use io;

# L101 unused-local - a local `def` that is never read.
func unusedLocal() {
    def stray as int init 1;   # lint-disable: L101
    return 0;
}

# L102 dead-code-after-terminator - a statement after return / throw / exit /
# break / continue. The directive goes on the unreachable statement.
func deadCode() {
    return 1;
    io.printf("unreachable\n");   # lint-disable: L102
}

# L103 empty-catch - a catch block that silently swallows the error. Anchored
# at the `catch` introducer, so that is where the directive goes.
func emptyCatch() {
    try {
        boom();
    } catch (e) {   # lint-disable: L103
    }
}

func boom() {
    throw Error{kind: "demo", message: "boom", file: "", line: 0, col: 0};
}

# L104 throw-non-error - throwing a bare value instead of an `Error` struct.
func throwNonError() {
    throw "raw string, not an Error";   # lint-disable: L104
}

# L105 constant-condition - an if / while test that is statically constant.
func constantCondition() {
    if (true) {   # lint-disable: L105
        return 1;
    }
    return 0;
}

# L201 method-too-long - a body over the 60-statement limit. The finding
# anchors at the method, so the directive lives on the `func` line.
func longMethod() {   # lint-disable: L201
    def a as int init 0;
    $a = 1; $a = 2; $a = 3; $a = 4; $a = 5; $a = 6; $a = 7; $a = 8;
    $a = 9; $a = 10; $a = 11; $a = 12; $a = 13; $a = 14; $a = 15; $a = 16;
    $a = 17; $a = 18; $a = 19; $a = 20; $a = 21; $a = 22; $a = 23; $a = 24;
    $a = 25; $a = 26; $a = 27; $a = 28; $a = 29; $a = 30; $a = 31; $a = 32;
    $a = 33; $a = 34; $a = 35; $a = 36; $a = 37; $a = 38; $a = 39; $a = 40;
    $a = 41; $a = 42; $a = 43; $a = 44; $a = 45; $a = 46; $a = 47; $a = 48;
    $a = 49; $a = 50; $a = 51; $a = 52; $a = 53; $a = 54; $a = 55; $a = 56;
    $a = 57; $a = 58; $a = 59; $a = 60;
    return $a;
}

# L202 nesting-too-deep - block nesting past depth 4. The finding anchors at
# the shallowest block that breaks the limit (here the fourth `if`), so the
# directive goes on that block's line. The tests use a parameter so they are
# not also constant conditions (L105).
func deepNesting(n as int) {
    if ($n > 0) {
        if ($n > 1) {
            if ($n > 2) {
                if ($n > 3) {   # lint-disable: L202
                    return 1;
                }
            }
        }
    }
    return 0;
}

# L203 line-too-long - a source line over the 100-column budget. The directive
# goes on the long line itself.
def const LONG as string init "this single string literal is padded out on purpose so the whole line runs comfortably past one hundred columns";   # lint-disable: L203
