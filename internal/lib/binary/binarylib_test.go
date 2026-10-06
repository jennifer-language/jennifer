// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package binarylib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	binarylib "jennifer-lang.dev/jennifer/internal/lib/binary"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg parses + runs a Jennifer program with io + convert + binary installed,
// returning captured stdout and the interpreter error.
func runProg(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	binarylib.Install(in)
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// TestMake covers the bytes allocator: zeroed by default, filled on request,
// empty at n=0, and catchable errors for a bad size or fill.
func TestMake(t *testing.T) {
	out, err := runProg(t, `
		use io; use binary;
		def z as bytes init binary.make(3);
		def f as bytes init binary.make(2, 255);
		def e as bytes init binary.make(0);
		io.printf("%d,%d,%d,%d/", len($z), $z[0], $z[1], $z[2]);
		io.printf("%d,%d,%d/", len($f), $f[0], $f[1]);
		io.printf("%d", len($e));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "3,0,0,0/2,255,255/0" {
		t.Fatalf("got %q", out)
	}
	// A negative size, an oversized size, and an out-of-range fill each throw a
	// catchable, distinguishable error rather than panicking.
	for _, c := range []struct{ src, want string }{
		{`use binary; def b as bytes init binary.make(-1);`, "n must be >= 0"},
		{`use binary; def b as bytes init binary.make(268435457);`, "exceeds the"},
		{`use binary; def b as bytes init binary.make(2, 256);`, "fill must be a byte value"},
		{`use binary; def b as bytes init binary.make(2, -1);`, "fill must be a byte value"},
	} {
		_, e := runProg(t, c.src)
		if e == nil || !strings.Contains(e.Error(), c.want) {
			t.Errorf("make(%q): want error containing %q, got %v", c.src, c.want, e)
		}
	}
}

// TestIndexOfFrom covers the optional start offset: find-all in one linear pass,
// resume semantics, an empty needle at the offset, and out-of-range errors.
func TestIndexOfFrom(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def b as bytes init convert.bytesFromString("a.b.c.d", "utf-8");
		def dot as bytes init convert.bytesFromString(".", "utf-8");
		def count as int init 0;
		def i as int init binary.indexOf($b, $dot, 0);
		while ($i >= 0) {
			$count = $count + 1;
			$i = binary.indexOf($b, $dot, $i + 1);
		}
		# count / first (2-arg) / resume at 2 / at end / empty needle at 3
		io.printf("%d/%d/%d/%d/%d", $count, binary.indexOf($b, $dot),
			binary.indexOf($b, $dot, 2), binary.indexOf($b, $dot, 7),
			binary.indexOf($b, convert.bytesFromString("", "utf-8"), 3));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "3/1/3/-1/3" {
		t.Fatalf("got %q, want %q", out, "3/1/3/-1/3")
	}
	for _, c := range []struct{ src, want string }{
		{`use convert; use binary; def b as bytes init convert.bytesFromString("ab","utf-8"); def i as int init binary.indexOf($b, $b, -1);`, "out of range"},
		{`use convert; use binary; def b as bytes init convert.bytesFromString("ab","utf-8"); def i as int init binary.indexOf($b, $b, 3);`, "out of range"},
		{`use convert; use binary; def b as bytes init convert.bytesFromString("ab","utf-8"); def i as int init binary.indexOf($b, $b, 0, 3);`, "out of range"},
		{`use convert; use binary; def b as bytes init convert.bytesFromString("ab","utf-8"); def i as int init binary.indexOf($b, $b, 2, 1);`, "out of range"},
	} {
		_, e := runProg(t, c.src)
		if e == nil || !strings.Contains(e.Error(), c.want) {
			t.Errorf("indexOf bounds: want error containing %q, got %v", c.want, e)
		}
	}
}

// TestIndexOfLimit covers the half-open [from, limit) search window: a match is
// reported only when it lies wholly inside it, and the scan never reads past
// limit - so an absent probe in a short range does not overscan the rest of the
// buffer.
func TestIndexOfLimit(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def b as bytes init convert.bytesFromString("a.b.c.d", "utf-8");
		def dot as bytes init convert.bytesFromString(".", "utf-8");
		def ab as bytes init convert.bytesFromString("ab", "utf-8");
		# first dot within [0,3) is index 1; within [2,3) none (-1);
		# a two-byte needle straddling the boundary does not match within [0,2) (-1);
		# full buffer via explicit limit finds it; empty needle matches at from.
		io.printf("%d/%d/%d/%d/%d",
			binary.indexOf($b, $dot, 0, 3),
			binary.indexOf($b, $dot, 2, 3),
			binary.indexOf(convert.bytesFromString("xabx","utf-8"), $ab, 0, 2),
			binary.indexOf($b, $dot, 0, 7),
			binary.indexOf($b, convert.bytesFromString("","utf-8"), 4, 7));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "1/-1/-1/1/4" {
		t.Fatalf("got %q, want %q", out, "1/-1/-1/1/4")
	}
}

// TestConcatSliceFind covers the core one-shot byte ops.
func TestConcatSliceFind(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def a as bytes init convert.bytesFromString("Hello, ", "utf-8");
		def b as bytes init convert.bytesFromString("World", "utf-8");
		def c as bytes init binary.concat($a, $b);
		io.printf("%s/", convert.stringFromBytes($c, "utf-8"));
		io.printf("%d/", binary.indexOf($c, $b));
		io.printf("%d/", binary.indexOf($c, convert.bytesFromString("zzz", "utf-8")));
		io.printf("%s/", convert.stringFromBytes(binary.slice($c, 7, 12), "utf-8"));
		io.printf("%s", convert.stringFromBytes(binary.slice($c, 7), "utf-8"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "Hello, World/7/-1/World/World" {
		t.Fatalf("got %q", out)
	}
}

// TestSplitAndPrefix covers split (list of bytes) plus startsWith / endsWith.
func TestSplitAndPrefix(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def b as bytes init convert.bytesFromString("a,bb,ccc", "utf-8");
		def sep as bytes init convert.bytesFromString(",", "utf-8");
		def parts as list of bytes init binary.split($b, $sep);
		io.printf("%d/%s/", len($parts), convert.stringFromBytes($parts[1], "utf-8"));
		io.printf("%t/%t", binary.startsWith($b, convert.bytesFromString("a,", "utf-8")),
		    binary.endsWith($b, convert.bytesFromString("bb", "utf-8")));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "3/bb/true/false" {
		t.Fatalf("got %q", out)
	}
}

// TestValueSemanticsPreserved - binary ops never alias their inputs: mutating a
// concat result, a slice result, or a split part must not leak into the source.
// slice and split are the ones that matter - Go's b[lo:hi] and bytes.Split both
// return windows into the source backing array, so each result MUST be copied.
func TestValueSemanticsPreserved(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def a as bytes init convert.bytesFromString("abcdef", "utf-8");
		# concat result is independent of both inputs
		def c as bytes init binary.concat($a, convert.bytesFromString("XYZ", "utf-8"));
		$c[0] = 122;
		# slice result is a copy, not a window into $a
		def s as bytes init binary.slice($a, 1, 4);
		$s[0] = 122;
		# a split part is a copy, not a window into $a
		def parts as list of bytes init binary.split($a, convert.bytesFromString("c", "utf-8"));
		$parts[0][0] = 122;
		io.printf("%s", convert.stringFromBytes($a, "utf-8"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "abcdef" {
		t.Fatalf("a binary result aliased its source (got %q, want \"abcdef\")", out)
	}
}

// TestSliceOutOfBounds - an out-of-range slice is a catchable error.
func TestSliceOutOfBounds(t *testing.T) {
	_, err := runProg(t, `
		use convert; use binary;
		def a as bytes init convert.bytesFromString("abc", "utf-8");
		def x as bytes init binary.slice($a, 0, 9);
	`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds error, got %v", err)
	}
}

// TestSplitEmptySep - an empty separator is a catchable error.
func TestSplitEmptySep(t *testing.T) {
	_, err := runProg(t, `
		use convert; use binary;
		def a as bytes init convert.bytesFromString("abc", "utf-8");
		def x as list of bytes init binary.split($a, convert.bytesFromString("", "utf-8"));
	`)
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("expected non-empty-separator error, got %v", err)
	}
}

// TestContains mirrors strings.contains for bytes (the boolean sibling of indexOf).
func TestContains(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def b as bytes init convert.bytesFromString("hello world", "utf-8");
		io.printf("%t/%t", binary.contains($b, convert.bytesFromString("o w", "utf-8")),
		    binary.contains($b, convert.bytesFromString("zzz", "utf-8")));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "true/false" {
		t.Fatalf("got %q", out)
	}
}

// TestJoin covers binary.join: no-sep, with-sep, empty list, round-trip with
// split, and the non-bytes-element error.
func TestJoin(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def parts as list of bytes init [
			convert.bytesFromString("a", "utf-8"),
			convert.bytesFromString("bb", "utf-8"),
			convert.bytesFromString("ccc", "utf-8")
		];
		def sep as bytes init convert.bytesFromString(",", "utf-8");
		io.printf("%s/", convert.stringFromBytes(binary.join($parts), "utf-8"));
		io.printf("%s/", convert.stringFromBytes(binary.join($parts, $sep), "utf-8"));
		def empty as list of bytes init [];
		io.printf("%d/", len(binary.join($empty)));
		def one as list of bytes init [convert.bytesFromString("x", "utf-8")];
		io.printf("%s", convert.stringFromBytes(binary.join($one, $sep), "utf-8"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "abbccc/a,bb,ccc/0/x" {
		t.Fatalf("got %q", out)
	}
}

// TestJoinRoundTripsSplit - join(split(b, sep), sep) == b.
func TestJoinRoundTrips(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def b as bytes init convert.bytesFromString("one,two,three", "utf-8");
		def sep as bytes init convert.bytesFromString(",", "utf-8");
		def back as bytes init binary.join(binary.split($b, $sep), $sep);
		io.printf("%s", convert.stringFromBytes($back, "utf-8"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "one,two,three" {
		t.Fatalf("got %q", out)
	}
}

// TestJoinNonBytesElement - a non-bytes element is a catchable error.
func TestJoinNonBytesElement(t *testing.T) {
	_, err := runProg(t, `
		use convert; use binary;
		def parts as list of int init [1, 2, 3];
		def x as bytes init binary.join($parts);
	`)
	if err == nil || !strings.Contains(err.Error(), "expected bytes") {
		t.Fatalf("expected list-of-bytes type error, got %v", err)
	}
}
