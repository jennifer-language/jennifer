// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// ---- Non-decimal int literals + digit separators ----

func TestHexLiteral(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 0xff);
io.printf("%d\n", 0xDEAD_BEEF);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "255\n3735928559\n" {
		t.Errorf("got %q", out)
	}
}

func TestOctalLiteral(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 0o755);
io.printf("%d\n", 0o1_000);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "493\n512\n" {
		t.Errorf("got %q", out)
	}
}

func TestBinaryLiteral(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 0b1010_0110);
io.printf("%d\n", 0b1111_1111);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "166\n255\n" {
		t.Errorf("got %q", out)
	}
}

func TestDecimalSeparator(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 1_000_000);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1000000\n" {
		t.Errorf("got %q", out)
	}
}

func TestSeparatorAtBoundaryErrors(t *testing.T) {
	cases := []string{
		`use io; io.printf("%d", 100_);`,
		`use io; io.printf("%d", 1__000);`,
		`use io; io.printf("%d", 0x_ff);`,
	}
	for _, src := range cases {
		_, err := run(t, src)
		if err == nil {
			t.Errorf("expected lex error for %q, got nil", src)
		}
	}
}

// ---- Bit operators ----

func TestBitwiseAndOrXor(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 0xff & 0x0f);
io.printf("%d\n", 0x0f | 0xf0);
io.printf("%d\n", 0xff ^ 0xaa);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "15\n255\n85\n" {
		t.Errorf("got %q", out)
	}
}

func TestBitwiseNot(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", ~0);
io.printf("%d\n", ~0xff);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "-1\n-256\n" {
		t.Errorf("got %q", out)
	}
}

func TestShifts(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 1 << 8);
io.printf("%d\n", 256 >> 1);
io.printf("%d\n", 0 - 8 >> 2);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "256\n128\n-2\n" {
		t.Errorf("got %q", out)
	}
}

// Python-style precedence: bitwise AND binds tighter than == .
func TestBitOpPrecedenceVsComparison(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%t\n", 1 & 0xff == 0);
io.printf("%t\n", 2 & 3 == 2);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// 1 & 0xff -> 1; 1 == 0 -> false. 2 & 3 -> 2; 2 == 2 -> true.
	if out != "false\ntrue\n" {
		t.Errorf("got %q", out)
	}
}

func TestShiftCountNegativeErrors(t *testing.T) {
	_, err := run(t, `use io; io.printf("%d", 1 << (0 - 1));`)
	if err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("expected non-negative shift error, got %v", err)
	}
}

