// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package decimallib

// maxDigits is the digit cap of the default build: 5^4^3^2 (183,231 digits)
// fits, and a 1,000,000-digit multiply or toString takes about 50 ms.
const maxDigits = 1000000
