// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("serial", `
open(string, int) serial.Port
openWith(string, serial.Options) serial.Port
read(serial.Port, int) bytes
write(serial.Port, bytes) int
flush(serial.Port) null
close(serial.Port) null
`)
}
