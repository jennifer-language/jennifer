// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
		def b as bytes init convert.bytesFromString("utf-8", "a.b.c.d");
		def dot as bytes init convert.bytesFromString("utf-8", ".");
		def count as int init 0;
		def i as int init binary.indexOf($b, $dot, 0);
		while ($i >= 0) {
			$count = $count + 1;
			$i = binary.indexOf($b, $dot, $i + 1);
		}
		# count / first (2-arg) / resume at 2 / at end / empty needle at 3
		io.printf("%d/%d/%d/%d/%d", $count, binary.indexOf($b, $dot),
			binary.indexOf($b, $dot, 2), binary.indexOf($b, $dot, 7),
			binary.indexOf($b, convert.bytesFromString("utf-8", ""), 3));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "3/1/3/-1/3" {
		t.Fatalf("got %q, want %q", out, "3/1/3/-1/3")
	}
	for _, c := range []struct{ src, want string }{
		{`use convert; use binary; def b as bytes init convert.bytesFromString("utf-8", "ab"); def i as int init binary.indexOf($b, $b, -1);`, "out of range"},
		{`use convert; use binary; def b as bytes init convert.bytesFromString("utf-8", "ab"); def i as int init binary.indexOf($b, $b, 3);`, "out of range"},
		{`use convert; use binary; def b as bytes init convert.bytesFromString("utf-8", "ab"); def i as int init binary.indexOf($b, $b, 0, 3);`, "out of range"},
		{`use convert; use binary; def b as bytes init convert.bytesFromString("utf-8", "ab"); def i as int init binary.indexOf($b, $b, 2, 1);`, "out of range"},
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
		def b as bytes init convert.bytesFromString("utf-8", "a.b.c.d");
		def dot as bytes init convert.bytesFromString("utf-8", ".");
		def ab as bytes init convert.bytesFromString("utf-8", "ab");
		# first dot within [0,3) is index 1; within [2,3) none (-1);
		# a two-byte needle straddling the boundary does not match within [0,2) (-1);
		# full buffer via explicit limit finds it; empty needle matches at from.
		io.printf("%d/%d/%d/%d/%d",
			binary.indexOf($b, $dot, 0, 3),
			binary.indexOf($b, $dot, 2, 3),
			binary.indexOf(convert.bytesFromString("utf-8", "xabx"), $ab, 0, 2),
			binary.indexOf($b, $dot, 0, 7),
			binary.indexOf($b, convert.bytesFromString("utf-8", ""), 4, 7));
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
		def a as bytes init convert.bytesFromString("utf-8", "Hello, ");
		def b as bytes init convert.bytesFromString("utf-8", "World");
		def c as bytes init binary.concat($a, $b);
		io.printf("%s/", convert.stringFromBytes("utf-8", $c));
		io.printf("%d/", binary.indexOf($c, $b));
		io.printf("%d/", binary.indexOf($c, convert.bytesFromString("utf-8", "zzz")));
		io.printf("%s/", convert.stringFromBytes("utf-8", binary.slice($c, 7, 12)));
		io.printf("%s", convert.stringFromBytes("utf-8", binary.slice($c, 7)));
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
		def b as bytes init convert.bytesFromString("utf-8", "a,bb,ccc");
		def sep as bytes init convert.bytesFromString("utf-8", ",");
		def parts as list of bytes init binary.split($b, $sep);
		io.printf("%d/%s/", len($parts), convert.stringFromBytes("utf-8", $parts[1]));
		io.printf("%t/%t", binary.startsWith($b, convert.bytesFromString("utf-8", "a,")),
		    binary.endsWith($b, convert.bytesFromString("utf-8", "bb")));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "3/bb/true/false" {
		t.Fatalf("got %q", out)
	}
}

