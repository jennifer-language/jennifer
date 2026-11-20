// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux

package fslib

import (
	"os"
	"syscall"
)

// stRdonly is ST_RDONLY in statfs(2)'s f_flags: the mount is read-only.
const stRdonly = 0x1

// mountReadOnly reports whether the open file lives on a read-only mount,
// asked of the handle (fstatfs), not the path. Any failure is "no", which
// keeps the strict check.
func mountReadOnly(f *os.File) bool {
	var st syscall.Statfs_t
	if err := syscall.Fstatfs(int(f.Fd()), &st); err != nil {
		return false
	}
	return st.Flags&stRdonly != 0
}
