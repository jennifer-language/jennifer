// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

// The parts of `crypto` that come from golang.org/x/crypto (M28.58), the one
// dependency the library takes, since crypto is never hand-rolled: the
// ChaCha20-Poly1305 AEADs, the libsodium sealed box, password hashing
// (Argon2id, bcrypt, scrypt) and the raw Argon2id / scrypt KDFs. All of it
// builds for both binaries.

import (
	"crypto/cipher"
	"crypto/ecdh"
	crand "crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/scrypt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// ----- authenticated encryption ---------------------------------------------

// chachaMaxPlaintext is ChaCha20-Poly1305's per-message limit (2^38 - 64
// bytes, ~256 GiB); Seal panics above it, so encrypt bounds it first.
const chachaMaxPlaintext = uint64(1<<38 - 64)

// aeadFor builds the AEAD an encrypt / decrypt names after checking the key
// and nonce, and returns the most plaintext one message may hold.
func aeadFor(fn, algo string, key, nonce []byte) (cipher.AEAD, uint64, error) {
	switch algo {
	case "aes-gcm":
		gcm, err := gcmFor(fn, key, nonce)
		return gcm, gcmMaxPlaintext, err
	case "chacha20-poly1305", "xchacha20-poly1305":
		if len(key) != chacha20poly1305.KeySize {
			return nil, 0, fmt.Errorf("crypto.%s: key must be %d bytes for %s, got %d", fn, chacha20poly1305.KeySize, algo, len(key))
		}
		want := chacha20poly1305.NonceSize
		mk := chacha20poly1305.New
		if algo == "xchacha20-poly1305" {
			want = chacha20poly1305.NonceSizeX
			mk = chacha20poly1305.NewX
		}
		if len(nonce) != want {
			return nil, 0, fmt.Errorf("crypto.%s: nonce must be %d bytes for %s, got %d", fn, want, algo, len(nonce))
		}
		aead, err := mk(key)
		if err != nil {
			return nil, 0, fmt.Errorf("crypto.%s: %v", fn, err)
		}
		return aead, chachaMaxPlaintext, nil
	}
	return nil, 0, unknownAlgo(fn, algo, aeadAlgos)
}

// ----- the libsodium sealed box ---------------------------------------------

// sealedBoxOverhead is what a sealed box adds: the ephemeral public key and
// the Poly1305 tag.
const sealedBoxOverhead = 32 + box.Overhead

// sealedBoxSeal implements wrap("nacl-sealedbox", plaintext, publicKey):
// libsodium's crypto_box_seal (an ephemeral X25519 key, XSalsa20-Poly1305,
// the nonce from BLAKE2b of both public keys). Only the recipient's private
// key opens it; the sender cannot.
func sealedBoxSeal(plaintext, pub []byte) (interpreter.Value, error) {
	if len(pub) != 32 {
		return interpreter.Null(), fmt.Errorf("crypto.wrap: a nacl-sealedbox public key is 32 bytes (X25519), got %d", len(pub))
	}
	if err := checkX25519Public(pub); err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.wrap: %v", err)
	}
	out, err := box.SealAnonymous(nil, plaintext, (*[32]byte)(pub), crand.Reader)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.wrap: %v", err)
	}
	return interpreter.BytesVal(out), nil
}

// sealedBoxOpen implements unwrap("nacl-sealedbox", box, privateKey). The
// public key is derived from the private one.
func sealedBoxOpen(sealed, priv []byte) (interpreter.Value, error) {
	if len(priv) != 32 {
		return interpreter.Null(), fmt.Errorf("crypto.unwrap: a nacl-sealedbox private key is 32 bytes (X25519), got %d", len(priv))
	}
	if len(sealed) < sealedBoxOverhead {
		return interpreter.Null(), fmt.Errorf("crypto.unwrap: a sealed box is at least %d bytes, got %d", sealedBoxOverhead, len(sealed))
	}
	k, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.unwrap: %v", err)
	}
	pt, ok := box.OpenAnonymous(nil, sealed, (*[32]byte)(k.PublicKey().Bytes()), (*[32]byte)(priv))
	if !ok {
		return interpreter.Null(), fmt.Errorf("crypto.unwrap: authentication failed (wrong key or tampered data)")
	}
	if pt == nil {
		pt = []byte{}
	}
	return interpreter.BytesVal(pt), nil
}

