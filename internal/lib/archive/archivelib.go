// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// maxDecompressed / maxEntries are the DEFAULT caps for archive.unpack: the total
// decompressed payload summed across all entries, and the member count. A small
// "zip bomb" cannot expand past them (a per-entry cap alone would still let N
// entries expand to N times it). Vars (not consts) so tests can lower them;
// archive.unpackWith lets a caller set its own. Exceeding any cap raises a
// catchable Error{kind: "limit"} - resource exhaustion, distinct from a "runtime"
// bug - so a caller can tell "the archive was too big" from "something broke".
var (
	maxDecompressed int64 = 256 << 20
	maxEntries            = 65536
)

// unpackCaps bounds one unpack call: total decompressed bytes, per-entry bytes,
// and entry count. A zero field in any dimension means "no limit" for it.
// skipSpecial drops a symlink / device / FIFO member instead of refusing the
// archive (the default refuses, so a dropped member is never silent).
type unpackCaps struct {
	total       int64
	entryBytes  int64
	entries     int
	skipSpecial bool
}

// defaultUnpackCaps reads the package defaults (which tests may lower). The
// per-entry default equals the total, so the default path adds no restriction
// beyond what archive.unpack already enforced.
func defaultUnpackCaps() unpackCaps {
	return unpackCaps{total: maxDecompressed, entryBytes: maxDecompressed, entries: maxEntries}
}

// capError marks a cap-exceeded (resource-exhaustion) failure so the builtin
// boundary can surface it as Error{kind: "limit"} rather than a generic runtime
// error. Other decode failures stay ordinary errors.
type capError struct{ msg string }

func (e *capError) Error() string { return e.msg }

