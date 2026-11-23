// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package kv

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.52: two stores on one file (two processes: two registries) - the second
// flush finds the file changed and is refused with kind "kv", writing nothing
// and leaving its memory unchanged.
func TestLostUpdateRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "st.kv")
	r1 := &registry{stores: map[int64]*store{}}
	r2 := &registry{stores: map[int64]*store{}}
	a := call(t, r1.openFileFn, str(path))
	b := call(t, r2.openFileFn, str(path))
	call(t, r1.setFn, a, str("x"), str("1"), iv(0))
	written, _ := os.ReadFile(path)
	_, err := r2.setFn(ctx, []interpreter.Value{b, str("y"), str("2"), iv(0)})
	if err == nil {
		t.Fatal("second store's flush over a changed file: no error")
	}
	if k := dataerr.KindOf(err); k != "kv" {
		t.Errorf("kind %q, want kv (%v)", k, err)
	}
	if !strings.Contains(err.Error(), "since this store last read or wrote it") {
		t.Errorf("message: %v", err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(written) {
		t.Error("refused flush changed the file")
	}
	if got := call(t, r2.hasFn, b, str("y")); got.Bool {
		t.Error("refused set left y in memory")
	}
	// The first store keeps flushing over its own writes.
	call(t, r1.setFn, a, str("x"), str("2"), iv(0))
	call(t, r1.deleteFn, a, str("x"))
	call(t, r1.setFn, a, str("z"), str("3"), iv(0))
	// Reopened, the second store sees the first's content and may write.
	call(t, r2.closeFn, b)
	b = call(t, r2.openFileFn, str(path))
	if got := call(t, r2.getFn, b, str("z")); got.Str != "3" {
		t.Errorf("reopened get z = %q, want 3", got.Str)
	}
	call(t, r2.setFn, b, str("y"), str("2"), iv(0))
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Errorf("lock file left behind: %v", err)
	}
}

// A file deleted, or created, behind the store's back is refused too.
func TestLostUpdateDeletedOrCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "st.kv")
	r := &registry{stores: map[int64]*store{}}
	s := call(t, r.openFileFn, str(path))
	call(t, r.setFn, s, str("x"), str("1"), iv(0))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err := r.setFn(ctx, []interpreter.Value{s, str("y"), str("2"), iv(0)})
	if dataerr.KindOf(err) != "kv" || !strings.Contains(err.Error(), "was deleted") {
		t.Errorf("flush after delete: %v, want a kv error", err)
	}
	if _, serr := os.Stat(path); !os.IsNotExist(serr) {
		t.Error("refused flush recreated the file")
	}

	created := filepath.Join(dir, "new.kv")
	r2 := &registry{stores: map[int64]*store{}}
	c := call(t, r2.openFileFn, str(created))
	if err := os.WriteFile(created, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = r2.setFn(ctx, []interpreter.Value{c, str("y"), str("2"), iv(0)})
	if dataerr.KindOf(err) != "kv" || !strings.Contains(err.Error(), "was created") {
		t.Errorf("flush after another writer created the file: %v, want a kv error", err)
	}
}

// A lock left by a crashed writer is broken once stale; a live one makes the
// flush wait and then time out.
func TestWriteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "st.kv")
	oldWait, oldStale := lockWait, lockStale
	defer func() { lockWait, lockStale = oldWait, oldStale }()
	lockWait, lockStale = 100*time.Millisecond, time.Hour
	if err := os.WriteFile(path+".lock", []byte("other 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &registry{stores: map[int64]*store{}}
	s := call(t, r.openFileFn, str(path))
	call(t, r.setFn, s, str("x"), str("1"), iv(0)) // stamp 1 (1970) is stale
	if err := os.WriteFile(path+".lock", []byte("other "+itoa(time.Now().Unix())), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := r.setFn(ctx, []interpreter.Value{s, str("y"), str("2"), iv(0)})
	if dataerr.KindOf(err) != "timeout" {
		t.Errorf("flush under a live lock: %v, want kind timeout", err)
	}
	if b, _ := os.ReadFile(path + ".lock"); !strings.HasPrefix(string(b), "other ") {
		t.Error("a live lock was removed")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
