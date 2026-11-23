package cryptolib

import "jennifer-lang.dev/jennifer/internal/interpreter"

// The cap and vector tests were written against the per-algorithm builtins
// (crypto.hkdf, pbkdf2, argon2id, scrypt), removed in 0.28.0 for
// crypto.deriveKey. The cores they wrapped are what deriveKey runs, so the
// tests keep calling them under the old names.

func hkdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return hkdfCore("crypto.hkdf", args)
}

func pbkdf2Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return pbkdf2Core("crypto.pbkdf2", args)
}

func argon2idFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return argon2idCore("crypto.argon2id", args)
}

func scryptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return scryptCore("crypto.scrypt", args)
}
