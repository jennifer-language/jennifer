// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo && windows

package netlib

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// errnoReason maps a Winsock error number to NetError.reason, "" for one that
// names none of them. Go's syscall.ECONNREFUSED and friends are invented
// numbers on Windows, so the WSA codes are matched instead.
func errnoReason(errno syscall.Errno) string {
	switch errno {
	case windows.WSAECONNREFUSED:
		return "refused"
	case windows.WSAECONNRESET, windows.WSAECONNABORTED, syscall.ERROR_BROKEN_PIPE:
		return "reset"
	case windows.WSAENETUNREACH, windows.WSAEHOSTUNREACH, windows.WSAENETDOWN, windows.WSAEHOSTDOWN:
		return "unreachable"
	}
	return ""
}
