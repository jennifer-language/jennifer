// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !unix

package main

// installTermSignalRestore is a no-op where the terminating Unix signals it
// traps (SIGINT / SIGTERM / SIGHUP handled via re-raise) do not apply the same
// way (Windows). The `defer termlib.RestoreAll()` teardown still runs on every
// normal exit path there; only the signal-interception refinement is Unix-only,
// mirroring diag_unix.go / diag_other.go. Returns a no-op stop function.
func installTermSignalRestore() func() { return func() {} }
