// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

// jennifer-tiny stubs for the "mlkem768" algorithm: crypto/mlkem (and the FIPS
// 140 SHA-3 machinery under it) is off the TinyGo build.
package cryptolib

import "fmt"

func mlkemUnavailable(fn string) error {
	return fmt.Errorf("crypto.%s: \"mlkem768\" is not available on jennifer-tiny (it needs crypto/mlkem, which is off the TinyGo build); use the default jennifer binary", fn)
}

func mlkemKeypair(fn string) (pub, priv []byte, err error) {
	return nil, nil, mlkemUnavailable(fn)
}

func mlkemPublic(fn string, priv []byte) ([]byte, error) {
	return nil, mlkemUnavailable(fn)
}

func mlkemEncapsulate(fn string, pub []byte) (ct, secret []byte, err error) {
	return nil, nil, mlkemUnavailable(fn)
}

func mlkemDecapsulate(fn string, ct, priv []byte) ([]byte, error) {
	return nil, mlkemUnavailable(fn)
}
