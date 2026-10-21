// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// lint resolves a file's `import`s the way `run` does, so a call or literal into
// a module is checked against the module's exported surface: a local `./` import
// here, and a bare name through an -I search dir.
func TestLintChecksImportedModuleSurface(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	for p, content := range map[string]string{
		filepath.Join(dir, "m.j"):   "export func two(a as int, b as int) { return $a + $b; }\nexport def struct P { x as int };\n",
		filepath.Join(lib, "lb.j"):  "export func one(a as int) { return $a; }\n",
		filepath.Join(dir, "use.j"): "import \"./m.j\" as m;\nimport \"lb.j\";\ndef a as int init m.two(1);\ndef p as m.P init m.P{x: 1, y: 2};\ndef b as int init lb.one(1, 2);\n",
	} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts, ok := parseLintArgs([]string{"-I", lib, "--checks=L109,L113", filepath.Join(dir, "use.j")})
	if !ok || len(opts.includes) != 1 {
		t.Fatalf("parseLintArgs: ok=%t includes=%v", ok, opts.includes)
	}
	diags, _, _, failed := lintComputeDiags(opts.paths[0], opts)
	if failed {
		t.Fatal("lint invocation failed")
	}
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%s", d.Line, d.ID))
	}
	if want := "3:L109 4:L113 5:L109"; strings.Join(got, " ") != want {
		t.Errorf("findings %v, want %s", got, want)
	}
}

// `use` is checked against the libraries the build installs (L115), the list
// lintLibSurface reads from a fully installed interpreter.
func TestLintUnknownLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "use.j")
	if err := os.WriteFile(path, []byte("use io;\nuse strngs;\nuse nosuchlib;\nio.printf(\"x\\n\");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts, ok := parseLintArgs([]string{"--checks=L115", path})
	if !ok {
		t.Fatal("parseLintArgs failed")
	}
	diags, _, _, failed := lintComputeDiags(opts.paths[0], opts)
	if failed {
		t.Fatal("lint invocation failed")
	}
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%s", d.Line, d.Message))
	}
	if want := "2:unknown library `strngs` - did you mean `strings`? | 3:unknown library `nosuchlib`"; strings.Join(got, " | ") != want {
		t.Errorf("findings %q, want %q", strings.Join(got, " | "), want)
	}
}

// Argument and binding types are checked across an import (L110 / L111), and a
// library constant or library struct field is typed from the registered library
// surface (lintLibSurface).
func TestLintChecksTypesAcrossBoundaries(t *testing.T) {
	dir := t.TempDir()
	for p, content := range map[string]string{
		filepath.Join(dir, "m.j"): "export def struct P { x as int };\nexport func two(a as int, p as P) { return $a; }\n",
		filepath.Join(dir, "use.j"): "import \"./m.j\" as m;\nuse time;\n" +
			"def a as int init m.two(\"x\", m.P{x: 1});\n" +
			"def b as int init m.two(1, 2);\n" +
			"def z as string init time.UTC;\n" +
			"def u as time.Zone init time.UTC;\n" +
			"def n as string init $u.offset;\n" +
			"def p as m.P init m.P{x: \"one\"};\n",
	} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts, ok := parseLintArgs([]string{"--checks=L110,L111", filepath.Join(dir, "use.j")})
	if !ok {
		t.Fatal("parseLintArgs failed")
	}
	diags, _, _, failed := lintComputeDiags(opts.paths[0], opts)
	if failed {
		t.Fatal("lint invocation failed")
	}
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%s", d.Line, d.ID))
	}
	if want := "3:L110 4:L110 5:L111 7:L111 8:L111"; strings.Join(got, " ") != want {
		t.Errorf("findings %v, want %s", got, want)
	}
}

// An include fragment (`NAME.inc.j`) linted on its own does not flag the names
// it takes from the file that includes it, but every other check still runs on
// it; linting the including file checks those names, the fragment spliced in.
func TestLintIncludeFragment(t *testing.T) {
	dir := t.TempDir()
	frag := filepath.Join(dir, "part.inc.j")
	parent := filepath.Join(dir, "whole.j")
	for p, content := range map[string]string{
		frag: "func shout(s as string) {\n    def n as int init \"x\";\n" +
			"    return strings.upper($s) + helper() + convert.toString(LIMIT + $n) + nope();\n}\n",
		parent: "use strings;\nuse convert;\ndef const LIMIT as int init 3;\n" +
			"func helper() { return \"!\"; }\ninclude \"./part.inc.j\";\nshout(\"hi\");\n",
	} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	findings := func(path string) string {
		t.Helper()
		opts, ok := parseLintArgs([]string{"--checks=L107,L108,L111,L112", path})
		if !ok {
			t.Fatal("parseLintArgs failed")
		}
		diags, _, _, failed := lintComputeDiags(opts.paths[0], opts)
		if failed {
			t.Fatal("lint invocation failed")
		}
		var got []string
		for _, d := range diags {
			got = append(got, fmt.Sprintf("%s:%d:%s", filepath.Base(d.File), d.Line, d.ID))
		}
		return strings.Join(got, " ")
	}
	if got, want := findings(frag), "part.inc.j:2:L111"; got != want {
		t.Errorf("fragment alone: %s, want %s", got, want)
	}
	if got, want := findings(parent), "part.inc.j:2:L111 part.inc.j:3:L107"; got != want {
		t.Errorf("including file: %s, want %s", got, want)
	}
}
