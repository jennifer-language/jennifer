// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package kv

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

var ctx = interpreter.BuiltinCtx{}

func str(s string) interpreter.Value { return interpreter.StringVal(s) }
func iv(n int64) interpreter.Value   { return interpreter.IntVal(n) }

func mustOpen(t *testing.T, r *registry) interpreter.Value {
	t.Helper()
	st, err := r.openFn(ctx, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return st
}

func call(t *testing.T, fn func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error), args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(ctx, args)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return v
}

// TestSetGetIncr covers the common path: store, read, increment, add-if-absent,
// touch, delete, and the memcache-style -1 from incr on a missing key.
func TestSetGetIncr(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	st := mustOpen(t, r)

	call(t, r.setFn, st, str("a"), str("1"), iv(0))
	if got := call(t, r.getFn, st, str("a")); got.Str != "1" {
		t.Errorf("get a = %q, want 1", got.Str)
	}
	if got := call(t, r.hasFn, st, str("a")); !got.Bool {
		t.Error("has a = false, want true")
	}
	if got := call(t, r.incrFn, st, str("a"), iv(5)); got.Int != 6 {
		t.Errorf("incr a = %d, want 6", got.Int)
	}
	// add: existing -> false, new -> true
	if got := call(t, r.addFn, st, str("a"), str("x"), iv(0)); got.Bool {
		t.Error("add existing = true, want false")
	}
	if got := call(t, r.addFn, st, str("b"), str("y"), iv(0)); !got.Bool {
		t.Error("add new = false, want true")
	}
	// touch present vs absent
	if got := call(t, r.touchFn, st, str("b"), iv(30)); !got.Bool {
		t.Error("touch b = false, want true")
	}
	if got := call(t, r.touchFn, st, str("gone"), iv(30)); got.Bool {
		t.Error("touch gone = true, want false")
	}
	// delete present then absent
	if got := call(t, r.deleteFn, st, str("a")); !got.Bool {
		t.Error("delete a = false, want true")
	}
	if got := call(t, r.deleteFn, st, str("a")); got.Bool {
		t.Error("delete a again = true, want false")
	}
	// get / incr on a missing key
	if got := call(t, r.getFn, st, str("missing")); got.Str != "" {
		t.Errorf("get missing = %q, want empty", got.Str)
	}
	if got := call(t, r.incrFn, st, str("missing"), iv(1)); got.Int != -1 {
		t.Errorf("incr missing = %d, want -1", got.Int)
	}
}

// TestExpiryEvicts drives the lazy-eviction path deterministically by writing an
// entry that is already expired and confirming get / has / delete treat it as
// absent.
func TestExpiryEvicts(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	st := mustOpen(t, r)
	s, err := r.resolve("test", st)
	if err != nil {
		t.Fatal(err)
	}
	s.data["exp"] = entry{value: "x", expires: time.Now().Add(-time.Second)}
	if got := call(t, r.getFn, st, str("exp")); got.Str != "" {
		t.Errorf("get expired = %q, want empty", got.Str)
	}
	if got := call(t, r.hasFn, st, str("exp")); got.Bool {
		t.Error("has expired = true, want false")
	}
	// a live TTL entry stays readable
	s.data["live"] = entry{value: "ok", expires: time.Now().Add(time.Hour)}
	if got := call(t, r.getFn, st, str("live")); got.Str != "ok" {
		t.Errorf("get live = %q, want ok", got.Str)
	}
}

// TestStoresIsolated: two stores from the same registry do not share keys, and a
// closed store's handle no longer resolves.
func TestStoresIsolated(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	a := mustOpen(t, r)
	b := mustOpen(t, r)
	call(t, r.setFn, a, str("k"), str("in-a"), iv(0))
	if got := call(t, r.getFn, b, str("k")); got.Str != "" {
		t.Errorf("store b saw store a's key: %q", got.Str)
	}
	call(t, r.closeFn, a)
	if _, err := r.getFn(ctx, []interpreter.Value{a, str("k")}); err == nil {
		t.Error("get on a closed store should error")
	}
}

// TestNonNumericIncr: incrementing a non-numeric value is a catchable error.
func TestNonNumericIncr(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	st := mustOpen(t, r)
	call(t, r.setFn, st, str("word"), str("hello"), iv(0))
	if _, err := r.incrFn(ctx, []interpreter.Value{st, str("word"), iv(1)}); err == nil {
		t.Error("incr on a non-numeric value should error")
	}
}

// TestIncrOverflowErrors: incr past int64 max errors rather than wrapping
// silently (matching Jennifer's overflow-errors stance).
func TestIncrOverflowErrors(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	st := mustOpen(t, r)
	call(t, r.setFn, st, str("big"), str("9223372036854775807"), iv(0)) // MaxInt64
	if _, err := r.incrFn(ctx, []interpreter.Value{st, str("big"), iv(1)}); err == nil {
		t.Error("incr past MaxInt64 should error, not wrap")
	}
	// still readable and unchanged after the rejected overflow
	if got := call(t, r.getFn, st, str("big")); got.Str != "9223372036854775807" {
		t.Errorf("value changed after rejected overflow: %q", got.Str)
	}
}

