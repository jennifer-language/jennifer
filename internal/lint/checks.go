// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkUnusedLocal (L101) flags a local `def` binding that is never read.
// It reuses the shared scope walk: a binding is reportable only when it is a
// local `def` (not a param, for-each iterator, catch var, or global) and is
// not declared inside a spawn body (the resolver's spawn carve-out means the
// linter inherits the same blind spot for spawn-local declarations). A read
// anywhere - including inside a spawn body that captured the enclosing scope
// - marks the binding used, so outer locals used only from a spawn are not
// falsely flagged.
func checkUnusedLocal(c *checkCtx) {
	s := &scoped{}
	s.onPop = func(f *sframe) {
		for _, b := range f.order {
			if !b.reportable || b.used {
				continue
			}
			noun := "local"
			if d, ok := b.def.(*parser.DefineStmt); ok && d.IsConst {
				noun = "constant"
			}
			c.report("L101", b.def, fmt.Sprintf("%s `%s` is declared but never used", noun, b.name))
		}
	}
	s.program(c.prog)
}

// checkDeadCode (L102) flags the first statement made unreachable by a
// preceding terminator (return / throw / exit / break / continue) in the same
// statement list. Reported once per list; nested lists are visited on their
// own.
func checkDeadCode(c *checkCtx) {
	w := walker{list: func(ss []parser.Stmt) {
		for i := 0; i+1 < len(ss); i++ {
			if name, ok := terminatorName(ss[i]); ok {
				c.report("L102", ss[i+1], fmt.Sprintf("unreachable code after `%s`", name))
				return
			}
		}
	}}
	w.program(c.prog)
}

// terminatorName reports whether s unconditionally ends its statement list,
// and the keyword to name in the diagnostic.
func terminatorName(s parser.Stmt) (string, bool) {
	switch s.(type) {
	case *parser.ReturnStmt:
		return "return", true
	case *parser.ThrowStmt:
		return "throw", true
	case *parser.ExitStmt:
		return "exit", true
	case *parser.BreakStmt:
		return "break", true
	case *parser.ContinueStmt:
		return "continue", true
	}
	return "", false
}

// checkEmptyCatch (L103) flags a catch block with no body: the handler
// receives the error and discards it silently. Anchored at the catch
// introducer.
func checkEmptyCatch(c *checkCtx) {
	w := walker{stmt: func(st parser.Stmt) {
		t, ok := st.(*parser.TryStmt)
		if !ok {
			return
		}
		if t.CatchBody == nil || len(t.CatchBody.Stmts) == 0 {
			c.reportAt("L103", t.CatchFile, t.CatchLine, t.CatchCol,
				fmt.Sprintf("empty catch block silently discards the error `%s`", t.CatchName))
		}
	}}
	w.program(c.prog)
}

// checkThrowNonError (L104) flags a throw whose value is not statically an
// Error struct. The convention is that user code throws `Error` so catch
// handlers can rely on its fields; bare-value throws break that contract.
// Only shapes the linter can decide without type inference are judged: an
// `Error{...}` literal passes, a `$var` whose declared type is `Error`
// passes, everything else is flagged.
func checkThrowNonError(c *checkCtx) {
	s := &scoped{}
	s.onThrow = func(t *parser.ThrowStmt, resolve func(string) *binding) {
		if throwsError(t.Value, resolve) {
			return
		}
		anchor := parser.Node(t)
		if t.Value != nil {
			anchor = t.Value
		}
		c.report("L104", anchor,
			"throw of a non-Error value; throw an `Error` struct so catch handlers can rely on its fields")
	}
	s.program(c.prog)
}

// throwsError reports whether e is statically known to be an Error value.
func throwsError(e parser.Expr, resolve func(string) *binding) bool {
	switch n := e.(type) {
	case *parser.StructLit:
		return n.NS == "" && n.Name == "Error"
	case *parser.VarExpr:
		b := resolve(n.Name)
		return b != nil && isErrorType(b.typ)
	}
	return false
}

// isErrorType reports whether t is the auto-hoisted user-visible Error struct.
func isErrorType(t parser.Type) bool {
	return t.Kind == parser.TypeStruct && t.StructName == "Error" && t.StructNS == ""
}

