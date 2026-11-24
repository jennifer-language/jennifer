// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

// The Argon2id and scrypt memory caps on the default binary: 1 GiB each
// (verifyPassword at m=1 GiB, t=16 measured 1044 MB, 12.6 s).
const (
	maxArgonMemory  = 1 << 20 // KiB
	maxScryptMemory = 1 << 30 // bytes
)
