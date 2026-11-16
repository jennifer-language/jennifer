// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build race

package regexlib

// raceEnabled reports whether the race detector is compiled in. The race
// detector makes sync.Pool drop a share of its Puts at random, and regexp
// keeps its matching machines in one, so a run of many matches allocates a
// machine for most of them (over 1 GB in TestReplaceOutputCap). The
// allocation bounds of TestFindAllMatchCap and TestReplaceOutputCap are not
// checked under -race; their error checks still run.
const raceEnabled = true