// checkX25519Public refuses a public key whose shared secret is all zeros (a
// low-order point), which would seal to a key anyone can compute.
func checkX25519Public(pub []byte) error {
	p, err := ecdh.X25519().NewPublicKey(pub)
	if err != nil {
		return err
	}
	probe, err := ecdh.X25519().GenerateKey(crand.Reader)
	if err != nil {
		return err
	}
	if _, err := probe.ECDH(p); err != nil {
		return errors.New("the public key is a low-order point")
	}
	return nil
}

// ----- password hashing -----------------------------------------------------

// randomBytes draws n bytes from the library's crypto-grade source.
func randomBytes(fn string, n int) ([]byte, error) {
	b := make([]byte, n)
	if err := RandFill(b); err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return b, nil
}

// The parameters hashPassword uses: RFC 9106's second recommended Argon2id
// setting, bcrypt cost 12, and scrypt as OWASP recommends (N = 2^17, r 8, p 1).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonSalt    = 16
	argonKey     = 32
	bcryptCost   = 12
	scryptLogN   = 17
	scryptR      = 8
	scryptP      = 1
	scryptSalt   = 16
	scryptKey    = 32
)

// The most work a hash or a raw KDF call may ask for, so a hostile stored
// hash or a typo cannot pin a core or exhaust memory: Argon2id 1 GiB, 16
// passes, 64 lanes; scrypt 1 GiB (128 * r * N bytes) and p 16; bcrypt cost 18
// (about 16 s).
const (
	maxArgonMemory  = 1 << 20 // KiB
	maxArgonTime    = 16
	maxArgonThreads = 64
	maxScryptMemory = 1 << 30 // bytes
	maxScryptP      = 16
	maxBcryptCost   = 18
	minSaltLen      = 8
	maxSaltLen      = 1024
)

var passwordAlgos = []string{"argon2id", "bcrypt", "scrypt"}

// b64 is the PHC string format's base64: the standard alphabet, unpadded.
var b64 = base64.RawStdEncoding

// hashPasswordFn implements crypto.hashPassword(algo, password) -> string: a
// self-describing hash with a fresh random salt, `$argon2id$v=19$m=..,t=..,p=..$salt$hash`,
// `$2a$12$...` or `$scrypt$ln=..,r=..,p=..$salt$hash`.
func hashPasswordFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 || args[0].Kind != interpreter.KindString || args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crypto.hashPassword expects (algo as string, password as string)")
	}
	algo, pw := args[0].Str, []byte(args[1].Str)
	switch algo {
	case "argon2id":
		salt, err := randomBytes("hashPassword", argonSalt)
		if err != nil {
			return interpreter.Null(), err
		}
		key := argon2.IDKey(pw, salt, argonTime, argonMemory, argonThreads, argonKey)
		return interpreter.StringVal(fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
			argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key))), nil
	case "bcrypt":
		h, err := bcrypt.GenerateFromPassword(pw, bcryptCost)
		if err != nil {
			return interpreter.Null(), bcryptError("hashPassword", err)
		}
		return interpreter.StringVal(string(h)), nil
	case "scrypt":
		salt, err := randomBytes("hashPassword", scryptSalt)
		if err != nil {
			return interpreter.Null(), err
		}
		key, err := scrypt.Key(pw, salt, 1<<scryptLogN, scryptR, scryptP, scryptKey)
		if err != nil {
			return interpreter.Null(), fmt.Errorf("crypto.hashPassword: %v", err)
		}
		return interpreter.StringVal(fmt.Sprintf("$scrypt$ln=%d,r=%d,p=%d$%s$%s",
			scryptLogN, scryptR, scryptP, b64.EncodeToString(salt), b64.EncodeToString(key))), nil
	}
	return interpreter.Null(), unknownAlgo("hashPassword", algo, passwordAlgos)
}

