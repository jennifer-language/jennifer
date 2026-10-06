// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import "jennifer-lang.dev/jennifer/internal/parser"

// Spawn-capture analysis. snapshotForSpawn deep-copies the whole global frame
// into every spawn snapshot, so a program holding a large global (a cache, a
// config, a session store) pays that copy on every spawn - including the
// per-request worker a server spawns - even when the body never touches it.
//
// This pass computes, per SpawnExpr, the set of global names the body actually
// needs (directly, and transitively through the user methods it calls) so the
// snapshot copies only those. It is sound by over-approximation: any shape it
// cannot bound - a dynamic / unresolved call, a module call (which can re-enter
// the host), a callback builtin, or an unrecognised node - sets AllGlobals, and
// the snapshot then copies everything, exactly as before. A missed global would
// be a silent wrong result, so every uncertain case falls back to "copy all".
//
// It reuses the call-graph / fixpoint shape of computeEntryGlobalSafe and runs
// in the same single-threaded Run setup (before any spawn), writing SpawnExpr
// .Captures on the shared AST under the one-Run-per-Program assumption.

// methodRefs is the per-method global-reference summary the fixpoint propagates.
type methodRefs struct {
	refs    map[string]bool     // global names the method references directly
	all     bool                // reaches a global it cannot name (dynamic call, etc.)
	callees []*parser.MethodDef // statically-named user-method callees
	spawns  []*parser.SpawnExpr // spawns lexically inside this method (for Captures)
}

// computeSpawnCaptures stamps every SpawnExpr in the program with the globals it
// needs. No-op (leaving Captures nil -> copy all) when the program has no spawn.
func (i *Interpreter) computeSpawnCaptures(prog *parser.Program) {
	// Summarise every method, collecting its direct refs, callees, and nested
	// spawns in one walk.
	summ := make(map[*parser.MethodDef]*methodRefs, len(i.methods))
	anySpawn := false
	for _, m := range i.methods {
		sc := i.scanGlobalRefs(m.Body.Stmts, m.Params)
		summ[m] = &methodRefs{refs: sc.refs, all: sc.all, callees: sc.callees, spawns: sc.spawns}
		if len(sc.spawns) > 0 {
			anySpawn = true
		}
	}
	// Top-level statements can also hold spawns (and reference globals / call
	// methods in a spawn body).
	topRefs := i.scanGlobalRefs(prog.TopLevel, nil)
	if len(topRefs.spawns) > 0 {
		anySpawn = true
	}
	if !anySpawn {
		return // nothing to optimise; every spawn keeps the copy-all default
	}

	// Fixpoint: a method's refs include its callees' refs, and all propagates,
	// until nothing changes (handles recursion / mutual recursion).
	for changed := true; changed; {
		changed = false
		for _, st := range summ {
			if st.all {
				continue
			}
			for _, c := range st.callees {
				cs, ok := summ[c]
				if !ok {
					continue
				}
				if cs.all && !st.all {
					st.all = true
					changed = true
					continue
				}
				for name := range cs.refs {
					if !st.refs[name] {
						st.refs[name] = true
						changed = true
					}
				}
			}
		}
	}

	// Stamp each spawn, anywhere in the program, with its needed set.
	for _, st := range summ {
		for _, sp := range st.spawns {
			i.stampSpawn(sp, summ)
		}
	}
	for _, sp := range topRefs.spawns {
		i.stampSpawn(sp, summ)
	}
}

// stampSpawn computes one spawn body's needed-global set from its own refs plus
// the (fixpointed) refs of the methods it calls, and records it on the node.
func (i *Interpreter) stampSpawn(sp *parser.SpawnExpr, summ map[*parser.MethodDef]*methodRefs) {
	sc := i.scanGlobalRefs(sp.Body, nil)
	caps := &parser.SpawnCaptures{Globals: map[string]bool{}}
	for name := range sc.refs {
		caps.Globals[name] = true
	}
	if sc.all {
		caps.AllGlobals = true
	}
	for _, c := range sc.callees {
		cs, ok := summ[c]
		if !ok {
			caps.AllGlobals = true // an unknown callee could read anything
			continue
		}
		if cs.all {
			caps.AllGlobals = true
		}
		for name := range cs.refs {
			caps.Globals[name] = true
		}
	}
	// A spawn nested inside this one snapshots from this body's frame, so this
	// snapshot must already hold whatever the nested spawn needs. scanGlobalRefs
	// descends into nested spawn bodies, so sc.refs already includes those refs;
	// nothing extra to merge here.
	if caps.AllGlobals {
		caps.Globals = nil
	}
	sp.Captures = caps
}

