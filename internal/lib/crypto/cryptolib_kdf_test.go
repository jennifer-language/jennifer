// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// kdfParams builds a crypto.KdfParams as a literal does: every field present,
// the ones not named zero.
func kdfParams(info []byte, ints map[string]int64) interpreter.Value {
	var fields []interpreter.StructField
	for _, f := range kdfParamFields {
		if f.Name == "info" {
			fields = append(fields, interpreter.StructField{Name: "info", Value: interpreter.BytesVal(info)})
			continue
		}
		fields = append(fields, interpreter.StructField{Name: f.Name, Value: interpreter.IntVal(ints[f.Name])})
	}
	return interpreter.NamespacedStructVal(LibraryName, "KdfParams", fields)
}

func derive(algo string, secret, salt []byte, length int64, p interpreter.Value) (interpreter.Value, error) {
	return deriveKeyFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.StringVal(algo), interpreter.BytesVal(secret), interpreter.BytesVal(salt), interpreter.IntVal(length), p,
	})
}

// deriveKey runs the same derivation as the function it replaces.
func TestDeriveKeyMatchesOldFunctions(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	b := func(s string) interpreter.Value { return interpreter.BytesVal([]byte(s)) }
	i := interpreter.IntVal
	salt := "0123456789abcdef"
	cases := []struct {
		algo   string
		params interpreter.Value
		old    func() (interpreter.Value, error)
	}{
		{"hkdf-sha256", kdfParams([]byte("ctx"), nil), func() (interpreter.Value, error) {
			return hkdfFn(ctx, []interpreter.Value{interpreter.StringVal("sha256"), b("secret"), b(salt), b("ctx"), i(42)})
		}},
		{"hkdf-sha512", kdfParams(nil, nil), func() (interpreter.Value, error) {
			return hkdfFn(ctx, []interpreter.Value{interpreter.StringVal("sha512"), b("secret"), b(salt), b(""), i(42)})
		}},
		{"pbkdf2-sha1", kdfParams(nil, map[string]int64{"iterations": 1000}), func() (interpreter.Value, error) {
			return pbkdf2Fn(ctx, []interpreter.Value{interpreter.StringVal("sha1"), b("secret"), b(salt), i(1000), i(42)})
		}},
		{"argon2id", kdfParams(nil, map[string]int64{"time": 1, "memoryKiB": 64, "threads": 1}), func() (interpreter.Value, error) {
			return argon2idFn(ctx, []interpreter.Value{b("secret"), b(salt), i(1), i(64), i(1), i(42)})
		}},
		{"scrypt", kdfParams(nil, map[string]int64{"n": 16, "r": 1, "p": 1}), func() (interpreter.Value, error) {
			return scryptFn(ctx, []interpreter.Value{b("secret"), b(salt), i(16), i(1), i(1), i(42)})
		}},
	}
	for _, c := range cases {
		got, err := derive(c.algo, []byte("secret"), []byte(salt), 42, c.params)
		if err != nil {
			t.Fatalf("%s: %v", c.algo, err)
		}
		want, err := c.old()
		if err != nil {
			t.Fatalf("%s old: %v", c.algo, err)
		}
		if !bytes.Equal(got.Bytes, want.Bytes) || len(got.Bytes) != 42 {
			t.Errorf("%s: deriveKey differs from the old function", c.algo)
		}
	}
}

func TestDeriveKeyParamsAreStrict(t *testing.T) {
	salt := []byte("0123456789abcdef")
	cases := []struct {
		algo   string
		params interpreter.Value
		want   string
	}{
		{"pbkdf2-sha256", kdfParams(nil, nil), "crypto.deriveKey (pbkdf2-sha256): needs KdfParams.iterations (there is no default)"},
		{"argon2id", kdfParams(nil, map[string]int64{"time": 1, "threads": 1}), "needs KdfParams.memoryKiB"},
		{"scrypt", kdfParams(nil, map[string]int64{"n": 16, "r": 1}), "needs KdfParams.p"},
		{"hkdf-sha256", kdfParams(nil, map[string]int64{"iterations": 5}), "crypto.deriveKey (hkdf-sha256): does not take KdfParams.iterations"},
		{"pbkdf2-sha256", kdfParams([]byte("x"), map[string]int64{"iterations": 5}), "does not take KdfParams.info"},
		{"argon2id", kdfParams(nil, map[string]int64{"time": 1, "memoryKiB": 64, "threads": 1, "n": 16}), "does not take KdfParams.n"},
		{"pbkdf2", kdfParams(nil, nil), `unknown algorithm "pbkdf2"; known: "argon2id", "hkdf-sha1"`},
		{"pbkdf2-sha256", kdfParams(nil, map[string]int64{"iterations": -1}), "crypto.deriveKey (pbkdf2-sha256): iterations must be > 0"},
	}
	for _, c := range cases {
		_, err := derive(c.algo, []byte("pw"), salt, 32, c.params)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.algo, err, c.want)
		}
	}
	_, err := derive("hkdf-sha256", []byte("pw"), salt, 32, interpreter.IntVal(1))
	if err == nil || !strings.Contains(err.Error(), "params must be a crypto.KdfParams") {
		t.Errorf("non-struct params: %v", err)
	}
}

// The caps of the functions it replaces keep their kind.
func TestDeriveKeyCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	_, err := derive("pbkdf2-sha256", []byte("pw"), []byte("0123456789abcdef"), maxKeyLen+1, kdfParams(nil, map[string]int64{"iterations": 1}))
	if !errors.As(err, &ce) || !strings.HasPrefix(err.Error(), "crypto.deriveKey (pbkdf2-sha256): ") {
		t.Errorf("pbkdf2 keyLen cap: %v", err)
	}
	_, err = derive("argon2id", []byte("pw"), []byte("0123456789abcdef"), 32, kdfParams(nil, map[string]int64{"time": 1, "memoryKiB": maxArgonMemory + 1, "threads": 1}))
	if !errors.As(err, &ce) {
		t.Errorf("argon2id memory cap: %v", err)
	}
	// HKDF's own maximum is the algorithm's bound, not a cap.
	_, err = derive("hkdf-sha256", []byte("pw"), nil, 8161, kdfParams(nil, nil))
	if err == nil || errors.As(err, &ce) {
		t.Errorf("hkdf maximum: %v", err)
	}
}
