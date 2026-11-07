// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package limits

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

func TestMulInt64(t *testing.T) {
	cases := []struct {
		a, b int64
		want int64
		ok   bool
	}{
		{0, math.MaxInt64, 0, true},
		{math.MaxInt64, 1, math.MaxInt64, true},
		{math.MaxInt64, 2, 0, false},
		{1 << 32, 1 << 32, 0, false},
		{1 << 31, 1 << 31, 1 << 62, true},
		{-1, math.MinInt64, 0, false},
		{math.MinInt64, -1, 0, false},
		{math.MinInt64, 1, math.MinInt64, true},
		{-3, 4, -12, true},
		{-3, -4, 12, true},
		{math.MaxInt64, -1, -math.MaxInt64, true},
		{math.MinInt64 / 2, 2, math.MinInt64, true},
		{math.MinInt64/2 - 1, 2, 0, false},
	}
	for _, c := range cases {
		got, ok := MulInt64(c.a, c.b)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("MulInt64(%d, %d) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}

func TestAddInt64(t *testing.T) {
	cases := []struct {
		a, b int64
		want int64
		ok   bool
	}{
		{math.MaxInt64, 0, math.MaxInt64, true},
		{math.MaxInt64, 1, 0, false},
		{math.MinInt64, -1, 0, false},
		{math.MinInt64, math.MaxInt64, -1, true},
		{-5, 3, -2, true},
	}
	for _, c := range cases {
		got, ok := AddInt64(c.a, c.b)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("AddInt64(%d, %d) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}

// TestIntHelpers runs on the platform int: 32 bits under GOARCH=386 and
// jennifer-tiny, 64 elsewhere.
func TestIntHelpers(t *testing.T) {
	if _, ok := MulInt(math.MaxInt, 2); ok {
		t.Error("MulInt(MaxInt, 2) should overflow")
	}
	if _, ok := MulInt(math.MinInt, -1); ok {
		t.Error("MulInt(MinInt, -1) should overflow")
	}
	if got, ok := MulInt(-7, 6); !ok || got != -42 {
		t.Errorf("MulInt(-7, 6) = %d, %v", got, ok)
	}
	if _, ok := MulInt(1<<16, 1<<16); ok != (math.MaxInt > 1<<32) {
		t.Errorf("MulInt(2^16, 2^16) ok = %v on a %d-bit int", ok, bitsOfInt())
	}
	if _, ok := AddInt(math.MaxInt, 1); ok {
		t.Error("AddInt(MaxInt, 1) should overflow")
	}
	if _, ok := AddInt(math.MinInt, -1); ok {
		t.Error("AddInt(MinInt, -1) should overflow")
	}
	if got, ok := AddInt(math.MaxInt, math.MinInt); !ok || got != -1 {
		t.Errorf("AddInt(MaxInt, MinInt) = %d, %v", got, ok)
	}
}

func bitsOfInt() int {
	if math.MaxInt > 1<<32 {
		return 64
	}
	return 32
}

func TestLimitPlusOne(t *testing.T) {
	cases := map[int64]int64{
		0:                 1,
		10:                11,
		math.MaxInt64 - 1: math.MaxInt64,
		math.MaxInt64:     math.MaxInt64,
		-1:                0,
		math.MinInt64:     0,
	}
	for in, want := range cases {
		if got := LimitPlusOne(in); got != want {
			t.Errorf("LimitPlusOne(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestReadAll(t *testing.T) {
	data, err := ReadAll(strings.NewReader("hello"), 5)
	if err != nil || string(data) != "hello" {
		t.Fatalf("at limit: %q, %v", data, err)
	}
	if _, err := ReadAll(strings.NewReader("hello!"), 5); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over limit: %v, want ErrTooLarge", err)
	}
	var tl *TooLargeError
	if _, err := ReadAll(strings.NewReader("x"), 0); !errors.As(err, &tl) || tl.Limit != 0 {
		t.Fatalf("limit 0: %v", err)
	}
	// MaxInt64 is "no practical limit", not "read nothing".
	data, err = ReadAll(strings.NewReader("hello"), math.MaxInt64)
	if err != nil || string(data) != "hello" {
		t.Fatalf("MaxInt64: %q, %v", data, err)
	}
	if _, err := ReadAll(strings.NewReader("x"), -1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("negative limit: %v, want ErrTooLarge", err)
	}
}

// stallReader returns its data, then blocks the test by returning
// (0, io.ErrNoProgress): a peer that announced more than it sent.
type stallReader struct {
	data  []byte
	reads int
}

func (s *stallReader) Read(p []byte) (int, error) {
	s.reads++
	if len(s.data) == 0 {
		return 0, io.ErrNoProgress
	}
	n := copy(p, s.data)
	s.data = s.data[n:]
	return n, nil
}

func TestReadNGrowsWithData(t *testing.T) {
	// A 256 MiB request on a peer that sends 10 bytes must not allocate 256 MiB.
	r := &stallReader{data: []byte("0123456789")}
	got, err := ReadN(r, MaxAllocBytes, nil)
	if !errors.Is(err, io.ErrNoProgress) || string(got) != "0123456789" {
		t.Fatalf("got %q, %v", got, err)
	}
	if cap(got) > readNChunk {
		t.Fatalf("buffer cap %d, want <= %d", cap(got), readNChunk)
	}
	allocs := testing.AllocsPerRun(5, func() {
		_, _ = ReadN(&stallReader{data: []byte("x")}, MaxAllocBytes, nil)
	})
	if allocs > 3 {
		t.Fatalf("%v allocations", allocs)
	}
}

func TestReadN(t *testing.T) {
	src := bytes.Repeat([]byte("ab"), 100_000) // 200 KB: several growth steps
	calls := 0
	got, err := ReadN(bytes.NewReader(src), int64(len(src)), func() { calls++ })
	if err != nil || !bytes.Equal(got, src) {
		t.Fatalf("full read: len %d, %v", len(got), err)
	}
	if calls == 0 {
		t.Fatal("before hook never ran")
	}
	got, err = ReadN(bytes.NewReader(src[:10]), 20, nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) || len(got) != 10 {
		t.Fatalf("short: %d, %v", len(got), err)
	}
	if _, err := ReadN(bytes.NewReader(nil), 5, nil); !errors.Is(err, io.EOF) {
		t.Fatalf("empty: %v", err)
	}
	if got, err := ReadN(bytes.NewReader(nil), 0, nil); err != nil || len(got) != 0 {
		t.Fatalf("n=0: %q, %v", got, err)
	}
	if _, err := ReadN(bytes.NewReader(nil), MaxAllocBytes+1, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over cap: %v", err)
	}
	if _, err := ReadN(bytes.NewReader(nil), math.MaxInt64, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("MaxInt64: %v", err)
	}
	if _, err := ReadN(bytes.NewReader(nil), -1, nil); err == nil || errors.Is(err, ErrTooLarge) {
		t.Fatalf("negative: %v", err)
	}
}

func TestReadDelim(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("ab\ncd"), 16)
	line, err := ReadDelim(br, '\n', 100)
	if err != nil || string(line) != "ab\n" {
		t.Fatalf("line 1: %q, %v", line, err)
	}
	line, err = ReadDelim(br, '\n', 100)
	if err != io.EOF || string(line) != "cd" {
		t.Fatalf("line 2: %q, %v", line, err)
	}
	// A line longer than the bufio buffer is assembled across ErrBufferFull.
	long := strings.Repeat("x", 100) + "\n"
	line, err = ReadDelim(bufio.NewReaderSize(strings.NewReader(long), 16), '\n', 101)
	if err != nil || string(line) != long {
		t.Fatalf("long: %d, %v", len(line), err)
	}
	if _, err := ReadDelim(bufio.NewReaderSize(strings.NewReader(long), 16), '\n', 100); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over: %v", err)
	}
	// An endless stream with no delimiter stops at the limit.
	if _, err := ReadDelim(bufio.NewReader(zeroReader{}), '\n', 1<<20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("endless: %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
