// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo && linux

package netlib

import "syscall"

// setSocketBindToDevice binds a raw socket fd to a network interface via
// SO_BINDTODEVICE (iface != ""), or clears the binding (iface == ""). This
// forces egress out that link and restricts ingress to datagrams arriving on
// it - the interface selection a wildcard bind (0.0.0.0:0) otherwise leaves to
// the kernel routing table. Linux-only. Unprivileged on Linux 5.7+ while the
// socket is not bound to a device; changing a binding, or any call on an
// older kernel, needs CAP_NET_RAW and returns EPERM without it.
func setSocketBindToDevice(fd uintptr, iface string) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
}
