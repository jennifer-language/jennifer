// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package decimallib

// maxDigits stays at 100,000 on jennifer-tiny: TinyGo's math/big takes
// 1.2 s for a 1,000,000-digit toString and 0.8 s for a division to scale
// 999,999, against 0.3 s and 0.1 s on the default build.
const maxDigits = 100000
