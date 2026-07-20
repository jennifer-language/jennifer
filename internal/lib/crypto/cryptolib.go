// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package cryptolib implements Jennifer's `crypto` library: crypto-grade
// randomness, constant-time comparison, and the two standard-library key
// derivation functions (HKDF and PBKDF2-HMAC-SHA256). Digests live in `hash`
// (MD5 / SHA-*) and the keyed-hash MAC in `hash.hmac`; this library is the
// place for the security primitives that need a cryptographically secure
// source or a timing-safe operation.
//
// Everything here is standard library only (`crypto/rand`, `crypto/subtle`,
// `crypto/hkdf`, `crypto/pbkdf2`, added to the Go stdlib in 1.24), so the
// library adds no dependency and stays TinyGo-clean.
//
// The random source is a package-level buffer refilled from `crypto/rand` so
// the common per-byte draw (uuid's `randByte`) does not pay a syscall each
// time; `crypto/rand.Read` never returns an error on the supported platform,
// and an impossible failure panics rather than silently yielding weak bytes.
package cryptolib

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/pbkdf2"
	crand "crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	gohash "hash"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`crypto.`) and the `use` name.
const LibraryName = "crypto"

// kdfHash maps a Jennifer-side algorithm string to the hash constructor the
// KDFs use as their PRF. The same names as `hash.compute` / `hash.hmac`, minus
// md5 (too weak to derive a key with). SHA-1 stays available because SCRAM-SHA-1
// (MongoDB, XMPP) mandates PBKDF2-HMAC-SHA1.
var kdfHash = map[string]func() gohash.Hash{
	"sha1":   sha1.New,
	"sha256": sha256.New,
	"sha512": sha512.New,
}

// kdfAlgoList is the rendered known-algorithm set for error messages.
const kdfAlgoList = `"sha1", "sha256", "sha512"`

// Install registers the crypto surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "randBytes", randBytesFn)
	in.RegisterNamespaced(LibraryName, "randInt", randIntFn)
	in.RegisterNamespaced(LibraryName, "hmacEqual", hmacEqualFn)
	in.RegisterNamespaced(LibraryName, "hkdf", hkdfFn)
	// Registered as `pbkdf` (not `pbkdf2`): a Jennifer method name is
	// letters-only, so the "2" can't appear - the same rule that makes uuid's
	// version a string arg (`uuid.generate("v4")`). The scheme is still
	// PBKDF2-HMAC-SHA256; the name just drops the digit.
	in.RegisterNamespaced(LibraryName, "pbkdf", pbkdf2Fn)
	// Authenticated symmetric encryption (AES-256-GCM). One algorithm, AEAD only:
	// no raw-mode / nonce footguns are expressible.
	in.RegisterNamespaced(LibraryName, "encrypt", encryptFn)
	in.RegisterNamespaced(LibraryName, "decrypt", decryptFn)
	// Ed25519 signatures. Parameterless keypair; no curve / scheme menu.
	in.RegisterNamespacedStruct(LibraryName, "Keypair", []parser.StructField{
		{Name: "public", Type: parser.PrimitiveType(parser.TypeBytes)},
		{Name: "private", Type: parser.PrimitiveType(parser.TypeBytes)},
	})
	in.RegisterNamespaced(LibraryName, "signKeypair", signKeypairFn)
	in.RegisterNamespaced(LibraryName, "sign", signFn)
	in.RegisterNamespaced(LibraryName, "verify", verifyFn)

	// Asymmetric RSA / ECDSA sign / verify over PEM keys (for JWT RS* / ES*).
	// Build-tag split like `net`: real on the default binary, a friendly stub on
	// jennifer-tiny (crypto/x509 is heavy and off the TinyGo build).
	in.RegisterNamespaced(LibraryName, "rsaSign", rsaSignFn)
	in.RegisterNamespaced(LibraryName, "rsaVerify", rsaVerifyFn)
	in.RegisterNamespaced(LibraryName, "ecdsaSign", ecdsaSignFn)
	in.RegisterNamespaced(LibraryName, "ecdsaVerify", ecdsaVerifyFn)

	// RSA / ECDSA key generation, CSR (PKCS#10), and canonical JWK - the
	// asymmetric operations an ACME (RFC 8555) client needs. Same build-tag
	// split (default-binary only; stubbed on jennifer-tiny).
	in.RegisterNamespaced(LibraryName, "rsaGenerateKey", rsaGenerateKeyFn)
	in.RegisterNamespaced(LibraryName, "ecGenerateKey", ecGenerateKeyFn)
	in.RegisterNamespaced(LibraryName, "jwkPublic", jwkPublicFn)
	in.RegisterNamespaced(LibraryName, "csr", csrFn)
}

