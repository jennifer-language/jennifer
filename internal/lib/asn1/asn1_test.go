// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package asn1lib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func call(fn interpreter.Builtin, args ...interpreter.Value) (interpreter.Value, error) {
	return fn(interpreter.BuiltinCtx{}, args)
}

// mustBuild runs a constructor and fails the test on error.
func mustBuild(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := call(fn, args...)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	return v
}

func der(t *testing.T, v interpreter.Value) []byte {
	t.Helper()
	out, err := call(encodeFn, v)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if out.Kind != interpreter.KindBytes {
		t.Fatalf("encode returned %s, want bytes", out.Kind)
	}
	return out.Bytes
}

func decode(t *testing.T, data []byte) interpreter.Value {
	t.Helper()
	v, err := call(decodeFn, interpreter.BytesVal(data))
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	return v
}

// TestDEREncoding pins the exact bytes of a known structure against a
// hand-verified reference: SEQUENCE { INTEGER 42, OCTET STRING "hi", NULL }.
func TestDEREncoding(t *testing.T) {
	seq := mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{
		mustBuild(t, integerFn, interpreter.IntVal(42)),
		mustBuild(t, octetStringFn, interpreter.BytesVal([]byte("hi"))),
		mustBuild(t, nullFn),
	}))
	got := der(t, seq)
	want := []byte{0x30, 0x09, 0x02, 0x01, 0x2a, 0x04, 0x02, 'h', 'i', 0x05, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("DER = % x, want % x", got, want)
	}
}

// TestIntegerCodec round-trips a spread of int64 values, including the
// two's-complement boundaries where a minimal encoding is subtle.
func TestIntegerCodec(t *testing.T) {
	for _, n := range []int64{0, 1, -1, 127, 128, 255, 256, -128, -129, -256, 32767, -32768, 1 << 40, -(1 << 40), 1<<63 - 1, -(1 << 63)} {
		v := mustBuild(t, integerFn, interpreter.IntVal(n))
		back := decode(t, der(t, v))
		got, err := call(asIntFn, back)
		if err != nil {
			t.Fatalf("asInt(%d): %v", n, err)
		}
		if got.Int != n {
			t.Errorf("integer round-trip: got %d, want %d", got.Int, n)
		}
	}
}

// TestOIDCodec round-trips a spread of OIDs, including large arcs (multi-byte
// base-128) and the first-arc / second-arc combining rule.
func TestOIDCodec(t *testing.T) {
	for _, oid := range []string{"1.2.840.113549.1.1.11", "0.0", "2.100.3", "1.3.6.1.4.1.311", "2.999999"} {
		v, err := call(oidFn, interpreter.StringVal(oid))
		if err != nil {
			t.Fatalf("oid(%q): %v", oid, err)
		}
		back := decode(t, der(t, v))
		got, err := call(asOidFn, back)
		if err != nil {
			t.Fatalf("asOid(%q): %v", oid, err)
		}
		if got.Str != oid {
			t.Errorf("OID round-trip: got %q, want %q", got.Str, oid)
		}
	}
}

// TestRoundTripStable checks that decode -> encode reproduces the original DER
// for a nested structure with several types.
func TestRoundTripStable(t *testing.T) {
	seq := mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{
		mustBuild(t, booleanFn, interpreter.BoolVal(true)),
		mustBuild(t, enumeratedFn, interpreter.IntVal(2)),
		mustBuild(t, utf8StringFn, interpreter.StringVal("héllo")),
		mustBuild(t, oidFn, interpreter.StringVal("1.2.840.113549")),
		mustBuild(t, taggedFn, interpreter.StringVal("context"), interpreter.IntVal(0),
			mustBuild(t, integerFn, interpreter.IntVal(99))),
	}))
	original := der(t, seq)
	reencoded := der(t, decode(t, original))
	if !bytes.Equal(original, reencoded) {
		t.Fatalf("decode/encode not stable:\n orig % x\n  got % x", original, reencoded)
	}
}

// TestImplicitRetag pins IMPLICIT tagging: retag replaces the outer tag but keeps
// the content, so an integer retagged [context 1] still reads back as that int.
func TestImplicitRetag(t *testing.T) {
	inner := mustBuild(t, integerFn, interpreter.IntVal(500))
	retagged, err := call(retagFn, interpreter.StringVal("context"), interpreter.IntVal(1), inner)
	if err != nil {
		t.Fatalf("retag: %v", err)
	}
	back := decode(t, der(t, retagged))
	cls, _ := call(tagClassFn, back)
	num, _ := call(tagNumberFn, back)
	if cls.Str != "context" || num.Int != 1 {
		t.Fatalf("retag class/num = %s/%d, want context/1", cls.Str, num.Int)
	}
	// The content is still the integer's, so a matching-shape read recovers it.
	if !bytes.Equal(elemContent(mustUnwrap(t, retagged)), encodeIntContent(500)) {
		t.Fatalf("retag did not preserve content")
	}
}

