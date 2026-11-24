// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import "testing"

// TestCountsCheckedBeforeNarrowing: a count past the 32-bit int range is
// rejected, not truncated. On a 32-bit target int(1<<32 + 1) is 1, which
// passed the range checks as a valid k before they compared the int64 (M29.6).
func TestCountsCheckedBeforeNarrowing(t *testing.T) {
	X := mat([]float64{1, 1}, []float64{2, 2}, []float64{8, 8}, []float64{9, 9})
	y := vec(0, 0, 1, 1)
	wrap := iv(1<<32 + 1)
	fails(t, kMeansFn, X, wrap)
	fails(t, kNNFn, X, y, wrap)
	fails(t, kNNRegressorFn, X, y, wrap)
	fails(t, pcaFn, X, wrap)
	fails(t, kFoldFn, iv(1<<32+4), iv(2))
}
