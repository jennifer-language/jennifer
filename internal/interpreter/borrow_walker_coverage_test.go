// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Read-only-parameter borrow soundness depends on four AST walkers agreeing on
// the grammar: the parser write-scan (writeScan.stmt), the interpreter locals
// collector (collectLocalsStmt) and hazard scan (globalHazardScan.walkStmt /
// walkExpr). Each now has a conservative default, so a missed node kind is
// merely a lost optimization, not a soundness hole - but a lost optimization
// that appears silently is exactly what this test catches. It parses the Go
// source to find every AST node kind (types with a stmtNode()/exprNode() method)
// and every kind each walker enumerates, and fails when a walker omits one. When
// this test fails after a grammar addition, add the new node's case to the named
// walker(s) - or, if it is deliberately covered by the default, add it to the
// allow-list below with a reason.
func TestBorrowWalkersCoverAllNodes(t *testing.T) {
	root := repoRoot(t)
	astFile := filepath.Join(root, "internal", "parser", "ast.go")
	resolverFile := filepath.Join(root, "internal", "parser", "resolver.go")
	globalsafeFile := filepath.Join(root, "internal", "interpreter", "globalsafe.go")

	stmtKinds := nodeKindsWithMethod(t, astFile, "stmtNode")
	exprKinds := nodeKindsWithMethod(t, astFile, "exprNode")

	// Statement walkers must enumerate every statement kind; the expression
	// walker every expression kind. A grouped `case *A, *B:` counts each name.
	stmtWalkers := []struct{ file, fn string }{
		{resolverFile, "stmt"}, // writeScan.stmt (borrow write-scan)
		{globalsafeFile, "collectLocalsStmt"},
		{globalsafeFile, "walkStmt"}, // globalHazardScan.walkStmt
	}
	for _, wk := range stmtWalkers {
		covered := switchCaseTypes(t, wk.file, wk.fn)
		assertCovers(t, wk.fn, stmtKinds, covered)
	}
	exprCovered := switchCaseTypes(t, globalsafeFile, "walkExpr") // globalHazardScan.walkExpr
	assertCovers(t, "walkExpr", exprKinds, exprCovered)
}

func assertCovers(t *testing.T, walker string, want, have map[string]bool) {
	t.Helper()
	var missing []string
	for k := range want {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("walker %s does not enumerate node kind(s): %s\n"+
			"add each as a case (or a deliberate grouped no-op case); a missing kind falls to the conservative default and silently loses the borrow optimization for methods using it",
			walker, strings.Join(missing, ", "))
	}
}

// nodeKindsWithMethod returns the set of type names T with a `func (*T) method()`
// (e.g. every T implementing stmtNode / exprNode).
func nodeKindsWithMethod(t *testing.T, file, method string) map[string]bool {
	t.Helper()
	f := parseGo(t, file)
	kinds := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != method || len(fn.Recv.List) != 1 {
			continue
		}
		if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
			if id, ok := star.X.(*ast.Ident); ok {
				kinds[id.Name] = true
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatalf("found no types with %s() in %s", method, file)
	}
	return kinds
}

// switchCaseTypes returns the set of pointer-type names named in `case *T:` (and
// grouped `case *A, *B:`) across every type-switch in function fn, stripping any
// package qualifier (parser.AssignStmt -> AssignStmt).
func switchCaseTypes(t *testing.T, file, fn string) map[string]bool {
	t.Helper()
	f := parseGo(t, file)
	types := map[string]bool{}
	var body *ast.BlockStmt
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == fn {
			body = d.Body
			break
		}
	}
	if body == nil {
		t.Fatalf("function %s not found in %s", fn, file)
	}
	ast.Inspect(body, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, e := range cc.List {
			star, ok := e.(*ast.StarExpr)
			if !ok {
				continue
			}
			switch x := star.X.(type) {
			case *ast.Ident:
				types[x.Name] = true
			case *ast.SelectorExpr:
				types[x.Sel.Name] = true
			}
		}
		return true
	})
	return types
}

func parseGo(t *testing.T, file string) *ast.File {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return f
}

// repoRoot walks up from this test file to the module root (the dir with go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(self)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test file")
		}
		dir = parent
	}
}
