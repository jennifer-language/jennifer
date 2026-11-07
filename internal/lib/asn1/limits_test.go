// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package asn1lib

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// allocated reports the bytes f allocates.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// sequenceOfNulls is a definite-length SEQUENCE of n NULL elements (2 bytes each).
func sequenceOfNulls(n int) []byte {
	body := bytes.Repeat([]byte{0x05, 0x00}, n)
	l := len(body)
	return append([]byte{0x30, 0x84, byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}, body...)
}

// TestDecodeAmplificationBounded pins the per-element cost of a decoded tree: a
// 200 KB input of 100k NULLs must not amplify to KBs per element (the struct-of-
// Values tree took ~2.6 KB each).
func TestDecodeAmplificationBounded(t *testing.T) {
	const n = 100_000
	data := sequenceOfNulls(n)
	var v interpreter.Value
	got := allocated(func() {
		var err error
		v, err = call(decodeFn, interpreter.BytesVal(data))
		if err != nil {
			t.Fatal(err)
		}
	})
	if per := got / n; per > 300 {
		t.Errorf("decode allocated %d bytes per element, want <= 300", per)
	}
	runtime.KeepAlive(v)
}

// TestEncodeDeepLinear pins that encode copies each byte once: a 1 MiB OCTET
// STRING under 500 SEQUENCEs must not allocate a copy per level.
func TestEncodeDeepLinear(t *testing.T) {
	v := mustBuild(t, octetStringFn, interpreter.BytesVal(make([]byte, 1<<20)))
	for i := 0; i < 500; i++ {
		v = mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{v}))
	}
	var out []byte
	got := allocated(func() { out = der(t, v) })
	if got > 4<<20 {
		t.Errorf("encode allocated %d bytes for a %d-byte result, want < 4 MiB", got, len(out))
	}
	if back := der(t, decode(t, out)); !bytes.Equal(back, out) {
		t.Error("deep tree does not round-trip")
	}
}

// TestEncodeSharedDAGCapped pins that a value doubling a shared child is
// measured, not expanded: 64 doublings would be 2^64 elements, so encode must
// fail fast with a size error, not hang or panic.
func TestEncodeSharedDAGCapped(t *testing.T) {
	v := mustBuild(t, nullFn)
	for i := 0; i < 64; i++ {
		v = mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{v, v}))
	}
	_, err := call(encodeFn, v)
	if err == nil || !(strings.Contains(err.Error(), "limit") || strings.Contains(err.Error(), "overflow")) {
		t.Fatalf("err = %v, want a size error", err)
	}
}

// TestSetEncodesSorted pins DER SET ordering (X.690 11.6): children appear in
// ascending order of their encodings whatever the build order.
func TestSetEncodesSorted(t *testing.T) {
	set := mustBuild(t, setFn, interpreter.ListVal(childListType, []interpreter.Value{
		mustBuild(t, integerFn, interpreter.IntVal(3)),
		mustBuild(t, octetStringFn, interpreter.BytesVal([]byte{0xaa})),
		mustBuild(t, integerFn, interpreter.IntVal(1)),
	}))
	want := []byte{0x31, 0x09, 0x02, 0x01, 0x01, 0x02, 0x01, 0x03, 0x04, 0x01, 0xaa}
	if got := der(t, set); !bytes.Equal(got, want) {
		t.Errorf("set encodes % x, want % x", got, want)
	}
	// A SEQUENCE keeps its order.
	seq := mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{
		mustBuild(t, integerFn, interpreter.IntVal(3)),
		mustBuild(t, integerFn, interpreter.IntVal(1)),
	}))
	if got := der(t, seq); !bytes.Equal(got, []byte{0x30, 0x06, 0x02, 0x01, 0x03, 0x02, 0x01, 0x01}) {
		t.Errorf("sequence reordered: % x", got)
	}
}

// TestRestrictedStringCharsets pins that printableString and ia5String reject
// characters outside their sets instead of emitting invalid DER.
func TestRestrictedStringCharsets(t *testing.T) {
	if _, err := call(printableStringFn, interpreter.StringVal("Acme Ltd. (DE) 1+1=2?")); err != nil {
		t.Errorf("valid PrintableString rejected: %v", err)
	}
	for _, s := range []string{"a@b", "x_y", "Müller", "a*b"} {
		if _, err := call(printableStringFn, interpreter.StringVal(s)); err == nil {
			t.Errorf("printableString(%q) accepted", s)
		}
	}
	if _, err := call(ia5StringFn, interpreter.StringVal("user@example.com\t~")); err != nil {
		t.Errorf("valid IA5String rejected: %v", err)
	}
	if _, err := call(ia5StringFn, interpreter.StringVal("bücher@example.com")); err == nil {
		t.Error("ia5String with non-ASCII accepted")
	}
}

// TestValueEqualityAndZero pins the language-level behaviour of the handle:
// equal trees compare equal, different ones do not, and the zero handle is an
// "empty" error in accessors.
func TestValueEqualityAndZero(t *testing.T) {
	a := decode(t, []byte{0x30, 0x03, 0x02, 0x01, 0x05})
	b := mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{
		mustBuild(t, integerFn, interpreter.IntVal(5)),
	}))
	c := decode(t, []byte{0x30, 0x03, 0x02, 0x01, 0x06})
	if !a.Equal(b) {
		t.Error("decoded and built equal trees compare unequal")
	}
	if a.Equal(c) {
		t.Error("different trees compare equal")
	}
	if got := a.Display(); got != "sequence { integer 5 }" {
		t.Errorf("display = %q", got)
	}
	zero := interpreter.ObjectVal(LibraryName, "Value", interpreter.Null())
	if _, err := call(typeOfFn, zero); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("zero handle: err = %v, want empty", err)
	}
}
