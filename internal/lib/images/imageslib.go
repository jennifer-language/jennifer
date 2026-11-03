// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package imageslib is the `images` library: raster images decoded, edited and
// encoded on the Go side. An image is an opaque `images.Image` whose pixels
// stay in a Go `*image.NRGBA` (8-bit, non-premultiplied RGBA), so a
// multi-megapixel photo costs 4 bytes a pixel rather than a list of Values,
// and every operation is one Go loop. The handle is immutable: each operation
// returns a new image, so copies of a handle may share the payload (value
// semantics hold without copying pixels). Codecs are the Go standard
// library's `image/png`, `image/jpeg` and `image/gif`; resampling, rotation,
// grayscale, dithering and compositing are hand-written here. No reflect and
// no dependency, so both binaries build it.
package imageslib

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`images.`) and the `use` name.
const LibraryName = "images"

// maxPixels is the default cap on the pixels an image may hold: 64 Mpx, which
// is 256 MiB as NRGBA. decode checks the header against it before allocating,
// so a small file declaring a gigapixel canvas fails with Error{kind: "limit"}
// instead of exhausting memory; decode's optional argument moves it. Every
// operation that allocates a new canvas (create, fromPixels, resize, fit)
// enforces it too.
const maxPixels = 64 << 20

// defaultJPEGQuality matches what most tools default to.
const defaultJPEGQuality = 90

type Value = interpreter.Value

// imageValue is the native payload of an images.Image. img always has
// Stride == 4*width and origin (0, 0). gray records that every pixel has
// r == g == b, so encode can write a grayscale PNG / JPEG.
type imageValue struct {
	img  *image.NRGBA
	gray bool
}

func (v *imageValue) EqualNative(other interpreter.NativeObject) bool {
	o, ok := other.(*imageValue)
	if !ok {
		return false
	}
	return v.img.Rect.Eq(o.img.Rect) && bytes.Equal(v.img.Pix, o.img.Pix)
}

func (v *imageValue) DisplayNative() string {
	return fmt.Sprintf("<images.Image %dx%d>", v.img.Rect.Dx(), v.img.Rect.Dy())
}

// Install registers the images surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedObject(LibraryName, "Image", nil)
	// An uninitialised `def m as images.Image;` is a 1x1 transparent pixel, so
	// every accessor works on it.
	in.SetNativeObjectZero(LibraryName, "Image", &imageValue{img: image.NewNRGBA(image.Rect(0, 0, 1, 1))})
	intField := func(name string) parser.StructField {
		return parser.StructField{Name: name, Type: parser.PrimitiveType(parser.TypeInt)}
	}
	in.RegisterNamespacedStruct(LibraryName, "Color", []parser.StructField{
		intField("r"), intField("g"), intField("b"), intField("a"),
	})
	in.RegisterNamespacedStruct(LibraryName, "Info", []parser.StructField{
		{Name: "format", Type: parser.PrimitiveType(parser.TypeString)},
		intField("width"), intField("height"),
	})

	reg := func(name string, fn interpreter.Builtin) { in.RegisterNamespaced(LibraryName, name, fn) }
	reg("decode", decodeFn)
	reg("info", infoFn)
	reg("encode", encodeFn)
	reg("create", createFn)
	reg("fromPixels", fromPixelsFn)
	reg("pixels", pixelsFn)
	reg("width", func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		m, err := oneImage("images.width", args)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.IntVal(int64(m.img.Rect.Dx())), nil
	})
	reg("height", func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		m, err := oneImage("images.height", args)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.IntVal(int64(m.img.Rect.Dy())), nil
	})
	reg("pixel", pixelFn)
	reg("rgb", rgbFn)
	reg("rgba", rgbaFn)
	reg("hex", hexFn)
	reg("crop", cropFn)
	reg("resize", resizeFn)
	reg("fit", fitFn)
	reg("rotate", rotateFn)
	reg("flip", flipFn)
	reg("grayscale", grayscaleFn)
	reg("threshold", thresholdFn)
	reg("dither", ditherFn)
	reg("fill", fillFn)
	reg("draw", drawFn)
}

// --- argument helpers -------------------------------------------------------

func wrap(img *image.NRGBA, gray bool) Value {
	return interpreter.NativeObjectVal(LibraryName, "Image", &imageValue{img: img, gray: gray})
}

