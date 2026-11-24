// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package archivelib

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// A tar.gz is read to its end, so its gzip trailer is checked and bytes after
// the gzip member are refused; a tar must carry its end-of-archive marker and
// only NUL padding after it.
func TestUnpackChecksTrailers(t *testing.T) {
	es := []entry{{name: "a.txt", data: bytes.Repeat([]byte("jennifer "), 100)}}
	tarB, err := packTar(es)
	if err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(tarB)
	zw.Close()
	tgz := gz.Bytes()
	with := func(b []byte, f func([]byte) []byte) []byte { return f(append([]byte(nil), b...)) }

	good := map[string][]byte{
		"tar":                      tarB,
		"tar.gz":                   tgz,
		"tar, record padding":      append(append([]byte(nil), tarB...), make([]byte, 9216)...),
		"tar.gz, two gzip members": append(append([]byte(nil), tgz...), gzipOf(t, make([]byte, 512))...),
	}
	for name, b := range good {
		format := "tar"
		if bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
			format = "tar.gz"
		}
		if _, err := unpackBytes(b, format, defaultUnpackCaps()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string][]byte{
		"tar.gz, CRC flipped":    with(tgz, func(b []byte) []byte { b[len(b)-8] ^= 0xff; return b }),
		"tar.gz, size flipped":   with(tgz, func(b []byte) []byte { b[len(b)-1] ^= 0xff; return b }),
		"tar.gz, no trailer":     tgz[:len(tgz)-8],
		"tar.gz, junk after":     append(append([]byte(nil), tgz...), bytes.Repeat([]byte("junk"), 5)...),
		"tar.gz, data after tar": gzipOf(t, append(append([]byte(nil), tarB...), 'x')),
		"tar, junk after marker": append(append([]byte(nil), tarB...), bytes.Repeat([]byte("junk"), 5)...),
		"tar, no marker":         tarB[:len(tarB)-1024],
		"tar, one zero block":    tarB[:len(tarB)-512],
	}
	for name, b := range bad {
		format := "tar"
		if bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
			format = "tar.gz"
		}
		_, err := unpackBytes(b, format, defaultUnpackCaps())
		if kindOf(err) != dataKind {
			t.Errorf("%s: %v (kind %q), want %q", name, err, kindOf(err), dataKind)
		}
	}
}

func gzipOf(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
