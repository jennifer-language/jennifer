// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package main

import (
	"os"
	"runtime/debug"

	"jennifer-lang.dev/jennifer/internal/memlimit"
)

// memoryLimitShare is the part of the machine memory limit the Go runtime is
// told about: the rest is headroom for what the GC does not count (stacks of
// blocked goroutines, cgo, the kernel's page cache charged to a cgroup).
const memoryLimitShare = 0.9

// setMemoryLimit sets the Go soft memory limit from the machine or cgroup
// limit, so the GC works harder as a program nears it instead of the process
// being OOM-killed with the heap full of garbage. A GOMEMLIMIT in the
// environment wins (the runtime has applied it already); an undetectable limit
// leaves the runtime default (none).
func setMemoryLimit() {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	limit, err := memlimit.Detect()
	if err != nil || limit <= 0 {
		return
	}
	debug.SetMemoryLimit(memoryLimitFor(limit))
}

// memoryLimitFor is the soft limit set for a detected machine limit.
func memoryLimitFor(limit int64) int64 {
	return int64(float64(limit) * memoryLimitShare)
}