func imageArg(fn string, v Value, what string) (*imageValue, error) {
	n, ok := v.AsNativeObject(LibraryName, "Image")
	if !ok {
		return nil, fmt.Errorf("%s: %s must be an images.Image, got %s", fn, what, describe(v))
	}
	return n.(*imageValue), nil
}

func oneImage(fn string, args []Value) (*imageValue, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s expects 1 argument (image), got %d", fn, len(args))
	}
	return imageArg(fn, args[0], "argument")
}

func describe(v Value) string {
	if v.Kind == interpreter.KindObject || v.Kind == interpreter.KindStruct {
		return v.StructNS + "." + v.StructName
	}
	return v.Kind.String()
}

func intArg(fn string, v Value, what string) (int, error) {
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fn, what, describe(v))
	}
	return int(v.Int), nil
}

func stringArg(fn string, v Value, what string) (string, error) {
	if v.Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fn, what, describe(v))
	}
	return v.Str, nil
}

// colorArg reads an images.Color; a field outside 0..255 is an error.
func colorArg(fn string, v Value) (color.NRGBA, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Color" {
		return color.NRGBA{}, fmt.Errorf("%s: color must be an images.Color, got %s", fn, describe(v))
	}
	var c [4]uint8
	for _, f := range v.Fields {
		idx := strings.Index("rgba", f.Name)
		if len(f.Name) != 1 || idx < 0 {
			continue
		}
		if f.Value.Kind != interpreter.KindInt || f.Value.Int < 0 || f.Value.Int > 255 {
			return color.NRGBA{}, fmt.Errorf("%s: images.Color.%s must be an int in 0..255", fn, f.Name)
		}
		c[idx] = uint8(f.Value.Int)
	}
	return color.NRGBA{R: c[0], G: c[1], B: c[2], A: c[3]}, nil
}

func colorVal(c color.NRGBA) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Color", []interpreter.StructField{
		{Name: "r", Value: interpreter.IntVal(int64(c.R))},
		{Name: "g", Value: interpreter.IntVal(int64(c.G))},
		{Name: "b", Value: interpreter.IntVal(int64(c.B))},
		{Name: "a", Value: interpreter.IntVal(int64(c.A))},
	})
}

// limitErr raises the catchable Error{kind: "limit"} for an oversized canvas.
func limitErr(ctx interpreter.BuiltinCtx, fn string, w, h int, limit int64) error {
	return interpreter.RaiseError("limit",
		fmt.Sprintf("%s: a %dx%d image exceeds the %d-pixel limit", fn, w, h, limit),
		ctx.File, ctx.Line, ctx.Col)
}

// newCanvas allocates a w x h image after checking the size against the cap.
func newCanvas(ctx interpreter.BuiltinCtx, fn string, w, h int) (*image.NRGBA, error) {
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("%s: width and height must be at least 1, got %dx%d", fn, w, h)
	}
	if int64(w)*int64(h) > maxPixels {
		return nil, limitErr(ctx, fn, w, h, maxPixels)
	}
	return image.NewNRGBA(image.Rect(0, 0, w, h)), nil
}

// --- codecs -----------------------------------------------------------------

// decodeFn implements `images.decode(data[, maxPixels]) -> images.Image`.
func decodeFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.decode"
	if len(args) != 1 && len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 or 2 arguments (data[, maxPixels]), got %d", fn, len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: data must be bytes, got %s", fn, describe(args[0]))
	}
	limit := int64(maxPixels)
	if len(args) == 2 {
		n, err := intArg(fn, args[1], "maxPixels")
		if err != nil {
			return interpreter.Null(), err
		}
		if n < 1 {
			return interpreter.Null(), fmt.Errorf("%s: maxPixels must be at least 1, got %d", fn, n)
		}
		limit = int64(n)
	}
	data := args[0].Bytes
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > limit {
		return interpreter.Null(), limitErr(ctx, fn, cfg.Width, cfg.Height, limit)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, err)
	}
	img, gray := toNRGBA(src)
	return wrap(img, gray), nil
}

// infoFn implements `images.info(data) -> images.Info`: the format and size
// from the header alone, without decoding the pixels.
func infoFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.info"
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (data), got %d", fn, len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: data must be bytes, got %s", fn, describe(args[0]))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(args[0].Bytes))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, err)
	}
	return interpreter.NamespacedStructVal(LibraryName, "Info", []interpreter.StructField{
		{Name: "format", Value: interpreter.StringVal(format)},
		{Name: "width", Value: interpreter.IntVal(int64(cfg.Width))},
		{Name: "height", Value: interpreter.IntVal(int64(cfg.Height))},
	}), nil
}

