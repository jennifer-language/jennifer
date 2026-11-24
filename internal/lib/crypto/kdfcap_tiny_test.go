// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package cryptolib

import "testing"

// TestKDFMemoryCapValues pins jennifer-tiny's KDF memory caps at 128 MiB
// (the default binary's are 1 GiB, kdfcap_std_test.go).
func TestKDFMemoryCapValues(t *testing.T) {
	if maxArgonMemory != 128<<10 || maxScryptMemory != 128<<20 {
		t.Fatalf("maxArgonMemory %d KiB, maxScryptMemory %d bytes; want 128 MiB each", maxArgonMemory, maxScryptMemory)
	}
}
