// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package cryptolib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// field pulls a named bytes field out of a returned struct Value.
func field(v interpreter.Value, name string) []byte {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value.Bytes
		}
	}
	return nil
}

// TestMlkemRoundTrip exercises the full KEM: a keypair, encapsulation under the
// public key, and decapsulation with the private key must recover the same
// 32-byte shared secret. Sizes are the fixed FIPS 203 ML-KEM-768 values.
func TestMlkemRoundTrip(t *testing.T) {
	kp, err := mlkemKeypairFn(noCtx, nil)
	if err != nil {
		t.Fatalf("mlkemKeypair: %v", err)
	}
	pub, priv := field(kp, "public"), field(kp, "private")
	if len(pub) != 1184 || len(priv) != 64 {
		t.Fatalf("key sizes: pub=%d (want 1184) priv=%d (want 64)", len(pub), len(priv))
	}

	enc, err := mlkemEncapsulateFn(noCtx, []interpreter.Value{bytesArg(pub)})
	if err != nil {
		t.Fatalf("mlkemEncapsulate: %v", err)
	}
	ct, secret := field(enc, "ciphertext"), field(enc, "sharedSecret")
	if len(ct) != 1088 || len(secret) != 32 {
		t.Fatalf("encapsulation sizes: ct=%d (want 1088) secret=%d (want 32)", len(ct), len(secret))
	}

	dec, err := mlkemDecapsulateFn(noCtx, []interpreter.Value{bytesArg(priv), bytesArg(ct)})
	if err != nil {
		t.Fatalf("mlkemDecapsulate: %v", err)
	}
	if dec.Kind != interpreter.KindBytes || string(dec.Bytes) != string(secret) {
		t.Fatalf("decapsulated secret does not match the encapsulated one")
	}
}

// TestMlkemIndependentSecrets confirms two encapsulations under the same key
// produce different shared secrets (fresh randomness each time), and that a
// second keypair's private key decapsulates to a different (useless) secret -
// ML-KEM's implicit rejection, which is not an error.
func TestMlkemIndependentSecrets(t *testing.T) {
	kp, _ := mlkemKeypairFn(noCtx, nil)
	pub := field(kp, "public")

	e1, _ := mlkemEncapsulateFn(noCtx, []interpreter.Value{bytesArg(pub)})
	e2, _ := mlkemEncapsulateFn(noCtx, []interpreter.Value{bytesArg(pub)})
	if string(field(e1, "sharedSecret")) == string(field(e2, "sharedSecret")) {
		t.Error("two encapsulations should yield different secrets")
	}

	// A different key's private part: decapsulation succeeds (implicit rejection)
	// but the secret does not match the one the ciphertext was made for.
	other, _ := mlkemKeypairFn(noCtx, nil)
	wrong, err := mlkemDecapsulateFn(noCtx, []interpreter.Value{bytesArg(field(other, "private")), bytesArg(field(e1, "ciphertext"))})
	if err != nil {
		t.Fatalf("decapsulate with a valid but unrelated key should not error: %v", err)
	}
	if string(wrong.Bytes) == string(field(e1, "sharedSecret")) {
		t.Error("an unrelated key must not recover the secret")
	}
}

// TestMlkemBadInputs covers the catchable error paths: wrong argument counts,
// non-bytes arguments, and malformed keys / ciphertext.
func TestMlkemBadInputs(t *testing.T) {
	kp, _ := mlkemKeypairFn(noCtx, nil)
	pub, priv := field(kp, "public"), field(kp, "private")
	enc, _ := mlkemEncapsulateFn(noCtx, []interpreter.Value{bytesArg(pub)})
	ct := field(enc, "ciphertext")

	cases := []struct {
		name string
		fn   func() (interpreter.Value, error)
	}{
		{"keypair with an argument", func() (interpreter.Value, error) {
			return mlkemKeypairFn(noCtx, []interpreter.Value{bytesArg(nil)})
		}},
		{"encapsulate arg count", func() (interpreter.Value, error) {
			return mlkemEncapsulateFn(noCtx, nil)
		}},
		{"encapsulate non-bytes", func() (interpreter.Value, error) {
			return mlkemEncapsulateFn(noCtx, []interpreter.Value{interpreter.IntVal(1)})
		}},
		{"encapsulate short public", func() (interpreter.Value, error) {
			return mlkemEncapsulateFn(noCtx, []interpreter.Value{bytesArg(pub[:100])})
		}},
		{"decapsulate arg count", func() (interpreter.Value, error) {
			return mlkemDecapsulateFn(noCtx, []interpreter.Value{bytesArg(priv)})
		}},
		{"decapsulate short private", func() (interpreter.Value, error) {
			return mlkemDecapsulateFn(noCtx, []interpreter.Value{bytesArg(priv[:10]), bytesArg(ct)})
		}},
		{"decapsulate short ciphertext", func() (interpreter.Value, error) {
			return mlkemDecapsulateFn(noCtx, []interpreter.Value{bytesArg(priv), bytesArg(ct[:10])})
		}},
	}
	for _, c := range cases {
		if _, err := c.fn(); err == nil {
			t.Errorf("%s: expected an error, got none", c.name)
		}
	}
}
