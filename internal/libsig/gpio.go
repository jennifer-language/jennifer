// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("gpio", `
open(string) gpio.Chip
close(gpio.Chip) null
setup(gpio.Chip, int, string) null
read(gpio.Chip, int) int
write(gpio.Chip, int, int) null
release(gpio.Chip, int) null
# Deprecated (M29.49): the pin-only forms on the process-wide chip; lint
# reports them (L301).
setup(int, string) null
read(int) int
write(int, int) null
release(int) null
chip(string) null
`)
	deprecate("gpio", "chip", "`gpio.open(path)`, passing the `gpio.Chip` to `setup` / `read` / `write` / `release`", "0.29.0")
}
