// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build linux && !tinygo

package i2clib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	i2clib "jennifer-lang.dev/jennifer/internal/lib/i2c"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func run(t *testing.T, src string) error {
	t.Helper()
	i2clib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return err
	}
	in := interpreter.New()
	in.Out = &bytes.Buffer{}
	iolib.Install(in)
	i2clib.Install(in)
	return in.Run(prog)
}

func TestI2cErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"missing device", `use i2c; def b as i2c.Bus init i2c.open("/dev/i2c-does-not-exist", 80);`, ""},
		{"addr out of range", `use i2c; def b as i2c.Bus init i2c.open("/dev/i2c-1", 200);`, "7-bit"},
		{"negative addr", `use i2c; def b as i2c.Bus init i2c.open("/dev/i2c-1", 0 - 1);`, "7-bit"},
		{"wrong handle arg", `use i2c; def n as int init i2c.read(5, 1);`, "i2c.Bus"},
		{"bad register", `use i2c; def b as i2c.Bus init i2c.open("/dev/i2c-1", 80); def x as bytes init i2c.readReg($b, 999, 1);`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := run(t, c.src)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if c.want != "" && !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q lacks %q", err.Error(), c.want)
			}
		})
	}
}
