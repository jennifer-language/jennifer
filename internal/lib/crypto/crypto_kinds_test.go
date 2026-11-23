// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// M29.21: input data found bad (a ciphertext that does not open, a malformed
// key, JWK or stored password hash, a hostile peer point) is kind "crypto", a
// reached cap stays "limit", and misuse (a wrong argument, an unknown name, a
// key or nonce of the wrong length or type) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	s, b := opStr, opBytes
	call := func(fn opFn, args ...interpreter.Value) func() error {
		return func() error { _, err := fn(ctx, args); return err }
	}
	key32, nonce12, iv16 := make([]byte, 32), make([]byte, 12), make([]byte, 16)
	box := opCall(t, encryptFn, s("aes-gcm"), b([]byte("hi")), b(key32), b(nonce12), b(nil)).Bytes
	tampered := append([]byte(nil), box...)
	tampered[0] ^= 1
	ed := opCall(t, keypairFn, s("ed25519"))
	edBad := append([]byte(nil), opField(ed, "private")...)
	edBad[40] ^= 1
	rsa := opCall(t, keypairFn, s("rsa2048"))
	ec := opCall(t, keypairFn, s("p256"))
	x := opCall(t, keypairFn, s("x25519"))
	ml := opCall(t, keypairFn, s("mlkem768"))
	cbcPad := opCall(t, blockEncryptFn, s("aes-cbc-nopad"), b(bytes.Repeat([]byte{0x20}, 16)), b(iv16[:16]), b(iv16)).Bytes
	notPEM := []byte("-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n")
	sealed := opCall(t, wrapFn, s("nacl-sealedbox"), b([]byte("hi")), b(opField(x, "public"))).Bytes
	sealed[len(sealed)-1] ^= 1
	b64 := base64.RawURLEncoding.EncodeToString
	hugeN := b64(bytes.Repeat([]byte{0xff}, 16384/8+2))
	salt := base64.RawStdEncoding.EncodeToString([]byte("saltsaltsaltsalt"))
	hash := base64.RawStdEncoding.EncodeToString([]byte("hashhashhashhash"))

	cases := []struct {
		name string
		call func() error
		kind string
	}{
		// data
		{"decrypt tampered", call(decryptFn, s("aes-gcm"), b(tampered), b(key32), b(nonce12), b(nil)), "crypto"},
		{"decrypt short box", call(decryptFn, s("chacha20-poly1305"), b([]byte{1}), b(key32), b(nonce12), b(nil)), "crypto"},
		{"decrypt wrong aad", call(decryptFn, s("aes-gcm"), b(box), b(key32), b(nonce12), b([]byte("x"))), "crypto"},
		{"blockDecrypt bad padding", call(blockDecryptFn, s("aes-cbc-pkcs7"), b(cbcPad), b(iv16), b(iv16)), "crypto"},
		{"blockDecrypt empty pkcs7", call(blockDecryptFn, s("aes-ecb-pkcs7"), b(nil), b(iv16), interpreter.Null()), "crypto"},
		{"blockDecrypt unaligned", call(blockDecryptFn, s("aes-cbc-nopad"), b([]byte{1, 2, 3}), b(iv16), b(iv16)), "crypto"},
		{"sign ed25519 mismatched half", call(signFn, s("ed25519"), b(nil), b(edBad)), "crypto"},
		{"publicKey ed25519 mismatched half", call(publicKeyFn, s("ed25519"), b(edBad)), "crypto"},
		{"verify rsa not PEM", call(verifyFn, s("rsa-sha256"), b(nil), b(nil), b([]byte("junk"))), "crypto"},
		{"verify rsa bad DER", call(verifyFn, s("rsa-sha256"), b(nil), b(nil), b(notPEM)), "crypto"},
		{"verify ecdsa bad DER", call(verifyFn, s("ecdsa-sha256"), b(nil), b(nil), b(notPEM)), "crypto"},
		{"sign rsa bad DER", call(signFn, s("rsa-sha256"), b(nil), b(notPEM)), "crypto"},
		{"sign ecdsa bad DER", call(signFn, s("ecdsa-sha256"), b(nil), b(notPEM)), "crypto"},
		{"jwkPublic bad DER", call(jwkPublicFn, b(notPEM)), "crypto"},
		{"csr bad DER", call(csrFn, b(notPEM), interpreter.ListVal(parser.PrimitiveType(parser.TypeString), []interpreter.Value{s("a.example")})), "crypto"},
		{"unwrap rsa garbage", call(unwrapFn, s("rsa-oaep-sha256"), b(make([]byte, 256)), b(opField(rsa, "private"))), "crypto"},
		{"unwrap sealedbox tampered", call(unwrapFn, s("nacl-sealedbox"), b(sealed), b(opField(x, "private"))), "crypto"},
		{"unwrap sealedbox short", call(unwrapFn, s("nacl-sealedbox"), b([]byte{1}), b(opField(x, "private"))), "crypto"},
		{"wrap sealedbox low-order key", call(wrapFn, s("nacl-sealedbox"), b(nil), b(make([]byte, 32))), "crypto"},
		{"agree low-order peer", call(agreeFn, s("x25519"), b(opField(x, "private")), b(make([]byte, 32))), "crypto"},
		{"encapsulate undecodable key", call(encapsulateFn, s("mlkem768"), b(bytes.Repeat([]byte{0xff}, 1184))), "crypto"},
		{"decapsulate short ciphertext", call(decapsulateFn, s("mlkem768"), b([]byte{1}), b(opField(ml, "private"))), "crypto"},
		{"jwkToPem bad JSON", call(jwkToPemFn, s("{")), "crypto"},
		{"jwkToPem unknown kty", call(jwkToPemFn, s(`{"kty":"OKP"}`)), "crypto"},
		{"jwkToPem missing n", call(jwkToPemFn, s(`{"kty":"RSA","e":"AQAB"}`)), "crypto"},
		{"jwkToPem bad curve", call(jwkToPemFn, s(`{"kty":"EC","crv":"P-256K","x":"AQ","y":"AQ"}`)), "crypto"},
		{"jwkToPem off curve", call(jwkToPemFn, s(`{"kty":"EC","crv":"P-256","x":"AQ","y":"AQ"}`)), "crypto"},
		{"verifyPassword unknown form", call(verifyPasswordFn, s("pw"), s("plain")), "crypto"},
		{"verifyPassword bad bcrypt", call(verifyPasswordFn, s("pw"), s("$2a$xx")), "crypto"},
		{"verifyPassword short bcrypt", call(verifyPasswordFn, s("pw"), s("$2a$04$abc")), "crypto"},
		{"verifyPassword bad argon2id", call(verifyPasswordFn, s("pw"), s("$argon2id$v=19$x")), "crypto"},
		{"verifyPassword argon2id m below 8p", call(verifyPasswordFn, s("pw"), s("$argon2id$v=19$m=8,t=1,p=2$"+salt+"$"+hash)), "crypto"},
		{"verifyPassword bad scrypt", call(verifyPasswordFn, s("pw"), s("$scrypt$ln=x")), "crypto"},
		{"sign rsa with EC key", call(signFn, s("rsa-sha256"), b(nil), b(opField(ec, "private"))), "crypto"},
		{"verify ecdsa with RSA key", call(verifyFn, s("ecdsa-sha256"), b(nil), b(nil), b(opField(rsa, "public"))), "crypto"},
		{"publicKey wrong curve", call(publicKeyFn, s("p384"), b(opField(ec, "private"))), "crypto"},
		{"publicKey wrong RSA size", call(publicKeyFn, s("rsa4096"), b(opField(rsa, "private"))), "crypto"},
		// caps
		{"verifyPassword argon2id cap", call(verifyPasswordFn, s("pw"), s("$argon2id$v=19$m=99999999,t=1,p=1$"+salt+"$"+hash)), "limit"},
		{"verifyPassword scrypt cap", call(verifyPasswordFn, s("pw"), s("$scrypt$ln=40,r=8,p=1$"+salt+"$"+hash)), "limit"},
		{"jwkToPem RSA modulus cap", call(jwkToPemFn, s(`{"kty":"RSA","n":"`+hugeN+`","e":"AQAB"}`)), "limit"},
		// misuse
		{"decrypt short key", call(decryptFn, s("aes-gcm"), b(box), b(key32[:5]), b(nonce12), b(nil)), ""},
		{"decrypt short nonce", call(decryptFn, s("aes-gcm"), b(box), b(key32), b(nonce12[:3]), b(nil)), ""},
		{"decrypt unknown algo", call(decryptFn, s("rot13"), b(box), b(key32), b(nonce12), b(nil)), ""},
		{"decrypt wrong arg type", call(decryptFn, s("aes-gcm"), s("box"), b(key32), b(nonce12), b(nil)), ""},
		{"blockEncrypt unaligned", call(blockEncryptFn, s("aes-cbc-nopad"), b([]byte{1}), b(iv16), b(iv16)), ""},
		{"verify ed25519 short key", call(verifyFn, s("ed25519"), b(nil), b(nil), b([]byte{1})), ""},
		{"sign ed25519 short key", call(signFn, s("ed25519"), b(nil), b([]byte{1})), ""},
		{"unwrap sealedbox short key", call(unwrapFn, s("nacl-sealedbox"), b(sealed), b([]byte{1})), ""},
		{"agree short peer", call(agreeFn, s("x25519"), b(opField(x, "private")), b([]byte{1})), ""},
		{"encapsulate short key", call(encapsulateFn, s("mlkem768"), b([]byte{1})), ""},
		{"decapsulate short key", call(decapsulateFn, s("mlkem768"), b(make([]byte, 1088)), b([]byte{1})), ""},
		{"hashPassword long bcrypt", call(hashPasswordFn, s("bcrypt"), s(strings.Repeat("x", 73))), ""},
		{"mtweiClientKey short server key", call(mtweiClientKeyFn, b(key32), b(key32), b(make([]byte, 33)), b(key32)), ""},
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

// A signature that does not verify is false, not an error.
func TestVerifyMismatchIsFalse(t *testing.T) {
	for _, algo := range []string{"ed25519", "rsa-sha256", "ecdsa-sha256"} {
		kpAlgo := map[string]string{"ed25519": "ed25519", "rsa-sha256": "rsa2048", "ecdsa-sha256": "p256"}[algo]
		kp := opCall(t, keypairFn, opStr(kpAlgo))
		sig := opCall(t, signFn, opStr(algo), opBytes([]byte("a")), opBytes(opField(kp, "private"))).Bytes
		v := opCall(t, verifyFn, opStr(algo), opBytes([]byte("b")), opBytes(sig), opBytes(opField(kp, "public")))
		if v.Bool {
			t.Errorf("%s: a changed message verified", algo)
		}
	}
}

// mtweiClientKey: a server key off the curve is kind "crypto".
func TestMtweiOffCurveKind(t *testing.T) {
	key := make([]byte, 33)
	for i := 0; i < 64; i++ {
		key[0] = byte(i)
		_, err := mtweiClientKeyFn(interpreter.BuiltinCtx{}, []interpreter.Value{
			opBytes(make([]byte, 32)), opBytes(key), opBytes(make([]byte, 33)), opBytes(bytes.Repeat([]byte{1}, 32)),
		})
		if err != nil && strings.Contains(err.Error(), "not on the curve") {
			if k := dataerr.KindOf(err); k != "crypto" {
				t.Fatalf("%v: kind %q", err, k)
			}
			return
		}
	}
	t.Skip("no off-curve candidate found")
}
