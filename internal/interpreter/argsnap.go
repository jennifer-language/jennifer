// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import "jennifer-lang.dev/jennifer/internal/parser"

// Argument snapshots. Value semantics say an argument is the value its
// expression had when it was evaluated. Evaluating a variable does not copy its
// backing, though: the copy happens when the argument is bound (a copying
// parameter bind), or never (a borrowed parameter, a builtin, which only reads).
// Arguments are evaluated left to right, so in `f($g, mutatesG())` the call in
// the second argument runs after `$g` was evaluated but before the call uses it,
// and an in-place write to the global would show through. stampArgSnapshots
// finds those calls once, at load, and marks the earlier arguments
// (CallExpr.ArgSnap and friends); the call paths copy a marked argument right
// after evaluating it (and never borrow it).
//
// Only the entry program of a script with mutable globals needs this: an
// argument can only alias a global there (a method local is private; a borrowed
// local or parameter lives in a GlobalSafe method, whose body holds no
// global-mutating call), and a script without mutable globals has nothing to
// mutate.

// stampArgSnapshots marks the arguments of every call in prog that alias a
// global while a later argument of the same call may mutate globals. Runs after
// computeEntryGlobalSafe (it reads MethodDef.GlobalSafe).
func (i *Interpreter) stampArgSnapshots(prog *parser.Program) {
	if i.isModule || prog == nil {
		return
	}
	w := &argSnapWalker{in: i, globals: map[string]bool{}}
	for _, s := range prog.TopLevel {
		if d, ok := s.(*parser.DefineStmt); ok && !d.IsConst {
			w.globals[d.VarName] = true
		}
	}
	w.stmts(prog.TopLevel)
	for _, m := range prog.Methods {
		if m != nil && m.Body != nil {
			w.stmts(m.Body.Stmts)
		}
	}
}

// snapArg returns the argument value a call should use: a private copy when the
// call's ArgSnap marks it, else v itself.
func snapArg(v Value, snap []bool, idx int) Value {
	if snap != nil && snap[idx] {
		return v.Copy()
	}
	return v
}

type argSnapWalker struct {
	in      *Interpreter
	globals map[string]bool // the mutable top-level variables
}

// snaps computes a call's ArgSnap: each argument that aliases a global and is
// followed by an argument that may mutate globals. nil when there is none.
func (w *argSnapWalker) snaps(args []parser.Expr) []bool {
	var out []bool
	hazardAfter := false
	for k := len(args) - 1; k >= 0; k-- {
		if hazardAfter && w.aliasesGlobal(args[k]) {
			if out == nil {
				out = make([]bool, len(args))
			}
			out[k] = true
		}
		if !hazardAfter && w.mayMutateGlobals(args[k]) {
			hazardAfter = true
		}
	}
	return out
}

// aliasesGlobal reports whether e evaluates to (part of) a global's backing
// without copying it: the global itself, or an element / field reached from it.
func (w *argSnapWalker) aliasesGlobal(e parser.Expr) bool {
	switch n := e.(type) {
	case *parser.VarExpr:
		return w.globals[n.Name]
	case *parser.IndexExpr:
		return w.aliasesGlobal(n.Target)
	case *parser.FieldAccessExpr:
		return w.aliasesGlobal(n.Target)
	}
	return false
}

// mayMutateGlobals reports whether evaluating e may run code that writes a
// global: a call to a method not proven GlobalSafe, a dynamic or unresolved
// call, a module call (meta.callMain re-entry), a callback builtin, or a spawn.
// Unknown nodes count as mutating.
func (w *argSnapWalker) mayMutateGlobals(e parser.Expr) bool {
	if e == nil {
		return false
	}
	switch n := e.(type) {
	case *parser.IntLit, *parser.FloatLit, *parser.StringLit, *parser.BoolLit,
		*parser.NullLit, *parser.VarExpr, *parser.ConstRefExpr,
		*parser.QualifiedConstRefExpr, *parser.PreEval:
		return false
	case *parser.BinaryExpr:
		return w.mayMutateGlobals(n.Left) || w.mayMutateGlobals(n.Right)
	case *parser.UnaryExpr:
		return w.mayMutateGlobals(n.Operand)
	case *parser.LenExpr:
		return w.mayMutateGlobals(n.Operand)
	case *parser.IndexExpr:
		return w.mayMutateGlobals(n.Target) || w.mayMutateGlobals(n.Index)
	case *parser.FieldAccessExpr:
		return w.mayMutateGlobals(n.Target)
	case *parser.RangeExpr:
		return w.mayMutateGlobals(n.Lo) || w.mayMutateGlobals(n.Hi)
	case *parser.SliceExpr:
		return w.mayMutateGlobals(n.Target) || w.mayMutateGlobals(n.Lo) || w.mayMutateGlobals(n.Hi)
	case *parser.ListLit:
		return w.anyMutates(n.Elements)
	case *parser.MapLit:
		return w.anyMutates(n.Keys) || w.anyMutates(n.Values)
	case *parser.StructLit:
		for _, f := range n.Fields {
			if w.mayMutateGlobals(f.Expr) {
				return true
			}
		}
		return false
	case *parser.InterpStringExpr:
		for _, p := range n.Parts {
			if w.mayMutateGlobals(p.Expr) {
				return true
			}
		}
		return false
	case *parser.CallExpr:
		if n.Method == nil || !n.Method.GlobalSafe {
			return true
		}
		return w.anyMutates(n.Args)
	case *parser.QualifiedCallExpr:
		if _, isModule := w.in.moduleAliases[n.Prefix]; isModule {
			return true
		}
		ns := n.Prefix
		if canon, err := w.in.resolveNamespacePrefix(n.Prefix); err == nil {
			ns = canon
		}
		if jCallbackBuiltins[[2]string{ns, n.Callee}] {
			return true
		}
		return w.anyMutates(n.Args)
	case *parser.CallValueExpr, *parser.SpawnExpr:
		return true
	}
	return true
}

