// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestConcurrentHostDispatchRaceFree is the race gate for concurrent handler
// dispatch. Many spawned workers each reach back into the shared entry-program
// interpreter through a module's meta.callMain, and the reached host method
// recurses (bumping the call-depth counter) and reads shared global state.
//
// Before the depth counter was made per-chain, the recursion inside each
// concurrently-dispatched host handler mutated one shared host-global
// callDepth, which `go test -race` flags as a data race. Run this package with
// -race; a clean pass is the gate. It also stands as a plain correctness check
// (every worker computes the same recursion result) without the detector.
func TestConcurrentHostDispatchRaceFree(t *testing.T) {
	dir := t.TempDir()

	// A module whose only job is to bounce a call back into the entry program:
	// the meta.callMain boundary that re-roots the handler frame at the shared
	// host global.
	modSrc := `use meta;
export func bounce(n as int) { return meta.callMain("hostWork", $n); }
`
	modPath := filepath.Join(dir, "depthmod.j")
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// The entry program spawns many workers; each dispatches into the module,
	// which dispatches back to hostWork, which recurses in the host (exercising
	// the call-depth counter) and reads a shared global constant + variable.
	prog := fmt.Sprintf(`use task;
use meta;
use testing;
import %q as dm;

def const BASE as int init 1000;
def seed as int init 7;
# A shared global map every concurrent handler reads by key: exercises the map
# hash-index read path (which must not lazily mutate the shared value) under
# concurrency.
def table as map of string to int init {"a": 5, "b": 6};

# Recurse in the HOST so each concurrent dispatch drives the call-depth counter.
# Also read shared globals (a const, a variable, and a map) on the way down - a
# concurrently-safe read pattern that must not race.
func hostWork(n as int) {
    if ($n <= 0) { return BASE + $seed + $table["b"] - $table["a"] - 1; }
    return hostWork($n - 1);
}

def tasks as list of task of int init [];
for (def i in 0..40) {
    $tasks[] = spawn { return dm.bounce(150); };
}
def results as list of int init task.waitAll($tasks);

# Every worker must compute the same answer, proving the shared host dispatch
# stayed correct under concurrency (not just race-clean).
for (def r in $results) {
    testing.assertEqual($r, 1007);
}
testing.assertEqual(len($results), 40);
`, modPath)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("concurrent host-dispatch program failed with code %d", code)
	}
}

// TestConcurrentHostDispatchGlobalWriteRaceFree is the race gate for concurrent
// handlers that WRITE a shared host global through meta.callMain. Unserialized
// this was a fatal `concurrent map writes` crash (and lost increments); host
// re-entry is now serialized per call chain, so every increment lands and the
// map write is safe. Run under -race; a clean pass plus the exact count is the
// gate.
func TestConcurrentHostDispatchGlobalWriteRaceFree(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "hitmod.j")
	if err := os.WriteFile(modPath, []byte(`use meta;
export func hit() { return meta.callMain("bump"); }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use task;
use testing;
import %q as m;
def hits as int init 0;
def cache as map of string to int init {};
func bump() { $hits = $hits + 1; $cache["k"] = $hits; return 0; }
def tasks as list of task of int init [];
for (def w in 0..8) {
    $tasks[] = spawn { for (def k in 0..500) { m.hit(); } return 0; };
}
task.waitAll($tasks);
testing.assertEqual($hits, 4000);
testing.assertEqual($cache["k"], 4000);
`, modPath)
	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("concurrent global-write program failed with code %d", code)
	}
}

// TestModuleBorrowValueSemantics proves a module method's never-written list
// parameter is a value copy, not an alias, even when the method re-enters the
// host via meta.callMain and the host mutates the original. The module borrow
// used to be unconditional, so the mutation leaked into the parameter.
func TestModuleBorrowValueSemantics(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "peekmod.j")
	if err := os.WriteFile(modPath, []byte(`use meta;
export func peek(p as list of int) {
    def before as int init $p[0];
    meta.callMain("bump");
    return $before * 1000 + $p[0];
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
import %q as m;
def xs as list of int init [1, 2, 3];
func bump() { $xs[0] = 99; return 0; }
testing.assertEqual(m.peek($xs), 1001);
`, modPath)
	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("module borrow value-semantics program failed with code %d", code)
	}
}

// TestSpawnCallbackUsesSnapshot proves a callback invoked inside a spawn body
// (here through lists.map) mutates the spawn's own globals snapshot, not the
// live globals: the main goroutine sees only its own writes, and the spawn sees
// only its own. Run under -race; it is also a plain value-semantics check.
func TestSpawnCallbackUsesSnapshot(t *testing.T) {
	dir := t.TempDir()
	prog := `use io;
use lists;
use task;
use testing;
def g as list of int init [0];
func touch(x as int) { $g[0] = $g[0] + 1; return $x; }
def f as func init touch;
def t as task of int init spawn {
    for (def k in 0..300) { def r as list of int init lists.map([1, 2, 3], $f); }
    return $g[0];
};
for (def k in 0..300) { $g[0] = $g[0] + 1; }
def spawnSaw as int init task.wait($t);
testing.assertEqual($g[0], 300);
testing.assertEqual($spawnSaw, 900);
`
	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("spawn-callback snapshot program failed with code %d", code)
	}
}