// checkMethodTooLong (L201) flags a method whose body exceeds the statement
// threshold - the "one concern per method" heuristic from the style guide.
func checkMethodTooLong(c *checkCtx) {
	for _, m := range c.prog.Methods {
		n := countStmts(m.Body)
		if n > c.cfg.MethodMaxStmts {
			c.report("L201", m, fmt.Sprintf(
				"method `%s` has %d statements, over the limit of %d; consider splitting it",
				m.Name, n, c.cfg.MethodMaxStmts))
		}
	}
}

// countStmts totals the statements in a block, recursively.
func countStmts(b *parser.Block) int {
	count := 0
	w := walker{stmt: func(parser.Stmt) { count++ }}
	w.block(b)
	return count
}

// checkNestingTooDeep (L202) flags a block whose nesting depth first exceeds
// the limit. Method bodies start at depth 1, top-level statements at depth 0;
// each control-flow block adds one. Reported once at the shallowest violating
// block so a deeply nested method yields a single finding per entry point.
func checkNestingTooDeep(c *checkCtx) {
	c.nestStmts(c.prog.TopLevel, 0)
	for _, m := range c.prog.Methods {
		if m.Body != nil {
			c.nestStmts(m.Body.Stmts, 1)
		}
	}
}

func (c *checkCtx) nestStmts(ss []parser.Stmt, depth int) {
	for _, st := range ss {
		c.nestStmt(st, depth)
	}
}

func (c *checkCtx) nestStmt(st parser.Stmt, depth int) {
	switch n := st.(type) {
	case *parser.IfStmt:
		d := depth + 1
		c.maybeReportNest(n, d)
		c.nestExpr(n.Cond, depth)
		c.nestStmts(n.Then.Stmts, d)
		for i := range n.ElseIfBodies {
			c.nestExpr(n.ElseIfs[i], depth)
			c.nestStmts(n.ElseIfBodies[i].Stmts, d)
		}
		if n.Else != nil {
			c.nestStmts(n.Else.Stmts, d)
		}
	case *parser.WhileStmt:
		d := depth + 1
		c.maybeReportNest(n, d)
		c.nestExpr(n.Cond, depth)
		c.nestStmts(n.Body.Stmts, d)
	case *parser.ForStmt:
		d := depth + 1
		c.maybeReportNest(n, d)
		c.nestStmt(n.Init, depth)
		c.nestExpr(n.Cond, depth)
		c.nestStmt(n.Step, depth)
		c.nestStmts(n.Body.Stmts, d)
	case *parser.ForEachStmt:
		d := depth + 1
		c.maybeReportNest(n, d)
		c.nestExpr(n.Coll, depth)
		if n.Body != nil {
			c.nestStmts(n.Body.Stmts, d)
		}
	case *parser.RepeatStmt:
		d := depth + 1
		c.maybeReportNest(n, d)
		c.nestStmts(n.Body.Stmts, d)
		c.nestExpr(n.Cond, depth)
	case *parser.TryStmt:
		d := depth + 1
		c.maybeReportNest(n, d)
		if n.Body != nil {
			c.nestStmts(n.Body.Stmts, d)
		}
		if n.CatchBody != nil {
			c.nestStmts(n.CatchBody.Stmts, d)
		}
	case *parser.DefineStmt:
		c.nestExpr(n.InitExpr, depth)
	case *parser.AssignStmt:
		c.nestExpr(n.Value, depth)
	case *parser.IndexAssignStmt:
		c.nestExpr(n.Target, depth)
		c.nestExpr(n.Value, depth)
	case *parser.AppendStmt:
		c.nestExpr(n.Target, depth)
		c.nestExpr(n.Value, depth)
	case *parser.FieldAssignStmt:
		c.nestExpr(n.Target, depth)
		c.nestExpr(n.Value, depth)
	case *parser.ReturnStmt:
		c.nestExpr(n.Value, depth)
	case *parser.ThrowStmt:
		c.nestExpr(n.Value, depth)
	case *parser.ExitStmt:
		c.nestExpr(n.Code, depth)
	case *parser.DeferStmt:
		// A spawn can hide in a deferred call's arguments
		// (`defer f(spawn { ... });`) - count its body like any other.
		c.nestExpr(n.Call, depth)
	case *parser.ExprStmt:
		c.nestExpr(n.Expr, depth)
	}
}

