// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package cryptolib

import (
	"encoding/hex"
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
		_, err := pbkdf2Fn(noCtx, []interpreter.Value{pw, salt, interpreter.IntVal(iter), interpreter.IntVal(keyLen), algo})
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
	if _, err := hkdfFn(noCtx, []interpreter.Value{secret, empty, empty, interpreter.IntVal(maxKeyLen + 1), algo}); err == nil {
		t.Error("over-limit hkdf length should error")
	}
	if _, err := hkdfFn(noCtx, []interpreter.Value{secret, empty, empty, interpreter.IntVal(32), algo}); err != nil {
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

func TestHmacEqual(t *testing.T) {
	cases := []struct {
		a, b []byte
		want bool
	}{
		{[]byte("abcdef"), []byte("abcdef"), true},
		{[]byte("abcdef"), []byte("abcdeg"), false},
		{[]byte("abc"), []byte("abcdef"), false},
		{[]byte{}, []byte{}, true},
		{[]byte{}, []byte("x"), false},
	}
	for _, c := range cases {
		v, err := hmacEqualFn(noCtx, []interpreter.Value{bytesArg(c.a), bytesArg(c.b)})
		if err != nil {
			t.Fatal(err)
		}
		if v.Bool != c.want {
			t.Errorf("hmacEqual(%q,%q) = %t, want %t", c.a, c.b, v.Bool, c.want)
		}
	}
	if _, err := hmacEqualFn(noCtx, []interpreter.Value{interpreter.StringVal("a"), bytesArg([]byte("a"))}); err == nil {
		t.Error("non-bytes argument should error")
	}
}

// RFC 5869 Appendix A.1 (HKDF-SHA256, Test Case 1).
func TestHkdfKnownAnswer(t *testing.T) {
	ikm := bytesRepeat(0x0b, 22)
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantHex := "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"
	v, err := hkdfFn(noCtx, []interpreter.Value{bytesArg(ikm), bytesArg(salt), bytesArg(info), interpreter.IntVal(42), interpreter.StringVal("sha256")})
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(v.Bytes); got != wantHex {
		t.Errorf("hkdf =\n %s\nwant\n %s", got, wantHex)
	}
	if _, err := hkdfFn(noCtx, []interpreter.Value{bytesArg(ikm), bytesArg(salt), bytesArg(info), interpreter.IntVal(0), interpreter.StringVal("sha256")}); err == nil {
		t.Error("length 0 should error")
	}
	if _, err := hkdfFn(noCtx, []interpreter.Value{bytesArg(ikm), bytesArg(salt), bytesArg(info), interpreter.IntVal(42), interpreter.StringVal("md5")}); err == nil {
		t.Error("md5 is not a KDF algorithm and should error")
	}
	// RFC 5869 Appendix A.4 (HKDF-SHA1, Test Case 4) - pins the sha1 PRF path.
	ikm1 := bytesRepeat(0x0b, 11)
	want1 := "085a01ea1b10f36933068b56efa5ad81a4f14b822f5b091568a9cdd4f155fda2c22e422478d305f3f896"
	v1, err := hkdfFn(noCtx, []interpreter.Value{bytesArg(ikm1), bytesArg(salt), bytesArg(info), interpreter.IntVal(42), interpreter.StringVal("sha1")})
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
		v, err := pbkdf2Fn(noCtx, []interpreter.Value{bytesArg([]byte("password")), bytesArg([]byte("salt")), interpreter.IntVal(c.iter), interpreter.IntVal(c.keyLen), interpreter.StringVal(c.algo)})
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(v.Bytes); got != c.wantHex {
			t.Errorf("pbkdf(%s) =\n %s\nwant\n %s", c.algo, got, c.wantHex)
		}
	}
	if _, err := pbkdf2Fn(noCtx, []interpreter.Value{bytesArg([]byte("p")), bytesArg([]byte("s")), interpreter.IntVal(0), interpreter.IntVal(32), interpreter.StringVal("sha256")}); err == nil {
		t.Error("iterations 0 should error")
	}
}

// RandByte is the source uuid draws on, and a spawned task can generate UUIDs
// concurrently with the main goroutine, so the shared refill buffer must be
// race-free. Run under -race.
func TestRandByteConcurrent(t *testing.T) {
	const goroutines, draws = 16, 2000
	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			var acc byte
			for i := 0; i < draws; i++ {
				acc ^= RandByte()
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

// ----- AES-256-GCM -----

// NIST AES-256-GCM known-answer vectors (GCM spec test cases 13/14: zero key,
// zero 96-bit IV). Driven through decryptFn with a hand-assembled
// nonce||ciphertext||tag box, so the box *layout* is pinned against external
// truth, not just against our own encrypt.
func TestGCMKnownAnswer(t *testing.T) {
	key := make([]byte, 32)
	iv := make([]byte, 12)
	// Case 13: empty plaintext -> tag only.
	tag13, _ := hex.DecodeString("530f8afbc74536b9a963b4f1c4cb738b")
	box := append(append([]byte{}, iv...), tag13...)
	v, err := decryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(box)})
	if err != nil || len(v.Bytes) != 0 {
		t.Errorf("NIST case 13: got %x err=%v, want empty plaintext", v.Bytes, err)
	}
	// Case 14: 16 zero bytes of plaintext.
	ct14, _ := hex.DecodeString("cea7403d4d606b6e074ec5d3baf39d18")
	tag14, _ := hex.DecodeString("d0d1c8a799996bf0265b98b5d48ab919")
	box = append(append(append([]byte{}, iv...), ct14...), tag14...)
	v, err = decryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(box)})
	if err != nil || string(v.Bytes) != string(make([]byte, 16)) {
		t.Errorf("NIST case 14: got %x err=%v, want 16 zero bytes", v.Bytes, err)
	}
	// A box of exactly nonce size (no tag at all) must fail cleanly.
	if _, err := decryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(iv)}); err == nil {
		t.Error("nonce-only box should fail authentication")
	}
}

