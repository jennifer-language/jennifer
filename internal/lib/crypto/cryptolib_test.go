// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

var noCtx = interpreter.BuiltinCtx{}

func bytesArg(b []byte) interpreter.Value { return interpreter.BytesVal(b) }

func TestRandBytesLengthAndBounds(t *testing.T) {
	for _, n := range []int64{0, 1, 16, 512, 513} {
		v, err := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(n)})
		if err != nil {
			t.Fatalf("randBytes(%d): %v", n, err)
		}
		if v.Kind != interpreter.KindBytes || int64(len(v.Bytes)) != n {
			t.Errorf("randBytes(%d): got %d bytes", n, len(v.Bytes))
		}
	}
	// Two draws of a decent width almost never collide.
	a, _ := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(32)})
	b, _ := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(32)})
	if string(a.Bytes) == string(b.Bytes) {
		t.Error("two 32-byte draws were identical")
	}
	if _, err := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(-1)}); err == nil {
		t.Error("negative n should error")
	}
	if _, err := randBytesFn(noCtx, []interpreter.Value{interpreter.StringVal("x")}); err == nil {
		t.Error("non-int n should error")
	}
	// An over-limit n must be a catchable error, never an unbounded make (the
	// interpreter has no recover(), so make([]byte, huge) is a fatal crash).
	if _, err := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(maxRandBytes + 1)}); err == nil {
		t.Error("over-limit n should error, not allocate")
	}
	// The cap itself is still allowed.
	if _, err := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(maxRandBytes)}); err != nil {
		t.Errorf("n == maxRandBytes should be allowed: %v", err)
	}
}

// PBKDF2's keyLen and iterations must be bounded so an untrusted value (e.g. a
// hostile SCRAM server's i=) cannot allocate gigabytes or spin a core forever;
// both should surface as a catchable error, and sane values still derive a key.
func TestPBKDFBounds(t *testing.T) {
	pw := interpreter.BytesVal([]byte("password"))
	salt := interpreter.BytesVal([]byte("saltsalt"))
	algo := interpreter.StringVal("sha256")
	call := func(iter, keyLen int64) error {
		_, err := pbkdf2Fn(noCtx, []interpreter.Value{algo, pw, salt, interpreter.IntVal(iter), interpreter.IntVal(keyLen)})
		return err
	}
	if call(1000, maxKeyLen+1) == nil {
		t.Error("over-limit keyLen should error")
	}
	// The work bound is on blocks*iterations, not either factor alone: a large
	// keyLen paired with a huge iteration count (individually plausible) is the
	// days-of-CPU case and must be rejected before it runs.
	if call(maxPBKDFWork, maxKeyLen) == nil {
		t.Error("keyLen*iterations over the work limit should error")
	}
	// sha256 = 32-byte blocks: 32768 blocks at 1MiB, so >maxPBKDFWork/32768 iters must fail.
	if call(maxPBKDFWork/32768+1, maxKeyLen) == nil {
		t.Error("work just over the limit should error")
	}
	// A modest single-block derivation (well under the work budget) is allowed
	// and cheap; the full-budget allowed case is ~20s of real work, so it is not
	// exercised here.
	if err := call(100000, 32); err != nil {
		t.Errorf("sane pbkdf params should derive a key: %v", err)
	}
}

// crypto.hkdf must reject an over-large length before the int() cast so a
// 32-bit target cannot truncate a huge length to a small accepted value and
// return a shorter-than-requested key.
func TestHKDFLengthBound(t *testing.T) {
	secret := interpreter.BytesVal([]byte("secret"))
	empty := interpreter.BytesVal(nil)
	algo := interpreter.StringVal("sha256")
	if _, err := hkdfFn(noCtx, []interpreter.Value{algo, secret, empty, empty, interpreter.IntVal(maxKeyLen + 1)}); err == nil {
		t.Error("over-limit hkdf length should error")
	}
	if _, err := hkdfFn(noCtx, []interpreter.Value{algo, secret, empty, empty, interpreter.IntVal(32)}); err != nil {
		t.Errorf("sane hkdf length should derive a key: %v", err)
	}
}

