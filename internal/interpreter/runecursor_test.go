// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"math/rand"
	"strings"
	"sync"
	"testing"
)

// naiveRuneOffset is the walk from the start the cursor must agree with.
func naiveRuneOffset(s string, r int) (int, int) {
	return skipRunesFwd(s, 0, r)
}

// cursorText builds a string past the cursor thresholds from ASCII, two- to
// four-byte runes and invalid bytes (a stray continuation byte, a truncated
// sequence, an encoded surrogate), which the walks count as one rune each.
func cursorText(rng *rand.Rand, n int) string {
	pieces := []string{"a", "b", "<", "é", "€", "漢", "🙂", "\xff", "\x82", "\xe2\x82", "\xed\xa0\x80", "\xf0\x9f"}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(pieces[rng.Intn(len(pieces))])
	}
	return b.String()
}

// TestRuneOffsetMatchesWalk: every conversion equals a walk from the start,
// whatever the cursor holds - forward runs, backward jumps after a forward
// scan, positions past the end, and switches between strings sharing a slot.
func TestRuneOffsetMatchesWalk(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	texts := []string{cursorText(rng, 3000), cursorText(rng, 3000), cursorText(rng, 500)}
	for step := 0; step < 20000; step++ {
		s := texts[rng.Intn(len(texts))]
		n := len([]rune(s)) // RuneCountInString agrees: invalid bytes are one rune each
		var r int
		switch rng.Intn(4) {
		case 0:
			r = rng.Intn(n + 10) // anywhere, a few past the end
		default:
			_, cr := cursorState(s)
			r = cr + rng.Intn(40) - 20 // near the cursor, either side
			if r < 0 {
				r = 0
			}
		}
		gotB, gotK := RuneOffset(s, r)
		wantB, wantK := naiveRuneOffset(s, r)
		if gotB != wantB || gotK != wantK {
			t.Fatalf("step %d: RuneOffset(len %d, %d) = (%d, %d), want (%d, %d)", step, len(s), r, gotB, gotK, wantB, wantK)
		}
	}
}

// cursorState returns whether s owns its slot and the slot's rune index.
func cursorState(s string) (bool, int) {
	c := runeCursorFor(s)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !sameString(c.s, s) {
		return false, 0
	}
	return true, c.r
}

// TestRuneCursorBackwardAfterForward: a forward scan leaves the cursor at its
// last position; a later call before it (near and far) is still exact.
func TestRuneCursorBackwardAfterForward(t *testing.T) {
	s := strings.Repeat("é\\H\\", 1000)
	for r := 0; r <= 4000; r += 7 {
		RuneOffset(s, r)
	}
	if hit, cr := cursorState(s); !hit || cr != 3997 {
		t.Fatalf("cursor after the scan: hit %v at %d, want 3997", hit, cr)
	}
	for _, r := range []int{3990, 2500, 100, 64, 63, 1, 0} {
		b, k := RuneOffset(s, r)
		wb, wk := naiveRuneOffset(s, r)
		if b != wb || k != wk {
			t.Fatalf("backward to %d: (%d, %d), want (%d, %d)", r, b, k, wb, wk)
		}
	}
}

// TestRuneCursorOtherString: the cursor of one string is never used for
// another - not for an equal copy in different memory, not for a prefix that
// shares the backing (a different length), and not for a modified string.
func TestRuneCursorOtherString(t *testing.T) {
	s := strings.Repeat("aé", 1000)
	RuneOffset(s, 1500)
	clone := strings.Clone(s)
	if hit, _ := cursorState(clone); hit {
		t.Fatal("an equal string in other memory matched the cursor")
	}
	prefix := s[:len(s)-1]
	if hit, _ := cursorState(prefix); hit {
		t.Fatal("a prefix sharing the backing matched the cursor")
	}
	modified := "é" + s[1:]
	// A string of the same length but other runes, in s's slot: its offsets
	// differ from s's at every odd position, so a cursor taken from s is wrong.
	var twin string
	for i := 0; i < 1000 && twin == ""; i++ {
		x := strings.Repeat("éa", 1000)
		if runeCursorFor(x) == runeCursorFor(s) {
			twin = x
		}
	}
	if twin == "" {
		t.Fatal("no string of s's length landed in its slot")
	}
	RuneOffset(s, 1501)
	if hit, _ := cursorState(twin); hit {
		t.Fatal("a string of the same length in other memory matched the cursor")
	}
	for _, x := range []string{clone, prefix, modified, twin} {
		for _, r := range []int{1500, 1999, 2000, 2100, 700} {
			b, k := RuneOffset(x, r)
			wb, wk := naiveRuneOffset(x, r)
			if b != wb || k != wk {
				t.Fatalf("len %d rune %d: (%d, %d), want (%d, %d)", len(x), r, b, k, wb, wk)
			}
		}
	}
}

// TestRuneCursorConcurrent: goroutines converting positions in shared strings
// (the same slot and different ones) stay exact; run with -race.
func TestRuneCursorConcurrent(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	texts := []string{cursorText(rng, 2000), cursorText(rng, 2000)}
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			s := texts[g%2]
			for r := g; r < 2000; r += 3 {
				b, k := RuneOffset(s, r)
				wb, wk := naiveRuneOffset(s, r)
				if b != wb || k != wk {
					errs <- "mismatch"
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

// TestRuneSpanSlices: runeSpan agrees with a []rune slice for forward,
// overlapping and backward slice sequences on a non-ASCII string.
func TestRuneSpanSlices(t *testing.T) {
	s := strings.Repeat("x漢é🙂", 300)
	rs := []rune(s)
	spans := [][2]int{{0, 5}, {100, 200}, {200, 200}, {150, 1199}, {1100, 1200}, {64, 65}, {500, 900}, {10, 1100}}
	for _, sp := range spans {
		lo, hi := runeSpan(s, sp[0], sp[1])
		if got, want := s[lo:hi], string(rs[sp[0]:sp[1]]); got != want {
			t.Fatalf("span %v: %q, want %q", sp, got, want)
		}
	}
}
