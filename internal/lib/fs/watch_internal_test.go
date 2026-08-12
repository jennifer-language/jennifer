// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package fslib

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestDiffSnap(t *testing.T) {
	prev := map[string]snapEntry{
		"/x":   {mtime: 5, isDir: true},
		"/x/a": {mtime: 1, isDir: false},
		"/x/b": {mtime: 2, isDir: false},
	}
	cur := map[string]snapEntry{
		"/x":   {mtime: 5, isDir: true},  // unchanged
		"/x/a": {mtime: 9, isDir: false}, // modified
		"/x/c": {mtime: 3, isDir: false}, // created
	}
	got := diffSnap(prev, cur)
	want := []watchEvent{
		{path: "/x/a", kind: "modified", isDir: false},
		{path: "/x/b", kind: "deleted", isDir: false},
		{path: "/x/c", kind: "created", isDir: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// No change -> no events.
	if e := diffSnap(cur, cur); len(e) != 0 {
		t.Errorf("identical snapshots should produce no events, got %+v", e)
	}
}

func TestScanTreeFileAndDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap := scanTree(dir)
	// root dir + a.txt + sub + sub/b.txt
	if len(snap) != 4 {
		t.Fatalf("scanTree got %d entries, want 4", len(snap))
	}
	if e, ok := snap[dir]; !ok || !e.isDir {
		t.Errorf("root should be present and marked a dir")
	}
	if e, ok := snap[filepath.Join(dir, "a.txt")]; !ok || e.isDir {
		t.Errorf("a.txt should be present and not a dir")
	}
	if e, ok := snap[filepath.Join(sub, "b.txt")]; !ok || e.isDir {
		t.Errorf("sub/b.txt should be present")
	}

	// A single file scans to exactly one entry.
	if single := scanTree(filepath.Join(dir, "a.txt")); len(single) != 1 {
		t.Fatalf("single-file scan got %d entries, want 1", len(single))
	}
	// A missing path scans to an empty snapshot (later appearance = "created").
	if missing := scanTree(filepath.Join(dir, "nope")); len(missing) != 0 {
		t.Fatalf("missing-path scan got %d entries, want 0", len(missing))
	}
}

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &watcher{root: dir, events: make(chan watchEvent, watchBuffer), done: make(chan struct{})}
	go w.run(20 * time.Millisecond)
	defer w.stop()

	// Let the initial snapshot settle, then modify the file.
	time.Sleep(60 * time.Millisecond)
	if err := os.WriteFile(target, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-w.events:
			if e.path == target && e.kind == "modified" {
				return // success
			}
		case <-deadline:
			t.Fatal("no modified event for f.txt within 3s")
		}
	}
}

func TestWatchBuiltinSurface(t *testing.T) {
	ResetWatchersForTest()
	defer ResetWatchersForTest()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	v, err := watchFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(dir), interpreter.IntVal(20)})
	if err != nil {
		t.Fatalf("fs.watch: %v", err)
	}
	if !isWatcher(v) {
		t.Fatalf("fs.watch did not return an fs.Watcher: %+v", v)
	}

	// Make a change and wait for the poller to enqueue an event.
	time.Sleep(60 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	has := false
	for i := 0; i < 200 && !has; i++ {
		hv, herr := watchHasEventFn(interpreter.BuiltinCtx{}, []Value{v})
		if herr != nil {
			t.Fatal(herr)
		}
		if has = hv.Bool; !has {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !has {
		t.Fatal("fs.hasEvent never became true after a change")
	}

	ev, err := watchNextFn(interpreter.BuiltinCtx{}, []Value{v})
	if err != nil {
		t.Fatalf("fs.next: %v", err)
	}
	if ev.Kind != interpreter.KindStruct || ev.StructName != "Event" {
		t.Fatalf("fs.next did not return an fs.Event: %+v", ev)
	}

	// fs.close stops it (polymorphic), and fs.next then errors rather than blocks.
	if _, err := closeFn(interpreter.BuiltinCtx{}, []Value{v}); err != nil {
		t.Fatalf("fs.close(watcher): %v", err)
	}
	if _, err := watchNextFn(interpreter.BuiltinCtx{}, []Value{v}); err == nil {
		t.Error("fs.next on a closed watcher should error, not block")
	}
}
