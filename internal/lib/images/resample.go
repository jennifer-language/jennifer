// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"fmt"
	"image"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// kernel is a separable resampling filter: support is its radius in source
// pixels at scale 1 and at its weight over that range. A nil at means
// nearest-neighbour, which picks one pixel instead of weighting several.
type kernel struct {
	support float64
	at      func(x float64) float64
}

var (
	nearest  = kernel{}
	bilinear = kernel{support: 1, at: func(x float64) float64 {
		return 1 - x
	}}
	// catmullRom is the cubic with B = 0, C = 0.5: sharp, and the usual default
	// for photographs.
	catmullRom = kernel{support: 2, at: func(x float64) float64 {
		if x < 1 {
			return (1.5*x-2.5)*x*x + 1
		}
		return ((-0.5*x+2.5)*x-4)*x + 2
	}}
)

// taps holds every output coordinate's contributions in three flat slices
// (12 bytes per output coordinate plus 4 per weight; a struct and a slice per
// coordinate cost ~40 bytes): output d weights source indices first[d],
// first[d]+1, ... by weights[off[d]:off[d+1]].
type taps struct {
	first   []int32
	off     []int32
	weights []float32
}

// span is the source range [lo, hi) that output coordinate d of an n -> m
// scale reads.
func span(d, n int, ratio, support float64) (int, int, float64) {
	center := (float64(d) + 0.5) * ratio
	lo := int(math.Floor(center - support))
	hi := int(math.Ceil(center + support))
	return max(lo, 0), min(hi, n), center
}

// planBytes is the size of plan(n, m, k) in bytes, ok=false past MaxInt64.
func planBytes(n, m int, k kernel) (int64, bool) {
	ratio := float64(n) / float64(m)
	support := k.support * math.Max(ratio, 1)
	var total int64
	for d := 0; d < m; d++ {
		lo, hi, _ := span(d, n, ratio, support)
		total += int64(hi - lo)
	}
	cells, ok := limits.AddInt64(total, 2*int64(m)+1)
	if !ok {
		return 0, false
	}
	return limits.MulInt64(cells, 4)
}

// plan computes the taps for scaling n source pixels to m. When shrinking, the
// kernel is widened by the scale factor so every source pixel contributes,
// which is what keeps a large reduction from aliasing. The caller checks
// planBytes first (the total weight count then fits int32).
func plan(n, m int, k kernel) taps {
	ratio := float64(n) / float64(m)
	scale := math.Max(ratio, 1)
	support := k.support * scale
	b, _ := planBytes(n, m, k)
	t := taps{
		first:   make([]int32, m),
		off:     make([]int32, m+1),
		weights: make([]float32, 0, b/4-2*int64(m)-1),
	}
	for d := 0; d < m; d++ {
		lo, hi, center := span(d, n, ratio, support)
		start := len(t.weights)
		var sum float64
		for i := lo; i < hi; i++ {
			x := math.Abs(float64(i)+0.5-center) / scale
			w := 0.0
			if x < k.support {
				w = k.at(x)
			}
			t.weights = append(t.weights, float32(w))
			sum += w
		}
		if sum != 0 {
			ws := t.weights[start:]
			for i := range ws {
				ws[i] = float32(float64(ws[i]) / sum)
			}
		}
		t.first[d] = int32(lo)
		t.off[d+1] = int32(len(t.weights))
	}
	return t
}

// at returns output d's first source index and weights.
func (t *taps) at(d int) (int, []float32) {
	return int(t.first[d]), t.weights[t.off[d]:t.off[d+1]]
}

// resampleCells caps resample's float work buffer: 16 bytes a cell, so the
// buffer stays within limits.MaxAllocBytes (256 MiB, 32 MiB on jennifer-tiny).
// A variable so tests can lower it.
var resampleCells = int64(limits.MaxAllocBytes / 16)

