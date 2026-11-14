// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

// Tests for the x/crypto-backed parts (M28.58). The vectors come from other
// implementations, so they show interoperability, not agreement with the Go
// packages the code wraps: libsodium through PyNaCl (ChaCha20-Poly1305,
// XChaCha20-Poly1305, the sealed box), argon2-cffi, the Python bcrypt
// package, and Python's hashlib.scrypt.

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func seq(from, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(from + i)
	}
	return b
}

func TestChachaVectorsFromLibsodium(t *testing.T) {
	key, aad, pt := seq(0, 32), []byte("header"), []byte("Jennifer meets libsodium")
	cases := []struct {
		algo  string
		nonce []byte
		box   string
	}{
		{"chacha20-poly1305", seq(100, 12), "7e74d5a4f1c31320dbb3701a86faf8ff19a4e15bfb399ec06c115e9ef74879db5c14aff7205c023f"},
		{"xchacha20-poly1305", seq(200, 24), "330d46ea2b14191478c613d334b01f0f71b6b688c3f25fdc9ea524a330f6add80444af41a741e271"},
	}
	for _, c := range cases {
		args := []interpreter.Value{strArg(c.algo), bytesArg(pt), bytesArg(key), bytesArg(c.nonce), bytesArg(aad)}
		got, err := encryptFn(noCtx, args)
		if err != nil {
			t.Fatalf("%s encrypt: %v", c.algo, err)
		}
		want := unhex(t, c.box)
		if !bytes.Equal(got.Bytes, want) {
			t.Errorf("%s: got %x, want %s", c.algo, got.Bytes, c.box)
		}
		args[1] = bytesArg(want)
		back, err := decryptFn(noCtx, args)
		if err != nil || !bytes.Equal(back.Bytes, pt) {
			t.Errorf("%s decrypt: %q, %v", c.algo, back.Bytes, err)
		}
		tampered := append([]byte{}, want...)
		tampered[0] ^= 1
		args[1] = bytesArg(tampered)
		_, err = decryptFn(noCtx, args)
		wantErr(t, c.algo+" tampered", err, "authentication failed")
	}
}

func TestChachaKeyAndNonceLengths(t *testing.T) {
	for _, c := range []struct {
		algo       string
		key, nonce int
		want       string
	}{
		{"chacha20-poly1305", 16, 12, "key must be 32 bytes"},
		{"chacha20-poly1305", 32, 24, "nonce must be 12 bytes"},
		{"xchacha20-poly1305", 32, 12, "nonce must be 24 bytes"},
	} {
		_, err := encryptFn(noCtx, []interpreter.Value{strArg(c.algo), bytesArg(nil), bytesArg(make([]byte, c.key)), bytesArg(make([]byte, c.nonce)), bytesArg(nil)})
		wantErr(t, c.algo, err, c.want)
	}
}

const (
	sbPriv = "404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f"
	sbPub  = "79a631eede1bf9c98f12032cdeadd0e7a079398fc786b88cc846ec89af85a51a"
	sbBox  = "d950b58a5425757adb6697e11eb78dfda13e8f6de6e55acd39e36a500f679d550c164d3e50d954c0c1b22cb1f458ee1b2913928f68f255d3c49437f7"
)

func TestSealedBoxOpensLibsodiumBox(t *testing.T) {
	got, err := unwrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), bytesArg(unhex(t, sbBox)), bytesArg(unhex(t, sbPriv))})
	if err != nil || string(got.Bytes) != "secret value" {
		t.Fatalf("unwrap: %q, %v", got.Bytes, err)
	}
	tampered := unhex(t, sbBox)
	tampered[len(tampered)-1] ^= 1
	_, err = unwrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), bytesArg(tampered), bytesArg(unhex(t, sbPriv))})
	wantErr(t, "tampered", err, "authentication failed")
}

