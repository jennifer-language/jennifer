// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

// Known-answer tests for the M28.40 surface (cryptolib_ops.go), against
// vectors from independent implementations: the published AES-GCM test cases
// (McGrew & Viega, reproduced with OpenSSL), FIPS-197 / OpenSSL `enc` for the
// block modes, RFC 8032 (Ed25519), RFC 7748 (X25519), and OpenSSL / Node
// fixtures under testdata/kat for RSA, ECDSA and RSA-OAEP. Plus the error
// contract: sizes, IVs, padding, unknown names and wrong key types.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func opHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func opFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/kat/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func opStr(s string) interpreter.Value   { return interpreter.StringVal(s) }
func opBytes(b []byte) interpreter.Value { return interpreter.BytesVal(b) }

type opFn func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error)

func opCall(t *testing.T, fn opFn, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

func opErr(t *testing.T, want string, fn opFn, args ...interpreter.Value) {
	t.Helper()
	_, err := fn(interpreter.BuiltinCtx{}, args)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("want an error containing %q, got %v", want, err)
	}
}

func opField(v interpreter.Value, name string) []byte {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value.Bytes
		}
	}
	return nil
}

// ----- encrypt / decrypt ----------------------------------------------------

func TestKatAESGCM(t *testing.T) {
	cases := []struct{ key, nonce, pt, aad, want string }{
		// McGrew & Viega, test case 2 and 4 (AES-128).
		{"00000000000000000000000000000000", "000000000000000000000000", "00000000000000000000000000000000", "",
			"0388dace60b6a392f328c2b971b2fe78ab6e47d42cec13bdf53a67b21257bddf"},
		{"feffe9928665731c6d6a8f9467308308", "cafebabefacedbaddecaf888",
			"d9313225f88406e5a55909c5aff5269a86a7a9531534f7da2e4c303d8a318a721c3c0c95956809532fcf0e2449a6b525b16aedf5aa0de657ba637b39",
			"feedfacedeadbeeffeedfacedeadbeefabaddad2",
			"42831ec2217774244b7221b784d0d49ce3aa212f2c02a4e035c17e2329aca12e21d514b25466931c7d8f6a5aac84aa051ba30b396a0aac973d58e0915bc94fbc3221a5db94fae95ae7121a47"},
		// OpenSSL via Node: AES-192 empty message, AES-256 with AAD.
		{"000102030405060708090a0b0c0d0e0f1011121314151617", "0a0b0c0d0e0f101112131415", "", "",
			"00350ffc7963633c93cac263679fb97d"},
		{"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f", "0a0b0c0d0e0f101112131415",
			hex.EncodeToString([]byte("jennifer")), hex.EncodeToString([]byte("hdr")),
			"05d854a602bbb207fe95407968566733f98211afb5900dfc"},
	}
	for i, c := range cases {
		key, nonce, pt, aad := opHex(t, c.key), opHex(t, c.nonce), opHex(t, c.pt), opHex(t, c.aad)
		got := opCall(t, encryptFn, opStr("aes-gcm"), opBytes(pt), opBytes(key), opBytes(nonce), opBytes(aad)).Bytes
		if hex.EncodeToString(got) != c.want {
			t.Errorf("case %d: encrypt = %x, want %s", i, got, c.want)
		}
		back := opCall(t, decryptFn, opStr("aes-gcm"), opBytes(got), opBytes(key), opBytes(nonce), opBytes(aad)).Bytes
		if !bytes.Equal(back, pt) {
			t.Errorf("case %d: decrypt round trip = %x, want %x", i, back, pt)
		}
	}
}

