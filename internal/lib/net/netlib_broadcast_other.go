// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo && !unix

package netlib

import "fmt"

// setSocketBroadcast is unsupported on non-Unix hosts (the best-effort Windows
// build). Linux is the only supported platform; broadcast UDP lives there.
func setSocketBroadcast(_ uintptr, _ int) error {
	return fmt.Errorf("SO_BROADCAST is not supported on this platform")
}
