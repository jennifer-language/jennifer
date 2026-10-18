// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package archivelib

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func str(s string) interpreter.Value { return interpreter.StringVal(s) }

func entriesVal(es []entry) interpreter.Value {
	vs := make([]interpreter.Value, len(es))
	for i, e := range es {
		vs[i] = makeEntry(e)
	}
	return interpreter.ListVal(parser.NamespacedStructType(LibraryName, "Entry"), vs)
}

func roundtrip(t *testing.T, format string, in []entry) []entry {
	t.Helper()
	packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal(in), str(format)})
	if err != nil {
		t.Fatalf("%s pack: %v", format, err)
	}
	if packed.Kind != interpreter.KindBytes {
		t.Fatalf("%s: pack returned %s, want bytes", format, packed.Kind)
	}
	back, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str(format)})
	if err != nil {
		t.Fatalf("%s unpack: %v", format, err)
	}
	out := make([]entry, len(back.List))
	for i, v := range back.List {
		e, err := extractEntry(i, v)
		if err != nil {
			t.Fatalf("%s extract %d: %v", format, i, err)
		}
		out[i] = e
	}
	return out
}

func TestRoundTrip(t *testing.T) {
	in := []entry{
		{name: "a.txt", data: []byte("alpha"), mode: 0o644, mtime: 1700000000},
		{name: "dir/b.txt", data: []byte("bravo bravo bravo"), mode: 0o600, mtime: 1700000000},
	}
	for _, format := range []string{"tar", "zip", "tar.gz", "tgz"} {
		out := roundtrip(t, format, in)
		if len(out) != len(in) {
			t.Fatalf("%s: got %d entries, want %d", format, len(out), len(in))
		}
		for i := range in {
			if out[i].name != in[i].name {
				t.Errorf("%s[%d] name = %q, want %q", format, i, out[i].name, in[i].name)
			}
			if string(out[i].data) != string(in[i].data) {
				t.Errorf("%s[%d] data = %q, want %q", format, i, out[i].data, in[i].data)
			}
		}
	}
}

func TestModeAndMtimePreserved(t *testing.T) {
	in := []entry{{name: "x", data: []byte("d"), mode: 0o600, mtime: 1700000000}}
	// tar preserves both exactly.
	tarOut := roundtrip(t, "tar", in)
	if tarOut[0].mode&0o777 != 0o600 {
		t.Errorf("tar mode = %o, want 600", tarOut[0].mode&0o777)
	}
	if tarOut[0].mtime != 1700000000 {
		t.Errorf("tar mtime = %d, want 1700000000", tarOut[0].mtime)
	}
	// zip preserves the permission bits (mtime uses the DOS field + an
	// extended timestamp; the permission round-trip is the stable assertion).
	zipOut := roundtrip(t, "zip", in)
	if zipOut[0].mode&0o777 != 0o600 {
		t.Errorf("zip mode = %o, want 600", zipOut[0].mode&0o777)
	}
}

func TestDefaultMode(t *testing.T) {
	out := roundtrip(t, "tar", []entry{{name: "x", data: []byte("d"), mode: 0}})
	if out[0].mode&0o777 != defaultMode {
		t.Errorf("default mode = %o, want %o", out[0].mode&0o777, defaultMode)
	}
}

func TestTarGzIsGzip(t *testing.T) {
	packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		entriesVal([]entry{{name: "x", data: []byte("d")}}), str("tar.gz")})
	if err != nil {
		t.Fatal(err)
	}
	if len(packed.Bytes) < 2 || packed.Bytes[0] != 0x1f || packed.Bytes[1] != 0x8b {
		t.Errorf("tar.gz output is not gzip-framed: %x", packed.Bytes[:min(4, len(packed.Bytes))])
	}
}

func TestEmptyArchive(t *testing.T) {
	if out := roundtrip(t, "tar", nil); len(out) != 0 {
		t.Errorf("empty tar produced %d entries", len(out))
	}
	if out := roundtrip(t, "zip", nil); len(out) != 0 {
		t.Errorf("empty zip produced %d entries", len(out))
	}
}

