// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

// TestValueSemanticsPreserved - binary ops never alias or mutate their inputs.
func TestValueSemanticsPreserved(t *testing.T) {
	out, err := runProg(t, `
		use io; use convert; use binary;
		def a as bytes init convert.bytesFromString("abc", "utf-8");
		def c as bytes init binary.concat($a, convert.bytesFromString("XYZ", "utf-8"));
		$c[0] = 122;
		io.printf("%s", convert.stringFromBytes($a, "utf-8"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "abc" {
		t.Fatalf("mutation of concat result leaked into source: got %q", out)
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
