// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"strings"
	"testing"
)

// TestClipToWindow covers the long-line source-context clipping: a short line is
// untouched, a very long line is windowed to caretWindow runes either side of the
// error column with `...` markers, and the returned caret column still lands on the
// error rune (rune-counted, so multi-byte characters are never split).
func TestClipToWindow(t *testing.T) {
	// A short line and its column pass through unchanged.
	if got, c := clipToWindow("hello world", 7); got != "hello world" || c != 7 {
		t.Errorf("short line changed: got %q col %d", got, c)
	}

	// Error in the middle of a long line: both ellipses, bounded width, caret on 'X'.
	long := strings.Repeat("a", 300) + "X" + strings.Repeat("b", 300) // 'X' at col 301
	disp, c := clipToWindow(long, 301)
	if !strings.HasPrefix(disp, "...") || !strings.HasSuffix(disp, "...") {
		t.Errorf("expected both ellipses, got %q...%q", disp[:6], disp[len(disp)-6:])
	}
	if n := len([]rune(disp)); n > 2*caretWindow+8 {
		t.Errorf("window not bounded: %d runes", n)
	}
	if r := []rune(disp); r[c-1] != 'X' {
		t.Errorf("caret col %d points at %q, want 'X'", c, string(r[c-1]))
	}

	// Error at the very start: no leading ellipsis, trailing ellipsis, caret on 'Y'.
	disp2, c2 := clipToWindow("Y"+strings.Repeat("z", 300), 1)
	if strings.HasPrefix(disp2, "...") {
		t.Errorf("unexpected leading ellipsis: %q", disp2[:6])
	}
	if !strings.HasSuffix(disp2, "...") {
		t.Error("expected trailing ellipsis for a long line clipped on the right")
	}
	if r := []rune(disp2); r[c2-1] != 'Y' {
		t.Errorf("caret points at %q, want 'Y'", string(r[c2-1]))
	}

	// Multi-byte runes: the window is rune-counted, so the caret still lands on the
	// error rune and no character is split.
	mb := strings.Repeat("é", 300) + "Ω" + strings.Repeat("é", 300) // 'Ω' at rune col 301
	dispM, cM := clipToWindow(mb, 301)
	if r := []rune(dispM); r[cM-1] != 'Ω' {
		t.Errorf("multibyte caret misaligned: got %q, want 'Ω'", string(r[cM-1]))
	}
}
