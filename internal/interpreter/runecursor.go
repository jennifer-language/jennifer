// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"sync"
	"unicode/utf8"
	"unsafe"
)

// Rune positions (`$s[a..b]`, strings.indexOf's `from`) are converted to byte
// offsets by walking the string. On ASCII text the callers skip the walk (rune
// offset == byte offset); on any other text a walk from the start makes the
// usual scanning loop (find the next delimiter from the last one, slice the
// piece between) quadratic. The rune cursor remembers the last converted
// position of a string, so the next conversion at, after or shortly before it
// resumes the walk from there and a forward scan costs O(n) in all.
//
// The cursor cannot live in the Value: a builtin or a slice sees a by-value
// copy of the binding, so a position stored on it would be lost with the copy.
// It is a small process-wide table instead, indexed by the string's data
// pointer. An entry holds the string it belongs to (data pointer and length
// compared, so it matches every copy of the same string and nothing else; Go
// strings are immutable, so a matching entry is never stale), and keeps that
// string alive until the slot is reused. Each slot has its own mutex: tasks run
// as goroutines and may convert positions in the same string at once. The walk
// itself runs outside the lock; a slot only serves as a hint, so a race between
// two walkers costs a rescan, never a wrong offset.

// cursorMinBytes / cursorMinRunes: below these a walk from the start is cheaper
// than the slot's lock, so short strings and positions near the start skip the
// table.
const (
	cursorMinBytes = 256
	cursorMinRunes = 64
	runeCursorSlot = 16 // power of two
)

type runeCursor struct {
	mu sync.Mutex
	s  string // the string the position belongs to
	r  int    // rune index r ...
	b  int    // ... starts at byte offset b of s
}

var runeCursors [runeCursorSlot]runeCursor

func runeCursorFor(s string) *runeCursor {
	p := uintptr(unsafe.Pointer(unsafe.StringData(s)))
	h := p>>4 ^ p>>12 ^ p>>20
	return &runeCursors[h&(runeCursorSlot-1)]
}

// sameString reports whether a and b are the same string: the same backing
// bytes and the same length (a substring sharing the start is a different one).
func sameString(a, b string) bool {
	return len(a) == len(b) && unsafe.StringData(a) == unsafe.StringData(b)
}

// skipRunesFwd advances n runes from byte offset off in s and returns the new
// byte offset and the number of runes skipped (fewer than n when s ends
// first). An invalid byte counts as one rune, as in utf8.RuneCountInString.
func skipRunesFwd(s string, off, n int) (int, int) {
	k := 0
	for k < n && off < len(s) {
		if s[off] < utf8.RuneSelf {
			off++
		} else {
			_, size := utf8.DecodeRuneInString(s[off:])
			off += size
		}
		k++
	}
	return off, k
}

// skipRunesBack moves back n runes from byte offset off, which must be a rune
// boundary with at least n runes before it. Decoding backward splits the bytes
// into the same runes as decoding forward, invalid bytes included: a valid
// rune holds no start byte after its first, so DecodeLastRuneInString finds
// the start the forward walk used, and an invalid byte is one rune either way.
func skipRunesBack(s string, off, n int) int {
	for ; n > 0; n-- {
		if s[off-1] < utf8.RuneSelf {
			off--
		} else {
			_, size := utf8.DecodeLastRuneInString(s[:off])
			off -= size
		}
	}
	return off
}

// RuneOffset returns the byte offset in s at which rune index r (r >= 0)
// starts, and the number of runes counted: r itself, or the rune count of s
// when s has fewer than r runes (the offset is then len(s)). Equal to a walk
// from the start, but resumes from the string's rune cursor and moves it to r,
// so ascending positions in one string cost O(distance) each.
func RuneOffset(s string, r int) (int, int) {
	if r <= 0 {
		return 0, 0
	}
	if len(s) < cursorMinBytes || r < cursorMinRunes {
		return skipRunesFwd(s, 0, r)
	}
	c := runeCursorFor(s)
	c.mu.Lock()
	hit := sameString(c.s, s)
	cr, cb := c.r, c.b
	c.mu.Unlock()
	var b, k int
	switch {
	case hit && r >= cr:
		b, k = skipRunesFwd(s, cb, r-cr)
		k += cr
	case hit && cr-r < r:
		b, k = skipRunesBack(s, cb, cr-r), r
	default:
		b, k = skipRunesFwd(s, 0, r)
	}
	if k == r {
		NoteRuneOffset(s, r, b)
	}
	return b, k
}

// NoteRuneOffset records that rune index r of s starts at byte offset b, for a
// caller that found the position by its own walk (strings.indexOf's hit, a
// slice's upper end). b must be the exact offset; short strings are ignored.
func NoteRuneOffset(s string, r, b int) {
	if len(s) < cursorMinBytes || r < cursorMinRunes {
		return
	}
	c := runeCursorFor(s)
	c.mu.Lock()
	c.s, c.r, c.b = s, r, b
	c.mu.Unlock()
}

// runeSpan returns the byte offsets of runes lo and hi of s (0 <= lo <= hi <=
// rune count), for a slice `$s[lo..hi]`. The cursor ends at hi, so the next
// slice or search starting there resumes without a rescan.
func runeSpan(s string, lo, hi int) (int, int) {
	blo, _ := RuneOffset(s, lo)
	bhi, _ := skipRunesFwd(s, blo, hi-lo)
	NoteRuneOffset(s, hi, bhi)
	return blo, bhi
}
