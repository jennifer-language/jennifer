// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

package interpreter

import "runtime"

// runtimeSnapshot reports the goroutine count and heap-in-use / OS-reserved bytes
// for the SIGUSR1 diagnostics dump. TinyGo's runtime.MemStats omits NumGC (its GC
// exposes no cycle count), so GC count is reported as 0 here; the rest is real.
func runtimeSnapshot() (goroutines int, heapAlloc, sys uint64, numGC uint32) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return runtime.NumGoroutine(), m.HeapAlloc, m.Sys, 0
}
