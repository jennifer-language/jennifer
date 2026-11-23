// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package archivelib

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io/fs"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// kindOf is the kind a catch sees: an *ErrorSignal's kind field, else the
// kind the error chain names ("" is "runtime").
func kindOf(err error) string {
	if _, ok := err.(*interpreter.ErrorSignal); ok {
		kind, _, _, _, _ := interpreter.ClassifyError(err)
		return kind
	}
	return dataerr.KindOf(err)
}

// M29.21: an archive unpack cannot read is kind "archive", a cap stays
// "limit", and misuse (wrong arguments, an unknown format, an entry pack
// refuses) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	ok := []entry{{name: "a.txt", data: bytes.Repeat([]byte("jennifer "), 100)}}
	packed := func(format string) []byte {
		v, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(format), entriesVal(ok)})
		if err != nil {
			t.Fatal(err)
		}
		return v.Bytes
	}
	tarB, zipB, tgzB := packed("tar"), packed("zip"), packed("tar.gz")
	unpack := func(format string, b []byte) func() error {
		return func() error {
			_, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(format), interpreter.BytesVal(b)})
			return err
		}
	}
	unpackWith := func(format string, b []byte, opts interpreter.Value) func() error {
		return func() error {
			_, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(format), interpreter.BytesVal(b), opts})
			return err
		}
	}
	pack := func(format string, es []entry) func() error {
		return func() error {
			_, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{str(format), entriesVal(es)})
			return err
		}
	}
	symlinkTar := tarWith(t, []member{{tar.Header{Name: "l", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, ""}})
	slipTar := tarWith(t, []member{{tar.Header{Name: "../evil", Mode: 0o644, Typeflag: tar.TypeReg}, "x"}})
	danglingLink := tarWith(t, []member{{tar.Header{Name: "a", Typeflag: tar.TypeLink, Linkname: "missing"}, ""}})
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	h := &zip.FileHeader{Name: "l"}
	h.SetMode(fs.ModeSymlink | 0o777)
	w, _ := zw.CreateHeader(h)
	_, _ = w.Write([]byte("/etc/passwd"))
	_ = zw.Close()
	symlinkZip := zbuf.Bytes()
	flip := func(i int) []byte {
		q := append([]byte(nil), zipB...)
		q[i] ^= 0xff
		return q
	}
	// The member's deflate data starts at byte 44 (30-byte header, "a.txt",
	// a 9-byte extended timestamp).
	corrupt, crc := flip(44), flip(48)
	opts := interpreter.NamespacedStructVal(LibraryName, "UnpackOptions", []interpreter.StructField{
		{Name: "maxEntries", Value: interpreter.IntVal(0)},
		{Name: "maxEntryBytes", Value: interpreter.IntVal(10)},
		{Name: "maxTotalBytes", Value: interpreter.IntVal(0)},
		{Name: "skipSpecial", Value: interpreter.BoolVal(false)},
	})

	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"zip not a zip", unpack("zip", []byte("not a zip file at all")), "archive"},
		{"zip truncated", unpack("zip", zipB[:len(zipB)/2]), "archive"},
		{"zip corrupt member", unpack("zip", corrupt), "archive"},
		{"zip checksum", unpack("zip", crc), "archive"},
		{"zip symlink", unpack("zip", symlinkZip), "archive"},
		{"tar truncated", unpack("tar", tarB[:700]), "archive"},
		{"tar bad header", unpack("tar", bytes.Repeat([]byte{'x'}, 1024)), "archive"},
		{"tar symlink", unpack("tar", symlinkTar), "archive"},
		{"tar zip-slip", unpack("tar", slipTar), "archive"},
		{"tar dangling hard link", unpack("tar", danglingLink), "archive"},
		{"tar.gz not gzip", unpack("tar.gz", tarB), "archive"},
		{"tar.gz truncated", unpack("tgz", tgzB[:len(tgzB)/2]), "archive"},
		{"unpackWith corrupt", unpackWith("zip", []byte("junk"), opts), "archive"},
		{"unpackWith entry cap", unpackWith("tar", tarB, opts), "limit"},
		{"unknown format", unpack("rar", tarB), ""},
		{"unpack not bytes", func() error {
			_, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("tar"), str("x")})
			return err
		}, ""},
		{"pack unsafe name", pack("tar", []entry{{name: "../x"}}), ""},
		{"pack bad mode", pack("zip", []entry{{name: "x", mode: 0o170000}}), ""},
		{"pack unknown format", pack("rar", ok), ""},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := kindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