func TestSealedBoxRoundTripAndChecks(t *testing.T) {
	for _, msg := range [][]byte{{}, []byte("x"), bytes.Repeat([]byte("ab"), 5000)} {
		box, err := wrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), bytesArg(msg), bytesArg(unhex(t, sbPub))})
		if err != nil {
			t.Fatalf("wrap: %v", err)
		}
		if len(box.Bytes) != len(msg)+48 {
			t.Errorf("a box of %d bytes is %d long, want %d", len(msg), len(box.Bytes), len(msg)+48)
		}
		back, err := unwrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), box, bytesArg(unhex(t, sbPriv))})
		if err != nil || !bytes.Equal(back.Bytes, msg) {
			t.Errorf("round trip of %d bytes: %v", len(msg), err)
		}
	}
	_, err := wrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), bytesArg(nil), bytesArg(make([]byte, 31))})
	wantErr(t, "short key", err, "public key is 32 bytes")
	_, err = wrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), bytesArg(nil), bytesArg(make([]byte, 32))})
	wantErr(t, "zero key", err, "low-order point")
	_, err = unwrapFn(noCtx, []interpreter.Value{strArg("nacl-sealedbox"), bytesArg(make([]byte, 47)), bytesArg(unhex(t, sbPriv))})
	wantErr(t, "short box", err, "at least 48 bytes")
}

func hashPw(t *testing.T, algo, pw string) string {
	t.Helper()
	v, err := hashPasswordFn(noCtx, []interpreter.Value{strArg(algo), strArg(pw)})
	if err != nil {
		t.Fatalf("hashPassword(%s): %v", algo, err)
	}
	return v.Str
}

func verifyPw(pw, stored string) (bool, error) {
	v, err := verifyPasswordFn(noCtx, []interpreter.Value{strArg(pw), strArg(stored)})
	return v.Bool, err
}

func TestVerifyPasswordHashesFromOtherLibraries(t *testing.T) {
	for _, stored := range []string{
		"$argon2id$v=19$m=1024,t=2,p=2$ahtLAyEB9UwQF/4YznPWXw$FJluVt22rqfsZN1xGRRwBodITSzeM14iGNeJwhdjGdM", // argon2-cffi
		"$2b$04$hXaEgd58.LJxfxHlvIoT8epQLsY4puU2fjnsbwprXwDZbw5QHhMg2",                                     // Python bcrypt
		"$scrypt$ln=10,r=8,p=1$MDEyMzQ1Njc4OWFiY2RlZg$6g3umF+uVrJsObaTZhIbbTlgrvOEFcCItdwSjtPF67M",         // hashlib.scrypt
	} {
		ok, err := verifyPw("correct horse", stored)
		if err != nil || !ok {
			t.Errorf("%s: %v, %v", stored[:12], ok, err)
		}
		ok, err = verifyPw("correct horsf", stored)
		if err != nil || ok {
			t.Errorf("%s with a wrong password: %v, %v", stored[:12], ok, err)
		}
	}
}

func TestHashPasswordRoundTrip(t *testing.T) {
	for algo, prefix := range map[string]string{
		"argon2id": "$argon2id$v=19$m=65536,t=3,p=4$",
		"bcrypt":   "$2a$12$",
		"scrypt":   "$scrypt$ln=17,r=8,p=1$",
	} {
		h := hashPw(t, algo, "pässword")
		if !strings.HasPrefix(h, prefix) {
			t.Errorf("%s: %s, want prefix %s", algo, h, prefix)
		}
		if h == hashPw(t, algo, "pässword") {
			t.Errorf("%s: two hashes of one password are equal (no fresh salt)", algo)
		}
		if ok, err := verifyPw("pässword", h); !ok || err != nil {
			t.Errorf("%s: verify %v, %v", algo, ok, err)
		}
		if ok, err := verifyPw("password", h); ok || err != nil {
			t.Errorf("%s: wrong password %v, %v", algo, ok, err)
		}
	}
	_, err := hashPasswordFn(noCtx, []interpreter.Value{strArg("bcrypt"), strArg(strings.Repeat("x", 73))})
	wantErr(t, "long bcrypt", err, "at most 72 bytes")
	_, err = hashPasswordFn(noCtx, []interpreter.Value{strArg("md5"), strArg("x")})
	wantErr(t, "unknown", err, `known: "argon2id", "bcrypt", "scrypt"`)
}

