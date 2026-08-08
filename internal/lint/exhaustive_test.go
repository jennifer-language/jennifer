// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// The lint package hand-rolls several AST type-switch traversals (the parser
// exposes no generic visitor). When a new node type is added to the parser, or
// an existing one gains children, each traversal must grow a case - forgetting
// one makes a check silently skip that node's whole subtree (the class of bug
// behind the slice-bound / match-arm false positives). This test enumerates
// every parser Expr / Stmt node type and asserts each general traversal either
// handles it or lists it as an intentional leaf, so a missed case fails loudly
// here instead of mis-linting in the field.

// markerReceivers parses the parser package and returns the set of type names
// whose pointer receiver defines the given marker method (exprNode / stmtNode).
func markerReceivers(t *testing.T, marker string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "parser"), nil, 0)
	if err != nil {
		t.Fatalf("parse parser package: %v", err)
	}
	out := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 || fd.Name.Name != marker {
					continue
				}
				if star, ok := fd.Recv.List[0].Type.(*ast.StarExpr); ok {
					if id, ok := star.X.(*ast.Ident); ok {
						out[id.Name] = true
					}
				}
			}
		}
	}
	return out
}

// switchCaseTypes returns the set of *parser.X type names appearing in the
// type-switch(es) of the function named fn in lintFile.
func switchCaseTypes(t *testing.T, lintFile, fn string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, lintFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", lintFile, err)
	}
	out := map[string]bool{}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				star, ok := e.(*ast.StarExpr)
				if !ok {
					continue
				}
				if sel, ok := star.X.(*ast.SelectorExpr); ok {
					out[sel.Sel.Name] = true
				}
			}
			return true
		})
	}
	return out
}

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func TestWalkerExhaustiveness(t *testing.T) {
	exprTypes := markerReceivers(t, "exprNode")
	stmtTypes := markerReceivers(t, "stmtNode")
	if len(exprTypes) < 10 || len(stmtTypes) < 10 {
		t.Fatalf("suspiciously few node types (expr %d, stmt %d) - did the parse break?", len(exprTypes), len(stmtTypes))
	}

	// Intentional leaves: node types a general traversal need not descend into.
	// Literals carry no child expression; VarExpr / ConstRefExpr / a qualified
	// const ref are bare names; PreEval wraps a runtime value and never appears
	// in a source AST.
	exprLeaves := setOf("IntLit", "FloatLit", "StringLit", "BoolLit", "NullLit",
		"VarExpr", "ConstRefExpr", "QualifiedConstRefExpr", "PreEval")
	// break / continue are leaves; a bare Block never appears in a statement
	// list (bodies are reached through *Block fields); struct / enum / method
	// definitions and imports carry no child statement a body walker sees
	// (methods are walked through the program's method list).
	stmtLeaves := setOf("BreakStmt", "ContinueStmt", "Block",
		"StructDef", "EnumDef", "MethodDef", "ImportStmt", "ModuleImportStmt")

	walkers := []struct {
		file, fn string
		types    map[string]bool
		leaves   map[string]bool
	}{
		{"scope.go", "doExpr", exprTypes, exprLeaves},
		{"scope.go", "doStmt", stmtTypes, stmtLeaves},
		{"walk.go", "doExpr", exprTypes, exprLeaves},
		{"walk.go", "doStmt", stmtTypes, stmtLeaves},
		{"checks.go", "nestExpr", exprTypes, exprLeaves},
		{"checks.go", "nestStmt", stmtTypes, stmtLeaves},
	}
	for _, w := range walkers {
		handled := switchCaseTypes(t, w.file, w.fn)
		if len(handled) == 0 {
			t.Errorf("%s:%s - no type-switch cases found (wrong function name?)", w.file, w.fn)
			continue
		}
		for typ := range w.types {
			if handled[typ] || w.leaves[typ] {
				continue
			}
			t.Errorf("%s:%s does not handle *parser.%s - add a case that descends into its children, "+
				"or add it to the leaf allowlist in this test if it has no child expr/stmt to descend into",
				w.file, w.fn, typ)
		}
	}
}
