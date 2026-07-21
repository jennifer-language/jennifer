# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The try / catch / throw walkthrough.
 * `try { body } catch (NAME) { handler }` runs the body; if anything
 * inside it throws (user-issued `throw EXPR;` or a runtime error like
 * out-of-bounds), the handler runs with NAME bound to the thrown
 * value. Convention is to throw an `Error` struct - the runtime
 * auto-defines that struct shape so user code can rely on it.
 * @module trycatch
 */

use io;
use convert;

# --- user-issued throw, caught and dispatched on `kind` ---
io.printf("=== user throw ===\n");

/**
 * Validate a `key=value` config line.
 * @param src {string} the config line to check
 * @return {string} src unchanged, when it is valid
 * @throws {Error} kind "parse_error" when src has no `=`
 */
func parseConfig(src as string) {
    if (not strings.contains($src, "=")) {
        throw Error{
            kind: "parse_error",
            message: "missing `=`",
            file: "",
            line: 0,
            col: 0
        };
    }
    return $src;
}

use strings;

try {
    parseConfig("no equals here");
} catch (err) {
    if ($err.kind == "parse_error") {
        io.printf("config invalid: %s\n", $err.message);
    } else {
        throw $err;
    }
}

# --- runtime error caught uniformly ---
#
# Runtime errors (out-of-bounds, missing map keys, division by zero,
# bad type) are wrapped into the same `Error` struct shape and bound
# to the catch variable. Their `kind` is `"runtime"` until the
# originating site opts in to a more specific tag.
io.printf("=== runtime error ===\n");
def xs as list of int init [10, 20];
try {
    def bad as int init $xs[99];
} catch (err) {
    io.printf("caught kind=%s msg=%s\n", $err.kind, $err.message);
}

# --- throwing a non-struct value still works ---
io.printf("=== throw any value ===\n");
try {
    throw "raw string error";   # lint-disable: L104
} catch (err) {
    io.printf("caught %s of kind %s\n", $err, convert.typeOf($err));
}

# --- re-throw inside catch propagates to outer try ---
io.printf("=== re-throw ===\n");
try {
    try {
        throw Error{kind: "inner", message: "boom", file: "", line: 0, col: 0};
    } catch (err) {
        io.printf("inner saw %s\n", $err.kind);
        throw $err;
    }
} catch (err) {
    io.printf("outer saw %s\n", $err.kind);
}

# --- value semantics: mutating the catch binding does not mutate the source ---
io.printf("=== value semantics ===\n");
def source as Error init Error{kind: "k", message: "m", file: "", line: 0, col: 0};
try {
    throw $source;
} catch (caught) {
    $caught.kind = "mutated";
}
io.printf("source.kind = %s\n", $source.kind);

io.printf("=== done ===\n");
