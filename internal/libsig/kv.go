// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("kv", `
open() kv.Store
openFile(string) kv.Store
set(kv.Store, string, string, int) null
add(kv.Store, string, string, int) bool
get(kv.Store, string) string
has(kv.Store, string) bool
delete(kv.Store, string) bool
touch(kv.Store, string, int) bool
incr(kv.Store, string, int) int
# value: any opaque library value (json.Value, toml.Value, ...)
setValue(kv.Store, string, any, int) null
getValue(kv.Store, string) any
close(kv.Store) null
`)
}