// nestExpr descends into any spawn bodies reachable from e, counting each
// spawn block as one nesting level - the same way a control-flow block counts.
// The resolver skips spawn bodies, so without this a deeply-nested spawn body
// would go unflagged by L202.
func (c *checkCtx) nestExpr(e parser.Expr, depth int) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *parser.SpawnExpr:
		d := depth + 1
		c.maybeReportNest(n, d)
		c.nestStmts(n.Body, d)
	case *parser.ListLit:
		for _, el := range n.Elements {
			c.nestExpr(el, depth)
		}
	case *parser.MapLit:
		for i := range n.Keys {
			c.nestExpr(n.Keys[i], depth)
			c.nestExpr(n.Values[i], depth)
		}
	case *parser.StructLit:
		for _, f := range n.Fields {
			c.nestExpr(f.Expr, depth)
		}
	case *parser.IndexExpr:
		c.nestExpr(n.Target, depth)
		c.nestExpr(n.Index, depth)
	case *parser.FieldAccessExpr:
		c.nestExpr(n.Target, depth)
	case *parser.CallExpr:
		for _, a := range n.Args {
			c.nestExpr(a, depth)
		}
	case *parser.QualifiedCallExpr:
		for _, a := range n.Args {
			c.nestExpr(a, depth)
		}
	case *parser.LenExpr:
		c.nestExpr(n.Operand, depth)
	case *parser.BinaryExpr:
		c.nestExpr(n.Left, depth)
		c.nestExpr(n.Right, depth)
	case *parser.UnaryExpr:
		c.nestExpr(n.Operand, depth)
	}
}

// maybeReportNest fires only at the shallowest depth that breaks the limit,
// so nesting deeper still yields just the one finding.
func (c *checkCtx) maybeReportNest(anchor parser.Node, depth int) {
	if depth == c.cfg.MaxNesting+1 {
		c.report("L202", anchor, fmt.Sprintf(
			"block nesting reaches depth %d, over the limit of %d; flatten with early returns or helper methods",
			depth, c.cfg.MaxNesting))
	}
}

// removedLibraries maps a removed library name to a short removal notice.
// The message stays terse - the linter flags the use at dev time; the full
// migration detail (what replaced it) lives in the runtime error the
// interpreter still raises for the same import, and in the docs. Entries are
// added here as libraries are retired.
var removedLibraries = map[string]string{
	"core": "the `core` library was removed",
}

// checkRemovedApi (L302) flags use of an API that has been removed, naming the
// successor. It parallels L301 (deprecation) but for names that are already
// gone rather than merely on the way out. v1 covers removed library imports.
func checkRemovedApi(c *checkCtx) {
	for _, imp := range c.prog.Imports {
		if msg, ok := removedLibraries[imp.Name]; ok {
			c.report("L302", imp, msg)
		}
	}
}

// checkLineTooLong (L203) flags a source line longer than the column limit
// (the style guide's 100-column recommendation). Columns are counted in runes
// to match the lexer, and the finding is anchored at the first column past the
// limit. Line-oriented, so it runs on the primary file's raw text; findings in
// included files are out of scope for v1.
func checkLineTooLong(c *checkCtx) {
	if c.cfg.MaxLineLength <= 0 || c.source == "" {
		return
	}
	for i, line := range strings.Split(c.source, "\n") {
		line = strings.TrimSuffix(line, "\r")
		n := utf8.RuneCountInString(line)
		if n > c.cfg.MaxLineLength {
			c.reportAt("L203", c.sourceFile, i+1, c.cfg.MaxLineLength+1,
				fmt.Sprintf("line is %d columns, over the limit of %d", n, c.cfg.MaxLineLength))
		}
	}
}

