// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import "jennifer-lang.dev/jennifer/internal/parser"

// The parser exposes no generic visitor, so the linter carries its own. Two
// traversals live here: a flat walker (walker) for checks that match node
// shapes without caring about scope, and a scope-aware traversal (scope.go)
// for the checks that need binding visibility. Both mirror the exhaustive
// type switches in internal/parser (resolver.go / ast.go Sprint) - when a new
// AST node lands, both must grow a case.

// walker performs a preorder traversal, invoking the non-nil hooks. list
// fires once per statement list (block bodies, spawn bodies, top level);
// stmt fires per statement; expr fires per expression. It descends into
// spawn bodies (unlike the resolver) so checks see the whole program.
type walker struct {
	list func([]parser.Stmt)
	stmt func(parser.Stmt)
	expr func(parser.Expr)
}

// program drives the walk over a whole program: top-level statements first,
// then every method body. Struct definitions carry no statements to lint.
func (w walker) program(p *parser.Program) {
	w.stmts(p.TopLevel)
	for _, m := range p.Methods {
		w.block(m.Body)
	}
}

func (w walker) block(b *parser.Block) {
	if b == nil {
		return
	}
	w.stmts(b.Stmts)
}

func (w walker) stmts(ss []parser.Stmt) {
	if w.list != nil {
		w.list(ss)
	}
	for _, s := range ss {
		w.doStmt(s)
	}
}

func (w walker) doStmt(s parser.Stmt) {
	if s == nil {
		return
	}
	if w.stmt != nil {
		w.stmt(s)
	}
	switch n := s.(type) {
	case *parser.DefineStmt:
		w.doExpr(n.InitExpr)
	case *parser.AssignStmt:
		w.doExpr(n.Value)
	case *parser.IndexAssignStmt:
		w.doExpr(n.Target)
		w.doExpr(n.Value)
	case *parser.AppendStmt:
		w.doExpr(n.Target)
		w.doExpr(n.Value)
	case *parser.FieldAssignStmt:
		w.doExpr(n.Target)
		w.doExpr(n.Value)
	case *parser.IfStmt:
		w.doExpr(n.Cond)
		w.block(n.Then)
		for i := range n.ElseIfs {
			w.doExpr(n.ElseIfs[i])
			w.block(n.ElseIfBodies[i])
		}
		w.block(n.Else)
	case *parser.WhileStmt:
		w.doExpr(n.Cond)
		w.block(n.Body)
	case *parser.ForStmt:
		w.doStmt(n.Init)
		w.doExpr(n.Cond)
		w.doStmt(n.Step)
		w.block(n.Body)
	case *parser.ForEachStmt:
		w.doExpr(n.Coll)
		w.block(n.Body)
	case *parser.RepeatStmt:
		w.block(n.Body)
		w.doExpr(n.Cond)
	case *parser.ReturnStmt:
		w.doExpr(n.Value)
	case *parser.ExitStmt:
		w.doExpr(n.Code)
	case *parser.ThrowStmt:
		w.doExpr(n.Value)
	case *parser.DeferStmt:
		// The deferred call (and its arguments) is ordinary expression
		// territory - checks must see it like any other call.
		w.doExpr(n.Call)
	case *parser.TryStmt:
		w.block(n.Body)
		w.block(n.CatchBody)
	case *parser.ExprStmt:
		w.doExpr(n.Expr)
	case *parser.BreakStmt, *parser.ContinueStmt:
		// leaves - no children
	}
}

func (w walker) doExpr(e parser.Expr) {
	if e == nil {
		return
	}
	if w.expr != nil {
		w.expr(e)
	}
	switch n := e.(type) {
	case *parser.ListLit:
		for _, el := range n.Elements {
			w.doExpr(el)
		}
	case *parser.MapLit:
		for i := range n.Keys {
			w.doExpr(n.Keys[i])
			w.doExpr(n.Values[i])
		}
	case *parser.StructLit:
		for _, f := range n.Fields {
			w.doExpr(f.Expr)
		}
	case *parser.IndexExpr:
		w.doExpr(n.Target)
		w.doExpr(n.Index)
	case *parser.FieldAccessExpr:
		w.doExpr(n.Target)
	case *parser.CallExpr:
		for _, a := range n.Args {
			w.doExpr(a)
		}
	case *parser.QualifiedCallExpr:
		for _, a := range n.Args {
			w.doExpr(a)
		}
	case *parser.LenExpr:
		w.doExpr(n.Operand)
	case *parser.SpawnExpr:
		w.stmts(n.Body)
	case *parser.BinaryExpr:
		// Folded is a derived duplicate of the subtree; walking Left/Right
		// is enough and avoids double-visiting a folded literal.
		w.doExpr(n.Left)
		w.doExpr(n.Right)
	case *parser.UnaryExpr:
		w.doExpr(n.Operand)
		// literals, VarExpr, ConstRefExpr, QualifiedConstRefExpr: leaves
	}
}
