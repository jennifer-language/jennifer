// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build tinygo

// TinyGo stubs for the crypto library's key-generation / CSR / JWK surface (the
// ACME asymmetric operations). Like the sign / verify stubs, these need
// crypto/rsa, crypto/ecdsa, and crypto/x509, which are off the TinyGo build, so
// jennifer-tiny returns a friendly "not available" error.
package cryptolib

import "jennifer-lang.dev/jennifer/internal/interpreter"

func rsaGenerateKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("rsaGenerateKey")
}

func ecGenerateKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("ecGenerateKey")
}

func jwkPublicFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("jwkPublic")
}

func csrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("csr")
}
