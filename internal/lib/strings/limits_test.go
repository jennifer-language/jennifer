// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package stringslib

import (
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestReplaceCap: a replacement whose result passes limits.MaxStringBytes
// is refused before allocating it. 1 MiB of "a" with each "a" replaced by
// 1025 bytes is just over 1 GiB (the pre-fix code built it).
func TestReplaceCap(t *testing.T) {
	in := newLib(t)
	s := strings.Repeat("a", limits.MaxStringBytes>>10)
	repl := strings.Repeat("b", 1025)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := rawFn(in, "replace", sv(s), sv("a"), sv(repl))
	runtime.ReadMemStats(&after)
	if err == nil || !strings.Contains(err.Error(), "strings.replace: result exceeds") {
		t.Fatalf("got %v", err)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 16<<20 {
		t.Fatalf("allocated %d bytes before refusing", got)
	}
	// An empty old is refused before any size check (M29.52).
	if _, err := rawFn(in, "replace", sv(strings.Repeat("a", 100000)), sv(""), sv(strings.Repeat("b", 100000))); err == nil {
		t.Fatal("empty old should error")
	}
	// Shrinking and in-range results still work.
	if got := callFn(t, in, "replace", sv("aXbXc"), sv("X"), sv("")).Str; got != "abc" {
		t.Fatalf("shrink: %q", got)
	}
}
