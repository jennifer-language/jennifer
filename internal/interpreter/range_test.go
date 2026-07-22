// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// Half-open range literal materialises a `list of int`.
func TestRangeLiteralHalfOpen(t *testing.T) {
	out, err := run(t, `use io; def r as list of int init 1..5;
for (def x in $r) { io.printf("%d ", $x); }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "1 2 3 4 " {
		t.Fatalf("got %q, want %q", out, "1 2 3 4 ")
	}
}

func TestRangeLiteralEmpty(t *testing.T) {
	out, err := run(t, `use io; def r as list of int init 3..3; io.printf("%d\n", len($r));`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "0\n" {
		t.Fatalf("got %q, want %q", out, "0\n")
	}
}

func TestRangeLoExceedsHi(t *testing.T) {
	_, err := run(t, `def r as list of int init 5..2;`)
	if err == nil || !strings.Contains(err.Error(), "exceeds upper bound") {
		t.Fatalf("expected lo>hi error, got %v", err)
	}
}

func TestRangeNonIntBounds(t *testing.T) {
	_, err := run(t, `def r as list of int init 1.5..3;`)
	if err == nil || !strings.Contains(err.Error(), "range bounds must be int") {
		t.Fatalf("expected non-int-bounds error, got %v", err)
	}
}

func TestRangeNonAssociative(t *testing.T) {
	_, err := run(t, `def r as list of int init 1..5..2;`)
	if err == nil || !strings.Contains(err.Error(), "non-associative") {
		t.Fatalf("expected non-associative parse error, got %v", err)
	}
}

// A materialised range that would overflow allocation raises a *catchable*
// runtime error, not Go's uncatchable "makeslice: cap out of range" panic.
func TestRangeMaterializeTooLarge(t *testing.T) {
	_, err := run(t, `def r as list of int init 0..9223372036854775807;`)
	if err == nil || !strings.Contains(err.Error(), "too large to materialise") {
		t.Fatalf("expected too-large materialise error, got %v", err)
	}
}

// The int64 span itself overflows here (hi-lo wraps negative); the guard must
// still catch it rather than reaching make() with a negative capacity.
func TestRangeSpanOverflow(t *testing.T) {
	_, err := run(t, `def r as list of int init -9223372036854775808..9223372036854775807;`)
	if err == nil || !strings.Contains(err.Error(), "too large to materialise") {
		t.Fatalf("expected span-overflow guard, got %v", err)
	}
}

// The lazy for-each form is deliberately NOT capped: it allocates nothing, so a
// huge upper bound must iterate (here broken early) without erroring or OOMing.
func TestForEachRangeLazyUnbounded(t *testing.T) {
	out, err := run(t, `use io;
def c as int init 0;
for (def i in 0..9223372036854775807) {
	$c = $c + 1;
	if ($c >= 3) { break; }
}
io.printf("%d", $c);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "3" {
		t.Fatalf("got %q, want %q", out, "3")
	}
}

// for-each over a bare range iterates lazily (never builds a list).
func TestForEachRange(t *testing.T) {
	out, err := run(t, `use io; for (def i in 0..4) { io.printf("%d", $i); }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "0123" {
		t.Fatalf("got %q, want %q", out, "0123")
	}
}

func TestForEachRangeBreakContinue(t *testing.T) {
	out, err := run(t, `use io;
for (def i in 0..10) {
	if ($i == 2) { continue; }
	if ($i == 5) { break; }
	io.printf("%d", $i);
}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "0134" {
		t.Fatalf("got %q, want %q", out, "0134")
	}
}

// List slicing returns a fresh, value-semantic copy.
func TestSliceList(t *testing.T) {
	out, err := run(t, `use io;
def xs as list of int init [10, 20, 30, 40, 50];
def mid as list of int init $xs[1..4];
for (def v in $mid) { io.printf("%d ", $v); }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "20 30 40 " {
		t.Fatalf("got %q, want %q", out, "20 30 40 ")
	}
}

func TestSliceIsCopy(t *testing.T) {
	out, err := run(t, `use io;
def xs as list of int init [1, 2, 3, 4];
def ys as list of int init $xs[0..2];
$ys[0] = 99;
io.printf("%d %d", $xs[0], $ys[0]);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "1 99" {
		t.Fatalf("got %q, want %q (slice must be a copy)", out, "1 99")
	}
}

func TestSliceOpenEnds(t *testing.T) {
	out, err := run(t, `use io;
def xs as list of int init [1, 2, 3, 4, 5];
io.printf("%d %d %d", len($xs[2..]), len($xs[..3]), len($xs[..]));`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "3 3 5" {
		t.Fatalf("got %q, want %q", out, "3 3 5")
	}
}

func TestSliceString(t *testing.T) {
	out, err := run(t, `use io; def s as string init "hello world"; io.printf("%s", $s[0..5]);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello" {
		t.Fatalf("got %q, want %q", out, "hello")
	}
}

// String slicing is rune-indexed, not byte-indexed.
func TestSliceStringRuneIndexed(t *testing.T) {
	out, err := run(t, `use io; def s as string init "aeiou"; io.printf("%s", $s[1..3]);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ei" {
		t.Fatalf("got %q, want %q", out, "ei")
	}
}

func TestSliceBytes(t *testing.T) {
	out, err := run(t, `use io; use convert;
def b as bytes init convert.bytesFromString("abcdef", "utf-8");
def sub as bytes init $b[2..5];
io.printf("%d %d", len($sub), $sub[0]);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "3 99" { // 'c' == 99
		t.Fatalf("got %q, want %q", out, "3 99")
	}
}

func TestSliceOutOfBounds(t *testing.T) {
	_, err := run(t, `def xs as list of int init [1, 2, 3]; def y as list of int init $xs[0..9];`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds error, got %v", err)
	}
}

func TestSliceLoExceedsHi(t *testing.T) {
	_, err := run(t, `def xs as list of int init [1, 2, 3]; def y as list of int init $xs[2..1];`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds error, got %v", err)
	}
}

func TestSliceAssignRejected(t *testing.T) {
	_, err := run(t, `def xs as list of int init [1, 2, 3]; $xs[0..2] = [9, 9];`)
	if err == nil || !strings.Contains(err.Error(), "slice assignment") {
		t.Fatalf("expected slice-assignment rejection, got %v", err)
	}
}

func TestSliceUnsupportedKind(t *testing.T) {
	_, err := run(t, `def n as int init 5; def y as list of int init $n[0..2];`)
	if err == nil || !strings.Contains(err.Error(), "cannot slice") {
		t.Fatalf("expected cannot-slice error, got %v", err)
	}
}

// A slice bound past int32 range must be an out-of-bounds error on every
// target: sliceBounds checks in int64 before the int() cast, so a 32-bit build
// (jennifer-tiny on embedded) cannot truncate 4294967296 to a small in-range
// index and silently return the wrong slice.
func TestSliceHugeBoundRejected(t *testing.T) {
	_, err := run(t, `def xs as list of int init [1, 2, 3]; def y as list of int init $xs[0..4294967296];`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds error for a >int32 slice bound, got %v", err)
	}
	_, err = run(t, `def xs as list of int init [1, 2, 3]; def y as list of int init $xs[4294967296..];`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds error for a >int32 lower bound, got %v", err)
	}
}

// defer inside a lazy range-loop body: execForEachRange must run the frame's
// deferred calls at the end of EACH iteration (finishFrame on the borrowed
// iterEnv), in LIFO order, including on the break exit.
func TestForEachRangeDeferPerIteration(t *testing.T) {
	out, err := run(t, `use io;
for (def i in 0..3) {
	defer io.printf("d%d ", $i);
	if ($i == 2) { break; }
	io.printf("b%d ", $i);
}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "b0 d0 b1 d1 d2 " {
		t.Fatalf("got %q, want %q", out, "b0 d0 b1 d1 d2 ")
	}
}

// Range expressions compose: materialised in argument position, and sliceable
// as a value (`(0..10)[2..5]`).
func TestRangeComposition(t *testing.T) {
	out, err := run(t, `use io;
func total(xs as list of int) { def s as int init 0; for (def v in $xs) { $s = $s + $v; } return $s; }
io.printf("%d,%d", total(1..5), total((0..10)[2..5]));`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// 1+2+3+4 = 10; (0..10)[2..5] = [2,3,4] -> 9
	if out != "10,9" {
		t.Fatalf("got %q, want 10,9", out)
	}
}