func mustEncrypt(t *testing.T, key, pt []byte) []byte {
	t.Helper()
	v, err := encryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(pt)})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return v.Bytes
}

func TestEncryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	for _, pt := range [][]byte{{}, []byte("x"), []byte("a longer secret message here")} {
		box := mustEncrypt(t, key, pt)
		// box = 12 nonce + len(pt) + 16 tag
		if len(box) != 12+len(pt)+16 {
			t.Errorf("box len = %d, want %d", len(box), 12+len(pt)+16)
		}
		v, err := decryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(box)})
		if err != nil || string(v.Bytes) != string(pt) {
			t.Errorf("decrypt: %q err=%v, want %q", v.Bytes, err, pt)
		}
	}
	// A fresh nonce each time: two encryptions of the same plaintext differ.
	if string(mustEncrypt(t, key, []byte("same"))) == string(mustEncrypt(t, key, []byte("same"))) {
		t.Error("two encryptions produced the same box (nonce reuse)")
	}
}

func TestDecryptRejectsTamperAndWrongKey(t *testing.T) {
	key := make([]byte, 32)
	box := mustEncrypt(t, key, []byte("authentic"))
	// tamper a ciphertext byte
	bad := append([]byte(nil), box...)
	bad[len(bad)-1] ^= 0x01
	if _, err := decryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(bad)}); err == nil {
		t.Error("tampered box should fail authentication")
	}
	// wrong key
	other := make([]byte, 32)
	other[0] = 0xff
	if _, err := decryptFn(noCtx, []interpreter.Value{bytesArg(other), bytesArg(box)}); err == nil {
		t.Error("wrong key should fail authentication")
	}
	// box too short to hold a nonce
	if _, err := decryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg([]byte{1, 2, 3})}); err == nil {
		t.Error("short box should error")
	}
}

func TestEncryptKeyLength(t *testing.T) {
	pt := []byte("x")
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := encryptFn(noCtx, []interpreter.Value{bytesArg(make([]byte, n)), bytesArg(pt)}); err == nil {
			t.Errorf("encrypt with a %d-byte key should error (need 32)", n)
		}
		if _, err := decryptFn(noCtx, []interpreter.Value{bytesArg(make([]byte, n)), bytesArg(make([]byte, 40))}); err == nil {
			t.Errorf("decrypt with a %d-byte key should error (need 32)", n)
		}
	}
	// non-bytes args
	if _, err := encryptFn(noCtx, []interpreter.Value{interpreter.StringVal("k"), bytesArg(pt)}); err == nil {
		t.Error("string key should error")
	}
}

// ----- Ed25519 -----

