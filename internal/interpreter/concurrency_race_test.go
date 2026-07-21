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
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// A spawn nested inside a spawn body snapshots its captured scope on a
// background goroutine. If snapshotForSpawn iterated the live i.global (rather
// than the launching goroutine's own root frame), that iteration would race the
// main goroutine still assigning globals - Go's fatal "concurrent map iteration
// and map write". Run under `go test -race` this flags the regression; run
// plain it still asserts the program computes the right answer.
func TestNestedSpawnGlobalMutationNoRace(t *testing.T) {
	src := `
use io;
use task;
use lists;
def g as int init 0;
def outer as task of int init spawn {
    def inners as list of task of int init [];
    def k as int init 0;
    while ($k < 300) {
        $inners[] = spawn { return 1; };
        $k = $k + 1;
    }
    def sum as int init 0;
    for (def t in $inners) { $sum = $sum + task.wait($t); }
    return $sum;
};
def j as int init 0;
while ($j < 300) { $g = $g + 1; $j = $j + 1; }
def total as int init task.wait($outer);
io.printf("%d %d\n", $g, $total);
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	listslib.Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "300 300" {
		t.Errorf("got %q, want %q", got, "300 300")
	}
}

// A `def x as alias.Struct;` inside a method body (or spawn body) is shared AST
// reached from every goroutine that runs it. resolveDeclaredStructNS used to
// re-stamp that node's StructNS on every execution; run concurrently that is a
// write-write race on the shared type node. The one-time single-threaded stamp
// (resolveDeclaredTypesOnce) plus the Resolved marker makes every later resolve
// a read-only no-op, so 8 spawn workers each declaring `w.Point` in a loop is
// race-free under `go test -race`.
func TestConcurrentDeclaredStructNoRace(t *testing.T) {
	src := `
use io;
use widgets as w;
use task;
use lists;
func makePoints(n as int) {
    def pts as list of w.Point init [];
    def i as int init 0;
    while ($i < $n) {
        def p as w.Point init w.Point{ x: $i, y: $i };
        $pts[] = $p;
        $i = $i + 1;
    }
    return len($pts);
}
def tasks as list of task of int init [];
def wk as int init 0;
while ($wk < 8) {
    $tasks[] = spawn { return makePoints(500); };
    $wk = $wk + 1;
}
def results as list of int init task.waitAll($tasks);
def sum as int init 0;
for (def r in $results) { $sum = $sum + $r; }
io.printf("%d\n", $sum);
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in, buf := newWidgetInterp()
	tasklib.Install(in)
	listslib.Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "4000" {
		t.Errorf("got %q, want %q", got, "4000")
	}
}