// rowSpan returns output d's source range [lo, hi).
func (t *taps) rowSpan(d int) (int, int) {
	lo := int(t.first[d])
	return lo, lo + int(t.off[d+1]-t.off[d])
}

// stripEnd returns the end of the strip of output rows that starts at y0 in a
// horizontal-first pass: rows [y0, y1) read source rows [lo, hi), and
// (hi - lo) * dw cells fit in resampleCells unless one row alone does not.
// Both ends of a row's span grow with the row, so the strip's source range is
// the first row's lo to the last row's hi.
func stripEnd(rows *taps, y0, dh, dw int) (y1, lo, hi int) {
	lo, hi = rows.rowSpan(y0)
	y1 = y0 + 1
	for y1 < dh {
		_, h := rows.rowSpan(y1)
		if int64(h-lo)*int64(dw) > resampleCells {
			break
		}
		hi = h
		y1++
	}
	return y1, lo, hi
}

// resample scales src into dst with the kernel. Filtering runs on
// premultiplied channels, so a transparent pixel's color cannot bleed into
// its neighbours. The two separable passes go through a float buffer of 16
// bytes per cell. Vertical-first needs one src-width row of it per output
// row. Horizontal-first needs dst-width x the source rows a strip of output
// rows reads, all src-height rows when that fits in resampleCells; otherwise
// it works in strips (re-filtering the few source rows two strips share), so
// the buffer never passes resampleCells. Of the two orders the one whose
// whole-image buffer (dst-width x src-height or src-width x dst-height) is
// smaller runs, the other one only when a single row or strip of the first
// does not fit; when neither fits, Error{kind: "limit"}.
func resample(ctx interpreter.BuiltinCtx, fn string, src, dst *image.NRGBA, k kernel) error {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	if k.at == nil {
		for y := 0; y < dh; y++ {
			sy := min(int((float64(y)+0.5)*float64(sh)/float64(dh)), sh-1)
			for x := 0; x < dw; x++ {
				sx := min(int((float64(x)+0.5)*float64(sw)/float64(dw)), sw-1)
				s := src.PixOffset(sx, sy)
				d := dst.PixOffset(x, y)
				copy(dst.Pix[d:d+4], src.Pix[s:s+4])
			}
		}
		return nil
	}
	// The filter weights: about 4 bytes per source pixel per unit of kernel
	// width along each side when shrinking, 12-28 per output pixel along a
	// side when enlarging. Capped like any one buffer, so a 1 x 1 image
	// stretched to a 67108864 x 1 one is a limit error, not gigabytes.
	cb, okc := planBytes(sw, dw, k)
	rb, okr := planBytes(sh, dh, k)
	need, ok := limits.AddInt64(cb, rb)
	if !okc || !okr || !ok || need > limits.MaxAllocBytes {
		return interpreter.RaiseError("limit",
			fmt.Sprintf("%s: resampling %dx%d to %dx%d needs %d bytes of filter weights, over the %d-byte limit", fn, sw, sh, dw, dh, need, int64(limits.MaxAllocBytes)),
			ctx.File, ctx.Line, ctx.Col)
	}
	cols := plan(sw, dw, k)
	rows := plan(sh, dh, k)
	hFirst := int64(dw) * int64(sh) // cells; each factor < 2^31, no overflow
	vFirst := int64(sw) * int64(dh)
	// The smallest buffer each order can work with: one strip of a single
	// output row horizontal-first, one row vertical-first.
	hMin := int64(0)
	for y := 0; y < dh; y++ {
		lo, hi := rows.rowSpan(y)
		hMin = max(hMin, int64(hi-lo)*int64(dw))
	}
	vMin := int64(sw)
	hOK, vOK := hMin <= resampleCells, vMin <= resampleCells
	if !hOK && !vOK {
		return interpreter.RaiseError("limit",
			fmt.Sprintf("%s: resampling %dx%d to %dx%d needs a %d-cell buffer, over the %d-cell limit", fn, sw, sh, dw, dh, min(hMin, vMin), resampleCells),
			ctx.File, ctx.Line, ctx.Col)
	}
	if hOK && (hFirst <= vFirst || !vOK) {
		resampleHFirst(src, dst, &cols, &rows)
		return nil
	}
	resampleVFirst(src, dst, &cols, &rows)
	return nil
}

