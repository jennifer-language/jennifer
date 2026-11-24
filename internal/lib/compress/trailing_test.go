// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package compresslib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func unpackErr(algo string, data []byte) error {
	_, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(algo), interpreter.BytesVal(data)})
	return err
}

// M29.41: zlib and deflate data is one stream, so any byte after it (junk or
// a second stream) is kind "compress"; an exact stream reads.
func TestZlibDeflateRefuseTrailingBytes(t *testing.T) {
	for _, algo := range []string{"zlib", "deflate"} {
		one := pack(t, str(algo), b("hello")).Bytes()
		two := pack(t, str(algo), b(" world")).Bytes()
		if got := unpack(t, str(algo), interpreter.BytesVal(one)); string(got.Bytes()) != "hello" {
			t.Errorf("%s: exact stream = %q", algo, got.Bytes())
		}
		for name, data := range map[string][]byte{
			"one junk byte":   cat(one, []byte{0}),
			"junk text":       cat(one, []byte("junk")),
			"a second stream": cat(one, two),
		} {
			err := unpackErr(algo, data)
			if err == nil {
				t.Errorf("%s, %s: no error", algo, name)
				continue
			}
			if k := dataerr.KindOf(err); k != "compress" {
				t.Errorf("%s, %s: kind %q, want compress (%v)", algo, name, k, err)
			}
			if !strings.Contains(err.Error(), "after the end of the "+algo+" stream") {
				t.Errorf("%s, %s: message %q", algo, name, err)
			}
		}
	}
	// The pooled decompressors (reused across calls) check it the same way.
	one := pack(t, str("zlib"), b("x")).Bytes()
	for range 3 {
		if err := unpackErr("zlib", cat(one, one)); err == nil {
			t.Fatal("pooled zlib: two streams read")
		}
	}
}

// gzip reads every member (RFC 1952) and refuses trailing bytes that are not
// one.
func TestGzipMembers(t *testing.T) {
	one := pack(t, str("gzip"), b("hello")).Bytes()
	two := pack(t, str("gzip"), b(" world")).Bytes()
	if got := unpack(t, str("gzip"), interpreter.BytesVal(cat(one, two))); string(got.Bytes()) != "hello world" {
		t.Errorf("two members = %q, want both", got.Bytes())
	}
	for name, data := range map[string][]byte{
		"junk":                   cat(one, []byte("junk")),
		"zero bytes":             cat(one, make([]byte, 16)),
		"junk after two members": cat(one, two, []byte{1, 2, 3}),
	} {
		err := unpackErr("gzip", data)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		if k := dataerr.KindOf(err); k != "compress" {
			t.Errorf("%s: kind %q, want compress (%v)", name, k, err)
		}
	}
}

func unpackOpts(maxBytes int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "UnpackOptions", []interpreter.StructField{
		{Name: "maxBytes", Value: interpreter.IntVal(maxBytes)},
	})
}

// M29.44: unpackWith's maxBytes is 0 for the default, a negative value throws
// "runtime", math.MAX_INT reads without a cap, and a reached cap is "limit".
func TestUnpackWith(t *testing.T) {
	data := strings.Repeat("a", 1000)
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		comp := pack(t, str(algo), b(data))
		call := func(o interpreter.Value) (interpreter.Value, error) {
			return unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(algo), comp, o})
		}
		for _, m := range []int64{0, 1000, math.MaxInt64} {
			got, err := call(unpackOpts(m))
			if err != nil || string(got.Bytes()) != data {
				t.Errorf("%s maxBytes %d: %v", algo, m, err)
			}
		}
		_, err := call(unpackOpts(999))
		sig, ok := err.(*interpreter.ErrorSignal)
		if !ok || errorField(sig.Value, "kind") != "limit" {
			t.Errorf("%s maxBytes 999: %T %v, want kind limit", algo, err, err)
		} else if m := errorField(sig.Value, "message"); !strings.HasPrefix(m, "compress.unpackWith: ") {
			t.Errorf("%s: message %q", algo, m)
		}
		_, err = call(unpackOpts(-1))
		if err == nil || dataerr.KindOf(err) != "" {
			t.Errorf("%s maxBytes -1: %v, want a runtime error", algo, err)
		}
		if _, err := call(interpreter.IntVal(10)); err == nil {
			t.Errorf("%s: an int for options should be refused", algo)
		}
	}
	// Trailing bytes are refused through unpackWith too.
	comp := pack(t, str("zlib"), b(data)).Bytes()
	_, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("zlib"), interpreter.BytesVal(cat(comp, comp)), unpackOpts(0)})
	if dataerr.KindOf(err) != "compress" {
		t.Errorf("unpackWith two zlib streams: %v", err)
	}
}

// Zero options are compress.unpack exactly.
func TestUnpackWithDefaultIsUnpack(t *testing.T) {
	data := strings.Repeat("ab", 5000)
	comp := pack(t, str("gzip"), b(data))
	a := unpack(t, str("gzip"), comp)
	w, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("gzip"), comp, unpackOpts(0)})
	if err != nil || string(w.Bytes()) != string(a.Bytes()) {
		t.Errorf("unpackWith{} = %v, want unpack's output", err)
	}
}
