// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package limits

import "fmt"

// CapError is a reached cap (a nesting depth, a node count, an input size): a
// Go error whose ErrorKind is "limit", so the interpreter raises it as
// Error{kind: "limit"} however many `%w` wrappers a library puts around it on
// the way out (the message is the wrapped text). A library builds one with
// Capf where it used fmt.Errorf; its callers wrap with %w, not %v.
type CapError struct {
	msg string
}

// Capf formats a CapError like fmt.Errorf.
func Capf(format string, a ...any) error {
	return &CapError{msg: fmt.Sprintf(format, a...)}
}

func (e *CapError) Error() string { return e.msg }

// ErrorKind is the Jennifer error kind the interpreter raises for it.
func (e *CapError) ErrorKind() string { return "limit" }