func TestDecryptRejectsTamperingAndMismatch(t *testing.T) {
	key, nonce, aad := bytes.Repeat([]byte{7}, 32), bytes.Repeat([]byte{9}, 12), []byte("header")
	box := opCall(t, encryptFn, opStr("aes-gcm"), opBytes([]byte("secret")), opBytes(key), opBytes(nonce), opBytes(aad)).Bytes
	const auth = "authentication failed"
	flipped := append([]byte(nil), box...)
	flipped[0] ^= 1
	opErr(t, auth, decryptFn, opStr("aes-gcm"), opBytes(flipped), opBytes(key), opBytes(nonce), opBytes(aad))
	otherKey := bytes.Repeat([]byte{8}, 32)
	opErr(t, auth, decryptFn, opStr("aes-gcm"), opBytes(box), opBytes(otherKey), opBytes(nonce), opBytes(aad))
	otherNonce := bytes.Repeat([]byte{1}, 12)
	opErr(t, auth, decryptFn, opStr("aes-gcm"), opBytes(box), opBytes(key), opBytes(otherNonce), opBytes(aad))
	opErr(t, auth, decryptFn, opStr("aes-gcm"), opBytes(box), opBytes(key), opBytes(nonce), opBytes([]byte("other")))
	opErr(t, auth, decryptFn, opStr("aes-gcm"), opBytes(box[:5]), opBytes(key), opBytes(nonce), opBytes(aad))
}

// A box sealed by the pre-0.26.0 crypto.encrypt(key, plaintext) - AES-256-GCM,
// nonce || ciphertext || tag, no AAD - opens with the new decrypt once the
// nonce is split off, so data stored that way (the registry's forge
// credentials) stays readable.
func TestOldEncryptLayoutOpens(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	nonce := opHex(t, "000102030405060708090a0b")
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	old := gcm.Seal(append([]byte(nil), nonce...), nonce, []byte("forge token"), nil)
	got := opCall(t, decryptFn, opStr("aes-gcm"), opBytes(old[12:]), opBytes(key), opBytes(old[:12]), opBytes(nil)).Bytes
	if string(got) != "forge token" {
		t.Errorf("old layout opened as %q", got)
	}
}

func TestEncryptArgumentErrors(t *testing.T) {
	k, n, e := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 12), []byte{}
	opErr(t, `unknown algorithm "aes-ccm"; known: "aes-gcm"`, encryptFn, opStr("aes-ccm"), opBytes(e), opBytes(k), opBytes(n), opBytes(e))
	opErr(t, "key must be 16, 24 or 32 bytes", encryptFn, opStr("aes-gcm"), opBytes(e), opBytes(k[:31]), opBytes(n), opBytes(e))
	opErr(t, "nonce must be 12 bytes", encryptFn, opStr("aes-gcm"), opBytes(e), opBytes(k), opBytes(n[:8]), opBytes(e))
	opErr(t, "expects 5 arguments", encryptFn, opStr("aes-gcm"), opBytes(e), opBytes(k))
	opErr(t, "algorithm (first argument) must be a string", encryptFn, opBytes(k), opBytes(e), opBytes(k), opBytes(n), opBytes(e))
	opErr(t, "argument 5 must be bytes", encryptFn, opStr("aes-gcm"), opBytes(e), opBytes(k), opBytes(n), interpreter.Null())
}

// ----- blockEncrypt / blockDecrypt ------------------------------------------

