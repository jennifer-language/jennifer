# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Demonstrate the external-program execution API.
 * Covers `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill` and the
 * library-provided namespaced structs that underpin them (`os.Result`,
 * `os.Process`). Not part of the golden-file test suite; does NOT run under
 * `jennifer-tiny` (TinyGo has no os/exec) - use the default `jennifer` binary.
 * @module exec
 */

use io;
use os;

# ---- Blocking run ---------------------------------------------------

# os.run takes an argv-style list and blocks until the child exits.
# It returns an os.Result struct with the exit code and the captured
# stdout and stderr streams. The struct is a library-provided
# namespaced struct - you write `as os.Result` and
# `os.Result{...}` the same way you write `as Point` for a
# user-defined struct.
def r as os.Result init os.run(["echo", "hello from os.run"]);
io.printf("=== os.run ===\n");
io.printf("exit code: %d\n", $r.exitCode);
io.printf("stdout:    %s", $r.stdout);

# Non-zero exit codes are values, NOT errors. Programs branch on
# $result.exitCode instead of wrapping every call in a try/catch.
def bad as os.Result init os.run(["sh", "-c", "echo to-out; echo to-err 1>&2; exit 3"]);
io.printf("=== non-zero exit ===\n");
io.printf("exit code: %d\n", $bad.exitCode);
io.printf("stdout:    %s", $bad.stdout);
io.printf("stderr:    %s", $bad.stderr);

# ---- Non-blocking spawn + wait --------------------------------------

# os.spawn returns immediately with an opaque os.Process handle.
# Drain the streams later with os.wait (blocking) or check
# completion with os.poll (non-blocking).
def p as os.Process init os.spawn(["sh", "-c", "echo from spawned child"]);
def wr as os.Result init os.wait($p);
io.printf("=== os.spawn + os.wait ===\n");
io.printf("exit code: %d\n", $wr.exitCode);
io.printf("stdout:    %s", $wr.stdout);

# os.wait is idempotent - calling it again on the same handle
# returns the same os.Result without re-waiting.
def again as os.Result init os.wait($p);
io.printf("idempotent wait: stdout matches = %t\n", $again.stdout == $wr.stdout);

# ---- Kill ------------------------------------------------------------

# os.kill sends SIGTERM to a spawned child. The exit code is < 0
# when a process was terminated by a signal (the OS reports this
# convention; Jennifer surfaces it verbatim).
def long as os.Process init os.spawn(["sleep", "30"]);
os.kill($long);
def killed as os.Result init os.wait($long);
io.printf("=== os.kill ===\n");
io.printf("exit code is negative (signal): %t\n", $killed.exitCode < 0);

# ---- Polling --------------------------------------------------------

# os.poll is a pure predicate - true once the child has exited and
# a following os.wait would return immediately. We've already
# waited on $long, so it polls true.
io.printf("=== os.poll ===\n");
io.printf("already-waited process polls true: %t\n", os.poll($long));
