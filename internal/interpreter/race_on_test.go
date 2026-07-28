// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build race

package interpreter_test

// raceEnabled reports whether the race detector is compiled in. Allocation-count
// assertions (TestFrameAllocationsStayLow) skip under -race because the detector
// allocates shadow state per memory access, which dwarfs the interpreter's own
// allocations and makes the count meaningless.
const raceEnabled = true