// encodeFn implements `images.encode(img, format[, quality]) -> bytes`.
func encodeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.encode"
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 or 3 arguments (image, format[, quality]), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	format, err := stringArg(fn, args[1], "format")
	if err != nil {
		return interpreter.Null(), err
	}
	quality := defaultJPEGQuality
	if len(args) == 3 {
		if format != "jpeg" {
			return interpreter.Null(), fmt.Errorf("%s: quality applies only to \"jpeg\", not %q", fn, format)
		}
		if quality, err = intArg(fn, args[2], "quality"); err != nil {
			return interpreter.Null(), err
		}
		if quality < 1 || quality > 100 {
			return interpreter.Null(), fmt.Errorf("%s: quality must be in 1..100, got %d", fn, quality)
		}
	}
	var buf bytes.Buffer
	switch format {
	case "png":
		var out image.Image = m.img
		if m.gray && opaque(m.img) {
			out = grayPlane(m.img)
		}
		err = png.Encode(&buf, out)
	case "jpeg":
		// JPEG has no alpha: a translucent pixel is composited over black.
		var out image.Image
		if m.gray {
			out = grayPlane(m.img)
		} else {
			out = premultiplied(m.img)
		}
		err = jpeg.Encode(&buf, out, &jpeg.Options{Quality: quality})
	case "gif":
		err = gif.Encode(&buf, m.img, nil)
	default:
		return interpreter.Null(), fmt.Errorf("%s: unknown format %q; known: \"png\", \"jpeg\", \"gif\"", fn, format)
	}
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, err)
	}
	return interpreter.BytesVal(buf.Bytes()), nil
}

// --- construction and pixel access -------------------------------------------

// createFn implements `images.create(width, height[, color]) -> images.Image`.
func createFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.create"
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 or 3 arguments (width, height[, color]), got %d", fn, len(args))
	}
	w, err := intArg(fn, args[0], "width")
	if err != nil {
		return interpreter.Null(), err
	}
	h, err := intArg(fn, args[1], "height")
	if err != nil {
		return interpreter.Null(), err
	}
	var c color.NRGBA
	if len(args) == 3 {
		if c, err = colorArg(fn, args[2]); err != nil {
			return interpreter.Null(), err
		}
	}
	img, err := newCanvas(ctx, fn, w, h)
	if err != nil {
		return interpreter.Null(), err
	}
	if c != (color.NRGBA{}) {
		px := [4]uint8{c.R, c.G, c.B, c.A}
		for i := 0; i < len(img.Pix); i += 4 {
			copy(img.Pix[i:i+4], px[:])
		}
	}
	return wrap(img, c.R == c.G && c.G == c.B), nil
}

// fromPixelsFn implements `images.fromPixels(width, height, data)`: data is
// width*height RGBA quadruples, row by row (the layout `pixels` returns).
func fromPixelsFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.fromPixels"
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("%s expects 3 arguments (width, height, data), got %d", fn, len(args))
	}
	w, err := intArg(fn, args[0], "width")
	if err != nil {
		return interpreter.Null(), err
	}
	h, err := intArg(fn, args[1], "height")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("%s: data must be bytes, got %s", fn, describe(args[2]))
	}
	img, err := newCanvas(ctx, fn, w, h)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(args[2].Bytes) != len(img.Pix) {
		return interpreter.Null(), fmt.Errorf("%s: a %dx%d image needs %d bytes (4 per pixel), got %d",
			fn, w, h, len(img.Pix), len(args[2].Bytes))
	}
	copy(img.Pix, args[2].Bytes)
	return wrap(img, false), nil
}

// pixelsFn implements `images.pixels(img) -> bytes`: RGBA, 4 bytes a pixel,
// row by row from the top left.
func pixelsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	m, err := oneImage("images.pixels", args)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]byte, len(m.img.Pix))
	copy(out, m.img.Pix)
	return interpreter.BytesVal(out), nil
}

