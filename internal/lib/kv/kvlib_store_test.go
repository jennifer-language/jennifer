// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package kv

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestSweepIntervalScalesWithStore: a store with many live entries is not swept
// every sweepInterval writes (that made n writes cost O(n^2)); it waits len/2.
func TestSweepIntervalScalesWithStore(t *testing.T) {
	s := &store{data: map[string]entry{}}
	now := time.Now()
	for i := 0; i < 4096; i++ {
		s.data[strconv.Itoa(i)] = entry{value: "v"}
	}
	s.data["old"] = entry{value: "v", expires: now.Add(-time.Second)}
	for i := 0; i < sweepInterval; i++ {
		s.maybeSweepLocked(now)
	}
	if _, ok := s.data["old"]; !ok {
		t.Fatalf("swept after %d writes on a %d-entry store; want the interval to scale with the store", sweepInterval, len(s.data))
	}
	for i := sweepInterval; i < len(s.data)/2; i++ {
		s.maybeSweepLocked(now)
	}
	if _, ok := s.data["old"]; ok {
		t.Error("expired entry still present after len/2 writes")
	}
	// A small store still sweeps every sweepInterval writes.
	small := &store{data: map[string]entry{"old": {value: "v", expires: now.Add(-time.Second)}}}
	for i := 0; i < sweepInterval; i++ {
		small.maybeSweepLocked(now)
	}
	if _, ok := small.data["old"]; ok {
		t.Error("small store not swept after sweepInterval writes")
	}
}

// TestOpenFileSamePathSharesStore: two openFile calls on one file (under two
// spellings) in one run share a store, so neither overwrites the other's write
// from a stale snapshot; the store stays open until every handle is closed.
func TestOpenFileSamePathSharesStore(t *testing.T) {
	r := &registry{stores: map[int64]*store{}}
	dir := t.TempDir()
	path := filepath.Join(dir, "st.kv")
	a := call(t, r.openFileFn, str(path))
	b := call(t, r.openFileFn, str(filepath.Join(dir, ".", "st.kv")))
	call(t, r.setFn, a, str("x"), str("1"), iv(0))
	call(t, r.setFn, b, str("y"), str("2"), iv(0))
	call(t, r.closeFn, a)
	if got := call(t, r.getFn, b, str("x")); got.Str != "1" {
		t.Errorf("get x via b = %q, want 1", got.Str)
	}
	call(t, r.closeFn, b)
	r2 := &registry{stores: map[int64]*store{}}
	c := call(t, r2.openFileFn, str(path))
	for k, want := range map[string]string{"x": "1", "y": "2"} {
		if got := call(t, r2.getFn, c, str(k)); got.Str != want {
			t.Errorf("reopened get %s = %q, want %q", k, got.Str, want)
		}
	}
	if _, err := r.setFn(ctx, []interpreter.Value{b, str("z"), str("3"), iv(0)}); err == nil {
		t.Error("set after every handle closed: want a closed-store error")
	}
}
