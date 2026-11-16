// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !race

package regexlib

// raceEnabled reports whether the race detector is compiled in. See the
// //go:build race counterpart for why the allocation bounds consult it.
const raceEnabled = false
