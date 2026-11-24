// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package dnsconf

// CustomResolver is false on jennifer-tiny: it has no network library, so the
// files are reported by `jennifer version -v` but never read, and the system
// behaviour stays.
const CustomResolver = false
