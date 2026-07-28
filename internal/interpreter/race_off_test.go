// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !race

package interpreter_test

// raceEnabled reports whether the race detector is compiled in. See the //go:build
// race counterpart for why the allocation guard consults it.
const raceEnabled = false