// ----- authenticated symmetric encryption (AES-256-GCM) --------------------

// aesKeyLen is the AES-256 key length; encrypt / decrypt accept nothing else.
const aesKeyLen = 32

// gcmMaxPlaintext is GCM's structural per-message limit: (2^32 - 2) blocks of 16
// bytes (~68 GiB). Go's gcm.Seal *panics* above it, and a builtin panic is not
// catchable, so encrypt bounds the plaintext to a catchable error instead. (This
// is far beyond any realistic single-message payload; split larger data.)
const gcmMaxPlaintext = uint64(((1 << 32) - 2) * 16)

// encryptFn implements crypto.encrypt(key, plaintext) -> bytes. It seals the
// plaintext with AES-256-GCM and returns nonce || ciphertext || tag: a fresh
// 12-byte nonce is drawn from the crypto-grade source and prepended, so the
// caller never handles (and so never reuses) a nonce.
func encryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crypto.encrypt expects 2 arguments (key, plaintext), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.encrypt: key and plaintext must be bytes")
	}
	key := args[0].Bytes
	if len(key) != aesKeyLen {
		return interpreter.Null(), fmt.Errorf("crypto.encrypt: key must be exactly %d bytes (AES-256); got %d (use crypto.randBytes(%d))", aesKeyLen, len(key), aesKeyLen)
	}
	if uint64(len(args[1].Bytes)) > gcmMaxPlaintext {
		return interpreter.Null(), fmt.Errorf("crypto.encrypt: plaintext too large for a single AES-GCM message (max %d bytes); split it into chunks", gcmMaxPlaintext)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.encrypt: %v", err)
	}
	// Allocate the nonce with capacity for the whole box up front, so Seal
	// appends ciphertext+tag in place (nonce||ct||tag) with no second
	// allocation-and-copy of the message.
	nonce := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(args[1].Bytes)+gcm.Overhead())
	RandFill(nonce)
	box := gcm.Seal(nonce, nonce, args[1].Bytes, nil)
	return interpreter.BytesVal(box), nil
}

// decryptFn implements crypto.decrypt(key, box) -> bytes. It splits the
// prepended nonce, verifies the tag, and returns the plaintext; a wrong key or a
// tampered box is a catchable authentication error, never silent garbage.
func decryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt expects 2 arguments (key, box), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt: key and box must be bytes")
	}
	key, box := args[0].Bytes, args[1].Bytes
	if len(key) != aesKeyLen {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt: key must be exactly %d bytes (AES-256); got %d", aesKeyLen, len(key))
	}
	gcm, err := newGCM(key)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt: %v", err)
	}
	ns := gcm.NonceSize()
	if len(box) < ns {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt: box is too short to contain a nonce (need at least %d bytes)", ns)
	}
	nonce, ct := box[:ns], box[ns:]
	pt, oerr := gcm.Open(nil, nonce, ct, nil)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt: authentication failed (wrong key or tampered ciphertext)")
	}
	return interpreter.BytesVal(pt), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ----- Ed25519 signatures --------------------------------------------------

// signKeypairFn implements crypto.signKeypair() -> crypto.Keypair. Generates a
// fresh Ed25519 keypair (32-byte public, 64-byte private) from crypto/rand.
func signKeypairFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("crypto.signKeypair expects no arguments, got %d", len(args))
	}
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.signKeypair: %v", err)
	}
	return interpreter.NamespacedStructVal(LibraryName, "Keypair", []interpreter.StructField{
		{Name: "public", Value: interpreter.BytesVal([]byte(pub))},
		{Name: "private", Value: interpreter.BytesVal([]byte(priv))},
	}), nil
}

