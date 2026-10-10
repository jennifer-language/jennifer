// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

// TinyGo stub for the crypto library's post-quantum KEM surface (ML-KEM). The
// real implementation (cryptolib_pqc_std.go) pulls in crypto/mlkem, which drags
// in the FIPS 140 module machinery (SHA-3 / Keccak) that is not part of the
// TinyGo build, so jennifer-tiny returns a friendly "not available" error - the
// same build-tag split as the RSA / ECDSA surface. The symmetric primitives,
// Ed25519, AES-GCM, and the KDFs stay on both binaries; only the ML-KEM verbs
// are default-binary-only.
package cryptolib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func mlkemUnavailable(fn string) (interpreter.Value, error) {
	return interpreter.Null(), fmt.Errorf("crypto.%s is not available on jennifer-tiny (ML-KEM needs crypto/mlkem, which is off the TinyGo build); use the default jennifer binary", fn)
}

func mlkemKeypairFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return mlkemUnavailable("mlkemKeypair")
}

func mlkemEncapsulateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return mlkemUnavailable("mlkemEncapsulate")
}

func mlkemDecapsulateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return mlkemUnavailable("mlkemDecapsulate")
}
