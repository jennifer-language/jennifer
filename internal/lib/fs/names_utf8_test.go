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
)

// TestNamesUTF8 (M29.50): list, walk, readlink and realpath refuse a name that
// is not valid UTF-8 (kind "fs", naming the entry and the byte offset);
// listBytes and walkBytes return every name as bytes.
func TestNamesUTF8(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs a filesystem that stores any byte in a name")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a\xffb"), []byte("x"), 0o644); err != nil {
		t.Skipf("filesystem refuses a non-UTF-8 name: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("t\xff", link); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(dir, "d\xfe")
	if err := os.Mkdir(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(badDir, filepath.Join(dir, "todir")); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		try { fs.list(%[2]q); } catch (e) { io.printf("list %%s %%s\n", $e.kind, $e.message); }
		try { fs.walk(%[1]q); } catch (e) { io.printf("walk %%s\n", $e.kind); }
		try { fs.readlink(%[3]q); } catch (e) { io.printf("readlink %%s\n", $e.kind); }
		try { fs.realpath(%[4]q); } catch (e) { io.printf("realpath %%s\n", $e.kind); }
		def names as list of bytes init fs.listBytes(%[2]q);
		io.printf("%%d %%d %%d\n", len($names), len($names[0]), $names[0][1]);
		def n as int init 0;
		for (def st in fs.walkBytes(%[1]q)) {
			if (len($st.path) > 0) { $n = $n + 1; }
		}
		io.printf("walkBytes %%d\n", $n);
	`, dir, sub, link, filepath.Join(dir, "todir")))
	if err != nil {
		t.Fatalf("%v (out %q)", err, out)
	}
	for _, want := range []string{
		`list fs fs.list: ` + sub + `: entry "a\xffb" is not valid UTF-8 at byte 1; fs.listBytes returns names as bytes`,
		"walk fs\n", "readlink fs\n", "realpath fs\n",
		"1 3 255\n",
		"walkBytes 5\n", // the root, d\xfe, ok.txt, sub, sub/a\xffb (links skipped)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
