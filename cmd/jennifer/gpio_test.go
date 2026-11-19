// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A .j program drives the gpio module against a mock sysfs tree (a temp dir
// pointed at by JENNIFER_GPIO_BASE) holding a Raspberry Pi 4 header chip at
// base 512, as Linux 6.2+ numbers it: setup exports global 529 for pin 17 and
// writes its direction, write / read round-trip the value, and release writes
// unexport - the module's acceptance criteria, verified black-box through a
// normal `import`. A mismatch throws and fails loadForTest.
func TestGpioSysfsMock(t *testing.T) {
	gpioMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "gpio.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("JENNIFER_GPIO_BASE", dir)
	t.Setenv("JENNIFER_GPIO_CHIP_LABEL", "")
	chip := filepath.Join(dir, "gpiochip512")
	if err := os.Mkdir(chip, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{"base": "512\n", "label": "pinctrl-bcm2711\n", "ngpio": "58\n"} {
		if err := os.WriteFile(filepath.Join(chip, name), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prog := fmt.Sprintf(`use testing;
use fs;
use strings;
import %q as gpio;

gpio.setup(17, "out");
testing.assertEqual(strings.trim(fs.readString(%q + "/export")), "529");
testing.assertEqual(strings.trim(fs.readString(%q + "/gpio529/direction")), "out");

gpio.write(17, 1);
testing.assertEqual(gpio.read(17), 1);
gpio.write(17, 0);
testing.assertEqual(gpio.read(17), 0);

gpio.release(17);
testing.assertEqual(strings.trim(fs.readString(%q + "/unexport")), "529");`,
		gpioMod, dir, dir, dir)

	progPath := filepath.Join(t.TempDir(), "blink.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("gpio program failed with code %d", code)
	}
}