func (w *argSnapWalker) anyMutates(es []parser.Expr) bool {
	for _, e := range es {
		if w.mayMutateGlobals(e) {
			return true
		}
	}
	return false
}

func (w *argSnapWalker) stmts(stmts []parser.Stmt) {
	for _, s := range stmts {
		w.stmt(s)
	}
}

func (w *argSnapWalker) block(b *parser.Block) {
	if b != nil {
		w.stmts(b.Stmts)
	}
}

func (w *argSnapWalker) stmt(s parser.Stmt) {
	switch st := s.(type) {
	case *parser.DefineStmt:
		w.expr(st.InitExpr)
	case *parser.AssignStmt:
		w.expr(st.Value)
	case *parser.IndexAssignStmt:
		w.expr(st.Target)
		w.expr(st.Value)
	case *parser.AppendStmt:
		w.expr(st.Target)
		w.expr(st.Value)
	case *parser.FieldAssignStmt:
		w.expr(st.Target)
		w.expr(st.Value)
	case *parser.IfStmt:
		w.expr(st.Cond)
		w.block(st.Then)
		for idx := range st.ElseIfs {
			w.expr(st.ElseIfs[idx])
			w.block(st.ElseIfBodies[idx])
		}
		w.block(st.Else)
	case *parser.WhileStmt:
		w.expr(st.Cond)
		w.block(st.Body)
	case *parser.ForStmt:
		if st.Init != nil {
			w.stmt(st.Init)
		}
		w.expr(st.Cond)
		if st.Step != nil {
			w.stmt(st.Step)
		}
		w.block(st.Body)
	case *parser.ForEachStmt:
		w.expr(st.Coll)
		w.block(st.Body)
	case *parser.RepeatStmt:
		w.block(st.Body)
		w.expr(st.Cond)
	case *parser.ReturnStmt:
		w.expr(st.Value)
	case *parser.ExitStmt:
		w.expr(st.Code)
	case *parser.ThrowStmt:
		w.expr(st.Value)
	case *parser.TryStmt:
		w.block(st.Body)
		w.block(st.CatchBody)
	case *parser.MatchStmt:
		w.expr(st.Subject)
		for idx := range st.Arms {
			for _, v := range st.Arms[idx].Values {
				w.expr(v)
			}
			w.block(st.Arms[idx].Body)
		}
		w.block(st.Else)
	case *parser.DeferStmt:
		w.expr(st.Call)
	case *parser.ExprStmt:
		w.expr(st.Expr)
	case *parser.Block:
		w.block(st)
	case *parser.BreakStmt, *parser.ContinueStmt, *parser.ImportStmt,
		*parser.ModuleImportStmt, *parser.StructDef, *parser.EnumDef,
		*parser.MethodDef:
		// No expressions to visit.
	}
}

func (w *argSnapWalker) exprs(es []parser.Expr) {
	for _, e := range es {
		w.expr(e)
	}
}

func (w *argSnapWalker) expr(e parser.Expr) {
	switch n := e.(type) {
	case *parser.CallExpr:
		n.ArgSnap = w.snaps(n.Args)
		w.exprs(n.Args)
	case *parser.CallValueExpr:
		n.ArgSnap = w.snaps(n.Args)
		w.expr(n.Callee)
		w.exprs(n.Args)
	case *parser.QualifiedCallExpr:
		n.ArgSnap = w.snaps(n.Args)
		w.exprs(n.Args)
	case *parser.BinaryExpr:
		w.expr(n.Left)
		w.expr(n.Right)
	case *parser.UnaryExpr:
		w.expr(n.Operand)
	case *parser.LenExpr:
		w.expr(n.Operand)
	case *parser.IndexExpr:
		w.expr(n.Target)
		w.expr(n.Index)
	case *parser.FieldAccessExpr:
		w.expr(n.Target)
	case *parser.RangeExpr:
		w.expr(n.Lo)
		w.expr(n.Hi)
	case *parser.SliceExpr:
		w.expr(n.Target)
		w.expr(n.Lo)
		w.expr(n.Hi)
	case *parser.ListLit:
		w.exprs(n.Elements)
	case *parser.MapLit:
		w.exprs(n.Keys)
		w.exprs(n.Values)
	case *parser.StructLit:
		for _, f := range n.Fields {
			w.expr(f.Expr)
		}
	case *parser.InterpStringExpr:
		for _, p := range n.Parts {
			w.expr(p.Expr)
		}
	case *parser.SpawnExpr:
		w.stmts(n.Body)
	}
}
