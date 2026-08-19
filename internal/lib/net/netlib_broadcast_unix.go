// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo && unix

package netlib

import "syscall"

// setSocketBroadcast sets (val=1) or clears (val=0) SO_BROADCAST on a raw
// socket fd. Unix path: the fd is an int.
func setSocketBroadcast(fd uintptr, val int) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, val)
}