// resampleHFirst filters each strip of output rows horizontally into a
// dst-width buffer of the source rows it reads, then vertically into dst.
func resampleHFirst(src, dst *image.NRGBA, cols, rows *taps) {
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	var cells int64
	for y0 := 0; y0 < dh; {
		y1, lo, hi := stripEnd(rows, y0, dh, dw)
		cells = max(cells, int64(hi-lo)*int64(dw))
		y0 = y1
	}
	tmp := make([]float32, cells*4)
	for y0 := 0; y0 < dh; {
		y1, lo, hi := stripEnd(rows, y0, dh, dw)
		for sy := lo; sy < hi; sy++ {
			row := src.Pix[sy*src.Stride:]
			for x := 0; x < dw; x++ {
				first, ws := cols.at(x)
				var r, g, b, a float32
				for j, w := range ws {
					p := row[4*(first+j):]
					pa := float32(p[3]) * w
					r += float32(p[0]) * pa
					g += float32(p[1]) * pa
					b += float32(p[2]) * pa
					a += pa
				}
				o := 4 * ((sy-lo)*dw + x)
				// r, g, b are premultiplied on a 0..255*255 scale.
				tmp[o], tmp[o+1], tmp[o+2], tmp[o+3] = r, g, b, a
			}
		}
		for y := y0; y < y1; y++ {
			first, ws := rows.at(y)
			for x := 0; x < dw; x++ {
				var r, g, b, a float32
				for j, w := range ws {
					o := 4 * ((first+j-lo)*dw + x)
					r += tmp[o] * w
					g += tmp[o+1] * w
					b += tmp[o+2] * w
					a += tmp[o+3] * w
				}
				store(dst, x, y, r, g, b, a)
			}
		}
		y0 = y1
	}
}

// resampleVFirst filters each output row vertically into a src-width buffer,
// then horizontally into dst.
func resampleVFirst(src, dst *image.NRGBA, cols, rows *taps) {
	sw := src.Rect.Dx()
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	tmp := make([]float32, sw*4)
	for y := 0; y < dh; y++ {
		first, ws := rows.at(y)
		for x := 0; x < sw; x++ {
			var r, g, b, a float32
			for j, w := range ws {
				p := src.Pix[(first+j)*src.Stride+4*x:]
				pa := float32(p[3]) * w
				r += float32(p[0]) * pa
				g += float32(p[1]) * pa
				b += float32(p[2]) * pa
				a += pa
			}
			o := 4 * x
			tmp[o], tmp[o+1], tmp[o+2], tmp[o+3] = r, g, b, a
		}
		for x := 0; x < dw; x++ {
			first, ws := cols.at(x)
			var r, g, b, a float32
			for j, w := range ws {
				o := 4 * (first + j)
				r += tmp[o] * w
				g += tmp[o+1] * w
				b += tmp[o+2] * w
				a += tmp[o+3] * w
			}
			store(dst, x, y, r, g, b, a)
		}
	}
}

// store un-premultiplies a filtered pixel into dst.
func store(dst *image.NRGBA, x, y int, r, g, b, a float32) {
	d := dst.PixOffset(x, y)
	if a <= 0.5 {
		dst.Pix[d], dst.Pix[d+1], dst.Pix[d+2], dst.Pix[d+3] = 0, 0, 0, 0
		return
	}
	dst.Pix[d] = clamp8(r / a)
	dst.Pix[d+1] = clamp8(g / a)
	dst.Pix[d+2] = clamp8(b / a)
	dst.Pix[d+3] = clamp8(a)
}

// clamp8 rounds to the nearest byte; Catmull-Rom overshoots near edges.
func clamp8(v float32) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v + 0.5)
}