// checkConstantCondition (L105) flags conditions a reader can see are
// statically constant: a bool literal, or a comparison of a value with
// itself. `while (true)` is left alone when the body can break or otherwise
// escape the loop - the deliberate spin-loop idiom.
func checkConstantCondition(c *checkCtx) {
	w := walker{stmt: func(st parser.Stmt) {
		switch n := st.(type) {
		case *parser.IfStmt:
			c.reportConstCond(n.Cond)
			for _, ec := range n.ElseIfs {
				c.reportConstCond(ec)
			}
		case *parser.WhileStmt:
			if b, ok := n.Cond.(*parser.BoolLit); ok && b.Value && loopCanEscape(n.Body) {
				// while (true) { ... break/return/... } - intentional.
				return
			}
			c.reportConstCond(n.Cond)
		case *parser.RepeatStmt:
			// repeat { ... } until (false) is the post-test spin-loop idiom -
			// leave it alone when the body can break / return out.
			if b, ok := n.Cond.(*parser.BoolLit); ok && !b.Value && loopCanEscape(n.Body) {
				return
			}
			c.reportConstCond(n.Cond)
		}
	}}
	w.program(c.prog)
}

func (c *checkCtx) reportConstCond(cond parser.Expr) {
	if msg, ok := constantCond(cond); ok {
		c.report("L105", cond, msg)
	}
}

// constantCond reports whether cond is statically constant and a message.
func constantCond(e parser.Expr) (string, bool) {
	switch n := e.(type) {
	case *parser.BoolLit:
		if n.Value {
			return "condition is always true", true
		}
		return "condition is always false", true
	case *parser.BinaryExpr:
		if n.Op.IsComparison() && sameSimpleExpr(n.Left, n.Right) {
			switch n.Op {
			case parser.OpEq, parser.OpLe, parser.OpGe:
				return "comparison of a value with itself is always true", true
			case parser.OpLt, parser.OpGt, parser.OpNeq:
				return "comparison of a value with itself is always false", true
			}
		}
	}
	return "", false
}

// sameSimpleExpr reports whether a and b are the identical simple expression
// (same variable, constant, or literal). Deliberately conservative: it never
// claims two calls or two compound expressions are equal, so `f() == f()` is
// not flagged.
func sameSimpleExpr(a, b parser.Expr) bool {
	switch x := a.(type) {
	case *parser.VarExpr:
		y, ok := b.(*parser.VarExpr)
		return ok && x.Name == y.Name
	case *parser.ConstRefExpr:
		y, ok := b.(*parser.ConstRefExpr)
		return ok && x.Name == y.Name
	case *parser.IntLit:
		y, ok := b.(*parser.IntLit)
		return ok && x.Value == y.Value
	case *parser.StringLit:
		y, ok := b.(*parser.StringLit)
		return ok && x.Value == y.Value
	case *parser.BoolLit:
		y, ok := b.(*parser.BoolLit)
		return ok && x.Value == y.Value
	}
	return false
}

// loopCanEscape reports whether a loop body contains a statement that can end
// the loop: a break not owned by a nested loop, or any return / throw / exit
// (which escape regardless of nesting). Used to spare the `while (true)`
// spin-loop idiom from L105.
func loopCanEscape(b *parser.Block) bool {
	if b == nil {
		return false
	}
	escaped := false
	var walk func(ss []parser.Stmt, inNestedLoop bool)
	walk = func(ss []parser.Stmt, inNestedLoop bool) {
		for _, st := range ss {
			switch n := st.(type) {
			case *parser.BreakStmt:
				if !inNestedLoop {
					escaped = true
				}
			case *parser.ReturnStmt, *parser.ThrowStmt, *parser.ExitStmt:
				escaped = true
			case *parser.IfStmt:
				walk(n.Then.Stmts, inNestedLoop)
				for i := range n.ElseIfBodies {
					walk(n.ElseIfBodies[i].Stmts, inNestedLoop)
				}
				if n.Else != nil {
					walk(n.Else.Stmts, inNestedLoop)
				}
			case *parser.TryStmt:
				if n.Body != nil {
					walk(n.Body.Stmts, inNestedLoop)
				}
				if n.CatchBody != nil {
					walk(n.CatchBody.Stmts, inNestedLoop)
				}
			case *parser.WhileStmt:
				walk(n.Body.Stmts, true)
			case *parser.ForStmt:
				walk(n.Body.Stmts, true)
			case *parser.ForEachStmt:
				if n.Body != nil {
					walk(n.Body.Stmts, true)
				}
			case *parser.RepeatStmt:
				walk(n.Body.Stmts, true)
			}
		}
	}
	walk(b.Stmts, false)
	return escaped
}
