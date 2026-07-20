// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

package iiclib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iiclib "jennifer-lang.dev/jennifer/internal/lib/iic"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func run(t *testing.T, src string) error {
	t.Helper()
	iiclib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return err
	}
	in := interpreter.New()
	in.Out = &bytes.Buffer{}
	iolib.Install(in)
	iiclib.Install(in)
	return in.Run(prog)
}

func TestIicErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"missing device", `use iic; def b as iic.Bus init iic.open("/dev/i2c-does-not-exist", 80);`, ""},
		{"addr out of range", `use iic; def b as iic.Bus init iic.open("/dev/i2c-1", 200);`, "7-bit"},
		{"negative addr", `use iic; def b as iic.Bus init iic.open("/dev/i2c-1", 0 - 1);`, "7-bit"},
		{"wrong handle arg", `use iic; def n as int init iic.read(5, 1);`, "iic.Bus"},
		{"bad register", `use iic; def b as iic.Bus init iic.open("/dev/i2c-1", 80); def x as bytes init iic.readReg($b, 999, 1);`, ""},
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