// TestDecodeBER decodes constructions Go's DER-only asn1 would reject: an
// indefinite-length SEQUENCE and a high-tag-number identifier.
func TestDecodeBER(t *testing.T) {
	// SEQUENCE (indefinite) { INTEGER 5 } EOC
	indef := []byte{0x30, 0x80, 0x02, 0x01, 0x05, 0x00, 0x00}
	v := decode(t, indef)
	length, err := call(lengthFn, v)
	if err != nil || length.Int != 1 {
		t.Fatalf("indefinite length: len = %v, %v; want 1", length, err)
	}
	got, err := call(asIntFn, v, interpreter.StringVal("/0"))
	if err != nil || got.Int != 5 {
		t.Fatalf("indefinite child: %v, %v; want 5", got, err)
	}

	// A high-tag-number identifier: [APPLICATION 31] primitive, empty.
	// 0x5f (class=01 app, primitive, tag=0x1f high form) 0x1f (tag=31) 0x00 (len 0)
	hi := []byte{0x5f, 0x1f, 0x00}
	hv := decode(t, hi)
	cls, _ := call(tagClassFn, hv)
	num, _ := call(tagNumberFn, hv)
	if cls.Str != "application" || num.Int != 31 {
		t.Fatalf("high-tag: class/num = %s/%d, want application/31", cls.Str, num.Int)
	}
}

// TestDecodeErrors pins the strict, catchable rejection of malformed input.
func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"truncated", []byte{0x02}},
		{"length exceeds data", []byte{0x04, 0x05, 0x01, 0x02}},
		{"trailing bytes", []byte{0x05, 0x00, 0xff}},
		{"indefinite on primitive", []byte{0x04, 0x80, 0x00, 0x00}},
		{"unterminated indefinite", []byte{0x30, 0x80, 0x05, 0x00}},
		{"missing length", []byte{0x02}},
	}
	for _, c := range cases {
		if _, err := call(decodeFn, interpreter.BytesVal(c.data)); err == nil {
			t.Errorf("%s: expected a decode error, got nil", c.name)
		}
	}
}

// TestAccessorTypeMismatch confirms a leaf extractor rejects the wrong element
// type rather than returning garbage.
func TestAccessorTypeMismatch(t *testing.T) {
	octet := mustBuild(t, octetStringFn, interpreter.BytesVal([]byte{1, 2, 3}))
	if _, err := call(asIntFn, octet); err == nil {
		t.Error("asInt on an octet string: expected an error")
	}
	if _, err := call(asOidFn, octet); err == nil {
		t.Error("asOid on an octet string: expected an error")
	}
	num := mustBuild(t, integerFn, interpreter.IntVal(1))
	if _, err := call(asStringFn, num); err == nil {
		t.Error("asString on an integer: expected an error")
	}
	// length on a primitive is an error (it is not a container).
	if _, err := call(lengthFn, num); err == nil {
		t.Error("length on a primitive: expected an error")
	}
	// asString rejects non-UTF-8 octet content.
	bad := mustBuild(t, octetStringFn, interpreter.BytesVal([]byte{0xff, 0xfe}))
	if _, err := call(asStringFn, bad); err == nil {
		t.Error("asString on non-UTF-8 octets: expected an error")
	}
}

// TestNestingDepthGuard rejects a pathologically deep indefinite nesting rather
// than overflowing the Go stack.
func TestNestingDepthGuard(t *testing.T) {
	// N nested indefinite-length SEQUENCEs: N*(0x30 0x80) then N*(0x00 0x00).
	n := 2000
	var data []byte
	for i := 0; i < n; i++ {
		data = append(data, 0x30, 0x80)
	}
	for i := 0; i < n; i++ {
		data = append(data, 0x00, 0x00)
	}
	if _, err := call(decodeFn, interpreter.BytesVal(data)); err == nil {
		t.Error("deeply nested indefinite input: expected a depth-limit error")
	}
}

// TestNodeBudgetGuard trips the decode-bomb guard cheaply by lowering the budget
// (a real trip would need ~a million elements).
func TestNodeBudgetGuard(t *testing.T) {
	saved := maxNodes
	maxNodes = 4
	defer func() { maxNodes = saved }()
	// SEQUENCE { NULL, NULL, NULL, NULL } is 5 nodes (root + 4), over the budget.
	data := []byte{0x30, 0x08, 0x05, 0x00, 0x05, 0x00, 0x05, 0x00, 0x05, 0x00}
	if _, err := call(decodeFn, interpreter.BytesVal(data)); err == nil {
		t.Error("input over the node budget: expected a decode-bomb error")
	}
}

