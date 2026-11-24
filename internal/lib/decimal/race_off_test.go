// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !race

package decimallib

// raceScale widens the speed tests' wall-clock budgets under the race
// detector; without it the budgets stand.
const raceScale = 1
