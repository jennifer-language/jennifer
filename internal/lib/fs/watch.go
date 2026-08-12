// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package fslib

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Polling filesystem watch. Deliberately mtime-polling (pure stdlib,
// TinyGo-clean, no dependency), matching what `jennifer serve --watch` does,
// rather than inotify / fsnotify (a dependency that does not build under
// TinyGo). One background goroutine per watcher re-scans the tree every
// `interval`, diffs a path->mtime snapshot, and pushes change events onto a
// channel a `fs.next` pull loop drains - the same handle-into-a-registry +
// pull-loop shape as fs.File and httpd, so a `spawn`ed loop can react to changes
// while the program does other work. Bursts within one interval (an editor's
// write / rename / chmod on save) coalesce into the snapshot diff, so a save is
// one event, not three.

const (
	defaultWatchIntervalMs = 300
	minWatchIntervalMs     = 20
	// watchBuffer bounds queued events; a slow consumer applies backpressure to
	// the poll goroutine, which then coalesces further changes into the next
	// snapshot diff (never losing the net change).
	watchBuffer = 256
	// maxWatchers bounds live watchers so a leak (watch without a matching
	// fs.close) surfaces a catchable error rather than growing without bound.
	maxWatchers = 1024
)

type watchEvent struct {
	path  string
	kind  string // "created" | "modified" | "deleted"
	isDir bool
}

// snapEntry is one path's polled state.
type snapEntry struct {
	mtime int64
	isDir bool
}

type watcher struct {
	root      string
	events    chan watchEvent
	done      chan struct{}
	closeOnce sync.Once
}

func (w *watcher) stop() {
	w.closeOnce.Do(func() { close(w.done) })
}

// The registry: integer id -> live watcher. Guarded by watchMu so spawn tasks
// share it safely. Package-global (survives across interpreters), mirroring the
// fs.File handle registry.
var (
	watchMu     sync.Mutex
	watchers    = map[int64]*watcher{}
	nextWatchID int64
)

func installWatch(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "watch", watchFn)
	in.RegisterNamespaced(LibraryName, "next", watchNextFn)
	in.RegisterNamespaced(LibraryName, "hasEvent", watchHasEventFn)
	// fs.close (handles.go) stops a watcher too - it dispatches on the struct.
}

// ResetWatchersForTest stops all watchers and clears the registry between tests.
// Exported for the `_test` package; not part of the user-facing surface.
func ResetWatchersForTest() {
	watchMu.Lock()
	defer watchMu.Unlock()
	for _, w := range watchers {
		w.stop()
	}
	watchers = map[int64]*watcher{}
	nextWatchID = 0
}

// makeWatcher builds the Jennifer-side `fs.Watcher{id}` value.
func makeWatcher(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Watcher", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// isWatcher reports whether v is an fs.Watcher value (for polymorphic fs.close).
func isWatcher(v Value) bool {
	return v.Kind == interpreter.KindStruct && v.StructNS == LibraryName && v.StructName == "Watcher"
}

// extractWatcherID pulls the integer id out of an `fs.Watcher{...}` value.
func extractWatcherID(fnName string, v Value) (int64, error) {
	if !isWatcher(v) {
		return 0, fmt.Errorf("%s: argument must be an fs.Watcher, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: fs.Watcher.id is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: fs.Watcher has no id field", fnName)
}

// makeEvent builds the Jennifer-side `fs.Event{path, kind, isDir}` value.
func makeEvent(e watchEvent) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Event", []interpreter.StructField{
		{Name: "path", Value: interpreter.StringVal(e.path)},
		{Name: "kind", Value: interpreter.StringVal(e.kind)},
		{Name: "isDir", Value: interpreter.BoolVal(e.isDir)},
	})
}

func watchFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.watch expects 1 or 2 arguments (path[, intervalMs]), got %d", len(args))
	}
	path, err := takeStringArg("fs.watch", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	intervalMs := int64(defaultWatchIntervalMs)
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindInt {
			return interpreter.Null(), fmt.Errorf("fs.watch: intervalMs must be int, got %s", args[1].Kind)
		}
		intervalMs = args[1].Int
		if intervalMs < minWatchIntervalMs {
			return interpreter.Null(), fmt.Errorf("fs.watch: intervalMs must be >= %d, got %d", minWatchIntervalMs, intervalMs)
		}
	}

	watchMu.Lock()
	if len(watchers) >= maxWatchers {
		watchMu.Unlock()
		return interpreter.Null(), fmt.Errorf("fs.watch: too many open watchers (limit %d) - close some with fs.close", maxWatchers)
	}
	nextWatchID++
	id := nextWatchID
	w := &watcher{
		root:   path,
		events: make(chan watchEvent, watchBuffer),
		done:   make(chan struct{}),
	}
	watchers[id] = w
	watchMu.Unlock()

	go w.run(time.Duration(intervalMs) * time.Millisecond)
	return makeWatcher(id), nil
}

