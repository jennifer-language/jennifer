// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build linux && !tinygo

package gpiolib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	gpiolib "jennifer-lang.dev/jennifer/internal/lib/gpio"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func run(t *testing.T, src string) error {
	t.Helper()
	gpiolib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return err
	}
	in := interpreter.New()
	in.Out = &bytes.Buffer{}
	iolib.Install(in)
	gpiolib.Install(in)
	return in.Run(prog)
}

func TestGpioErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bad direction", `use gpio; gpio.setup(17, "sideways");`, "gpio.IN"},
		{"pin out of range", `use gpio; gpio.setup(999, gpio.OUT);`, "out of range"},
		{"read not set up", `use gpio; def v as int init gpio.read(17);`, "not set up"},
		{"write not set up", `use gpio; gpio.write(17, 1);`, "not set up"},
		{"release not set up", `use gpio; gpio.release(17);`, "not set up"},
		{"bad value", `use gpio; gpio.write(17, 5);`, ""},
		{"missing chip", `use gpio; gpio.chip("/dev/gpiochip-nope"); gpio.setup(17, gpio.OUT);`, ""},
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

// gpio.IN / gpio.OUT constants match the sysfs module ("in" / "out").
func TestGpioDirectionConstants(t *testing.T) {
	if err := run(t, `use gpio; use io; io.printf("%s %s", gpio.IN, gpio.OUT);`); err != nil {
		t.Fatalf("run: %v", err)
	}
}