func TestErrors(t *testing.T) {
	entries := entriesVal([]entry{{name: "x", data: []byte("d")}})
	ctx := interpreter.BuiltinCtx{}
	if _, err := packFn(ctx, []interpreter.Value{entries, str("rar")}); err == nil {
		t.Error("pack: unknown format should error")
	}
	if _, err := unpackFn(ctx, []interpreter.Value{interpreter.BytesVal([]byte("x")), str("rar")}); err == nil {
		t.Error("unpack: unknown format should error")
	}
	if _, err := packFn(ctx, []interpreter.Value{str("not a list"), str("tar")}); err == nil {
		t.Error("pack: non-list first argument should error")
	}
	// a list whose element is not an archive.Entry
	badList := interpreter.ListVal(parser.NamespacedStructType(LibraryName, "Entry"), []interpreter.Value{interpreter.IntVal(3)})
	if _, err := packFn(ctx, []interpreter.Value{badList, str("tar")}); err == nil {
		t.Error("pack: non-Entry list element should error")
	}
	if _, err := unpackFn(ctx, []interpreter.Value{interpreter.BytesVal([]byte("not a zip file")), str("zip")}); err == nil {
		t.Error("unpack: corrupt zip should error")
	}
	if _, err := unpackFn(ctx, []interpreter.Value{str("x"), str("tar")}); err == nil {
		t.Error("unpack: non-bytes first argument should error")
	}
}

// The decompression cap must bound the TOTAL expansion of one unpack call,
// not each entry separately - otherwise a small archive with many entries
// each just under the cap expands to (entries x cap) bytes and OOMs on
// untrusted input. The entry count is capped for the same reason.
func TestUnpackAggregateCaps(t *testing.T) {
	lowerCaps := func(bytes int64, entries int) func() {
		ob, oe := maxDecompressed, maxEntries
		maxDecompressed, maxEntries = bytes, entries
		return func() { maxDecompressed, maxEntries = ob, oe }
	}

	// Three 600-byte entries pass a 1 KiB per-entry check but total ~1.8 KiB.
	big := make([]byte, 600)
	bomb := []entry{
		{name: "a", data: big, mtime: 1700000000},
		{name: "b", data: big, mtime: 1700000000},
		{name: "c", data: big, mtime: 1700000000},
	}
	for _, format := range []string{"tar", "zip", "tar.gz"} {
		packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal(bomb), str(format)})
		if err != nil {
			t.Fatalf("%s pack: %v", format, err)
		}
		restore := lowerCaps(1024, 1000)
		_, err = unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str(format)})
		restore()
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Errorf("%s: want aggregate-size error, got %v", format, err)
		}
	}

	// Entry-count cap: five entries against a cap of four.
	many := make([]entry, 5)
	for i := range many {
		many[i] = entry{name: string(rune('a' + i)), data: []byte("x"), mtime: 1700000000}
	}
	for _, format := range []string{"tar", "zip"} {
		packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal(many), str(format)})
		if err != nil {
			t.Fatalf("%s pack: %v", format, err)
		}
		restore := lowerCaps(maxDecompressed, 4)
		_, err = unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str(format)})
		restore()
		if err == nil || !strings.Contains(err.Error(), "entries") {
			t.Errorf("%s: want entry-count error, got %v", format, err)
		}
	}

	// Under the caps everything still unpacks.
	packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal(bomb), str("tar")})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str("tar")}); err != nil {
		t.Errorf("in-budget unpack failed: %v", err)
	}
}

// TestUnpackWithCaps covers caller-set caps and the kind:"limit" contract: each
// dimension trips its own cap, a breach classifies as kind "limit", a 0 field
// takes the default, and a negative field disables the cap.
func TestUnpackWithCaps(t *testing.T) {
	opts := func(total, entryBytes, entries int64) interpreter.Value {
		return interpreter.NamespacedStructVal("archive", "UnpackOptions", []interpreter.StructField{
			{Name: "maxTotalBytes", Value: interpreter.IntVal(total)},
			{Name: "maxEntryBytes", Value: interpreter.IntVal(entryBytes)},
			{Name: "maxEntries", Value: interpreter.IntVal(entries)},
			{Name: "skipSpecial", Value: interpreter.BoolVal(false)},
		})
	}
	es := []entry{
		{name: "a", data: make([]byte, 600), mtime: 1700000000},
		{name: "b", data: make([]byte, 600), mtime: 1700000000},
	}
	packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal(es), str("tar")})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	// Each cap trips, and the breach classifies as kind "limit".
	check := func(name string, o interpreter.Value) {
		_, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str("tar"), o})
		if err == nil {
			t.Errorf("%s: expected a cap error", name)
			return
		}
		if kind, _, _, _, _ := interpreter.ClassifyError(err); kind != "limit" {
			t.Errorf("%s: kind = %q, want limit", name, kind)
		}
	}
	check("entries", opts(0, 0, 1))
	check("per-entry", opts(0, 100, 0))
	check("total", opts(1000, 0, 0))
	// A 0 field = default, so a bare options unpacks fine.
	if _, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str("tar"), opts(0, 0, 0)}); err != nil {
		t.Errorf("default (all-zero) options should unpack: %v", err)
	}
	// A negative field disables that cap (unlimited): a tiny entry cap of -1 passes.
	if _, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{packed, str("tar"), opts(-1, -1, -1)}); err != nil {
		t.Errorf("unlimited (negative) options should unpack: %v", err)
	}
}

