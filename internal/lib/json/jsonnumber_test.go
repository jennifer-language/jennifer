// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestNumberGrammarConformance pins the decoder to the json.org number
// grammar: -? (0 | [1-9][0-9]*) (. [0-9]+)? ([eE][+-]? [0-9]+)?. strconv is
// more permissive (leading zeros, a bare trailing dot), so these cases guard
// the explicit grammar walk in parseNumber.
func TestNumberGrammarConformance(t *testing.T) {
	valid := []string{
		"0", "-0", "42", "-42", "10", "100",
		"3.14", "-3.14", "0.5",
		"1e10", "6.022e23", "1E-5", "2.5e+3",
	}
	for _, s := range valid {
		if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(s)}); err != nil {
			t.Errorf("decode(%q): unexpected error %v", s, err)
		}
	}

	invalid := []string{
		"01", "-01", "00", // leading zeros
		"1.", "1.e3", // fraction with no digit
		".5",        // no integer part
		"1e", "1e+", // exponent with no digit
		"+1",              // leading plus
		"0x10",            // hex
		"1.2.3",           // two dots
		"Infinity", "NaN", // not JSON
	}
	for _, s := range invalid {
		if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(s)}); err == nil {
			t.Errorf("decode(%q): expected a grammar error, got none", s)
		}
	}
}
