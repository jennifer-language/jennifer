// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package uuidlib

import (
	"sort"
	"strings"
	"sync"
	"testing"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestGenerateV4(t *testing.T) {
	s := mustGen(t, genV4)
	b, ok := parse(s)
	if !ok {
		t.Fatalf("v4 %q did not parse", s)
	}
	if b[6]>>4 != 4 {
		t.Errorf("version nibble = %d, want 4", b[6]>>4)
	}
	if b[8]>>6 != 0b10 {
		t.Errorf("variant bits = %02b, want 10", b[8]>>6)
	}
	if len(s) != 36 {
		t.Errorf("length = %d, want 36", len(s))
	}
}

func TestGenerateV7(t *testing.T) {
	s := mustGen(t, genV7)
	b, ok := parse(s)
	if !ok {
		t.Fatalf("v7 %q did not parse", s)
	}
	if b[6]>>4 != 7 {
		t.Errorf("version nibble = %d, want 7", b[6]>>4)
	}
	if b[8]>>6 != 0b10 {
		t.Errorf("variant bits = %02b, want 10", b[8]>>6)
	}
}

func TestV7SortsByCreationTime(t *testing.T) {
	orig := nowFunc
	defer func() { nowFunc = orig; resetV7() }()
	resetV7()
	base := stdtime.UnixMilli(1_700_000_000_000)

	nowFunc = func() stdtime.Time { return base }
	early := mustGen(t, genV7)
	nowFunc = func() stdtime.Time { return base.Add(stdtime.Hour) }
	late := mustGen(t, genV7)

	if !(early < late) {
		t.Errorf("v7 not time-ordered: early=%s late=%s", early, late)
	}
}

// resetV7 clears the v7 monotonic state so a test's fixed clock is not
// overridden by a later timestamp from an earlier test.
func resetV7() {
	v7Mu.Lock()
	v7LastMs, v7RandA, v7RandB = -1, 0, 0
	v7Mu.Unlock()
}

func TestV7MonotonicWithinMillisecond(t *testing.T) {
	orig := nowFunc
	defer func() { nowFunc = orig; resetV7() }()
	resetV7()
	base := stdtime.UnixMilli(1_700_000_000_000)
	nowFunc = func() stdtime.Time { return base }
	prev := mustGen(t, genV7)
	for i := 0; i < 1000; i++ {
		cur := mustGen(t, genV7)
		if cur <= prev {
			t.Fatalf("call %d: %s <= %s", i, cur, prev)
		}
		b, _ := parse(cur)
		if b[6]>>4 != 7 || b[8]>>6 != 0b10 {
			t.Fatalf("call %d: bad version/variant in %s", i, cur)
		}
		prev = cur
	}
	b, _ := parse(prev)
	ms := int64(b[0])<<40 | int64(b[1])<<32 | int64(b[2])<<24 | int64(b[3])<<16 | int64(b[4])<<8 | int64(b[5])
	if ms != base.UnixMilli() {
		t.Errorf("timestamp = %d, want %d", ms, base.UnixMilli())
	}
}

func TestV7MonotonicClockBackwards(t *testing.T) {
	orig := nowFunc
	defer func() { nowFunc = orig; resetV7() }()
	resetV7()
	base := stdtime.UnixMilli(1_700_000_000_000)
	nowFunc = func() stdtime.Time { return base }
	a := mustGen(t, genV7)
	nowFunc = func() stdtime.Time { return base.Add(-stdtime.Second) }
	b := mustGen(t, genV7)
	if b <= a {
		t.Errorf("v7 after clock step back: %s <= %s", b, a)
	}
}

func TestV7CarryIntoTimestamp(t *testing.T) {
	orig := nowFunc
	defer func() { nowFunc = orig; resetV7() }()
	base := stdtime.UnixMilli(1_700_000_000_000)
	nowFunc = func() stdtime.Time { return base }
	v7Mu.Lock()
	v7LastMs, v7RandA, v7RandB = base.UnixMilli(), randAMax-1, randBMax-1
	v7Mu.Unlock()
	s := mustGen(t, genV7)
	b, _ := parse(s)
	ms := int64(b[0])<<40 | int64(b[1])<<32 | int64(b[2])<<24 | int64(b[3])<<16 | int64(b[4])<<8 | int64(b[5])
	if ms != base.UnixMilli()+1 {
		t.Errorf("timestamp after tail overflow = %d, want %d", ms, base.UnixMilli()+1)
	}
}

func TestV7MonotonicConcurrent(t *testing.T) {
	orig := nowFunc
	defer func() { nowFunc = orig; resetV7() }()
	resetV7()
	base := stdtime.UnixMilli(1_700_000_000_000)
	nowFunc = func() stdtime.Time { return base }
	const workers, per = 8, 200
	out := make([][]string, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				s, err := genV7()
				if err != nil {
					t.Error(err)
					return
				}
				out[w] = append(out[w], s)
			}
		}(w)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, ws := range out {
		if !sort.StringsAreSorted(ws) {
			t.Fatal("v7s from one goroutine not increasing")
		}
		for _, s := range ws {
			if seen[s] {
				t.Fatalf("duplicate v7 %s", s)
			}
			seen[s] = true
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	s := mustGen(t, genV4)
	b, ok := parse(s)
	if !ok {
		t.Fatalf("parse(%q) failed", s)
	}
	if format(b) != s {
		t.Errorf("round-trip: %q -> %q", s, format(b))
	}
	// Parsing is case-insensitive; NIL is a valid UUID.
	if _, ok := parse(strings.ToUpper(s)); !ok {
		t.Errorf("uppercase %q should parse", strings.ToUpper(s))
	}
	if _, ok := parse(NIL); !ok {
		t.Error("NIL should parse")
	}
}

func TestParseInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"not-a-uuid",
		"00000000-0000-0000-0000-00000000000",   // one short
		"00000000-0000-0000-0000-0000000000000", // one long
		"00000000x0000-0000-0000-000000000000",  // wrong separator
		"0000000g-0000-0000-0000-000000000000",  // non-hex digit
	} {
		if _, ok := parse(s); ok {
			t.Errorf("parse(%q) should have failed", s)
		}
	}
}

func TestVersionOfNilIsZero(t *testing.T) {
	b, ok := parse(NIL)
	if !ok || b[6]>>4 != 0 {
		t.Errorf("NIL version = %d, want 0", b[6]>>4)
	}
}

func TestGeneratorFns(t *testing.T) {
	out, err := v4Fn(interpreter.BuiltinCtx{}, nil)
	if err != nil {
		t.Fatalf("v4: %v", err)
	}
	if b, ok := parse(out.Str); !ok || b[6]>>4 != 4 {
		t.Errorf("v4() produced %q (version %d), want a valid v4", out.Str, b[6]>>4)
	}
	out7, err := v7Fn(interpreter.BuiltinCtx{}, nil)
	if err != nil {
		t.Fatalf("v7: %v", err)
	}
	if b, ok := parse(out7.Str); !ok || b[6]>>4 != 7 {
		t.Errorf("v7() produced %q (version %d), want a valid v7", out7.Str, b[6]>>4)
	}
	if _, err := v4Fn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("v4")}); err == nil {
		t.Error("expected an arity error when v4 is given an argument")
	}
	if _, err := v7Fn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("expected an arity error when v7 is given an argument")
	}
}

// mustGen runs a UUID generator and fails the test if the random source does.
func mustGen(t *testing.T, gen func() (string, error)) string {
	t.Helper()
	s, err := gen()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return s
}
