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
	// nested statements and waits), its wait (time blocked in a builtin that
	// waits, see blockingBuiltins) and cumulative time (including all of it).
	RecordStmt(file string, line, col int, self, wait, cum time.Duration)
	// DeclareFunc tells the profiler where a function starts (name non-empty)
	// or a top-level statement does (name ""), before the program runs, so a
	// report can name the function of each statement position. body lists the
	// positions of a function body's own statements (line, col), in order.
	DeclareFunc(name, file string, line, col int, body [][2]int)
	// RecordTask counts one spawned task.
	RecordTask()
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

// declareProfileFuncs declares prog's functions and top-level statements to
// the profiler (Profiler.DeclareFunc).
func (i *Interpreter) declareProfileFuncs(prog *parser.Program) {
	for _, m := range prog.Methods {
		if m == nil {
			continue
		}
		var body [][2]int
		if m.Body != nil {
			for _, st := range m.Body.Stmts {
				l, c := st.Pos()
				body = append(body, [2]int{l, c})
			}
		}
		file, line, col := posFor(m)
		i.prof.DeclareFunc(m.Name, file, line, col, body)
	}
	for _, st := range prog.TopLevel {
		if _, ok := st.(*parser.MethodDef); ok {
			continue
		}
		file, line, col := posFor(st)
		i.prof.DeclareFunc("", file, line, col, nil)
	}
}

// blockingBuiltins are the library functions whose time is mostly spent
// waiting: the builtins that watch task cancellation while they block, plus
// httpd.accept and net.accept. Under the statement profiler their time is
// reported as the statement's wait, not its self time.
var blockingBuiltins = map[string]map[string]bool{
	"time":    {"sleep": true},
	"channel": {"send": true, "recv": true, "recvTimeout": true, "select": true},
	"task":    {"wait": true, "waitAll": true, "waitAny": true, "waitTimeout": true, "waitAnyTimeout": true},
	"net":     {"accept": true, "readBytes": true, "readAll": true, "readN": true, "recvFrom": true},
	"serial":  {"read": true},
	"httpd":   {"accept": true},
}

// blockingCall reports whether c calls one of blockingBuiltins. Only asked
// while statement profiling is on.
func (i *Interpreter) blockingCall(c *parser.QualifiedCallExpr) bool {
	ns, err := i.resolveNamespacePrefix(c.Prefix)
	if err != nil {
		return false
	}
	return blockingBuiltins[ns][c.Callee]
}

// waitCall runs a blocking builtin and adds its duration to the running
// statement's wait (CallChain.wait), which execStmt keeps out of self time.
func (i *Interpreter) waitCall(fn Builtin, ctx BuiltinCtx, args []Value, env *Environment) (Value, error) {
	start := time.Now()
	v, err := fn(ctx, args)
	if chain := env.depth; chain != nil {
		chain.wait += int64(time.Since(start))
	}
	return v, err
}

// isCompoundCopyKind reports whether a value of kind k gets a real deep copy
// from Copy() (bytes/list/map/struct). Scalars copy trivially and tasks share
// a pointer, so neither is a meaningful "eager copy" to count.
func isCompoundCopyKind(k ValueKind) bool {
	return (k >= KindBytes && k <= KindStruct) || k == KindEnum
}

// eagerCopy is the profiling-aware form of Value.Copy() used wherever value
// semantics store a copy: def / assignment, a literal element, a field / index /
// append write, a thrown value, a defer argument, a catch binding. It behaves
// exactly like Copy() but, when allocation profiling is on and the value is a
// compound (so the copy is real work), attributes the copy to n's position.
func (i *Interpreter) eagerCopy(v Value, n positioned) Value {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		i.recordEagerCopy(v, n)
	}
	return v.Copy()
}

// recordCopy reports, under --allocs, a copy of the compound v made at n by a
// helper that copies on its own (writeIndexedSlot, appendInto, a slice).
func (i *Interpreter) recordCopy(v Value, n positioned) {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		i.recordEagerCopy(v, n)
	}
}

// recordEagerCopy reports one eager copy of v at n's position, with its size and
// type. The caller has already checked that allocation profiling is on and v is
// a compound, so the size walk is paid only by `jennifer profile --allocs`.
func (i *Interpreter) recordEagerCopy(v Value, n positioned) {
	file, line, col := posOf(n)
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

// copyBytes is the heap a deep copy of v allocates: one Value cell per list
// element and struct / enum field, two per map entry, plus each bytes buffer,
// recursively. Strings count nothing - they are immutable, so a copy shares
// their backing. Unlike shallowValueBytes (the recursion memory guard's O(1)
// estimate) this walks the whole value, which is what makes a struct holding a
// large list rank above a one-byte buffer under `profile --allocs`, and what
// the allocation budget charges for an eager copy (the walk is paid only by a
// task with a budget, and costs no more than the copy itself).
func copyBytes(v Value) int64 {
	return copyBytesOf(&v)
}

// copyBytesOf is copyBytes through a pointer, recursing once per nesting level
// with no Value in its frame, for the reason deepCopyInto does (jennifer-tiny's
// fixed stack).
//
//go:noinline
func copyBytesOf(v *Value) int64 {
	switch v.Kind {
	case KindBytes:
		return int64(len(v.Bytes()))
	case KindList:
		l := v.List()
		n := valueCellBytes * int64(len(l))
		for k := range l {
			n += copyBytesOf(&l[k])
		}
		return n
	case KindMap:
		m := v.Map()
		n := valueCellBytes * 2 * int64(len(m))
		for k := range m {
			n += copyBytesOf(&m[k].Key) + copyBytesOf(&m[k].Value)
		}
		return n
	case KindStruct, KindEnum:
		f := v.Fields()
		n := valueCellBytes * int64(len(f))
		for k := range f {
			n += copyBytesOf(&f[k].Value)
		}
		return n
	default:
		return 0
	}
}
