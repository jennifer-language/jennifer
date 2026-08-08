// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package uuidlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// These tests exercise the builtin wrappers (parseFn / isValidFn / versionFn +
// their arg validation), which the core-logic tests leave uncovered.

func str(s string) interpreter.Value { return interpreter.StringVal(s) }

func TestInstallRegistersEveryUuidBuiltin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"v4", "v7", "parse", "isValid", "version"} {
		if in.LookupNamespacedBuiltin("uuid", name) == nil {
			t.Errorf("uuid.%s is not registered", name)
		}
	}
}

func TestParseFnBuiltin(t *testing.T) {
	// A well-formed v4 UUID: byte 6 is 0x41, so the version nibble is 4.
	const v4 = "550e8400-e29b-41d4-a716-446655440000"
	got, err := parseFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(v4)})
	if err != nil {
		t.Fatalf("parse(%q): %v", v4, err)
	}
	if got.Kind != interpreter.KindBytes || len(got.Bytes) != 16 {
		t.Fatalf("parse: got %+v, want 16 bytes", got)
	}
	if got.Bytes[0] != 0x55 || got.Bytes[6] != 0x41 || got.Bytes[15] != 0x00 {
		t.Errorf("parse decoded wrong bytes: % x", got.Bytes)
	}
	// Errors: malformed string, wrong type, wrong arity.
	for _, bad := range []string{"not-a-uuid", "550e8400e29b41d4a716446655440000", ""} {
		if _, err := parseFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(bad)}); err == nil {
			t.Errorf("parse(%q) should error", bad)
		}
	}
	if _, err := parseFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("parse(int) should error")
	}
	if _, err := parseFn(interpreter.BuiltinCtx{}, nil); err == nil {
		t.Error("parse() with no args should error")
	}
}

func TestIsValidFnBuiltin(t *testing.T) {
	cases := map[string]bool{
		"550e8400-e29b-41d4-a716-446655440000": true,
		"00000000-0000-0000-0000-000000000000": true,  // the nil UUID is well-formed
		"550E8400-E29B-41D4-A716-446655440000": true,  // uppercase hex accepted
		"550e8400-e29b-41d4-a716-44665544000":  false, // one short
		"550e8400e29b41d4a716446655440000":     false, // no dashes
		"zzzzzzzz-e29b-41d4-a716-446655440000": false, // non-hex
	}
	for s, want := range cases {
		got, err := isValidFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(s)})
		if err != nil {
			t.Errorf("isValid(%q): %v", s, err)
			continue
		}
		if got.Kind != interpreter.KindBool || got.Bool != want {
			t.Errorf("isValid(%q) = %v, want %v", s, got.Bool, want)
		}
	}
	if _, err := isValidFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("isValid(int) should error")
	}
}

func TestVersionFnBuiltin(t *testing.T) {
	cases := map[string]int64{
		"550e8400-e29b-41d4-a716-446655440000": 4, // byte 6 = 0x41 -> 4
		"017f22e2-79b0-7cc3-98c4-dc0c0c07398f": 7, // byte 6 = 0x7c -> 7
		"00000000-0000-0000-0000-000000000000": 0, // nil UUID -> version 0
	}
	for s, want := range cases {
		got, err := versionFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(s)})
		if err != nil {
			t.Errorf("version(%q): %v", s, err)
			continue
		}
		if got.Kind != interpreter.KindInt || got.Int != want {
			t.Errorf("version(%q) = %d, want %d", s, got.Int, want)
		}
	}
	// Errors: malformed input, wrong type.
	if _, err := versionFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("nope")}); err == nil {
		t.Error("version(malformed) should error")
	}
	if _, err := versionFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("version(int) should error")
	}
}
