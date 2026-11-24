// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.45: fs.watchWith takes the poll interval in an fs.WatchOptions.

func watchOpts(ms int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "WatchOptions", []interpreter.StructField{
		{Name: "intervalMs", Value: interpreter.IntVal(ms)},
	})
}

// TestWatchWithDeliversEvents: a 20 ms interval reports a change, and the
// zero options (the 300 ms default) start a watcher too.
func TestWatchWithDeliversEvents(t *testing.T) {
	ResetWatchersForTest()
	defer ResetWatchersForTest()
	dir := t.TempDir()
	target := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := watchWithFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(dir), watchOpts(20)})
	if err != nil {
		t.Fatalf("fs.watchWith: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := os.WriteFile(target, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := make(chan Value, 1)
	go func() {
		ev, _ := watchNextFn(interpreter.BuiltinCtx{}, []Value{v})
		got <- ev
	}()
	select {
	case ev := <-got:
		if ev.Kind != interpreter.KindStruct || ev.StructName != "Event" {
			t.Fatalf("fs.next: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3 s")
	}
	if _, err := closeFn(interpreter.BuiltinCtx{}, []Value{v}); err != nil {
		t.Fatal(err)
	}
	zero := interpreter.NamespacedStructVal(LibraryName, "WatchOptions", nil)
	w, err := watchWithFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(dir), zero})
	if err != nil || !isWatcher(w) {
		t.Fatalf("zero options: %v, %v", w, err)
	}
	if _, err := watchWithFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(dir), watchOpts(0)}); err != nil {
		t.Fatalf("intervalMs 0: %v", err)
	}
}

// TestWatchWithRejects: an interval below 20 ms (negative too) or too large for
// a duration, a wrong options value and a wrong argument count are errors that
// register no watcher.
func TestWatchWithRejects(t *testing.T) {
	ResetWatchersForTest()
	defer ResetWatchersForTest()
	dir := interpreter.StringVal(t.TempDir())
	for _, c := range []struct {
		args []Value
		frag string
	}{
		{[]Value{dir, watchOpts(19)}, "fs.watchWith: intervalMs must be >= 20, got 19"},
		{[]Value{dir, watchOpts(-1)}, "fs.watchWith: intervalMs must be >= 20, got -1"},
		{[]Value{dir, watchOpts(18014398509481984)}, "fs.watchWith: intervalMs 18014398509481984 is too large"},
		{[]Value{dir, interpreter.IntVal(100)}, "fs.watchWith: options must be an fs.WatchOptions"},
		{[]Value{dir}, "fs.watchWith expects 2 arguments"},
	} {
		_, err := watchWithFn(interpreter.BuiltinCtx{}, c.args)
		if err == nil || !strings.Contains(err.Error(), c.frag) {
			t.Errorf("err = %v, want %q", err, c.frag)
		}
	}
	watchMu.Lock()
	n := len(watchers)
	watchMu.Unlock()
	if n != 0 {
		t.Errorf("%d watchers registered by rejected calls", n)
	}
}
