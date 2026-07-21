// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"time"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// Profiler receives instrumentation events from the evaluator when profiling
// is active. It is an interface so the interpreter (which compiles into both
// binaries) carries no profiling machinery of its own - the concrete
// collector lives in internal/profile and is injected only by the default
// binary's `jennifer profile` subcommand. All time values are wall-clock.
type Profiler interface {
	// Start marks the profile's zero time; call spans are relative to it.
	Start(t time.Time)
	// RecordStmt reports one statement execution's self time (excluding
	// nested statements) and cumulative time (including them).
	RecordStmt(file string, line, col int, self, cum time.Duration)
	// RecordCall reports one method-call span for the trace timeline.
	RecordCall(name, file string, line, col int, start, end time.Time)
	// RecordCallDepth reports the current nested method-call depth at a call
	// site, so the statement profile can surface the deepest recursion reached
	// (per call site and overall).
	RecordCallDepth(file string, line, col int, depth int)
	// RecordEagerCopy counts one eager deep copy at a value-storage site
	// (def / assignment / parameter binding), where value semantics turn a
	// store into a real copy up front rather than deferring to Ensure.
	RecordEagerCopy(file string, line, col int)
	// RecordSpawnCopy counts one spawn-frame deep copy and its cost at the
	// spawn site.
	RecordSpawnCopy(file string, line, col int, dur time.Duration)
}

// SetProfiler installs a profiler and selects which streams to record.
// timeStmts drives the statement profile (default mode); timeCalls records the
// method-call timeline (the trace form); trackAllocs records eager value-copies
// and spawn-frame copies (--allocs mode). Passing nil disables profiling.
func (i *Interpreter) SetProfiler(p Profiler, timeStmts, timeCalls, trackAllocs bool) {
	i.prof = p
	i.profStmts = timeStmts
	i.profCalls = timeCalls
	i.profAllocs = trackAllocs
}

// isCompoundCopyKind reports whether a value of kind k gets a real deep copy
// from Copy() (bytes/list/map/struct). Scalars copy trivially and tasks share
// a pointer, so neither is a meaningful "eager copy" to count.
func isCompoundCopyKind(k ValueKind) bool { return k >= KindBytes && k <= KindStruct }

// eagerCopy is the profiling-aware form of Value.Copy() used at value-storage
// sites (def / assignment). It behaves exactly like Copy() but, when
// allocation profiling is on and the value is a compound (so the copy is real
// work), attributes the copy to node n's position.
func (i *Interpreter) eagerCopy(v Value, n parser.Node) Value {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		file, line, col := posFor(n)
		i.prof.RecordEagerCopy(file, line, col)
	}
	return v.Copy()
}