// pixelFn implements `images.pixel(img, x, y) -> images.Color`.
func pixelFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.pixel"
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("%s expects 3 arguments (image, x, y), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	x, err := intArg(fn, args[1], "x")
	if err != nil {
		return interpreter.Null(), err
	}
	y, err := intArg(fn, args[2], "y")
	if err != nil {
		return interpreter.Null(), err
	}
	w, h := m.img.Rect.Dx(), m.img.Rect.Dy()
	if x < 0 || y < 0 || x >= w || y >= h {
		return interpreter.Null(), fmt.Errorf("%s: (%d, %d) is outside the %dx%d image", fn, x, y, w, h)
	}
	return colorVal(m.img.NRGBAAt(x, y)), nil
}

func channels(fn string, args []Value, names ...string) ([]uint8, error) {
	if len(args) != len(names) {
		return nil, fmt.Errorf("%s expects %d arguments (%s), got %d", fn, len(names), strings.Join(names, ", "), len(args))
	}
	out := make([]uint8, len(names))
	for i, a := range args {
		n, err := intArg(fn, a, names[i])
		if err != nil {
			return nil, err
		}
		if n < 0 || n > 255 {
			return nil, fmt.Errorf("%s: %s must be in 0..255, got %d", fn, names[i], n)
		}
		out[i] = uint8(n)
	}
	return out, nil
}

// rgbFn implements `images.rgb(r, g, b) -> images.Color` (opaque).
func rgbFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	c, err := channels("images.rgb", args, "r", "g", "b")
	if err != nil {
		return interpreter.Null(), err
	}
	return colorVal(color.NRGBA{R: c[0], G: c[1], B: c[2], A: 255}), nil
}

// rgbaFn implements `images.rgba(r, g, b, a) -> images.Color`.
func rgbaFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	c, err := channels("images.rgba", args, "r", "g", "b", "a")
	if err != nil {
		return interpreter.Null(), err
	}
	return colorVal(color.NRGBA{R: c[0], G: c[1], B: c[2], A: c[3]}), nil
}

// hexFn implements `images.hex(s) -> images.Color` for "#rgb", "#rrggbb" and
// "#rrggbbaa" (a missing alpha is opaque).
func hexFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.hex"
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (string), got %d", fn, len(args))
	}
	s, err := stringArg(fn, args[0], "argument")
	if err != nil {
		return interpreter.Null(), err
	}
	bad := fmt.Errorf("%s: %q is not a #rgb, #rrggbb or #rrggbbaa color", fn, s)
	if !strings.HasPrefix(s, "#") {
		return interpreter.Null(), bad
	}
	digits := s[1:]
	if len(digits) == 3 {
		digits = string([]byte{digits[0], digits[0], digits[1], digits[1], digits[2], digits[2]})
	}
	if len(digits) == 6 {
		digits += "ff"
	}
	if len(digits) != 8 {
		return interpreter.Null(), bad
	}
	var c [4]uint8
	for i := 0; i < 4; i++ {
		hi, ok1 := hexDigit(digits[2*i])
		lo, ok2 := hexDigit(digits[2*i+1])
		if !ok1 || !ok2 {
			return interpreter.Null(), bad
		}
		c[i] = hi<<4 | lo
	}
	return colorVal(color.NRGBA{R: c[0], G: c[1], B: c[2], A: c[3]}), nil
}

