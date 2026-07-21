// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	listslib "jennifer-lang.dev/jennifer/internal/lib/lists"
	mapslib "jennifer-lang.dev/jennifer/internal/lib/maps"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runMap runs src with io/maps/lists installed and returns stdout.
func runMap(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	mapslib.Install(in)
	listslib.Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

// runMapErr runs src and returns its run error (nil on success) without failing
// the test - for asserting that a program is rejected.
func runMapErr(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	mapslib.Install(in)
	listslib.Install(in)
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// The index must not change map semantics: insertion order is preserved whether
// the map is built by literal or by keyed writes, an update lands in place (no
// reorder), and reads return the right values.
func TestMapIndexPreservesOrderAndValues(t *testing.T) {
	// Built by writes into an empty map (exercises the lazy index + append).
	out := runMap(t, `
use io;
def m as map of string to int;
$m["a"] = 1;
$m["b"] = 2;
$m["c"] = 3;
$m["b"] = 20;
io.printf("%v %d\n", $m, $m["b"]);
`)
	if got := strings.TrimSpace(out); got != `{"a": 1, "b": 20, "c": 3} 20` {
		t.Errorf("built-by-writes: got %q", got)
	}
	// Built by literal (exercises evalMapLit index), then read + update.
	out = runMap(t, `
use io;
def m as map of string to int init {"x": 1, "y": 2, "z": 3};
$m["y"] = 99;
io.printf("%v %d %d\n", $m, $m["x"], $m["z"]);
`)
	if got := strings.TrimSpace(out); got != `{"x": 1, "y": 99, "z": 3} 1 3` {
		t.Errorf("built-by-literal: got %q", got)
	}
}

// A missing hashable key is still a positioned, catchable error (the O(1) miss
// path must report exactly like the linear scan did).
func TestMapIndexMissStillErrors(t *testing.T) {
	out := runMap(t, `
use io;
def m as map of string to int init {"a": 1};
try { def x as int init $m["nope"]; } catch (e) { io.printf("caught: %s\n", $e.message); }
`)
	if !strings.Contains(out, "map has no entry for key") {
		t.Errorf("miss did not error as before: %q", out)
	}
}

// Value semantics: an eager-copied map and its source have independent indexes,
// so mutating one never leaks a key into the other.
func TestMapIndexValueSemanticsIndependent(t *testing.T) {
	out := runMap(t, `
use io;
use maps;
def a as map of string to int init {"x": 1};
def b as map of string to int init $a;
$b["y"] = 2;
$a["z"] = 3;
io.printf("%t %t %t %t\n", maps.has($a, "y"), maps.has($a, "z"), maps.has($b, "z"), maps.has($b, "y"));
`)
	// a has z not y; b has y not z.
	if got := strings.TrimSpace(out); got != "false true false true" {
		t.Errorf("aliasing leaked through the index: %q", got)
	}
}

// A duplicate key in a map literal is a positioned error: a two-entry map where
// only the first is reachable is a corrupt value, not a well-formed one.
func TestMapLiteralDuplicateKeyErrors(t *testing.T) {
	if _, err := runMapErr(t, `def m as map of string to int init {"a": 1, "a": 2};`); err == nil {
		t.Error("expected a duplicate-key error for a string-keyed literal")
	}
	if _, err := runMapErr(t, `def m as map of int to int init {1: 1, 2: 2, 1: 3};`); err == nil {
		t.Error("expected a duplicate-key error for an int-keyed literal")
	}
	// Distinct keys still build fine.
	out := runMap(t, `use io; def m as map of string to int init {"a": 1, "b": 2}; io.printf("%d", $m["b"]);`)
	if strings.TrimSpace(out) != "2" {
		t.Errorf("distinct keys: got %q, want 2", strings.TrimSpace(out))
	}
}

// A map keyed by a non-hashable type (a list) is never indexed and must keep
// working through the linear scan.
func TestMapIndexNonHashableKeyLinearScans(t *testing.T) {
	out := runMap(t, `
use io;
def m as map of list of int to string;
$m[[1, 2]] = "a";
$m[[3, 4]] = "b";
$m[[1, 2]] = "A";
io.printf("%s %s\n", $m[[1, 2]], $m[[3, 4]]);
`)
	if got := strings.TrimSpace(out); got != "A b" {
		t.Errorf("non-hashable (list) keys: got %q", got)
	}
}

// Correctness at scale: many distinct keys build, read back, and update
// correctly. If the index desynced, a read would return the wrong slot.
func TestMapIndexManyKeysConsistent(t *testing.T) {
	out := runMap(t, `
use io;
def m as map of int to int;
def i as int init 0;
while ($i < 5000) { $m[$i] = $i * 2; $i = $i + 1; }
# update every even key
$i = 0;
while ($i < 5000) { $m[$i] = $i * 3; $i = $i + 2; }
def sum as int init 0;
$i = 0;
while ($i < 5000) { $sum = $sum + $m[$i]; $i = $i + 1; }
io.printf("%d %d\n", len($m), $sum);
`)
	// evens 0,2,..4998 -> i*3 ; odds -> i*2.
	// sum = sum over i of (i*3 if even else i*2)
	sum := 0
	for i := 0; i < 5000; i++ {
		if i%2 == 0 {
			sum += i * 3
		} else {
			sum += i * 2
		}
	}
	want := "5000 " + strconv.Itoa(sum)
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("many-keys: got %q, want %q", got, want)
	}
}
