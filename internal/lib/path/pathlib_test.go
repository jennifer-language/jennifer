// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package pathlib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	pathlib "jennifer-lang.dev/jennifer/internal/lib/path"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg parses + runs a Jennifer program with io + path installed, returning
// captured stdout and the interpreter error.
func runProg(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	pathlib.Install(in)
	// Run before reading buf: Go evaluates return operands left-to-right, so
	// returning buf.String() inline with in.Run would capture the buffer empty.
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// out is a helper: run `use path; use io; io.printf(...)` and return trimmed stdout.
func out(t *testing.T, expr string) string {
	t.Helper()
	s, err := runProg(t, "use path; use io; io.printf(\"%s\", "+expr+");")
	if err != nil {
		t.Fatalf("run error for %s: %v", expr, err)
	}
	return s
}

func TestStringResults(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`path.base("/a/b/c.txt")`, "c.txt"},
		{`path.base("c.txt")`, "c.txt"},
		{`path.base("")`, "."},
		{`path.base("/")`, "/"},
		{`path.base("/a/b/")`, "b"},
		{`path.dir("/a/b/c.txt")`, "/a/b"},
		{`path.dir("c.txt")`, "."},
		{`path.ext("/a/b/c.txt")`, ".txt"},
		{`path.ext("/a/b/c")`, ""},
		{`path.ext("archive.tar.gz")`, ".gz"},
		{`path.stem("/a/b/c.txt")`, "c"},
		{`path.stem("/a/b/c")`, "c"},
		{`path.stem("archive.tar.gz")`, "archive.tar"},
		{`path.clean("a//b/../c")`, "a/c"},
		{`path.clean("")`, "."},
		{`path.clean("./a/./b/")`, "a/b"},
		{`path.join("a", "b", "c")`, "a/b/c"},
		{`path.join("/a/", "b")`, "/a/b"},
		{`path.join("a", "", "b")`, "a/b"},
		{`path.join("only")`, "only"},
	}
	for _, c := range cases {
		if got := out(t, c.expr); got != c.want {
			t.Errorf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
}

func TestIsAbs(t *testing.T) {
	// %t (not %s): isAbs returns a bool.
	abs, err := runProg(t, `use path; use io; io.printf("%t", path.isAbs("/x"));`)
	if err != nil {
		t.Fatal(err)
	}
	if abs != "true" {
		t.Errorf(`path.isAbs("/x") = %q, want "true"`, abs)
	}
	rel, err := runProg(t, `use path; use io; io.printf("%t", path.isAbs("x/y"));`)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "false" {
		t.Errorf(`path.isAbs("x/y") = %q, want "false"`, rel)
	}
}

func TestSplit(t *testing.T) {
	// dir keeps its trailing separator; dir + file reconstructs the input.
	// Separator is "," not "|" - a bare "|" after a verb starts a printf modifier.
	src := `use path; use io;
		def s as list of string init path.split("/a/b/c.txt");
		io.printf("%s,%s", $s[0], $s[1]);`
	got, err := runProg(t, src)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/a/b/,c.txt" {
		t.Errorf("split = %q, want %q", got, "/a/b/,c.txt")
	}
}

func TestArgErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`use path; def x as string init path.base(42);`, "must be string"},
		{`use path; def x as string init path.base("a", "b");`, "expects 1 argument"},
		{`use path; use io; io.printf("%t", path.isAbs(3.14));`, "must be string"},
		{`use path; def x as list of string init path.split(7);`, "must be string"},
	}
	for _, c := range cases {
		_, err := runProg(t, c.src)
		if err == nil {
			t.Errorf("expected error for %q, got nil", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("error for %q = %q, want substring %q", c.src, err.Error(), c.want)
		}
	}
}

func TestJoinNoArgs(t *testing.T) {
	// path.join with zero arguments is an error (needs at least one element).
	_, err := runProg(t, `use path; def x as string init path.join();`)
	if err == nil || !strings.Contains(err.Error(), "at least 1 argument") {
		t.Errorf("path.join() error = %v, want 'at least 1 argument'", err)
	}
}
