// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import "testing"

// convert.toCodepoint / convert.fromCodepoint are the rune <-> code-point pair
// the idna module's Punycode arithmetic builds on.
func TestConvertCodepointRoundTrip(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
io.printf("%d %d %d\n", convert.toCodepoint("A"), convert.toCodepoint("é"), convert.toCodepoint("😀"));
io.printf("%s%s%s\n", convert.fromCodepoint(65), convert.fromCodepoint(233), convert.fromCodepoint(128512));
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "65 233 128512\nAé😀\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConvertCodepointErrors(t *testing.T) {
	cases := []string{
		`use convert; convert.toCodepoint("ab");`,    // more than one character
		`use convert; convert.toCodepoint("");`,      // zero characters
		`use convert; convert.fromCodepoint(-1);`,    // negative
		`use convert; convert.fromCodepoint(55296);`, // surrogate (0xD800)
	}
	for _, src := range cases {
		if _, err := run(t, src); err == nil {
			t.Errorf("expected an error for: %s", src)
		}
	}
}