func TestVerifyPasswordRefusesMalformedAndCostly(t *testing.T) {
	for _, c := range []struct{ stored, want string }{
		{"plain", "not a hash hashPassword makes"},
		{"$argon2i$v=19$m=1024,t=2,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "not a hash hashPassword makes"},
		{"$argon2id$v=16$m=1024,t=2,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "malformed argon2id"},
		{"$argon2id$v=19$t=2,m=1024,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "malformed argon2id"},
		{"$argon2id$v=19$m=01024,t=2,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "malformed argon2id"},
		{"$argon2id$v=19$m=1024,t=2,p=2$c2FsdHNhbHQ=$aGFzaGhhc2hoYXNoaGFzaA", "malformed argon2id"},
		{"$argon2id$v=19$m=1024,t=2,p=2$c2FsdA$aGFzaGhhc2hoYXNoaGFzaA", "salt is 4 bytes"},
		{"$argon2id$v=19$m=4194304,t=2,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "outside the limits"},
		{"$argon2id$v=19$m=1024,t=99,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "outside the limits"},
		{"$scrypt$ln=25,r=8,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "outside the limits"},
		{"$scrypt$ln=10,r=8$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "malformed scrypt"},
		{"$2b$31$hXaEgd58.LJxfxHlvIoT8epQLsY4puU2fjnsbwprXwDZbw5QHhMg2", "over the limit of 18"},
		{"$2b$04$short", "malformed bcrypt"},
	} {
		_, err := verifyPw("x", c.stored)
		wantErr(t, c.stored, err, c.want)
	}
}

func TestRawKdfVectors(t *testing.T) {
	a, err := argon2idFn(noCtx, []interpreter.Value{bytesArg([]byte("pw")), bytesArg([]byte("saltsalt")), interpreter.IntVal(2), interpreter.IntVal(1024), interpreter.IntVal(2), interpreter.IntVal(32)})
	if err != nil || !bytes.Equal(a.Bytes, unhex(t, "1eb9fb74859d81724892f66222f75c1801c1b3933543766d6e0a051d223e6989")) {
		t.Errorf("argon2id: %x, %v", a.Bytes, err)
	}
	s, err := scryptFn(noCtx, []interpreter.Value{bytesArg([]byte("pw")), bytesArg([]byte("saltsalt")), interpreter.IntVal(1024), interpreter.IntVal(8), interpreter.IntVal(2), interpreter.IntVal(32)})
	if err != nil || !bytes.Equal(s.Bytes, unhex(t, "db7b553159cf6630d4449ffb48d14876c5070363617dac28a89f18dfd7fac115")) {
		t.Errorf("scrypt: %x, %v", s.Bytes, err)
	}
	for _, c := range []struct {
		fn   func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error)
		args []int64
		salt string
		want string
	}{
		{argon2idFn, []int64{0, 1024, 1, 32}, "saltsalt", "time 1 to 16"},
		{argon2idFn, []int64{1, 1 << 21, 1, 32}, "saltsalt", "time 1 to 16"},
		{argon2idFn, []int64{1, 1024, 1, 32}, "short", "at least 8 bytes"},
		{scryptFn, []int64{1000, 8, 1, 32}, "saltsalt", "power of two"},
		{scryptFn, []int64{1 << 21, 8, 1, 32}, "saltsalt", "outside the limits"},
		{scryptFn, []int64{1024, 8, 1, 0}, "saltsalt", "keyLen must be 1 to"},
	} {
		args := []interpreter.Value{bytesArg([]byte("pw")), bytesArg([]byte(c.salt))}
		for _, n := range c.args {
			args = append(args, interpreter.IntVal(n))
		}
		_, err := c.fn(noCtx, args)
		wantErr(t, c.want, err, c.want)
	}
}
