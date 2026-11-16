// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build race

package netlib_test

// raceEnabled reports whether the race detector is compiled in. The race
// detector makes sync.Pool drop a share of its Puts at random, so the
// allocation guard of TestRecvFromReusesBuffer skips under -race.
const raceEnabled = true
