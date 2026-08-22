// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo && !linux

package netlib

import "fmt"

// setSocketBindToDevice is unsupported off Linux (the best-effort macOS /
// Windows builds). SO_BINDTODEVICE is a Linux-specific socket option; Linux is
// the one supported platform, and interface-scoped binding lives there.
func setSocketBindToDevice(_ uintptr, _ string) error {
	return fmt.Errorf("SO_BINDTODEVICE is not supported on this platform")
}
