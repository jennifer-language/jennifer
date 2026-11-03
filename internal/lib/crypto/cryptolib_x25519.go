// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

// X25519 Diffie-Hellman key agreement (RFC 7748), over crypto/ecdh. Both sides
// draw a keypair, swap public keys, and x25519(ownPrivate, peerPublic) gives
// each the same 32-byte shared secret - raw curve output, to be run through
// crypto.hkdf before use as a key. It is the classical half of a hybrid with
// ML-KEM and the exchange under Noise / age / WireGuard-style protocols.
// crypto/ecdh builds under TinyGo, so unlike the ML-KEM / RSA / ECDSA surface
// this is on both binaries (no build-tag split).

import (
	"crypto/ecdh"
	crand "crypto/rand"
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// x25519KeyLen is the length of an X25519 private key, public key and shared
// secret alike.
const x25519KeyLen = 32

// x25519KeypairFn implements crypto.x25519Keypair() -> crypto.Keypair, a fresh
// keypair from crypto/rand (32-byte public, 32-byte private).
func x25519KeypairFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("crypto.x25519Keypair expects no arguments, got %d", len(args))
	}
	priv, err := ecdh.X25519().GenerateKey(crand.Reader)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.x25519Keypair: %v", err)
	}
	return interpreter.NamespacedStructVal(LibraryName, "Keypair", []interpreter.StructField{
		{Name: "public", Value: interpreter.BytesVal(priv.PublicKey().Bytes())},
		{Name: "private", Value: interpreter.BytesVal(priv.Bytes())},
	}), nil
}

// x25519Private reads a 32-byte private-key argument.
func x25519Private(fn string, v interpreter.Value) (*ecdh.PrivateKey, error) {
	if v.Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("%s: private key must be bytes", fn)
	}
	if len(v.Bytes) != x25519KeyLen {
		return nil, fmt.Errorf("%s: private key must be %d bytes (from crypto.x25519Keypair), got %d", fn, x25519KeyLen, len(v.Bytes))
	}
	priv, err := ecdh.X25519().NewPrivateKey(v.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid private key: %v", fn, err)
	}
	return priv, nil
}

// x25519Fn implements crypto.x25519(private, peerPublic) -> bytes, the 32-byte
// shared secret. A peer public key of low order (one that would force an
// all-zero secret) is refused, as RFC 7748 section 6.1 allows, so a hostile
// peer cannot pin the secret to a known value.
func x25519Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	const fn = "crypto.x25519"
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (private, peerPublic), got %d", fn, len(args))
	}
	priv, err := x25519Private(fn, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: peer public key must be bytes", fn)
	}
	if len(args[1].Bytes) != x25519KeyLen {
		return interpreter.Null(), fmt.Errorf("%s: peer public key must be %d bytes, got %d", fn, x25519KeyLen, len(args[1].Bytes))
	}
	pub, err := ecdh.X25519().NewPublicKey(args[1].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: invalid peer public key: %v", fn, err)
	}
	secret, err := priv.ECDH(pub)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: peer public key is a low-order point (the shared secret would be all zero)", fn)
	}
	return interpreter.BytesVal(secret), nil
}

// x25519PublicFn implements crypto.x25519Public(private) -> bytes, the public
// key belonging to a stored private key.
func x25519PublicFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	const fn = "crypto.x25519Public"
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (private), got %d", fn, len(args))
	}
	priv, err := x25519Private(fn, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(priv.PublicKey().Bytes()), nil
}
