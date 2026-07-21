// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

// TinyGo stub for the crypto library's asymmetric RSA / ECDSA surface. The real
// implementation (cryptolib_asym_std.go) pulls in crypto/rsa, crypto/ecdsa, and
// crypto/x509, which are not part of the TinyGo build, so jennifer-tiny returns
// a friendly "not available" error - the same build-tag split as `net`. The
// symmetric primitives, Ed25519 (crypto.sign / verify), AES-GCM, and the KDFs
// all stay on both binaries; only RSA / ECDSA (the JWT RS* / ES* algorithms)
// are default-binary-only.
package cryptolib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func asymUnavailable(fn string) (interpreter.Value, error) {
	return interpreter.Null(), fmt.Errorf("crypto.%s is not available on jennifer-tiny (RSA / ECDSA need crypto/x509, which is off the TinyGo build); use the default jennifer binary, or an HS* / EdDSA algorithm", fn)
}

func rsaSignFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("rsaSign")
}

func rsaVerifyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("rsaVerify")
}

func ecdsaSignFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("ecdsaSign")
}

func ecdsaVerifyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return asymUnavailable("ecdsaVerify")
}
