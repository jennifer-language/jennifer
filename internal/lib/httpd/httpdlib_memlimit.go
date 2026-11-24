// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

// Sizing the listenWith memory guard from the machine memory limit
// (internal/memlimit) for httpd.setMaxBufferBudgetFromRAM, an opt-in helper.

package httpdlib

// budgetFromLimit applies the caller's fraction to the detected limit, floors
// the result at one default body so the guard can never reject every listenWith,
// and caps it at the sanity ceiling (an absurd /proc or cgroup value can't push
// the budget past what setMaxBufferBudget itself would accept).
func budgetFromLimit(limit int64, fraction float64) int64 {
	b := int64(float64(limit) * fraction)
	if b < minBufferBudget {
		b = minBufferBudget
	}
	if b > maxBufferBudget {
		b = maxBufferBudget
	}
	return b
}
