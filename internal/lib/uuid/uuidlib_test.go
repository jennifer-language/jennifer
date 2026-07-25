// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package uuidlib

import (
	"strings"
	"testing"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestGenerateV4(t *testing.T) {
	s := genV4()
	b, ok := parse(s)
	if !ok {
		t.Fatalf("v4 %q did not parse", s)
	}
	if b[6]>>4 != 4 {
		t.Errorf("version nibble = %d, want 4", b[6]>>4)
	}
	if b[8]>>6 != 0b10 {
		t.Errorf("variant bits = %02b, want 10", b[8]>>6)
	}
	if len(s) != 36 {
		t.Errorf("length = %d, want 36", len(s))
	}
}

func TestGenerateV7(t *testing.T) {
	s := genV7()
	b, ok := parse(s)
	if !ok {
		t.Fatalf("v7 %q did not parse", s)
	}
	if b[6]>>4 != 7 {
		t.Errorf("version nibble = %d, want 7", b[6]>>4)
	}
	if b[8]>>6 != 0b10 {
		t.Errorf("variant bits = %02b, want 10", b[8]>>6)
	}
}

func TestV7SortsByCreationTime(t *testing.T) {
	orig := nowFunc
	defer func() { nowFunc = orig }()
	base := stdtime.UnixMilli(1_700_000_000_000)

	nowFunc = func() stdtime.Time { return base }
	early := genV7()
	nowFunc = func() stdtime.Time { return base.Add(stdtime.Hour) }
	late := genV7()

	if !(early < late) {
		t.Errorf("v7 not time-ordered: early=%s late=%s", early, late)
	}
}

func TestParseRoundTrip(t *testing.T) {
	s := genV4()
	b, ok := parse(s)
	if !ok {
		t.Fatalf("parse(%q) failed", s)
	}
	if format(b) != s {
		t.Errorf("round-trip: %q -> %q", s, format(b))
	}
	// Parsing is case-insensitive; NIL is a valid UUID.
	if _, ok := parse(strings.ToUpper(s)); !ok {
		t.Errorf("uppercase %q should parse", strings.ToUpper(s))
	}
	if _, ok := parse(NIL); !ok {
		t.Error("NIL should parse")
	}
}

func TestParseInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"not-a-uuid",
		"00000000-0000-0000-0000-00000000000",   // one short
		"00000000-0000-0000-0000-0000000000000", // one long
		"00000000x0000-0000-0000-000000000000",  // wrong separator
		"0000000g-0000-0000-0000-000000000000",  // non-hex digit
	} {
		if _, ok := parse(s); ok {
			t.Errorf("parse(%q) should have failed", s)
		}
	}
}

func TestVersionOfNilIsZero(t *testing.T) {
	b, ok := parse(NIL)
	if !ok || b[6]>>4 != 0 {
		t.Errorf("NIL version = %d, want 0", b[6]>>4)
	}
}

func TestGeneratorFns(t *testing.T) {
	out, err := v4Fn(interpreter.BuiltinCtx{}, nil)
	if err != nil {
		t.Fatalf("v4: %v", err)
	}
	if b, ok := parse(out.Str); !ok || b[6]>>4 != 4 {
		t.Errorf("v4() produced %q (version %d), want a valid v4", out.Str, b[6]>>4)
	}
	out7, err := v7Fn(interpreter.BuiltinCtx{}, nil)
	if err != nil {
		t.Fatalf("v7: %v", err)
	}
	if b, ok := parse(out7.Str); !ok || b[6]>>4 != 7 {
		t.Errorf("v7() produced %q (version %d), want a valid v7", out7.Str, b[6]>>4)
	}
	if _, err := v4Fn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("v4")}); err == nil {
		t.Error("expected an arity error when v4 is given an argument")
	}
	if _, err := v7Fn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("expected an arity error when v7 is given an argument")
	}
}
