// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func num(n int) Value         { return interpreter.IntVal(int64(n)) }
func str(s string) Value      { return interpreter.StringVal(s) }
func raw(b []byte) Value      { return interpreter.BytesVal(b) }
func col(c color.NRGBA) Value { return colorVal(c) }

// call runs a builtin the way the interpreter would and fails on an error.
func call(t *testing.T, fn interpreter.Builtin, args ...Value) Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("call%v: %v", args, err)
	}
	return v
}

func fails(t *testing.T, fn interpreter.Builtin, want string, args ...Value) {
	t.Helper()
	_, err := fn(interpreter.BuiltinCtx{}, args)
	if err == nil {
		t.Fatalf("call%v: want an error containing %q", args, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("call%v: error %q does not contain %q", args, err, want)
	}
}

func payload(t *testing.T, v Value) *imageValue {
	t.Helper()
	m, err := imageArg("test", v, "value")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func at(t *testing.T, v Value, x, y int) color.NRGBA {
	t.Helper()
	return payload(t, v).img.NRGBAAt(x, y)
}

func size(t *testing.T, v Value) (int, int) {
	t.Helper()
	r := payload(t, v).img.Rect
	return r.Dx(), r.Dy()
}

// checker builds a w x h black-and-white checkerboard of 1-pixel squares.
func checker(w, h int) Value {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				m.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
			} else {
				m.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
			}
		}
	}
	return wrap(m, true)
}

func errorKind(t *testing.T, err error) string {
	t.Helper()
	sig, ok := err.(*interpreter.ErrorSignal)
	if !ok {
		t.Fatalf("want *ErrorSignal, got %T %v", err, err)
	}
	for _, f := range sig.Value.Fields {
		if f.Name == "kind" {
			return f.Value.Str
		}
	}
	return ""
}

var (
	red   = color.NRGBA{255, 0, 0, 255}
	blue  = color.NRGBA{0, 0, 255, 255}
	white = color.NRGBA{255, 255, 255, 255}
	black = color.NRGBA{0, 0, 0, 255}
)

