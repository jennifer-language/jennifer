// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// TestForEachSnapshotIsDeep proves the iteration variable reflects a deep
// snapshot taken at loop entry: a nested in-place write to the original binding
// during the loop does not leak into a later iteration's value.
func TestForEachSnapshotIsDeep(t *testing.T) {
	out, err := run(t, `use io;
def grid as list of list of int init [[1], [2], [3]];
for (def row in $grid) {
    io.printf("%d\n", $row[0]);
    $grid[1][0] = 99;
    $grid[2] = [77];
}
def struct P { age as int };
def ps as list of P init [P{age: 1}, P{age: 2}, P{age: 3}];
for (def p in $ps) { io.printf("a%d\n", $p.age); $ps[1].age = 88; }`)
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n2\n3\na1\na2\na3\n"
	if out != want {
		t.Errorf("for-each snapshot leaked an in-loop mutation:\n got %q\nwant %q", out, want)
	}
}

// TestStringSliceUnicode proves slicing is rune-indexed and correct for ASCII
// (the direct-byte fast path) and multi-byte UTF-8 (the byte-offset walk).
func TestStringSliceUnicode(t *testing.T) {
	out, err := run(t, `use io;
def s as string init "hello";
io.printf("%s %s %s\n", $s[0..2], $s[2..], $s[..3]);
def u as string init "aébc";
io.printf("%s %s %s\n", $u[0..1], $u[1..2], $u[1..4]);`)
	if err != nil {
		t.Fatal(err)
	}
	want := "he llo hel\na é ébc\n"
	if out != want {
		t.Errorf("string slicing wrong:\n got %q\nwant %q", out, want)
	}
}

// TestDisplayNestedUnchanged pins the display form of nested containers (the
// one-builder rewrite must produce byte-identical output): strings quote inside
// containers, scalars do not, nesting recurses.
func TestDisplayNestedUnchanged(t *testing.T) {
	out, err := run(t, `use io;
def v as list of map of string to list of int init [{"a": [1, 2]}, {"b": [3]}];
io.printf("%v\n", $v);
def s as list of string init ["x", "y"];
io.printf("%v\n", $s);`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"a": [1, 2]}, {"b": [3]}]` + "\n" + `["x", "y"]` + "\n"
	if out != want {
		t.Errorf("display changed:\n got %q\nwant %q", out, want)
	}
	if strings.Count(out, "\n") != 2 {
		t.Errorf("unexpected line count in %q", out)
	}
}
