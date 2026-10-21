// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import "jennifer-lang.dev/jennifer/internal/parser"

// Per-function escape analysis that widens read-only-parameter borrow to an
// entry program holding mutable globals. A method's never-written parameter can
// be borrowed when nothing reachable from the method can mutate a global (which
// is the only thing an argument could alias and have change under a value-
// semantic callee). computeEntryGlobalSafe proves that per method and stamps
// MethodDef.GlobalSafe; methodBorrowCtx consults it.
//
// A spawned task normally cannot affect the caller: it runs on a deep-copied
// snapshot. The exception is a task that re-enters the host (a module call that
// reaches meta.callMain, a callback builtin, a func-value call): that writes the
// live host globals, at any time, from outside the call chain the per-method
// analysis follows. So when any spawn body in the run can re-enter, the run is
// marked (spawnReentry on the host) and the GlobalSafe widening is off for it.

// jCallbackBuiltins is the (canonical namespace, callee) of every builtin that
// can re-enter and run *arbitrary .j code* - the higher-order `lists` layer
// (each invokes its func-value argument via BuiltinCtx.Invoke) and the by-name
// dispatchers (meta.call / meta.callMain via CallByName* / CallHostWith*;
// testing.run / runWith / assertThrows / assertThrowsWith via CallByName*). A method calling one of
// these can reach code that mutates a global, so it is not GlobalSafe.
//
// INVARIANT: any builtin that calls BuiltinCtx.Invoke or Interpreter.CallByName* /
// CallHostWith* MUST appear here (keyed by canonical namespace, not an alias).
// Missing one would let the per-function analysis mark a method GlobalSafe that
// can in fact reach a global write, making borrow in a mutable-globals script
// unsound. Verified against a grep for those entry points accross internal/lib.
var jCallbackBuiltins = map[[2]string]bool{
	{"meta", "call"}:                true,
	{"meta", "callMain"}:            true,
	{"testing", "run"}:              true,
	{"testing", "runWith"}:          true,
	{"testing", "assertThrows"}:     true,
	{"testing", "assertThrowsWith"}: true,
	{"lists", "map"}:                true,
	{"lists", "filter"}:             true,
	{"lists", "reduce"}:             true,
	{"lists", "find"}:               true,
	{"lists", "any"}:                true,
	{"lists", "all"}:                true,
	{"lists", "sortBy"}:             true,
}

// computeEntryGlobalSafe stamps GlobalSafe on every method that (transitively
// over its named calls) mutates no global. Runs for the entry program and for
// module sub-interpreters: a module method that reaches meta.callMain can mutate
// a host global a borrowed argument aliases, and the callback-builtin hazard set
// catches that. It reads moduleAliases and the namespace table, so it runs after
// module load.
//
// Concurrency: GlobalSafe is written onto the shared *parser.MethodDef here, so
// it is (like the resolveQualifiedRefs / resolveDeclaredTypesOnce stamps) valid
// only under the one-Run-per-resolved-Program assumption the interpreter already
// makes. It runs single-threaded in Run's setup, before any top-level statement
// or spawn, so there is no intra-interpreter race; running one resolved Program
// through two interpreters concurrently is unsupported (it would also race the
// pre-existing qualified-ref stamps).
func (i *Interpreter) computeEntryGlobalSafe(prog *parser.Program) {
	type mstate struct {
		unsafe, reenters bool
		callers          []*parser.MethodDef
	}
	states := make(map[*parser.MethodDef]*mstate, len(i.methods))
	scans := make(map[*parser.MethodDef]*globalHazardScan, len(i.methods))
	for _, m := range i.methods {
		locals := make(map[string]bool, len(m.Params))
		for _, p := range m.Params {
			locals[p.Name] = true
		}
		collectMethodLocals(m.Body.Stmts, locals)
		scan := &globalHazardScan{in: i, locals: locals}
		scan.walkStmts(m.Body.Stmts)
		scans[m] = scan
		states[m] = &mstate{unsafe: scan.unsafe, reenters: scan.reenters}
	}
	// Reverse call edges, so each fact propagates with a worklist in time linear
	// in the call graph (a fixpoint that rescans every method per round is
	// quadratic on a long call chain).
	for m, scan := range scans {
		for _, c := range scan.callees {
			if cs, ok := states[c]; ok {
				cs.callers = append(cs.callers, m)
			}
		}
	}
	propagate := func(has func(*mstate) bool, set func(*mstate)) {
		var work []*parser.MethodDef
		for m, st := range states {
			if has(st) {
				work = append(work, m)
			}
		}
		for len(work) > 0 {
			m := work[len(work)-1]
			work = work[:len(work)-1]
			for _, caller := range states[m].callers {
				if cs := states[caller]; !has(cs) {
					set(cs)
					work = append(work, caller)
				}
			}
		}
	}
	propagate(func(st *mstate) bool { return st.unsafe }, func(st *mstate) { st.unsafe = true })
	propagate(func(st *mstate) bool { return st.reenters }, func(st *mstate) { st.reenters = true })
	for m, st := range states {
		m.GlobalSafe = !st.unsafe
	}

	// A spawn body re-enters when it holds a re-entering node itself or calls a
	// method that (transitively) re-enters. Spawns sit in method bodies and in
	// the top-level code.
	spawnReenters := func(scan *globalHazardScan) bool {
		if scan.spawnReenters {
			return true
		}
		for _, c := range scan.spawnCallees {
			if cs, ok := states[c]; ok && cs.reenters {
				return true
			}
		}
		return false
	}
	reentry := false
	for _, scan := range scans {
		if spawnReenters(scan) {
			reentry = true
			break
		}
	}
	if !reentry && prog != nil {
		top := &globalHazardScan{in: i, locals: map[string]bool{}}
		top.walkStmts(prog.TopLevel)
		reentry = spawnReenters(top)
	}
	if reentry {
		i.Host().spawnReentry.Store(true)
	}
}

