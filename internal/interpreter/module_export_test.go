// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

func TestModulePrivateNameNotExported(t *testing.T) {
	_, err := runScopedModuleMain(t, map[string]string{
		"lib.j": `export func pub() { return priv(); }
func priv() { return 5; }`,
		"main.j": `use io;
import "./lib.j" as lib;
io.printf("%d\n", lib.priv());`,
	})
	if err == nil {
		t.Fatal("calling a private module function should error")
	}
	if !strings.Contains(err.Error(), "not exported from module") {
		t.Errorf("error should say not exported: %v", err)
	}
}

func TestModuleExportedNameResolves(t *testing.T) {
	// The exported `pub` reaches the private `priv` internally by bare name.
	out, err := runScopedModuleMain(t, map[string]string{
		"lib.j": `export func pub() { return priv() + 1; }
func priv() { return 5; }`,
		"main.j": `use io;
import "./lib.j" as lib;
io.printf("%d\n", lib.pub());`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "6" {
		t.Errorf("output = %q, want 6", out)
	}
}

func TestModuleReferentialClosureRejectsPrivateField(t *testing.T) {
	_, err := runScopedModuleMain(t, map[string]string{
		"lib.j": `def struct Secret { code as int };
export def struct Public { s as Secret };`,
		"main.j": `import "./lib.j" as lib;`,
	})
	if err == nil {
		t.Fatal("an exported struct with a private-struct field should error")
	}
	if !strings.Contains(err.Error(), "private struct") {
		t.Errorf("error should mention the private struct: %v", err)
	}
}

func TestModuleReferentialClosureAllowsExportedField(t *testing.T) {
	_, err := runScopedModuleMain(t, map[string]string{
		"lib.j": `export def struct Inner { code as int };
export def struct Outer { i as Inner };
export func mk() { return Outer{i: Inner{code: 1}}; }`,
		"main.j": `use io;
import "./lib.j" as lib;
def o as lib.Outer init lib.mk();
io.printf("ok\n");`,
	})
	if err != nil {
		t.Fatalf("an all-exported struct chain should load: %v", err)
	}
}

func TestModuleExportInScriptRejected(t *testing.T) {
	// main.j is run as a script (not imported); an `export` in it is rejected.
	_, err := runScopedModuleMain(t, map[string]string{
		"main.j": `export func f() { return 1; }`,
	})
	if err == nil {
		t.Fatal("`export` in a run script should be a parse/load error")
	}
	if !strings.Contains(err.Error(), "only allowed in a module") {
		t.Errorf("error should say export is module-only: %v", err)
	}
}

func TestModuleConsumerStructTypeAndConstruction(t *testing.T) {
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as points;
def p as points.Point init points.make(3, 4);
def q as points.Point init points.Point{x: 10, y: 20};
io.printf("%d %d %d\n", $p.x, points.getX($p), points.getX($q));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "3 3 10" {
		t.Errorf("output = %q, want '3 3 10'", got)
	}
}

func TestModuleStructAsFieldTypeInMainStruct(t *testing.T) {
	// A module struct used as a *field type* in a main-program struct must
	// type-check (regression: this used to fail "expects points.Point, got
	// struct" even though the same struct works fine as a variable type). Covers
	// construction, a chained lvalue into the nested module-struct field, value
	// semantics, and passing the nested field back into a module function.
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as points;
def struct Line { from as points.Point, to as points.Point };
def L as Line init Line{ from: points.Point{x: 1, y: 2}, to: points.Point{x: 3, y: 4} };
$L.from.x = 5;
def M as Line init $L;
$M.to.y = 99;
io.printf("%d %d %d %d\n", $L.from.x, $L.to.y, $M.to.y, points.getX($L.from));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "5 4 99 5" {
		t.Errorf("output = %q, want '5 4 99 5'", got)
	}
}

func TestModuleStructWithSiblingStructField(t *testing.T) {
	// A module struct whose own field is another struct in the same module
	// (`Seg { a as Point }`), constructed and mutated from the importer. The
	// module's bare field type must retag to the module identity the field value
	// carries across the boundary - at both construction and field assignment.
	const segModule = `
export def struct Point { x as int, y as int };
export def struct Seg { a as Point, b as Point };
`
	out, err := runScopedModuleMain(t, map[string]string{
		"seg.j": segModule,
		"main.j": `use io;
import "./seg.j" as geo;
def s as geo.Seg init geo.Seg{ a: geo.Point{x: 1, y: 2}, b: geo.Point{x: 3, y: 4} };
$s.a = geo.Point{x: 7, y: 8};
$s.a.x = 100;
io.printf("%d %d %d %d\n", $s.a.x, $s.a.y, $s.b.x, $s.b.y);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "100 8 3 4" {
		t.Errorf("output = %q, want '100 8 3 4'", got)
	}
}

func TestModuleListOfStructCrossesBoundary(t *testing.T) {
	// A consumer-typed `list of points.Point` handed back into a module
	// function must satisfy the module's bare `list of Point` parameter: the
	// boundary retag has to fix the list's element-type tag, not only the
	// struct element values.
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as points;
def ps as list of points.Point init [];
$ps[] = points.make(3, 4);
$ps[] = points.make(10, 20);
$ps[] = points.Point{x: 100, y: 0};
io.printf("%d\n", points.totalX($ps));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "113" {
		t.Errorf("output = %q, want 113", got)
	}
}

func TestModuleListOfStructUnderMismatchedAlias(t *testing.T) {
	// When the alias differs from the file stem (`as p`, stem `points`), a
	// consumer `list of p.Point` must still accept `p.make(...)` values: the
	// declared element type has to be stamped from the alias to the module
	// stem, recursing into the list element type, so it matches the identity
	// values carry across the boundary. A top-level `def x as p.Point` alone
	// worked before; the list element type did not.
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as p;
def ps as list of p.Point init [];
$ps[] = p.make(3, 4);
$ps[] = p.make(10, 20);
io.printf("%d\n", p.totalX($ps));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "13" {
		t.Errorf("output = %q, want 13", got)
	}
}

func TestModuleAliasedStructTypeInLoopIsIdempotent(t *testing.T) {
	// A `def as list of p.Point` inside a loop re-resolves its declared type on
	// every iteration. Stamping the alias (`p`) to the module stem (`points`)
	// must be idempotent: after the first pass the type names the stem, which
	// is no longer an importer alias, so re-resolution has to recognise it
	// rather than fail with "unknown namespace".
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as p;
def total as int init 0;
for (def i as int init 0; $i < 3; $i = $i + 1) {
	def ps as list of p.Point init [];
	$ps[] = p.make($i, 0);
	$total = $total + p.totalX($ps);
}
io.printf("%d\n", $total);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "3" {
		t.Errorf("output = %q, want 3", got)
	}
}

func TestModuleStructIdentitiesAreDistinct(t *testing.T) {
	// A struct from module `a` must not satisfy module `b`'s same-named type.
	_, err := runScopedModuleMain(t, map[string]string{
		"a.j": `export def struct Point { x as int };
export func mk() { return Point{x: 1}; }`,
		"b.j": `export def struct Point { x as int };`,
		"main.j": `import "./a.j" as a;
import "./b.j" as b;
def p as b.Point init a.mk();`,
	})
	if err == nil {
		t.Fatal("a.Point should not satisfy b.Point")
	}
	if !strings.Contains(err.Error(), "Point") {
		t.Errorf("error should be a struct type mismatch: %v", err)
	}
}
