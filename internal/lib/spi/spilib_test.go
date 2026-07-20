// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

package spilib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	spilib "jennifer-lang.dev/jennifer/internal/lib/spi"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func run(t *testing.T, src string) error {
	t.Helper()
	spilib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return err
	}
	in := interpreter.New()
	in.Out = &bytes.Buffer{}
	iolib.Install(in)
	spilib.Install(in)
	return in.Run(prog)
}

func TestSpiErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"missing device", `use spi; def d as spi.Device init spi.open("/dev/spidev-nope");`, ""},
		{"wrong handle arg", `use spi; def b as bytes init spi.transfer(5, convert.bytesFromString("x","utf-8"));`, ""},
		{"transfer on closed", `use spi; use convert; def d as spi.Device init spi.open("/dev/spidev-nope");`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := run(t, c.src); err == nil {
				t.Fatalf("expected an error")
			} else if c.want != "" && !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q lacks %q", err.Error(), c.want)
			}
		})
	}
}
