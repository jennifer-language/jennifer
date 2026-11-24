// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package compresslib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: input unpack cannot decompress is kind "compress", the output cap
// stays "limit", and misuse (wrong arguments, an unknown algorithm or level)
// stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	data := strings.Repeat("jennifer ", 200)
	packed := func(algo string) []byte { return pack(t, str(algo), b(data)).Bytes() }
	flip := func(p []byte, i int) []byte {
		q := append([]byte(nil), p...)
		if i < 0 {
			i += len(q)
		}
		q[i] ^= 0xff
		return q
	}
	unpackOf := func(algo string, in []byte, extra ...interpreter.Value) func() error {
		return func() error {
			args := append([]interpreter.Value{str(algo), interpreter.BytesVal(in)}, extra...)
			_, err := unpackFn(interpreter.BuiltinCtx{}, args)
			return err
		}
	}
	gz, zl, df := packed("gzip"), packed("zlib"), packed("deflate")
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"gzip not gzip", unpackOf("gzip", []byte("not gzip data")), "compress"},
		{"gzip empty", unpackOf("gzip", nil), "compress"},
		{"gzip short header", unpackOf("gzip", gz[:5]), "compress"},
		{"gzip truncated", unpackOf("gzip", gz[:len(gz)-6]), "compress"},
		{"gzip crc", unpackOf("gzip", flip(gz, -5)), "compress"},
		{"zlib bad header", unpackOf("zlib", []byte{0x00, 0x00, 0x00}), "compress"},
		{"zlib checksum", unpackOf("zlib", flip(zl, -1)), "compress"},
		{"zlib truncated", unpackOf("zlib", zl[:len(zl)/2]), "compress"},
		{"deflate corrupt", unpackOf("deflate", []byte{0xff, 0xff, 0xff, 0xff}), "compress"},
		{"deflate truncated", unpackOf("deflate", df[:len(df)/2]), "compress"},
		{"unknown algorithm", unpackOf("lzma", gz), ""},
		{"maxBytes zero", unpackOf("gzip", gz, interpreter.IntVal(0)), ""},
		{"unpack not bytes", func() error {
			_, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("gzip"), str("x")})
			return err
		}, ""},
		{"pack unknown level", func() error {
			_, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("gzip"), b("x"), str("max")})
			return err
		}, ""},
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

	// The output cap is raised as kind "limit" directly.
	err := unpackOf("gzip", gz, interpreter.IntVal(10))()
	sig, ok := err.(*interpreter.ErrorSignal)
	if !ok || errorField(sig.Value, "kind") != "limit" {
		t.Errorf("over maxBytes: %T %v, want kind limit", err, err)
	}
}
