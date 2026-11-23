// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package asn1lib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// M29.21: malformed DER / BER input is kind "asn1", also where an accessor
// reads a decoded element's content and finds it malformed; a cap stays
// "limit"; misuse (wrong arguments, a builder value out of range, an accessor
// on an element of another type, content well-formed but past what the
// accessor returns) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	b := interpreter.BytesVal
	s := interpreter.StringVal
	dec := func(data []byte) interpreter.Value {
		v, err := call(decodeFn, b(data))
		if err != nil {
			t.Fatalf("decode % x: %v", data, err)
		}
		return v
	}
	errOf := func(fn interpreter.Builtin, args ...interpreter.Value) func() error {
		return func() error { _, err := call(fn, args...); return err }
	}
	deep := append(bytes.Repeat([]byte{0x30, 0x80}, limits.MaxNestingDepth+2), 0x00, 0x00)
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"truncated", errOf(decodeFn, b([]byte{0x30})), "asn1"},
		{"length past data", errOf(decodeFn, b([]byte{0x04, 0x05, 0x01})), "asn1"},
		{"trailing bytes", errOf(decodeFn, b([]byte{0x05, 0x00, 0x00})), "asn1"},
		{"indefinite primitive", errOf(decodeFn, b([]byte{0x04, 0x80, 0x00, 0x00})), "asn1"},
		{"empty input", errOf(decodeFn, b(nil)), "asn1"},
		{"depth cap", errOf(decodeFn, b(deep)), "limit"},
		{"empty integer", errOf(asIntFn, dec([]byte{0x02, 0x00})), "asn1"},
		{"boolean of two octets", errOf(asBoolFn, dec([]byte{0x01, 0x02, 0xff, 0xff})), "asn1"},
		{"UTF8String not UTF-8", errOf(asStringFn, dec([]byte{0x0c, 0x01, 0xff})), "asn1"},
		{"empty OID", errOf(asOidFn, dec([]byte{0x06, 0x00})), "asn1"},
		{"truncated OID", errOf(asOidFn, dec([]byte{0x06, 0x01, 0x81})), "asn1"},
		{"not bytes", errOf(decodeFn, s("x")), ""},
		{"integer wider than int", errOf(asIntFn, dec([]byte{0x02, 0x09, 0x01, 0, 0, 0, 0, 0, 0, 0, 0})), ""},
		{"OCTET STRING read as text", errOf(asStringFn, dec([]byte{0x04, 0x01, 0xff})), ""},
		{"accessor on another type", errOf(asIntFn, dec([]byte{0x05, 0x00})), ""},
		{"bad OID text", errOf(oidFn, s("1")), ""},
		{"unknown class", errOf(taggedFn, s("nope"), interpreter.IntVal(0), dec([]byte{0x05, 0x00})), ""},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
