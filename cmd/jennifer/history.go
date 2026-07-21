// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

// replHistory is a tiny in-memory ring of past REPL submissions. The line
// editor walks it with Up/Down. Persistence to disk is a future feature -
// for now the history lives only for the duration of the session.
//
// Each entry is one logical REPL submission, which may be a single line
// or a multi-line block accumulated through the continuation prompt. We
// store the joined text so Up reproduces the same statement the user
// originally typed, not just the last continuation fragment.
type replHistory struct {
	entries []string
	max     int
}

const defaultHistoryCap = 100

func newReplHistory() *replHistory {
	return &replHistory{max: defaultHistoryCap}
}

// Add appends an entry. Adjacent duplicates are collapsed (a user retrying
// the same statement doesn't bloat the ring). Empty strings are ignored.
// When the ring reaches its cap, the oldest entry is dropped.
func (h *replHistory) Add(entry string) {
	if entry == "" {
		return
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == entry {
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
}
