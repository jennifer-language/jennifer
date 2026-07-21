// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package profile collects and renders execution profiles for Jennifer
// programs. The interpreter, when handed a *Collector, calls its Record*
// methods at instrumented eval sites; the Collector aggregates them and can
// render a flat table, a pprof-compatible profile, or a Chrome-trace stream.
//
// It attributes work back to Jennifer source positions (file:line:col) - the
// gap left by `go tool pprof`, which profiles the interpreter binary, not the
// .j program running inside it. Two modes: the default statement/call profile
// (hit counts + wall-clock time per position) and an allocation profile
// (--allocs) that surfaces the eager deep copies and spawn-frame deep copies
// that value semantics turn into real work.
package profile

import (
	"bytes"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Mode selects what a run records.
type Mode int

const (
	// ModeStatement records per-position hit counts and wall-clock time
	// (self and cumulative) plus a method-call timeline for the trace form.
	ModeStatement Mode = iota
	// ModeAllocs records value-semantics work: eager deep copies and
	// spawn-frame deep copies, per position.
	ModeAllocs
)

// posKey identifies a source position for aggregation.
type posKey struct {
	file string
	line int
	col  int
}

// stmtSample aggregates one source position's statement-timing data.
type stmtSample struct {
	posKey
	hits int64
	self time.Duration // time in this statement excluding nested statements
	cum  time.Duration // time in this statement including nested statements
}

// eventSample aggregates a counted event at a position (eager copy, spawn copy).
type eventSample struct {
	posKey
	count int64
	total time.Duration // 0 for pure counts (eager copies)
}

// callEvent is one method-call span for the trace timeline.
type callEvent struct {
	name  string
	file  string
	line  int
	col   int
	start time.Duration // since run start
	end   time.Duration // since run start
	gid   int           // goroutine id, so concurrent spawn spans land on separate trace tracks
}

// Collector accumulates instrumentation events. It is safe for concurrent
// use: `spawn` bodies run on their own goroutines and record onto the same
// Collector, so every Record* method and every render-time accessor takes
// `mu`. Recording a statement from a parallel `spawn` is exactly the case
// that once crashed with "concurrent map read and map write". The interpreter
// gates each Record* call on a corresponding "want" flag, so an unused stream
// costs nothing; the lock is taken only when a stream is actually recording.
type Collector struct {
	mu       sync.Mutex
	mode     Mode
	runStart time.Time

	stmts   map[posKey]*stmtSample
	eager   map[posKey]*eventSample
	spawn   map[posKey]*eventSample
	calls   []callEvent
	maxCall int // cap on recorded call events (trace); 0 = unlimited

	depths   map[posKey]int // max nested method-call depth observed per call site
	maxDepth int            // deepest nested method-call chain reached in the run
}

// NewCollector returns a Collector for the given mode. maxCallEvents bounds the
// call timeline recorded for the trace format (0 = unlimited); it is ignored
// outside ModeStatement.
func NewCollector(mode Mode, maxCallEvents int) *Collector {
	return &Collector{
		mode:    mode,
		stmts:   map[posKey]*stmtSample{},
		eager:   map[posKey]*eventSample{},
		spawn:   map[posKey]*eventSample{},
		depths:  map[posKey]int{},
		maxCall: maxCallEvents,
	}
}

// Mode reports the collector's mode.
func (c *Collector) Mode() Mode { return c.mode }

// Position is a source location where a statement executed. Exposed so a
// coverage consumer can reuse the statement-profile hit data.
type Position struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// StatementHits returns every position that recorded at least one statement
// execution, mapped to its hit count - the coverage numerator, a second reader
// of the same per-position data the statement profile renders (no separate
// counting path). Empty unless statement profiling was active.
func (c *Collector) StatementHits() map[Position]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[Position]int64, len(c.stmts))
	for k, s := range c.stmts {
		out[Position{File: k.file, Line: k.line, Col: k.col}] = s.hits
	}
	return out
}

// Start marks the profile's zero time; call events are timestamped relative to
// it. The interpreter calls this once before executing top-level statements.
func (c *Collector) Start(t time.Time) {
	c.mu.Lock()
	c.runStart = t
	c.mu.Unlock()
}

// RecordStmt accumulates one statement execution's self and cumulative time.
func (c *Collector) RecordStmt(file string, line, col int, self, cum time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := posKey{file, line, col}
	s := c.stmts[k]
	if s == nil {
		s = &stmtSample{posKey: k}
		c.stmts[k] = s
	}
	s.hits++
	s.self += self
	s.cum += cum
}

