// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("spi", `
open(string) spi.Device
configure(spi.Device, int, int) null
transfer(spi.Device, bytes) bytes
close(spi.Device) null
`)
}
