// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// A runtime error raised inside a higher-order builtin's callback (here a
// Kind:"limit" depth error inside lists.map) must keep its kind and position
// when it crosses the builtin boundary, instead of being re-wrapped into a plain
// Kind:"runtime" error with a doubled "runtime error at ..." prefix.
func TestCallbackErrorPreservesKind(t *testing.T) {
	out, err := runAllLibs(t, `
		use io; use lists; use strings;
		func rec(n as int) { return rec($n + 1); }
		func viaMap(x as int) { return rec(0); }
		def kind as string init "none"; def msg as string init "";
		try { def r as list of int init lists.map([1], viaMap); }
		catch (e) { $kind = $e.kind; $msg = $e.message; }
		io.printf("%s %t", $kind, strings.contains($msg, "runtime error at"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "limit false" {
		t.Fatalf("callback limit error: got %q, want %q (kind preserved, no doubled prefix)", out, "limit|false")
	}
}

// An arity error raised when a user method is reached through a callback
// now carries a position (the callee's declaration) instead of none. (The file
// is the callee's source file in a real run; this harness parses anonymously, so
// only the line is asserted.)
func TestCallbackArityErrorHasPosition(t *testing.T) {
	out, err := runAllLibs(t, `
		use io; use lists;
		func needsTwo(a as int, b as int) { return $a; }
		def line as int init 0;
		try { def r as list of int init lists.map([1], needsTwo); }
		catch (e) { $line = $e.line; }
		io.printf("%t", $line > 0);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "true" {
		t.Fatalf("callback arity error lacks a position: got %q", out)
	}
}
