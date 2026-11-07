// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"math/rand"
	"runtime"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestCanvasProductOverflow: 2^32 x 2^32 wraps to 0 in int64; it must be a
// limit error, not a NewNRGBA panic. Skipped where int is 32 bits (the
// arguments narrow before newCanvas sees them).
func TestCanvasProductOverflow(t *testing.T) {
	if ^uint(0)>>32 == 0 {
		t.Skip("32-bit int")
	}
	big := interpreter.IntVal(1 << 32)
	_, err := createFn(interpreter.BuiltinCtx{}, []Value{big, big})
	if err == nil || errorKind(t, err) != "limit" {
		t.Fatalf("create: %v", err)
	}
	_, err = fromPixelsFn(interpreter.BuiltinCtx{}, []Value{big, big, raw(nil)})
	if err == nil || errorKind(t, err) != "limit" {
		t.Fatalf("fromPixels: %v", err)
	}
	src := call(t, createFn, num(2), num(2))
	_, err = resizeFn(interpreter.BuiltinCtx{}, []Value{src, big, big})
	if err == nil || errorKind(t, err) != "limit" {
		t.Fatalf("resize: %v", err)
	}
}

// TestResampleBufferBounded: a tall, thin source widened to a short, wide
// result needs a dw x sh buffer horizontal-first (here 4096 x 16384 cells,
// 1 GiB) but 1 cell vertical-first; resize must pick the small one.
func TestResampleBufferBounded(t *testing.T) {
	src := call(t, createFn, num(1), num(16384), col(red))
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	out := call(t, resizeFn, src, num(4096), num(1), str("bilinear"))
	runtime.ReadMemStats(&b)
	if got := b.TotalAlloc - a.TotalAlloc; got > 64<<20 {
		t.Fatalf("resize allocated %d bytes", got)
	}
	if w, h := size(t, out); w != 4096 || h != 1 || at(t, out, 100, 0) != red {
		t.Fatalf("got %dx%d %v", w, h, at(t, out, 100, 0))
	}
}

// TestResampleOrderSymmetric: vertical-first on an image equals
// horizontal-first on its transpose, transposed back.
func TestResampleOrderSymmetric(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const sw, sh, dw, dh = 5, 20, 40, 7 // sw*dh < dw*sh: vertical first
	src := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	srcT := image.NewNRGBA(image.Rect(0, 0, sh, sw))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			c := color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))}
			src.SetNRGBA(x, y, c)
			srcT.SetNRGBA(y, x, c)
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	dstT := image.NewNRGBA(image.Rect(0, 0, dh, dw))
	if err := resample(interpreter.BuiltinCtx{}, "t", src, dst, catmullRom); err != nil {
		t.Fatal(err)
	}
	if err := resample(interpreter.BuiltinCtx{}, "t", srcT, dstT, catmullRom); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			if dst.NRGBAAt(x, y) != dstT.NRGBAAt(y, x) {
				t.Fatalf("(%d,%d): %v vs %v", x, y, dst.NRGBAAt(x, y), dstT.NRGBAAt(y, x))
			}
		}
	}
}

// TestResamplePlanCompact: stretching 1x1 to 4194304x1 needs one weight per
// output column. The per-column taps were ~40 bytes each (160 MB here, 2.5 GB
// at 67108864 x 1); the flat plan is 12.
func TestResamplePlanCompact(t *testing.T) {
	src := call(t, createFn, num(1), num(1), col(red))
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	out := call(t, resizeFn, src, num(4194304), num(1), str("bilinear"))
	runtime.ReadMemStats(&b)
	// 16 MiB result + 48 MiB plan.
	if got := b.TotalAlloc - a.TotalAlloc; got > 72<<20 {
		t.Fatalf("resize allocated %d bytes", got)
	}
	if w, h := size(t, out); w != 4194304 || h != 1 || at(t, out, 4194303, 0) != red {
		t.Fatalf("got %dx%d", w, h)
	}
}

// TestResamplePlanCapped: a plan over limits.MaxAllocBytes is a limit error
// raised before anything is allocated.
func TestResamplePlanCapped(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	dst := &image.NRGBA{Rect: image.Rect(0, 0, 67108864, 1)} // never written
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	err := resample(interpreter.BuiltinCtx{}, "t", src, dst, catmullRom)
	runtime.ReadMemStats(&b)
	if err == nil || errorKind(t, err) != "limit" {
		t.Fatalf("err = %v", err)
	}
	if got := b.TotalAlloc - a.TotalAlloc; got > 1<<20 {
		t.Fatalf("allocated %d bytes before refusing", got)
	}
}

