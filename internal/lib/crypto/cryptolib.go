// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package cryptolib implements Jennifer's `crypto` library: crypto-grade
// randomness and the two standard-library key
// derivation functions (HKDF and PBKDF2-HMAC-SHA256). Digests live in `hash`
// (MD5 / SHA-*), the keyed-hash MAC in `hash.hmac` and the constant-time
// compare in `hash.equal`; this library is the
// place for the security primitives that need a cryptographically secure
// source or a timing-safe operation.
//
// Everything here is standard library only (`crypto/rand`, `crypto/subtle`,
// `crypto/hkdf`, `crypto/pbkdf2`, added to the Go stdlib in 1.24), so the
// library adds no dependency and stays TinyGo-clean.
//
// The random source is a package-level buffer refilled from `crypto/rand` so
// the common small draw (a UUID, a nonce) does not pay a syscall each time. A
// failed read is a catchable Jennifer error, never weak bytes: the standard
// build reads the getrandom syscall and does not fail in practice, but
// jennifer-tiny's TinyGo `crypto/rand` reads /dev/urandom, which a minimal
// container or embedded image may not have.
package cryptolib

import (
	"crypto/hkdf"
	"crypto/pbkdf2"
	crand "crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
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
	in.RegisterNamespaced(LibraryName, "hkdf", hkdfFn)
	// PBKDF2-HMAC. The name keeps its "2" (interior digits are legal in
	// identifiers), so it reads exactly as the standard names it.
	in.RegisterNamespaced(LibraryName, "pbkdf2", pbkdf2Fn)
	// One function per operation, the algorithm its first argument (see
	// cryptolib_ops.go).
	in.RegisterNamespacedStruct(LibraryName, "Keypair", []parser.StructField{
		{Name: "public", Type: parser.PrimitiveType(parser.TypeBytes)},
		{Name: "private", Type: parser.PrimitiveType(parser.TypeBytes)},
	})
	in.RegisterNamespacedStruct(LibraryName, "Encapsulation", []parser.StructField{
		{Name: "ciphertext", Type: parser.PrimitiveType(parser.TypeBytes)},
		{Name: "sharedSecret", Type: parser.PrimitiveType(parser.TypeBytes)},
	})
	in.RegisterNamespaced(LibraryName, "encrypt", encryptFn)
	in.RegisterNamespaced(LibraryName, "decrypt", decryptFn)
	in.RegisterNamespaced(LibraryName, "blockEncrypt", blockEncryptFn)
	in.RegisterNamespaced(LibraryName, "blockDecrypt", blockDecryptFn)
	in.RegisterNamespaced(LibraryName, "sign", signFn)
	in.RegisterNamespaced(LibraryName, "verify", verifyFn)
	in.RegisterNamespaced(LibraryName, "keypair", keypairFn)
	in.RegisterNamespaced(LibraryName, "publicKey", publicKeyFn)
	in.RegisterNamespaced(LibraryName, "agree", agreeFn)
	in.RegisterNamespaced(LibraryName, "encapsulate", encapsulateFn)
	in.RegisterNamespaced(LibraryName, "decapsulate", decapsulateFn)
	in.RegisterNamespaced(LibraryName, "wrap", wrapFn)
	in.RegisterNamespaced(LibraryName, "unwrap", unwrapFn)

	// EC-SRP (Curve25519) key agreement for MikroTik MAC-Telnet / Winbox login
	// on RouterOS 6.43+ and v7. Dependency-free (math/big); TinyGo-clean.
	in.RegisterNamespaced(LibraryName, "mtweiKeygen", mtweiKeygenFn)
	in.RegisterNamespaced(LibraryName, "mtweiId", mtweiIdFn)
	in.RegisterNamespaced(LibraryName, "mtweiClientKey", mtweiClientKeyFn)

	// CSR (PKCS#10) and canonical JWK - what an ACME (RFC 8555) client needs
	// beyond keypair / sign. Default binary only (crypto/x509); stubbed on
	// jennifer-tiny.
	in.RegisterNamespaced(LibraryName, "jwkPublic", jwkPublicFn)
	in.RegisterNamespaced(LibraryName, "jwkToPem", jwkToPemFn)
	in.RegisterNamespaced(LibraryName, "csr", csrFn)
}

// ----- random source -------------------------------------------------------

var (
	randMu  sync.Mutex
	randBuf [512]byte
	randPos = len(randBuf) // past the end: force a refill on the first draw
	// randRead is the OS source; a variable so a test can make it fail.
	randRead = crand.Read
)

// RandByte returns one cryptographically secure random byte. Buffered so a
// per-byte caller does not hit `crypto/rand` on every call.
func RandByte() (byte, error) {
	randMu.Lock()
	defer randMu.Unlock()
	if randPos >= len(randBuf) {
		if err := fill(randBuf[:]); err != nil {
			return 0, err
		}
		randPos = 0
	}
	b := randBuf[randPos]
	randPos++
	return b, nil
}

// RandFill fills b with cryptographically secure random bytes from the shared
// buffer, taking the lock once for the whole slice. uuid fills all 16 (or 10)
// of a UUID's random bytes in one call, so it pays one lock acquisition instead
// of one per byte. Refills the buffer mid-copy as needed. On error b holds no
// usable randomness and must be discarded.
func RandFill(b []byte) error {
	// A draw bigger than half the buffer would cycle it under the shared lock;
	// crypto/rand.Read is itself thread-safe, so serve large draws directly and
	// keep the lock for the small-draw fast path (nonces, UUIDs).
	if len(b) > len(randBuf)/2 {
		return fill(b)
	}
	randMu.Lock()
	defer randMu.Unlock()
	for filled := 0; filled < len(b); {
		if randPos >= len(randBuf) {
			if err := fill(randBuf[:]); err != nil {
				return err
			}
			randPos = 0
		}
		n := copy(b[filled:], randBuf[randPos:])
		randPos += n
		filled += n
	}
	return nil
}

