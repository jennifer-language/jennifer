// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// identityGeo is a module whose struct, enum and constant are read from both
// sides of the boundary by the tests below.
const identityGeo = `use io;
export def struct Point { x as int };
export def struct Box { p as Point };
export def enum Shape { A, B };
export def const ORIGIN as Point init Point{x: 0};
export func make(x as int) { return Point{x: $x}; }
export func px(p as Point) { return $p.x; }
export func same(a as Point, b as Point) { return $a == $b; }
export func show(p as Point) { return io.sprintf("%v", $p); }
export func shape() { return Shape.B; }
export func isB(s as Shape) { return $s == Shape.B; }
export func fail(x as int) { throw Point{x: $x}; }`

// A module's own struct is one type on both sides of the boundary (M29.14): a
// value the module builds, one the consumer builds with the alias, a module
// constant and a zero value compare equal, cross back in, print the same in
// the module as in the consumer, and a thrown one binds to the alias type.
func TestModuleStructOneIdentity(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"geo.j": identityGeo,
		"main.j": `use io; import "./geo.j" as g;
def a as g.Point init g.make(1);
def b as g.Point init g.Point{x: 1};
io.printf("%v %v ", $a == $b, g.same($a, $b));
def z as g.Point;
io.printf("%v %v ", g.ORIGIN == g.make(0), $z == g.ORIGIN);
io.printf("%v %s ", $a, g.show($a));
def k as g.Shape;
io.printf("%v %v %v ", $k == g.Shape.A, g.isB(g.shape()), g.isB(g.Shape.B));
def bx as g.Box init g.Box{p: g.make(3)};
$bx.p = g.ORIGIN;
io.printf("%d ", g.px($bx.p));
try { g.fail(7); } catch (e) { def p as g.Point init $e; io.printf("%d", g.px($p)); }`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "true true true true geo.Point{x: 1} geo.Point{x: 1} true true true 0 7"
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

// The consumer's own struct that shares a name and the field layout of a module
// struct is a different type wherever the two meet, since the identities differ
// by construction: a binding, a field, an argument, a parameter of the
// consumer's own type, and the comparison all tell them apart.
func TestModuleSameNamedConsumerStructIsDistinct(t *testing.T) {
	cases := []struct{ name, stmt, want string }{
		{"module binding", `def p as g.Point init $mine;`, "cannot initialize geo.Point variable"},
		{"consumer binding", `def p as Point init g.make(1);`, "cannot initialize Point variable"},
		{"module struct field", `def bx as g.Box init g.Box{p: $mine};`, `field "p" of struct "Box" expects geo.Point`},
		{"field assignment", `def bx as g.Box init g.Box{p: g.make(1)}; $bx.p = $mine;`, `field "p" of struct "Box" expects geo.Point`},
		{"argument", `g.px($mine);`, "cannot impersonate a module struct"},
		{"consumer parameter", `func mx(p as Point) { return $p.x; } mx(g.make(1));`, "must be Point"},
		{"enum", `def enum Shape { A, B }; g.isB(Shape.B);`, "cannot impersonate a module enum"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := runModuleMain(t, map[string]string{
				"geo.j": identityGeo,
				"main.j": `import "./geo.j" as g;
def struct Point { x as int };
def mine as Point init Point{x: 1};
` + c.stmt,
			})
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q\nwant it to contain %q", err.Error(), c.want)
			}
		})
	}
	out, err := runModuleMain(t, map[string]string{
		"geo.j": identityGeo,
		"main.j": `use io; import "./geo.j" as g;
def struct Point { x as int };
def mine as Point init Point{x: 1};
io.printf("%v %v", $mine == g.make(1), $mine);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "false Point{x: 1}" {
		t.Errorf("got %q, want %q", out, "false Point{x: 1}")
	}
}
