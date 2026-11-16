// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"bytes"
	"image"
	"math/rand"
	"runtime"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestMaxPixelsFollowsAllocCap: the canvas cap is one MaxAllocBytes buffer of
// NRGBA, 64 Mpx on the default binary and 8 Mpx on jennifer-tiny (it was a
// fixed 64 Mpx, 256 MiB, eight times tiny's cap). Run with -tags tinygo for
// the tiny figures.
func TestMaxPixelsFollowsAllocCap(t *testing.T) {
	if maxPixels*4 != limits.MaxAllocBytes {
		t.Fatalf("maxPixels = %d, want MaxAllocBytes / 4 = %d", int64(maxPixels), int64(limits.MaxAllocBytes/4))
	}
	want := int64(64 << 20)
	if limits.MaxAllocBytes == 32<<20 {
		want = 8 << 20
	}
	if maxPixels != want {
		t.Fatalf("maxPixels = %d, want %d", int64(maxPixels), want)
	}
	// One row past the cap is refused before anything is allocated.
	w := int64(4096)
	_, err := createFn(interpreter.BuiltinCtx{}, []Value{interpreter.IntVal(w), interpreter.IntVal(maxPixels/w + 1)})
	if err == nil || errorKind(t, err) != "limit" {
		t.Fatalf("create past the cap: %v", err)
	}
}

// withResampleCells runs f with the resample buffer cap lowered to n cells.
func withResampleCells(t *testing.T, n int64, f func()) {
	t.Helper()
	old := resampleCells
	resampleCells = n
	defer func() { resampleCells = old }()
	f()
}

func randomImage(rng *rand.Rand, w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng.Read(m.Pix)
	return m
}

// TestResampleStrips: a resize whose whole horizontal-first buffer is over
// the cap works in strips, gives the same pixels as the unstripped pass, and
// allocates no more than the cap for its buffer. An 8192x8192 to 8191x8191
// resize used to allocate a 1 GiB buffer; here the same shape is scaled down
// with the cap.
func TestResampleStrips(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, tc := range []struct {
		sw, sh, dw, dh int
		k              kernel
	}{
		{256, 256, 255, 255, catmullRom}, // hFirst 65280 cells, vFirst 65280
		{300, 200, 97, 31, catmullRom},   // shrink: wide spans
		{64, 300, 50, 700, bilinear},     // enlarge
	} {
		src := randomImage(rng, tc.sw, tc.sh)
		want := image.NewNRGBA(image.Rect(0, 0, tc.dw, tc.dh))
		if err := resample(interpreter.BuiltinCtx{}, "t", src, want, tc.k); err != nil {
			t.Fatal(err)
		}
		hFirst := int64(tc.dw) * int64(tc.sh)
		limit := hFirst / 8
		got := image.NewNRGBA(image.Rect(0, 0, tc.dw, tc.dh))
		var a, b runtime.MemStats
		withResampleCells(t, limit, func() {
			runtime.ReadMemStats(&a)
			if err := resample(interpreter.BuiltinCtx{}, "t", src, got, tc.k); err != nil {
				t.Fatal(err)
			}
			runtime.ReadMemStats(&b)
		})
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%v: stripped result differs", tc)
		}
		// Buffer (16 bytes a cell) plus the two filter plans, with room for
		// allocation size classes.
		cb, _ := planBytes(tc.sw, tc.dw, tc.k)
		rb, _ := planBytes(tc.sh, tc.dh, tc.k)
		if alloc := int64(b.TotalAlloc - a.TotalAlloc); alloc > 16*limit+(cb+rb)*5/4+8192 {
			t.Fatalf("%v: allocated %d bytes, cap %d", tc, alloc, 16*limit)
		}
	}
}

// TestResampleBufferCapLimit: when not even one output row fits the buffer
// cap in either order, resize is Error{kind: "limit"}.
func TestResampleBufferCapLimit(t *testing.T) {
	src := call(t, createFn, num(100), num(100), col(red))
	withResampleCells(t, 64, func() {
		_, err := resizeFn(interpreter.BuiltinCtx{}, []Value{src, num(99), num(99), str("bilinear")})
		if err == nil || errorKind(t, err) != "limit" {
			t.Fatalf("err = %v", err)
		}
	})
	// The default cap passes the same resize.
	if w, h := size(t, call(t, resizeFn, src, num(99), num(99), str("bilinear"))); w != 99 || h != 99 {
		t.Fatalf("got %dx%d", w, h)
	}
}
