// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

// The crypto operations, one function per operation with the algorithm as its
// first argument (docs/user-guide/style-guide.md, "Function signatures"): a new
// algorithm is a new name, never a new function or a changed signature.
//
//	encrypt / decrypt          authenticated encryption   "aes-gcm", "chacha20-poly1305", "xchacha20-poly1305"
//	blockEncrypt / Decrypt     block modes, no MAC        "aes-ecb-nopad" ...
//	sign / verify              signatures                 "ed25519", "rsa-sha256", "ecdsa-sha256" ...
//	keypair / publicKey        keys                       "ed25519", "x25519", "mlkem768", "rsa2048", "p256" ...
//	agree                      key agreement              "x25519"
//	encapsulate / decapsulate  KEM                        "mlkem768"
//	wrap / unwrap              encrypt to a public key    "rsa-oaep-sha1", "rsa-oaep-sha256", "nacl-sealedbox"
//
// AES, Ed25519 and X25519 run on both binaries and live here. RSA, ECDSA and
// RSA-OAEP (crypto/x509) and ML-KEM (crypto/mlkem) are off the TinyGo build: the
// helpers they call are in the *_std.go files, and the *_tiny.go files stub them
// with an error naming the algorithm.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	crand "crypto/rand"
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// The algorithm names each operation accepts, in the order error messages list
// them.
var (
	aeadAlgos    = []string{"aes-gcm", "chacha20-poly1305", "xchacha20-poly1305"}
	blockAlgos   = []string{"aes-ecb-nopad", "aes-ecb-pkcs7", "aes-cbc-nopad", "aes-cbc-pkcs7", "aes-ctr", "aes-cfb"}
	signAlgos    = []string{"ed25519", "rsa-sha256", "rsa-sha384", "rsa-sha512", "ecdsa-sha256", "ecdsa-sha384", "ecdsa-sha512"}
	keypairAlgos = []string{"ed25519", "x25519", "mlkem768", "rsa2048", "rsa3072", "rsa4096", "p256", "p384", "p521"}
	agreeAlgos   = []string{"x25519"}
	kemAlgos     = []string{"mlkem768"}
	wrapAlgos    = []string{"rsa-oaep-sha1", "rsa-oaep-sha256", "nacl-sealedbox"}
)

// unknownAlgo is the error for a name the operation does not know.
func unknownAlgo(fn, algo string, known []string) error {
	return fmt.Errorf("crypto.%s: unknown algorithm %q; known: %s", fn, algo, quoteList(known))
}

func quoteList(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ")
}

