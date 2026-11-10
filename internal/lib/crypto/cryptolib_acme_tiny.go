// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

// TinyGo stubs for the crypto library's CSR / JWK surface (the ACME
// asymmetric operations). Like the RSA / ECDSA algorithm stubs, these need
// crypto/x509, which is off the TinyGo build, so jennifer-tiny returns a
// friendly "not available" error.
package cryptolib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func x509Unavailable(fn string) (interpreter.Value, error) {
	return interpreter.Null(), fmt.Errorf("crypto.%s is not available on jennifer-tiny (it needs crypto/x509, which is off the TinyGo build); use the default jennifer binary", fn)
}

func jwkPublicFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return x509Unavailable("jwkPublic")
}

func jwkToPemFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return x509Unavailable("jwkToPem")
}

func csrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return x509Unavailable("csr")
}