func hexDigit(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

// --- geometry ---------------------------------------------------------------

// cropFn implements `images.crop(img, x, y, width, height)`; the rectangle must
// lie inside the image.
func cropFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.crop"
	if len(args) != 5 {
		return interpreter.Null(), fmt.Errorf("%s expects 5 arguments (image, x, y, width, height), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	r, err := rectArgs(fn, args[1:5])
	if err != nil {
		return interpreter.Null(), err
	}
	if r.Dx() < 1 || r.Dy() < 1 {
		return interpreter.Null(), fmt.Errorf("%s: width and height must be at least 1, got %dx%d", fn, r.Dx(), r.Dy())
	}
	if !r.In(m.img.Rect) {
		return interpreter.Null(), fmt.Errorf("%s: %dx%d at (%d, %d) is outside the %dx%d image",
			fn, r.Dx(), r.Dy(), r.Min.X, r.Min.Y, m.img.Rect.Dx(), m.img.Rect.Dy())
	}
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowBytes := 4 * r.Dx()
	for y := 0; y < r.Dy(); y++ {
		off := m.img.PixOffset(r.Min.X, r.Min.Y+y)
		copy(out.Pix[y*out.Stride:y*out.Stride+rowBytes], m.img.Pix[off:off+rowBytes])
	}
	return wrap(out, m.gray), nil
}

// rectArgs reads x, y, width, height into a rectangle.
func rectArgs(fn string, args []Value) (image.Rectangle, error) {
	var n [4]int
	for i, name := range []string{"x", "y", "width", "height"} {
		v, err := intArg(fn, args[i], name)
		if err != nil {
			return image.Rectangle{}, err
		}
		n[i] = v
	}
	if n[2] < 0 || n[3] < 0 {
		return image.Rectangle{}, fmt.Errorf("%s: width and height must not be negative, got %dx%d", fn, n[2], n[3])
	}
	return image.Rect(n[0], n[1], n[0]+n[2], n[1]+n[3]), nil
}

// filterArg reads the optional resampling filter name.
func filterArg(fn string, args []Value, idx int) (kernel, error) {
	if len(args) <= idx {
		return catmullRom, nil
	}
	name, err := stringArg(fn, args[idx], "filter")
	if err != nil {
		return kernel{}, err
	}
	switch name {
	case "nearest":
		return nearest, nil
	case "bilinear":
		return bilinear, nil
	case "catmullrom":
		return catmullRom, nil
	}
	return kernel{}, fmt.Errorf("%s: unknown filter %q; known: \"nearest\", \"bilinear\", \"catmullrom\"", fn, name)
}

// resizeFn implements `images.resize(img, width, height[, filter])`. A 0
// width or height is computed from the other, keeping the aspect ratio.
func resizeFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.resize"
	if len(args) != 3 && len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("%s expects 3 or 4 arguments (image, width, height[, filter]), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	w, err := intArg(fn, args[1], "width")
	if err != nil {
		return interpreter.Null(), err
	}
	h, err := intArg(fn, args[2], "height")
	if err != nil {
		return interpreter.Null(), err
	}
	k, err := filterArg(fn, args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	sw, sh := m.img.Rect.Dx(), m.img.Rect.Dy()
	switch {
	case w < 0 || h < 0 || (w == 0 && h == 0):
		return interpreter.Null(), fmt.Errorf("%s: width and height must not be negative, and only one may be 0; got %dx%d", fn, w, h)
	case w == 0:
		w = max(1, int((int64(sw)*int64(h)+int64(sh)/2)/int64(sh)))
	case h == 0:
		h = max(1, int((int64(sh)*int64(w)+int64(sw)/2)/int64(sw)))
	}
	out, err := newCanvas(ctx, fn, w, h)
	if err != nil {
		return interpreter.Null(), err
	}
	resample(m.img, out, k)
	return wrap(out, m.gray), nil
}

// fitFn implements `images.fit(img, maxWidth, maxHeight[, filter])`: the
// largest size inside the box that keeps the aspect ratio, never enlarging.
func fitFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.fit"
	if len(args) != 3 && len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("%s expects 3 or 4 arguments (image, maxWidth, maxHeight[, filter]), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	bw, err := intArg(fn, args[1], "maxWidth")
	if err != nil {
		return interpreter.Null(), err
	}
	bh, err := intArg(fn, args[2], "maxHeight")
	if err != nil {
		return interpreter.Null(), err
	}
	if bw < 1 || bh < 1 {
		return interpreter.Null(), fmt.Errorf("%s: maxWidth and maxHeight must be at least 1, got %dx%d", fn, bw, bh)
	}
	k, err := filterArg(fn, args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	sw, sh := m.img.Rect.Dx(), m.img.Rect.Dy()
	if sw <= bw && sh <= bh {
		return args[0], nil
	}
	// Scale by the tighter of the two ratios, comparing bw/sw with bh/sh
	// exactly in integers.
	w, h := bw, bh
	if int64(bw)*int64(sh) <= int64(bh)*int64(sw) {
		h = max(1, int((int64(sh)*int64(bw)+int64(sw)/2)/int64(sw)))
	} else {
		w = max(1, int((int64(sw)*int64(bh)+int64(sh)/2)/int64(sh)))
	}
	out, err := newCanvas(ctx, fn, w, h)
	if err != nil {
		return interpreter.Null(), err
	}
	resample(m.img, out, k)
	return wrap(out, m.gray), nil
}

// rotateFn implements `images.rotate(img, degrees)` for multiples of 90,
// clockwise (a negative angle turns counter-clockwise).
func rotateFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.rotate"
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (image, degrees), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	deg, err := intArg(fn, args[1], "degrees")
	if err != nil {
		return interpreter.Null(), err
	}
	if deg%90 != 0 {
		return interpreter.Null(), fmt.Errorf("%s: degrees must be a multiple of 90, got %d", fn, deg)
	}
	turns := ((deg/90)%4 + 4) % 4
	if turns == 0 {
		return args[0], nil
	}
	src := m.img
	w, h := src.Rect.Dx(), src.Rect.Dy()
	var out *image.NRGBA
	if turns == 2 {
		out = image.NewNRGBA(image.Rect(0, 0, w, h))
	} else {
		out = image.NewNRGBA(image.Rect(0, 0, h, w))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch turns {
			case 1: // clockwise
				dx, dy = h-1-y, x
			case 2:
				dx, dy = w-1-x, h-1-y
			case 3: // counter-clockwise
				dx, dy = y, w-1-x
			}
			s := src.PixOffset(x, y)
			d := out.PixOffset(dx, dy)
			copy(out.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return wrap(out, m.gray), nil
}

// flipFn implements `images.flip(img, "horizontal" | "vertical")`.
func flipFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.flip"
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (image, direction), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	dir, err := stringArg(fn, args[1], "direction")
	if err != nil {
		return interpreter.Null(), err
	}
	src := m.img
	w, h := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(src.Rect)
	switch dir {
	case "vertical":
		for y := 0; y < h; y++ {
			copy(out.Pix[y*out.Stride:(y+1)*out.Stride], src.Pix[(h-1-y)*src.Stride:(h-y)*src.Stride])
		}
	case "horizontal":
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s := src.PixOffset(w-1-x, y)
				d := out.PixOffset(x, y)
				copy(out.Pix[d:d+4], src.Pix[s:s+4])
			}
		}
	default:
		return interpreter.Null(), fmt.Errorf("%s: direction must be \"horizontal\" or \"vertical\", got %q", fn, dir)
	}
	return wrap(out, m.gray), nil
}

// --- tone -------------------------------------------------------------------

// luma is the Rec. 601 luminance Go's color.GrayModel uses, on 8-bit channels.
func luma(r, g, b uint8) uint8 {
	return uint8((19595*uint32(r) + 38470*uint32(g) + 7471*uint32(b) + 1<<15) >> 16)
}

// mapLuma returns a copy of src with every pixel's r, g and b replaced by
// f(luminance); alpha is kept.
func mapLuma(src *image.NRGBA, f func(uint8) uint8) *image.NRGBA {
	out := image.NewNRGBA(src.Rect)
	for i := 0; i < len(src.Pix); i += 4 {
		v := f(luma(src.Pix[i], src.Pix[i+1], src.Pix[i+2]))
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = v, v, v, src.Pix[i+3]
	}
	return out
}

// grayscaleFn implements `images.grayscale(img)`.
func grayscaleFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	m, err := oneImage("images.grayscale", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(mapLuma(m.img, func(v uint8) uint8 { return v }), true), nil
}

// thresholdFn implements `images.threshold(img, level)`: luminance at or
// above level becomes white, below it black.
func thresholdFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.threshold"
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (image, level), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	level, err := intArg(fn, args[1], "level")
	if err != nil {
		return interpreter.Null(), err
	}
	if level < 0 || level > 256 {
		return interpreter.Null(), fmt.Errorf("%s: level must be in 0..256, got %d", fn, level)
	}
	return wrap(mapLuma(m.img, func(v uint8) uint8 {
		if int(v) >= level {
			return 255
		}
		return 0
	}), true), nil
}

// ditherFn implements `images.dither(img)`: black and white by Floyd-Steinberg
// error diffusion over the luminance, so mid-tones survive as dot density.
func ditherFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	m, err := oneImage("images.dither", args)
	if err != nil {
		return interpreter.Null(), err
	}
	src := m.img
	w, h := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(src.Rect)
	// cur / next hold the luminance of this row and the error pushed into the
	// next one, padded by one cell each side so the kernel needs no bounds checks.
	cur := make([]int32, w+2)
	next := make([]int32, w+2)
	for x := 0; x < w; x++ {
		i := src.PixOffset(x, 0)
		cur[x+1] = int32(luma(src.Pix[i], src.Pix[i+1], src.Pix[i+2]))
	}
	for y := 0; y < h; y++ {
		for x := range next {
			next[x] = 0
		}
		if y+1 < h {
			for x := 0; x < w; x++ {
				i := src.PixOffset(x, y+1)
				next[x+1] = int32(luma(src.Pix[i], src.Pix[i+1], src.Pix[i+2]))
			}
		}
		for x := 1; x <= w; x++ {
			old := cur[x]
			var v int32
			if old >= 128 {
				v = 255
			}
			e := old - v
			cur[x+1] += e * 7 / 16
			next[x-1] += e * 3 / 16
			next[x] += e * 5 / 16
			next[x+1] += e / 16
			d := out.PixOffset(x-1, y)
			b := uint8(v)
			out.Pix[d], out.Pix[d+1], out.Pix[d+2], out.Pix[d+3] = b, b, b, src.Pix[d+3]
		}
		cur, next = next, cur
	}
	return wrap(out, true), nil
}

