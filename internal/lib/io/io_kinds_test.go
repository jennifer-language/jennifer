// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package iolib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: stdin text that is not valid UTF-8 is kind "io"; end of input and
// misuse stay "runtime" ("").
func TestErrorKinds(t *testing.T) {
	in := interpreter.New()
	Install(in)
	readChars := in.LookupNamespacedBuiltin("io", "readChars")
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"readLine", func() error { _, err := callReadLine(in, stdinCtx(strings.NewReader("a\xff\n")), nil); return err }, "io"},
		{"readLines", func() error { _, err := callReadLines(in, stdinCtx(strings.NewReader("ok\n\xc3"))); return err }, "io"},
		{"readChars", func() error {
			_, err := readChars(stdinCtx(strings.NewReader("\xff")), []interpreter.Value{interpreter.IntVal(1)})
			return err
		}, "io"},
		{"end of input", func() error { _, err := callReadLine(in, stdinCtx(strings.NewReader("")), nil); return err }, ""},
		{"readChars count", func() error {
			_, err := readChars(stdinCtx(strings.NewReader("x")), []interpreter.Value{interpreter.StringVal("1")})
			return err
		}, ""},
	}
	for _, c := range cases {
		resetInputForTest()
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