// opArgs checks the arity and that the first argument is the algorithm string,
// then that every other argument is bytes (an index in nullable may also be
// null). It returns the algorithm.
func opArgs(fn, shape string, args []interpreter.Value, want int, nullable int) (string, error) {
	if len(args) != want {
		return "", fmt.Errorf("crypto.%s expects %d arguments %s, got %d", fn, want, shape, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return "", fmt.Errorf("crypto.%s: the algorithm (first argument) must be a string, got %s", fn, args[0].Kind)
	}
	for i := 1; i < want; i++ {
		if i == nullable && args[i].Kind == interpreter.KindNull {
			continue
		}
		if args[i].Kind != interpreter.KindBytes {
			return "", fmt.Errorf("crypto.%s: argument %d must be bytes, got %s %s", fn, i+1, args[i].Kind, shape)
		}
	}
	return args[0].Str, nil
}

// ----- authenticated encryption: encrypt / decrypt -------------------------

// gcmNonceLen is the only GCM nonce size accepted: 96 bits, the size GCM is
// specified for (a longer nonce is hashed down, and nothing interoperates with it).
const gcmNonceLen = 12

// gcmMaxPlaintext is GCM's structural per-message limit: (2^32 - 2) blocks of 16
// bytes (~68 GiB). Go's gcm.Seal *panics* above it, and a builtin panic is not
// catchable, so encrypt bounds the plaintext to a catchable error instead.
const gcmMaxPlaintext = uint64(((1 << 32) - 2) * 16)

// aesKey checks an AES key length (AES-128 / 192 / 256).
func aesKey(fn string, key []byte) error {
	switch len(key) {
	case 16, 24, 32:
		return nil
	}
	return fmt.Errorf("crypto.%s: key must be 16, 24 or 32 bytes (AES-128 / 192 / 256), got %d", fn, len(key))
}

// gcmFor builds AES-GCM for encrypt / decrypt after checking key and nonce.
func gcmFor(fn string, key, nonce []byte) (cipher.AEAD, error) {
	if err := aesKey(fn, key); err != nil {
		return nil, err
	}
	if len(nonce) != gcmNonceLen {
		return nil, fmt.Errorf("crypto.%s: nonce must be %d bytes, got %d", fn, gcmNonceLen, len(nonce))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return gcm, nil
}

// encryptFn implements crypto.encrypt(algo, plaintext, key, nonce, aad) -> bytes:
// the ciphertext with the 16-byte tag appended. The caller supplies the nonce,
// and must never repeat one under the same key.
func encryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("encrypt", "(algo, plaintext, key, nonce, aad)", args, 5, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	plaintext, key, nonce, aad := args[1].Bytes, args[2].Bytes, args[3].Bytes, args[4].Bytes
	aead, most, err := aeadFor("encrypt", algo, key, nonce)
	if err != nil {
		return interpreter.Null(), err
	}
	if uint64(len(plaintext)) > most {
		return interpreter.Null(), fmt.Errorf("crypto.encrypt: plaintext too large for a single %s message (max %d bytes); split it into chunks", algo, most)
	}
	return interpreter.BytesVal(aead.Seal(nil, nonce, plaintext, aad)), nil
}

// decryptFn implements crypto.decrypt(algo, box, key, nonce, aad) -> bytes. A
// wrong key, nonce or aad, or a tampered box, is one authentication error.
func decryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("decrypt", "(algo, box, key, nonce, aad)", args, 5, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	box, key, nonce, aad := args[1].Bytes, args[2].Bytes, args[3].Bytes, args[4].Bytes
	aead, _, err := aeadFor("decrypt", algo, key, nonce)
	if err != nil {
		return interpreter.Null(), err
	}
	pt, oerr := aead.Open(nil, nonce, box, aad)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("crypto.decrypt: authentication failed (wrong key, nonce or aad, or tampered data)")
	}
	return interpreter.BytesVal(pt), nil
}

// ----- block modes, no authentication: blockEncrypt / blockDecrypt ---------

// blockMode splits a block algorithm name into its mode and whether it pads.
func blockMode(fn, algo string) (mode string, pkcs7 bool, err error) {
	switch algo {
	case "aes-ecb-nopad":
		return "ecb", false, nil
	case "aes-ecb-pkcs7":
		return "ecb", true, nil
	case "aes-cbc-nopad":
		return "cbc", false, nil
	case "aes-cbc-pkcs7":
		return "cbc", true, nil
	case "aes-ctr":
		return "ctr", false, nil
	case "aes-cfb":
		return "cfb", false, nil
	}
	return "", false, unknownAlgo(fn, algo, blockAlgos)
}

func blockFn(fn string, encrypt bool, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs(fn, "(algo, data, key, iv)", args, 4, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	mode, pkcs7, err := blockMode(fn, algo)
	if err != nil {
		return interpreter.Null(), err
	}
	data, key := args[1].Bytes, args[2].Bytes
	if err := aesKey(fn, key); err != nil {
		return interpreter.Null(), err
	}
	ivNull := args[3].Kind == interpreter.KindNull
	if mode == "ecb" && !ivNull {
		return interpreter.Null(), fmt.Errorf("crypto.%s: %q takes no IV; pass null", fn, algo)
	}
	if mode != "ecb" {
		if ivNull {
			return interpreter.Null(), fmt.Errorf("crypto.%s: %q needs a %d-byte IV", fn, algo, aes.BlockSize)
		}
		if len(args[3].Bytes) != aes.BlockSize {
			return interpreter.Null(), fmt.Errorf("crypto.%s: IV must be %d bytes, got %d", fn, aes.BlockSize, len(args[3].Bytes))
		}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.%s: %v", fn, err)
	}
	out, err := runBlock(fn, block, mode, pkcs7, encrypt, data, args[3].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(out), nil
}

// runBlock applies one mode. The stream modes (CTR, CFB) take any length; ECB
// and CBC take whole blocks, padded with PKCS#7 when the name says so.
func runBlock(fn string, block cipher.Block, mode string, pkcs7, encrypt bool, data, iv []byte) ([]byte, error) {
	bs := block.BlockSize()
	switch mode {
	case "ctr":
		out := make([]byte, len(data))
		cipher.NewCTR(block, iv).XORKeyStream(out, data)
		return out, nil
	case "cfb":
		out := make([]byte, len(data))
		if encrypt {
			cipher.NewCFBEncrypter(block, iv).XORKeyStream(out, data) //nolint:staticcheck // CFB is the interop mode asked for (SNMPv3)
		} else {
			cipher.NewCFBDecrypter(block, iv).XORKeyStream(out, data) //nolint:staticcheck // as above
		}
		return out, nil
	}
	in := data
	if encrypt && pkcs7 {
		pad := bs - len(data)%bs
		in = append(append(make([]byte, 0, len(data)+pad), data...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	}
	if len(in)%bs != 0 {
		return nil, fmt.Errorf("crypto.%s: input must be a multiple of %d bytes for a -nopad mode, got %d", fn, bs, len(in))
	}
	if !encrypt && pkcs7 && len(in) == 0 {
		return nil, fmt.Errorf("crypto.%s: bad padding", fn)
	}
	out := make([]byte, len(in))
	switch {
	case mode == "cbc" && encrypt:
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, in)
	case mode == "cbc":
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, in)
	default: // ECB: each block on its own
		for i := 0; i < len(in); i += bs {
			if encrypt {
				block.Encrypt(out[i:i+bs], in[i:i+bs])
			} else {
				block.Decrypt(out[i:i+bs], in[i:i+bs])
			}
		}
	}
	if !encrypt && pkcs7 {
		return unpad(fn, out, bs)
	}
	return out, nil
}

// unpad strips PKCS#7 padding, checking every pad byte.
func unpad(fn string, b []byte, bs int) ([]byte, error) {
	n := int(b[len(b)-1])
	if n == 0 || n > bs || n > len(b) {
		return nil, fmt.Errorf("crypto.%s: bad padding", fn)
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, fmt.Errorf("crypto.%s: bad padding", fn)
		}
	}
	return b[:len(b)-n], nil
}

func blockEncryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return blockFn("blockEncrypt", true, args)
}

func blockDecryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return blockFn("blockDecrypt", false, args)
}