// signFn implements crypto.sign(private, message) -> bytes (a 64-byte signature).
func signFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crypto.sign expects 2 arguments (private, message), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.sign: private key and message must be bytes")
	}
	priv := args[0].Bytes
	if len(priv) != ed25519.PrivateKeySize {
		return interpreter.Null(), fmt.Errorf("crypto.sign: private key must be %d bytes (from crypto.signKeypair), got %d", ed25519.PrivateKeySize, len(priv))
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), args[1].Bytes)
	return interpreter.BytesVal(sig), nil
}

// verifyFn implements crypto.verify(public, message, signature) -> bool. A
// genuine mismatch (wrong key / message / signature) is false; a malformed key
// or signature length is a positioned error.
func verifyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("crypto.verify expects 3 arguments (public, message, signature), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.verify: public key, message, and signature must be bytes")
	}
	pub, sig := args[0].Bytes, args[2].Bytes
	if len(pub) != ed25519.PublicKeySize {
		return interpreter.Null(), fmt.Errorf("crypto.verify: public key must be %d bytes, got %d", ed25519.PublicKeySize, len(pub))
	}
	if len(sig) != ed25519.SignatureSize {
		return interpreter.Null(), fmt.Errorf("crypto.verify: signature must be %d bytes, got %d", ed25519.SignatureSize, len(sig))
	}
	ok := ed25519.Verify(ed25519.PublicKey(pub), args[1].Bytes, sig)
	return interpreter.BoolVal(ok), nil
}

// ----- random source -------------------------------------------------------

var (
	randMu  sync.Mutex
	randBuf [512]byte
	randPos = len(randBuf) // past the end: force a refill on the first draw
)

// RandByte returns one cryptographically secure random byte. It is the source
// `uuidlib.randByte` draws on, so v4 / v7 UUIDs are unguessable. Buffered so a
// per-byte caller does not hit `crypto/rand` on every call.
func RandByte() byte {
	randMu.Lock()
	defer randMu.Unlock()
	if randPos >= len(randBuf) {
		fill(randBuf[:])
		randPos = 0
	}
	b := randBuf[randPos]
	randPos++
	return b
}

// RandFill fills b with cryptographically secure random bytes from the shared
// buffer, taking the lock once for the whole slice. uuid fills all 16 (or 10)
// of a UUID's random bytes in one call, so it pays one lock acquisition instead
// of one per byte. Refills the buffer mid-copy as needed.
func RandFill(b []byte) {
	// A draw bigger than half the buffer would cycle it under the shared lock;
	// crypto/rand.Read is itself thread-safe, so serve large draws directly and
	// keep the lock for the small-draw fast path (nonces, UUIDs).
	if len(b) > len(randBuf)/2 {
		fill(b)
		return
	}
	randMu.Lock()
	defer randMu.Unlock()
	for filled := 0; filled < len(b); {
		if randPos >= len(randBuf) {
			fill(randBuf[:])
			randPos = 0
		}
		n := copy(b[filled:], randBuf[randPos:])
		randPos += n
		filled += n
	}
}

// fill reads len(b) crypto-grade random bytes into b. crypto/rand.Read fills
// completely and does not return an error on the supported platform; a failure
// is unrecoverable (no weak fallback), so it panics.
func fill(b []byte) {
	if _, err := crand.Read(b); err != nil {
		panic("crypto: secure random source unavailable: " + err.Error())
	}
}

// randUint64 draws a uniform 64-bit value directly from crypto/rand (not the
// per-byte buffer): callers here need 8 bytes at a time, not one.
func randUint64() uint64 {
	var b [8]byte
	fill(b[:])
	return binary.BigEndian.Uint64(b[:])
}

