// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// M29.21: text read from a file that is not valid UTF-8 is kind "fs"; an
// operating-system failure and misuse stay "runtime".
func TestErrorKinds(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("ab\xffc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		try { fs.readString(%[1]q); } catch (e) { io.printf("readString %%s\n", $e.kind); }
		try { fs.readSecretString(%[1]q); } catch (e) { io.printf("readSecretString %%s\n", $e.kind); }
		def f as fs.File init fs.open(%[1]q, "read");
		try { fs.readLine($f); } catch (e) { io.printf("readLine %%s\n", $e.kind); }
		fs.close($f);
		def g as fs.File init fs.open(%[1]q, "read");
		try { fs.readChars($g, 4); } catch (e) { io.printf("readChars %%s\n", $e.kind); }
		fs.close($g);
		try { fs.readString(%[2]q); } catch (e) { io.printf("missing %%s\n", $e.kind); }
		try { fs.open(%[1]q, "rw"); } catch (e) { io.printf("mode %%s\n", $e.kind); }
	`, bad, filepath.Join(dir, "missing")))
	if err != nil {
		t.Fatal(err)
	}
	want := "readString fs\nreadSecretString fs\nreadLine fs\nreadChars fs\nmissing runtime\nmode runtime\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}
