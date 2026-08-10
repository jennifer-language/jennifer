// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package capabilities

// buildSet: the standard Go build ships the full host surface - the network stack
// (net / TLS / DNS and every net-backed library), os/exec (external processes), and
// the `sql` database drivers. All three are absent on the TinyGo build.
var buildSet = []string{"net", "exec", "sql"}