// RFC 8032 section 7.1, TEST 2: a fixed keypair and one-byte message with a
// published signature, so sign / verify are pinned against external truth.
func TestEd25519KnownAnswer(t *testing.T) {
	seed, _ := hex.DecodeString("4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb")
	pub, _ := hex.DecodeString("3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c")
	wantSig, _ := hex.DecodeString("92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00")
	msg := []byte{0x72}
	priv := append(append([]byte{}, seed...), pub...) // Go's private key is seed||public
	sv, err := signFn(noCtx, []interpreter.Value{bytesArg(priv), bytesArg(msg)})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if hex.EncodeToString(sv.Bytes) != hex.EncodeToString(wantSig) {
		t.Errorf("sign =\n %x\nwant\n %x", sv.Bytes, wantSig)
	}
	if v, _ := verifyFn(noCtx, []interpreter.Value{bytesArg(pub), bytesArg(msg), bytesArg(wantSig)}); !v.Bool {
		t.Error("RFC 8032 signature should verify true")
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	kpV, err := signKeypairFn(noCtx, nil)
	if err != nil {
		t.Fatalf("signKeypair: %v", err)
	}
	var pub, priv []byte
	for _, f := range kpV.Fields {
		switch f.Name {
		case "public":
			pub = f.Value.Bytes
		case "private":
			priv = f.Value.Bytes
		}
	}
	if len(pub) != 32 || len(priv) != 64 {
		t.Fatalf("key sizes: pub=%d priv=%d", len(pub), len(priv))
	}
	msg := []byte("sign this message")
	sigV, err := signFn(noCtx, []interpreter.Value{bytesArg(priv), bytesArg(msg)})
	if err != nil || len(sigV.Bytes) != 64 {
		t.Fatalf("sign: %v (len %d)", err, len(sigV.Bytes))
	}
	// valid
	if v, _ := verifyFn(noCtx, []interpreter.Value{bytesArg(pub), bytesArg(msg), bytesArg(sigV.Bytes)}); !v.Bool {
		t.Error("valid signature should verify true")
	}
	// tampered message
	if v, _ := verifyFn(noCtx, []interpreter.Value{bytesArg(pub), bytesArg([]byte("sign that message")), bytesArg(sigV.Bytes)}); v.Bool {
		t.Error("tampered message should verify false")
	}
	// cross-key: another keypair's public must reject this signature
	kp2, _ := signKeypairFn(noCtx, nil)
	var pub2 []byte
	for _, f := range kp2.Fields {
		if f.Name == "public" {
			pub2 = f.Value.Bytes
		}
	}
	if v, _ := verifyFn(noCtx, []interpreter.Value{bytesArg(pub2), bytesArg(msg), bytesArg(sigV.Bytes)}); v.Bool {
		t.Error("wrong public key should verify false")
	}
}

func TestEd25519LengthErrors(t *testing.T) {
	msg := []byte("m")
	if _, err := signFn(noCtx, []interpreter.Value{bytesArg(make([]byte, 32)), bytesArg(msg)}); err == nil {
		t.Error("sign with a 32-byte private key should error (need 64)")
	}
	// verify: a malformed public key or signature length is an error, not a panic
	if _, err := verifyFn(noCtx, []interpreter.Value{bytesArg(make([]byte, 10)), bytesArg(msg), bytesArg(make([]byte, 64))}); err == nil {
		t.Error("verify with a short public key should error")
	}
	if _, err := verifyFn(noCtx, []interpreter.Value{bytesArg(make([]byte, 32)), bytesArg(msg), bytesArg(make([]byte, 10))}); err == nil {
		t.Error("verify with a short signature should error")
	}
}

// Concurrent encryptions must draw distinct nonces from the shared random pool.
// A pool race handing two callers the same bytes would be GCM nonce reuse (a
// catastrophic key-recovery break), so this both stresses the pool under -race
// and asserts uniqueness.
func TestConcurrentEncryptDistinctNonces(t *testing.T) {
	key := make([]byte, 32)
	pt := []byte("same plaintext under one key")
	const n = 300
	nonces := make([][]byte, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := encryptFn(noCtx, []interpreter.Value{bytesArg(key), bytesArg(pt)})
			if err != nil {
				t.Errorf("encrypt: %v", err)
				return
			}
			nonces[i] = append([]byte(nil), v.Bytes[:12]...) // the prepended nonce
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, nc := range nonces {
		if nc == nil {
			continue
		}
		if seen[string(nc)] {
			t.Fatal("duplicate nonce across concurrent encryptions - GCM nonce reuse")
		}
		seen[string(nc)] = true
	}
	if len(seen) != n {
		t.Errorf("got %d distinct nonces, want %d", len(seen), n)
	}
}
