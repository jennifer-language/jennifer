// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("maps", `
keys(map of any to any) list of any
values(map of any to any) list of any
has(map of any to any, any) bool
delete(map of any to any, any) map of any to any
merge(map of any to any, map of any to any) map of any to any
`)
}