// RandFill fills a whole slice in one lock acquisition and yields distinct
// output across calls; the buffer refills correctly across its 512-byte edge.
func TestRandFill(t *testing.T) {
	var a, b [40]byte
	RandFill(a[:])
	RandFill(b[:])
	if a == b {
		t.Error("two RandFill draws were identical")
	}
	// A draw larger than the internal buffer must still fill completely.
	big := make([]byte, len(randBuf)*2+7)
	RandFill(big)
	allZero := true
	for _, x := range big {
		if x != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("RandFill left an over-buffer-sized slice all zero")
	}
}

func TestRandIntUniformAndBounds(t *testing.T) {
	// Every value in a small inclusive range appears over enough draws, and
	// none falls outside it.
	seen := map[int64]bool{}
	for i := 0; i < 4000; i++ {
		v, err := randIntFn(noCtx, []interpreter.Value{interpreter.IntVal(3), interpreter.IntVal(9)})
		if err != nil {
			t.Fatal(err)
		}
		if v.Int < 3 || v.Int > 9 {
			t.Fatalf("randInt out of range: %d", v.Int)
		}
		seen[v.Int] = true
	}
	for want := int64(3); want <= 9; want++ {
		if !seen[want] {
			t.Errorf("value %d never drawn over 4000 tries", want)
		}
	}
	// lo == hi is a fixed point; lo > hi errors.
	v, _ := randIntFn(noCtx, []interpreter.Value{interpreter.IntVal(5), interpreter.IntVal(5)})
	if v.Int != 5 {
		t.Errorf("randInt(5,5) = %d, want 5", v.Int)
	}
	if _, err := randIntFn(noCtx, []interpreter.Value{interpreter.IntVal(9), interpreter.IntVal(3)}); err == nil {
		t.Error("lo > hi should error")
	}
}

// The full int64 span must not overflow or spin forever (mod == full-range
// fast path).
func TestRandIntFullRangeDoesNotHang(t *testing.T) {
	for i := 0; i < 100; i++ {
		if _, err := randIntFn(noCtx, []interpreter.Value{interpreter.IntVal(-9223372036854775808), interpreter.IntVal(9223372036854775807)}); err != nil {
			t.Fatal(err)
		}
	}
}

// RFC 5869 Appendix A.1 (HKDF-SHA256, Test Case 1).
func TestHkdfKnownAnswer(t *testing.T) {
	ikm := bytesRepeat(0x0b, 22)
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantHex := "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"
	v, err := hkdfFn(noCtx, []interpreter.Value{interpreter.StringVal("sha256"), bytesArg(ikm), bytesArg(salt), bytesArg(info), interpreter.IntVal(42)})
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(v.Bytes); got != wantHex {
		t.Errorf("hkdf =\n %s\nwant\n %s", got, wantHex)
	}
	if _, err := hkdfFn(noCtx, []interpreter.Value{interpreter.StringVal("sha256"), bytesArg(ikm), bytesArg(salt), bytesArg(info), interpreter.IntVal(0)}); err == nil {
		t.Error("length 0 should error")
	}
	if _, err := hkdfFn(noCtx, []interpreter.Value{interpreter.StringVal("md5"), bytesArg(ikm), bytesArg(salt), bytesArg(info), interpreter.IntVal(42)}); err == nil {
		t.Error("md5 is not a KDF algorithm and should error")
	}
	// RFC 5869 Appendix A.4 (HKDF-SHA1, Test Case 4) - pins the sha1 PRF path.
	ikm1 := bytesRepeat(0x0b, 11)
	want1 := "085a01ea1b10f36933068b56efa5ad81a4f14b822f5b091568a9cdd4f155fda2c22e422478d305f3f896"
	v1, err := hkdfFn(noCtx, []interpreter.Value{interpreter.StringVal("sha1"), bytesArg(ikm1), bytesArg(salt), bytesArg(info), interpreter.IntVal(42)})
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(v1.Bytes); got != want1 {
		t.Errorf("hkdf(sha1) =\n %s\nwant\n %s", got, want1)
	}
}