// TestValueSemanticsPreserved - mutating a concat result, a slice result or a
// split part from `.j` leaves the source unchanged. End-to-end only: the
// `def` bind site copies builtin results, so this passes even for an aliased
// result; TestResultsDoNotAlias calls the builtins directly.
func TestValueSemanticsPreserved(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def a as bytes init convert.bytesFromString("utf-8", "abcdef");
		# concat result is independent of both inputs
		def c as bytes init binary.concat($a, convert.bytesFromString("utf-8", "XYZ"));
		$c[0] = 122;
		# slice result is a copy, not a window into $a
		def s as bytes init binary.slice($a, 1, 4);
		$s[0] = 122;
		# a split part is a copy, not a window into $a
		def parts as list of bytes init binary.split($a, convert.bytesFromString("utf-8", "c"));
		$parts[0][0] = 122;
		io.printf("%s", convert.stringFromBytes("utf-8", $a));
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
		def a as bytes init convert.bytesFromString("utf-8", "abc");
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
		def a as bytes init convert.bytesFromString("utf-8", "abc");
		def x as list of bytes init binary.split($a, convert.bytesFromString("utf-8", ""));
	`)
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("expected non-empty-separator error, got %v", err)
	}
}

// TestContains mirrors strings.contains for bytes (the boolean sibling of indexOf).
func TestContains(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def b as bytes init convert.bytesFromString("utf-8", "hello world");
		io.printf("%t/%t", binary.contains($b, convert.bytesFromString("utf-8", "o w")),
		    binary.contains($b, convert.bytesFromString("utf-8", "zzz")));
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
			convert.bytesFromString("utf-8", "a"),
			convert.bytesFromString("utf-8", "bb"),
			convert.bytesFromString("utf-8", "ccc")
		];
		def sep as bytes init convert.bytesFromString("utf-8", ",");
		io.printf("%s/", convert.stringFromBytes("utf-8", binary.join($parts)));
		io.printf("%s/", convert.stringFromBytes("utf-8", binary.join($parts, $sep)));
		def empty as list of bytes init [];
		io.printf("%d/", len(binary.join($empty)));
		def one as list of bytes init [convert.bytesFromString("utf-8", "x")];
		io.printf("%s", convert.stringFromBytes("utf-8", binary.join($one, $sep)));
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
		def b as bytes init convert.bytesFromString("utf-8", "one,two,three");
		def sep as bytes init convert.bytesFromString("utf-8", ",");
		def back as bytes init binary.join(binary.split($b, $sep), $sep);
		io.printf("%s", convert.stringFromBytes("utf-8", $back));
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

// TestBitXor covers an equal-length key, a repeating key (the WebSocket 4-byte
// mask, here over 70 bytes so the repeated block wraps), empty data, the
// involution property, and the errors.
func TestBitXor(t *testing.T) {
	out, err := runProg(t, `
		use io; use binary; use convert;
		def a as bytes init convert.bytesFromString("utf-8", "abc");
		def k as bytes init convert.bytesFromString("utf-8", "   ");
		io.printf("%s/", convert.stringFromBytes("utf-8", binary.bitXor($a, $k)));
		def data as bytes init binary.make(70, 7);
		def mask as bytes;
		$mask[] = 1; $mask[] = 2; $mask[] = 3; $mask[] = 4;
		def m as bytes init binary.bitXor($data, $mask);
		def ok as bool init len($m) == 70;
		for (def i in 0..70) {
			if ($m[$i] != (7 ^ $mask[$i % 4])) { $ok = false; }
		}
		io.printf("%v/", $ok);
		io.printf("%v/", binary.bitXor($m, $mask) == $data);
		io.printf("%d", len(binary.bitXor(binary.make(0), $mask)));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "ABC/true/true/0" {
		t.Fatalf("got %q", out)
	}
	for _, c := range []struct{ src, want string }{
		{`use binary; def b as bytes init binary.bitXor(binary.make(3), binary.make(0));`, "key must not be empty"},
		{`use binary; def b as bytes init binary.bitXor(binary.make(3));`, "expects 2 arguments"},
		{`use binary; def b as bytes init binary.bitXor("x", binary.make(1));`, "data must be bytes"},
	} {
		_, e := runProg(t, c.src)
		if e == nil || !strings.Contains(e.Error(), c.want) {
			t.Errorf("bitXor(%q): want error containing %q, got %v", c.src, c.want, e)
		}
	}
}