func TestCreatePixelAndColors(t *testing.T) {
	m := call(t, createFn, num(3), num(2), col(red))
	if w, h := size(t, m); w != 3 || h != 2 {
		t.Fatalf("size %dx%d", w, h)
	}
	if got := at(t, m, 2, 1); got != red {
		t.Errorf("pixel = %v", got)
	}
	if got := at(t, call(t, createFn, num(1), num(1)), 0, 0); got != (color.NRGBA{}) {
		t.Errorf("default fill should be transparent, got %v", got)
	}
	c := call(t, pixelFn, m, num(0), num(0))
	if c.StructName != "Color" || c.Fields[0].Value.Int != 255 || c.Fields[3].Value.Int != 255 {
		t.Errorf("pixel returned %v", c.Display())
	}
	fails(t, pixelFn, "outside the 3x2 image", m, num(3), num(0))
	fails(t, createFn, "at least 1", num(0), num(5))
	_, err := createFn(interpreter.BuiltinCtx{}, []Value{num(1 << 14), num(1 << 14)})
	if k := errorKind(t, err); k != "limit" {
		t.Errorf("oversized create: kind %q", k)
	}
	for in, want := range map[string]color.NRGBA{
		"#f80":      {255, 136, 0, 255},
		"#FF8800":   {255, 136, 0, 255},
		"#11223344": {0x11, 0x22, 0x33, 0x44},
	} {
		got, err := colorArg("t", call(t, hexFn, str(in)))
		if err != nil || got != want {
			t.Errorf("hex(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"f80", "#ff88", "#gg0000", ""} {
		fails(t, hexFn, "is not a", str(bad))
	}
	fails(t, rgbFn, "in 0..255", num(256), num(0), num(0))
	if got, _ := colorArg("t", call(t, rgbaFn, num(1), num(2), num(3), num(4))); got != (color.NRGBA{1, 2, 3, 4}) {
		t.Errorf("rgba = %v", got)
	}
	bad := interpreter.NamespacedStructVal(LibraryName, "Color", []interpreter.StructField{{Name: "r", Value: num(300)}})
	fails(t, createFn, "0..255", num(1), num(1), bad)
}

func TestCodecsRoundTrip(t *testing.T) {
	m := call(t, createFn, num(16), num(8), col(color.NRGBA{10, 20, 30, 128}))
	pngData := call(t, encodeFn, str("png"), m)
	back := call(t, decodeFn, pngData)
	if !payload(t, back).EqualNative(payload(t, m)) {
		t.Error("png should round-trip exactly, alpha included")
	}
	info := call(t, infoFn, pngData)
	if info.Fields[0].Value.Str != "png" || info.Fields[1].Value.Int != 16 || info.Fields[2].Value.Int != 8 {
		t.Errorf("info = %v", info.Display())
	}

	photo := call(t, createFn, num(32), num(32), col(color.NRGBA{200, 100, 50, 255}))
	jpg := call(t, encodeFn, str("jpeg"), photo, num(95))
	if got := at(t, call(t, decodeFn, jpg), 16, 16); diff(got, color.NRGBA{200, 100, 50, 255}) > 6 {
		t.Errorf("jpeg pixel drifted to %v", got)
	}
	gifData := call(t, encodeFn, str("gif"), call(t, createFn, num(4), num(4), col(white)))
	if got := at(t, call(t, decodeFn, gifData), 1, 1); got != white {
		t.Errorf("gif pixel = %v", got)
	}
	fails(t, encodeFn, "unknown format", str("bmp"), m)
	fails(t, encodeFn, "only to \"jpeg\"", str("png"), m, num(50))
	fails(t, encodeFn, "1..100", str("jpeg"), m, num(0))
	fails(t, decodeFn, "images.decode", raw([]byte("not an image")))
}

func diff(a, b color.NRGBA) int {
	d := 0
	for _, p := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
		if p[0] > p[1] {
			d = max(d, int(p[0]-p[1]))
		} else {
			d = max(d, int(p[1]-p[0]))
		}
	}
	return d
}

// A grayscale image is written as a grayscale PNG, and decoding one sets the
// gray flag again.
func TestGrayPNG(t *testing.T) {
	g := call(t, grayscaleFn, call(t, createFn, num(8), num(8), col(red)))
	data := call(t, encodeFn, str("png"), g).Bytes
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded.(*image.Gray); !ok {
		t.Errorf("gray image encoded as %T", decoded)
	}
	if !payload(t, call(t, decodeFn, raw(data))).gray {
		t.Error("decoding a gray PNG should keep the gray flag")
	}
	if got := at(t, g, 0, 0).R; got != luma(255, 0, 0) {
		t.Errorf("luma of red = %d", got)
	}
}

// decode checks the header against the pixel cap before decoding, so the
// limit holds however small the file is.
func TestDecodeLimit(t *testing.T) {
	data := call(t, encodeFn, str("png"), call(t, createFn, num(100), num(100)))
	_, err := decodeFn(interpreter.BuiltinCtx{}, []Value{data, num(9999)})
	if k := errorKind(t, err); k != "limit" {
		t.Errorf("kind = %q", k)
	}
	if !strings.Contains(err.Error(), "9999-pixel limit") && !strings.Contains(errMessage(err), "9999-pixel limit") {
		t.Errorf("message should name the limit: %v", errMessage(err))
	}
	call(t, decodeFn, data, num(10000))
	fails(t, decodeFn, "at least 1", data, num(0))
}

func errMessage(err error) string {
	if sig, ok := err.(*interpreter.ErrorSignal); ok {
		for _, f := range sig.Value.Fields {
			if f.Name == "message" {
				return f.Value.Str
			}
		}
	}
	return err.Error()
}

func TestPixelsRoundTrip(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	m := call(t, fromPixelsFn, num(2), num(1), raw(data))
	if got := call(t, pixelsFn, m).Bytes; !bytes.Equal(got, data) {
		t.Errorf("pixels = %v", got)
	}
	// pixels hands out a copy: editing it leaves the image alone.
	out := call(t, pixelsFn, m)
	out.Bytes[0] = 99
	if at(t, m, 0, 0).R != 1 {
		t.Error("pixels must not alias the image")
	}
	fails(t, fromPixelsFn, "needs 8 bytes", num(2), num(1), raw(data[:7]))
}

func TestResize(t *testing.T) {
	src := call(t, createFn, num(40), num(20), col(blue))
	for _, f := range []string{"nearest", "bilinear", "catmullrom"} {
		for _, wh := range [][2]int{{10, 5}, {97, 31}, {1, 1}} {
			out := call(t, resizeFn, src, num(wh[0]), num(wh[1]), str(f))
			if w, h := size(t, out); w != wh[0] || h != wh[1] {
				t.Fatalf("%s: size %dx%d", f, w, h)
			}
			if got := at(t, out, wh[0]-1, wh[1]-1); got != blue {
				t.Errorf("%s %v: a uniform image must stay uniform, got %v", f, wh, got)
			}
		}
	}
	// 0 keeps the aspect ratio.
	if w, h := size(t, call(t, resizeFn, src, num(10), num(0))); w != 10 || h != 5 {
		t.Errorf("aspect from width: %dx%d", w, h)
	}
	if w, h := size(t, call(t, resizeFn, src, num(0), num(10))); w != 20 || h != 10 {
		t.Errorf("aspect from height: %dx%d", w, h)
	}
	fails(t, resizeFn, "only one may be 0", src, num(0), num(0))
	fails(t, resizeFn, "unknown filter", src, num(5), num(5), str("lanczos"))
}

// Shrinking a 1-pixel checkerboard by 8 averages it to mid-gray rather than
// aliasing to black or white: the widened kernel covers every source pixel.
func TestResizeDoesNotAlias(t *testing.T) {
	out := call(t, resizeFn, checker(64, 64), num(8), num(8), str("bilinear"))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if v := at(t, out, x, y).R; v < 118 || v > 137 {
				t.Fatalf("(%d,%d) = %d, want about 128", x, y, v)
			}
		}
	}
	if !payload(t, out).gray {
		t.Error("resizing a gray image keeps it gray")
	}
}

