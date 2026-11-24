// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("spi", `
open(string) spi.Device
openWith(string, spi.Options) spi.Device
configure(spi.Device, int, int) null
transfer(spi.Device, bytes) bytes
close(spi.Device) null
`)
	// M29.49: the configuration is set at open.
	deprecate("spi", "configure", "`spi.openWith(path, spi.Options{mode: ..., speedHz: ..., bitsPerWord: ...})`", "0.29.0")
}
