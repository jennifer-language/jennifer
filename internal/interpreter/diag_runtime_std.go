// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package interpreter

import "runtime"

// runtimeSnapshot reports the process goroutine count and the runtime's
// heap-in-use / OS-reserved bytes / GC count, for the SIGUSR1 diagnostics dump.
// ReadMemStats briefly stops the world, which is fine for a rare, user-triggered
// dump.
func runtimeSnapshot() (goroutines int, heapAlloc, sys uint64, numGC uint32) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return runtime.NumGoroutine(), m.HeapAlloc, m.Sys, m.NumGC
}
