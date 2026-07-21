// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package archivelib is the `archive` library: tar / zip container read and
// write over `bytes`, value-semantic (no `fs` dependency). It shares the
// `pack` / `unpack` verbs with `compress` - byte streams there, file bundles
// here - with the container format a string argument (`"tar"` / `"zip"` /
// `"tar.gz"`). A bundle is a `list of archive.Entry`, each a regular file.
// Backed by Go's archive/tar + archive/zip (TinyGo-clean).
package archivelib

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// maxDecompressed caps the TOTAL decompressed payload of one unpack call,
// summed across every entry, so a small "zip bomb" input cannot expand to
// gigabytes in memory - a per-entry cap alone would still let an archive
// with N entries expand to N times the cap. maxEntries bounds the member
// count for the same reason. Vars (not consts) so tests can lower them;
// fixed defaults (configurable later).
var (
	maxDecompressed int64 = 256 << 20
	maxEntries            = 65536
)

// readCapped reads r fully but errors once the shared unpack budget is
// exhausted, rather than allocating without bound. On success it deducts
// the bytes read from *budget, so the cap spans all entries of one call.
func readCapped(r io.Reader, budget *int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, *budget+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > *budget {
		return nil, fmt.Errorf("total decompressed size exceeds the %d-byte limit", maxDecompressed)
	}
	*budget -= int64(len(data))
	return data, nil
}

// LibraryName is the namespace prefix (`archive.`) and the `use` name.
const LibraryName = "archive"

// formatList is the rendered known-format string for error messages.
const formatList = `"tar", "zip", "tar.gz" (alias "tgz")`

// defaultMode is applied to an entry whose mode field is 0.
const defaultMode = 0o644

// entry is the Go-side view of one archive member.
type entry struct {
	name  string
	data  []byte
	mode  int64
	mtime int64 // unix seconds
}

// Install registers the archive surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Entry", []parser.StructField{
		{Name: "name", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "data", Type: parser.PrimitiveType(parser.TypeBytes)},
		{Name: "mode", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "mtime", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "pack", packFn)
	in.RegisterNamespaced(LibraryName, "unpack", unpackFn)
}

// makeEntry builds the Jennifer-side `archive.Entry` value.
func makeEntry(e entry) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Entry", []interpreter.StructField{
		{Name: "name", Value: interpreter.StringVal(e.name)},
		{Name: "data", Value: interpreter.BytesVal(e.data)},
		{Name: "mode", Value: interpreter.IntVal(e.mode)},
		{Name: "mtime", Value: interpreter.IntVal(e.mtime)},
	})
}

// extractEntry pulls the four fields out of an `archive.Entry`.
func extractEntry(idx int, v interpreter.Value) (entry, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Entry" {
		return entry{}, fmt.Errorf("archive.pack: entry %d must be an archive.Entry, got %s", idx, v.Kind)
	}
	var e entry
	for _, f := range v.Fields {
		switch f.Name {
		case "name":
			if f.Value.Kind != interpreter.KindString {
				return entry{}, fmt.Errorf("archive.pack: entry %d name must be string, got %s", idx, f.Value.Kind)
			}
			e.name = f.Value.Str
		case "data":
			if f.Value.Kind != interpreter.KindBytes {
				return entry{}, fmt.Errorf("archive.pack: entry %d data must be bytes, got %s", idx, f.Value.Kind)
			}
			e.data = f.Value.Bytes
		case "mode":
			if f.Value.Kind != interpreter.KindInt {
				return entry{}, fmt.Errorf("archive.pack: entry %d mode must be int, got %s", idx, f.Value.Kind)
			}
			e.mode = f.Value.Int
		case "mtime":
			if f.Value.Kind != interpreter.KindInt {
				return entry{}, fmt.Errorf("archive.pack: entry %d mtime must be int, got %s", idx, f.Value.Kind)
			}
			e.mtime = f.Value.Int
		}
	}
	return e, nil
}

func packFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("archive.pack expects 2 arguments (entries, format), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("archive.pack: first argument must be a list of archive.Entry, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("archive.pack: format must be string, got %s", args[1].Kind)
	}
	entries := make([]entry, 0, len(args[0].List))
	for i, ev := range args[0].List {
		e, err := extractEntry(i, ev)
		if err != nil {
			return interpreter.Null(), err
		}
		entries = append(entries, e)
	}
	var (
		out []byte
		err error
	)
	switch args[1].Str {
	case "tar":
		out, err = packTar(entries)
	case "zip":
		out, err = packZip(entries)
	case "tar.gz", "tgz":
		var raw []byte
		if raw, err = packTar(entries); err == nil {
			out, err = gzipBytes(raw)
		}
	default:
		return interpreter.Null(), fmt.Errorf("archive.pack: unknown format %q; known: %s", args[1].Str, formatList)
	}
	if err != nil {
		return interpreter.Null(), fmt.Errorf("archive.pack: %v", err)
	}
	return interpreter.BytesVal(out), nil
}

func unpackFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("archive.unpack expects 2 arguments (bytes, format), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("archive.unpack: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("archive.unpack: format must be string, got %s", args[1].Kind)
	}
	var (
		entries []entry
		err     error
	)
	switch args[1].Str {
	case "tar":
		entries, err = unpackTar(args[0].Bytes)
	case "zip":
		entries, err = unpackZip(args[0].Bytes)
	case "tar.gz", "tgz":
		var raw []byte
		if raw, err = gunzipBytes(args[0].Bytes); err == nil {
			entries, err = unpackTar(raw)
		}
	default:
		return interpreter.Null(), fmt.Errorf("archive.unpack: unknown format %q; known: %s", args[1].Str, formatList)
	}
	if err != nil {
		return interpreter.Null(), fmt.Errorf("archive.unpack: %v", err)
	}
	out := make([]interpreter.Value, len(entries))
	for i, e := range entries {
		out[i] = makeEntry(e)
	}
	return interpreter.ListVal(parser.NamespacedStructType(LibraryName, "Entry"), out), nil
}

// modeOf returns the entry's mode, or the default when unset.
func modeOf(e entry) int64 {
	if e.mode == 0 {
		return defaultMode
	}
	return e.mode
}

func packTar(entries []entry) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     modeOf(e),
			Size:     int64(len(e.data)),
			ModTime:  stdtime.Unix(e.mtime, 0).UTC(),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// checkEntryName rejects a member whose name would escape an extraction
// directory: an absolute path or a `..` traversal. Archive paths are
// slash-separated by spec; backslashes are normalized so a Windows-style
// `..\..\x` is caught too. The library never touches the filesystem, but the
// obvious extraction loop (fs.write(dir + "/" + name)) with such a name is the
// zip-slip hole, so it's closed at the decode source.
func checkEntryName(name string) error {
	if name == "" {
		return fmt.Errorf("archive entry has an empty name")
	}
	norm := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(norm, "/") || (len(norm) >= 2 && norm[1] == ':') {
		return fmt.Errorf("archive entry %q has an absolute path", name)
	}
	clean := path.Clean(norm)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("archive entry %q escapes the target directory", name)
	}
	return nil
}

func unpackTar(b []byte) ([]entry, error) {
	tr := tar.NewReader(bytes.NewReader(b))
	var entries []entry
	budget := maxDecompressed
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // only regular files map to an Entry
		}
		if err := checkEntryName(hdr.Name); err != nil {
			return nil, err
		}
		if len(entries) >= maxEntries {
			return nil, fmt.Errorf("archive holds more than %d entries", maxEntries)
		}
		data, err := readCapped(tr, &budget)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{name: hdr.Name, data: data, mode: hdr.Mode, mtime: hdr.ModTime.Unix()})
	}
	return entries, nil
}

func packZip(entries []entry) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{
			Name:     e.name,
			Method:   zip.Deflate,
			Modified: stdtime.Unix(e.mtime, 0).UTC(),
		}
		hdr.SetMode(fs.FileMode(modeOf(e)))
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unpackZip(b []byte) ([]entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	// Reject on the declared sizes up front (cheap - no decompression).
	// The declared values can lie, so the readCapped budget below stays
	// the authoritative check.
	var declared uint64
	fileCount := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		fileCount++
		if fileCount > maxEntries {
			return nil, fmt.Errorf("archive holds more than %d entries", maxEntries)
		}
		declared += f.UncompressedSize64
		if declared > uint64(maxDecompressed) {
			return nil, fmt.Errorf("total decompressed size exceeds the %d-byte limit", maxDecompressed)
		}
	}
	var entries []entry
	budget := maxDecompressed
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := checkEntryName(f.Name); err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := readCapped(rc, &budget)
		rc.Close()
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{
			name:  f.Name,
			data:  data,
			mode:  int64(f.Mode().Perm()),
			mtime: f.Modified.Unix(),
		})
	}
	return entries, nil
}

// gzipBytes / gunzipBytes wrap compress/gzip for the tar.gz combo.
func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipBytes(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	// The outer gzip stream gets its own full budget; the tar members
	// inside are then re-budgeted by unpackTar. Either cap tripping is a
	// bomb either way.
	budget := maxDecompressed
	out, err := readCapped(r, &budget)
	r.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
