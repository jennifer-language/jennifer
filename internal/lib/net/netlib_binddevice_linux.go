// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo && linux

package netlib

import "syscall"

// setSocketBindToDevice binds a raw socket fd to a network interface via
// SO_BINDTODEVICE (iface != ""), or clears the binding (iface == ""). This
// forces egress out that link and restricts ingress to datagrams arriving on
// it - the interface selection a wildcard bind (0.0.0.0:0) otherwise leaves to
// the kernel routing table. Linux-only; the setsockopt needs CAP_NET_RAW
// (typically root), returning EPERM otherwise.
func setSocketBindToDevice(fd uintptr, iface string) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
}
