// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("task", `
wait(task of any) any
poll(task of any) bool
discard(task of any) null
waitAll(list of task of any) list of any
waitAny(list of task of any) int
cancel(task of any) null
cancelled() bool
waitTimeout(task of any, int) any
waitAnyTimeout(list of task of any, int) int
`)
}