// Filtering is premultiplied: blending into a transparent (black) neighbour
// lowers alpha but leaves the color red, where straight filtering would darken
// it towards black.
func TestResizeNoTransparentBleed(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	m.SetNRGBA(0, 0, red)
	m.SetNRGBA(1, 0, red)
	out := call(t, resizeFn, wrap(m, false), num(2), num(1), str("bilinear"))
	for x := 0; x < 2; x++ {
		got := at(t, out, x, 0)
		if got.A == 0 || got.R != 255 || got.G != 0 || got.B != 0 {
			t.Errorf("x=%d: %v, want red at some alpha", x, got)
		}
	}
	if a0, a1 := at(t, out, 0, 0).A, at(t, out, 1, 0).A; a0 <= a1 {
		t.Errorf("alpha should fall towards the transparent side: %d, %d", a0, a1)
	}
}

func TestFit(t *testing.T) {
	src := call(t, createFn, num(400), num(300))
	if w, h := size(t, call(t, fitFn, src, num(100), num(100))); w != 100 || h != 75 {
		t.Errorf("fit landscape: %dx%d", w, h)
	}
	if w, h := size(t, call(t, fitFn, src, num(1000), num(150))); w != 200 || h != 150 {
		t.Errorf("fit by height: %dx%d", w, h)
	}
	if w, h := size(t, call(t, fitFn, src, num(800), num(800))); w != 400 || h != 300 {
		t.Errorf("fit must not enlarge: %dx%d", w, h)
	}
}

