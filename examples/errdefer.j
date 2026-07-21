# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# `errdefer` schedules a call to run only when the enclosing block exits with a
# propagating error - the undo half of an acquire that must survive on success.
# This traces the canonical connect-handshake pattern without real I/O so the
# output is deterministic.
use io;

func acquire(name as string) { io.printf("acquire %s\n", $name); }
func release(name as string) { io.printf("release %s\n", $name); }

# On success the connection outlives the function (the caller owns it), so a
# plain `defer` would be wrong - `errdefer` releases only when the handshake
# throws partway through.
func connect(ok as bool) {
    acquire("socket");
    errdefer release("socket");        # runs only on an error exit
    if (not $ok) {
        throw Error{kind: "connect", message: "handshake failed", file: "", line: 0, col: 0};
    }
    io.printf("connected\n");
}

connect(true);                          # success: the socket stays open
try {
    connect(false);                     # failure: errdefer closes the socket
} catch (e) {
    io.printf("caught: %s\n", $e.message);
}

# defer and errdefer share one LIFO teardown; on error both kinds run.
func mixed() {
    defer release("always");
    errdefer release("only-on-error");
    throw Error{kind: "mixed", message: "boom", file: "", line: 0, col: 0};
}
try { mixed(); } catch (e) { io.printf("caught: %s\n", $e.message); }
