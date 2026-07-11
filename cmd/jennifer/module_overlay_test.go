// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A `MODULE_test.j` overlay is spliced onto its `MODULE.j` so the test methods
// reach the module's private names by bare identifier, and the combined
// program is run as the module it tests (so `export` is legal).
func TestModuleTestOverlaySplicesModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calc.j"), []byte(
		`export func add(a as int, b as int) { return $a + $b; }
func secret() { return 42; }
def const BASE as int init 100;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc_test.j"), []byte(
		`use testing;
func testPublic() { testing.assertEqual(add(2, 3), 5); }
func testPrivate() { testing.assertEqual(secret(), 42); }`), 0o644); err != nil {
		t.Fatal(err)
	}

	in, code := loadForTest(filepath.Join(dir, "calc_test.j"))
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest returned code %d", code)
	}
	// The test methods are hoisted...
	for _, name := range []string{"testPublic", "testPrivate"} {
		if !hasMethod(in, name) {
			t.Errorf("test method %q not discovered", name)
		}
	}
	// ...and the module's private names were spliced in (reachable by bare
	// identifier). A private test running secret()/BASE proves it end to end.
	if !hasMethod(in, "secret") {
		t.Errorf("module private method `secret` not spliced into scope")
	}
	if _, err := in.CallByName("testPrivate"); err != nil {
		t.Errorf("white-box test reading a private name failed: %v", err)
	}
}

// The shipped ansi module's white-box overlay (modules/ansi_test.j over
// modules/ansi.j) loads and its tests pass - a guard against ansi.j /
// ansi_test.j drifting out of sync.
func TestShippedAnsiOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "ansi_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the ansi overlay failed: code %d", code)
	}
	for _, name := range []string{
		"testEscIsOneByte", "testSgrCodes", "testStripRoundTrips", "testUnknownColourThrows",
	} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// A module that itself `import`s a sibling module can be tested through its
// overlay: the test path enables the module system, so the spliced module's
// own imports resolve (local, relative to the test file's directory).
func TestOverlayModuleImportsSibling(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dep.j"), []byte(
		`export func base() { return 40; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mod.j"), []byte(
		`import "./dep.j" as dep;
export func answer() { return dep.base() + 2; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mod_test.j"), []byte(
		`use testing;
func testAnswer() { testing.assertEqual(answer(), 42); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	in, code := loadForTest(filepath.Join(dir, "mod_test.j"))
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest returned code %d (module-importing-module overlay)", code)
	}
	if _, err := in.CallByName("testAnswer"); err != nil {
		t.Errorf("overlay whose module imports a sibling failed: %v", err)
	}
}

// The shipped markdown module imports the htmlwriter and ansi modules, so its
// overlay exercises the test path's module resolution end to end. A guard
// against markdown.j / markdown_test.j drifting out of sync.
func TestShippedMarkdownOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "markdown_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the markdown overlay failed: code %d", code)
	}
	for _, name := range []string{"testHtmlHeading", "testHtmlLists", "testAnsiListMarkers"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// A plain test file with no sibling module keeps working (no overlay spliced),
// and its own `export` is still rejected (it is not a module).
func TestNonOverlayTestFileUnaffected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain_test.j"), []byte(
		`use testing;
func testOk() { testing.assertEqual(1, 1); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	in, code := loadForTest(filepath.Join(dir, "plain_test.j"))
	if in == nil || code != testExitPass {
		t.Fatalf("plain test file failed to load: code %d", code)
	}
	if !hasMethod(in, "testOk") {
		t.Errorf("test method not discovered in plain test file")
	}
}
