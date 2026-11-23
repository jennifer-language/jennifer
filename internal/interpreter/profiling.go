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
	// RecordBoundaryCopy counts one module-boundary rebuild at a call site or
	// constant read: a value holding a module's own structs, rebuilt by
	// retagStructs so its struct identities match the side it crosses into.
	// bytes estimates what the rebuild allocated (retagBytes), typ labels the
	// value, and outward is true for a value leaving the module (a result or a
	// constant) and false for an argument entering it.
	RecordBoundaryCopy(file string, line, col int, bytes int64, typ string, outward bool)
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

// retagAt is retagStructs for a value crossing a module boundary at node n.
// Under --allocs it reports the rebuild at n's position; a value that needs no
// retag (the common case) is reported nowhere. The type label is the caller's
// view: the value before an inward retag, the result of an outward one.
func (i *Interpreter) retagAt(v Value, fromNS, toNS, fromMod, toMod string, isOwn func(string) bool, n parser.Node, outward bool) Value {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		b := retagBytes(v, fromNS, fromMod, isOwn)
		r := retagStructs(v, fromNS, toNS, fromMod, toMod, isOwn)
		ns := fromNS
		if outward {
			ns = toNS
		}
		i.recordBoundaryCopy(callerView(v, r, outward), b, n, outward, ns, isOwn)
		return r
	}
	return retagStructs(v, fromNS, toNS, fromMod, toMod, isOwn)
}

// crossRetagAt is crossRetag (a func value called across interpreters) with the
// same --allocs reporting as retagAt, summing the bytes of both of its passes.
func (i *Interpreter) crossRetagAt(v Value, from, to *Interpreter, n parser.Node, outward bool) Value {
	if i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
		b := retagBytes(v, to.moduleNS, to.modulePath, to.isOwnStructName)
		mid := retagStructs(v, to.moduleNS, "", to.modulePath, "", to.isOwnStructName)
		b += retagBytes(mid, "", "", from.isOwnStructName)
		r := retagStructs(mid, "", from.moduleNS, "", from.modulePath, from.isOwnStructName)
		i.recordBoundaryCopy(callerView(v, r, outward), b, n, outward, "", nil)
		return r
	}
	return crossRetag(v, from, to)
}

// callerView picks the side of a boundary crossing the calling program sees:
// the value it passed in, or the value it gets back.
func callerView(before, after Value, outward bool) Value {
	if outward {
		return after
	}
	return before
}

// recordBoundaryCopy reports one rebuild of bytes at n. A module struct can
// carry its module path but no namespace (one built inside the module and
// returned), which valueTypeLabel would show bare; ns, the module's name as the
// caller sees it, qualifies such a struct when isOwn says the module owns it.
func (i *Interpreter) recordBoundaryCopy(v Value, bytes int64, n parser.Node, outward bool, ns string, isOwn func(string) bool) {
	if bytes == 0 {
		return
	}
	label := valueTypeLabel(v)
	if (v.Kind == KindStruct || v.Kind == KindEnum) && v.StructNS == "" && ns != "" && isOwn != nil && isOwn(v.StructName) {
		label = ns + "." + v.StructName
	}
	file, line, col := posFor(n)
	i.prof.RecordBoundaryCopy(file, line, col, bytes, label, outward)
}

// retagBytes estimates the heap retagStructs(v, fromNS, ...) allocates, walking
// the same nodes it rebuilds: a fresh field array for every struct / enum, a
// fresh element array for every list and entry array for every map, on each
// subtree that holds a struct to retag (subtreeNeedsRetag). A subtree that does
// not is shared, not rebuilt, and counts nothing, so a value with no module
// struct reports 0. The cell sizes match copyBytes.
func retagBytes(v Value, fromNS, fromMod string, isOwn func(string) bool) int64 {
	if !subtreeNeedsRetag(v, fromNS, fromMod, isOwn) {
		return 0
	}
	switch v.Kind {
	case KindStruct, KindEnum:
		n := valueCellBytes * int64(len(v.Fields))
		for _, f := range v.Fields {
			n += retagBytes(f.Value, fromNS, fromMod, isOwn)
		}
		return n
	case KindList:
		n := valueCellBytes * int64(len(v.List))
		for _, e := range v.List {
			n += retagBytes(e, fromNS, fromMod, isOwn)
		}
		return n
	case KindMap:
		n := valueCellBytes * 2 * int64(len(v.Map))
		for _, e := range v.Map {
			n += retagBytes(e.Key, fromNS, fromMod, isOwn) + retagBytes(e.Value, fromNS, fromMod, isOwn)
		}
		return n
	default:
		return 0
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