// TestBitAndOr covers bitAnd / bitOr with an equal-length key and a repeating one (a
// netmask-style mask over 70 bytes), and the shared argument checks.
func TestBitAndOr(t *testing.T) {
	out, err := runProg(t, `
		use io; use binary;
		def data as bytes init binary.make(70, 0xAC);
		def m as bytes;
		$m[] = 0xF0; $m[] = 0x0F;
		def a as bytes init binary.bitAnd($data, $m);
		def o as bytes init binary.bitOr($data, $m);
		def ok as bool init len($a) == 70 and len($o) == 70;
		for (def i in 0..70) {
			if ($a[$i] != (0xAC & $m[$i % 2]) or $o[$i] != (0xAC | $m[$i % 2])) { $ok = false; }
		}
		io.printf("%v/", $ok);
		def one as bytes init binary.make(1, 0x0F);
		io.printf("%d,%d", binary.bitAnd(binary.make(1, 0xFF), $one)[0], binary.bitOr(binary.make(1, 0xF0), $one)[0]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "true/15,255" {
		t.Fatalf("got %q", out)
	}
	for _, c := range []struct{ src, want string }{
		{`use binary; def b as bytes init binary.bitAnd(binary.make(3), binary.make(0));`, "binary.bitAnd: key must not be empty"},
		{`use binary; def b as bytes init binary.bitOr(binary.make(3));`, "binary.bitOr expects 2 arguments"},
	} {
		_, e := runProg(t, c.src)
		if e == nil || !strings.Contains(e.Error(), c.want) {
			t.Errorf("%q: want error containing %q, got %v", c.src, c.want, e)
		}
	}
}

// TestBitNot inverts every byte, and bitNot twice is the identity.
func TestBitNot(t *testing.T) {
	out, err := runProg(t, `
		use io; use binary;
		def b as bytes;
		$b[] = 0; $b[] = 0x0F; $b[] = 0xFF;
		def n as bytes init binary.bitNot($b);
		io.printf("%d,%d,%d/%v/%d", $n[0], $n[1], $n[2], binary.bitNot($n) == $b, len(binary.bitNot(binary.make(0))));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "255,240,0/true/0" {
		t.Fatalf("got %q", out)
	}
	if _, e := runProg(t, `use binary; def b as bytes init binary.bitNot(binary.make(1), binary.make(1));`); e == nil || !strings.Contains(e.Error(), "expects 1 argument") {
		t.Errorf("bitNot arity: got %v", e)
	}
}

// TestEdgeCases covers empty and boundary inputs across the library.
func TestEdgeCases(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		func s(x as string) { return convert.bytesFromString("utf-8", $x); }
		func show(ps as list of bytes) {
			def r as string init "";
			for (def p in $ps) { $r = $r + "[" + convert.stringFromBytes("utf-8", $p) + "]"; }
			return $r;
		}
		def e as bytes;
		io.printf("%s\n", show(binary.split($e, s(","))));
		io.printf("%s\n", show(binary.split(s(",a,,b,"), s(","))));
		io.printf("%s\n", show(binary.split(s("a--b---c"), s("--"))));
		io.printf("%s\n", show(binary.split(s("ab"), s("abc"))));
		io.printf("%d %d\n", binary.indexOf(s("abc"), $e, 3), binary.indexOf(s("abc"), $e, 3, 3));
		io.printf("%t %t %t\n", binary.contains(s("ab"), $e), binary.startsWith(s("ab"), $e), binary.endsWith(s("ab"), $e));
		io.printf("%t %t %t\n", binary.contains(s("ab"), s("abc")), binary.startsWith(s("ab"), s("abc")), binary.endsWith(s("ab"), s("abc")));
		io.printf("%d\n", len(binary.make(0, 255)));
		io.printf("%d\n", len(binary.slice(s("abc"), 3, 3)));
		io.printf("%d\n", len(binary.bitXor(s("ab"), s("0123456789"))));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "[]\n[][a][][b][]\n[a][b][-c]\n[ab]\n3 3\ntrue true true\nfalse false false\n0\n0\n2\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestArgumentErrors covers wrong argument kinds and bounds not covered above.
func TestArgumentErrors(t *testing.T) {
	cases := []struct{ call, want string }{
		{`binary.join($a)`, "first argument must be a list of bytes"},
		{`binary.join([$a], "x")`, "separator must be bytes"},
		{`binary.indexOf($a, $a, "0")`, "from must be int"},
		{`binary.indexOf($a, $a, 0, "1")`, "limit must be int"},
		{`binary.make("1")`, "n must be int"},
		{`binary.make(1, "1")`, "fill must be int"},
		{`binary.slice($a, 0, -1)`, "out of bounds"},
	}
	for _, c := range cases {
		_, err := runProg(t, "use convert; use binary;\ndef a as bytes init convert.bytesFromString(\"utf-8\", \"abc\");\n"+c.call+";\n")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.call, err, c.want)
		}
	}
}