// maxRandBytes bounds a single crypto.randBytes request. Keys, tokens, nonces,
// and salts are tens of bytes to a few KB; 64 MiB is far beyond any legitimate
// draw and keeps an untrusted length (e.g. a request field) from triggering an
// unbounded `make` - which, since the interpreter has no recover(), is a fatal
// process crash (uncatchable OOM), not a Jennifer error.
const maxRandBytes = 1 << 26 // 64 MiB

// randBytesFn implements `crypto.randBytes(n) -> bytes`: n crypto-grade random
// bytes. n must be in [0, maxRandBytes]; n == 0 yields empty bytes.
func randBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("crypto.randBytes expects 1 argument (n), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.randBytes: n must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("crypto.randBytes: n must be >= 0, got %d", n)
	}
	if n > maxRandBytes {
		return interpreter.Null(), fmt.Errorf("crypto.randBytes: n (%d) exceeds the %d-byte limit", n, maxRandBytes)
	}
	out := make([]byte, n)
	fill(out)
	return interpreter.BytesVal(out), nil
}

// randIntFn implements `crypto.randInt(lo, hi) -> int`: a uniform int in the
// inclusive range [lo, hi], drawn crypto-grade. Same shape as math.randInt,
// but unpredictable and unseedable - the drop-in for a security context.
// Rejection sampling makes the distribution exactly uniform (no modulo bias).
func randIntFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crypto.randInt expects 2 arguments (lo, hi), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.randInt: lo and hi must be int")
	}
	lo, hi := args[0].Int, args[1].Int
	if lo > hi {
		return interpreter.Null(), fmt.Errorf("crypto.randInt: lo (%d) must be <= hi (%d)", lo, hi)
	}
	return interpreter.IntVal(randInRange(lo, hi)), nil
}

// randInRange returns a uniform int64 in [lo, hi]. Width is computed in uint64
// so the full int64 span (MinInt64..MaxInt64) does not overflow; the top
// partial block is rejected so every value is equally likely.
func randInRange(lo, hi int64) int64 {
	width := uint64(hi) - uint64(lo) // exact for hi >= lo, even across the sign boundary
	if width == ^uint64(0) {
		// The interval is the entire int64 range: every 64-bit draw is valid.
		return int64(uint64(lo) + randUint64())
	}
	n := width + 1
	mod := ((^uint64(0) % n) + 1) % n // 2^64 mod n, computed without overflowing
	for {
		r := randUint64()
		if mod != 0 && r >= -mod { // r in the top [2^64-mod, 2^64) block: biased, reject
			continue
		}
		return int64(uint64(lo) + r%n)
	}
}

// ----- constant-time comparison --------------------------------------------

// hmacEqualFn implements `crypto.hmacEqual(a, b) -> bool`: a constant-time
// equality test over two `bytes`, for comparing a computed MAC against a
// supplied one without leaking, through response timing, how many leading
// bytes matched. Unequal lengths return false (also in constant time relative
// to the inputs' contents).
func hmacEqualFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crypto.hmacEqual expects 2 arguments (a, b), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.hmacEqual: both arguments must be bytes")
	}
	eq := subtle.ConstantTimeCompare(args[0].Bytes, args[1].Bytes) == 1
	return interpreter.BoolVal(eq), nil
}

// ----- key derivation ------------------------------------------------------

// hkdfFn implements `crypto.hkdf(secret, salt, info, length, algo) -> bytes`:
// the HMAC-based Extract-and-Expand KDF (RFC 5869), deriving `length` bytes of
// keying material from a high-entropy `secret`. `salt` and `info` may be empty
// bytes; `algo` picks the PRF hash ("sha1" / "sha256" / "sha512").
func hkdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf expects 5 arguments (secret, salt, info, length, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: secret, salt, and info must be bytes")
	}
	if args[3].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: length must be int, got %s", args[3].Kind)
	}
	h, err := kdfHashOf("crypto.hkdf", args[4])
	if err != nil {
		return interpreter.Null(), err
	}
	length := args[3].Int
	if length <= 0 {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: length must be > 0, got %d", length)
	}
	// Reject an over-large length before the int() cast. On a 64-bit build
	// hkdf.Key would reject it anyway (max 255*hashLen), but on a 32-bit target
	// (jennifer-tiny) a value like 1<<40 could truncate to a small int that
	// HKDF accepts, silently yielding a shorter-than-requested key. maxKeyLen is
	// far above any real HKDF output, so this is a safe explicit bound.
	if length > maxKeyLen {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: length (%d) exceeds the %d-byte limit", length, maxKeyLen)
	}
	key, kerr := hkdf.Key(h, args[0].Bytes, args[1].Bytes, string(args[2].Bytes), int(length))
	if kerr != nil {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: %v", kerr)
	}
	return interpreter.BytesVal(key), nil
}