// RecordCall appends one method-call span to the trace timeline (bounded by
// maxCall). start/end are absolute times; they are stored relative to runStart.
func (c *Collector) RecordCall(name, file string, line, col int, start, end time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxCall > 0 && len(c.calls) >= c.maxCall {
		return
	}
	c.calls = append(c.calls, callEvent{
		name:  name,
		file:  file,
		line:  line,
		col:   col,
		start: start.Sub(c.runStart),
		end:   end.Sub(c.runStart),
		gid:   goroutineID(),
	})
}

// goroutineID returns the current goroutine's id by parsing the header of a
// single-goroutine stack dump. It is only called from RecordCall (bounded by
// maxCall, and only while profiling), so the cost is acceptable; it lets the
// Chrome trace put each spawn goroutine's spans on its own track instead of
// overlapping them all on tid 1.
func goroutineID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := buf[:n]
	s = bytes.TrimPrefix(s, []byte("goroutine "))
	i := bytes.IndexByte(s, ' ')
	if i < 0 {
		return 0
	}
	id, _ := strconv.Atoi(string(s[:i]))
	return id
}

// RecordCallDepth keeps the maximum nested method-call depth seen at a call
// site (and overall). Called on every method entry while statement profiling
// is active, so it only ever grows a running max - never allocates per call
// after the first sighting of a position.
func (c *Collector) RecordCallDepth(file string, line, col int, depth int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := posKey{file, line, col}
	if depth > c.depths[k] {
		c.depths[k] = depth
	}
	if depth > c.maxDepth {
		c.maxDepth = depth
	}
}

// RecordEagerCopy counts one eager deep copy at a value-storage site (def,
// assignment, or parameter binding).
func (c *Collector) RecordEagerCopy(file string, line, col int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := posKey{file, line, col}
	e := c.eager[k]
	if e == nil {
		e = &eventSample{posKey: k}
		c.eager[k] = e
	}
	e.count++
}

// RecordSpawnCopy counts one spawn-frame deep copy and its cost at the spawn
// site.
func (c *Collector) RecordSpawnCopy(file string, line, col int, dur time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := posKey{file, line, col}
	e := c.spawn[k]
	if e == nil {
		e = &eventSample{posKey: k}
		c.spawn[k] = e
	}
	e.count++
	e.total += dur
}

// stmtSorted returns the statement samples sorted by cumulative time desc.
// Each returned sample is a copy taken under the lock, so a render can't race
// a still-recording `spawn` goroutine (the maps and the sample fields are both
// snapshotted before the lock is released).
func (c *Collector) stmtSorted() []*stmtSample {
	c.mu.Lock()
	out := make([]*stmtSample, 0, len(c.stmts))
	for _, s := range c.stmts {
		cp := *s
		out = append(out, &cp)
	}
	c.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].cum != out[j].cum {
			return out[i].cum > out[j].cum
		}
		return out[i].self > out[j].self
	})
	return out
}

// eventsSorted flattens and sorts an event map by count desc. Like stmtSorted,
// it snapshots copies under the lock so render-time reads are race-free.
func (c *Collector) eventsSorted(m map[posKey]*eventSample) []*eventSample {
	c.mu.Lock()
	out := make([]*eventSample, 0, len(m))
	for _, e := range m {
		cp := *e
		out = append(out, &cp)
	}
	c.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].count > out[j].count
	})
	return out
}

// depthSample pairs a call site with the deepest nested call depth seen there.
type depthSample struct {
	posKey
	depth int
}

// depthsSorted returns the per-call-site max depths (desc) plus the overall
// max, snapshotted under the lock so a render can't race a recording spawn.
func (c *Collector) depthsSorted() ([]depthSample, int) {
	c.mu.Lock()
	out := make([]depthSample, 0, len(c.depths))
	for k, d := range c.depths {
		out = append(out, depthSample{posKey: k, depth: d})
	}
	overall := c.maxDepth
	c.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].depth != out[j].depth {
			return out[i].depth > out[j].depth
		}
		return out[i].pos() < out[j].pos()
	})
	return out, overall
}

// callsSnapshot returns a copy of the call timeline taken under the lock.
func (c *Collector) callsSnapshot() []callEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]callEvent(nil), c.calls...)
}
