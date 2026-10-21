// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("channel", `
make(int) channel of any
send(channel of any, any) null
recv(channel of any) any
close(channel of any) null
select(list of channel of any) any
len(channel of any) int
capacity(channel of any) int
`)
}
