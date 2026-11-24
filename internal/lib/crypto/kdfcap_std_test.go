// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

import "testing"

// TestKDFMemoryCapValues pins the default binary's KDF memory caps at 1 GiB
// (jennifer-tiny's are 128 MiB, kdfcap_tiny_test.go).
func TestKDFMemoryCapValues(t *testing.T) {
	if maxArgonMemory != 1<<20 || maxScryptMemory != 1<<30 {
		t.Fatalf("maxArgonMemory %d KiB, maxScryptMemory %d bytes; want 1 GiB each", maxArgonMemory, maxScryptMemory)
	}
}
