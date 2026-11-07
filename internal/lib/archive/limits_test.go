// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package archivelib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestUnpackWithMaxInt64Caps: caps of MaxInt64 mean "no practical limit".
// limit+1 used to wrap to MinInt64 and every member came back empty (tar,
// zip) or the tar.gz stream read as zero entries.
func TestUnpackWithMaxInt64Caps(t *testing.T) {
	opts := func(entryBytes int64) interpreter.Value {
		return interpreter.NamespacedStructVal("archive", "UnpackOptions", []interpreter.StructField{
			{Name: "maxTotalBytes", Value: interpreter.IntVal(math.MaxInt64)},
			{Name: "maxEntryBytes", Value: interpreter.IntVal(entryBytes)},
			{Name: "maxEntries", Value: interpreter.IntVal(0)},
			{Name: "skipSpecial", Value: interpreter.BoolVal(false)},
		})
	}
	es := []entry{{name: "a.txt", data: []byte("hello world"), mtime: 1700000000}}
	for _, format := range []string{"tar", "zip", "tar.gz"} {
		packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal(es), str(format)})
		if err != nil {
			t.Fatalf("%s pack: %v", format, err)
		}
		for _, eb := range []int64{0, math.MaxInt64} {
			v, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str(format), opts(eb)})
			if err != nil {
				t.Fatalf("%s/%d: %v", format, eb, err)
			}
			if len(v.List) != 1 {
				t.Fatalf("%s/%d: %d entries, want 1", format, eb, len(v.List))
			}
			for _, f := range v.List[0].Fields {
				if f.Name == "data" && string(f.Value.Bytes) != "hello world" {
					t.Fatalf("%s/%d: data %q", format, eb, f.Value.Bytes)
				}
			}
		}
	}
}
