// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package cryptolib

// The Argon2id and scrypt memory caps on jennifer-tiny: 128 MiB each. A hash
// string read from storage sets the memory, and TinyGo's allocation failure is
// fatal, so the cap must fit a small target's heap (1 GiB is the default
// binary's). hashPassword's defaults (Argon2id 64 MiB, scrypt 128 MiB) fit.
const (
	maxArgonMemory  = 128 << 10 // KiB
	maxScryptMemory = 128 << 20 // bytes
)
