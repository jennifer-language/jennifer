// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
