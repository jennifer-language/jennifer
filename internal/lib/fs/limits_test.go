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

// sparseFile creates a file of size bytes (all zero, no newline) without
// writing them.
func sparseFile(t *testing.T, size int64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "big.dat")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

// TestWholeReadCap: fs.readBytes / readString on a file over
// limits.MaxAllocBytes fail with a catchable error from the stat size,
// without reading or allocating the file.
func TestWholeReadCap(t *testing.T) {
	p := sparseFile(t, limits.MaxAllocBytes+1)
	for _, fn := range []string{"readBytes", "readString"} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		out, err := runProg(t, fmt.Sprintf(`
			use io; use fs;
			try { fs.%s(%q); } catch (e) { io.printf("%%s", $e.message); }
		`, fn, p))
		runtime.ReadMemStats(&after)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !strings.Contains(out, "fs."+fn+": ") || !strings.Contains(out, "byte limit") {
			t.Fatalf("%s: got %q, want the byte-limit error", fn, out)
		}
		if got := after.TotalAlloc - before.TotalAlloc; got > 16<<20 {
			t.Fatalf("%s allocated %d bytes before refusing", fn, got)
		}
	}
}

// TestWholeReadDevZero: a device with no size (stat 0) stops at the cap.
func TestWholeReadDevZero(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	out, err := runProg(t, `
		use io; use fs;
		try { fs.readBytes("/dev/zero"); } catch (e) { io.printf("%s", $e.message); }
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fs.readBytes: /dev/zero exceeds") {
		t.Fatalf("got %q", out)
	}
}

// TestReadLineCap: a line longer than limits.MaxAllocBytes is a catchable
// error, not an unbounded buffer.
func TestReadLineCap(t *testing.T) {
	p := sparseFile(t, limits.MaxAllocBytes+1)
	out, err := runProg(t, fmt.Sprintf(`
		use io; use fs;
		def f as fs.File init fs.open(%q, "read");
		try { fs.readLine($f); } catch (e) { io.printf("%%s", $e.message); }
		fs.close($f);
	`, p))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fs.readLine: ") || !strings.Contains(out, "line exceeds") {
		t.Fatalf("got %q", out)
	}
}

// TestReadCharsCap: n past the per-call character limit is refused up front.
func TestReadCharsCap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.txt")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io; use fs;
		def f as fs.File init fs.open(%q, "read");
		try { fs.readChars($f, 9000000000000); } catch (e) { io.printf("%%s;", $e.message); }
		io.printf("%%s", fs.readChars($f, 2));
		fs.close($f);
	`, p))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fs.readChars: 9000000000000 exceeds") || !strings.HasSuffix(out, ";ab") {
		t.Fatalf("got %q", out)
	}
}
