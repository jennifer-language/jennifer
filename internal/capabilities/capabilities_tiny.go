// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

package capabilities

// buildSet: the TinyGo build has neither the network stack nor os/exec (the `net`
// and process libraries are compile-time stubs). A minimal-footprint target rebuilt
// with a network stack would add "net" here.
var buildSet = []string{}
