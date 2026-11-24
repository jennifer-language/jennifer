// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package decimallib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestToStringReplacements (M29.48): decimal.toString (deprecated) and its
// replacements convert.toString and %v render every Decimal identically.
func TestToStringReplacements(t *testing.T) {
	prog, err := parser.Parse(`
use io; use convert; use decimal;
for (def s in ["0", "-0.00", "1.50", "123456789012345678901234567890.000000001", "-7e-30", "1e25"]) {
    def d as decimal.Decimal init decimal.from($s);
    def a as string init decimal.toString($d);
    if ($a != convert.toString($d) or $a != io.sprintf("%v", $d)) {
        io.printf("differ: %s %s %s\n", $a, convert.toString($d), io.sprintf("%v", $d));
    }
    io.printf("%s;", $a);
}
`)
	if err != nil {
		t.Fatal(err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "0;0.00;1.50;123456789012345678901234567890.000000001;-0.000000000000000000000000000007;10000000000000000000000000;"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
