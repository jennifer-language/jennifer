// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("i2c", `
open(string, int) i2c.Bus
read(i2c.Bus, int) bytes
write(i2c.Bus, bytes) int
readReg(i2c.Bus, int, int) bytes
writeReg(i2c.Bus, int, bytes) int
close(i2c.Bus) null
`)
}
