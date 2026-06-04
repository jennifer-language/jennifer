// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package stdlib

import (
	"fmt"
	"io"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// Install registers stdlib functions on an interpreter.
// Call this before Interpreter.Run(prog).
func Install(in *interpreter.Interpreter) {
	in.Builtins["printf"] = printf
}

// printf: M1 form. Takes exactly one argument; writes its Display() form to out.
// No format specifiers yet - that lands in M3 once we have multi-arg call typing.
func printf(out io.Writer, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("printf expects 1 argument, got %d", len(args))
	}
	if _, err := io.WriteString(out, args[0].Display()); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}