// Standard PBKDF2 vectors: SHA-256 (P="password", S="salt", c=1, dkLen=32) and
// SHA-1 (RFC 6070 case 1, dkLen=20) - the latter is what SCRAM-SHA-1 needs.
func TestPbkdf2KnownAnswer(t *testing.T) {
	cases := []struct {
		algo         string
		iter, keyLen int64
		wantHex      string
	}{
		{"sha256", 1, 32, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"sha1", 1, 20, "0c60c80f961f0e71f3a9b524af6012062fe037a6"},
		{"sha512", 1, 64, "867f70cf1ade02cff3752599a3a53dc4af34c7a669815ae5d513554e1c8cf252c02d470a285a0501bad999bfe943c08f050235d7d68b1da55e63f73b60a57fce"},
	}
	for _, c := range cases {
		v, err := pbkdf2Fn(noCtx, []interpreter.Value{interpreter.StringVal(c.algo), bytesArg([]byte("password")), bytesArg([]byte("salt")), interpreter.IntVal(c.iter), interpreter.IntVal(c.keyLen)})
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(v.Bytes); got != c.wantHex {
			t.Errorf("pbkdf(%s) =\n %s\nwant\n %s", c.algo, got, c.wantHex)
		}
	}
	if _, err := pbkdf2Fn(noCtx, []interpreter.Value{interpreter.StringVal("sha256"), bytesArg([]byte("p")), bytesArg([]byte("s")), interpreter.IntVal(0), interpreter.IntVal(32)}); err == nil {
		t.Error("iterations 0 should error")
	}
}

// RandByte shares the refill buffer with RandFill (the source uuid draws on),
// and a spawned task can generate UUIDs concurrently with the main goroutine,
// so the shared buffer must be race-free. Run under -race.
func TestRandByteConcurrent(t *testing.T) {
	const goroutines, draws = 16, 2000
	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			var acc byte
			for i := 0; i < draws; i++ {
				b, err := RandByte()
				if err != nil {
					t.Error(err)
					break
				}
				acc ^= b
			}
			_ = acc
			done <- struct{}{}
		}()
	}
	for g := 0; g < goroutines; g++ {
		<-done
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func strArg(s string) interpreter.Value { return interpreter.StringVal(s) }

// genKeypair calls crypto.keypair(algo) and returns the public and private
// halves, failing the test on an error or a struct of the wrong shape.
func genKeypair(t *testing.T, algo string) (pub, priv []byte) {
	t.Helper()
	v, err := keypairFn(noCtx, []interpreter.Value{strArg(algo)})
	if err != nil {
		t.Fatalf("keypair(%q): %v", algo, err)
	}
	if v.StructName != "Keypair" || len(v.Fields) != 2 {
		t.Fatalf("keypair(%q): got %s, want a crypto.Keypair", algo, v.Display())
	}
	for _, f := range v.Fields {
		switch f.Name {
		case "public":
			pub = f.Value.Bytes
		case "private":
			priv = f.Value.Bytes
		}
	}
	return pub, priv
}

// wantErr fails unless err is non-nil and its message contains every want.
func wantErr(t *testing.T, what string, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: no error, want one containing %q", what, want)
		return
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%s: error %q, want it to contain %q", what, err, w)
		}
	}
}

// ----- AES-GCM: encrypt / decrypt -----

func gcmSeal(key, pt, nonce, aad []byte) (interpreter.Value, error) {
	return encryptFn(noCtx, []interpreter.Value{strArg("aes-gcm"), bytesArg(pt), bytesArg(key), bytesArg(nonce), bytesArg(aad)})
}

func gcmOpen(key, box, nonce, aad []byte) (interpreter.Value, error) {
	return decryptFn(noCtx, []interpreter.Value{strArg("aes-gcm"), bytesArg(box), bytesArg(key), bytesArg(nonce), bytesArg(aad)})
}