// collectMethodLocals gathers every name bound within a method body (defs, loop
// iterators, for-init, catch bindings, match binders), descending through nested
// control-flow blocks but not into `spawn` bodies (those are frame-scoped to the
// spawn snapshot). A write whose root is not in this set targets a global.
// Under-collecting is safe: it only misclassifies a local write as a global one,
// which disables borrow (conservative), never enables it wrongly.
func collectMethodLocals(stmts []parser.Stmt, locals map[string]bool) {
	for _, s := range stmts {
		collectLocalsStmt(s, locals)
	}
}

func collectLocalsStmt(s parser.Stmt, locals map[string]bool) {
	switch st := s.(type) {
	case *parser.DefineStmt:
		locals[st.VarName] = true
	case *parser.IfStmt:
		collectMethodLocals(st.Then.Stmts, locals)
		for _, b := range st.ElseIfBodies {
			collectMethodLocals(b.Stmts, locals)
		}
		if st.Else != nil {
			collectMethodLocals(st.Else.Stmts, locals)
		}
	case *parser.MatchStmt:
		for _, a := range st.Arms {
			if a.Bind != "" {
				locals[a.Bind] = true
			}
			collectMethodLocals(a.Body.Stmts, locals)
		}
		if st.Else != nil {
			collectMethodLocals(st.Else.Stmts, locals)
		}
	case *parser.WhileStmt:
		collectMethodLocals(st.Body.Stmts, locals)
	case *parser.ForStmt:
		if st.Init != nil {
			collectLocalsStmt(st.Init, locals)
		}
		if st.Step != nil {
			collectLocalsStmt(st.Step, locals)
		}
		collectMethodLocals(st.Body.Stmts, locals)
	case *parser.ForEachStmt:
		locals[st.VarName] = true
		collectMethodLocals(st.Body.Stmts, locals)
	case *parser.RepeatStmt:
		collectMethodLocals(st.Body.Stmts, locals)
	case *parser.TryStmt:
		collectMethodLocals(st.Body.Stmts, locals)
		locals[st.CatchName] = true
		collectMethodLocals(st.CatchBody.Stmts, locals)
	case *parser.Block:
		collectMethodLocals(st.Stmts, locals)
	case *parser.AssignStmt, *parser.IndexAssignStmt, *parser.AppendStmt,
		*parser.FieldAssignStmt, *parser.ReturnStmt, *parser.ExitStmt,
		*parser.ThrowStmt, *parser.DeferStmt, *parser.ExprStmt,
		*parser.BreakStmt, *parser.ContinueStmt, *parser.ImportStmt,
		*parser.ModuleImportStmt, *parser.StructDef, *parser.EnumDef,
		*parser.MethodDef:
		// Introduce no binding and hold no statement block to descend into
		// (their expressions never bind a name). Listed explicitly so a new
		// statement kind hits the default and is reviewed, rather than silently
		// under-collecting a binding it might introduce.
	default:
		// Unknown statement: under-collect (ignore). Safe - a missed local only
		// makes a write to it look global, disabling borrow. TestBorrowWalkers-
		// CoverAllNodes fails on a new kind so this stays a conscious choice.
	}
}

