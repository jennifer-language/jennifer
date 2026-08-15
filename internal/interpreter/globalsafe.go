// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import "jennifer-lang.dev/jennifer/internal/parser"

// Per-function escape analysis that widens read-only-parameter borrow to an
// entry program holding mutable globals. A method's never-written parameter can
// be borrowed when nothing reachable from the method can mutate a global (which
// is the only thing an argument could alias and have change under a value-
// semantic callee). computeEntryGlobalSafe proves that per method and stamps
// MethodDef.GlobalSafe; borrowContext consults it.

// jCallbackBuiltins is the (canonical namespace, callee) of every builtin that
// can re-enter and run *arbitrary .j code* - the higher-order `lists` layer
// (each invokes its func-value argument via BuiltinCtx.Invoke) and the by-name
// dispatchers (meta.call / meta.callMain via CallByName* / CallHostWith*;
// testing.run / runWith / assertThrows via CallByName*). A method calling one of
// these can reach code that mutates a global, so it is not GlobalSafe.
//
// INVARIANT: any builtin that calls BuiltinCtx.Invoke or Interpreter.CallByName* /
// CallHostWith* MUST appear here (keyed by canonical namespace, not an alias).
// Missing one would let the per-function analysis mark a method GlobalSafe that
// can in fact reach a global write, making borrow in a mutable-globals script
// unsound. Verified against a grep for those entry points accross internal/lib.
var jCallbackBuiltins = map[[2]string]bool{
	{"meta", "call"}:            true,
	{"meta", "callMain"}:        true,
	{"testing", "run"}:          true,
	{"testing", "runWith"}:      true,
	{"testing", "assertThrows"}: true,
	{"lists", "map"}:            true,
	{"lists", "filter"}:         true,
	{"lists", "reduce"}:         true,
	{"lists", "find"}:           true,
	{"lists", "any"}:            true,
	{"lists", "all"}:            true,
	{"lists", "sortBy"}:         true,
}

// computeEntryGlobalSafe stamps GlobalSafe on every entry-program method that
// (transitively over its named calls) mutates no global. Only meaningful for the
// entry program (a module borrows via isModule); it reads moduleAliases and the
// namespace table, so it runs after module load.
//
// Concurrency: GlobalSafe is written onto the shared *parser.MethodDef here, so
// it is (like the resolveQualifiedRefs / resolveDeclaredTypesOnce stamps) valid
// only under the one-Run-per-resolved-Program assumption the interpreter already
// makes. It runs single-threaded in Run's setup, before any top-level statement
// or spawn, so there is no intra-interpreter race; running one resolved Program
// through two interpreters concurrently is unsupported (it would also race the
// pre-existing qualified-ref stamps).
func (i *Interpreter) computeEntryGlobalSafe() {
	// Nothing can borrow unless some method has a borrowable parameter, so skip
	// the whole scan + fixpoint (which would otherwise walk every method body on
	// every Run of any globals-holding script) when none does. GlobalSafe stays
	// false, which is the conservative, correct state.
	if !i.anyBorrowableParam() {
		return
	}
	type mstate struct {
		unsafe  bool
		callees []*parser.MethodDef
	}
	states := make(map[*parser.MethodDef]*mstate, len(i.methods))
	for _, m := range i.methods {
		locals := make(map[string]bool, len(m.Params))
		for _, p := range m.Params {
			locals[p.Name] = true
		}
		collectMethodLocals(m.Body.Stmts, locals)
		scan := &globalHazardScan{in: i, locals: locals}
		scan.walkStmts(m.Body.Stmts)
		states[m] = &mstate{unsafe: scan.unsafe, callees: scan.callees}
	}
	// Propagate: a method that calls a not-yet-proven-safe method is unsafe.
	// Iterate to a fixpoint (handles recursion and mutual recursion).
	for changed := true; changed; {
		changed = false
		for _, st := range states {
			if st.unsafe {
				continue
			}
			for _, c := range st.callees {
				if cs, ok := states[c]; ok && cs.unsafe {
					st.unsafe = true
					changed = true
					break
				}
			}
		}
	}
	for m, st := range states {
		m.GlobalSafe = !st.unsafe
	}
}

// anyBorrowableParam reports whether any entry method has a parameter the
// resolver marked borrowable - the precondition for the escape analysis to buy
// anything.
func (i *Interpreter) anyBorrowableParam() bool {
	for _, m := range i.methods {
		for _, p := range m.Params {
			if p.Borrow {
				return true
			}
		}
	}
	return false
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
// meta.callMain), or a callback builtin. It records statically-named user-method
// callees for the fixpoint. `spawn` bodies are skipped (they mutate their own
// deep-copied snapshot). Unrecognised nodes default to unsafe, so a new AST node
// never silently escapes the analysis.
type globalHazardScan struct {
	in      *Interpreter
	locals  map[string]bool
	unsafe  bool
	callees []*parser.MethodDef
}

func (s *globalHazardScan) walkStmts(stmts []parser.Stmt) {
	for _, st := range stmts {
		s.walkStmt(st)
	}
}

func (s *globalHazardScan) writesGlobal(root string) {
	if root == "" || !s.locals[root] {
		// Root is not a method-local: it is a mutable global (a const target is
		// rejected at resolve, so any legal write to a non-local hits a mutable
		// global). Empty root means an unrecognised lvalue shape - conservative.
		s.unsafe = true
	}
}

func (s *globalHazardScan) walkStmt(st parser.Stmt) {
	if s.unsafe {
		return
	}
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
		if n.Target != nil {
			s.writesGlobal(n.Target.Name)
		} else {
			s.unsafe = true
		}
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
		s.unsafe = true
	}
}

func (s *globalHazardScan) walkExpr(e parser.Expr) {
	if s.unsafe || e == nil {
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
		if n.Method != nil {
			s.callees = append(s.callees, n.Method)
		} else {
			// A bare-name call not resolving to a user method: a builtin-shaped
			// bare call or an unresolved name. Cannot analyse - conservative.
			s.unsafe = true
		}
		for _, a := range n.Args {
			s.walkExpr(a)
		}
	case *parser.QualifiedCallExpr:
		s.qualifiedCall(n)
	case *parser.CallValueExpr:
		// Dynamic dispatch through a function value: the target is unknown.
		s.unsafe = true
	case *parser.SpawnExpr:
		// A spawn body mutates its own deep-copied snapshot, not the live
		// globals, so it cannot change a borrowed argument's backing. Skip it.
	default:
		s.unsafe = true
	}
}

func (s *globalHazardScan) qualifiedCall(n *parser.QualifiedCallExpr) {
	// A module call may re-enter the host (meta.callMain) and reach host code
	// that mutates a global, so treat every module call as unsafe.
	if _, isModule := s.in.moduleAliases[n.Prefix]; isModule {
		s.unsafe = true
		return
	}
	// Canonicalise the prefix (an alias -> its library name) before the callback
	// check, which keys on canonical names.
	ns := n.Prefix
	if canon, err := s.in.resolveNamespacePrefix(n.Prefix); err == nil {
		ns = canon
	}
	if jCallbackBuiltins[[2]string{ns, n.Callee}] {
		s.unsafe = true
		return
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