// refScan accumulates the result of walking one body for global references.
type refScan struct {
	in      *Interpreter
	locals  map[string]bool
	refs    map[string]bool
	all     bool
	callees []*parser.MethodDef
	spawns  []*parser.SpawnExpr
}

// scanGlobalRefs walks stmts (a method or spawn body) and returns the global
// names it references, whether it reaches an un-nameable global, its named
// callees, and the spawns lexically within it. params seeds the local set (for a
// method body); a spawn body passes nil (its own locals are collected as it
// walks, and any enclosing-local name it references is harmlessly over-included,
// since the globals filter ignores a name that is not actually a global).
func (i *Interpreter) scanGlobalRefs(stmts []parser.Stmt, params []parser.Param) *refScan {
	locals := make(map[string]bool, len(params))
	for _, p := range params {
		locals[p.Name] = true
	}
	collectMethodLocals(stmts, locals)
	s := &refScan{in: i, locals: locals, refs: map[string]bool{}}
	s.walkStmts(stmts)
	return s
}

func (s *refScan) use(name string) {
	if name != "" && !s.locals[name] {
		s.refs[name] = true
	}
}

func (s *refScan) walkStmts(stmts []parser.Stmt) {
	for _, st := range stmts {
		s.walkStmt(st)
	}
}

func (s *refScan) walkStmt(st parser.Stmt) {
	switch n := st.(type) {
	case *parser.DefineStmt:
		s.walkExpr(n.InitExpr)
	case *parser.AssignStmt:
		s.use(n.VarName)
		s.walkExpr(n.Value)
	case *parser.IndexAssignStmt:
		s.use(lvalueRoot(n.Target))
		s.walkExpr(n.Target)
		s.walkExpr(n.Value)
	case *parser.AppendStmt:
		if n.Target != nil {
			s.use(n.Target.Name)
		} else {
			s.all = true
		}
		s.walkExpr(n.Value)
	case *parser.FieldAssignStmt:
		s.use(lvalueRoot(n.Target))
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
		// No global reference, no call, no nested body reachable here.
	default:
		s.all = true // unknown statement: cannot bound its references
	}
}

func (s *refScan) walkExpr(e parser.Expr) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *parser.IntLit, *parser.FloatLit, *parser.StringLit, *parser.BoolLit,
		*parser.NullLit, *parser.PreEval, *parser.QualifiedConstRefExpr:
		// Leaves. A QualifiedConstRefExpr is a library constant (math.PI), not a
		// user global, so it needs nothing from the global frame.
	case *parser.VarExpr:
		s.use(n.Name)
	case *parser.ConstRefExpr:
		// A reference to a user global const, or a bare method name used as a func
		// value. Record the name either way: if it is not a global, the filter
		// ignores it; if the func value is later called, that is a CallValueExpr
		// below and sets all.
		s.use(n.Name)
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
		// one; resolve the callee by name against the method table instead. A
		// name that is not a user method is a bare builtin or unresolved call,
		// which could reach any global.
		m := n.Method
		if m == nil {
			m = s.in.methods[n.Callee]
		}
		if m != nil {
			s.callees = append(s.callees, m)
		} else {
			s.all = true
		}
		for _, a := range n.Args {
			s.walkExpr(a)
		}
	case *parser.QualifiedCallExpr:
		s.qualifiedCall(n)
	case *parser.CallValueExpr:
		s.all = true // dynamic dispatch through a func value: unknown target
	case *parser.SpawnExpr:
		// Descend into a nested spawn: it snapshots from this frame, so this
		// snapshot must hold whatever the nested one needs. Its own locals are
		// collected as the walk descends; its global refs join this body's.
		s.spawns = append(s.spawns, n)
		s.walkStmts(n.Body)
	default:
		s.all = true // unknown expression: cannot bound its references
	}
}

func (s *refScan) qualifiedCall(n *parser.QualifiedCallExpr) {
	if _, isModule := s.in.moduleAliases[n.Prefix]; isModule {
		s.all = true // a module call can re-enter the host and read any global
		return
	}
	ns := n.Prefix
	if canon, err := s.in.resolveNamespacePrefix(n.Prefix); err == nil {
		ns = canon
	}
	if jCallbackBuiltins[[2]string{ns, n.Callee}] {
		s.all = true // runs arbitrary .j code, which may read any global
		return
	}
	for _, a := range n.Args {
		s.walkExpr(a)
	}
}
