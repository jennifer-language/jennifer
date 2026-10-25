// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
	// store into a real copy up front rather than deferring to Ensure. bytes
	// estimates what the copy allocated (copyBytes) and typ labels the copied
	// value (valueTypeLabel), so a report can rank sites by size, not count.
	RecordEagerCopy(file string, line, col int, bytes int64, typ string)
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

// ProfileModules controls whether a profiler installed with SetProfiler is also
// propagated into imported modules' sub-interpreters, so a module's statements
// are recorded against their own file positions (a reader sees `markdown.j:2160`
// beside their own lines). `jennifer profile` turns this on. `jennifer test
// --coverage`, which reuses the profiler for the entry program only, leaves it
// off, so standard-library module lines never pollute a coverage report.
func (i *Interpreter) ProfileModules(on bool) {
	i.profModules = on
}

// isCompoundCopyKind reports whether a value of kind k gets a real deep copy
// from Copy() (bytes/list/map/struct). Scalars copy trivially and tasks share
// a pointer, so neither is a meaningful "eager copy" to count.
func isCompoundCopyKind(k ValueKind) bool {
	return (k >= KindBytes && k <= KindStruct) || k == KindEnum
}

// eagerCopy is the profiling-aware form of Value.Copy() used at value-storage
// sites (def / assignment). It behaves exactly like Copy() but, when
// allocation profiling is on and the value is a compound (so the copy is real
// work), attributes the copy to node n's position.
func (i *Interpreter) eagerCopy(v Value, n parser.Node) Value {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		i.recordEagerCopy(v, n)
	}
	return v.Copy()
}

// recordEagerCopy reports one eager copy of v at n's position, with its size and
// type. The caller has already checked that allocation profiling is on and v is
// a compound, so the size walk is paid only by `jennifer profile --allocs`.
func (i *Interpreter) recordEagerCopy(v Value, n parser.Node) {
	file, line, col := posFor(n)
	i.prof.RecordEagerCopy(file, line, col, copyBytes(v), valueTypeLabel(v))
}

// recordParamCopy reports an eager copy made binding parameter p on a dispatch
// path that has no call-site node (a call from another module, a func value
// called across a module boundary, meta.call / callMain), so the copy is placed
// at the parameter's declaration instead.
func (i *Interpreter) recordParamCopy(v Value, p parser.Param) {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		i.prof.RecordEagerCopy(p.File, p.Line, p.Col, copyBytes(v), valueTypeLabel(v))
	}
}

// copyBytes estimates the heap a deep copy of v allocates: one Value cell per
// list element and struct / enum field, two per map entry, plus each bytes
// buffer, recursively. Strings count nothing - they are immutable, so a copy
// shares their backing. Unlike shallowValueBytes (the recursion memory guard's
// O(1) estimate) this walks the whole value, which is what makes a struct
// holding a large list rank above a one-byte buffer.
func copyBytes(v Value) int64 {
	switch v.Kind {
	case KindBytes:
		return int64(len(v.Bytes))
	case KindList:
		n := valueCellBytes * int64(len(v.List))
		for _, e := range v.List {
			n += copyBytes(e)
		}
		return n
	case KindMap:
		n := valueCellBytes * 2 * int64(len(v.Map))
		for _, e := range v.Map {
			n += copyBytes(e.Key) + copyBytes(e.Value)
		}
		return n
	case KindStruct, KindEnum:
		n := valueCellBytes * int64(len(v.Fields))
		for _, f := range v.Fields {
			n += copyBytes(f.Value)
		}
		return n
	default:
		return 0
	}
}