// verifyPasswordFn implements crypto.verifyPassword(password, hash) -> bool.
// The algorithm and its parameters come from the hash; the comparison is
// constant-time. A wrong password is false; a hash that is malformed, of an
// unknown kind, or asks for more work than the caps allow is an error.
func verifyPasswordFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 || args[0].Kind != interpreter.KindString || args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crypto.verifyPassword expects (password as string, hash as string)")
	}
	pw, stored := []byte(args[0].Str), args[1].Str
	switch {
	case strings.HasPrefix(stored, "$argon2id$"):
		ok, err := verifyArgon2id(pw, stored)
		return interpreter.BoolVal(ok), err
	case strings.HasPrefix(stored, "$scrypt$"):
		ok, err := verifyScrypt(pw, stored)
		return interpreter.BoolVal(ok), err
	case strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$"):
		cost, err := bcrypt.Cost([]byte(stored))
		if err != nil {
			return interpreter.Null(), fmt.Errorf("crypto.verifyPassword: a malformed bcrypt hash: %v", err)
		}
		if cost > maxBcryptCost {
			return interpreter.Null(), fmt.Errorf("crypto.verifyPassword: bcrypt cost %d is over the limit of %d", cost, maxBcryptCost)
		}
		err = bcrypt.CompareHashAndPassword([]byte(stored), pw)
		if err == nil {
			return interpreter.BoolVal(true), nil
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return interpreter.BoolVal(false), nil
		}
		return interpreter.Null(), bcryptError("verifyPassword", err)
	}
	return interpreter.Null(), fmt.Errorf("crypto.verifyPassword: not a hash hashPassword makes ($argon2id$, $2a$ / $2b$ / $2y$, $scrypt$)")
}

func bcryptError(fn string, err error) error {
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return fmt.Errorf("crypto.%s: bcrypt takes at most 72 bytes of password", fn)
	}
	return fmt.Errorf("crypto.%s: %v", fn, err)
}

// phcParams splits a PHC parameter list `a=1,b=2` into exactly the names
// wanted, in that order, each a decimal integer.
func phcParams(text string, names ...string) ([]int64, error) {
	parts := strings.Split(text, ",")
	if len(parts) != len(names) {
		return nil, fmt.Errorf("want parameters %s", strings.Join(names, ","))
	}
	out := make([]int64, len(names))
	for i, p := range parts {
		name, val, ok := strings.Cut(p, "=")
		if !ok || name != names[i] || val == "" || (len(val) > 1 && val[0] == '0') {
			return nil, fmt.Errorf("want parameters %s", strings.Join(names, ","))
		}
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("parameter %s must be a positive integer", name)
		}
		out[i] = n
	}
	return out, nil
}

// phcBytes decodes a salt or hash field, refusing padding and any length
// outside the bounds.
func phcBytes(field, what string, lo, hi int) ([]byte, error) {
	b, err := b64.Strict().DecodeString(field)
	if err != nil {
		return nil, fmt.Errorf("the %s is not unpadded base64", what)
	}
	if len(b) < lo || len(b) > hi {
		return nil, fmt.Errorf("the %s is %d bytes, not %d to %d", what, len(b), lo, hi)
	}
	return b, nil
}

