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
		io.printf("%d %d\n", binary.readInt("le", $b, 0, 1), binary.readUint("le", $b, 0, 1));
		io.printf("%d %d\n", binary.readInt("le", $b, 0, 2), binary.readInt("be", $b, 0, 2));
		io.printf("%d %d\n", binary.readUint("le", $b, 0, 2), binary.readUint("be", $b, 0, 2));
		io.printf("%d\n", binary.readInt("le", $b, 2, 4));
		io.printf("%d\n", binary.readUint("be", $b, 0, 4));
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := "-1 255\n-257 -2\n65279 65534\n-2147483647\n4294836480\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	out, err = runProg(t, "use io; use binary;\n"+bytesOf("1, 0, 0, 0, 0, 0, 0, 128, 255, 255, 255, 255, 255, 255, 255, 127")+`
		io.printf("%d\n", binary.readInt("le", $b, 0, 8));
		io.printf("%d\n", binary.readUint("le", $b, 8, 8));
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
		io.printf("%v\n", binary.readFloat("be", $b, 0, 8));
		io.printf("%v\n", binary.readFloat("le", $b, 8, 4));
		io.printf("%v\n", binary.readFloat("le", $b, 4, 8));
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
		{`binary.readInt("le", $b, 7, 2)`, "2-byte value at offset 7 runs past the end of 8 bytes"},
		{`binary.readInt("le", $b, -1, 1)`, "offset -1 is negative"},
		{`binary.readInt("le", $b, 0, 3)`, "size must be one of"},
		{`binary.readInt("LE", $b, 0, 2)`, `order must be "le" or "be"`},
		{`binary.readUint("be", $b, 0, 8)`, "exceeds the int range"},
		{`binary.readFloat("be", $b, 0, 2)`, "size must be one of"},
		{`binary.readFloat("be", $b, 0, 8)`, "not a finite number"},
		{`binary.readFloat("be", $b, 0, 4)`, "not a finite number"},
		{`binary.readInts("le", $b, 0, 9, 1)`, "count must be in"},
		{`binary.readInts("le", $b, 0, 2, 2, 0)`, "stride must be >= 1"},
		{`binary.readInts("le", $b, 0, 3, 2, 4)`, "run past the end"},
		{`binary.readFloats("be", $b, 0, 1, 8)`, "not a finite number"},
		{`binary.readInts("le", $b, 0, 2, 2, 9223372036854775807)`, "run past the end"},
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
		io.printf("%v\n", binary.readInts("le", $b, 0, 3, 2, 3));
		io.printf("%v\n", binary.readInts("le", $b, 0, 2, 1));
		io.printf("%d\n", len(binary.readFloats("be", $b, 0, 0, 8)));
	`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[1, 2, -1]\n[1, 0]\n0\n" {
		t.Fatalf("got %q", out)
	}
	out, err = runProg(t, "use io; use binary;\n"+bytesOf("63, 248, 0, 0, 0, 0, 0, 0, 64, 4, 0, 0, 0, 0, 0, 0")+`
		io.printf("%v\n", binary.readFloats("be", $b, 0, 2, 8));
	`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[1.5, 2.5]\n" {
		t.Fatalf("got %q", out)
	}
}

// TestReadNumberEdges: the 8-byte minimum, big-endian with a stride, a
// count-1 read with stride past the end, and an empty read's offset check.
func TestReadNumberEdges(t *testing.T) {
	out, err := runProg(t, "use io; use binary;\n"+bytesOf("0, 0, 0, 0, 0, 0, 0, 128, 1, 2")+`
		io.printf("%d\n", binary.readInt("le", $b, 0, 8));
		io.printf("%v\n", binary.readInts("be", $b, 7, 2, 1, 2));
		io.printf("%v\n", binary.readInts("be", $b, 8, 2, 1, 1));
		io.printf("%v\n", binary.readUint("be", $b, 8, 2));
		io.printf("%v\n", binary.readInts("be", $b, 8, 1, 2, 99));
		io.printf("%d\n", len(binary.readInts("le", $b, 10, 0, 1)));
	`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "-9223372036854775808\n[-128, 2]\n[1, 2]\n258\n[258]\n0\n" {
		t.Fatalf("got %q", out)
	}
}

// TestReadNumberArgErrors: wrong kinds, arity, a count-0 read at a bad
// offset, and a bulk float error naming the failing element's offset.
func TestReadNumberArgErrors(t *testing.T) {
	cases := []struct{ call, want string }{
		{`binary.readInts("le", $b, -99, 0, 8)`, "offset -99 is outside 8 bytes"},
		{`binary.readInts("le", $b, 999, 0, 8)`, "offset 999 is outside 8 bytes"},
		{`binary.readInt("le", $b, "0", 1)`, "offset must be int"},
		{`binary.readInt("le", $b, 0, "1")`, "size must be int"},
		{`binary.readInt(1, $b, 0, 1)`, "order must be a string"},
		{`binary.readInts("le", $b, 0, 1, 1, "1")`, "stride must be int"},
		{`binary.readInts("le", $b, 0, "1", 1)`, "count must be int"},
		{`binary.readInt($b, 0, 1)`, "expects 4 arguments"},
		{`binary.readUint("le", $b, 0, 1, 1)`, "expects 4 arguments"},
		{`binary.readFloat($b)`, "expects 4 arguments"},
		{`binary.readInts($b, 0, 1, 1)`, "expects 5 or 6 arguments"},
		{`binary.readFloats("le", $b, 0, 1, 4, 4, 4)`, "expects 5 or 6 arguments"},
		{`binary.readFloats("le", $b, 0, 2, 4)`, "the 4 bytes at offset 4 are not a finite number"},
	}
	for _, c := range cases {
		// 0 0 0 0 | 255 255 255 255: float32 0 then a NaN.
		_, err := runProg(t, "use binary;\n"+bytesOf("0, 0, 0, 0, 255, 255, 255, 255")+c.call+";\n")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.call, err, c.want)
		}
	}
}