// TestCheckEntryNameDriveLetter pins the Windows drive-letter guard must
// reject only a real drive reference (letter + ':' + separator/end), not any
// Unix name whose second character is ':'.
func TestCheckEntryNameDriveLetter(t *testing.T) {
	ok := []string{"a:b.txt", "1:1.log", "dir/a:b", "a.txt", "sub/x"}
	for _, n := range ok {
		if err := checkEntryName(n); err != nil {
			t.Errorf("checkEntryName(%q) = %v, want nil (legal Unix name)", n, err)
		}
	}
	bad := []string{"C:/x.txt", "c:\\x", "D:", "/abs/path", "../up", "x/../../y",
		"a/b/c/d/src/../../../../PWNED.txt", "a/../b", "a\\..\\b", "x/.."}
	for _, n := range bad {
		if err := checkEntryName(n); err == nil {
			t.Errorf("checkEntryName(%q) = nil, want rejection", n)
		}
	}
}

// TestUnpackTarMasksSpecialBits pins a tar header's setuid / setgid / sticky
// and type bits must be masked to plain permission bits on unpack, matching the
// zip path, so an untrusted tar cannot yield a setuid Entry.mode.
func TestUnpackTarMasksSpecialBits(t *testing.T) {
	packed, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		entriesVal([]entry{{name: "suid", data: []byte("x"), mode: 0o4755, mtime: 1700000000}}),
		str("tar"),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	entries, err := unpackTar(packed.Bytes, defaultUnpackCaps())
	if err != nil {
		t.Fatalf("unpackTar: %v", err)
	}
	if len(entries) != 1 || entries[0].mode != 0o755 {
		t.Fatalf("tar entry mode = 0o%o, want 0o755 (special bits stripped)", entries[0].mode)
	}
}

// A name whose cleaned form stays inside but which carries `..` segments is
// refused on unpack (and never written by pack): unpack returns names verbatim,
// so a caller that re-roots a sub-path of the name must not be handed `..`.
func TestDotDotSegmentsRefused(t *testing.T) {
	name := "a/b/c/d/src/../../../../PWNED.txt"
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	if _, err := unpackTar(buf.Bytes(), defaultUnpackCaps()); err == nil || !strings.Contains(err.Error(), "`..` path segment") {
		t.Errorf("unpackTar(%q) = %v, want a `..` segment error", name, err)
	}
	if _, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{entriesVal([]entry{{name: name, data: []byte("x")}}), str("tar")}); err == nil {
		t.Errorf("pack(%q) succeeded, want a refusal", name)
	}
}

