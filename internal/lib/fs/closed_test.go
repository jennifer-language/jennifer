// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib_test

import (
	"fmt"
	"path/filepath"
	"testing"
)

// M29.46: any use of a closed fs.File or fs.Watcher throws kind "closed"; a
// second close does nothing.
func TestClosedHandles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.txt")
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		def f as fs.File init fs.open(%[1]q, "write");
		fs.close($f);
		fs.close($f);
		try { fs.writeString($f, "x"); } catch (e) { io.printf("writeString %%s\n", $e.kind); }
		try { fs.sync($f); } catch (e) { io.printf("sync %%s\n", $e.kind); }
		try { fs.eof($f); } catch (e) { io.printf("eof %%s\n", $e.kind); }
		def g as fs.File init fs.open(%[1]q, "read");
		fs.close($g);
		try { fs.readLine($g); } catch (e) { io.printf("readLine %%s\n", $e.kind); }
		try { fs.readBytes($g, 1); } catch (e) { io.printf("readBytes %%s\n", $e.kind); }
		def w as fs.Watcher init fs.watch(%[2]q, 50);
		fs.close($w);
		fs.close($w);
		try { fs.hasEvent($w); } catch (e) { io.printf("hasEvent %%s\n", $e.kind); }
		try { fs.next($w); } catch (e) { io.printf("next %%s\n", $e.kind); }
	`, path, dir))
	if err != nil {
		t.Fatal(err)
	}
	want := "writeString closed\nsync closed\neof closed\nreadLine closed\nreadBytes closed\nhasEvent closed\nnext closed\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}
