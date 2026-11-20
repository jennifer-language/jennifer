// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !unix

package fslib

import (
	"fmt"
	"os"
)

// openSecret refuses on a platform without POSIX permission bits: reading
// the file unchecked would promise a check that did not happen (M29.22; a
// Windows ACL check belongs to M31).
func openSecret(fn, path string) (*os.File, error) {
	return nil, &secretRefusal{msg: fmt.Sprintf("%s: %s: this platform has no owner-only file permissions to check", fn, path)}
}
