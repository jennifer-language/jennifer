// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

// M29.51: the RSA signature names (PKCS#1 v1.5 and PSS), bytes passwords,
// jwkToPem returning bytes, every hash but md5 in HKDF / PBKDF2, and the
// "(weak)" tag in the unknown-name lists.

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func signWith(t *testing.T, algo string, msg, priv []byte) []byte {
	t.Helper()
	v, err := signFn(noCtx, []interpreter.Value{strArg(algo), bytesArg(msg), bytesArg(priv)})
	if err != nil {
		t.Fatalf("sign(%s): %v", algo, err)
	}
	return v.Bytes
}

func verifyWith(t *testing.T, algo string, msg, sig, pub []byte) bool {
	t.Helper()
	v, err := verifyFn(noCtx, []interpreter.Value{strArg(algo), bytesArg(msg), bytesArg(sig), bytesArg(pub)})
	if err != nil {
		t.Fatalf("verify(%s): %v", algo, err)
	}
	return v.Bool
}

func TestRSASignNamesRoundTrip(t *testing.T) {
	pub, priv := genKeypair(t, "rsa2048")
	msg := []byte("ship it")
	for _, algo := range []string{"rsa-pkcs1-sha256", "rsa-pkcs1-sha384", "rsa-pkcs1-sha512", "rsa-pss-sha256", "rsa-pss-sha384", "rsa-pss-sha512"} {
		sig := signWith(t, algo, msg, priv)
		if !verifyWith(t, algo, msg, sig, pub) {
			t.Errorf("%s: own signature does not verify", algo)
		}
		if verifyWith(t, algo, []byte("ship it!"), sig, pub) {
			t.Errorf("%s: verified a changed message", algo)
		}
		// PKCS#1 v1.5 and PSS signatures are not interchangeable.
		other := strings.Replace(algo, "pkcs1", "pss", 1)
		if strings.Contains(algo, "pss") {
			other = strings.Replace(algo, "pss", "pkcs1", 1)
		}
		if verifyWith(t, other, msg, sig, pub) {
			t.Errorf("%s signature verified as %s", algo, other)
		}
	}
}

// The deprecated "rsa-sha*" names are "rsa-pkcs1-sha*": PKCS#1 v1.5 is
// deterministic, so both names give the same bytes.
func TestRSAOldNamesArePKCS1(t *testing.T) {
	pub, priv := genKeypair(t, "rsa2048")
	msg := []byte("same bytes")
	for _, bits := range []string{"256", "384", "512"} {
		old, now := "rsa-sha"+bits, "rsa-pkcs1-sha"+bits
		a, b := signWith(t, old, msg, priv), signWith(t, now, msg, priv)
		if !bytes.Equal(a, b) {
			t.Errorf("%s and %s sign differently", old, now)
		}
		if !verifyWith(t, old, msg, b, pub) || !verifyWith(t, now, msg, a, pub) {
			t.Errorf("%s and %s do not verify each other", old, now)
		}
	}
}

// A signature from Go's rsa.SignPSS with the documented parameters (MGF1 with
// the message hash, salt length equal to the hash) verifies; one with another
// salt length does not.
func TestRSAPSSVerifiesGoSignature(t *testing.T) {
	key, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	msg := []byte("signed elsewhere")
	digest := sha256.Sum256(msg)
	sig, err := rsa.SignPSS(crand.Reader, key, crypto.SHA256, digest[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	if err != nil {
		t.Fatal(err)
	}
	if !verifyWith(t, "rsa-pss-sha256", msg, sig, pub) {
		t.Error("rsa.SignPSS signature (salt = hash length) does not verify")
	}
	short, err := rsa.SignPSS(crand.Reader, key, crypto.SHA256, digest[:], &rsa.PSSOptions{SaltLength: 8})
	if err != nil {
		t.Fatal(err)
	}
	if verifyWith(t, "rsa-pss-sha256", msg, short, pub) {
		t.Error("a PSS signature with an 8-byte salt verified")
	}
	// And the other way: Go verifies ours with the same parameters.
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	ours := signWith(t, "rsa-pss-sha256", msg, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))
	if err := rsa.VerifyPSS(&key.PublicKey, crypto.SHA256, digest[:], ours, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}); err != nil {
		t.Errorf("rsa.VerifyPSS rejects crypto.sign's rsa-pss-sha256: %v", err)
	}
}

