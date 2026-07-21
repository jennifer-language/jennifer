// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	listslib "jennifer-lang.dev/jennifer/internal/lib/lists"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// allocsTable runs src under the allocation profiler and returns the rendered
// --allocs table. The eager-copy section prints "copies across N sites" only
// when at least one binding site actually deep-copied a compound value.
func allocsTable(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	listslib.Install(in)
	col := profile.NewCollector(profile.ModeAllocs, 0)
	in.SetProfiler(col, false, false, true)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out bytes.Buffer
	col.Table(&out)
	return out.String()
}

// A fresh list / map / struct literal is already private (its evaluator copies
// every element), so binding it does no redundant whole-value deep copy: the
// allocation profile shows zero eager copies.
func TestLiteralBindingSkipsEagerCopy(t *testing.T) {
	out := allocsTable(t, `
def xs as list of int init [1, 2, 3];
def m as map of string to int init {"a": 1, "b": 2};
def ys as list of list of int init [[1], [2, 3]];
`)
	if !strings.Contains(out, "Eager copies") {
		t.Fatalf("allocs table missing eager-copy section:\n%s", out)
	}
	if strings.Contains(out, "copies across") {
		t.Errorf("literal bindings should record no eager copies, but some were counted:\n%s", out)
	}
}

// An aliasing RHS (a variable read) can hand back a reference into a live
// binding, so it is still eager-copied - value semantics intact. This is the
// counterpart that proves the literal skip is not just suppressing the counter.
func TestAliasingBindingStillEagerCopies(t *testing.T) {
	out := allocsTable(t, `
def a as list of int init [1, 2, 3];
def b as list of int init $a;
`)
	if !strings.Contains(out, "copies across") {
		t.Errorf("an aliasing `def b init $a` must eager-copy, but none was counted:\n%s", out)
	}
}

// The value-semantics guarantee the skip relies on: mutating a binding filled
// from a literal, and a binding aliased from another variable, never leak into
// each other.
func TestLiteralAndAliasBindingsStayIndependent(t *testing.T) {
	prog, err := parser.Parse(`
use io;
use lists;
def a as list of int init [1, 2, 3];
def b as list of int init $a;
$b[0] = 99;
$a[] = 4;
io.printf("%v %v\n", $a, $b);
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	listslib.Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[1, 2, 3, 4] [99, 2, 3]" {
		t.Errorf("aliasing leaked between bindings: got %q", got)
	}
}
