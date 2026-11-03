// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"image"
	"math"
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

// taps is one output coordinate's contributions: weights for the source
// indices first, first+1, ...
type taps struct {
	first   int
	weights []float32
}

// plan computes the taps for scaling n source pixels to m. When shrinking, the
// kernel is widened by the scale factor so every source pixel contributes,
// which is what keeps a large reduction from aliasing.
func plan(n, m int, k kernel) []taps {
	ratio := float64(n) / float64(m)
	scale := math.Max(ratio, 1)
	support := k.support * scale
	out := make([]taps, m)
	for d := range out {
		center := (float64(d) + 0.5) * ratio
		lo := int(math.Floor(center - support))
		hi := int(math.Ceil(center + support))
		lo, hi = max(lo, 0), min(hi, n)
		ws := make([]float32, 0, hi-lo)
		var sum float64
		for i := lo; i < hi; i++ {
			x := math.Abs(float64(i)+0.5-center) / scale
			w := 0.0
			if x < k.support {
				w = k.at(x)
			}
			ws = append(ws, float32(w))
			sum += w
		}
		if sum != 0 {
			for i := range ws {
				ws[i] = float32(float64(ws[i]) / sum)
			}
		}
		out[d] = taps{first: lo, weights: ws}
	}
	return out
}

// resample scales src into dst with the kernel. Filtering runs on
// premultiplied channels, so a transparent pixel's color cannot bleed into
// its neighbours; the horizontal pass goes into a float buffer of
// dst-width x src-height and the vertical pass writes dst.
func resample(src, dst *image.NRGBA, k kernel) {
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
		return
	}
	cols := plan(sw, dw, k)
	rows := plan(sh, dh, k)
	tmp := make([]float32, dw*sh*4)
	for y := 0; y < sh; y++ {
		row := src.Pix[y*src.Stride:]
		for x, t := range cols {
			var r, g, b, a float32
			for j, w := range t.weights {
				p := row[4*(t.first+j):]
				pa := float32(p[3]) * w
				r += float32(p[0]) * pa
				g += float32(p[1]) * pa
				b += float32(p[2]) * pa
				a += pa
			}
			o := 4 * (y*dw + x)
			// r, g, b are premultiplied on a 0..255*255 scale.
			tmp[o], tmp[o+1], tmp[o+2], tmp[o+3] = r, g, b, a
		}
	}
	for y, t := range rows {
		for x := 0; x < dw; x++ {
			var r, g, b, a float32
			for j, w := range t.weights {
				o := 4 * ((t.first+j)*dw + x)
				r += tmp[o] * w
				g += tmp[o+1] * w
				b += tmp[o+2] * w
				a += tmp[o+3] * w
			}
			d := dst.PixOffset(x, y)
			if a <= 0.5 {
				dst.Pix[d], dst.Pix[d+1], dst.Pix[d+2], dst.Pix[d+3] = 0, 0, 0, 0
				continue
			}
			dst.Pix[d] = clamp8(r / a)
			dst.Pix[d+1] = clamp8(g / a)
			dst.Pix[d+2] = clamp8(b / a)
			dst.Pix[d+3] = clamp8(a)
		}
	}
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