// ----- signatures: sign / verify ------------------------------------------

// signFn implements crypto.sign(algo, message, privateKey) -> bytes.
func signFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("sign", "(algo, message, privateKey)", args, 3, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	msg, key := args[1].Bytes, args[2].Bytes
	var sig []byte
	switch algo {
	case "ed25519":
		priv, perr := ed25519Private("sign", key)
		if perr != nil {
			return interpreter.Null(), perr
		}
		sig = ed25519.Sign(priv, msg)
	case "rsa-sha256", "rsa-sha384", "rsa-sha512":
		sig, err = rsaSign("sign", algo, strings.TrimPrefix(algo, "rsa-"), msg, key)
	case "ecdsa-sha256", "ecdsa-sha384", "ecdsa-sha512":
		sig, err = ecdsaSign("sign", algo, strings.TrimPrefix(algo, "ecdsa-"), msg, key)
	default:
		return interpreter.Null(), unknownAlgo("sign", algo, signAlgos)
	}
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(sig), nil
}

// verifyFn implements crypto.verify(algo, message, signature, publicKey) -> bool.
// A genuine mismatch is false; a malformed key is an error.
func verifyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("verify", "(algo, message, signature, publicKey)", args, 4, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	msg, sig, key := args[1].Bytes, args[2].Bytes, args[3].Bytes
	var ok bool
	switch algo {
	case "ed25519":
		if len(key) != ed25519.PublicKeySize {
			return interpreter.Null(), fmt.Errorf("crypto.verify: an ed25519 public key is %d raw bytes, got %d", ed25519.PublicKeySize, len(key))
		}
		ok = len(sig) == ed25519.SignatureSize && ed25519.Verify(ed25519.PublicKey(key), msg, sig)
	case "rsa-sha256", "rsa-sha384", "rsa-sha512":
		ok, err = rsaVerify("verify", algo, strings.TrimPrefix(algo, "rsa-"), msg, sig, key)
	case "ecdsa-sha256", "ecdsa-sha384", "ecdsa-sha512":
		ok, err = ecdsaVerify("verify", algo, strings.TrimPrefix(algo, "ecdsa-"), msg, sig, key)
	default:
		return interpreter.Null(), unknownAlgo("verify", algo, signAlgos)
	}
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(ok), nil
}

