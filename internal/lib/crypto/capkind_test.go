// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestRandBytesCapIsKindLimit: a randBytes length past the 64 MiB cap is kind
// "limit"; a negative one stays an ordinary error.
func TestRandBytesCapIsKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := randBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(maxRandBytes + 1)}); !errors.As(err, &ce) {
		t.Errorf("over the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := randBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(-1)}); err == nil || errors.As(err, &ce) {
		t.Errorf("negative: %T %v, want a plain error", err, err)
	}
}

// TestKDFCapsAreKindLimit: the pbkdf2 keyLen and work caps are kind "limit";
// hkdf's bound (255 hash lengths) is the algorithm's, not a cap, so it stays
// an ordinary error, as does a non-positive length.
func TestKDFCapsAreKindLimit(t *testing.T) {
	b := interpreter.BytesVal([]byte("x"))
	algo := interpreter.StringVal("sha256")
	var ce *limits.CapError
	if _, err := hkdfFn(interpreter.BuiltinCtx{}, []interpreter.Value{algo, b, b, b, interpreter.IntVal(8161)}); err == nil || errors.As(err, &ce) {
		t.Errorf("hkdf past 255 hash lengths: %T %v, want a plain error", err, err)
	}
	if _, err := hkdfFn(interpreter.BuiltinCtx{}, []interpreter.Value{algo, b, b, b, interpreter.IntVal(0)}); err == nil || errors.As(err, &ce) {
		t.Errorf("hkdf length 0: %T %v, want a plain error", err, err)
	}
	pb := func(iterations, keyLen int64) error {
		_, err := pbkdf2Fn(interpreter.BuiltinCtx{}, []interpreter.Value{algo, b, b, interpreter.IntVal(iterations), interpreter.IntVal(keyLen)})
		return err
	}
	if err := pb(1, maxKeyLen+1); !errors.As(err, &ce) {
		t.Errorf("pbkdf2 over the keyLen cap: %T %v, want a limits.CapError", err, err)
	}
	if err := pb(maxPBKDFWork+1, 32); !errors.As(err, &ce) {
		t.Errorf("pbkdf2 over the work cap: %T %v, want a limits.CapError", err, err)
	}
	if err := pb(0, 32); err == nil || errors.As(err, &ce) {
		t.Errorf("pbkdf2 iterations 0: %T %v, want a plain error", err, err)
	}
}

// TestPasswordKDFCapsAreKindLimit: the argon2id, scrypt and verifyPassword caps
// (key length, memory, time, threads, bcrypt cost) are kind "limit", as
// pbkdf2's are; an invalid parameter (below its minimum) stays an ordinary error.
func TestPasswordKDFCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	kdf := func(fn func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error), n ...int64) error {
		args := []interpreter.Value{interpreter.BytesVal([]byte("pw")), interpreter.BytesVal([]byte("saltsalt"))}
		for _, v := range n {
			args = append(args, interpreter.IntVal(v))
		}
		_, err := fn(interpreter.BuiltinCtx{}, args)
		return err
	}
	for name, err := range map[string]error{
		"argon2id keyLen":  kdf(argon2idFn, 1, 1024, 1, maxKeyLen+1),
		"argon2id memory":  kdf(argon2idFn, 1, maxArgonMemory+1, 1, 32),
		"argon2id time":    kdf(argon2idFn, maxArgonTime+1, 1024, 1, 32),
		"argon2id threads": kdf(argon2idFn, 1, 1<<20, maxArgonThreads+1, 32),
		"scrypt keyLen":    kdf(scryptFn, 1024, 8, 1, maxKeyLen+1),
		"scrypt memory":    kdf(scryptFn, 1<<21, 8, 1, 32),
		"scrypt p":         kdf(scryptFn, 1024, 8, maxScryptP+1, 32),
	} {
		if !errors.As(err, &ce) {
			t.Errorf("%s: %T %v, want a limits.CapError", name, err, err)
		}
	}
	for name, err := range map[string]error{
		"argon2id time 0":   kdf(argon2idFn, 0, 1024, 1, 32),
		"argon2id memory 1": kdf(argon2idFn, 1, 1, 1, 32),
		"scrypt keyLen 0":   kdf(scryptFn, 1024, 8, 1, 0),
	} {
		if err == nil || errors.As(err, &ce) {
			t.Errorf("%s: %T %v, want a plain error", name, err, err)
		}
	}
	for _, stored := range []string{
		"$argon2id$v=19$m=4194304,t=2,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"$scrypt$ln=25,r=8,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"$2b$31$hXaEgd58.LJxfxHlvIoT8epQLsY4puU2fjnsbwprXwDZbw5QHhMg2",
	} {
		if _, err := verifyPw("x", stored); !errors.As(err, &ce) {
			t.Errorf("verifyPassword %s: %T %v, want a limits.CapError", stored[:12], err, err)
		}
	}
}
