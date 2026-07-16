// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

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