func mustGCMSeal(t *testing.T, key, pt, nonce, aad []byte) []byte {
	t.Helper()
	v, err := gcmSeal(key, pt, nonce, aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return v.Bytes
}

// NIST AES-256-GCM known-answer vectors (GCM spec test cases 13/14: zero key,
// zero 96-bit IV, no AAD). The box is ciphertext||tag with no nonce in it, so
// encrypt must produce exactly the published bytes and decrypt must accept
// them: the box *layout* is pinned against external truth, not just against
// our own encrypt.
func TestGCMKnownAnswer(t *testing.T) {
	key := make([]byte, 32)
	iv := make([]byte, 12)
	// Case 13: empty plaintext -> tag only.
	tag13, _ := hex.DecodeString("530f8afbc74536b9a963b4f1c4cb738b")
	if got := mustGCMSeal(t, key, nil, iv, nil); !bytes.Equal(got, tag13) {
		t.Errorf("NIST case 13 encrypt = %x, want %x", got, tag13)
	}
	v, err := gcmOpen(key, tag13, iv, nil)
	if err != nil || len(v.Bytes) != 0 {
		t.Errorf("NIST case 13: got %x err=%v, want empty plaintext", v.Bytes, err)
	}
	// Case 14: 16 zero bytes of plaintext.
	ct14, _ := hex.DecodeString("cea7403d4d606b6e074ec5d3baf39d18")
	tag14, _ := hex.DecodeString("d0d1c8a799996bf0265b98b5d48ab919")
	box := append(append([]byte{}, ct14...), tag14...)
	if got := mustGCMSeal(t, key, make([]byte, 16), iv, nil); !bytes.Equal(got, box) {
		t.Errorf("NIST case 14 encrypt = %x, want %x", got, box)
	}
	v, err = gcmOpen(key, box, iv, nil)
	if err != nil || !bytes.Equal(v.Bytes, make([]byte, 16)) {
		t.Errorf("NIST case 14: got %x err=%v, want 16 zero bytes", v.Bytes, err)
	}
	// A box shorter than the tag (here empty, and 15 bytes) must fail cleanly.
	for _, short := range [][]byte{nil, tag13[:15]} {
		_, err := gcmOpen(key, short, iv, nil)
		wantErr(t, "short box", err, "authentication failed")
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	nonce := []byte("twelve bytes")
	aad := []byte("header")
	for _, n := range []int{16, 24, 32} {
		key := make([]byte, n)
		for i := range key {
			key[i] = byte(i)
		}
		for _, pt := range [][]byte{{}, []byte("x"), []byte("a longer secret message here")} {
			for _, a := range [][]byte{nil, aad} {
				box := mustGCMSeal(t, key, pt, nonce, a)
				// box = ciphertext (len(pt)) + 16 tag, no nonce
				if len(box) != len(pt)+16 {
					t.Errorf("AES-%d: box len = %d, want %d", n*8, len(box), len(pt)+16)
				}
				v, err := gcmOpen(key, box, nonce, a)
				if err != nil || !bytes.Equal(v.Bytes, pt) {
					t.Errorf("AES-%d decrypt: %q err=%v, want %q", n*8, v.Bytes, err, pt)
				}
			}
		}
	}
	// The caller owns the nonce: the same inputs seal to the same box, and
	// another nonce to another box.
	key := make([]byte, 32)
	a := mustGCMSeal(t, key, []byte("same"), nonce, nil)
	if !bytes.Equal(a, mustGCMSeal(t, key, []byte("same"), nonce, nil)) {
		t.Error("encrypt is not deterministic for a fixed key, nonce and aad")
	}
	if bytes.Equal(a, mustGCMSeal(t, key, []byte("same"), []byte("other nonce!"), nil)) {
		t.Error("two nonces produced the same box")
	}
}

// A wrong key, nonce or aad, or any tampered byte, is the one authentication
// error, never a partial plaintext.
func TestDecryptRejectsTamperAndWrongKey(t *testing.T) {
	key := make([]byte, 32)
	nonce := make([]byte, 12)
	aad := []byte("aad")
	box := mustGCMSeal(t, key, []byte("authentic"), nonce, aad)
	if _, err := gcmOpen(key, box, nonce, aad); err != nil {
		t.Fatalf("genuine box: %v", err)
	}
	flip := func(b []byte, i int) []byte {
		out := append([]byte(nil), b...)
		out[i] ^= 0x01
		return out
	}
	cases := []struct {
		name                 string
		key, box, nonce, aad []byte
	}{
		{"tampered ciphertext", key, flip(box, 0), nonce, aad},
		{"tampered tag", key, flip(box, len(box)-1), nonce, aad},
		{"truncated box", key, box[:len(box)-1], nonce, aad},
		{"wrong key", flip(key, 0), box, nonce, aad},
		{"wrong nonce", key, box, flip(nonce, 11), aad},
		{"wrong aad", key, box, nonce, []byte("aaD")},
		{"missing aad", key, box, nonce, nil},
	}
	for _, c := range cases {
		v, err := gcmOpen(c.key, c.box, c.nonce, c.aad)
		wantErr(t, c.name, err, "crypto.decrypt: authentication failed")
		if v.Kind == interpreter.KindBytes {
			t.Errorf("%s: returned bytes alongside the error", c.name)
		}
	}
}

func TestEncryptKeyAndNonceLength(t *testing.T) {
	pt := []byte("x")
	nonce := make([]byte, 12)
	for _, n := range []int{0, 1, 15, 17, 31, 33, 64} {
		_, err := gcmSeal(make([]byte, n), pt, nonce, nil)
		wantErr(t, "encrypt key", err, "key must be 16, 24 or 32 bytes")
		_, err = gcmOpen(make([]byte, n), make([]byte, 40), nonce, nil)
		wantErr(t, "decrypt key", err, "key must be 16, 24 or 32 bytes")
	}
	for _, n := range []int{0, 8, 11, 13, 16} {
		_, err := gcmSeal(make([]byte, 32), pt, make([]byte, n), nil)
		wantErr(t, "encrypt nonce", err, "nonce must be 12 bytes")
		_, err = gcmOpen(make([]byte, 32), make([]byte, 40), make([]byte, n), nil)
		wantErr(t, "decrypt nonce", err, "nonce must be 12 bytes")
	}
	// Argument kinds and arity.
	key := bytesArg(make([]byte, 32))
	for name, args := range map[string][]interpreter.Value{
		"string key":    {strArg("aes-gcm"), bytesArg(pt), strArg("k"), bytesArg(nonce), bytesArg(nil)},
		"null aad":      {strArg("aes-gcm"), bytesArg(pt), key, bytesArg(nonce), interpreter.Null()},
		"bytes algo":    {bytesArg([]byte("aes-gcm")), bytesArg(pt), key, bytesArg(nonce), bytesArg(nil)},
		"old two-arg":   {key, bytesArg(pt)},
		"missing aad":   {strArg("aes-gcm"), bytesArg(pt), key, bytesArg(nonce)},
		"one too many":  {strArg("aes-gcm"), bytesArg(pt), key, bytesArg(nonce), bytesArg(nil), bytesArg(nil)},
		"no arguments":  nil,
		"int plaintext": {strArg("aes-gcm"), interpreter.IntVal(1), key, bytesArg(nonce), bytesArg(nil)},
	} {
		if _, err := encryptFn(noCtx, args); err == nil {
			t.Errorf("encrypt %s: no error", name)
		}
		if _, err := decryptFn(noCtx, args); err == nil {
			t.Errorf("decrypt %s: no error", name)
		}
	}
}

// An algorithm name the operation does not know is an error that lists the
// names it does.
func TestEncryptUnknownAlgorithm(t *testing.T) {
	key, nonce := make([]byte, 32), make([]byte, 12)
	for _, algo := range []string{"aes-256-gcm", "AES-GCM", "xsalsa20-poly1305", ""} {
		args := []interpreter.Value{strArg(algo), bytesArg([]byte("x")), bytesArg(key), bytesArg(nonce), bytesArg(nil)}
		_, err := encryptFn(noCtx, args)
		wantErr(t, "encrypt "+algo, err, "crypto.encrypt: unknown algorithm", `known: "aes-gcm", "chacha20-poly1305", "xchacha20-poly1305"`)
		_, err = decryptFn(noCtx, args)
		wantErr(t, "decrypt "+algo, err, "crypto.decrypt: unknown algorithm", `known: "aes-gcm", "chacha20-poly1305", "xchacha20-poly1305"`)
	}
}

// ----- Ed25519 -----

func edSign(priv, msg []byte) (interpreter.Value, error) {
	return signFn(noCtx, []interpreter.Value{strArg("ed25519"), bytesArg(msg), bytesArg(priv)})
}

func edVerify(pub, msg, sig []byte) (interpreter.Value, error) {
	return verifyFn(noCtx, []interpreter.Value{strArg("ed25519"), bytesArg(msg), bytesArg(sig), bytesArg(pub)})
}

// RFC 8032 section 7.1, TEST 2: a fixed keypair and one-byte message with a
// published signature, so sign / verify / publicKey are pinned against
// external truth.
func TestEd25519KnownAnswer(t *testing.T) {
	seed, _ := hex.DecodeString("4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb")
	pub, _ := hex.DecodeString("3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c")
	wantSig, _ := hex.DecodeString("92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00")
	msg := []byte{0x72}
	priv := append(append([]byte{}, seed...), pub...) // Go's private key is seed||public
	sv, err := edSign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if !bytes.Equal(sv.Bytes, wantSig) {
		t.Errorf("sign =\n %x\nwant\n %x", sv.Bytes, wantSig)
	}
	if v, err := edVerify(pub, msg, wantSig); err != nil || !v.Bool {
		t.Errorf("RFC 8032 signature should verify true (err %v)", err)
	}
	pv, err := publicKeyFn(noCtx, []interpreter.Value{strArg("ed25519"), bytesArg(priv)})
	if err != nil || !bytes.Equal(pv.Bytes, pub) {
		t.Errorf("publicKey = %x, %v; want %x", pv.Bytes, err, pub)
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv := genKeypair(t, "ed25519")
	if len(pub) != 32 || len(priv) != 64 {
		t.Fatalf("key sizes: pub=%d priv=%d", len(pub), len(priv))
	}
	pv, err := publicKeyFn(noCtx, []interpreter.Value{strArg("ed25519"), bytesArg(priv)})
	if err != nil || !bytes.Equal(pv.Bytes, pub) {
		t.Errorf("publicKey does not match the keypair's public key (err %v)", err)
	}
	msg := []byte("sign this message")
	sigV, err := edSign(priv, msg)
	if err != nil || len(sigV.Bytes) != 64 {
		t.Fatalf("sign: %v (len %d)", err, len(sigV.Bytes))
	}
	// valid
	if v, _ := edVerify(pub, msg, sigV.Bytes); !v.Bool {
		t.Error("valid signature should verify true")
	}
	// tampered message
	if v, _ := edVerify(pub, []byte("sign that message"), sigV.Bytes); v.Bool {
		t.Error("tampered message should verify false")
	}
	// cross-key: another keypair's public must reject this signature
	pub2, _ := genKeypair(t, "ed25519")
	if v, _ := edVerify(pub2, msg, sigV.Bytes); v.Bool {
		t.Error("wrong public key should verify false")
	}
}

func TestEd25519LengthErrors(t *testing.T) {
	msg := []byte("m")
	_, err := edSign(make([]byte, 32), msg)
	wantErr(t, "sign with a 32-byte private key", err, "ed25519 private key is 64 raw bytes")
	_, err = publicKeyFn(noCtx, []interpreter.Value{strArg("ed25519"), bytesArg(make([]byte, 32))})
	wantErr(t, "publicKey with a 32-byte private key", err, "ed25519 private key is 64 raw bytes")
	// verify: a malformed public key is an error, not a panic
	_, err = edVerify(make([]byte, 10), msg, make([]byte, 64))
	wantErr(t, "verify with a short public key", err, "ed25519 public key is 32 raw bytes")
	// A signature of the wrong length is not a valid signature: false, not an
	// error (and not a panic).
	for _, n := range []int{0, 10, 63, 65} {
		v, err := edVerify(make([]byte, 32), msg, make([]byte, n))
		if err != nil || v.Kind != interpreter.KindBool || v.Bool {
			t.Errorf("verify with a %d-byte signature = %v (err %v), want false", n, v.Display(), err)
		}
	}
}

// Concurrent nonce draws must be distinct. crypto.encrypt now takes the nonce
// from the caller, and the documented way to make one is crypto.randBytes(12)
// per message, so a pool race handing two callers the same bytes would be GCM
// nonce reuse (a catastrophic key-recovery break). This stresses the shared
// random pool under -race and asserts uniqueness.
func TestConcurrentRandBytesDistinctNonces(t *testing.T) {
	const n = 300
	nonces := make([][]byte, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := randBytesFn(noCtx, []interpreter.Value{interpreter.IntVal(12)})
			if err != nil {
				t.Errorf("randBytes: %v", err)
				return
			}
			nonces[i] = v.Bytes
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, nc := range nonces {
		if nc == nil {
			continue
		}
		if seen[string(nc)] {
			t.Fatal("duplicate nonce across concurrent randBytes(12) draws - GCM nonce reuse")
		}
		seen[string(nc)] = true
	}
	if len(seen) != n {
		t.Errorf("got %d distinct nonces, want %d", len(seen), n)
	}
}

// An unavailable OS random source (jennifer-tiny without /dev/urandom) must be a
// catchable error from every random-drawing builtin - never a process abort and
// never predictable bytes - and the shared buffer must recover once the source
// comes back rather than serving the bytes a failed refill left behind.
func TestRandomSourceUnavailableIsCatchable(t *testing.T) {
	orig := randRead
	defer func() { randRead = orig }()
	randRead = func([]byte) (int, error) { return 0, errors.New("open /dev/urandom: no such file or directory") }
	randMu.Lock()
	randPos = len(randBuf) // force the next small draw to refill
	randMu.Unlock()

	for _, c := range []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
	}{
		{"randBytes", randBytesFn, []interpreter.Value{interpreter.IntVal(16)}},
		{"randInt", randIntFn, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(6)}},
		{"mtweiKeygen", mtweiKeygenFn, nil},
	} {
		_, err := c.fn(interpreter.BuiltinCtx{}, c.args)
		if err == nil || !strings.Contains(err.Error(), "secure random source unavailable") {
			t.Errorf("crypto.%s: got error %v, want a secure-random-source error", c.name, err)
		}
	}
	if err := RandFill(make([]byte, 16)); err == nil {
		t.Error("RandFill: want an error while the source is down")
	}

	randRead = orig
	b := make([]byte, 16)
	if err := RandFill(b); err != nil {
		t.Fatalf("RandFill after the source recovered: %v", err)
	}
	if bytes.Equal(b, make([]byte, 16)) {
		t.Error("RandFill after recovery returned all-zero bytes (served the failed refill)")
	}
}

// TestSignRejectsMismatchedPublicHalf pins that crypto.sign("ed25519", ...)
// (and publicKey) refuse a 64-byte key whose public half belongs to another
// seed: Ed25519 hashes that half into the signature, and two signatures under
// one seed with different halves leak the private scalar.
func TestSignRejectsMismatchedPublicHalf(t *testing.T) {
	_, a := genKeypair(t, "ed25519")
	_, b := genKeypair(t, "ed25519")
	mixed := append(append([]byte{}, a[:32]...), b[32:]...)
	_, err := edSign(mixed, []byte("m"))
	wantErr(t, "sign", err, "does not match its seed")
	_, err = publicKeyFn(noCtx, []interpreter.Value{strArg("ed25519"), bytesArg(mixed)})
	wantErr(t, "publicKey", err, "does not match its seed")
}

// Every operation rejects a name it does not know, and lists the ones it does.
func TestEveryOperationRejectsUnknownAlgorithm(t *testing.T) {
	b := bytesArg([]byte("x"))
	cases := []struct {
		fn    string
		call  interpreter.Builtin
		args  []interpreter.Value
		known string
	}{
		{"blockEncrypt", blockEncryptFn, []interpreter.Value{strArg("aes-xts"), b, b, interpreter.Null()}, `"aes-ecb-nopad", "aes-ecb-pkcs7", "aes-cbc-nopad", "aes-cbc-pkcs7", "aes-ctr", "aes-cfb"`},
		{"blockDecrypt", blockDecryptFn, []interpreter.Value{strArg("aes-xts"), b, b, interpreter.Null()}, `"aes-ctr", "aes-cfb"`},
		{"sign", signFn, []interpreter.Value{strArg("rsa-md5"), b, b}, `"ed25519", "rsa-sha256", "rsa-sha384", "rsa-sha512", "ecdsa-sha256", "ecdsa-sha384", "ecdsa-sha512"`},
		{"verify", verifyFn, []interpreter.Value{strArg("ed448"), b, b, b}, `"ed25519", "rsa-sha256"`},
		{"keypair", keypairFn, []interpreter.Value{strArg("rsa1024")}, `"ed25519", "x25519", "mlkem768", "rsa2048", "rsa3072", "rsa4096", "p256", "p384", "p521"`},
		{"publicKey", publicKeyFn, []interpreter.Value{strArg("secp256k1"), b}, `"p521"`},
		{"agree", agreeFn, []interpreter.Value{strArg("x448"), b, b}, `known: "x25519"`},
		{"encapsulate", encapsulateFn, []interpreter.Value{strArg("mlkem1024"), b}, `known: "mlkem768"`},
		{"decapsulate", decapsulateFn, []interpreter.Value{strArg("mlkem512"), b, b}, `known: "mlkem768"`},
		{"wrap", wrapFn, []interpreter.Value{strArg("rsa-pkcs1"), b, b}, `known: "rsa-oaep-sha1", "rsa-oaep-sha256"`},
		{"unwrap", unwrapFn, []interpreter.Value{strArg("rsa-oaep-sha512"), b, b}, `known: "rsa-oaep-sha1", "rsa-oaep-sha256"`},
	}
	for _, c := range cases {
		_, err := c.call(noCtx, c.args)
		wantErr(t, c.fn, err, "crypto."+c.fn+": unknown algorithm", c.known)
	}
}
