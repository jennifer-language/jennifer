// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package cryptolib implements Jennifer's `crypto` library: crypto-grade
// randomness, key derivation, password hashing, encryption, signatures, key
// agreement and key encapsulation. Digests live in `hash`
// (MD5 / SHA-*), the keyed-hash MAC in `hash.hmac` and the constant-time
// compare in `hash.equal`; this library is the
// place for the security primitives that need a cryptographically secure
// source or a timing-safe operation.
//
// The KDFs here are standard library (`crypto/hkdf`, `crypto/pbkdf2`, Go
// 1.24). The ChaCha20-Poly1305 AEADs, the libsodium sealed box, password
// hashing and the Argon2id / scrypt KDFs come from golang.org/x/crypto
// (cryptolib_xcrypto.go, M28.58), the one dependency this library takes; it
// builds for both binaries.
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
	"crypto/sha3"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	gohash "hash"
	"sync"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/blake2s"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`crypto.`) and the `use` name.
const LibraryName = "crypto"

// kdfHashNames are the PRF hashes of HKDF and PBKDF2: the `hash` library's
// names minus md5, in the order messages list them. sha1 stays for protocols
// that fix it (SCRAM-SHA-1, legacy PBKDF2 stores) and is tagged weak.
var kdfHashNames = []string{
	"sha1", "sha256", "sha384", "sha512",
	"sha3-224", "sha3-256", "sha3-384", "sha3-512",
	"blake2b-256", "blake2b-384", "blake2b-512", "blake2s-256",
}

// kdfHash maps each of kdfHashNames to its constructor.
var kdfHash = map[string]func() gohash.Hash{
	"sha1":        sha1.New,
	"sha256":      sha256.New,
	"sha384":      sha512.New384,
	"sha512":      sha512.New,
	"sha3-224":    func() gohash.Hash { return sha3.New224() },
	"sha3-256":    func() gohash.Hash { return sha3.New256() },
	"sha3-384":    func() gohash.Hash { return sha3.New384() },
	"sha3-512":    func() gohash.Hash { return sha3.New512() },
	"blake2b-256": func() gohash.Hash { return mustHash(blake2b.New256(nil)) },
	"blake2b-384": func() gohash.Hash { return mustHash(blake2b.New384(nil)) },
	"blake2b-512": func() gohash.Hash { return mustHash(blake2b.New512(nil)) },
	"blake2s-256": func() gohash.Hash { return mustHash(blake2s.New256(nil)) },
}

// mustHash unwraps a BLAKE2 constructor, which fails only for a key that is
// too long; kdfHash passes none.
func mustHash(h gohash.Hash, err error) gohash.Hash {
	if err != nil {
		panic("crypto: unkeyed BLAKE2 failed: " + err.Error())
	}
	return h
}

// md5KDF is the refusal of md5 as a KDF hash, which `hash` offers but
// key derivation does not.
const md5KDF = "md5 is not offered for key derivation (it is broken); use sha256"

// Install registers the crypto surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "randBytes", randBytesFn)
	in.RegisterNamespaced(LibraryName, "randInt", randIntFn)
	// Every key derivation, the algorithm first (cryptolib_kdf.go). hkdf,
	// pbkdf2, argon2id and scrypt are deprecated, removed in 0.28.0.
	in.RegisterNamespacedStruct(LibraryName, "KdfParams", kdfParamFields)
	in.RegisterNamespaced(LibraryName, "deriveKey", deriveKeyFn)
	// PBKDF2-HMAC. The name keeps its "2" (interior digits are legal in
	// identifiers), so it reads exactly as the standard names it.
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
	in.RegisterNamespaced(LibraryName, "hashPassword", hashPasswordFn)
	in.RegisterNamespaced(LibraryName, "verifyPassword", verifyPasswordFn)

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
		return interpreter.Null(), limits.Capf("crypto.randBytes: n (%d) exceeds the %d-byte limit", n, maxRandBytes)
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

// hkdfCore is hkdf with fn, the name its messages give (crypto.deriveKey reuses it).
func hkdfCore(fn string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("%s expects 5 arguments (algo, secret, salt, info, length), got %d", fn, len(args))
	}
	h, err := kdfHashOf(fn, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes || args[3].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: secret, salt, and info must be bytes", fn)
	}
	if args[4].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("%s: length must be int, got %s", fn, args[4].Kind)
	}
	length := args[4].Int
	if length <= 0 {
		return interpreter.Null(), fmt.Errorf("%s: length must be > 0, got %d", fn, length)
	}
	// RFC 5869 bounds the output at 255 hash lengths (8160 bytes for sha256).
	// Checked here, before the int() cast, so a value like 1<<40 cannot
	// truncate to a small int on a 32-bit target (jennifer-tiny), and with a
	// message of our own. It is the algorithm's bound, not one of our caps.
	if most := int64(255 * h().Size()); length > most {
		return interpreter.Null(), fmt.Errorf("%s: length %d is over HKDF's maximum of %d bytes for %s (255 times the hash size)", fn, length, most, args[0].Str)
	}
	key, kerr := hkdf.Key(h, args[1].Bytes, args[2].Bytes, string(args[3].Bytes), int(length))
	if kerr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, kerr)
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

// pbkdf2Core is pbkdf2 with fn, the name its messages give (crypto.deriveKey reuses it).
func pbkdf2Core(fn string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("%s expects 5 arguments (algo, password, salt, iterations, keyLen), got %d", fn, len(args))
	}
	h, err := kdfHashOf(fn, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: password and salt must be bytes", fn)
	}
	if args[3].Kind != interpreter.KindInt || args[4].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("%s: iterations and keyLen must be int", fn)
	}
	iterations, keyLen := args[3].Int, args[4].Int
	if iterations <= 0 {
		return interpreter.Null(), fmt.Errorf("%s: iterations must be > 0, got %d", fn, iterations)
	}
	if keyLen <= 0 {
		return interpreter.Null(), fmt.Errorf("%s: keyLen must be > 0, got %d", fn, keyLen)
	}
	if keyLen > maxKeyLen {
		return interpreter.Null(), limits.Capf("%s: keyLen (%d) exceeds the %d-byte limit", fn, keyLen, maxKeyLen)
	}
	// Bound the total work (blocks * iterations). Divide rather than multiply so
	// the check itself cannot overflow int64; blocks >= 1 since keyLen >= 1.
	hashLen := int64(h().Size())
	blocks := (keyLen + hashLen - 1) / hashLen
	if iterations > maxPBKDFWork/blocks {
		return interpreter.Null(), limits.Capf("%s: work (%d blocks x %d iterations) exceeds the %d limit; lower keyLen or iterations", fn, blocks, iterations, maxPBKDFWork)
	}
	key, kerr := pbkdf2.Key(h, string(args[1].Bytes), args[2].Bytes, int(iterations), int(keyLen))
	if kerr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, kerr)
	}
	return interpreter.BytesVal(key), nil
}

// kdfHashOf resolves the algo argument shared by hkdf and pbkdf2.
func kdfHashOf(fn string, v interpreter.Value) (func() gohash.Hash, error) {
	if v.Kind != interpreter.KindString {
		return nil, fmt.Errorf("%s: algo must be string, got %s", fn, v.Kind)
	}
	if v.Str == "md5" {
		return nil, fmt.Errorf("%s: %s", fn, md5KDF)
	}
	h, ok := kdfHash[v.Str]
	if !ok {
		return nil, fmt.Errorf("%s: unknown algorithm %q; known: %s", fn, v.Str, quoteList(kdfHashNames))
	}
	return h, nil
}