func TestShiftLargeCountSaturates(t *testing.T) {
	out, err := run(t, `
use io;
io.printf("%d\n", 1 << 65);
io.printf("%d\n", (0 - 1) >> 65);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// `<<` past int width is 0; arithmetic `>>` of a negative is -1.
	if out != "0\n-1\n" {
		t.Errorf("got %q", out)
	}
}

func TestBitOpOnFloatErrors(t *testing.T) {
	_, err := run(t, `use io; io.printf("%d", 1.5 & 2);`)
	if err == nil || !strings.Contains(err.Error(), "int operands") {
		t.Errorf("expected int-required error, got %v", err)
	}
}

func TestBitNotOnNonIntErrors(t *testing.T) {
	_, err := run(t, `use io; io.printf("%v", ~true);`)
	if err == nil || !strings.Contains(err.Error(), "requires int") {
		t.Errorf("expected int-required error, got %v", err)
	}
}

// ---- bytes type ----

func TestBytesFromStringRoundTrip(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
def b as bytes init convert.bytesFromString("Hello", "utf-8");
io.printf("len=%d\n", len($b));
io.printf("b0=%d\n", $b[0]);
def s as string init convert.stringFromBytes($b, "utf-8");
io.printf("s=%s\n", $s);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "len=5\nb0=72\ns=Hello\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestBytesIndexAssign(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
def b as bytes init convert.bytesFromString("Hi", "utf-8");
$b[0] = 0x68;
io.printf("%s\n", convert.stringFromBytes($b, "utf-8"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hi\n" {
		t.Errorf("got %q", out)
	}
}

func TestBytesAppendSugar(t *testing.T) {
	out, err := run(t, `
use io;
def b as bytes;
$b[] = 0x48;
$b[] = 0x69;
io.printf("len=%d b0=%d b1=%d\n", len($b), $b[0], $b[1]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "len=2 b0=72 b1=105\n" {
		t.Errorf("got %q", out)
	}
}

func TestBytesByteRangeRejected(t *testing.T) {
	_, err := run(t, `
use io;
def b as bytes;
$b[] = 256;
`)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestBytesOutOfBoundsRead(t *testing.T) {
	_, err := run(t, `
use io;
def b as bytes;
io.printf("%d", $b[0]);
`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("expected out-of-bounds error, got %v", err)
	}
}

// Bytes are value-typed: copy on assignment, mutation does not leak.
func TestBytesValueSemanticsOnAssign(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
def src as bytes init convert.bytesFromString("Hi", "utf-8");
def dst as bytes init $src;
$dst[0] = 0x78;
io.printf("src=%v dst=%v\n", $src, $dst);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// src must be unchanged.
	if !strings.Contains(out, "src=bytes[48 69]") {
		t.Errorf("src mutated: %q", out)
	}
}

func TestBytesValueSemanticsOnParameter(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
func mut(b as bytes) {
    $b[0] = 0x78;
}
def src as bytes init convert.bytesFromString("Hi", "utf-8");
mut($src);
io.printf("%v\n", $src);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "bytes[48 69]") {
		t.Errorf("src mutated through parameter: %q", out)
	}
}

func TestBytesConstRejectsIndexWrite(t *testing.T) {
	_, err := run(t, `
use io;
use convert;
def const B as bytes init convert.bytesFromString("Hi", "utf-8");
$B[0] = 0;
`)
	if err == nil || !strings.Contains(err.Error(), "cannot mutate contents of constant") {
		t.Errorf("expected const-mutation error, got %v", err)
	}
}

func TestBytesInvalidUTF8Rejected(t *testing.T) {
	_, err := run(t, `
use io;
use convert;
def b as bytes;
$b[] = 0xff;
def s as string init convert.stringFromBytes($b, "utf-8");
`)
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("expected UTF-8 error, got %v", err)
	}
}

func TestUnknownCodecRejected(t *testing.T) {
	_, err := run(t, `
use io;
use convert;
def b as bytes init convert.bytesFromString("hi", "iso-8859-1");
`)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected unsupported-codec error, got %v", err)
	}
}

// ---- io.readBytes / io.readChars end-to-end ----

func TestReadBytesExactCount(t *testing.T) {
	out, err := runWithStdin(t, `
use io;
def b as bytes init io.readBytes(5);
io.printf("%d\n", len($b));
io.printf("%d\n", $b[0]);
`, "Hello world")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5\n72\n" {
		t.Errorf("got %q", out)
	}
}

func TestReadBytesShortReadAtEOF(t *testing.T) {
	// Asking for more than available yields the partial result and
	// flips eof() to true.
	out, err := runWithStdin(t, `
use io;
def b as bytes init io.readBytes(100);
io.printf("got=%d eof=%t\n", len($b), io.eof());
`, "Hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "got=2 eof=true\n" {
		t.Errorf("got %q", out)
	}
}

func TestReadCharsHandlesMultiByteRunes(t *testing.T) {
	// Each rune may take 1-4 bytes; readChars counts runes, not bytes.
	out, err := runWithStdin(t, `
use io;
def s as string init io.readChars(3);
io.printf("%s len=%d\n", $s, len($s));
`, "héllo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// `len` on a string is rune count, so the read-3 result should be
	// "hél" with len 3.
	if out != "hél len=3\n" {
		t.Errorf("got %q", out)
	}
}