func TestCropRotateFlip(t *testing.T) {
	// 3x2: row 0 = 0 1 2, row 1 = 3 4 5 (value in the red channel).
	m := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		m.SetNRGBA(i%3, i/3, color.NRGBA{uint8(i), 0, 0, 255})
	}
	v := wrap(m, false)
	grid := func(img Value) [][]uint8 {
		w, h := size(t, img)
		out := make([][]uint8, h)
		for y := range out {
			for x := 0; x < w; x++ {
				out[y] = append(out[y], at(t, img, x, y).R)
			}
		}
		return out
	}
	check := func(name string, got [][]uint8, want [][]uint8) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: %v, want %v", name, got, want)
		}
		for i := range got {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("%s: %v, want %v", name, got, want)
			}
		}
	}
	check("rotate 90", grid(call(t, rotateFn, v, num(90))), [][]uint8{{3, 0}, {4, 1}, {5, 2}})
	check("rotate 180", grid(call(t, rotateFn, v, num(180))), [][]uint8{{5, 4, 3}, {2, 1, 0}})
	check("rotate -90", grid(call(t, rotateFn, v, num(-90))), [][]uint8{{2, 5}, {1, 4}, {0, 3}})
	check("rotate 360", grid(call(t, rotateFn, v, num(360))), [][]uint8{{0, 1, 2}, {3, 4, 5}})
	check("flip h", grid(call(t, flipFn, v, str("horizontal"))), [][]uint8{{2, 1, 0}, {5, 4, 3}})
	check("flip v", grid(call(t, flipFn, v, str("vertical"))), [][]uint8{{3, 4, 5}, {0, 1, 2}})
	check("crop", grid(call(t, cropFn, v, num(1), num(0), num(2), num(2))), [][]uint8{{1, 2}, {4, 5}})
	fails(t, rotateFn, "multiple of 90", v, num(45))
	fails(t, flipFn, "\"horizontal\" or \"vertical\"", v, str("diagonal"))
	fails(t, cropFn, "outside the 3x2 image", v, num(2), num(0), num(2), num(1))
	fails(t, cropFn, "at least 1", v, num(0), num(0), num(0), num(1))
}

func TestThresholdAndDither(t *testing.T) {
	gray := call(t, createFn, num(64), num(64), col(color.NRGBA{128, 128, 128, 255}))
	if got := at(t, call(t, thresholdFn, gray, num(128)), 0, 0); got != white {
		t.Errorf("threshold at the level = %v", got)
	}
	if got := at(t, call(t, thresholdFn, gray, num(129)), 0, 0); got != black {
		t.Errorf("threshold above = %v", got)
	}
	fails(t, thresholdFn, "0..256", gray, num(300))
	d := payload(t, call(t, ditherFn, gray)).img
	whites := 0
	for i := 0; i < len(d.Pix); i += 4 {
		switch d.Pix[i] {
		case 255:
			whites++
		case 0:
		default:
			t.Fatalf("dither left a %d pixel", d.Pix[i])
		}
	}
	if n := 64 * 64; whites < n*45/100 || whites > n*55/100 {
		t.Errorf("50%% gray dithered to %d of %d white", whites, n)
	}
}

func TestFillAndDraw(t *testing.T) {
	base := call(t, createFn, num(4), num(4), col(white))
	half := call(t, fillFn, base, num(1), num(1), num(10), num(10), col(color.NRGBA{0, 0, 0, 128}))
	if got := at(t, half, 0, 0); got != white {
		t.Errorf("outside the rect = %v", got)
	}
	if got := at(t, half, 3, 3); diff(got, color.NRGBA{127, 127, 127, 255}) > 1 {
		t.Errorf("50%% black over white = %v", got)
	}
	if got := at(t, base, 3, 3); got != white {
		t.Error("fill must not change its input")
	}
	stamp := call(t, createFn, num(2), num(2), col(red))
	out := call(t, drawFn, base, stamp, num(-1), num(3))
	if got := at(t, out, 0, 3); got != red {
		t.Errorf("clipped stamp = %v", got)
	}
	if got := at(t, out, 1, 3); got != white {
		t.Errorf("past the stamp = %v", got)
	}
	if payload(t, out).gray {
		t.Error("drawing red onto white is no longer gray")
	}
	fails(t, drawFn, "src must be an images.Image", base, num(1), num(0), num(0))
}

func TestDisplayAndEquality(t *testing.T) {
	a := call(t, createFn, num(5), num(3), col(red))
	b := call(t, createFn, num(5), num(3), col(red))
	if !a.Equal(b) {
		t.Error("equal pixels should compare equal")
	}
	if a.Equal(call(t, createFn, num(5), num(3), col(blue))) {
		t.Error("different pixels compare unequal")
	}
	if got := a.Display(); got != "<images.Image 5x3>" {
		t.Errorf("display = %q", got)
	}
}
