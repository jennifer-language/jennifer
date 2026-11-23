// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib

import (
	"bytes"
	"fmt"
	"io"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// maxSecretBytes caps fs.readSecretBytes / readSecretString: a key, token or password file is small,
// and a path pointed at something huge must not be read whole.
const maxSecretBytes = 1 << 20

// secretKind is the kind of a refused secret file (M29.22): the file is
// there, but reading it would trust a secret others could have read or
// replaced.
const secretKind = "fs"

// secretRefusal is a refused secret file; raiseSecret turns it into an
// Error of kind secretKind.
type secretRefusal struct{ msg string }

func (e *secretRefusal) Error() string { return e.msg }

func readSecretFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	data, err := readSecret(ctx, "fs.readSecretBytes", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BytesVal(data), nil
}

func readSecretStringFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	data, err := readSecret(ctx, "fs.readSecretString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	s := string(data)
	if err := dataerr.Wrap(dataKind, limits.CheckUTF8("fs.readSecretString", s)); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(s), nil
}

// readSecret opens the file and checks the open handle, not the path, so the
// file read is the file checked: owner-only permissions, owned by the
// effective user (or root), a regular file once links are followed.
func readSecret(ctx interpreter.BuiltinCtx, fn string, args []Value) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s expects 1 argument (path), got %d", fn, len(args))
	}
	path, err := takeStringArg(fn, args, 0, "path")
	if err != nil {
		return nil, err
	}
	f, err := openSecret(fn, path)
	if err != nil {
		if r, ok := err.(*secretRefusal); ok {
			return nil, interpreter.RaiseError(secretKind, r.msg, ctx.File, ctx.Line, ctx.Col)
		}
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(io.LimitReader(f, maxSecretBytes+1)); err != nil {
		return nil, fmt.Errorf("%s: %s: %s", fn, path, osErr(err, path))
	}
	if buf.Len() > maxSecretBytes {
		return nil, limits.Capf("%s: %s exceeds the %d-byte limit for a secret", fn, path, maxSecretBytes)
	}
	return buf.Bytes(), nil
}
