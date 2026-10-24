// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// defaultFifoMode is the mode fs.mkfifo uses when none is given: owner-only,
// like the temp-entry verbs, since a pipe usually carries one user's data
// between that user's processes.
const defaultFifoMode = 0o600

// mkfifoFn creates a named pipe: fs.mkfifo(path[, mode]). mode is the
// permission bits in [0, 0o777] (the process umask still applies), default
// 0o600. An existing path is an error, as with mkfifo(1). Unix only: the call
// is in the platform-split mkfifoAt, which errors where there are no FIFOs.
func mkfifoFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("fs.mkfifo expects 1 or 2 arguments (path[, mode]), got %d", len(args))
	}
	path, err := takeStringArg("fs.mkfifo", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	mode := int64(defaultFifoMode)
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindInt {
			return interpreter.Null(), fmt.Errorf("fs.mkfifo: mode must be int, got %s", args[1].Kind)
		}
		mode = args[1].Int
		if mode < 0 || mode > 0o777 {
			return interpreter.Null(), fmt.Errorf("fs.mkfifo: mode %d out of range [0, 0o777]", mode)
		}
	}
	if ferr := mkfifoAt(path, uint32(mode)); ferr != nil {
		return interpreter.Null(), fmt.Errorf("fs.mkfifo: %s: %v", path, ferr)
	}
	return interpreter.Null(), nil
}