// maxKeyLen bounds the derived-key length so the output allocation is bounded
// (real keys are tens of bytes; 1 MiB is far beyond any legitimate use).
//
// maxPBKDFWork bounds the actual compute: PBKDF2 runs one PRF chain per output
// block - ceil(keyLen / hashLen) blocks - each `iterations` rounds, so the work
// is the *product* blocks*iterations, not either factor alone. Bounding them
// separately still lets a caller pair a large keyLen with a huge iteration
// count and peg a core for days; this caps the product instead. 1e8 leaves
// generous headroom over real use (OWASP suggests ~600k iterations for a
// single-block key = 6e5 work) while keeping the worst case to a bounded ~20s.
// It also caps a hostile SCRAM server's `i=` (there keyLen is one block).
const (
	maxKeyLen    = 1 << 20     // 1 MiB
	maxPBKDFWork = 100_000_000 // blocks * iterations ceiling (~1e8)
)

// pbkdf2Fn implements `crypto.pbkdf(password, salt, iterations, keyLen, algo)
// -> bytes`: PBKDF2 (RFC 8018) deriving a `keyLen`-byte key from a low-entropy
// `password` stretched by `iterations` rounds against `salt`, with `algo` the
// HMAC hash ("sha1" / "sha256" / "sha512"). A higher iteration count is slower
// to brute-force; pick the largest the deployment can afford. (SCRAM-SHA-1
// needs "sha1"; new password stores want "sha256" or better.)
func pbkdf2Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf expects 5 arguments (password, salt, iterations, keyLen, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: password and salt must be bytes")
	}
	if args[2].Kind != interpreter.KindInt || args[3].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: iterations and keyLen must be int")
	}
	h, err := kdfHashOf("crypto.pbkdf", args[4])
	if err != nil {
		return interpreter.Null(), err
	}
	iterations, keyLen := args[2].Int, args[3].Int
	if iterations <= 0 {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: iterations must be > 0, got %d", iterations)
	}
	if keyLen <= 0 {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: keyLen must be > 0, got %d", keyLen)
	}
	if keyLen > maxKeyLen {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: keyLen (%d) exceeds the %d-byte limit", keyLen, maxKeyLen)
	}
	// Bound the total work (blocks * iterations). Divide rather than multiply so
	// the check itself cannot overflow int64; blocks >= 1 since keyLen >= 1.
	hashLen := int64(h().Size())
	blocks := (keyLen + hashLen - 1) / hashLen
	if iterations > maxPBKDFWork/blocks {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: work (%d blocks x %d iterations) exceeds the %d limit; lower keyLen or iterations", blocks, iterations, maxPBKDFWork)
	}
	key, kerr := pbkdf2.Key(h, string(args[0].Bytes), args[1].Bytes, int(iterations), int(keyLen))
	if kerr != nil {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf: %v", kerr)
	}
	return interpreter.BytesVal(key), nil
}

// kdfHashOf resolves the algo argument shared by hkdf and pbkdf.
func kdfHashOf(fn string, v interpreter.Value) (func() gohash.Hash, error) {
	if v.Kind != interpreter.KindString {
		return nil, fmt.Errorf("%s: algo must be string, got %s", fn, v.Kind)
	}
	h, ok := kdfHash[v.Str]
	if !ok {
		return nil, fmt.Errorf("%s: unknown algorithm %q; known: %s", fn, v.Str, kdfAlgoList)
	}
	return h, nil
}
