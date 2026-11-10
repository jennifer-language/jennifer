// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package compresslib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestUnpackMaxInt64Cap: maxBytes of MaxInt64 reads the whole stream.
// limit+1 used to wrap to MinInt64 and unpack returned empty bytes.
func TestUnpackMaxInt64Cap(t *testing.T) {
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		p := pack(t, str(algo), b("compress me, please"))
		got := unpack(t, str(algo), p, interpreter.IntVal(math.MaxInt64))
		if string(got.Bytes) != "compress me, please" {
			t.Fatalf("%s: got %q", algo, got.Bytes)
		}
	}
}
