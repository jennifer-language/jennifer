// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// copiesTable is allocsTable with the task library installed.
func copiesTable(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	col := profile.NewCollector(profile.ModeAllocs, 0)
	in.SetProfiler(col, false, false, true)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out bytes.Buffer
	col.Table(&out)
	return out.String()
}

// eagerRowAt returns the eager-copy row of an --allocs table whose position is
// on line, or "" when the section has none.
func eagerRowAt(out string, line int) string {
	sec := out
	if k := strings.Index(sec, "Spawn-frame"); k >= 0 {
		sec = sec[:k]
	}
	want := fmt.Sprintf(":%d:", line)
	for _, row := range strings.Split(sec, "\n") {
		if strings.Contains(row, want) {
			return row
		}
	}
	return ""
}

// Every site where value semantics store a copy is reported by --allocs, at
// the position where the copy happens (M29.53). Each case names the line of
// the copy and a substring its row must carry (type or count).
func TestAllocsReportsEveryCopySite(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
		want string
	}{
		{"struct literal field", `
def struct Cur { names as list of string, at as int };
func step(c as Cur) { return Cur{names: $c.names, at: $c.at + 1}; }
def names as list of string;
for (def k in 0..2000) { $names[] = "n"; }
def c as Cur init Cur{names: $names, at: 0};
for (def k in 0..5) { $c = step($c); }
`, 3, "list of string"},
		{"list literal element", `
def xs as list of int init [1, 2, 3];
def ys as list of list of int init [$xs, $xs];
`, 3, "list of int"},
		{"map literal value", `
def xs as list of int init [1, 2, 3];
def m as map of string to list of int init {"a": $xs};
`, 3, "list of int"},
		{"enum literal payload", `
def enum Box { Full { xs as list of int }, Empty };
def xs as list of int init [1, 2, 3];
def b as Box init Box.Full{xs: $xs};
`, 4, "list of int"},
		{"field write", `
def struct P { xs as list of int };
def xs as list of int init [1, 2, 3];
def p as P;
$p.xs = $xs;
`, 5, "list of int"},
		{"index write", `
def xs as list of int init [1, 2, 3];
def ys as list of list of int init [[0]];
$ys[0] = $xs;
`, 4, "list of int"},
		{"map write", `
def xs as list of int init [1, 2, 3];
def m as map of string to list of int;
$m["k"] = $xs;
`, 4, "list of int"},
		{"append", `
def xs as list of int init [1, 2, 3];
def ys as list of list of int;
$ys[] = $xs;
`, 4, "list of int"},
		{"throw snapshot", `
def struct Oops { kind as string, message as string, xs as list of int };
def o as Oops init Oops{kind: "x", message: "m", xs: [1, 2]};
try { throw $o; } catch (e) { }
`, 4, "Oops"},
		{"defer argument", `
use io;
def xs as list of int init [1, 2, 3];
func f(ys as list of int) { defer show($ys); return 0; }
func show(zs as list of int) { io.printf(""); }
f($xs);
`, 4, "list of int"},
		{"catch binding", `
def struct Oops { kind as string, message as string, xs as list of int };
try {
    throw Oops{kind: "x", message: "m", xs: [1, 2]};
} catch (e) {
}
`, 5, "Oops"},
		{"list slice", `
def xs as list of list of int init [[1], [2], [3]];
def ys as list of list of int init $xs[0..2];
`, 3, "list of list of int"},
		{"bytes slice", `
def b as bytes;
for (def k in 0..5) { $b[] = 65; }
def c as bytes init $b[1..3];
`, 4, "bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := copiesTable(t, tc.src)
			row := eagerRowAt(out, tc.line)
			if row == "" || !strings.Contains(row, tc.want) {
				t.Fatalf("want an eager-copy row on line %d with %q:\n%s", tc.line, tc.want, out)
			}
		})
	}
}

// The repro from M29.53: a changed copy built with a literal from a parameter's
// field reports one copy per call, sized by the 2,000-string list.
func TestAllocsLiteralCopyCountsAndBytes(t *testing.T) {
	out := copiesTable(t, `
def struct Cur { names as list of string, at as int };
func step(c as Cur) { return Cur{names: $c.names, at: $c.at + 1}; }
def names as list of string;
for (def k in 0..2000) { $names[] = "n"; }
def c as Cur init Cur{names: $names, at: 0};
for (def k in 0..10) { $c = step($c); }
`)
	row := eagerRowAt(out, 3)
	if !strings.Contains(row, "     10 ") {
		t.Fatalf("want 10 copies at the literal field:\n%s", out)
	}
	if !strings.Contains(row, "MiB") {
		t.Fatalf("ten copies of a 2,000-element list should be MiB-sized:\n%s", out)
	}
}

// The environment snapshot a spawn takes is reported as a spawn-frame copy.
func TestAllocsReportsSpawnSnapshot(t *testing.T) {
	out := copiesTable(t, `
use task;
def xs as list of int init [1, 2, 3];
def t as task of int init spawn { return len($xs); };
def n as int init task.wait($t);
`)
	sec := out[strings.Index(out, "Spawn-frame"):]
	if !strings.Contains(strings.SplitN(sec, "Module-boundary", 2)[0], ":4:") {
		t.Fatalf("want a spawn-frame copy at line 4:\n%s", out)
	}
}
