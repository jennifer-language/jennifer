// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeProg writes src as main.j in a fresh directory and returns its path.
func writeProg(t *testing.T, src string) string {
	t.Helper()
	main := filepath.Join(t.TempDir(), "main.j")
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return main
}

// profileOut runs `jennifer profile args... main` and returns the table.
func profileOut(t *testing.T, main string, args ...string) string {
	t.Helper()
	var code int
	out := captureStdout(t, func() { code = runProfile(append(args, main)) })
	if code != 0 {
		t.Fatalf("profile exit %d\n%s", code, out)
	}
	return out
}

// rowAt returns the fields of the table row whose position ends in pos.
func rowAt(out, pos string) []string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if p := positionField(f); p >= 0 && strings.HasSuffix(f[p], pos) {
			return f
		}
	}
	return nil
}

// Each statement row names its function (or <top level>) and shows the
// source line; --by=function gives one row per function with its calls; the
// header states the wall time and the tasks profiled (M29.31, M29.32).
func TestProfileRowsNameFunctionAndSource(t *testing.T) {
	main := writeProg(t, `use task;
func add(a as int, b as int) {
    def s as int init $a + $b;
    return $s;
}
def t as int init 0;
for (def k in 0..3) { $t = add($t, $k); }
def w as task of int init spawn { return add(1, 2); };
task.wait($w);
`)
	out := profileOut(t, main)
	if !strings.Contains(out, "wall, 2 tasks (main + 1 spawned)") {
		t.Errorf("header should give the wall time and the tasks:\n%s", out)
	}
	row := strings.Join(rowAt(out, "main.j:3:5"), " ")
	if !strings.Contains(row, " add ") || !strings.HasSuffix(row, "def s as int init $a + $b;") {
		t.Errorf("row 3:5 should name add and show its source, got %q\n%s", row, out)
	}
	row = strings.Join(rowAt(out, "main.j:6:1"), " ")
	if !strings.Contains(row, " <top level> ") || !strings.HasSuffix(row, "def t as int init 0;") {
		t.Errorf("row 6:1 should be <top level>, got %q\n%s", row, out)
	}

	out = profileOut(t, main, "--by=function")
	f := rowAt(out, "main.j:2:1")
	if len(f) < 6 || f[0] != "4" || f[4] != "add" {
		t.Errorf("--by=function: want add called 4 times, got %q\n%s", f, out)
	}
	if !strings.Contains(out, "  CALLS ") {
		t.Errorf("--by=function header:\n%s", out)
	}

	out = profileOut(t, main, "--top", "2")
	if !strings.Contains(out, "(showing 2 of ") {
		t.Errorf("--top 2 should cut the table:\n%s", out)
	}
	out = profileOut(t, main, "--focus", "^add$")
	if rowAt(out, "main.j:6:1") != nil || rowAt(out, "main.j:3:5") == nil {
		t.Errorf("--focus ^add$ should keep only add's rows:\n%s", out)
	}
}

// A program that sleeps 1 s in a loop shows that second as WAIT, not SELF.
func TestProfileSleepIsWaitNotSelf(t *testing.T) {
	main := writeProg(t, `use time;
for (def k in 0..10) {
    time.sleep(time.fromMilliseconds(100));
}
`)
	out := profileOut(t, main)
	f := rowAt(out, "main.j:3:5")
	if len(f) < 4 {
		t.Fatalf("no row for the sleep:\n%s", out)
	}
	self, err1 := time.ParseDuration(f[1])
	wait, err2 := time.ParseDuration(f[2])
	if err1 != nil || err2 != nil {
		t.Fatalf("bad row %q", f)
	}
	if wait < 900*time.Millisecond || self > 100*time.Millisecond {
		t.Errorf("want ~1s of wait and little self, got self %v wait %v\n%s", self, wait, out)
	}
	// --by=function carries the wait too.
	out = profileOut(t, main, "--by=function")
	f = nil
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "<top level>") {
			f = strings.Fields(line)
			break
		}
	}
	if len(f) < 4 {
		t.Fatalf("no top-level row:\n%s", out)
	}
	if wait, err := time.ParseDuration(f[2]); err != nil || wait < 900*time.Millisecond {
		t.Errorf("--by=function should show the wait, got %q\n%s", f, out)
	}
}

// --out writes the report to the file; stdout stays the program's.
func TestProfileOutWritesFile(t *testing.T) {
	main := writeProg(t, `use io;
io.printf("hello\n");
`)
	report := filepath.Join(t.TempDir(), "p.txt")
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{"--out", report, main}) })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out != "hello\n" {
		t.Errorf("stdout should be the program's own output, got %q", out)
	}
	data, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(data), "Jennifer statement profile") {
		t.Fatalf("report not written: %v\n%s", err, data)
	}
}

// Flags that shape the table are rejected with another format or --allocs.
func TestProfileTableFlagsRejected(t *testing.T) {
	for _, args := range [][]string{
		{"--format=pprof", "--top", "3", "x.j"},
		{"--format=trace", "--by=function", "x.j"},
		{"--allocs", "--by=function", "x.j"},
		{"--by=line", "x.j"},
		{"--top", "0", "x.j"},
		{"--focus", "(", "x.j"},
		{"--out"},
	} {
		if _, ok := parseProfileArgs(args); ok {
			t.Errorf("%v should be rejected", args)
		}
	}
	opts, ok := parseProfileArgs([]string{"--out", "p.txt", "--by", "function", "--top=5", "--focus=web", "x.j", "a"})
	if !ok || opts.out != "p.txt" || opts.by != "function" || opts.top != 5 || opts.focus == nil ||
		opts.path != "x.j" || len(opts.progArgs) != 1 {
		t.Errorf("flags not parsed: %+v", opts)
	}
}
