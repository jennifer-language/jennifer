// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.40: an integer outside int64 keeps every digit; asInt refuses it and
// asDecimal reads it; asFloat refuses an integer the float does not hold
// exactly.
func TestBigIntegers(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	doc := func(src string) interpreter.Value {
		t.Helper()
		n, err := decodeFn([]interpreter.Value{interpreter.StringVal(src)})
		if err != nil {
			t.Fatalf("decode %s: %v", src, err)
		}
		return lib.Wrap(n)
	}
	call := func(name string, args ...interpreter.Value) (interpreter.Value, error) {
		return lib.Builtin(name)(ctx, args)
	}
	ptr := interpreter.StringVal

	v := doc(`{"id": 12345678901234567891, "neg": -98765432109876543210, "edge": 9223372036854775807}`)
	if got, _ := call("typeOf", v, ptr("/id")); got.Str != "int" {
		t.Errorf("typeOf big = %q", got.Str)
	}
	if got, err := call("asInt", v, ptr("/edge")); err != nil || got.Int != 9223372036854775807 {
		t.Errorf("asInt at the int64 edge: %v, %v", got, err)
	}
	_, err := call("asInt", v, ptr("/id"))
	if dataerr.KindOf(err) != "json" || err.Error() != "json.asInt: integer 12345678901234567891 does not fit int64; read it with json.asDecimal" {
		t.Errorf("asInt big: %v (kind %q)", err, dataerr.KindOf(err))
	}
	for p, want := range map[string]string{"/id": "12345678901234567891", "/neg": "-98765432109876543210"} {
		if d, err := call("asDecimal", v, ptr(p)); err != nil || d.Display() != want {
			t.Errorf("asDecimal %s = %v, %v", p, d, err)
		}
	}
	if enc, err := encodeFn([]interpreter.Value{v}, false); err != nil ||
		enc.Str != `{"id":12345678901234567891,"neg":-98765432109876543210,"edge":9223372036854775807}` {
		t.Errorf("encode: %q, %v", enc.Str, err)
	}
	if !doc(`[12345678901234567891]`).Equal(doc(`[12345678901234567891]`)) || doc(`[12345678901234567891]`).Equal(doc(`[12345678901234567892]`)) {
		t.Error("big int equality")
	}
	if !doc(`[18446744073709551616]`).Equal(doc(`[18446744073709551616.0]`)) {
		t.Error("2^64 as an integer equals the float 2^64")
	}

	// asFloat: exact or a data error.
	f := doc(`[9007199254740992, 9007199254740993, 18446744073709551616, 12345678901234567891, 0.1]`)
	if got, err := call("asFloat", f, ptr("/0")); err != nil || got.Float != 9007199254740992 {
		t.Errorf("asFloat 2^53: %v, %v", got, err)
	}
	if _, err := call("asFloat", f, ptr("/1")); dataerr.KindOf(err) != "json" ||
		!strings.Contains(err.Error(), "integer 9007199254740993 has no exact float value; read it with json.asDecimal") {
		t.Errorf("asFloat 2^53+1: %v", err)
	}
	if got, err := call("asFloat", f, ptr("/2")); err != nil || got.Float != 18446744073709551616 {
		t.Errorf("asFloat 2^64: %v, %v", got, err)
	}
	if _, err := call("asFloat", f, ptr("/3")); dataerr.KindOf(err) != "json" {
		t.Errorf("asFloat big: %v", err)
	}
	if _, err := call("asFloats", f); err == nil || !strings.Contains(err.Error(), "element 1 integer 9007199254740993") {
		t.Errorf("asFloats: %v", err)
	}
	if _, err := call("asInts", doc(`[1, 12345678901234567891]`)); dataerr.KindOf(err) != "json" ||
		!strings.Contains(err.Error(), "element 1 integer 12345678901234567891 does not fit int64") {
		t.Errorf("asInts: %v", err)
	}
	if d, err := call("asDecimal", f, ptr("/4")); err != nil || d.Display() != "0.1" {
		t.Errorf("asDecimal 0.1: %v, %v", d, err)
	}
	if _, err := call("asDecimal", doc(`["1"]`), ptr("/0")); err == nil || dataerr.KindOf(err) != "" {
		t.Errorf("asDecimal on a string is misuse: %v", err)
	}

	// Canonical form holds only IEEE doubles.
	n, _ := decodeFn([]interpreter.Value{interpreter.StringVal(`12345678901234567891`)})
	e := &encoder{canonical: true}
	if err := e.node(n, 0); err == nil || !strings.Contains(err.Error(), "canonical form cannot hold the integer 12345678901234567891") {
		t.Errorf("canonical big int: %v", err)
	}
}
