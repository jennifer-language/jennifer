// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("images", `
decode(bytes, int?) images.Image
info(bytes) images.Info
encode(images.Image, string, int?) bytes
create(int, int, images.Color?) images.Image
fromPixels(int, int, bytes) images.Image
pixels(images.Image) bytes
width(images.Image) int
height(images.Image) int
pixel(images.Image, int, int) images.Color
rgb(int, int, int) images.Color
rgba(int, int, int, int) images.Color
hex(string) images.Color
crop(images.Image, int, int, int, int) images.Image
resize(images.Image, int, int, string?) images.Image
fit(images.Image, int, int, string?) images.Image
rotate(images.Image, int) images.Image
flip(images.Image, string) images.Image
grayscale(images.Image) images.Image
threshold(images.Image, int) images.Image
dither(images.Image) images.Image
fill(images.Image, int, int, int, int, images.Color) images.Image
draw(images.Image, images.Image, int, int) images.Image
`)
}
