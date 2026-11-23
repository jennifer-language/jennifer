// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package dataerr is the error a built-in library returns when input data the
// program did not write itself is read and found bad: malformed JSON, a
// corrupt gzip stream, a ciphertext that fails authentication (M29.21). The
// interpreter raises it as Error{kind: <the library's kind>}, however many
// `%w` wrappers a library puts around it, so a handler can tell "the input was
// bad" from a fault in the program (kind "runtime") without reading the
// message. Misuse (a wrong argument type or count, an unknown algorithm name,
// a key of the wrong length) stays a plain error, kind "runtime".
package dataerr

import (
	"errors"
	"fmt"
)

// Error is malformed input data, raised with kind Kind.
type Error struct {
	kind string
	msg  string
}

// New formats an Error of the given kind like fmt.Errorf; `%w` is formatted
// as text, not wrapped (use Wrap to classify an existing error).
func New(kind, format string, a ...any) error {
	return &Error{kind: kind, msg: fmt.Sprintf(format, a...)}
}

// Wrap classifies err as a data error of the given kind, keeping its message.
// An err that already names a kind (a limits.CapError reached while decoding,
// a data error of an inner library) is returned unchanged: the generic kinds
// keep priority, and the innermost classification wins. A nil err is nil.
func Wrap(kind string, err error) error {
	if err == nil {
		return nil
	}
	var kinded interface{ ErrorKind() string }
	if errors.As(err, &kinded) {
		return err
	}
	return &Error{kind: kind, msg: err.Error()}
}

func (e *Error) Error() string { return e.msg }

// ErrorKind is the Jennifer error kind the interpreter raises for it.
func (e *Error) ErrorKind() string { return e.kind }

// KindOf returns the kind err names (through `%w` wrappers), or "" if none:
// "" is what the interpreter raises as "runtime". For tests.
func KindOf(err error) string {
	var kinded interface{ ErrorKind() string }
	if errors.As(err, &kinded) {
		return kinded.ErrorKind()
	}
	return ""
}