// fill reads len(b) crypto-grade random bytes into b. There is no weak
// fallback: if the OS source is unavailable the caller gets an error (raised as
// a catchable Jennifer error), never predictable bytes. A failed refill of the
// shared buffer leaves randPos past the end, so the next draw retries the
// refill instead of serving the unfilled bytes.
func fill(b []byte) error {
	if _, err := randRead(b); err != nil {
		return fmt.Errorf("secure random source unavailable: %v", err)
	}
	return nil
}

// randUint64 draws a uniform 64-bit value directly from crypto/rand (not the
// per-byte buffer): callers here need 8 bytes at a time, not one.
func randUint64() (uint64, error) {
	var b [8]byte
	if err := fill(b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b[:]), nil
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
	if err := fill(out); err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.randBytes: %v", err)
	}
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
	v, err := randInRange(lo, hi)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.randInt: %v", err)
	}
	return interpreter.IntVal(v), nil
}

// randInRange returns a uniform int64 in [lo, hi]. Width is computed in uint64
// so the full int64 span (MinInt64..MaxInt64) does not overflow; the top
// partial block is rejected so every value is equally likely.
func randInRange(lo, hi int64) (int64, error) {
	width := uint64(hi) - uint64(lo) // exact for hi >= lo, even across the sign boundary
	if width == ^uint64(0) {
		// The interval is the entire int64 range: every 64-bit draw is valid.
		r, err := randUint64()
		return int64(uint64(lo) + r), err
	}
	n := width + 1
	mod := ((^uint64(0) % n) + 1) % n // 2^64 mod n, computed without overflowing
	for {
		r, err := randUint64()
		if err != nil {
			return 0, err
		}
		if mod != 0 && r >= -mod { // r in the top [2^64-mod, 2^64) block: biased, reject
			continue
		}
		return int64(uint64(lo) + r%n), nil
	}
}

// ----- key derivation ------------------------------------------------------

// hkdfFn implements `crypto.hkdf(algo, secret, salt, info, length) -> bytes`:
// the HMAC-based Extract-and-Expand KDF (RFC 5869), deriving `length` bytes of
// keying material from a high-entropy `secret`. `salt` and `info` may be empty
// bytes; `algo` picks the PRF hash ("sha1" / "sha256" / "sha512").
func hkdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf expects 5 arguments (algo, secret, salt, info, length), got %d", len(args))
	}
	h, err := kdfHashOf("crypto.hkdf", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes || args[3].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: secret, salt, and info must be bytes")
	}
	if args[4].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.hkdf: length must be int, got %s", args[4].Kind)
	}
	length := args[4].Int
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
	key, kerr := hkdf.Key(h, args[1].Bytes, args[2].Bytes, string(args[3].Bytes), int(length))
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
// single-block key = 6e5 work) while bounding the worst case (measured on a
// 2020 desktop CPU: ~15 s with sha256, ~45 s with sha512).
// It also caps a hostile SCRAM server's `i=` (there keyLen is one block).
const (
	maxKeyLen    = 1 << 20     // 1 MiB
	maxPBKDFWork = 100_000_000 // blocks * iterations ceiling (~1e8)
)

// pbkdf2Fn implements `crypto.pbkdf2(algo, password, salt, iterations, keyLen)
// -> bytes`: PBKDF2 (RFC 8018) deriving a `keyLen`-byte key from a low-entropy
// `password` stretched by `iterations` rounds against `salt`, with `algo` the
// HMAC hash ("sha1" / "sha256" / "sha512"). A higher iteration count is slower
// to brute-force; pick the largest the deployment can afford. (SCRAM-SHA-1
// needs "sha1"; new password stores want "sha256" or better.)
func pbkdf2Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2 expects 5 arguments (algo, password, salt, iterations, keyLen), got %d", len(args))
	}
	h, err := kdfHashOf("crypto.pbkdf2", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: password and salt must be bytes")
	}
	if args[3].Kind != interpreter.KindInt || args[4].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: iterations and keyLen must be int")
	}
	iterations, keyLen := args[3].Int, args[4].Int
	if iterations <= 0 {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: iterations must be > 0, got %d", iterations)
	}
	if keyLen <= 0 {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: keyLen must be > 0, got %d", keyLen)
	}
	if keyLen > maxKeyLen {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: keyLen (%d) exceeds the %d-byte limit", keyLen, maxKeyLen)
	}
	// Bound the total work (blocks * iterations). Divide rather than multiply so
	// the check itself cannot overflow int64; blocks >= 1 since keyLen >= 1.
	hashLen := int64(h().Size())
	blocks := (keyLen + hashLen - 1) / hashLen
	if iterations > maxPBKDFWork/blocks {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: work (%d blocks x %d iterations) exceeds the %d limit; lower keyLen or iterations", blocks, iterations, maxPBKDFWork)
	}
	key, kerr := pbkdf2.Key(h, string(args[1].Bytes), args[2].Bytes, int(iterations), int(keyLen))
	if kerr != nil {
		return interpreter.Null(), fmt.Errorf("crypto.pbkdf2: %v", kerr)
	}
	return interpreter.BytesVal(key), nil
}

// kdfHashOf resolves the algo argument shared by hkdf and pbkdf2.
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
