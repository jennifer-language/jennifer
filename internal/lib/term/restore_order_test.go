// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package termlib

import (
	"testing"

	"golang.org/x/term"
)

// TestRestoreAllNewestFirst: nested un-restored sessions must be undone newest
// first, so the oldest one's saved (cooked) state is the one left applied.
func TestRestoreAllNewestFirst(t *testing.T) {
	ResetForTest()
	saved := map[*term.State]int{}
	statesMu.Lock()
	for id := 0; id < 16; id++ {
		st := &term.State{}
		saved[st] = id
		states[id] = &rawEntry{fd: -1, prev: st}
	}
	statesMu.Unlock()
	var order []int
	old := restoreTerm
	restoreTerm = func(_ int, st *term.State) error {
		order = append(order, saved[st])
		return nil
	}
	defer func() { restoreTerm = old }()
	RestoreAll()
	if len(order) != 16 {
		t.Fatalf("restored %d entries", len(order))
	}
	for i, id := range order {
		if id != 15-i {
			t.Fatalf("restore order %v, want 15..0", order)
		}
	}
}
