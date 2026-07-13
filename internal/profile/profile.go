// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package profile collects and renders execution profiles for Jennifer
// programs. The interpreter, when handed a *Collector, calls its Record*
// methods at instrumented eval sites; the Collector aggregates them and can
// render a flat table, a pprof-compatible profile, or a Chrome-trace stream.
//
// It attributes work back to Jennifer source positions (file:line:col) - the
// gap left by `go tool pprof`, which profiles the interpreter binary, not the
// .j program running inside it. Two modes: the default statement/call profile
// (hit counts + wall-clock time per position) and an allocation profile
// (--allocs) that surfaces the shared-marker COW detachments and spawn-frame
// deep copies that value semantics turn into real work.
package profile

import (
	"sort"
	"sync"
	"time"
)

// Mode selects what a run records.
type Mode int

const (
	// ModeStatement records per-position hit counts and wall-clock time
	// (self and cumulative) plus a method-call timeline for the trace form.
	ModeStatement Mode = iota
	// ModeAllocs records value-semantics work: COW detachments and
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

// eventSample aggregates a counted event at a position (COW detach, spawn copy).
type eventSample struct {
	posKey
	count int64
	total time.Duration // 0 for pure counts (detaches)
}

// callEvent is one method-call span for the trace timeline.
type callEvent struct {
	name  string
	file  string
	line  int
	col   int
	start time.Duration // since run start
	end   time.Duration // since run start
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
	detach  map[posKey]*eventSample
	eager   map[posKey]*eventSample
	spawn   map[posKey]*eventSample
	calls   []callEvent
	maxCall int // cap on recorded call events (trace); 0 = unlimited
}

// NewCollector returns a Collector for the given mode. maxCallEvents bounds the
// call timeline recorded for the trace format (0 = unlimited); it is ignored
// outside ModeStatement.
func NewCollector(mode Mode, maxCallEvents int) *Collector {
	return &Collector{
		mode:    mode,
		stmts:   map[posKey]*stmtSample{},
		detach:  map[posKey]*eventSample{},
		eager:   map[posKey]*eventSample{},
		spawn:   map[posKey]*eventSample{},
		maxCall: maxCallEvents,
	}
}

// Mode reports the collector's mode.
func (c *Collector) Mode() Mode { return c.mode }

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
	})
}

// RecordDetach counts one COW detachment (an Ensure that copied a shared
// backing) at a position.
func (c *Collector) RecordDetach(file string, line, col int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := posKey{file, line, col}
	e := c.detach[k]
	if e == nil {
		e = &eventSample{posKey: k}
		c.detach[k] = e
	}
	e.count++
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

// callsSnapshot returns a copy of the call timeline taken under the lock.
func (c *Collector) callsSnapshot() []callEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]callEvent(nil), c.calls...)
}