// --- compositing ------------------------------------------------------------

// over blends the non-premultiplied source pixel s onto d in place (Porter-Duff
// "source over destination").
func over(d []uint8, sr, sg, sb, sa uint8) {
	switch sa {
	case 255:
		d[0], d[1], d[2], d[3] = sr, sg, sb, 255
		return
	case 0:
		return
	}
	sa32, da32 := uint32(sa), uint32(d[3])
	// Every term is scaled by 255*255 so the blend stays in integers.
	oa := sa32*255 + da32*(255-sa32)
	if oa == 0 {
		d[0], d[1], d[2], d[3] = 0, 0, 0, 0
		return
	}
	blend := func(sc, dc uint8) uint8 {
		return uint8((uint32(sc)*sa32*255 + uint32(dc)*da32*(255-sa32) + oa/2) / oa)
	}
	d[0], d[1], d[2] = blend(sr, d[0]), blend(sg, d[1]), blend(sb, d[2])
	d[3] = uint8((oa + 127) / 255)
}

func clone(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Rect)
	copy(out.Pix, src.Pix)
	return out
}

// fillFn implements `images.fill(img, x, y, width, height, color)`: the
// rectangle (clipped to the image) painted over with color, alpha-blended.
func fillFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.fill"
	if len(args) != 6 {
		return interpreter.Null(), fmt.Errorf("%s expects 6 arguments (image, x, y, width, height, color), got %d", fn, len(args))
	}
	m, err := imageArg(fn, args[0], "image")
	if err != nil {
		return interpreter.Null(), err
	}
	r, err := rectArgs(fn, args[1:5])
	if err != nil {
		return interpreter.Null(), err
	}
	c, err := colorArg(fn, args[5])
	if err != nil {
		return interpreter.Null(), err
	}
	out := clone(m.img)
	r = r.Intersect(out.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := out.PixOffset(x, y)
			over(out.Pix[i:i+4], c.R, c.G, c.B, c.A)
		}
	}
	gray := m.gray && (r.Empty() || c.A == 0 || (c.R == c.G && c.G == c.B))
	return wrap(out, gray), nil
}

// drawFn implements `images.draw(dst, src, x, y)`: src composited over dst
// with its top left at (x, y), clipped to dst.
func drawFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "images.draw"
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("%s expects 4 arguments (dst, src, x, y), got %d", fn, len(args))
	}
	dst, err := imageArg(fn, args[0], "dst")
	if err != nil {
		return interpreter.Null(), err
	}
	src, err := imageArg(fn, args[1], "src")
	if err != nil {
		return interpreter.Null(), err
	}
	ox, err := intArg(fn, args[2], "x")
	if err != nil {
		return interpreter.Null(), err
	}
	oy, err := intArg(fn, args[3], "y")
	if err != nil {
		return interpreter.Null(), err
	}
	out := clone(dst.img)
	r := src.img.Rect.Add(image.Pt(ox, oy)).Intersect(out.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			s := src.img.PixOffset(x-ox, y-oy)
			d := out.PixOffset(x, y)
			p := src.img.Pix[s : s+4]
			over(out.Pix[d:d+4], p[0], p[1], p[2], p[3])
		}
	}
	return wrap(out, dst.gray && (src.gray || r.Empty())), nil
}