// readCapped reads r fully, bounded by both the per-entry cap (caps.entryBytes)
// and the shared cross-entry budget (*budget); a zero in either dimension is
// unlimited. On success it deducts the bytes read from *budget so the total spans
// the whole call. A cap breach is a *capError.
func readCapped(r io.Reader, budget *int64, caps unpackCaps) ([]byte, error) {
	// Read ceiling: the tighter of the remaining total budget and the per-entry
	// cap (-1 = unlimited, so a 0 in either dimension drops out).
	limit := int64(-1)
	if *budget > 0 {
		limit = *budget
	}
	if caps.entryBytes > 0 && (limit < 0 || caps.entryBytes < limit) {
		limit = caps.entryBytes
	}
	var (
		data []byte
		err  error
	)
	if limit < 0 {
		data, err = io.ReadAll(r)
	} else {
		data, err = io.ReadAll(io.LimitReader(r, limit+1))
	}
	if err != nil {
		return nil, err
	}
	if caps.entryBytes > 0 && int64(len(data)) > caps.entryBytes {
		return nil, &capError{fmt.Sprintf("an archive entry exceeds the %d-byte per-entry limit", caps.entryBytes)}
	}
	if *budget > 0 && int64(len(data)) > *budget {
		return nil, &capError{fmt.Sprintf("total decompressed size exceeds the %d-byte limit", caps.total)}
	}
	if *budget > 0 {
		*budget -= int64(len(data))
	}
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
	// Caps for unpackWith: a 0 field takes the default, a negative field disables
	// that cap (unlimited), a positive field sets it. The zero value is all
	// defaults, so a bare `archive.UnpackOptions{}` behaves like archive.unpack.
	in.RegisterNamespacedStruct(LibraryName, "UnpackOptions", []parser.StructField{
		{Name: "maxTotalBytes", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "maxEntryBytes", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "maxEntries", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "skipSpecial", Type: parser.PrimitiveType(parser.TypeBool)},
	})
	in.RegisterNamespaced(LibraryName, "pack", packFn)
	in.RegisterNamespaced(LibraryName, "unpack", unpackFn)
	in.RegisterNamespaced(LibraryName, "unpackWith", unpackWithFn)
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
		// Refuse to write a member unpack would refuse to read, so pack never
		// produces an archive that is unsafe (or impossible) to extract.
		if err := checkEntryName(e.name); err != nil {
			return interpreter.Null(), fmt.Errorf("archive.pack: %v", err)
		}
		// Permission bits only: a higher bit is a file type to Go's archive
		// writers (a zip member with the symlink bit is written as a symlink).
		if e.mode < 0 || e.mode > 0o7777 {
			return interpreter.Null(), fmt.Errorf("archive.pack: entry %q has mode %#o; a mode holds permission bits only (0 to 0o7777)", e.name, e.mode)
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

func unpackFn(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("archive.unpack expects 2 arguments (bytes, format), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("archive.unpack: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("archive.unpack: format must be string, got %s", args[1].Kind)
	}
	entries, err := unpackBytes(args[0].Bytes, args[1].Str, defaultUnpackCaps())
	return finishUnpack(ctx, "archive.unpack", entries, err)
}

// unpackWithFn is archive.unpack with caller-set caps: archive.unpackWith(bytes,
// format, archive.UnpackOptions). A 0 option field takes the default cap; a
// negative field disables that cap (unlimited - for a trusted archive). Exceeding
// a cap raises Error{kind: "limit"}.
func unpackWithFn(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("archive.unpackWith expects 3 arguments (bytes, format, archive.UnpackOptions), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("archive.unpackWith: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("archive.unpackWith: format must be string, got %s", args[1].Kind)
	}
	caps, err := capsFromOptions(args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	entries, derr := unpackBytes(args[0].Bytes, args[1].Str, caps)
	return finishUnpack(ctx, "archive.unpackWith", entries, derr)
}

// unpackBytes decodes a bundle in the given format under the given caps.
func unpackBytes(b []byte, format string, caps unpackCaps) ([]entry, error) {
	switch format {
	case "tar":
		return unpackTar(b, caps)
	case "zip":
		return unpackZip(b, caps)
	case "tar.gz", "tgz":
		raw, err := gunzipBytes(b, caps)
		if err != nil {
			return nil, err
		}
		return unpackTar(raw, caps)
	default:
		return nil, fmt.Errorf("unknown format %q; known: %s", format, formatList)
	}
}

// finishUnpack renders the decoded entries as a `list of archive.Entry`, or turns
// a failure into an error - a cap breach (*capError) into a catchable
// Error{kind: "limit"} anchored at the call site, any other failure into a
// generic runtime error.
func finishUnpack(ctx interpreter.BuiltinCtx, fnName string, entries []entry, err error) (interpreter.Value, error) {
	if err != nil {
		var ce *capError
		if errors.As(err, &ce) {
			return interpreter.Null(), interpreter.RaiseError("limit", fnName+": "+ce.msg, ctx.File, ctx.Line, ctx.Col)
		}
		return interpreter.Null(), fmt.Errorf("%s: %v", fnName, err)
	}
	out := make([]interpreter.Value, len(entries))
	for i, e := range entries {
		out[i] = makeEntry(e)
	}
	return interpreter.ListVal(parser.NamespacedStructType(LibraryName, "Entry"), out), nil
}

// capsFromOptions reads an archive.UnpackOptions into unpackCaps: a 0 field takes
// the default, a negative field is unlimited, a positive field is that cap.
func capsFromOptions(v interpreter.Value) (unpackCaps, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "UnpackOptions" {
		return unpackCaps{}, fmt.Errorf("archive.unpackWith: options must be an archive.UnpackOptions, got %s", v.Kind)
	}
	def := defaultUnpackCaps()
	caps := def
	for _, f := range v.Fields {
		switch f.Name {
		case "maxTotalBytes":
			caps.total = capFromOpt(f.Value.Int, def.total)
		case "maxEntryBytes":
			caps.entryBytes = capFromOpt(f.Value.Int, def.entryBytes)
		case "maxEntries":
			caps.entries = int(capFromOpt(f.Value.Int, int64(def.entries)))
		case "skipSpecial":
			caps.skipSpecial = f.Value.Bool
		}
	}
	return caps, nil
}

// capFromOpt maps a UnpackOptions field: 0 -> the default, a negative -> unlimited
// (internal 0), a positive -> that value.
func capFromOpt(v, def int64) int64 {
	switch {
	case v == 0:
		return def
	case v < 0:
		return 0
	default:
		return v
	}
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

// hasDriveLetter reports whether s (already slash-normalized) begins with a
// Windows drive reference - an ASCII letter, a `:`, then a separator or the end
// of the string (`C:`, `C:/foo`). A bare `:` at index 1 is not enough: ordinary
// Unix names like `a:b.txt` or `1:1.log` are legal and must unpack.
func hasDriveLetter(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return false
	}
	return len(s) == 2 || s[2] == '/'
}

// checkEntryName rejects a member whose name could escape an extraction
// directory: an absolute path, or any `..` segment. Archive paths are
// slash-separated by spec; backslashes are normalized so a Windows-style
// `..\\x` is caught too. A `..` segment is refused even when the cleaned path
// stays inside (`a/../b`), because unpack returns the name verbatim: a caller
// that re-roots a sub-path of it (strip a wrapper directory, keep what follows
// `src/`) would otherwise turn `a/b/src/../../../x` into a write outside its
// target. The library never touches the filesystem, but the obvious extraction
// loop (fs.write(dir + "/" + name)) is the zip-slip hole, so it is closed at the
// decode source.
func checkEntryName(name string) error {
	if name == "" {
		return fmt.Errorf("archive entry has an empty name")
	}
	norm := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(norm, "/") || hasDriveLetter(norm) {
		return fmt.Errorf("archive entry %q has an absolute path", name)
	}
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return fmt.Errorf("archive entry %q has a `..` path segment", name)
		}
	}
	return nil
}

// specialError reports a member that is not a regular file or a directory - a
// symlink, device, FIFO or socket. It has no content an Entry could carry, and
// dropping it silently would hide that the archive held it, so unpack refuses
// unless the caller opted into skipping via UnpackOptions.skipSpecial.
func specialError(name, what string) error {
	return fmt.Errorf("archive entry %q is a %s, which an archive.Entry cannot represent; "+
		"pass archive.UnpackOptions{skipSpecial: true, ...} to archive.unpackWith to skip such entries", name, what)
}

// GNU tar header types archive/tar has no constant for.
const (
	tarTypeGNUDumpDir = 'D' // a directory, carrying a listing of its names
	tarTypeGNUVolume  = 'V' // the archive's volume label
)

// tarSkipped reports a header unpack consumes without yielding an Entry: a
// directory (implied by the member paths), or archive-wide metadata that is no
// member at all - a pax global header (`git archive` writes one first, holding
// the commit id) or a GNU volume label. It is checked before the member name,
// since a global header's POSIX default name is an absolute path.
func tarSkipped(flag byte) bool {
	switch flag {
	case tar.TypeDir, tarTypeGNUDumpDir, tar.TypeXGlobalHeader, tarTypeGNUVolume:
		return true
	}
	return false
}

// tarSpecialKind names a tar member type that maps to no Entry, or "" for one
// that does: a regular file (including the POSIX contiguous-file type and an
// old-style GNU sparse file, whose content archive/tar expands) or a hard link.
func tarSpecialKind(flag byte) string {
	switch flag {
	case tar.TypeReg, tar.TypeLink, tar.TypeCont, tar.TypeGNUSparse:
		return ""
	case tar.TypeSymlink:
		return "symbolic link"
	case tar.TypeChar:
		return "character device"
	case tar.TypeBlock:
		return "block device"
	case tar.TypeFifo:
		return "FIFO"
	default:
		return fmt.Sprintf("member of unsupported tar type %q", flag)
	}
}

func unpackTar(b []byte, caps unpackCaps) ([]entry, error) {
	tr := tar.NewReader(bytes.NewReader(b))
	var entries []entry
	// byName maps a regular member's name to its index in entries, so a hardlink
	// (which names an earlier member instead of carrying data) resolves to that
	// member's content rather than vanishing.
	byName := map[string]int{}
	skipped := map[string]bool{} // members dropped by skipSpecial
	budget := caps.total
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if tarSkipped(hdr.Typeflag) {
			continue
		}
		if err := checkEntryName(hdr.Name); err != nil {
			return nil, err
		}
		if kind := tarSpecialKind(hdr.Typeflag); kind != "" {
			if caps.skipSpecial {
				skipped[memberKey(hdr.Name)] = true
				continue
			}
			return nil, specialError(hdr.Name, kind)
		}
		// A hard link to a member skipped as special is skipped with it.
		if hdr.Typeflag == tar.TypeLink && skipped[memberKey(hdr.Linkname)] {
			skipped[memberKey(hdr.Name)] = true
			continue
		}
		if caps.entries > 0 && len(entries) >= caps.entries {
			return nil, &capError{fmt.Sprintf("archive holds more than %d entries", caps.entries)}
		}
		var data []byte
		if hdr.Typeflag == tar.TypeLink {
			idx, ok := byName[memberKey(hdr.Linkname)]
			if !ok {
				return nil, fmt.Errorf("archive entry %q is a hard link to %q, which is not an earlier regular file in the archive", hdr.Name, hdr.Linkname)
			}
			// Charge the linked content against the budget like a real copy: every
			// Entry owns its bytes (the copy below), so N links to one big member
			// cost N times its size (a link bomb otherwise skirts the total cap).
			src := entries[idx].data
			if caps.entryBytes > 0 && int64(len(src)) > caps.entryBytes {
				return nil, &capError{fmt.Sprintf("an archive entry exceeds the %d-byte per-entry limit", caps.entryBytes)}
			}
			if budget > 0 {
				if int64(len(src)) > budget {
					return nil, &capError{fmt.Sprintf("total decompressed size exceeds the %d-byte limit", caps.total)}
				}
				budget -= int64(len(src))
			}
			data = append([]byte(nil), src...)
		} else if data, err = readCapped(tr, &budget, caps); err != nil {
			return nil, err
		}
		byName[memberKey(hdr.Name)] = len(entries)
		// Mask to permission bits (0o777) like the zip path (f.Mode().Perm()): a
		// tar header carries setuid / setgid / sticky and file-type bits verbatim,
		// and Entry.mode is documented as plain permission bits. Dropping the
		// special bits keeps an untrusted archive from yielding a setuid mode an
		// extraction loop would then fs.chmod onto disk.
		entries = append(entries, entry{name: hdr.Name, data: data, mode: hdr.Mode & 0o777, mtime: hdr.ModTime.Unix()})
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

func unpackZip(b []byte, caps unpackCaps) ([]entry, error) {
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
		if caps.entries > 0 && fileCount > caps.entries {
			return nil, &capError{fmt.Sprintf("archive holds more than %d entries", caps.entries)}
		}
		if caps.entryBytes > 0 && f.UncompressedSize64 > uint64(caps.entryBytes) {
			return nil, &capError{fmt.Sprintf("an archive entry exceeds the %d-byte per-entry limit", caps.entryBytes)}
		}
		// Compare each declared size against the *remaining* total budget instead of
		// summing into `declared` and testing after: a crafted set of sizes that
		// sums past 2^64 would wrap the running total to a small value and slip
		// through the post-sum check. `declared <= total` is an invariant here, so
		// `total - declared` never underflows. A zero total means unlimited.
		if caps.total > 0 {
			maxDec := uint64(caps.total)
			if f.UncompressedSize64 > maxDec-declared {
				return nil, &capError{fmt.Sprintf("total decompressed size exceeds the %d-byte limit", caps.total)}
			}
			declared += f.UncompressedSize64
		}
	}
	var entries []entry
	budget := caps.total
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := checkEntryName(f.Name); err != nil {
			return nil, err
		}
		if kind := zipSpecialKind(f.Mode()); kind != "" {
			if caps.skipSpecial {
				continue
			}
			return nil, specialError(f.Name, kind)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := readCapped(rc, &budget, caps)
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

// zipSpecialKind names a zip member mode that maps to no Entry, or "" for a
// regular file. A zip symlink stores its target as the member's content, so
// without this check it would come back as a regular file holding the target
// path, unlike the same archive in tar form.
func zipSpecialKind(m fs.FileMode) string {
	switch {
	case m&fs.ModeSymlink != 0:
		return "symbolic link"
	case m&fs.ModeNamedPipe != 0:
		return "FIFO"
	case m&fs.ModeSocket != 0:
		return "socket"
	case m&fs.ModeCharDevice != 0:
		return "character device"
	case m&fs.ModeDevice != 0:
		return "block device"
	case m&fs.ModeIrregular != 0:
		return "member of unknown type"
	}
	return ""
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

func gunzipBytes(b []byte, caps unpackCaps) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	// The outer gzip stream gets the full total budget; the tar members inside are
	// then re-budgeted by unpackTar. Either cap tripping is a bomb either way. The
	// per-entry cap does not apply to the whole gzip stream, so clear it here.
	budget := caps.total
	out, err := readCapped(r, &budget, unpackCaps{total: caps.total})
	r.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// memberKey is the lookup form of a tar member name for hardlink resolution:
// slash-normalized and cleaned, so `./a/b` and `a/b` name the same member.
func memberKey(name string) string {
	return path.Clean(strings.ReplaceAll(name, "\\", "/"))
}
