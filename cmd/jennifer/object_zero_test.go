// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"path/filepath"
	"testing"
)

// A struct field (or enum payload field) of an opaque library type zeroes to
// that type's empty value - a JSON null, an empty XML text node - the same as a
// `def v as json.Value;`, not to a fieldless struct the library rejects.
func TestOpaqueTypeFieldZeroValue(t *testing.T) {
	src := `use io;
use json;
use xml;
def struct S { v as json.Value, x as xml.Value, n as int };
def s as S;
io.printf("%s %s %v\n", json.typeOf($s.v), xml.typeOf($s.x), $s.v);
$s.v = json.decode("[1]");
io.printf("%v\n", $s.v);
`
	out, err := runProgramOutput(filepath.Join(t.TempDir(), "zero.j"), src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "null text null\n[1]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