// ed25519Private checks a raw 64-byte Ed25519 private key (seed || public).
// Ed25519 hashes the stored public half into each signature, so a public half
// that does not belong to the seed is refused: two signatures of one message
// under such a key would reveal the private scalar.
func ed25519Private(fn string, key []byte) (ed25519.PrivateKey, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("crypto.%s: an ed25519 private key is %d raw bytes (from crypto.keypair(\"ed25519\")), got %d", fn, ed25519.PrivateKeySize, len(key))
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])[ed25519.SeedSize:], key[ed25519.SeedSize:]) {
		return nil, fmt.Errorf("crypto.%s: ed25519 private key's public half does not match its seed", fn)
	}
	return ed25519.PrivateKey(key), nil
}

// ----- keys: keypair / publicKey -------------------------------------------

func keypairVal(pub, priv []byte) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Keypair", []interpreter.StructField{
		{Name: "public", Value: interpreter.BytesVal(pub)},
		{Name: "private", Value: interpreter.BytesVal(priv)},
	})
}

// keypairFn implements crypto.keypair(algo) -> crypto.Keypair{public, private}.
// Raw bytes for ed25519 / x25519 / mlkem768; PEM for RSA and EC (private
// PKCS#8 / SEC1, public PKIX).
func keypairFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("keypair", "(algo)", args, 1, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	var pub, priv []byte
	switch algo {
	case "ed25519":
		p, s, gerr := ed25519.GenerateKey(crand.Reader)
		if gerr != nil {
			return interpreter.Null(), fmt.Errorf("crypto.keypair: %v", gerr)
		}
		pub, priv = p, s
	case "x25519":
		k, gerr := ecdh.X25519().GenerateKey(crand.Reader)
		if gerr != nil {
			return interpreter.Null(), fmt.Errorf("crypto.keypair: %v", gerr)
		}
		pub, priv = k.PublicKey().Bytes(), k.Bytes()
	case "mlkem768":
		pub, priv, err = mlkemKeypair("keypair")
	case "rsa2048", "rsa3072", "rsa4096":
		pub, priv, err = rsaKeypair("keypair", algo, rsaBits(algo))
	case "p256", "p384", "p521":
		pub, priv, err = ecKeypair("keypair", algo)
	default:
		return interpreter.Null(), unknownAlgo("keypair", algo, keypairAlgos)
	}
	if err != nil {
		return interpreter.Null(), err
	}
	return keypairVal(pub, priv), nil
}

func rsaBits(algo string) int {
	switch algo {
	case "rsa3072":
		return 3072
	case "rsa4096":
		return 4096
	}
	return 2048
}

// publicKeyFn implements crypto.publicKey(algo, privateKey) -> bytes: the public
// key of a stored private key, in the form keypair returns it.
func publicKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("publicKey", "(algo, privateKey)", args, 2, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	key := args[1].Bytes
	var pub []byte
	switch algo {
	case "ed25519":
		priv, perr := ed25519Private("publicKey", key)
		if perr != nil {
			return interpreter.Null(), perr
		}
		pub = []byte(priv.Public().(ed25519.PublicKey))
	case "x25519":
		priv, perr := x25519Private("publicKey", key)
		if perr != nil {
			return interpreter.Null(), perr
		}
		pub = priv.PublicKey().Bytes()
	case "mlkem768":
		pub, err = mlkemPublic("publicKey", key)
	case "rsa2048", "rsa3072", "rsa4096":
		pub, err = rsaPublicPEM("publicKey", algo, rsaBits(algo), key)
	case "p256", "p384", "p521":
		pub, err = ecPublicPEM("publicKey", algo, key)
	default:
		return interpreter.Null(), unknownAlgo("publicKey", algo, keypairAlgos)
	}
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(pub), nil
}

// ----- key agreement and KEM ----------------------------------------------

