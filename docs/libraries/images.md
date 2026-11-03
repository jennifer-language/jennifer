# `images` - raster images

Enable with `use images;`. Decode, edit and encode PNG, JPEG and GIF pictures.
An image is an opaque `images.Image` whose pixels stay on the Go side (8-bit
RGBA, 4 bytes a pixel), so a multi-megapixel photo costs its pixel memory and
nothing more, and each operation runs as one Go loop rather than a `.j` loop
over a list. Images are **immutable**: every call returns a new image and
leaves its input alone, so value semantics hold without copying pixels on
every assignment. Backed by Go's `image/png`, `image/jpeg` and `image/gif`;
resampling, rotation, grayscale, dithering and compositing are written for the
library. No dependency; works on both binaries.

```jennifer
use images;
use fs;

def photo as images.Image init images.decode(fs.readBytes("holiday.jpg"));
def thumb as images.Image init images.fit($photo, 320, 320);
fs.writeBytes("thumb.jpg", images.encode($thumb, "jpeg", 85));
```

Runnable: [`examples/images.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/images.j).

## Types

- `images.Image` - the opaque image. `%v` shows `<images.Image 640x480>`; `==`
  compares size and pixels. An uninitialised `def m as images.Image;` is a 1x1
  transparent image.
- `images.Color { r as int, g as int, b as int, a as int }` - one pixel,
  non-premultiplied, each channel `0`..`255`. `a` is opacity: `255` opaque, `0`
  transparent. Build one with `rgb` / `rgba` / `hex`, or as a literal; a field
  left out of a literal is `0`, so `images.Color{r: 255}` is **transparent** red -
  use `images.rgb(255, 0, 0)` for opaque.
- `images.Info { format as string, width as int, height as int }` - what
  `info` reads from a header.

Coordinates are pixels from the top-left corner, `x` to the right and `y`
down.

## Surface

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `images.decode(data[, maxPixels])` | `images.Image` | Decode PNG, JPEG or GIF `bytes`; the format is detected. A GIF yields its first frame. Images over `maxPixels` (default 64 Mpx) are refused, see below. |
| `images.info(data)` | `images.Info` | Format (`"png"`, `"jpeg"`, `"gif"`) and size from the header alone, without decoding the pixels. |
| `images.encode(img, format[, quality])` | `bytes` | `format` is `"png"`, `"jpeg"` or `"gif"`; `quality` (`1`..`100`, default `90`) is for `"jpeg"` only. |
| `images.create(width, height[, color])` | `images.Image` | A new image filled with `color` (default transparent). |
| `images.fromPixels(width, height, data)` | `images.Image` | Build from raw RGBA `bytes`: `width * height * 4`, row by row. |
| `images.pixels(img)` | `bytes` | The raw RGBA bytes, row by row from the top left (a copy). |
| `images.width(img)` / `images.height(img)` | `int` | The size. |
| `images.pixel(img, x, y)` | `images.Color` | One pixel; outside the image is an error. |
| `images.rgb(r, g, b)` | `images.Color` | An opaque color. |
| `images.rgba(r, g, b, a)` | `images.Color` | A color with opacity `a`. |
| `images.hex(s)` | `images.Color` | Parse `"#rgb"`, `"#rrggbb"` or `"#rrggbbaa"` (no alpha = opaque). |
| `images.crop(img, x, y, width, height)` | `images.Image` | A rectangle, which must lie inside the image. |
| `images.resize(img, width, height[, filter])` | `images.Image` | Scale to exactly `width` x `height`. Pass `0` for one of them to keep the aspect ratio. |
| `images.fit(img, maxWidth, maxHeight[, filter])` | `images.Image` | Scale down to the largest size inside the box, keeping the aspect ratio. An image that already fits is returned unchanged (never enlarged). |
| `images.rotate(img, degrees)` | `images.Image` | Turn clockwise by a multiple of 90 (negative turns counter-clockwise). |
| `images.flip(img, direction)` | `images.Image` | Mirror: `"horizontal"` (left-right) or `"vertical"` (top-bottom). |
| `images.grayscale(img)` | `images.Image` | Luminance (Rec. 601 weights); alpha kept. |
| `images.threshold(img, level)` | `images.Image` | Black and white: luminance `>= level` becomes white. `level` is `0`..`256` (`256` = all black). |
| `images.dither(img)` | `images.Image` | Black and white by Floyd-Steinberg error diffusion, so mid-tones survive as dot density - for label and receipt printers, e-paper, 1-bit displays. |
| `images.fill(img, x, y, width, height, color)` | `images.Image` | Paint a rectangle (clipped to the image), alpha-blended over what is there. |
| `images.draw(dst, src, x, y)` | `images.Image` | Composite `src` over `dst` with its top left at (`x`, `y`), alpha-blended and clipped; `x` / `y` may be negative. |

A malformed file, an unknown format or filter, or an out-of-range argument is a
positioned runtime error, catchable with `try` / `catch`.

## Resizing

`filter` is optional: `"catmullrom"` (default; a sharp cubic, the usual choice
for photographs), `"bilinear"` (softer), or `"nearest"` (no blending; keeps hard
pixel edges, for pixel art and upscaling icons). When shrinking, the filter is
widened by the scale factor so every source pixel contributes, which keeps a
large reduction from aliasing (a fine pattern averages to its mean tone instead
of breaking into moire). Filtering runs on premultiplied color, so a
transparent pixel's color never bleeds into the edge of an opaque one.

```jennifer
def banner as images.Image init images.resize($logo, 1200, 0);     # keep aspect
def icon as images.Image init images.resize($sprite, 64, 64, "nearest");
```

## Editing pixels in bulk

There is deliberately no `setPixel`: an image is immutable, so each call would
copy the whole picture and a loop over pixels would be quadratic. For per-pixel
work take the bytes out, edit them in place (a `bytes` index write is cheap),
and build a new image:

```jennifer
def data as bytes init images.pixels($img);
for (def i in 0..len($data) // 4) {
    $data[$i * 4 + 3] = 128;                  # half opacity everywhere
}
def faded as images.Image init images.fromPixels(
    images.width($img), images.height($img), $data);
```

The same layout feeds other code a raster: rows top to bottom, 4 bytes (`r`,
`g`, `b`, `a`) a pixel, no padding.

## Encoding

- **PNG** is lossless and keeps alpha. An opaque image the library knows to be
  gray (made by `grayscale`, `threshold` or `dither`, or decoded from a gray
  file) is written as a one-channel grayscale PNG, which is smaller.
- **JPEG** is lossy and has no alpha: a translucent pixel is composited over
  **black**. To flatten onto white instead, draw the image onto a white canvas
  first: `images.draw(images.create($w, $h, images.rgb(255, 255, 255)), $img, 0, 0)`.
- **GIF** writes a single frame reduced to a 256-color palette; transparency is
  not kept. Prefer PNG unless a GIF is required.

## Untrusted images

A few hundred bytes of PNG can declare a 100000 x 100000 canvas, tens of
gigabytes once decoded. `decode` therefore reads the header first and refuses
an image with more than `maxPixels` pixels (default 67108864 = 64 Mpx, 256 MiB
of pixels) with a catchable `Error{kind: "limit"}` before allocating anything -
the same kind [`compress`](compress.md)`.unpack` and [`archive`](archive.md) use
for their caps. Pass a lower `maxPixels` for uploads, or check `info` first:

```jennifer
try {
    def avatar as images.Image init images.decode($upload, 4000000);   # 4 Mpx
} catch (e) {
    if ($e.kind == "limit") {
        # too large: reject the upload
    }
}
```

`create`, `fromPixels`, `resize` and `fit` refuse a result over the default
cap the same way.

## Not supported

Animated GIF frames beyond the first, WebP, BMP, TIFF, AVIF and HEIC, text
rendering, and arbitrary-angle rotation. Color is 8 bits a channel; a 16-bit PNG
is decoded to 8 bits.
