// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVendorTree lays out a vendor/ with two same-name decks under different
// scopes, plus an app, and returns the app's main path and the vendor dir.
func writeVendorTree(t *testing.T) (mainPath, vendorDir string) {
	t.Helper()
	root := t.TempDir()
	vendorDir = filepath.Join(root, "vendor")
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("vendor/acme/widgets/widgets.j", `export def struct Box { n as int };
		export func make(n as int) { return Box{ n: $n }; }
		export func size(b as Box) { return $b.n; }`)
	write("vendor/acme/widgets/extra.j", `export func answer() { return 42; }`)
	write("vendor/other/widgets/widgets.j", `export def struct Box { s as int };
		export func make(n as int) { return Box{ s: $n }; }`)
	mainPath = filepath.Join(root, "app", "main.j")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatal(err)
	}
	return mainPath, vendorDir
}

// The @scope/package/ entry form, an explicit file, the free package-named
// default alias, and canonical-path struct identity across decks all work.
func TestVendorDeckImports(t *testing.T) {
	mainPath, vendorDir := writeVendorTree(t)
	if err := os.WriteFile(mainPath, []byte(`import "@acme/widgets/" as w;
import "@acme/widgets/extra.j" as x;
import "@acme/widgets";
use io;
def b as w.Box init w.make(9);
io.printf("size=%d answer=%d default=%d\n", w.size($b), x.answer(), widgets.size(widgets.make(3)));`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { runFile(mainPath, nil, vendorDir) })
	if !strings.Contains(out, "size=9 answer=42 default=3") {
		t.Errorf("vendor imports failed:\n%s", out)
	}
}

// @acme/widgets and @other/widgets are distinct types (identity is the
// canonical path), so one's Box does not satisfy the other's.
func TestVendorDecksAreDistinctTypes(t *testing.T) {
	mainPath, vendorDir := writeVendorTree(t)
	if err := os.WriteFile(mainPath, []byte(`import "@acme/widgets/" as a;
import "@other/widgets/" as o;
def x as o.Box init a.make(1);`), 0o644); err != nil {
		t.Fatal(err)
	}
	code := runFile(mainPath, nil, vendorDir)
	if code == 0 {
		t.Error("assigning @acme/widgets Box to an @other/widgets Box should be a type error")
	}
}

// An @ import with no vendor root gives a guided error, not a crash.
func TestVendorNoRootErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "main.j")
	if err := os.WriteFile(p, []byte(`import "@acme/widgets/" as w;`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Empty vendorFlag and no vendor/ above dir -> no root.
	if code := runFile(p, nil, ""); code == 0 {
		t.Error("an @ import with no vendor root should fail")
	}
}