// tarWith builds a tar from raw headers (with their data, if any).
func tarWith(t *testing.T, members []struct {
	hdr  tar.Header
	data string
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, m := range members {
		h := m.hdr
		h.Size = int64(len(m.data))
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type member = struct {
	hdr  tar.Header
	data string
}

// A hard link resolves to the content of the member it names, rather than the
// file vanishing from the unpacked list.
func TestTarHardlinkResolves(t *testing.T) {
	b := tarWith(t, []member{
		{tar.Header{Name: "./src/hard.j", Mode: 0o644, Typeflag: tar.TypeReg}, "payload"},
		{tar.Header{Name: "src/real.j", Mode: 0o600, Typeflag: tar.TypeLink, Linkname: "src/hard.j"}, ""},
	})
	es, err := unpackTar(b, defaultUnpackCaps())
	if err != nil {
		t.Fatalf("unpackTar: %v", err)
	}
	if len(es) != 2 || es[1].name != "src/real.j" || string(es[1].data) != "payload" || es[1].mode != 0o600 {
		t.Fatalf("entries = %+v, want src/real.j carrying the linked content", es)
	}
	// A link to a member that is not in the archive (or not yet) is an error.
	dangling := tarWith(t, []member{{tar.Header{Name: "a", Typeflag: tar.TypeLink, Linkname: "missing"}, ""}})
	if _, err := unpackTar(dangling, defaultUnpackCaps()); err == nil {
		t.Error("a hard link to a missing member should error")
	}
	// Links count against the total budget like copies, so a link bomb is bounded.
	bomb := []member{{tar.Header{Name: "big", Mode: 0o644, Typeflag: tar.TypeReg}, strings.Repeat("x", 600)}}
	for i := 0; i < 3; i++ {
		bomb = append(bomb, member{tar.Header{Name: "l" + string(rune('a'+i)), Typeflag: tar.TypeLink, Linkname: "big"}, ""})
	}
	caps := defaultUnpackCaps()
	caps.total = 1500
	if _, err := unpackTar(tarWith(t, bomb), caps); err == nil {
		t.Error("hard links past the total budget should trip the cap")
	}
	// Each linked entry owns its bytes.
	es[1].data[0] = 'X'
	if string(es[0].data) != "payload" {
		t.Error("a hard-linked entry shares its target's bytes")
	}
	// A link to a member skipped as special is skipped with it.
	viaSymlink := tarWith(t, []member{
		{tar.Header{Name: "sym", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, ""},
		{tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: "sym"}, ""},
		{tar.Header{Name: "real", Mode: 0o644, Typeflag: tar.TypeReg}, "x"},
	})
	skipCaps := defaultUnpackCaps()
	skipCaps.skipSpecial = true
	if es, err := unpackTar(viaSymlink, skipCaps); err != nil || len(es) != 1 || es[0].name != "real" {
		t.Errorf("link to a skipped symlink: %+v, %v; want only real", es, err)
	}
}

// pack refuses a mode with bits above the permission bits: Go's zip writer
// turns the symlink bit into a real symbolic link member.
func TestPackRefusesFileTypeModeBits(t *testing.T) {
	for _, format := range []string{"zip", "tar"} {
		in := entriesVal([]entry{{name: "link", data: []byte("/etc/passwd"), mode: 1<<27 | 0o777}})
		if _, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{in, str(format)}); err == nil || !strings.Contains(err.Error(), "permission bits only") {
			t.Errorf("%s: err = %v, want a mode refusal", format, err)
		}
	}
	if got := roundtrip(t, "zip", []entry{{name: "s", data: []byte("x"), mode: 0o4755}}); len(got) != 1 {
		t.Errorf("a setuid permission mode should still pack: %+v", got)
	}
}

// Symlinks, devices and FIFOs refuse the archive by default (an Entry cannot
// represent them, and dropping them silently hides that they were there), and
// are skipped when the caller opts in.
func TestSpecialEntries(t *testing.T) {
	for _, flag := range []byte{tar.TypeSymlink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		b := tarWith(t, []member{
			{tar.Header{Name: "src/link.j", Typeflag: flag, Linkname: "/etc/passwd"}, ""},
			{tar.Header{Name: "src/real.j", Mode: 0o644, Typeflag: tar.TypeReg}, "x"},
		})
		if _, err := unpackTar(b, defaultUnpackCaps()); err == nil || !strings.Contains(err.Error(), "skipSpecial") {
			t.Errorf("tar type %q: err = %v, want a refusal naming skipSpecial", flag, err)
		}
		caps := defaultUnpackCaps()
		caps.skipSpecial = true
		es, err := unpackTar(b, caps)
		if err != nil || len(es) != 1 || es[0].name != "src/real.j" {
			t.Errorf("tar type %q with skipSpecial: %+v, %v; want only src/real.j", flag, es, err)
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "src/link.j"}
	h.SetMode(fs.ModeSymlink | 0o777)
	w, _ := zw.CreateHeader(h)
	_, _ = w.Write([]byte("/etc/passwd"))
	h2 := &zip.FileHeader{Name: "src/real.j"}
	h2.SetMode(0o644)
	w, _ = zw.CreateHeader(h2)
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	if _, err := unpackZip(buf.Bytes(), defaultUnpackCaps()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("zip symlink: err = %v, want a refusal (not a regular file holding the target)", err)
	}
	caps := defaultUnpackCaps()
	caps.skipSpecial = true
	es, err := unpackZip(buf.Bytes(), caps)
	if err != nil || len(es) != 1 || es[0].name != "src/real.j" {
		t.Errorf("zip symlink with skipSpecial: %+v, %v; want only src/real.j", es, err)
	}

	// The option reaches unpackWith through archive.UnpackOptions.
	opts := interpreter.NamespacedStructVal("archive", "UnpackOptions", []interpreter.StructField{
		{Name: "maxTotalBytes", Value: interpreter.IntVal(0)},
		{Name: "maxEntryBytes", Value: interpreter.IntVal(0)},
		{Name: "maxEntries", Value: interpreter.IntVal(0)},
		{Name: "skipSpecial", Value: interpreter.BoolVal(true)},
	})
	got, err := unpackWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.BytesVal(buf.Bytes()), str("zip"), opts})
	if err != nil || len(got.List) != 1 {
		t.Errorf("unpackWith skipSpecial: %v entries, %v", len(got.List), err)
	}
}

// Archive-wide metadata is consumed, never a member: a pax global header (what
// `git archive` writes first, holding the commit id; its POSIX default name is
// an absolute path) and a GNU volume label. A GNU dumpdir is a directory. None
// is an Entry, and none needs skipSpecial, which keeps its meaning: a symlink
// beside them is still refused.
func TestTarMetadataHeadersConsumed(t *testing.T) {
	for _, name := range []string{"pax_global_header", "/tmp/GlobalHead.1.1"} {
		b := tarWith(t, []member{
			{tar.Header{Name: name, Typeflag: tar.TypeXGlobalHeader,
				PAXRecords: map[string]string{"comment": "6f1c0a2e"}}, ""},
			{tar.Header{Name: "vol", Typeflag: tarTypeGNUVolume, Format: tar.FormatGNU}, ""},
			{tar.Header{Name: "src/", Typeflag: tarTypeGNUDumpDir, Format: tar.FormatGNU}, "Ya.txt\x00"},
			{tar.Header{Name: "src/a.txt", Mode: 0o644, Typeflag: tar.TypeReg}, "hi"},
		})
		es, err := unpackTar(b, defaultUnpackCaps())
		if err != nil || len(es) != 1 || es[0].name != "src/a.txt" || string(es[0].data) != "hi" {
			t.Errorf("global header %q: %+v, %v; want only src/a.txt", name, es, err)
		}
	}
	b := tarWith(t, []member{
		{tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader,
			PAXRecords: map[string]string{"comment": "6f1c0a2e"}}, ""},
		{tar.Header{Name: "src/link.j", Typeflag: tar.TypeSymlink, Linkname: "a.txt"}, ""},
	})
	if _, err := unpackTar(b, defaultUnpackCaps()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("symlink after a global header: err = %v, want a refusal", err)
	}
}

// A POSIX contiguous file is a regular file.
func TestTarContiguousFileIsRegular(t *testing.T) {
	b := tarWith(t, []member{
		{tar.Header{Name: "data.bin", Mode: 0o644, Typeflag: tar.TypeCont}, "abc"},
	})
	es, err := unpackTar(b, defaultUnpackCaps())
	if err != nil || len(es) != 1 || string(es[0].data) != "abc" {
		t.Errorf("contiguous file: %+v, %v", es, err)
	}
}

// A real `git archive` tarball unpacks under the default options.
func TestGitArchiveTarball(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(gitBin, append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return out
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("init", "-q")
	run("add", "a.txt")
	run("commit", "-qm", "x")
	tarball := run("archive", "HEAD")
	if !bytes.Contains(tarball, []byte("pax_global_header")) {
		t.Fatal("git archive wrote no pax global header; the test proves nothing")
	}
	es, err := unpackTar(tarball, defaultUnpackCaps())
	if err != nil || len(es) != 1 || es[0].name != "a.txt" || string(es[0].data) != "hi" {
		t.Errorf("git archive: %+v, %v; want a.txt", es, err)
	}
}
