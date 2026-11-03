// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"image"
	"image/color"
)

// toNRGBA converts a decoded image to the library's one in-memory form: a
// compact NRGBA with origin (0, 0). The decoders' common outputs (YCbCr JPEG,
// paletted GIF / PNG, gray, RGBA, NRGBA) get direct loops; anything else goes
// through the generic color model. gray reports a grayscale source.
func toNRGBA(src image.Image) (*image.NRGBA, bool) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	switch s := src.(type) {
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			i := s.PixOffset(b.Min.X, b.Min.Y+y)
			copy(out.Pix[y*out.Stride:(y+1)*out.Stride], s.Pix[i:i+4*w])
		}
		return out, false
	case *image.RGBA:
		for y := 0; y < h; y++ {
			i := s.PixOffset(b.Min.X, b.Min.Y+y)
			for x := 0; x < w; x++ {
				p := s.Pix[i+4*x : i+4*x+4]
				d := out.Pix[y*out.Stride+4*x:]
				a := uint32(p[3])
				if a == 0 {
					continue
				}
				d[0] = uint8((uint32(p[0])*255 + a/2) / a)
				d[1] = uint8((uint32(p[1])*255 + a/2) / a)
				d[2] = uint8((uint32(p[2])*255 + a/2) / a)
				d[3] = p[3]
			}
		}
		return out, false
	case *image.Gray:
		for y := 0; y < h; y++ {
			i := s.PixOffset(b.Min.X, b.Min.Y+y)
			for x := 0; x < w; x++ {
				v := s.Pix[i+x]
				d := out.Pix[y*out.Stride+4*x:]
				d[0], d[1], d[2], d[3] = v, v, v, 255
			}
		}
		return out, true
	case *image.YCbCr:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				yi := s.YOffset(b.Min.X+x, b.Min.Y+y)
				ci := s.COffset(b.Min.X+x, b.Min.Y+y)
				r, g, bb := color.YCbCrToRGB(s.Y[yi], s.Cb[ci], s.Cr[ci])
				d := out.Pix[y*out.Stride+4*x:]
				d[0], d[1], d[2], d[3] = r, g, bb, 255
			}
		}
		return out, false
	case *image.Paletted:
		pal := make([]color.NRGBA, len(s.Palette))
		gray := true
		for i, c := range s.Palette {
			pal[i] = color.NRGBAModel.Convert(c).(color.NRGBA)
			gray = gray && pal[i].R == pal[i].G && pal[i].G == pal[i].B
		}
		for y := 0; y < h; y++ {
			i := s.PixOffset(b.Min.X, b.Min.Y+y)
			for x := 0; x < w; x++ {
				idx := int(s.Pix[i+x])
				if idx >= len(pal) {
					continue // an index past the palette reads as transparent
				}
				c := pal[idx]
				d := out.Pix[y*out.Stride+4*x:]
				d[0], d[1], d[2], d[3] = c.R, c.G, c.B, c.A
			}
		}
		return out, gray
	}
	gray := src.ColorModel() == color.GrayModel || src.ColorModel() == color.Gray16Model
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			d := out.Pix[y*out.Stride+4*x:]
			d[0], d[1], d[2], d[3] = c.R, c.G, c.B, c.A
		}
	}
	return out, gray
}

// opaque reports whether every pixel has full alpha.
func opaque(m *image.NRGBA) bool {
	for i := 3; i < len(m.Pix); i += 4 {
		if m.Pix[i] != 255 {
			return false
		}
	}
	return true
}

// grayPlane takes the red channel of an image whose r, g and b are equal.
func grayPlane(m *image.NRGBA) *image.Gray {
	out := image.NewGray(m.Rect)
	for i, j := 0, 0; i < len(m.Pix); i, j = i+4, j+1 {
		out.Pix[j] = m.Pix[i]
	}
	return out
}

// premultiplied converts to RGBA, which is what the JPEG encoder reads fast;
// premultiplying composites a translucent pixel over black.
func premultiplied(m *image.NRGBA) *image.RGBA {
	out := image.NewRGBA(m.Rect)
	for i := 0; i < len(m.Pix); i += 4 {
		a := uint32(m.Pix[i+3])
		out.Pix[i] = uint8((uint32(m.Pix[i])*a + 127) / 255)
		out.Pix[i+1] = uint8((uint32(m.Pix[i+1])*a + 127) / 255)
		out.Pix[i+2] = uint8((uint32(m.Pix[i+2])*a + 127) / 255)
		out.Pix[i+3] = 255
	}
	return out
}
