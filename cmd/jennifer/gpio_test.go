// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A .j program drives the gpio module against a mock sysfs tree (a temp dir
// pointed at by JENNIFER_GPIO_BASE): setup writes export + direction, write /
// read round-trip a pin's value, and release writes unexport - the module's
// acceptance criteria, verified black-box through a normal `import`. A mismatch
// throws and fails loadForTest.
func TestGpioSysfsMock(t *testing.T) {
	gpioMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "gpio.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("JENNIFER_GPIO_BASE", dir)

	prog := fmt.Sprintf(`use testing;
use fs;
use strings;
import %q as gpio;

gpio.setup(17, "out");
testing.assertEqual(strings.trim(fs.readString(%q + "/export")), "17");
testing.assertEqual(strings.trim(fs.readString(%q + "/gpio17/direction")), "out");

gpio.write(17, 1);
testing.assertEqual(gpio.read(17), 1);
gpio.write(17, 0);
testing.assertEqual(gpio.read(17), 0);

gpio.release(17);
testing.assertEqual(strings.trim(fs.readString(%q + "/unexport")), "17");`,
		gpioMod, dir, dir, dir)

	progPath := filepath.Join(t.TempDir(), "blink.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("gpio program failed with code %d", code)
	}
}
