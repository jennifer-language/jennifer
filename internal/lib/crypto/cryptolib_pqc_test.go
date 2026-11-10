// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

import (
	"bytes"
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

func encap(pub []byte) (interpreter.Value, error) {
	return encapsulateFn(noCtx, []interpreter.Value{strArg("mlkem768"), bytesArg(pub)})
}

func decap(ct, priv []byte) (interpreter.Value, error) {
	return decapsulateFn(noCtx, []interpreter.Value{strArg("mlkem768"), bytesArg(ct), bytesArg(priv)})
}

// TestMlkemRoundTrip exercises the full KEM: a keypair, encapsulation under the
// public key, and decapsulation with the private key must recover the same
// 32-byte shared secret. Sizes are the fixed FIPS 203 ML-KEM-768 values, and
// publicKey derives the encapsulation key from the stored private seed.
func TestMlkemRoundTrip(t *testing.T) {
	pub, priv := genKeypair(t, "mlkem768")
	if len(pub) != 1184 || len(priv) != 64 {
		t.Fatalf("key sizes: pub=%d (want 1184) priv=%d (want 64)", len(pub), len(priv))
	}
	pv, err := publicKeyFn(noCtx, []interpreter.Value{strArg("mlkem768"), bytesArg(priv)})
	if err != nil || !bytes.Equal(pv.Bytes, pub) {
		t.Errorf("publicKey does not match the keypair's public key (err %v)", err)
	}

	enc, err := encap(pub)
	if err != nil {
		t.Fatalf("encapsulate: %v", err)
	}
	if enc.StructName != "Encapsulation" {
		t.Fatalf("encapsulate returned %s, want a crypto.Encapsulation", enc.Display())
	}
	ct, secret := field(enc, "ciphertext"), field(enc, "sharedSecret")
	if len(ct) != 1088 || len(secret) != 32 {
		t.Fatalf("encapsulation sizes: ct=%d (want 1088) secret=%d (want 32)", len(ct), len(secret))
	}

	dec, err := decap(ct, priv)
	if err != nil {
		t.Fatalf("decapsulate: %v", err)
	}
	if dec.Kind != interpreter.KindBytes || !bytes.Equal(dec.Bytes, secret) {
		t.Fatalf("decapsulated secret does not match the encapsulated one")
	}
}

// TestMlkemIndependentSecrets confirms two encapsulations under the same key
// produce different shared secrets (fresh randomness each time), and that a
// second keypair's private key decapsulates to a different (useless) secret -
// ML-KEM's implicit rejection, which is not an error.
func TestMlkemIndependentSecrets(t *testing.T) {
	pub, _ := genKeypair(t, "mlkem768")

	e1, _ := encap(pub)
	e2, _ := encap(pub)
	if bytes.Equal(field(e1, "sharedSecret"), field(e2, "sharedSecret")) {
		t.Error("two encapsulations should yield different secrets")
	}

	// A different key's private part: decapsulation succeeds (implicit rejection)
	// but the secret does not match the one the ciphertext was made for.
	_, otherPriv := genKeypair(t, "mlkem768")
	wrong, err := decap(field(e1, "ciphertext"), otherPriv)
	if err != nil {
		t.Fatalf("decapsulate with a valid but unrelated key should not error: %v", err)
	}
	if bytes.Equal(wrong.Bytes, field(e1, "sharedSecret")) {
		t.Error("an unrelated key must not recover the secret")
	}
}

// TestMlkemBadInputs covers the catchable error paths: wrong argument counts,
// non-bytes arguments, and malformed keys / ciphertext.
func TestMlkemBadInputs(t *testing.T) {
	pub, priv := genKeypair(t, "mlkem768")
	enc, _ := encap(pub)
	ct := field(enc, "ciphertext")
	alg := strArg("mlkem768")

	cases := []struct {
		name string
		fn   func() (interpreter.Value, error)
	}{
		{"keypair with an extra argument", func() (interpreter.Value, error) {
			return keypairFn(noCtx, []interpreter.Value{alg, bytesArg(nil)})
		}},
		{"encapsulate arg count", func() (interpreter.Value, error) {
			return encapsulateFn(noCtx, []interpreter.Value{alg})
		}},
		{"encapsulate without algorithm", func() (interpreter.Value, error) {
			return encapsulateFn(noCtx, []interpreter.Value{bytesArg(pub)})
		}},
		{"encapsulate non-bytes", func() (interpreter.Value, error) {
			return encapsulateFn(noCtx, []interpreter.Value{alg, interpreter.IntVal(1)})
		}},
		{"encapsulate short public", func() (interpreter.Value, error) {
			return encap(pub[:100])
		}},
		{"decapsulate arg count", func() (interpreter.Value, error) {
			return decapsulateFn(noCtx, []interpreter.Value{alg, bytesArg(ct)})
		}},
		{"decapsulate short private", func() (interpreter.Value, error) {
			return decap(ct, priv[:10])
		}},
		{"decapsulate short ciphertext", func() (interpreter.Value, error) {
			return decap(ct[:10], priv)
		}},
		{"decapsulate old (private, ciphertext) order", func() (interpreter.Value, error) {
			return decap(priv, ct)
		}},
		{"publicKey short private", func() (interpreter.Value, error) {
			return publicKeyFn(noCtx, []interpreter.Value{alg, bytesArg(priv[:63])})
		}},
	}
	for _, c := range cases {
		if _, err := c.fn(); err == nil {
			t.Errorf("%s: expected an error, got none", c.name)
		}
	}
}
