// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package binarylib_test

import (
	"strings"
	"testing"
)

// bytesOf is a Jennifer snippet building $b from the given byte values.
func bytesOf(vals string) string {
	return "def b as bytes; for (def x in [" + vals + "]) { $b[] = $x; }\n"
}

// TestReadIntAndUint covers both byte orders, every width, sign extension
// and the unsigned range.
func TestReadIntAndUint(t *testing.T) {
	out, err := runProg(t, "use io; use binary;\n"+bytesOf("255, 254, 1, 0, 0, 128")+`
		io.printf("%d %d\n", binary.readInt($b, 0, 1, "le"), binary.readUint($b, 0, 1, "le"));
		io.printf("%d %d\n", binary.readInt($b, 0, 2, "le"), binary.readInt($b, 0, 2, "be"));
		io.printf("%d %d\n", binary.readUint($b, 0, 2, "le"), binary.readUint($b, 0, 2, "be"));
		io.printf("%d\n", binary.readInt($b, 2, 4, "le"));
		io.printf("%d\n", binary.readUint($b, 0, 4, "be"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := "-1 255\n-257 -2\n65279 65534\n-2147483647\n4294836480\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	out, err = runProg(t, "use io; use binary;\n"+bytesOf("1, 0, 0, 0, 0, 0, 0, 128, 255, 255, 255, 255, 255, 255, 255, 127")+`
		io.printf("%d\n", binary.readInt($b, 0, 8, "le"));
		io.printf("%d\n", binary.readUint($b, 8, 8, "le"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "-9223372036854775807\n9223372036854775807\n" {
		t.Fatalf("got %q", out)
	}
}

// TestReadFloat covers float32 and float64 in both orders.
func TestReadFloat(t *testing.T) {
	out, err := runProg(t, "use io; use binary;\n"+bytesOf("63, 248, 0, 0, 0, 0, 0, 0, 0, 0, 192, 63")+`
		io.printf("%v\n", binary.readFloat($b, 0, 8, "be"));
		io.printf("%v\n", binary.readFloat($b, 8, 4, "le"));
		io.printf("%v\n", binary.readFloat($b, 4, 8, "le"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || lines[0] != "1.5" || lines[1] != "1.5" {
		t.Fatalf("got %q", out)
	}
}

// TestReadNumberErrors: every refusal is a catchable error with a message
// naming the problem.
func TestReadNumberErrors(t *testing.T) {
	cases := []struct{ call, want string }{
		{`binary.readInt($b, 7, 2, "le")`, "run past the end"},
		{`binary.readInt($b, -1, 1, "le")`, "run past the end"},
		{`binary.readInt($b, 0, 3, "le")`, "size must be one of"},
		{`binary.readInt($b, 0, 2, "LE")`, `order must be "le" or "be"`},
		{`binary.readUint($b, 0, 8, "be")`, "exceeds the int range"},
		{`binary.readFloat($b, 0, 2, "be")`, "size must be one of"},
		{`binary.readFloat($b, 0, 8, "be")`, "not a finite number"},
		{`binary.readFloat($b, 0, 4, "be")`, "not a finite number"},
		{`binary.readInts($b, 0, 9, 1, "le")`, "count must be in"},
		{`binary.readInts($b, 0, 2, 2, "le", 0)`, "stride must be >= 1"},
		{`binary.readInts($b, 0, 3, 2, "le", 4)`, "run past the end"},
		{`binary.readFloats($b, 0, 1, 8, "be")`, "not a finite number"},
		{`binary.readInts($b, 0, 2, 2, "le", 9223372036854775807)`, "run past the end"},
	}
	for _, c := range cases {
		// 255 255 ... : float NaN in both widths, uint64 above the int range.
		_, err := runProg(t, "use binary;\n"+bytesOf("255, 255, 255, 255, 255, 255, 255, 255")+c.call+";\n")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.call, err, c.want)
		}
	}
}

// TestReadBulk covers strided column reads and the empty read.
func TestReadBulk(t *testing.T) {
	out, err := runProg(t, "use io; use binary;\n"+bytesOf("1, 0, 9, 2, 0, 9, 255, 255, 9")+`
		io.printf("%v\n", binary.readInts($b, 0, 3, 2, "le", 3));
		io.printf("%v\n", binary.readInts($b, 0, 2, 1, "le"));
		io.printf("%d\n", len(binary.readFloats($b, 0, 0, 8, "be")));
	`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[1, 2, -1]\n[1, 0]\n0\n" {
		t.Fatalf("got %q", out)
	}
	out, err = runProg(t, "use io; use binary;\n"+bytesOf("63, 248, 0, 0, 0, 0, 0, 0, 64, 4, 0, 0, 0, 0, 0, 0")+`
		io.printf("%v\n", binary.readFloats($b, 0, 2, 8, "be"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[1.5, 2.5]\n" {
		t.Fatalf("got %q", out)
	}
}
