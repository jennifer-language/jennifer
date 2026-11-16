// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build race

package compresslib

// raceEnabled reports whether the race detector is compiled in. The race
// detector makes sync.Pool drop a share of its Puts at random, so the
// allocation guard of TestUnpackReusesDecompressors skips under -race.
const raceEnabled = true