// resolveWatcher returns the live watcher for an id, or a positioned error if
// the id is unknown (typical cause: use after close).
func resolveWatcher(fnName string, id int64) (*watcher, error) {
	watchMu.Lock()
	defer watchMu.Unlock()
	w, ok := watchers[id]
	if !ok {
		return nil, fmt.Errorf("%s: fs.Watcher id %d is not open (already closed, or never opened)", fnName, id)
	}
	return w, nil
}

func watchNextFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.next expects 1 argument (fs.Watcher), got %d", len(args))
	}
	id, err := extractWatcherID("fs.next", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	w, err := resolveWatcher("fs.next", id)
	if err != nil {
		return interpreter.Null(), err
	}
	// Block until the next change, or until the watcher is closed (so a `spawn`
	// pull loop can be released by fs.close from another task).
	select {
	case e := <-w.events:
		return makeEvent(e), nil
	case <-w.done:
		return interpreter.Null(), fmt.Errorf("fs.next: fs.Watcher id %d was closed", id)
	}
}

func watchHasEventFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.hasEvent expects 1 argument (fs.Watcher), got %d", len(args))
	}
	id, err := extractWatcherID("fs.hasEvent", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	w, err := resolveWatcher("fs.hasEvent", id)
	if err != nil {
		return interpreter.Null(), err
	}
	// A non-consuming check: len() of a buffered channel is the queued count.
	return interpreter.BoolVal(len(w.events) > 0), nil
}

// closeWatcher stops a watcher and removes it from the registry. Called by the
// polymorphic fs.close (handles.go).
func closeWatcher(id int64) error {
	watchMu.Lock()
	w, ok := watchers[id]
	if !ok {
		watchMu.Unlock()
		return fmt.Errorf("fs.close: fs.Watcher id %d is not open (already closed?)", id)
	}
	delete(watchers, id)
	watchMu.Unlock()
	w.stop()
	return nil
}

// run is the poll loop: every `interval`, re-scan the tree, diff against the
// last snapshot, and deliver the change events.
func (w *watcher) run(interval time.Duration) {
	snap := scanTree(w.root)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			cur := scanTree(w.root)
			for _, e := range diffSnap(snap, cur) {
				select {
				case w.events <- e:
				case <-w.done:
					return
				}
			}
			snap = cur
		}
	}
}

// scanTree returns a path->state snapshot of `root`: a single file, or every
// entry under a directory (recursive; symlinks are not followed). A missing /
// unreadable path yields an empty map, so its later appearance reads as
// "created" and its removal as "deleted".
func scanTree(root string) map[string]snapEntry {
	out := map[string]snapEntry{}
	info, err := os.Lstat(root)
	if err != nil {
		return out
	}
	if !info.IsDir() {
		out[root] = snapEntry{mtime: info.ModTime().UnixNano(), isDir: false}
		return out
	}
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, werr error) error {
		if werr != nil || fi == nil {
			return nil // skip an unreadable entry, keep walking
		}
		out[p] = snapEntry{mtime: fi.ModTime().UnixNano(), isDir: fi.IsDir()}
		return nil
	})
	return out
}

// diffSnap compares two snapshots and returns the change events, sorted by path
// for deterministic delivery order within a tick.
func diffSnap(prev, cur map[string]snapEntry) []watchEvent {
	var events []watchEvent
	for p, ce := range cur {
		if pe, ok := prev[p]; !ok {
			events = append(events, watchEvent{path: p, kind: "created", isDir: ce.isDir})
		} else if pe.mtime != ce.mtime {
			events = append(events, watchEvent{path: p, kind: "modified", isDir: ce.isDir})
		}
	}
	for p, pe := range prev {
		if _, ok := cur[p]; !ok {
			events = append(events, watchEvent{path: p, kind: "deleted", isDir: pe.isDir})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].path < events[j].path })
	return events
}