func TestKatBlockModes(t *testing.T) {
	keys := map[int]string{
		128: "000102030405060708090a0b0c0d0e0f",
		192: "000102030405060708090a0b0c0d0e0f1011121314151617",
		256: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
	}
	iv := opHex(t, "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
	b32 := opHex(t, "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t20, t21 := []byte("jennifer block mode!"), []byte("twenty-one bytes long")
	// openssl enc -aes-<bits>-<mode> -K <key> [-iv <iv>] [-nopad]
	want := map[string]string{
		"aes-ecb-nopad/128": "69c4e0d86a7b0430d8cdb78070b4c55a69c4e0d86a7b0430d8cdb78070b4c55a",
		"aes-ecb-pkcs7/128": "0b8a665af99d3193e0116d4e1d4b0676ac87491104c3d99262f6c88d31074f63",
		"aes-cbc-nopad/128": "7702fc9b71c63d26a2f09df5c445102a24ea9d759e280fbdd6b1d259a6bacc8a",
		"aes-cbc-pkcs7/128": "93029069f00734f266f914907d21707a1c93236b7a351f3d952fa3cbb60732fc",
		"aes-ctr/128":       "12d0a286402b1c27f934fe654a62c8de92edb86ed0",
		"aes-cfb/128":       "12d0a286402b1c27f934fe654a62c8de2fa68a4903",
		"aes-ecb-nopad/192": "dda97ca4864cdfe06eaf70a0ec0d7191dda97ca4864cdfe06eaf70a0ec0d7191",
		"aes-ecb-pkcs7/192": "ded07d2a3e17caf784a83d22fdca0a19b86730753018faa02399bd7e44a18ffa",
		"aes-cbc-nopad/192": "e332288fa5726b29d5e8b9bac5bec78345924cde52f14d082ce665d9e1d0ea84",
		"aes-cbc-pkcs7/192": "d731f62f11164e28352bffcdf93a47025414165ea5f96ce521fae64dda9de7fd",
		"aes-ctr/192":       "5ff52d3c208b44ffddbc16608991cd80fb6444edae",
		"aes-cfb/192":       "5ff52d3c208b44ffddbc16608991cd80d9c03b06ff",
		"aes-ecb-nopad/256": "8ea2b7ca516745bfeafc49904b4960898ea2b7ca516745bfeafc49904b496089",
		"aes-ecb-pkcs7/256": "7c34ecc2ef57dc1ed94e8bb25c2de19d851a90218d3a94d4c16c52598a63d2ce",
		"aes-cbc-nopad/256": "f1801e62e61a0b8937d26622c2a5141f613c9c5ed65e218b106c7f169eb6c588",
		"aes-cbc-pkcs7/256": "aadfd24459db2f03fc6d45176786c0accaff4b3eee398737a0e15dc5506b1490",
		"aes-ctr/256":       "e677a8e357efada4340cc63639460667ea33142e96",
		"aes-cfb/256":       "e677a8e357efada4340cc63639460667241dea72ba",
	}
	for name, wantHex := range want {
		algo, bitsStr, _ := strings.Cut(name, "/")
		bits := map[string]int{"128": 128, "192": 192, "256": 256}[bitsStr]
		key := opHex(t, keys[bits])
		var data []byte
		switch {
		case strings.HasSuffix(algo, "-nopad"):
			data = b32
		case strings.HasSuffix(algo, "-pkcs7"):
			data = t20
		default:
			data = t21
		}
		ivArg := opBytes(iv)
		if strings.HasPrefix(algo, "aes-ecb") {
			ivArg = interpreter.Null()
		}
		got := opCall(t, blockEncryptFn, opStr(algo), opBytes(data), opBytes(key), ivArg).Bytes
		if hex.EncodeToString(got) != wantHex {
			t.Errorf("%s: blockEncrypt = %x, want %s", name, got, wantHex)
		}
		back := opCall(t, blockDecryptFn, opStr(algo), opBytes(got), opBytes(key), ivArg).Bytes
		if !bytes.Equal(back, data) {
			t.Errorf("%s: blockDecrypt round trip = %q, want %q", name, back, data)
		}
	}
}

func TestBlockModeErrors(t *testing.T) {
	k, iv := bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 16)
	null := interpreter.Null()
	opErr(t, `takes no IV; pass null`, blockEncryptFn, opStr("aes-ecb-nopad"), opBytes(make([]byte, 16)), opBytes(k), opBytes(iv))
	opErr(t, `needs a 16-byte IV`, blockEncryptFn, opStr("aes-cbc-pkcs7"), opBytes([]byte("x")), opBytes(k), null)
	opErr(t, `IV must be 16 bytes`, blockEncryptFn, opStr("aes-ctr"), opBytes([]byte("x")), opBytes(k), opBytes(iv[:8]))
	opErr(t, `multiple of 16 bytes`, blockEncryptFn, opStr("aes-cbc-nopad"), opBytes(make([]byte, 17)), opBytes(k), opBytes(iv))
	opErr(t, `multiple of 16 bytes`, blockDecryptFn, opStr("aes-ecb-pkcs7"), opBytes(make([]byte, 15)), opBytes(k), null)
	opErr(t, `key must be 16, 24 or 32 bytes`, blockEncryptFn, opStr("aes-cfb"), opBytes([]byte("x")), opBytes(k[:10]), opBytes(iv))
	opErr(t, `unknown algorithm "aes-cbc"`, blockEncryptFn, opStr("aes-cbc"), opBytes(make([]byte, 16)), opBytes(k), opBytes(iv))
	// A padding that does not check out is an error, never silently trimmed.
	good := opCall(t, blockEncryptFn, opStr("aes-ecb-nopad"), opBytes(append(bytes.Repeat([]byte("A"), 13), 3, 3, 4)), opBytes(k), null).Bytes
	opErr(t, "bad padding", blockDecryptFn, opStr("aes-ecb-pkcs7"), opBytes(good), opBytes(k), null)
	zero := opCall(t, blockEncryptFn, opStr("aes-ecb-nopad"), opBytes(make([]byte, 16)), opBytes(k), null).Bytes
	opErr(t, "bad padding", blockDecryptFn, opStr("aes-ecb-pkcs7"), opBytes(zero), opBytes(k), null)
	opErr(t, "bad padding", blockDecryptFn, opStr("aes-ecb-pkcs7"), opBytes(nil), opBytes(k), null)
	// A full block of padding is added when the input is already aligned.
	full := opCall(t, blockEncryptFn, opStr("aes-ecb-pkcs7"), opBytes(make([]byte, 16)), opBytes(k), null).Bytes
	if len(full) != 32 {
		t.Errorf("pkcs7 of 16 bytes is %d bytes, want 32", len(full))
	}
}

// The encrypt-then-MAC recipe from crypto.md (MAC over iv || ciphertext,
// checked before decrypting) catches a flipped bit before any decryption runs.
func TestEncryptThenMacRecipe(t *testing.T) {
	// Covered as a Jennifer program in the docs and examples; here the block
	// layer's part: a flipped ciphertext bit decrypts to garbage or a padding
	// error, which is why the MAC must be checked first.
	k, iv := bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 16)
	ct := opCall(t, blockEncryptFn, opStr("aes-cbc-pkcs7"), opBytes([]byte("attack at dawn")), opBytes(k), opBytes(iv)).Bytes
	ct[len(ct)-1] ^= 0x01
	if v, err := blockDecryptFn(interpreter.BuiltinCtx{}, []interpreter.Value{opStr("aes-cbc-pkcs7"), opBytes(ct), opBytes(k), opBytes(iv)}); err == nil && string(v.Bytes) == "attack at dawn" {
		t.Error("a tampered CBC ciphertext decrypted to the original plaintext")
	}
}

