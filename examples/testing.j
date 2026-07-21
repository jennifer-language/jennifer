# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `testing` library. Defines a
 * handful of test methods hitting every failure mode
 * (pass, user throw, runtime error, exit inside a test) and
 * renders the accumulated results in each of the three shipping
 * formats: human text, TAP, and JUnit XML.
 * Timings vary run to run; the example prints `ms=0` fields by
 * overriding the ms via testing.results()'s copy semantics-
 * well, not really: the accumulator returns a copy but we can't
 * mutate the copy back into anything. So we simply skip printing
 * the elapsed millisecond field to keep the golden output
 * deterministic.
 * @module testing
 */

use io;
use testing;

# The testing library's accumulator is process-wide. `reset` at
# the start of a suite makes re-runs (e.g. under `jennifer fmt`
# + rerun) start from a clean slate.
testing.reset();

# ---- pass path ----
func addPasses() {
    if (1 + 1 == 2) {
        return;
    }
    throw Error{
        kind: "assertion",
        message: "1 + 1 was not 2",
        file: "", line: 0, col: 0
    };
}

# ---- failure via user throw with an Error struct ----
func addFails() {
    throw Error{
        kind: "assertion",
        message: "expected 42, got 41",
        file: "", line: 0, col: 0
    };
}

# ---- failure via runtime error ----
func indexTooFar() {
    def xs as list of int init [];
    $xs[5];
}

# ---- exit inside a test (captured by the runner) ----
func earlyExit() {
    exit 7;
}

# ---- unknown method (via the runner) ----
# We ask the runner for a method that doesn't exist. This is what
# a real harness would encounter when a suite lists tests that
# were renamed or removed.

# Run them all. Each testing.run appends a testing.Result.
io.printf("running suite:\n");
testing.run("addPasses");
testing.run("addFails");
testing.run("indexTooFar");
testing.run("earlyExit");
testing.run("missingMethod");

def results as list of testing.Result init testing.results();
io.printf("  ran %d tests\n\n", len($results));

# ---- text report ----
io.printf("=== text report ===\n");
# Overwrite the ms field on every result to 0 so the golden file
# doesn't drift with wall-clock timings.
def normalised as list of testing.Result init [];
for (def r in $results) {
    # ms=0 so the golden doesn't drift with wall-clock timings.
    # file="" line=0 col=0 so the golden doesn't drift with the
    # absolute path of the source file.
    $normalised[] = testing.Result{
        name: $r.name,
        ms: 0,
        passed: $r.passed,
        errorKind: $r.errorKind,
        errorMessage: $r.errorMessage,
        file: "",
        line: 0,
        col: 0
    };
}
io.printf("%s", testing.report($normalised, "text"));

# ---- TAP report ----
io.printf("\n=== tap report ===\n");
io.printf("%s", testing.report($normalised, "tap"));

# ---- JUnit report ----
io.printf("\n=== junit report ===\n");
io.printf("%s", testing.report($normalised, "junit"));
