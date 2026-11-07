// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package archivelib

import (
	"runtime"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestUnpackAllocatesAboutTheMemberSize: a large member is read into a buffer
// sized from its header, and tar.gz streams the gzip output into the tar reader,
// so unpack allocates about the member size - not the 3-4x that io.ReadAll
// growth plus a materialised tar stream cost.
func TestUnpackAllocatesAboutTheMemberSize(t *testing.T) {
	const size = 16 << 20
	for _, format := range []string{"tar", "tar.gz", "zip"} {
		packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{
			entriesVal([]entry{{name: "big", data: make([]byte, size), mode: 0o644, mtime: 1700000000}}),
			str(format),
		})
		if err != nil {
			t.Fatalf("%s pack: %v", format, err)
		}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		es, err := unpackBytes(packed.Bytes, format, defaultUnpackCaps())
		runtime.ReadMemStats(&after)
		if err != nil || len(es) != 1 || len(es[0].data) != size {
			t.Fatalf("%s unpack: %v", format, err)
		}
		if got := after.TotalAlloc - before.TotalAlloc; got > size*3/2 {
			t.Errorf("%s: unpack allocated %d bytes for a %d-byte member, want under 1.5x", format, got, size)
		}
	}
}