// jwkToPem returns bytes that verify takes as they are, for PSS too.
func TestJwkToPemBytesVerify(t *testing.T) {
	_, priv := genKeypair(t, "rsa2048")
	jwk, err := jwkPublicFn(noCtx, []interpreter.Value{bytesArg(priv)})
	if err != nil {
		t.Fatal(err)
	}
	pemV, err := jwkToPemFn(noCtx, []interpreter.Value{strArg(jwk.Str)})
	if err != nil {
		t.Fatal(err)
	}
	if pemV.Kind != interpreter.KindBytes {
		t.Fatalf("jwkToPem returned %s, want bytes", pemV.Kind)
	}
	msg := []byte("m")
	if !verifyWith(t, "rsa-pss-sha512", msg, signWith(t, "rsa-pss-sha512", msg, priv), pemV.Bytes) {
		t.Error("rsa-pss-sha512 signature does not verify against jwkToPem's key")
	}
}

// A bytes password and the deprecated string one are the same password.
func TestPasswordBytesAndString(t *testing.T) {
	const pw = "correct horse battery staple"
	hashWith := func(v interpreter.Value) string {
		h, err := hashPasswordFn(noCtx, []interpreter.Value{strArg("argon2id"), v})
		if err != nil {
			t.Fatal(err)
		}
		return h.Str
	}
	check := func(v interpreter.Value, stored string) bool {
		ok, err := verifyPasswordFn(noCtx, []interpreter.Value{v, strArg(stored)})
		if err != nil {
			t.Fatal(err)
		}
		return ok.Bool
	}
	fromBytes, fromString := hashWith(bytesArg([]byte(pw))), hashWith(strArg(pw))
	for _, stored := range []string{fromBytes, fromString} {
		if !check(bytesArg([]byte(pw)), stored) || !check(strArg(pw), stored) {
			t.Errorf("bytes and string password disagree on %s", stored)
		}
		if check(bytesArg([]byte(pw+"!")), stored) {
			t.Error("a wrong password verified")
		}
	}
	// A password need not be text.
	raw := []byte{0xff, 0x00, 0xfe}
	if !check(bytesArg(raw), hashWith(bytesArg(raw))) {
		t.Error("non-UTF-8 bytes password does not verify")
	}
	_, err := hashPasswordFn(noCtx, []interpreter.Value{strArg("argon2id"), interpreter.IntVal(1)})
	wantErr(t, "int password", err, "crypto.hashPassword: password must be bytes, got int")
	_, err = verifyPasswordFn(noCtx, []interpreter.Value{interpreter.IntVal(1), strArg(fromBytes)})
	wantErr(t, "int password", err, "crypto.verifyPassword: password must be bytes, got int")
}