// ----- sign / verify ----------------------------------------------------

func TestKatEd25519(t *testing.T) {
	// RFC 8032 section 7.1, test 1 (empty message).
	seed := opHex(t, "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	pub := opHex(t, "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
	want := "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"
	priv := append(append([]byte(nil), seed...), pub...)
	sig := opCall(t, signFn, opStr("ed25519"), opBytes(nil), opBytes(priv)).Bytes
	if hex.EncodeToString(sig) != want {
		t.Errorf("ed25519 sign = %x", sig)
	}
	if !opCall(t, verifyFn, opStr("ed25519"), opBytes(nil), opBytes(sig), opBytes(pub)).Bool {
		t.Error("ed25519 verify of the RFC vector is false")
	}
	if opCall(t, verifyFn, opStr("ed25519"), opBytes([]byte("x")), opBytes(sig), opBytes(pub)).Bool {
		t.Error("ed25519 verify of another message is true")
	}
	if got := opCall(t, publicKeyFn, opStr("ed25519"), opBytes(priv)).Bytes; !bytes.Equal(got, pub) {
		t.Errorf("publicKey(ed25519) = %x", got)
	}
	bad := append([]byte(nil), priv...)
	bad[40] ^= 1
	opErr(t, "public half does not match its seed", signFn, opStr("ed25519"), opBytes(nil), opBytes(bad))
	opErr(t, "ed25519 public key is 32 raw bytes", verifyFn, opStr("ed25519"), opBytes(nil), opBytes(sig), opBytes(pub[:31]))
}

func TestKatRSASignatures(t *testing.T) {
	priv, pub, msg := opFile(t, "rsa.pem"), opFile(t, "rsa.pub"), opFile(t, "msg")
	for _, h := range []string{"sha256", "sha384", "sha512"} {
		algo := "rsa-" + h
		// PKCS#1 v1.5 is deterministic: Go must produce OpenSSL's exact bytes.
		want := opFile(t, "rsa-"+h+".sig")
		got := opCall(t, signFn, opStr(algo), opBytes(msg), opBytes(priv)).Bytes
		if !bytes.Equal(got, want) {
			t.Errorf("%s: signature differs from openssl dgst -sign", algo)
		}
		if !opCall(t, verifyFn, opStr(algo), opBytes(msg), opBytes(want), opBytes(pub)).Bool {
			t.Errorf("%s: openssl signature does not verify", algo)
		}
	}
	// The hash is part of the name: a sha256 signature does not verify as sha384.
	sig := opFile(t, "rsa-sha256.sig")
	if opCall(t, verifyFn, opStr("rsa-sha384"), opBytes(msg), opBytes(sig), opBytes(pub)).Bool {
		t.Error("rsa-sha256 signature verified as rsa-sha384")
	}
}

func TestKatECDSASignatures(t *testing.T) {
	msg := opFile(t, "msg")
	cases := []struct{ key, hash string }{{"ec256", "sha256"}, {"ec384", "sha384"}, {"ec256", "sha512"}}
	for _, c := range cases {
		algo := "ecdsa-" + c.hash
		priv, pub := opFile(t, c.key+".pem"), opFile(t, c.key+".pub")
		node := opFile(t, "ecdsa-"+c.key+"-"+c.hash+".sig")
		if !opCall(t, verifyFn, opStr(algo), opBytes(msg), opBytes(node), opBytes(pub)).Bool {
			t.Errorf("%s/%s: Node's R||S signature does not verify", c.key, algo)
		}
		own := opCall(t, signFn, opStr(algo), opBytes(msg), opBytes(priv)).Bytes
		if len(own) != len(node) {
			t.Errorf("%s/%s: signature is %d bytes, Node's %d", c.key, algo, len(own), len(node))
		}
		if !opCall(t, verifyFn, opStr(algo), opBytes(msg), opBytes(own), opBytes(pub)).Bool {
			t.Errorf("%s/%s: own signature does not verify", c.key, algo)
		}
		if opCall(t, verifyFn, opStr(algo), opBytes(msg), opBytes(own[:10]), opBytes(pub)).Bool {
			t.Errorf("%s/%s: a truncated signature verified", c.key, algo)
		}
	}
}

func TestSignRejectsWrongKeyType(t *testing.T) {
	msg := []byte("m")
	opErr(t, "could not parse an RSA private key", signFn, opStr("rsa-sha256"), opBytes(msg), opBytes(opFile(t, "ec256.pem")))
	opErr(t, "non-ECDSA private key", signFn, opStr("ecdsa-sha256"), opBytes(msg), opBytes(opFile(t, "rsa.pem")))
	opErr(t, "ed25519 private key is 64 raw bytes", signFn, opStr("ed25519"), opBytes(msg), opBytes(opFile(t, "rsa.pem")))
	opErr(t, "key is not valid PEM", signFn, opStr("rsa-sha256"), opBytes(msg), opBytes(make([]byte, 64)))
	opErr(t, `unknown algorithm "rs256"`, signFn, opStr("rs256"), opBytes(msg), opBytes(nil))
	opErr(t, `unknown algorithm "hmac-sha256"`, verifyFn, opStr("hmac-sha256"), opBytes(msg), opBytes(nil), opBytes(nil))
}

// ----- keypair / publicKey --------------------------------------------------

func TestKeypairForms(t *testing.T) {
	for _, algo := range []string{"ed25519", "x25519", "mlkem768"} {
		kp := opCall(t, keypairFn, opStr(algo))
		pub, priv := opField(kp, "public"), opField(kp, "private")
		if bytes.HasPrefix(priv, []byte("-----")) || len(pub) == 0 {
			t.Errorf("%s: want raw bytes", algo)
		}
		if got := opCall(t, publicKeyFn, opStr(algo), opBytes(priv)).Bytes; !bytes.Equal(got, pub) {
			t.Errorf("%s: publicKey(private) differs from keypair's public", algo)
		}
	}
	for _, c := range []struct{ algo, privHead string }{
		{"rsa2048", "-----BEGIN PRIVATE KEY-----"}, {"p256", "-----BEGIN EC PRIVATE KEY-----"},
		{"p384", "-----BEGIN EC PRIVATE KEY-----"}, {"p521", "-----BEGIN EC PRIVATE KEY-----"},
	} {
		kp := opCall(t, keypairFn, opStr(c.algo))
		pub, priv := opField(kp, "public"), opField(kp, "private")
		if !bytes.HasPrefix(priv, []byte(c.privHead)) || !bytes.HasPrefix(pub, []byte("-----BEGIN PUBLIC KEY-----")) {
			t.Errorf("%s: private %q / public %q", c.algo, priv[:30], pub[:26])
		}
		if got := opCall(t, publicKeyFn, opStr(c.algo), opBytes(priv)).Bytes; !bytes.Equal(got, pub) {
			t.Errorf("%s: publicKey(private) differs from keypair's public", c.algo)
		}
	}
	// A generated key signs, and the matching public key verifies.
	kp := opCall(t, keypairFn, opStr("p384"))
	sig := opCall(t, signFn, opStr("ecdsa-sha384"), opBytes([]byte("m")), opBytes(opField(kp, "private"))).Bytes
	if !opCall(t, verifyFn, opStr("ecdsa-sha384"), opBytes([]byte("m")), opBytes(sig), opBytes(opField(kp, "public"))).Bool {
		t.Error("p384 keypair does not sign and verify")
	}
	// publicKey checks the key matches the algorithm's size or curve.
	opErr(t, `"rsa3072" needs a 3072-bit RSA key, got 2048 bits`, publicKeyFn, opStr("rsa3072"), opBytes(opFile(t, "rsa.pem")))
	opErr(t, `"p384" needs a key on that curve`, publicKeyFn, opStr("p384"), opBytes(opFile(t, "ec256.pem")))
	opErr(t, `unknown algorithm "rsa1024"`, keypairFn, opStr("rsa1024"))
	opErr(t, `unknown algorithm "secp256k1"`, publicKeyFn, opStr("secp256k1"), opBytes(nil))
}

// ----- agree / encapsulate / decapsulate --------------------------------

func TestKatX25519(t *testing.T) {
	// RFC 7748 section 6.1.
	alicePriv := opHex(t, "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a")
	alicePub := opHex(t, "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a")
	bobPriv := opHex(t, "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb")
	bobPub := opHex(t, "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f")
	want := "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742"
	if got := opCall(t, agreeFn, opStr("x25519"), opBytes(alicePriv), opBytes(bobPub)).Bytes; hex.EncodeToString(got) != want {
		t.Errorf("alice's secret = %x", got)
	}
	if got := opCall(t, agreeFn, opStr("x25519"), opBytes(bobPriv), opBytes(alicePub)).Bytes; hex.EncodeToString(got) != want {
		t.Errorf("bob's secret = %x", got)
	}
	if got := opCall(t, publicKeyFn, opStr("x25519"), opBytes(alicePriv)).Bytes; !bytes.Equal(got, alicePub) {
		t.Errorf("publicKey(x25519) = %x", got)
	}
	opErr(t, "low-order point", agreeFn, opStr("x25519"), opBytes(alicePriv), opBytes(make([]byte, 32)))
	opErr(t, "peer public key is 32 raw bytes", agreeFn, opStr("x25519"), opBytes(alicePriv), opBytes(bobPub[:31]))
	opErr(t, `unknown algorithm "p256"; known: "x25519"`, agreeFn, opStr("p256"), opBytes(alicePriv), opBytes(bobPub))
}

func TestMLKEMRoundTrip(t *testing.T) {
	kp := opCall(t, keypairFn, opStr("mlkem768"))
	enc := opCall(t, encapsulateFn, opStr("mlkem768"), opBytes(opField(kp, "public")))
	ct, secret := opField(enc, "ciphertext"), opField(enc, "sharedSecret")
	if len(ct) != 1088 || len(secret) != 32 {
		t.Fatalf("ciphertext %d bytes, secret %d", len(ct), len(secret))
	}
	got := opCall(t, decapsulateFn, opStr("mlkem768"), opBytes(ct), opBytes(opField(kp, "private"))).Bytes
	if !bytes.Equal(got, secret) {
		t.Error("decapsulated secret differs")
	}
	opErr(t, `unknown algorithm "mlkem1024"`, encapsulateFn, opStr("mlkem1024"), opBytes(nil))
	opErr(t, "invalid mlkem768 public key", encapsulateFn, opStr("mlkem768"), opBytes(make([]byte, 10)))
}

// ----- wrap / unwrap ------------------------------------------------------

func TestKatRSAOAEP(t *testing.T) {
	priv, pub := opFile(t, "rsa.pem"), opFile(t, "rsa.pub")
	const plain = "a 32-byte key wrapped for you!!!"
	for _, h := range []string{"sha1", "sha256"} {
		algo := "rsa-oaep-" + h
		// Node (OpenSSL) wrapped it; Go unwraps it.
		got := opCall(t, unwrapFn, opStr(algo), opBytes(opFile(t, "oaep-"+h+".bin")), opBytes(priv)).Bytes
		if string(got) != plain {
			t.Errorf("%s: unwrap of Node's ciphertext = %q", algo, got)
		}
		// Round trip through Go.
		ct := opCall(t, wrapFn, opStr(algo), opBytes([]byte(plain)), opBytes(pub)).Bytes
		if back := opCall(t, unwrapFn, opStr(algo), opBytes(ct), opBytes(priv)).Bytes; string(back) != plain {
			t.Errorf("%s: round trip = %q", algo, back)
		}
	}
	// The hash is part of the name; one error for every failure (no oracle).
	opErr(t, "decryption failed", unwrapFn, opStr("rsa-oaep-sha256"), opBytes(opFile(t, "oaep-sha1.bin")), opBytes(priv))
	opErr(t, "decryption failed", unwrapFn, opStr("rsa-oaep-sha1"), opBytes(make([]byte, 256)), opBytes(priv))
	opErr(t, `unknown algorithm "rsa-oaep-sha512"`, wrapFn, opStr("rsa-oaep-sha512"), opBytes(nil), opBytes(pub))
	opErr(t, "non-RSA public key", wrapFn, opStr("rsa-oaep-sha256"), opBytes([]byte("k")), opBytes(opFile(t, "ec256.pub")))
}