// TestFilePersistence: a file-backed store's contents survive across a fresh
// registry (simulating a new process / run).
func TestFilePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.dat")
	r1 := &registry{stores: map[int64]*store{}}
	st1, err := r1.openFileFn(ctx, []interpreter.Value{str(path)})
	if err != nil {
		t.Fatalf("openFile: %v", err)
	}
	call(t, r1.setFn, st1, str("user"), str("ada"), iv(0))
	call(t, r1.setFn, st1, str("n"), str("5"), iv(0))
	call(t, r1.incrFn, st1, str("n"), iv(2)) // -> 7, flushed

	// Reopen in a fresh registry: the file is loaded.
	r2 := &registry{stores: map[int64]*store{}}
	st2, err := r2.openFileFn(ctx, []interpreter.Value{str(path)})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := call(t, r2.getFn, st2, str("user")); got.Str != "ada" {
		t.Errorf("persisted user = %q, want ada", got.Str)
	}
	if got := call(t, r2.getFn, st2, str("n")); got.Str != "7" {
		t.Errorf("persisted n = %q, want 7", got.Str)
	}
}

// TestActiveExpirySweep: expired entries that are never accessed again are still
// evicted by the periodic sweep (so a flood of distinct short-lived keys cannot
// accumulate unbounded), while unexpired entries survive.
func TestActiveExpirySweep(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	st := mustOpen(t, r)
	s, err := r.resolve("t", st)
	if err != nil {
		t.Fatal(err)
	}
	// Insert already-expired entries directly; under lazy-only eviction these
	// would linger forever (nothing accesses them again).
	for i := 0; i < 100; i++ {
		s.data[fmt.Sprintf("old%d", i)] = entry{value: "x", expires: time.Now().Add(-time.Hour)}
	}
	// Do sweepInterval mutations with live keys to trigger one sweep.
	for i := 0; i < sweepInterval; i++ {
		call(t, r.setFn, st, str(fmt.Sprintf("live%d", i)), str("v"), iv(3600))
	}
	for i := 0; i < 100; i++ {
		if _, ok := s.data[fmt.Sprintf("old%d", i)]; ok {
			t.Fatalf("expired entry old%d survived the active sweep", i)
		}
	}
	if got := call(t, r.getFn, st, str("live0")); got.Str != "v" {
		t.Errorf("live entry evicted: %q", got.Str)
	}
}

// TestConcurrentIncr proves the per-store mutex makes incr atomic under
// concurrent access (8 goroutines x 500 = 4000). Run under -race in CI.
func TestConcurrentIncr(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	st := mustOpen(t, r)
	call(t, r.setFn, st, str("c"), str("0"), iv(0))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				if _, err := r.incrFn(ctx, []interpreter.Value{st, str("c"), iv(1)}); err != nil {
					t.Errorf("incr: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := call(t, r.getFn, st, str("c")); got.Str != "4000" {
		t.Errorf("concurrent incr total = %q, want 4000", got.Str)
	}
}

// TestConcurrentCrossProcessFlush simulates N separate processes (each its own
// registry, so no shared mutex) persisting the same file at once. Before the
// unique-temp fix these raced on a fixed ".tmp" sibling: one writer renamed it
// out from under another, crashing with `rename ...: no such file or directory`
// (and could publish a torn file). With a unique temp per writer the worst case
// is a lost write (last rename wins) - never an error, never a torn file. Run
// under -race.
func TestConcurrentCrossProcessFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.dat")
	const procs = 8
	const ops = 40
	var wg sync.WaitGroup
	errs := make(chan error, procs*ops+procs)
	for p := 0; p < procs; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			r := &registry{stores: map[int64]*store{}}
			stv, err := r.openFileFn(ctx, []interpreter.Value{str(path)})
			if err != nil {
				errs <- fmt.Errorf("proc %d openFile: %w", p, err)
				return
			}
			for i := 0; i < ops; i++ {
				if _, err := r.setFn(ctx, []interpreter.Value{stv, str(fmt.Sprintf("k%d-%d", p, i)), str("v"), iv(0)}); err != nil {
					errs <- fmt.Errorf("proc %d set: %w", p, err)
					return
				}
			}
		}(p)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent cross-process flush errored (a fixed temp path ENOENT-crashes here): %v", err)
	}
	// The file must still be a well-formed store afterwards (never torn).
	if _, err := loadFile(path); err != nil {
		t.Fatalf("file was corrupted by concurrent writers: %v", err)
	}
}
