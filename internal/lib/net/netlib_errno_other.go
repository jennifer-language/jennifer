// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo && !windows

package netlib

import "syscall"

// errnoReason maps an OS socket error number to NetError.reason, "" for one
// that names none of them.
func errnoReason(errno syscall.Errno) string {
	switch errno {
	case syscall.ECONNREFUSED:
		return "refused"
	case syscall.ECONNRESET, syscall.EPIPE, syscall.ECONNABORTED:
		return "reset"
	case syscall.ENETUNREACH, syscall.EHOSTUNREACH, syscall.ENETDOWN, syscall.EHOSTDOWN:
		return "unreachable"
	}
	return ""
}
