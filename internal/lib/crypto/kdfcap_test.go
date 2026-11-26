// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"errors"
	"fmt"
	"math/bits"
	"testing"

	"golang.org/x/crypto/scrypt"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestScryptParallelismCap: p up to 4 runs, p 5 is a "limit" error. Before
// 0.27.0 the cap was 16 (ln=20, r=8, p=16 took 48 s).
func TestScryptParallelismCap(t *testing.T) {
	var ce *limits.CapError
	args := func(p int64) []interpreter.Value {
		return []interpreter.Value{interpreter.BytesVal([]byte("pw")), interpreter.BytesVal([]byte("saltsalt")),
			interpreter.IntVal(16), interpreter.IntVal(1), interpreter.IntVal(p), interpreter.IntVal(16)}
	}
	if _, err := scryptFn(interpreter.BuiltinCtx{}, args(4)); err != nil {
		t.Fatalf("p=4: %v", err)
	}
	if _, err := scryptFn(interpreter.BuiltinCtx{}, args(5)); !errors.As(err, &ce) {
		t.Fatalf("p=5: %T %v, want a limits.CapError", err, err)
	}
}

// TestScryptMemoryCapBoundary: 128 * r * 2^ln at maxScryptMemory passes the
// bounds check, one step of r past it does not.
func TestScryptMemoryCapBoundary(t *testing.T) {
	ln := int64(bits.TrailingZeros64(maxScryptMemory / 128))
	if err := scryptBounds(ln, 1, 1, maxScryptP); err != nil {
		t.Fatalf("at the cap (ln=%d, r=1): %v", ln, err)
	}
	if err := scryptBounds(ln, 2, 1, maxScryptP); err == nil {
		t.Fatalf("past the cap (ln=%d, r=2) passed", ln)
	}
}

// A stored scrypt hash verifies up to p 16, the cap before 0.28.0, so hashes
// written then still check; p 17 is "limit". New derivations stay at p 4.
func TestScryptVerifyAcceptsOlderParallelism(t *testing.T) {
	salt := []byte("saltsalt")
	stored := func(p int) string {
		key, err := scrypt.Key([]byte("pw"), salt, 16, 1, p, 16)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("$scrypt$ln=4,r=1,p=%d$%s$%s", p, b64.EncodeToString(salt), b64.EncodeToString(key))
	}
	ok, err := verifyScrypt([]byte("pw"), stored(16))
	if err != nil || !ok {
		t.Fatalf("p=16: %v, %v", ok, err)
	}
	var ce *limits.CapError
	if _, err := verifyScrypt([]byte("pw"), stored(17)); !errors.As(err, &ce) {
		t.Fatalf("p=17: %T %v, want a limits.CapError", err, err)
	}
}