// globalHazardScan walks a method body looking for anything that could mutate a
// global during a call: a write to a non-local (global) binding, a dynamic or
// unresolved call, a module call (a module can re-enter the host via
// meta.callMain), or a callback builtin. The last three re-enter (reenters): they
// can run arbitrary .j code against the live host globals. It records
// statically-named user-method callees for the fixpoints. Inside a `spawn` body a
// global write is harmless (it lands in the task's snapshot), but a re-entering
// node is not, so there it only sets spawnReenters / spawnCallees. Unrecognised
// nodes default to unsafe and re-entering, so a new AST node never silently
// escapes the analysis.
type globalHazardScan struct {
	in       *Interpreter
	locals   map[string]bool
	unsafe   bool
	reenters bool
	callees  []*parser.MethodDef
	// inSpawn counts the enclosing spawn bodies at the current walk position.
	inSpawn       int
	spawnReenters bool
	spawnCallees  []*parser.MethodDef
}

// reentry records a node that can run arbitrary .j code (and so write the live
// host globals).
func (s *globalHazardScan) reentry() {
	if s.inSpawn > 0 {
		s.spawnReenters = true
		return
	}
	s.unsafe = true
	s.reenters = true
}

// call records a statically-named user-method callee.
func (s *globalHazardScan) call(m *parser.MethodDef) {
	if s.inSpawn > 0 {
		s.spawnCallees = append(s.spawnCallees, m)
		return
	}
	s.callees = append(s.callees, m)
}

func (s *globalHazardScan) walkStmts(stmts []parser.Stmt) {
	for _, st := range stmts {
		s.walkStmt(st)
	}
}

func (s *globalHazardScan) writesGlobal(root string) {
	if s.inSpawn > 0 {
		// A spawn body writes its own snapshot, never the live globals.
		return
	}
	if root == "" || !s.locals[root] {
		// Root is not a method-local: it is a mutable global (a const target is
		// rejected at resolve, so any legal write to a non-local hits a mutable
		// global). Empty root means an unrecognised lvalue shape - conservative.
		s.unsafe = true
	}
}

func (s *globalHazardScan) walkStmt(st parser.Stmt) {
	switch n := st.(type) {
	case *parser.DefineStmt:
		s.walkExpr(n.InitExpr)
	case *parser.AssignStmt:
		s.writesGlobal(n.VarName)
		s.walkExpr(n.Value)
	case *parser.IndexAssignStmt:
		s.writesGlobal(lvalueRoot(n.Target))
		s.walkExpr(n.Target)
		s.walkExpr(n.Value)
	case *parser.AppendStmt:
		s.writesGlobal(lvalueRoot(n.Target))
		s.walkExpr(n.Target)
		s.walkExpr(n.Value)
	case *parser.FieldAssignStmt:
		s.writesGlobal(lvalueRoot(n.Target))
		s.walkExpr(n.Target)
		s.walkExpr(n.Value)
	case *parser.IfStmt:
		s.walkExpr(n.Cond)
		s.walkStmts(n.Then.Stmts)
		for idx, c := range n.ElseIfs {
			s.walkExpr(c)
			s.walkStmts(n.ElseIfBodies[idx].Stmts)
		}
		if n.Else != nil {
			s.walkStmts(n.Else.Stmts)
		}
	case *parser.MatchStmt:
		s.walkExpr(n.Subject)
		for _, a := range n.Arms {
			for _, v := range a.Values {
				s.walkExpr(v)
			}
			s.walkStmts(a.Body.Stmts)
		}
		if n.Else != nil {
			s.walkStmts(n.Else.Stmts)
		}
	case *parser.WhileStmt:
		s.walkExpr(n.Cond)
		s.walkStmts(n.Body.Stmts)
	case *parser.ForStmt:
		if n.Init != nil {
			s.walkStmt(n.Init)
		}
		s.walkExpr(n.Cond)
		if n.Step != nil {
			s.walkStmt(n.Step)
		}
		s.walkStmts(n.Body.Stmts)
	case *parser.ForEachStmt:
		s.walkExpr(n.Coll)
		s.walkStmts(n.Body.Stmts)
	case *parser.RepeatStmt:
		s.walkStmts(n.Body.Stmts)
		s.walkExpr(n.Cond)
	case *parser.ReturnStmt:
		s.walkExpr(n.Value)
	case *parser.ExitStmt:
		s.walkExpr(n.Code)
	case *parser.ThrowStmt:
		s.walkExpr(n.Value)
	case *parser.DeferStmt:
		s.walkExpr(n.Call)
	case *parser.TryStmt:
		s.walkStmts(n.Body.Stmts)
		s.walkStmts(n.CatchBody.Stmts)
	case *parser.ExprStmt:
		s.walkExpr(n.Expr)
	case *parser.Block:
		s.walkStmts(n.Stmts)
	case *parser.BreakStmt, *parser.ContinueStmt,
		*parser.ImportStmt, *parser.ModuleImportStmt,
		*parser.StructDef, *parser.EnumDef, *parser.MethodDef:
		// No writes, no calls, no nested method bodies reachable here.
	default:
		s.reentry()
	}
}

