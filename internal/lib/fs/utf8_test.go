// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestTextReadsInvalidUTF8: readString, readLine and readChars name the
// function, the path and the offset of the first invalid byte. Before the fix
// readLine returned the raw bytes and the other two gave no offset.
func TestTextReadsInvalidUTF8(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.txt")
	if err := os.WriteFile(p, []byte("ok\nab\xffc\nnext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io; use fs;
		try { fs.readString(%[1]q); } catch (e) { io.printf("%%s\n", $e.message); }
		def f as fs.File init fs.open(%[1]q, "read");
		io.printf("%%s\n", fs.readLine($f));
		try { fs.readLine($f); } catch (e) { io.printf("%%s\n", $e.message); }
		io.printf("%%s\n", fs.readLine($f));
		fs.close($f);
		def g as fs.File init fs.open(%[1]q, "read");
		try { fs.readChars($g, 10); } catch (e) { io.printf("%%s\n", $e.message); }
		fs.close($g);
	`, p))
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("fs.readString: %[1]s: invalid UTF-8 at byte 5\nok\nfs.readLine: %[1]s: invalid UTF-8 at byte 2\nnext\nfs.readChars: %[1]s: invalid UTF-8 at byte 5\n", p)
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}
