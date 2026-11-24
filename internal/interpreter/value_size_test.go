// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"testing"
	"unsafe"
)

// Every evaluation step copies a Value and every collection element is one, so
// its size is a hot-path and a memory cost: the packed layout (value_layout.go)
// is 48 bytes, down from 272. A field may still be added, but deliberately -
// raise the bound here and re-run examples/benchmark.j.
func TestValueSizeBound(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("size bound is for 64-bit targets")
	}
	if got := unsafe.Sizeof(Value{}); got > 48 {
		t.Errorf("Value is %d bytes, bound is 48", got)
	}
}

// A goroutine's depth / chain counters fill whole cache lines, so parallel
// spawn workers never share one (see CallChain).
func TestCounterCellsFillTheirLines(t *testing.T) {
	if s := unsafe.Sizeof(CallChain{}); s < counterLine {
		t.Errorf("CallChain is %d bytes, want >= %d", s, counterLine)
	}
	if s := unsafe.Sizeof(chainCell{}); s < counterLine {
		t.Errorf("chainCell is %d bytes, want >= %d", s, counterLine)
	}
	if d := newDepthCounter(); d.n != 0 || d.prof != 0 {
		t.Errorf("fresh call chain = %+v", *d)
	}
}
