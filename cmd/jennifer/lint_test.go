// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLintSuppressionInIncludedFile checks that a `# lint-disable` directive in
// an included file suppresses findings anchored to that file. The preprocessor
// strips comments, so the directive is only reachable by re-lexing the included
// file - the regression the fix restores.
func TestLintSuppressionInIncludedFile(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	writeFile("main.j", "include \"helper.j\";\nuse io;\nio.printf(\"hi\\n\");\n")
	mainPath := filepath.Join(dir, "main.j")
	opts := lintOptions{}

	// Baseline: an unused local in the include is reported against helper.j.
	writeFile("helper.j", "func f() {\n    def x as int init 5;\n}\n")
	diags, _, _, failed := lintComputeDiags(mainPath, opts)
	if failed {
		t.Fatalf("lint invocation failed")
	}
	found := false
	for _, d := range diags {
		if d.ID == "L101" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected L101 for the unused local in helper.j, got %v", diags)
	}

	// A file-level directive in the include suppresses it.
	writeFile("helper.j", "# lint-disable-file: L101\nfunc f() {\n    def x as int init 5;\n}\n")
	diags, _, _, _ = lintComputeDiags(mainPath, opts)
	for _, d := range diags {
		if d.ID == "L101" {
			t.Fatalf("file-level directive in include did not suppress L101: %v", diags)
		}
	}

	// A line-level directive on the offending line suppresses it too.
	writeFile("helper.j", "func f() {\n    def x as int init 5;   # lint-disable: L101\n}\n")
	diags, _, _, _ = lintComputeDiags(mainPath, opts)
	for _, d := range diags {
		if d.ID == "L101" {
			t.Fatalf("line-level directive in include did not suppress L101: %v", diags)
		}
	}
}

// TestLintSplicesModuleOverlay checks that linting a `MODULE_test.j` overlay
// splices the sibling `MODULE.j` in front (like `jennifer test`), so scope
// analysis sees the module's private declarations. Without the splice a
// `when Variant(bind)` arm over a module-private enum goes unrecognised and the
// bound name reads as a false "undefined variable" (L002) - a finding `test`
// never produces. Findings anchored to the base module itself are not reported
// when linting the overlay (they belong to `lint MODULE.j`).
func TestLintSplicesModuleOverlay(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	// A module with a private enum (used via a binder pattern in the overlay) and
	// a private unused local (to prove base-module findings are not double-reported).
	writeFile("shape.j", "def enum Shape { Circle { r as int }, Square { s as int } };\n"+
		"export func area(sh as Shape) { def dead as int init 9; return 0; }\n")
	overlayPath := writeFile("shape_test.j", "use testing;\n"+
		"func testBinder() {\n"+
		"    def sh as Shape init Shape.Circle{r: 5};\n"+
		"    def out as int init 0;\n"+
		"    match ($sh) {\n"+
		"        when Circle(c) { $out = $c.r; }\n"+
		"        when Square(s) { $out = $s.s; }\n"+
		"    }\n"+
		"    testing.assertEqual($out, 5);\n"+
		"}\n")

	diags, _, _, failed := lintComputeDiags(overlayPath, lintOptions{})
	if failed {
		t.Fatalf("lint invocation failed")
	}
	for _, d := range diags {
		if d.ID == "L002" {
			t.Fatalf("overlay lint reported a source error (splice not applied?): %s at %s:%d:%d",
				d.Message, d.File, d.Line, d.Col)
		}
		if d.ID == "L101" {
			t.Fatalf("base-module finding leaked into overlay lint: %s at %s:%d:%d",
				d.Message, d.File, d.Line, d.Col)
		}
	}

	// The base module's own unused local is still reported when it is linted directly.
	baseDiags, _, _, _ := lintComputeDiags(filepath.Join(dir, "shape.j"), lintOptions{})
	found := false
	for _, d := range baseDiags {
		if d.ID == "L101" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected L101 for the unused local when linting the base module directly, got %v", baseDiags)
	}
}
