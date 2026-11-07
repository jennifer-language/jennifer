// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestHandleReadBytesGrowsWithData: fs.readBytes($f, n) on a short file
// allocates about what it reads, not n up front.
func TestHandleReadBytesGrowsWithData(t *testing.T) {
	p := filepath.Join(t.TempDir(), "six.txt")
	if err := os.WriteFile(p, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, err := runProg(t, fmt.Sprintf(`
		use io; use fs;
		def f as fs.File init fs.open(%q, "read");
		def b as bytes init fs.readBytes($f, %d);
		fs.close($f);
		io.printf("%%d", len($b));
	`, p, limits.MaxAllocBytes))
	runtime.ReadMemStats(&after)
	if err != nil || out != "6" {
		t.Fatalf("got %q, %v", out, err)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 16<<20 {
		t.Fatalf("readBytes of 6 bytes allocated %d bytes", got)
	}
}

// TestChownRange: ids outside [-1, 4294967295] are rejected before the
// syscall instead of wrapping (-2 -> 4294967294) or truncating on 32-bit.
func TestChownRange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ uid, gid, want string }{
		{"-2", "-1", "fs.chown: uid -2 out of range [-1, "},
		{"-1", "5000000000", "fs.chown: gid 5000000000 out of range [-1, "},
	} {
		out, err := runProg(t, fmt.Sprintf(`
			use io; use fs;
			try { fs.chown(%q, %s, %s); } catch (e) { io.printf("%%s", $e.message); }
		`, p, c.uid, c.gid))
		if err != nil || !strings.HasPrefix(out, c.want) {
			t.Errorf("chown(%s, %s): got %q, %v; want prefix %q", c.uid, c.gid, out, err, c.want)
		}
	}
}

// TestWalkSkipsUnreadableDir: an unreadable subdirectory is listed but not
// descended into; the rest of the tree is still returned.
func TestWalkSkipsUnreadableDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a non-root Unix user for chmod 000 to deny reads")
	}
	root := t.TempDir()
	for _, d := range []string{"a", "locked"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"a/x", "locked/hidden", "z"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	out, err := runProg(t, fmt.Sprintf(`
		use io; use fs;
		for (def s in fs.walk(%q)) { io.printf("%%s\n", $s.path); }
	`, root))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{root, root + "/a", root + "/a/x", root + "/locked", root + "/z"}, "\n") + "\n"
	if out != want {
		t.Fatalf("walk:\n%s\nwant:\n%s", out, want)
	}

	// An unreadable root is still an error.
	out, err = runProg(t, fmt.Sprintf(`
		use io; use fs;
		try { fs.walk(%q); } catch (e) { io.printf("%%s", $e.message); }
	`, locked))
	if err != nil || !strings.HasPrefix(out, "fs.walk: "+locked) {
		t.Fatalf("unreadable root: got %q, %v", out, err)
	}
}
