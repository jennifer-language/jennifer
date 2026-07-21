// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// ---- Structs: positive path ----

func TestStructBasicConstructionAndRead(t *testing.T) {
	out, err := run(t, `
use io;
def struct Point { x as int, y as int };
def p as Point init Point{ x: 1, y: 2 };
io.printf("p = %v\n", $p);
io.printf("x=%d y=%d\n", $p.x, $p.y);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "p = Point{x: 1, y: 2}\nx=1 y=2\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStructFieldAssign(t *testing.T) {
	out, err := run(t, `
use io;
def struct Point { x as int, y as int };
def p as Point init Point{ x: 1, y: 2 };
$p.x = 99;
io.printf("%v\n", $p);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "Point{x: 99, y: 2}\n" {
		t.Errorf("got %q", out)
	}
}

func TestStructValueSemanticsOnAssignment(t *testing.T) {
	// `def q as Point init $p;` must take an independent copy: mutating
	// q must not bleed into p.
	out, err := run(t, `
use io;
def struct Point { x as int, y as int };
def p as Point init Point{ x: 1, y: 2 };
def q as Point init $p;
$q.y = -7;
io.printf("p=%v q=%v\n", $p, $q);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "p=Point{x: 1, y: 2} q=Point{x: 1, y: -7}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStructZeroInit(t *testing.T) {
	// `def x as Point;` (no init) gives every field its declared zero.
	out, err := run(t, `
use io;
def struct Point { x as int, y as int, name as string };
def z as Point;
io.printf("%v\n", $z);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != `Point{x: 0, y: 0, name: ""}`+"\n" {
		t.Errorf("got %q", out)
	}
}

func TestStructNestedAccessAndWrite(t *testing.T) {
	// $L.from.x = 5 walks two levels of FieldAccessExpr in the lvalue.
	out, err := run(t, `
use io;
def struct Point { x as int, y as int };
def struct Line { from as Point, to as Point };
def L as Line init Line{ from: Point{x: 0, y: 0}, to: Point{x: 10, y: 20} };
io.printf("L.to.x=%d\n", $L.to.x);
$L.from.x = 5;
io.printf("%v\n", $L);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "L.to.x=10\nLine{from: Point{x: 5, y: 0}, to: Point{x: 10, y: 20}}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStructNestedZeroInit(t *testing.T) {
	// Uninitialised nested struct must recurse so every leaf gets a
	// proper zero rather than KindNull.
	out, err := run(t, `
use io;
def struct Point { x as int, y as int };
def struct Line { from as Point, to as Point };
def L as Line;
io.printf("%v\n", $L);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "Line{from: Point{x: 0, y: 0}, to: Point{x: 0, y: 0}}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStructFieldOfListIsCopiedDeeply(t *testing.T) {
	// A list-typed field must follow value semantics on whole-struct
	// assignment: mutating the copy's list must not aliases into the
	// original.
	out, err := run(t, `
use io;
def struct Bag { items as list of int };
def a as Bag init Bag{ items: [1, 2, 3] };
def b as Bag init $a;
$b.items[0] = 99;
io.printf("a=%v b=%v\n", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "a=Bag{items: [1, 2, 3]} b=Bag{items: [99, 2, 3]}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// ---- Structs: error paths ----

func TestStructUnknownTypeAtDefine(t *testing.T) {
	_, err := run(t, `def x as Widget;`)
	if err == nil || !strings.Contains(err.Error(), "unknown struct type") {
		t.Fatalf("got %v", err)
	}
}

func TestStructMissingFieldInLiteral(t *testing.T) {
	_, err := run(t, `
def struct Point { x as int, y as int };
def p as Point init Point{ x: 1 };
`)
	if err == nil || !strings.Contains(err.Error(), `missing field "y"`) {
		t.Fatalf("got %v", err)
	}
}

func TestStructUnknownFieldInLiteral(t *testing.T) {
	_, err := run(t, `
def struct Point { x as int, y as int };
def p as Point init Point{ x: 1, y: 2, z: 3 };
`)
	if err == nil || !strings.Contains(err.Error(), `unknown field "z"`) {
		t.Fatalf("got %v", err)
	}
}

func TestStructFieldTypeMismatchAtLiteral(t *testing.T) {
	_, err := run(t, `
def struct Point { x as int, y as int };
def p as Point init Point{ x: "hi", y: 2 };
`)
	if err == nil || !strings.Contains(err.Error(), `field "x"`) {
		t.Fatalf("got %v", err)
	}
}

func TestStructConstIsDeep(t *testing.T) {
	_, err := run(t, `
def struct Point { x as int, y as int };
def const P as Point init Point{ x: 1, y: 2 };
$P.x = 99;
`)
	if err == nil || !strings.Contains(err.Error(), "constant") {
		t.Fatalf("got %v", err)
	}
}

func TestStructFieldAccessOnNonStructErrors(t *testing.T) {
	_, err := run(t, `
use io;
def p as int init 5;
io.printf("%d\n", $p.x);
`)
	if err == nil || !strings.Contains(err.Error(), "field access") {
		t.Fatalf("got %v", err)
	}
}

func TestStructFieldAssignTypeMismatch(t *testing.T) {
	_, err := run(t, `
def struct Point { x as int, y as int };
def p as Point init Point{ x: 1, y: 2 };
$p.x = "hi";
`)
	if err == nil || !strings.Contains(err.Error(), `field "x"`) {
		t.Fatalf("got %v", err)
	}
}

func TestStructDuplicateDefinitionRejected(t *testing.T) {
	_, err := run(t, `
def struct Point { x as int, y as int };
def struct Point { a as int };
`)
	if err == nil || !strings.Contains(err.Error(), "Point") {
		t.Fatalf("got %v", err)
	}
}
