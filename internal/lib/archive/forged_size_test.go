// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package archivelib

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"hash/crc32"
	"runtime"
	"testing"
)

// forgedArchives returns tiny archives whose one member declares 200 MiB but
// carries five bytes.
func forgedArchives(t *testing.T) map[string][]byte {
	t.Helper()
	const declared = 200 << 20
	out := map[string][]byte{}

	var tb bytes.Buffer
	if err := tar.NewWriter(&tb).WriteHeader(&tar.Header{Name: "big", Mode: 0o644, Size: declared, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tb.WriteString("hello")
	out["tar"] = append([]byte(nil), tb.Bytes()...)
	gz, err := gzipBytes(tb.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	out["tar.gz"] = gz

	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name: "big", Method: zip.Store, CRC32: crc32.ChecksumIEEE([]byte("hello")),
		CompressedSize64: 5, UncompressedSize64: declared,
	})
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out["zip"] = zb.Bytes()
	return out
}

// TestForgedMemberSizeDoesNotPreallocate: the buffer for a member is sized from
// its declared size only as far as the input could expand to, so a few hundred
// bytes declaring a 200 MiB member fail without allocating 200 MiB first.
func TestForgedMemberSizeDoesNotPreallocate(t *testing.T) {
	for format, b := range forgedArchives(t) {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err := unpackBytes(b, format, defaultUnpackCaps())
		runtime.ReadMemStats(&after)
		if err == nil {
			t.Errorf("%s: a truncated member unpacked", format)
		}
		if got := after.TotalAlloc - before.TotalAlloc; got > 8<<20 {
			t.Errorf("%s: unpack of %d input bytes allocated %d bytes", format, len(b), got)
		}
	}
}