// TestRectangleOverflow: x + width past MaxInt used to wrap, and image.Rect
// then painted / cropped a different rectangle (fill coloured pixel (0,0)).
func TestRectangleOverflow(t *testing.T) {
	m := call(t, createFn, num(4), num(4))
	maxX := interpreter.IntVal(int64(^uint(0) >> 1))
	fails(t, fillFn, "out of range", m, maxX, num(0), num(2), num(1), col(red))
	fails(t, cropFn, "out of range", m, maxX, num(0), num(2), num(1))
	fails(t, fillFn, "out of range", m, num(0), maxX, num(1), num(2), col(red))
	if at(t, m, 0, 0) == red {
		t.Fatal("source changed")
	}
}

// TestIntArgRange: on a 32-bit int an argument past the int range is an
// error, not a truncated value (create(2^32 + 1, 1) made a 1x1 image).
func TestIntArgRange(t *testing.T) {
	if ^uint(0)>>32 != 0 {
		t.Skip("64-bit int: every int64 fits")
	}
	big := interpreter.IntVal(1<<32 + 1)
	fails(t, createFn, "out of range", big, num(1))
	m := call(t, createFn, num(2), num(2))
	fails(t, pixelFn, "out of range", m, interpreter.IntVal(1<<32), num(0))
	fails(t, rotateFn, "out of range", m, interpreter.IntVal(1<<32+90))
}

// TestGIFSubFrame: a first frame smaller than the logical screen decodes at
// the screen size (as `info` reports it), the frame at its offset.
func TestGIFSubFrame(t *testing.T) {
	frame := image.NewPaletted(image.Rect(2, 2, 3, 3), color.Palette{color.Transparent, red})
	frame.SetColorIndex(2, 2, 1)
	var buf bytes.Buffer
	err := gif.EncodeAll(&buf, &gif.GIF{
		Image:  []*image.Paletted{frame},
		Delay:  []int{0},
		Config: image.Config{ColorModel: frame.Palette, Width: 4, Height: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := call(t, decodeFn, raw(buf.Bytes()))
	if w, h := size(t, m); w != 4 || h != 4 {
		t.Fatalf("decoded %dx%d, want 4x4", w, h)
	}
	if at(t, m, 2, 2) != red || at(t, m, 0, 0).A != 0 || at(t, m, 3, 3).A != 0 {
		t.Fatalf("pixels (2,2)=%v (0,0)=%v", at(t, m, 2, 2), at(t, m, 0, 0))
	}
}

// TestConvertDirect16BitAndCMYK: 16-bit PNG and CMYK JPEG outputs convert
// without an allocation per pixel (the generic At path made one), with the
// values the generic color model gives (NRGBA64: the high bytes).
func TestConvertDirect16BitAndCMYK(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	r := image.Rect(1, 2, 33, 34)
	n64, r64, g16, cmyk := image.NewNRGBA64(r), image.NewRGBA64(r), image.NewGray16(r), image.NewCMYK(r)
	for _, pix := range [][]uint8{n64.Pix, g16.Pix, cmyk.Pix} {
		rng.Read(pix)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a := uint16(rng.Intn(0x10000))
			switch rng.Intn(4) {
			case 0:
				a = 0
			case 1:
				a = 0xffff
			}
			c := func() uint16 { return uint16(rng.Intn(int(a) + 1)) }
			r64.SetRGBA64(x, y, color.RGBA64{c(), c(), c(), a})
		}
	}
	for _, src := range []image.Image{n64, r64, g16, cmyk} {
		got, _ := toNRGBA(src)
		for y := 0; y < r.Dy(); y++ {
			for x := 0; x < r.Dx(); x++ {
				c := src.At(r.Min.X+x, r.Min.Y+y)
				want := color.NRGBAModel.Convert(c).(color.NRGBA)
				if n, ok := c.(color.NRGBA64); ok {
					want = color.NRGBA{uint8(n.R >> 8), uint8(n.G >> 8), uint8(n.B >> 8), uint8(n.A >> 8)}
				}
				if g := got.NRGBAAt(x, y); g != want {
					t.Fatalf("%T (%d,%d): %v, want %v", src, x, y, g, want)
				}
			}
		}
		if allocs := testing.AllocsPerRun(2, func() { toNRGBA(src) }); allocs > 3 {
			t.Errorf("%T: %v allocations for %d pixels", src, allocs, r.Dx()*r.Dy())
		}
	}
}
