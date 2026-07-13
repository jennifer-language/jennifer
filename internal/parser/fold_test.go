// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import "testing"

// Constant-fold tests. Resolve() runs the fold pass on
// BinaryExpr / UnaryExpr; when both operands collapse to literals,
// the .Folded field on the AST node carries a pre-computed literal
// the interpreter returns instead of walking the operand tree.

func mustResolve(t *testing.T, src string) *Program {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := Resolve(prog); err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	return prog
}

// firstStmtExpr pulls the RHS Expr out of the first top-level
// `def x as ... init EXPR;` in the program, which is the shape the
// tests use to plant a fold candidate at a known location.
func firstStmtExpr(t *testing.T, prog *Program) Expr {
	t.Helper()
	if len(prog.TopLevel) == 0 {
		t.Fatalf("no top-level statements")
	}
	def, ok := prog.TopLevel[0].(*DefineStmt)
	if !ok {
		t.Fatalf("first top-level not a DefineStmt: %T", prog.TopLevel[0])
	}
	return def.InitExpr
}

func TestFoldIntArithmetic(t *testing.T) {
	prog := mustResolve(t, `def r as int init 1 + 2 * 3 + 4;`)
	root := firstStmtExpr(t, prog)
	bin, ok := root.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr root, got %T", root)
	}
	if bin.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := bin.Folded.(*IntLit)
	if !ok || lit.Value != 11 {
		t.Errorf("Folded: got %+v, want IntLit(11)", bin.Folded)
	}
}

func TestFoldUnaryNeg(t *testing.T) {
	prog := mustResolve(t, `def r as int init -(5 + 3);`)
	root := firstStmtExpr(t, prog)
	u, ok := root.(*UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", root)
	}
	if u.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := u.Folded.(*IntLit)
	if !ok || lit.Value != -8 {
		t.Errorf("Folded: got %+v, want IntLit(-8)", u.Folded)
	}
}

func TestFoldMixedIntFloat(t *testing.T) {
	prog := mustResolve(t, `def r as float init 1 + 2.5;`)
	root := firstStmtExpr(t, prog)
	bin := root.(*BinaryExpr)
	if bin.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := bin.Folded.(*FloatLit)
	if !ok || lit.Value != 3.5 {
		t.Errorf("Folded: got %+v, want FloatLit(3.5)", bin.Folded)
	}
}

func TestFoldStringConcat(t *testing.T) {
	prog := mustResolve(t, `def r as string init "hi" + " world";`)
	root := firstStmtExpr(t, prog)
	bin := root.(*BinaryExpr)
	if bin.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := bin.Folded.(*StringLit)
	if !ok || lit.Value != "hi world" {
		t.Errorf("Folded: got %+v, want StringLit(\"hi world\")", bin.Folded)
	}
}

func TestFoldComparison(t *testing.T) {
	prog := mustResolve(t, `def r as bool init 5 < 10;`)
	root := firstStmtExpr(t, prog)
	bin := root.(*BinaryExpr)
	if bin.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := bin.Folded.(*BoolLit)
	if !ok || lit.Value != true {
		t.Errorf("Folded: got %+v, want BoolLit(true)", bin.Folded)
	}
}

// Two ints one apart, both above 2^53, are not equal. Folding them
// through float64 would round both to the same double and wrongly yield
// true; the fold path compares ints exactly, matching runtime semantics.
func TestFoldLargeIntComparisonExact(t *testing.T) {
	prog := mustResolve(t, `def r as bool init 9007199254740993 == 9007199254740992;`)
	root := firstStmtExpr(t, prog)
	bin := root.(*BinaryExpr)
	if bin.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := bin.Folded.(*BoolLit)
	if !ok || lit.Value != false {
		t.Errorf("Folded: got %+v, want BoolLit(false)", bin.Folded)
	}
}

func TestFoldBitOps(t *testing.T) {
	prog := mustResolve(t, `def r as int init 0xff & 0x0f;`)
	root := firstStmtExpr(t, prog)
	bin := root.(*BinaryExpr)
	if bin.Folded == nil {
		t.Fatalf("expected Folded literal, got nil")
	}
	lit, ok := bin.Folded.(*IntLit)
	if !ok || lit.Value != 15 {
		t.Errorf("Folded: got %+v, want IntLit(15)", bin.Folded)
	}
}

func TestFoldSkipsDivisionByZero(t *testing.T) {
	prog := mustResolve(t, `def r as int init 10 // 0;`)
	root := firstStmtExpr(t, prog)
	bin := root.(*BinaryExpr)
	// Folding must NOT run on an operation that would error at
	// runtime - the runtime should hit the error at its actual
	// source position, not surface it as a parse-time diagnostic
	// (that would change the exception's file/line).
	if bin.Folded != nil {
		t.Errorf("expected Folded nil for div-by-zero, got %+v", bin.Folded)
	}
}

func TestFoldSkipsNonLiteralOperand(t *testing.T) {
	prog := mustResolve(t, `def n as int init 5; def r as int init $n + 1;`)
	if len(prog.TopLevel) < 2 {
		t.Fatalf("expected 2 top-level defs, got %d", len(prog.TopLevel))
	}
	def := prog.TopLevel[1].(*DefineStmt)
	bin := def.InitExpr.(*BinaryExpr)
	// $n is a runtime variable, so nothing to fold.
	if bin.Folded != nil {
		t.Errorf("expected Folded nil when operand is a VarExpr, got %+v", bin.Folded)
	}
}
