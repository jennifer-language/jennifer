// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// ---- Enums: construction, display, value semantics ----

func TestEnumConstructAndDisplay(t *testing.T) {
	out, err := run(t, `
use io;
def enum Shape { Circle { r as float }, Rect { w as float, h as float }, Empty };
def a as Shape init Shape.Circle{ r: 2.0 };
def b as Shape init Shape.Empty;
io.printf("%v\n", $a);
io.printf("%v\n", $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "Shape.Circle{r: 2.0}\nShape.Empty\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestEnumMatchBindsPayload(t *testing.T) {
	out, err := run(t, `
use io;
def enum Shape { Circle { r as float }, Rect { w as float, h as float }, Empty };
func area(s as Shape) {
    match ($s) {
        when Circle(c) { return $c.r * $c.r; }
        when Rect(rc) { return $rc.w * $rc.h; }
        when Empty { return 0.0; }
    }
    return -1.0;
}
io.printf("%f %f %f\n", area(Shape.Circle{r: 3.0}), area(Shape.Rect{w: 2.0, h: 5.0}), area(Shape.Empty));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if want := "9.0 10.0 0.0\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestEnumPayloadlessMatch(t *testing.T) {
	out, err := run(t, `
use io;
def enum Dir { North, South, East, West };
def d as Dir init Dir.East;
match ($d) {
    when North { io.printf("n\n"); }
    when South { io.printf("s\n"); }
    when East { io.printf("e\n"); }
    when West { io.printf("w\n"); }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "e\n" {
		t.Errorf("got %q", out)
	}
}

func TestEnumEquality(t *testing.T) {
	out, err := run(t, `
use io;
def enum Box { Full { n as int }, Empty };
def a as Box init Box.Full{ n: 1 };
def b as Box init $a;
io.printf("copyEq=%t payloadEq=%t self=%t empty=%t diff=%t\n",
    $a == $b, $a == Box.Full{n: 1}, $a == $a, $a == Box.Empty, $a == Box.Full{n: 2});
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if want := "copyEq=true payloadEq=true self=true empty=false diff=false\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestEnumZeroValueIsFirstVariant(t *testing.T) {
	out, err := run(t, `
use io;
def enum State { Idle, Running { pid as int } };
def s as State;
io.printf("%v\n", $s);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "State.Idle\n" {
		t.Errorf("zero value: got %q, want State.Idle", out)
	}
}

func TestEnumConstShapedVariantNames(t *testing.T) {
	// A variant whose name is constant-shaped (all-caps) is resolved at eval,
	// not mistaken for a constant reference.
	out, err := run(t, `
use io;
def enum Sig { OK, ERR { code as int } };
def s as Sig init Sig.ERR{ code: 42 };
match ($s) {
    when OK { io.printf("ok\n"); }
    when ERR(e) { io.printf("err %d\n", $e.code); }
}
io.printf("%v\n", Sig.OK);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if want := "err 42\nSig.OK\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestEnumCaseAgnostic(t *testing.T) {
	// Lowercase enum + variant names work exactly like PascalCase: resolution is
	// structural (what the name refers to), never based on capitalisation.
	out, err := run(t, `
use io;
def enum color { red, green, blue };
def c as color init color.green;
match ($c) {
    when red { io.printf("r\n"); }
    when green { io.printf("g\n"); }
    when blue { io.printf("b\n"); }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "g\n" {
		t.Errorf("got %q", out)
	}
}

// ---- Enums: negative / error paths ----

func TestEnumNonExhaustiveMatchErrors(t *testing.T) {
	_, err := run(t, `
def enum Ev { A, B, C };
def x as Ev init Ev.A;
match ($x) {
    when A { }
    when B { }
}
`)
	if err == nil || !strings.Contains(err.Error(), "not exhaustive") {
		t.Fatalf("want exhaustiveness error, got %v", err)
	}
	if !strings.Contains(err.Error(), "C") {
		t.Errorf("error should name the missing variant C: %v", err)
	}
}

func TestEnumExhaustiveWithElse(t *testing.T) {
	out, err := run(t, `
use io;
def enum Ev { A, B, C };
def x as Ev init Ev.C;
match ($x) {
    when A { io.printf("a\n"); }
    else { io.printf("other\n"); }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "other\n" {
		t.Errorf("got %q", out)
	}
}

func TestEnumUnknownVariantInMatch(t *testing.T) {
	_, err := run(t, `
def enum Ev { A, B };
def x as Ev init Ev.A;
match ($x) {
    when A { }
    when Z { }
}
`)
	if err == nil || !strings.Contains(err.Error(), "not a variant") {
		t.Fatalf("want unknown-variant error, got %v", err)
	}
}

func TestEnumBindOnPayloadlessErrors(t *testing.T) {
	_, err := run(t, `
def enum Ev { A, B };
def x as Ev init Ev.A;
match ($x) {
    when A(c) { }
    when B { }
}
`)
	if err == nil || !strings.Contains(err.Error(), "no payload to bind") {
		t.Fatalf("want payload-bind error, got %v", err)
	}
}

func TestEnumBareConstructionOfPayloadedErrors(t *testing.T) {
	_, err := run(t, `
def enum Ev { A { n as int }, B };
def x as Ev init Ev.A;
`)
	if err == nil || !strings.Contains(err.Error(), "carries a payload") {
		t.Fatalf("want payload-construction error, got %v", err)
	}
}

func TestEnumUnknownVariantConstruction(t *testing.T) {
	_, err := run(t, `
def enum Ev { A, B };
def x as Ev init Ev.Zonk;
`)
	if err == nil || !strings.Contains(err.Error(), "has no variant") {
		t.Fatalf("want unknown-variant construction error, got %v", err)
	}
}

func TestEnumNameCollidesWithStruct(t *testing.T) {
	_, err := run(t, `
def struct Foo { x as int };
def enum Foo { A, B };
`)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("want name-collision error, got %v", err)
	}
}

func TestEnumZeroCycleRejected(t *testing.T) {
	_, err := run(t, `
def enum Bad { Cons { tail as Bad }, Nil };
def x as Bad;
`)
	if err == nil || !strings.Contains(err.Error(), "no finite zero value") {
		t.Fatalf("want zero-cycle error, got %v", err)
	}
}

func TestEnumRecursiveWithSafeBaseCase(t *testing.T) {
	// A recursive enum is fine when its first variant is a payload-less base case.
	out, err := run(t, `
use io;
def enum Lst { Nil, Cons { head as int, tail as Lst } };
def x as Lst;
io.printf("%v\n", $x);
def one as Lst init Lst.Cons{ head: 5, tail: Lst.Nil };
match ($one) {
    when Nil { io.printf("nil\n"); }
    when Cons(c) { io.printf("head=%d\n", $c.head); }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if want := "Lst.Nil\nhead=5\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestEnumDuplicateVariantRejected(t *testing.T) {
	_, err := run(t, `
def enum Ev { A, A };
`)
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("want duplicate-variant error, got %v", err)
	}
}

// TestEnumMatchInsideSpawn pins the fix for pattern matching inside a spawn body
// (which the resolver skips, so the runtime fallback must recognize the pattern).
func TestEnumMatchInsideSpawn(t *testing.T) {
	out, runErr, _ := runSpawn(t, `
use io;
def enum E { A { n as int }, B };
def e as E init E.A{ n: 7 };
spawn {
    match ($e) {
        when A(a) { io.printf("a%d\n", $a.n); }
        when B { io.printf("b\n"); }
    }
};
`)
	if runErr != nil {
		t.Fatalf("err: %v", runErr)
	}
	if !strings.Contains(out, "a7") {
		t.Errorf("spawn enum match: got %q, want a7", out)
	}
}

// TestEnumBinderCannotLeakToTypedSlot pins the soundness fix: a match-bound
// payload (a struct) must not satisfy an `as EnumType` (or same-named struct)
// binding, even when a variant is named like its enum.
func TestEnumBinderCannotLeakToEnumSlot(t *testing.T) {
	_, err := run(t, `
def enum Shape { Shape { r as int }, Empty };
def s as Shape init Shape.Shape{ r: 5 };
match ($s) {
    when Shape(inner) { def y as Shape init $inner; }
    when Empty { }
}
`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize") {
		t.Fatalf("payload must not bind to an enum-typed slot, got %v", err)
	}
}

func TestEnumBinderCannotLeakToStructSlot(t *testing.T) {
	_, err := run(t, `
def struct Circle { a as string };
def enum Shape { Circle { r as int }, Empty };
def s as Shape init Shape.Circle{ r: 5 };
match ($s) {
    when Circle(c) { def x as Circle init $c; }
    else { }
}
`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize") {
		t.Fatalf("payload must not bind to a same-named struct slot, got %v", err)
	}
}

// TestEnumBinderIsReadOnly pins that a write to a bound payload gets a clean
// "cannot mutate constant" error, not an internal struct-def failure.
func TestEnumBinderIsReadOnly(t *testing.T) {
	_, err := run(t, `
def enum E { A { n as int }, B };
def x as E init E.A{ n: 5 };
match ($x) { when A(a) { $a.n = 9; } when B { } }
`)
	if err == nil || !strings.Contains(err.Error(), "cannot mutate") {
		t.Fatalf("binder write should be a clean const error, got %v", err)
	}
	if strings.Contains(err.Error(), "internal") {
		t.Errorf("binder write leaked an internal error: %v", err)
	}
}

// TestEnumMatchOnStructField pins that a `match` subject that is a struct's
// enum-typed field is recognized as a pattern match.
func TestEnumMatchOnStructField(t *testing.T) {
	out, err := run(t, `
use io;
def enum E { A { n as int }, B };
def struct W { e as E };
def w as W init W{ e: E.A{ n: 7 } };
match ($w.e) {
    when A(a) { io.printf("a%d\n", $a.n); }
    when B { io.printf("b\n"); }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a7\n" {
		t.Errorf("got %q, want a7", out)
	}
}

// TestEnumVariantFieldRejectsSigil pins that an enum payload field name cannot
// carry a `$` sigil (mirroring struct fields).
func TestEnumVariantFieldRejectsSigil(t *testing.T) {
	_, err := run(t, `def enum E { V { $x as int } };`)
	if err == nil || !strings.Contains(err.Error(), "has no `$`") {
		t.Fatalf("want $-sigil field error, got %v", err)
	}
}
