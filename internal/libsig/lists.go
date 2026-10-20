// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("lists", `
push(list of any, any) list of any
pop(list of any) list of any
first(list of any) any
last(list of any) any
head(list of any, int) list of any
tail(list of any, int) list of any
reverse(list of any) list of any
sort(list of any) list of any
contains(list of any, any) bool
indexOf(list of any, any) int
concat(list of any, list of any) list of any
slice(list of any, int, int?) list of any
shuffle(list of any) list of any
range(int, int, int?) list of int
map(list of any, func) list of any
filter(list of any, func) list of any
reduce(list of any, func, any) any
find(list of any, func) any
any(list of any, func) bool
all(list of any, func) bool
sortBy(list of any, func) list of any
`)
}
