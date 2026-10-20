// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("testing", `
run(string) testing.Result
runWith(string, list of any) testing.Result
results() list of testing.Result
reset() null
report(list of testing.Result, string) string
assertEqual(any, any) null
assertNotEqual(any, any) null
assertTrue(bool) null
assertFalse(bool) null
assertContains(string, string) null
assertContains(list of any|map of any to any, any) null
assertThrows(string, string) null
assertThrowsWith(string, list of any, string) null
`)
}
