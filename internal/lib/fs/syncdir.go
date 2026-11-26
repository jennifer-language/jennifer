// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib

import (
	"fmt"
	"os"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// syncDirFn implements fs.syncDir(path): flush a directory's entries to the
// storage device, so a file created, renamed or removed in it survives a crash.
// fs.sync flushes a file's data; the name that points at it lives in the
// directory, so a write-temp-then-rename save is durable only once the
// directory is synced too. path must be a directory. The flush itself is in the
// platform-split syncDirAt (a no-op where the OS has no directory fsync).
func syncDirFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.syncDir expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.syncDir", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	info, serr := os.Stat(path)
	if serr != nil {
		return interpreter.Null(), fmt.Errorf("fs.syncDir: %s: %s", path, osErr(serr, path))
	}
	if !info.IsDir() {
		return interpreter.Null(), fmt.Errorf("fs.syncDir: %s: not a directory", path)
	}
	if ferr := syncDirAt(path); ferr != nil {
		return interpreter.Null(), fmt.Errorf("fs.syncDir: %s: %s", path, osErr(ferr, path))
	}
	return interpreter.Null(), nil
}