// TestLongFormLength pins that an oversized long-form length is a clean,
// catchable error rather than an integer overflow / misparse - the accumulation
// is width-independent, so an 8-octet length that would wrap a 32-bit int is
// still rejected against the buffer size.
func TestLongFormLength(t *testing.T) {
	cases := [][]byte{
		{0x04, 0x84, 0xff, 0xff, 0xff, 0xff},                         // 4-octet length 0xFFFFFFFF, no content
		{0x04, 0x88, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // 8-octet length 2^64-1
	}
	for _, data := range cases {
		if _, err := call(decodeFn, interpreter.BytesVal(data)); err == nil {
			t.Errorf("oversized long-form length % x: expected a decode error, got nil", data)
		}
	}
	// A valid long-form length still works: a 200-byte octet string needs the
	// 0x81 0xC8 two-byte form.
	body := make([]byte, 200)
	valid := append([]byte{0x04, 0x81, 0xC8}, body...)
	v, err := call(decodeFn, interpreter.BytesVal(valid))
	if err != nil {
		t.Fatalf("valid long-form length: %v", err)
	}
	b, err := call(asBytesFn, v)
	if err != nil || len(b.Bytes) != 200 {
		t.Fatalf("long-form octet string: got %v (%d bytes), %v; want 200 bytes", b.Kind, len(b.Bytes), err)
	}
}

// TestTagNumberBounds pins that a builder rejects a tag number decode could not
// read back, keeping build and decode symmetric.
func TestTagNumberBounds(t *testing.T) {
	inner := mustBuild(t, integerFn, interpreter.IntVal(1))
	// At the cap: accepted and round-trips.
	if _, err := call(taggedFn, interpreter.StringVal("context"), interpreter.IntVal(maxTagNumber), inner); err != nil {
		t.Errorf("tagged at the cap: unexpected error %v", err)
	}
	// Over the cap: rejected by both builders.
	over := interpreter.IntVal(maxTagNumber + 1)
	if _, err := call(taggedFn, interpreter.StringVal("context"), over, inner); err == nil {
		t.Error("tagged past the cap: expected an error, got nil")
	}
	if _, err := call(retagFn, interpreter.StringVal("context"), over, inner); err == nil {
		t.Error("retag past the cap: expected an error, got nil")
	}
	// A negative tag is rejected too.
	if _, err := call(taggedFn, interpreter.StringVal("context"), interpreter.IntVal(-1), inner); err == nil {
		t.Error("tagged with a negative tag: expected an error, got nil")
	}
}

// TestHighTagDecodeCap confirms decode enforces the same tag cap as the
// builders: a high-tag-number identifier encoding a tag past maxTagNumber is
// rejected (not accepted one continuation octet over, nor wrapped on a 32-bit
// int). The bytes below encode tag 2^25, which is > maxTagNumber (2^24).
func TestHighTagDecodeCap(t *testing.T) {
	overCap := []byte{0x1f, 0x90, 0x80, 0x80, 0x00, 0x00} // id (tag 2^25) + length 0
	if _, err := call(decodeFn, interpreter.BytesVal(overCap)); err == nil {
		t.Error("high-tag identifier past the cap: expected a decode error, got nil")
	}
	// A tag exactly at the cap round-trips build -> encode -> decode symmetrically.
	atCap := mustBuild(t, taggedFn, interpreter.StringVal("application"),
		interpreter.IntVal(maxTagNumber), mustBuild(t, integerFn, interpreter.IntVal(1)))
	back := decode(t, der(t, atCap))
	num, err := call(tagNumberFn, back)
	if err != nil || num.Int != maxTagNumber {
		t.Fatalf("tag at cap round-trip: got %v, %v; want %d", num, err, maxTagNumber)
	}
}

// TestOIDArcCap confirms an out-of-range OID arc is rejected at build time and a
// large-but-valid arc round-trips - so a value the library builds is one it can
// decode back.
func TestOIDArcCap(t *testing.T) {
	// A big-but-valid arc (2^32) round-trips.
	big := "1.2.4294967296"
	v, err := call(oidFn, interpreter.StringVal(big))
	if err != nil {
		t.Fatalf("oid(%q): %v", big, err)
	}
	got, err := call(asOidFn, decode(t, der(t, v)))
	if err != nil || got.Str != big {
		t.Fatalf("big-arc OID round-trip: got %v, %v; want %q", got, err, big)
	}
	// An arc past the cap is rejected rather than encoded to something decode
	// would refuse. 2^57 is one past maxOIDArc (2^57 - 1).
	if _, err := call(oidFn, interpreter.StringVal("1.2.144115188075855872")); err == nil {
		t.Error("OID with an over-cap arc: expected an error, got nil")
	}
}

// mustUnwrap pulls the inner element out of an asn1.Value for a white-box check.
func mustUnwrap(t *testing.T, v interpreter.Value) interpreter.Value {
	t.Helper()
	inner, err := takeElem("test", v)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	return inner
}
