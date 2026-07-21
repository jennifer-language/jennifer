// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package fslib_test

import (
	"strings"
	"testing"
)

// TestMakeTempFileAndDir covers the happy path: a temp file and a temp dir are
// created (atomically, so they exist), are of the right kind, and are unique.
func TestMakeTempFileAndDir(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use fs;
		def f as string init fs.makeTempFile();
		def d as string init fs.makeTempDir();
		io.printf("file=%t dir=%t distinct=%t\n",
			fs.isFile($f), fs.isDir($d), not ($f == $d));
		def g as string init fs.makeTempFile();
		io.printf("unique=%t\n", not ($f == $g));
		fs.remove($f); fs.remove($g); fs.removeAll($d);
		io.printf("cleaned=%t\n", not fs.exists($f) and not fs.exists($d));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "file=true dir=true distinct=true\nunique=true\ncleaned=true\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestMakeTempPrefixSuffix checks that prefix and suffix bracket the random
// component (a real extension via suffix), and that a custom dir is honoured.
func TestMakeTempPrefixSuffix(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use fs;
		def d as string init fs.makeTempDir("", "unit-");
		def f as string init fs.makeTempFile($d, "report-", ".json");
		io.printf("%s\n%s\n", $d, $f);
		fs.removeAll($d);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two paths, got %q", out)
	}
	d, f := lines[0], lines[1]
	if !strings.Contains(d, "unit-") {
		t.Errorf("dir %q missing prefix", d)
	}
	if !strings.HasPrefix(f, d) {
		t.Errorf("file %q not under custom dir %q", f, d)
	}
	if !strings.Contains(f, "report-") {
		t.Errorf("file %q missing prefix", f)
	}
	if !strings.HasSuffix(f, ".json") {
		t.Errorf("file %q missing .json suffix", f)
	}
}

// TestMakeTempRejectsSeparator ensures a prefix / suffix cannot inject a path
// separator (which would escape the target directory).
func TestMakeTempRejectsSeparator(t *testing.T) {
	for _, arg := range []string{
		`fs.makeTempFile("", "../evil", "")`,
		`fs.makeTempFile("", "", "/x")`,
		`fs.makeTempDir("", "a/b")`,
	} {
		_, err := runProg(t, "use fs;\ndef bad as string init "+arg+";")
		if err == nil {
			t.Errorf("%s: expected a rejection", arg)
			continue
		}
		if !strings.Contains(err.Error(), "path separator") {
			t.Errorf("%s: error = %v, want a path-separator message", arg, err)
		}
	}
}

// TestMakeTempMissingParent proves makeTempDir does not create parents
// (mkdir, not mkdir -p): a missing dir is an error, not a silent mkdir -p.
func TestMakeTempMissingParent(t *testing.T) {
	_, err := runProg(t, `
		use fs;
		def x as string init fs.makeTempDir("/no/such/parent/here", "t-");
	`)
	if err == nil {
		t.Fatal("expected an error for a missing parent directory")
	}
	if !strings.Contains(err.Error(), "makeTempDir") {
		t.Errorf("error = %v, want an fs.makeTempDir error", err)
	}
}

// TestMakeTempArity rejects too many arguments and a non-string argument.
func TestMakeTempArity(t *testing.T) {
	cases := []string{
		`fs.makeTempDir("", "p", "extra")`, // makeTempDir takes at most 2
		`fs.makeTempFile("", "p", ".x", "y")`,
		`fs.makeTempFile(42)`,
	}
	for _, c := range cases {
		if _, err := runProg(t, "use fs;\ndef x as string init "+c+";"); err == nil {
			t.Errorf("%s: expected an arity/type error", c)
		}
	}
}

// TestMakeTempCatchable confirms the error is an ordinary catchable runtime
// error (try / catch), not a panic.
func TestMakeTempCatchable(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use fs;
		try {
			def bad as string init fs.makeTempDir("/no/such/parent", "t-");
		} catch (e) {
			io.printf("caught\n");
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "caught\n" {
		t.Errorf("got %q, want the error to be caught", out)
	}
}
