// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fs.seek drops the read buffer and the sticky EOF flag; fs.tell reports the
// logical position (bytes returned so far), not the OS offset.
func TestSeekTellRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def f as fs.File init fs.open(%q, "read");
		io.printf("t0 %%d\n", fs.tell($f));
		io.printf("l1 %%s\n", fs.readLine($f));
		io.printf("t1 %%d\n", fs.tell($f));
		io.printf("c %%s\n", fs.readChars($f, 2));
		io.printf("t2 %%d\n", fs.tell($f));
		fs.seek($f, 11);
		io.printf("l3 %%s t %%d\n", fs.readLine($f), fs.tell($f));
		io.printf("eof %%v\n", fs.eof($f));
		fs.seek($f, 6);
		io.printf("eof %%v\n", fs.eof($f));
		io.printf("b %%s\n", fs.readChars($f, 4));
		io.printf("t3 %%d\n", fs.tell($f));
		fs.seek($f, 0);
		io.printf("l0 %%s\n", fs.readLine($f));
		fs.seek($f, 100);
		io.printf("past eof %%v t %%d\n", fs.eof($f), fs.tell($f));
		io.printf("past bytes %%d\n", len(fs.readBytes($f, 10)));
		try { fs.readLine($f); } catch (e) { io.printf("past readLine %%s\n", $e.kind); }
		fs.seek($f, 16);
		io.printf("last %%d\n", len(fs.readBytes($f, 10)));
		try { fs.seek($f, -1); } catch (e) { io.printf("negative %%s\n", $e.kind); }
		fs.close($f);
		try { fs.seek($f, 0); } catch (e) { io.printf("closed seek %%s\n", $e.kind); }
		try { fs.tell($f); } catch (e) { io.printf("closed tell %%s\n", $e.kind); }
	`, path))
	if err != nil {
		t.Fatal(err)
	}
	want := "t0 0\nl1 alpha\nt1 6\nc be\nt2 8\nl3 gamma t 17\neof true\neof false\n" +
		"b beta\nt3 10\nl0 alpha\npast eof true t 100\npast bytes 0\npast readLine runtime\n" +
		"last 1\nnegative runtime\nclosed seek closed\nclosed tell closed\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

// A write handle writes at the sought offset; past the end the gap is zero
// bytes. An append handle refuses seek and tells the end of the file.
func TestSeekTellWriteAppend(t *testing.T) {
	dir := t.TempDir()
	w := filepath.Join(dir, "w.bin")
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def f as fs.File init fs.open(%[1]q, "write");
		fs.writeString($f, "abcdef");
		io.printf("w %%d\n", fs.tell($f));
		fs.seek($f, 2);
		fs.writeString($f, "XY");
		io.printf("w %%d\n", fs.tell($f));
		fs.seek($f, 8);
		fs.writeString($f, "Z");
		fs.close($f);
		def g as fs.File init fs.open(%[2]q, "append");
		io.printf("a %%d\n", fs.tell($g));
		fs.writeString($g, "67");
		io.printf("a %%d\n", fs.tell($g));
		try { fs.seek($g, 0); } catch (e) { io.printf("append seek %%s\n", $e.kind); }
		fs.close($g);
	`, w, a))
	if err != nil {
		t.Fatal(err)
	}
	if want := "w 6\nw 4\na 5\na 7\nappend seek runtime\n"; out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	got, err := os.ReadFile(w)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abXYef\x00\x00Z" {
		t.Fatalf("write file = %q", got)
	}
	got, err = os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1234567" {
		t.Fatalf("append file = %q", got)
	}
}