func (s *globalHazardScan) walkExpr(e parser.Expr) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *parser.IntLit, *parser.FloatLit, *parser.StringLit, *parser.BoolLit,
		*parser.NullLit, *parser.VarExpr, *parser.ConstRefExpr,
		*parser.QualifiedConstRefExpr, *parser.PreEval:
		// Leaves: no sub-expressions, no calls, no writes.
	case *parser.BinaryExpr:
		s.walkExpr(n.Left)
		s.walkExpr(n.Right)
	case *parser.UnaryExpr:
		s.walkExpr(n.Operand)
	case *parser.LenExpr:
		s.walkExpr(n.Operand)
	case *parser.IndexExpr:
		s.walkExpr(n.Target)
		s.walkExpr(n.Index)
	case *parser.FieldAccessExpr:
		s.walkExpr(n.Target)
	case *parser.RangeExpr:
		s.walkExpr(n.Lo)
		s.walkExpr(n.Hi)
	case *parser.SliceExpr:
		s.walkExpr(n.Target)
		s.walkExpr(n.Lo)
		s.walkExpr(n.Hi)
	case *parser.ListLit:
		for _, el := range n.Elements {
			s.walkExpr(el)
		}
	case *parser.MapLit:
		for _, k := range n.Keys {
			s.walkExpr(k)
		}
		for _, v := range n.Values {
			s.walkExpr(v)
		}
	case *parser.StructLit:
		for _, f := range n.Fields {
			s.walkExpr(f.Expr)
		}
	case *parser.InterpStringExpr:
		for _, p := range n.Parts {
			s.walkExpr(p.Expr)
		}
	case *parser.CallExpr:
		// The resolver skips spawn bodies, so Method is nil for a call inside
		// one; resolve the callee by name against the method table instead.
		m := n.Method
		if m == nil {
			m = s.in.methods[n.Callee]
		}
		if m != nil {
			s.call(m)
		} else {
			// A bare-name call not resolving to a user method: a builtin-shaped
			// bare call or an unresolved name. Cannot analyse - conservative.
			s.reentry()
		}
		for _, a := range n.Args {
			s.walkExpr(a)
		}
	case *parser.QualifiedCallExpr:
		s.qualifiedCall(n)
	case *parser.CallValueExpr:
		// Dynamic dispatch through a function value: the target is unknown.
		s.reentry()
		s.walkExpr(n.Callee)
		for _, a := range n.Args {
			s.walkExpr(a)
		}
	case *parser.SpawnExpr:
		// The body's global writes land in its snapshot; only re-entry matters.
		s.inSpawn++
		s.walkStmts(n.Body)
		s.inSpawn--
	default:
		s.reentry()
	}
}

func (s *globalHazardScan) qualifiedCall(n *parser.QualifiedCallExpr) {
	// A module call may re-enter the host (meta.callMain) and reach host code
	// that mutates a global, so treat every module call as unsafe.
	if _, isModule := s.in.moduleAliases[n.Prefix]; isModule {
		s.reentry()
		for _, a := range n.Args {
			s.walkExpr(a)
		}
		return
	}
	// Canonicalise the prefix (an alias -> its library name) before the callback
	// check, which keys on canonical names.
	ns := n.Prefix
	if canon, err := s.in.resolveNamespacePrefix(n.Prefix); err == nil {
		ns = canon
	}
	if jCallbackBuiltins[[2]string{ns, n.Callee}] {
		s.reentry()
	}
	for _, a := range n.Args {
		s.walkExpr(a)
	}
}

// lvalueRoot returns the root variable name of an index/field lvalue chain
// (`$p[i].f` -> "p"), or "" when the root is not a plain variable.
func lvalueRoot(e parser.Expr) string {
	switch ex := e.(type) {
	case *parser.VarExpr:
		return ex.Name
	case *parser.IndexExpr:
		return lvalueRoot(ex.Target)
	case *parser.FieldAccessExpr:
		return lvalueRoot(ex.Target)
	}
	return ""
}