// x25519KeyLen is the length of an X25519 key, private or public.
const x25519KeyLen = 32

func x25519Private(fn string, key []byte) (*ecdh.PrivateKey, error) {
	if len(key) != x25519KeyLen {
		return nil, fmt.Errorf("crypto.%s: an x25519 private key is %d raw bytes, got %d", fn, x25519KeyLen, len(key))
	}
	priv, err := ecdh.X25519().NewPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: invalid x25519 private key: %v", fn, err)
	}
	return priv, nil
}

// agreeFn implements crypto.agree(algo, privateKey, peerPublic) -> bytes: the
// shared secret, to be run through crypto.hkdf before use as a key.
func agreeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("agree", "(algo, privateKey, peerPublic)", args, 3, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	if algo != "x25519" {
		return interpreter.Null(), unknownAlgo("agree", algo, agreeAlgos)
	}
	priv, err := x25519Private("agree", args[1].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(args[2].Bytes) != x25519KeyLen {
		return interpreter.Null(), fmt.Errorf("crypto.agree: an x25519 peer public key is %d raw bytes, got %d", x25519KeyLen, len(args[2].Bytes))
	}
	pub, err := ecdh.X25519().NewPublicKey(args[2].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.agree: invalid peer public key: %v", err)
	}
	secret, err := priv.ECDH(pub)
	if err != nil {
		// A low-order peer point would force an all-zero secret a hostile peer
		// can predict.
		return interpreter.Null(), fmt.Errorf("crypto.agree: peer public key is a low-order point (the shared secret would be all zero)")
	}
	return interpreter.BytesVal(secret), nil
}

// encapsulateFn implements crypto.encapsulate(algo, publicKey) ->
// crypto.Encapsulation{ciphertext, sharedSecret}.
func encapsulateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("encapsulate", "(algo, publicKey)", args, 2, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	if algo != "mlkem768" {
		return interpreter.Null(), unknownAlgo("encapsulate", algo, kemAlgos)
	}
	ct, secret, err := mlkemEncapsulate("encapsulate", args[1].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.NamespacedStructVal(LibraryName, "Encapsulation", []interpreter.StructField{
		{Name: "ciphertext", Value: interpreter.BytesVal(ct)},
		{Name: "sharedSecret", Value: interpreter.BytesVal(secret)},
	}), nil
}

// decapsulateFn implements crypto.decapsulate(algo, ciphertext, privateKey) ->
// bytes. A ciphertext made for another key yields a different, useless secret
// (implicit rejection), not an error.
func decapsulateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("decapsulate", "(algo, ciphertext, privateKey)", args, 3, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	if algo != "mlkem768" {
		return interpreter.Null(), unknownAlgo("decapsulate", algo, kemAlgos)
	}
	secret, err := mlkemDecapsulate("decapsulate", args[1].Bytes, args[2].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(secret), nil
}

// ----- RSA key wrapping: wrap / unwrap ------------------------------------

// wrapFn implements crypto.wrap(algo, plaintext, publicKey) -> bytes (RSAES-OAEP).
func wrapFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("wrap", "(algo, plaintext, publicKey)", args, 3, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	if algo == "nacl-sealedbox" {
		return sealedBoxSeal(args[1].Bytes, args[2].Bytes)
	}
	if algo != "rsa-oaep-sha1" && algo != "rsa-oaep-sha256" {
		return interpreter.Null(), unknownAlgo("wrap", algo, wrapAlgos)
	}
	out, err := rsaOAEP("wrap", algo, true, args[1].Bytes, args[2].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(out), nil
}

// unwrapFn implements crypto.unwrap(algo, ciphertext, privateKey) -> bytes.
func unwrapFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	algo, err := opArgs("unwrap", "(algo, ciphertext, privateKey)", args, 3, -1)
	if err != nil {
		return interpreter.Null(), err
	}
	if algo == "nacl-sealedbox" {
		return sealedBoxOpen(args[1].Bytes, args[2].Bytes)
	}
	if algo != "rsa-oaep-sha1" && algo != "rsa-oaep-sha256" {
		return interpreter.Null(), unknownAlgo("unwrap", algo, wrapAlgos)
	}
	out, err := rsaOAEP("unwrap", algo, false, args[1].Bytes, args[2].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(out), nil
}