func verifyArgon2id(pw []byte, stored string) (bool, error) {
	fail := func(err error) (bool, error) {
		return false, fmt.Errorf("crypto.verifyPassword: a malformed argon2id hash: %v", err)
	}
	f := strings.Split(stored, "$")
	if len(f) != 6 || f[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return fail(fmt.Errorf("want $argon2id$v=%d$m=..,t=..,p=..$salt$hash", argon2.Version))
	}
	p, err := phcParams(f[3], "m", "t", "p")
	if err != nil {
		return fail(err)
	}
	m, t, lanes := p[0], p[1], p[2]
	if m > maxArgonMemory || t > maxArgonTime || lanes > maxArgonThreads || m < 8*lanes {
		return false, fmt.Errorf("crypto.verifyPassword: argon2id m=%d,t=%d,p=%d is outside the limits (m 8*p to %d KiB, t to %d, p to %d)",
			m, t, lanes, maxArgonMemory, maxArgonTime, maxArgonThreads)
	}
	salt, err := phcBytes(f[4], "salt", minSaltLen, maxSaltLen)
	if err != nil {
		return fail(err)
	}
	want, err := phcBytes(f[5], "hash", 16, 1024)
	if err != nil {
		return fail(err)
	}
	got := argon2.IDKey(pw, salt, uint32(t), uint32(m), uint8(lanes), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func verifyScrypt(pw []byte, stored string) (bool, error) {
	fail := func(err error) (bool, error) {
		return false, fmt.Errorf("crypto.verifyPassword: a malformed scrypt hash: %v", err)
	}
	f := strings.Split(stored, "$")
	if len(f) != 5 {
		return fail(errors.New("want $scrypt$ln=..,r=..,p=..$salt$hash"))
	}
	p, err := phcParams(f[2], "ln", "r", "p")
	if err != nil {
		return fail(err)
	}
	ln, r, par := p[0], p[1], p[2]
	if err := scryptBounds(ln, r, par); err != nil {
		return false, fmt.Errorf("crypto.verifyPassword: %v", err)
	}
	salt, err := phcBytes(f[3], "salt", minSaltLen, maxSaltLen)
	if err != nil {
		return fail(err)
	}
	want, err := phcBytes(f[4], "hash", 16, 1024)
	if err != nil {
		return fail(err)
	}
	got, err := scrypt.Key(pw, salt, 1<<ln, int(r), int(par), len(want))
	if err != nil {
		return false, fmt.Errorf("crypto.verifyPassword: %v", err)
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// scryptBounds refuses scrypt parameters over the memory and parallelism caps.
func scryptBounds(ln, r, p int64) error {
	if ln < 1 || ln > 30 || r < 1 || p < 1 || p > maxScryptP || r > maxScryptMemory/(128<<ln) {
		return fmt.Errorf("scrypt ln=%d,r=%d,p=%d is outside the limits (128 * r * 2^ln up to %d bytes, p to %d)",
			ln, r, p, maxScryptMemory, maxScryptP)
	}
	return nil
}

// ----- raw KDFs -------------------------------------------------------------

// kdfInts reads the int arguments of argon2id / scrypt.
func kdfInts(fn, shape string, args []interpreter.Value) ([]int64, error) {
	if len(args) != 6 {
		return nil, fmt.Errorf("crypto.%s expects 6 arguments %s, got %d", fn, shape, len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("crypto.%s: password and salt must be bytes %s", fn, shape)
	}
	out := make([]int64, 4)
	for i := 0; i < 4; i++ {
		if args[2+i].Kind != interpreter.KindInt {
			return nil, fmt.Errorf("crypto.%s: argument %d must be int %s", fn, 3+i, shape)
		}
		out[i] = args[2+i].Int
	}
	if out[3] < 1 || out[3] > maxKeyLen {
		return nil, fmt.Errorf("crypto.%s: keyLen must be 1 to %d, got %d", fn, maxKeyLen, out[3])
	}
	if len(args[1].Bytes) < minSaltLen {
		return nil, fmt.Errorf("crypto.%s: the salt must be at least %d bytes, got %d", fn, minSaltLen, len(args[1].Bytes))
	}
	return out, nil
}

// argon2idFn implements crypto.argon2id(password, salt, time, memoryKiB,
// threads, keyLen) -> bytes (RFC 9106).
func argon2idFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := kdfInts("argon2id", "(password, salt, time, memoryKiB, threads, keyLen)", args)
	if err != nil {
		return interpreter.Null(), err
	}
	t, m, lanes := n[0], n[1], n[2]
	if t < 1 || t > maxArgonTime || lanes < 1 || lanes > maxArgonThreads || m < 8*lanes || m > maxArgonMemory {
		return interpreter.Null(), fmt.Errorf("crypto.argon2id: time 1 to %d, threads 1 to %d, memoryKiB 8*threads to %d; got %d, %d, %d",
			maxArgonTime, maxArgonThreads, maxArgonMemory, t, lanes, m)
	}
	return interpreter.BytesVal(argon2.IDKey(args[0].Bytes, args[1].Bytes, uint32(t), uint32(m), uint8(lanes), uint32(n[3]))), nil
}

// scryptFn implements crypto.scrypt(password, salt, n, r, p, keyLen) -> bytes
// (RFC 7914); n is a power of two.
func scryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	v, err := kdfInts("scrypt", "(password, salt, n, r, p, keyLen)", args)
	if err != nil {
		return interpreter.Null(), err
	}
	n, r, p := v[0], v[1], v[2]
	if n < 2 || n&(n-1) != 0 {
		return interpreter.Null(), fmt.Errorf("crypto.scrypt: n must be a power of two above 1, got %d", n)
	}
	ln := int64(0)
	for 1<<ln < n {
		ln++
	}
	if err := scryptBounds(ln, r, p); err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.scrypt: %v", err)
	}
	key, err := scrypt.Key(args[0].Bytes, args[1].Bytes, int(n), int(r), int(p), int(v[3]))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.scrypt: %v", err)
	}
	return interpreter.BytesVal(key), nil
}
