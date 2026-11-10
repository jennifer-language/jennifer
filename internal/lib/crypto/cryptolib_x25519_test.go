// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func keypairField(v interpreter.Value, name string) []byte {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value.Bytes
		}
	}
	return nil
}

func x25519Pub(priv []byte) (interpreter.Value, error) {
	return publicKeyFn(noCtx, []interpreter.Value{strArg("x25519"), bytesArg(priv)})
}

func x25519Agree(priv, peer []byte) (interpreter.Value, error) {
	return agreeFn(noCtx, []interpreter.Value{strArg("x25519"), bytesArg(priv), bytesArg(peer)})
}

// The Diffie-Hellman test vector of RFC 7748 section 6.1: each side's public key
// comes from its private key, and both arrive at the same shared secret.
func TestX25519RFC7748Vector(t *testing.T) {
	alicePriv := unhex(t, "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a")
	alicePub := unhex(t, "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a")
	bobPriv := unhex(t, "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb")
	bobPub := unhex(t, "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f")
	shared := unhex(t, "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742")

	for _, c := range []struct{ priv, pub []byte }{{alicePriv, alicePub}, {bobPriv, bobPub}} {
		got, err := x25519Pub(c.priv)
		if err != nil || !bytes.Equal(got.Bytes, c.pub) {
			t.Errorf("publicKey(\"x25519\") = %x, %v; want %x", got.Bytes, err, c.pub)
		}
	}
	a, err := x25519Agree(alicePriv, bobPub)
	if err != nil || !bytes.Equal(a.Bytes, shared) {
		t.Errorf("alice side = %x, %v; want %x", a.Bytes, err, shared)
	}
	b, err := x25519Agree(bobPriv, alicePub)
	if err != nil || !bytes.Equal(b.Bytes, shared) {
		t.Errorf("bob side = %x, %v; want %x", b.Bytes, err, shared)
	}
}

// Two fresh keypairs agree on a secret, and the keypair's public half is the
// one publicKey derives.
func TestX25519KeypairRoundTrip(t *testing.T) {
	ka, err := keypairFn(noCtx, []interpreter.Value{strArg("x25519")})
	if err != nil {
		t.Fatal(err)
	}
	kb, err := keypairFn(noCtx, []interpreter.Value{strArg("x25519")})
	if err != nil {
		t.Fatal(err)
	}
	if ka.StructName != "Keypair" || len(keypairField(ka, "public")) != 32 || len(keypairField(ka, "private")) != 32 {
		t.Fatalf("keypair shape: %v", ka.Display())
	}
	s1, err := x25519Agree(keypairField(ka, "private"), keypairField(kb, "public"))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := x25519Agree(keypairField(kb, "private"), keypairField(ka, "public"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s1.Bytes, s2.Bytes) || len(s1.Bytes) != 32 {
		t.Errorf("secrets differ: %x vs %x", s1.Bytes, s2.Bytes)
	}
	pub, _ := x25519Pub(keypairField(ka, "private"))
	if !bytes.Equal(pub.Bytes, keypairField(ka, "public")) {
		t.Error("publicKey does not match the keypair's public key")
	}
	if bytes.Equal(keypairField(ka, "private"), keypairField(kb, "private")) {
		t.Error("two keypairs share a private key")
	}
}

// A low-order peer key (the all-zero point) would force an all-zero secret and
// is refused; wrong lengths and kinds are argument errors.
func TestX25519Rejects(t *testing.T) {
	kp, _ := keypairFn(noCtx, []interpreter.Value{strArg("x25519")})
	priv := bytesArg(keypairField(kp, "private"))
	alg := strArg("x25519")
	cases := []struct {
		args []interpreter.Value
		want string
	}{
		{[]interpreter.Value{alg, priv, bytesArg(make([]byte, 32))}, "low-order point"},
		{[]interpreter.Value{alg, priv, bytesArg(make([]byte, 31))}, "x25519 peer public key is 32 raw bytes, got 31"},
		{[]interpreter.Value{alg, bytesArg(make([]byte, 64)), bytesArg(make([]byte, 32))}, "x25519 private key is 32 raw bytes, got 64"},
		{[]interpreter.Value{alg, priv, interpreter.StringVal("k")}, "argument 3 must be bytes"},
		{[]interpreter.Value{alg, priv}, "expects 3 arguments"},
		{[]interpreter.Value{priv, priv}, "expects 3 arguments"},
		{[]interpreter.Value{priv, priv, priv}, "the algorithm (first argument) must be a string"},
	}
	for _, c := range cases {
		_, err := agreeFn(noCtx, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("agree: error %v, want %q", err, c.want)
		}
	}
	if _, err := publicKeyFn(noCtx, []interpreter.Value{alg, interpreter.IntVal(1)}); err == nil {
		t.Error("publicKey should reject a non-bytes key")
	}
	if _, err := x25519Pub(make([]byte, 31)); err == nil {
		t.Error("publicKey should reject a 31-byte x25519 key")
	}
	if _, err := keypairFn(noCtx, []interpreter.Value{alg, priv}); err == nil {
		t.Error("keypair takes only the algorithm")
	}
	if _, err := keypairFn(noCtx, nil); err == nil {
		t.Error("keypair needs the algorithm")
	}
}