func unhexKDF(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func deriveOK(t *testing.T, algo string, secret, salt []byte, length int64, p interpreter.Value) []byte {
	t.Helper()
	v, err := derive(algo, secret, salt, length, p)
	if err != nil {
		t.Fatalf("%s: %v", algo, err)
	}
	return v.Bytes
}

// Published vectors: RFC 5869 A.1 (HKDF-SHA256) and A.4 (HKDF-SHA1), RFC 6070
// (PBKDF2-HMAC-SHA1), RFC 7914 11 (PBKDF2-HMAC-SHA256).
func TestDeriveKeyPublishedVectors(t *testing.T) {
	ikm22 := bytes.Repeat([]byte{0x0b}, 22)
	salt := unhexKDF(t, "000102030405060708090a0b0c")
	info := unhexKDF(t, "f0f1f2f3f4f5f6f7f8f9")
	iters := func(n int64) interpreter.Value { return kdfParams(nil, map[string]int64{"iterations": n}) }
	cases := []struct {
		name, algo   string
		secret, salt []byte
		length       int64
		params       interpreter.Value
		want         string
	}{
		{"RFC 5869 A.1", "hkdf-sha256", ikm22, salt, 42, kdfParams(info, nil),
			"3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"},
		{"RFC 5869 A.4", "hkdf-sha1", ikm22[:11], salt, 42, kdfParams(info, nil),
			"085a01ea1b10f36933068b56efa5ad81a4f14b822f5b091568a9cdd4f155fda2c22e422478d305f3f896"},
		{"RFC 6070 c=1", "pbkdf2-sha1", []byte("password"), []byte("salt"), 20, iters(1),
			"0c60c80f961f0e71f3a9b524af6012062fe037a6"},
		{"RFC 6070 c=2", "pbkdf2-sha1", []byte("password"), []byte("salt"), 20, iters(2),
			"ea6c014dc72d6f8ccd1ed92ace1d41f0d8de8957"},
		{"RFC 6070 c=4096", "pbkdf2-sha1", []byte("password"), []byte("salt"), 20, iters(4096),
			"4b007901b765489abead49d926f721d065a429c1"},
		{"RFC 7914 11", "pbkdf2-sha256", []byte("passwd"), []byte("salt"), 64, iters(1),
			"55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783"},
	}
	for _, c := range cases {
		got := deriveOK(t, c.algo, c.secret, c.salt, c.length, c.params)
		if hex.EncodeToString(got) != c.want {
			t.Errorf("%s (%s): got %x", c.name, c.algo, got)
		}
	}
}

// refHKDF and refPBKDF2 are RFC 5869 and RFC 8018 written out over crypto/hmac,
// independent of crypto/hkdf and crypto/pbkdf2, to check every hash name.
func refHKDF(newMAC func(key []byte) func(data ...[]byte) []byte, secret, salt, info []byte, length int) []byte {
	prk := newMAC(salt)(secret)
	var out, prev []byte
	for i := byte(1); len(out) < length; i++ {
		prev = newMAC(prk)(prev, info, []byte{i})
		out = append(out, prev...)
	}
	return out[:length]
}

func refPBKDF2(newMAC func(key []byte) func(data ...[]byte) []byte, password, salt []byte, iter, length int) []byte {
	mac := newMAC(password)
	var out []byte
	for block := uint32(1); len(out) < length; block++ {
		var idx [4]byte
		binary.BigEndian.PutUint32(idx[:], block)
		u := mac(salt, idx[:])
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			u = mac(u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:length]
}

func TestDeriveKeyEveryHash(t *testing.T) {
	if len(kdfHashNames) != len(kdfHash) {
		t.Fatalf("kdfHashNames has %d names, kdfHash %d", len(kdfHashNames), len(kdfHash))
	}
	secret, salt, info := []byte("input keying material"), []byte("0123456789abcdef"), []byte("ctx")
	for _, name := range kdfHashNames {
		ctor := kdfHash[name]
		newMAC := func(key []byte) func(data ...[]byte) []byte {
			return func(data ...[]byte) []byte {
				m := hmac.New(ctor, key)
				for _, d := range data {
					m.Write(d)
				}
				return m.Sum(nil)
			}
		}
		// Two blocks or more of output for every hash, so the counter is used.
		length := 2*ctor().Size() + 3
		got := deriveOK(t, "hkdf-"+name, secret, salt, int64(length), kdfParams(info, nil))
		if want := refHKDF(newMAC, secret, salt, info, length); !bytes.Equal(got, want) {
			t.Errorf("hkdf-%s differs from the reference", name)
		}
		got = deriveOK(t, "pbkdf2-"+name, secret, salt, int64(length), kdfParams(nil, map[string]int64{"iterations": 3}))
		if want := refPBKDF2(newMAC, secret, salt, 3, length); !bytes.Equal(got, want) {
			t.Errorf("pbkdf2-%s differs from the reference", name)
		}
	}
}

func TestDeriveKeyRefusesMD5(t *testing.T) {
	for _, algo := range []string{"hkdf-md5", "pbkdf2-md5"} {
		_, err := derive(algo, []byte("pw"), []byte("0123456789abcdef"), 16, kdfParams(nil, map[string]int64{"iterations": 1}))
		wantErr(t, algo, err, "md5 is not offered for key derivation")
		if err != nil && strings.Contains(err.Error(), "unknown algorithm") {
			t.Errorf("%s: reported as unknown: %v", algo, err)
		}
	}
}

// Every unknown-name list tags the weak names and leaves the deprecated ones out.
func TestUnknownNameListsTagWeakNames(t *testing.T) {
	b := bytesArg([]byte("x"))
	cases := []struct {
		fn   string
		call interpreter.Builtin
		args []interpreter.Value
		weak []string
	}{
		{"sign", signFn, []interpreter.Value{strArg("rsa-md5"), b, b}, nil},
		{"verify", verifyFn, []interpreter.Value{strArg("rsa-sha1"), b, b, b}, nil},
		{"blockEncrypt", blockEncryptFn, []interpreter.Value{strArg("des"), b, b, interpreter.Null()}, []string{`"aes-ecb-nopad" (weak)`, `"aes-ecb-pkcs7" (weak)`}},
		{"wrap", wrapFn, []interpreter.Value{strArg("rsa-pkcs1"), b, b}, []string{`"rsa-oaep-sha1" (weak)`}},
		{"deriveKey", deriveKeyFn, []interpreter.Value{strArg("hkdf-sha224"), b, b, interpreter.IntVal(16), kdfParams(nil, nil)}, []string{`"sha1" (weak)`}},
	}
	for _, c := range cases {
		_, err := c.call(noCtx, c.args)
		if err == nil {
			t.Fatalf("%s: no error", c.fn)
		}
		msg := err.Error()
		for _, w := range c.weak {
			if !strings.Contains(msg, w) {
				t.Errorf("%s: %q lacks %s", c.fn, msg, w)
			}
		}
		for old := range rsaSignNames {
			if strings.Contains(msg, `"`+old+`"`) {
				t.Errorf("%s: %q lists the deprecated %q", c.fn, msg, old)
			}
		}
		if strings.Count(msg, "(weak)") != len(c.weak) {
			t.Errorf("%s: %q tags other names weak", c.fn, msg)
		}
	}
}
