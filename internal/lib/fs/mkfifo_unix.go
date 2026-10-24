// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix

package fslib

import "syscall"

func mkfifoAt(path string, mode uint32) error {
	return syscall.Mkfifo(path, mode)
}
