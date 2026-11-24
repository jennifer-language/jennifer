// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"image/color"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: bytes decode / info cannot read as an image are kind "images", the
// pixel cap stays "limit", and misuse (wrong arguments, out-of-range sizes,
// an unknown format, filter or color string) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	m := call(t, createFn, num(16), num(16), col(color.NRGBA{10, 20, 30, 255}))
	pngData := call(t, encodeFn, str("png"), m).Bytes()
	jpgData := call(t, encodeFn, str("jpeg"), m).Bytes()
	gifData := call(t, encodeFn, str("gif"), m).Bytes()
	run := func(fn interpreter.Builtin, args ...Value) func() error {
		return func() error { _, err := fn(interpreter.BuiltinCtx{}, args); return err }
	}
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"decode not an image", run(decodeFn, raw([]byte("not an image"))), "images"},
		{"decode empty", run(decodeFn, raw(nil)), "images"},
		{"decode png header only", run(decodeFn, raw(pngData[:8])), "images"},
		{"decode png truncated pixels", run(decodeFn, raw(pngData[:len(pngData)-20])), "images"},
		{"decode jpeg truncated", run(decodeFn, raw(jpgData[:len(jpgData)/2])), "images"},
		{"decode gif truncated", run(decodeFn, raw(gifData[:len(gifData)-10])), "images"},
		{"info not an image", run(infoFn, raw([]byte("GIF8"))), "images"},
		{"info png truncated", run(infoFn, raw(pngData[:12])), "images"},
		{"decode maxPixels below 1", run(decodeFn, raw(pngData), num(0)), ""},
		{"decode not bytes", run(decodeFn, str("x")), ""},
		{"encode unknown format", run(encodeFn, str("bmp"), m), ""},
		{"hex bad color", run(hexFn, str("red")), ""},
		{"crop outside", run(cropFn, m, num(10), num(10), num(10), num(10)), ""},
		{"fromPixels short", run(fromPixelsFn, num(2), num(2), raw([]byte{1})), ""},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}

	// The pixel cap is raised as kind "limit" directly.
	_, err := decodeFn(interpreter.BuiltinCtx{}, []Value{raw(pngData), num(10)})
	if k := errorKind(t, err); k != "limit" {
		t.Errorf("over maxPixels: kind %q, want limit", k)
	}
}
