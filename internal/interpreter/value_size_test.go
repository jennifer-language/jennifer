// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"testing"
	"unsafe"
)

// Every evaluation step copies a Value, so its size is a hot-path cost: one
// added word measurably slowed numeric loops. A field may still be added, but
// deliberately - raise the bound here and re-run examples/benchmark.j.
func TestValueSizeBound(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("size bound is for 64-bit targets")
	}
	if got := unsafe.Sizeof(Value{}); got > 272 {
		t.Errorf("Value is %d bytes, bound is 272", got)
	}
}

// A goroutine's depth / chain counters fill whole cache lines, so parallel
// spawn workers never share one (see depthCell).
func TestCounterCellsFillTheirLines(t *testing.T) {
	if s := unsafe.Sizeof(depthCell{}); s < counterLine {
		t.Errorf("depthCell is %d bytes, want >= %d", s, counterLine)
	}
	if s := unsafe.Sizeof(chainCell{}); s < counterLine {
		t.Errorf("chainCell is %d bytes, want >= %d", s, counterLine)
	}
	if d := newDepthCounter(); *d != 0 {
		t.Errorf("fresh depth counter = %d", *d)
	}
}
