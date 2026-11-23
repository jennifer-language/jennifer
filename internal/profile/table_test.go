// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package profile_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/profile"
)

const goldenProg = `func add(a as int, b as int) {
    def s as int init $a + $b;
    return $s;
}
def t as int init 0;
for (def k in 0..3) {
	$t = add($t, $k);
}
`

const goldenLib = `export func pause() {
    time.sleep(time.fromMilliseconds(250));
}
`

// goldenCollector is a statement profile of goldenProg, which calls add three
// times, and of a system module whose pause() waited twice, with fixed times.
func goldenCollector(t *testing.T, v profile.View) *profile.Collector {
	t.Helper()
	dir := t.TempDir()
	sys := filepath.Join(dir, "sys")
	if err := os.Mkdir(sys, 0o755); err != nil {
		t.Fatal(err)
	}
	prog, lib := filepath.Join(dir, "prog.j"), filepath.Join(sys, "lib.j")
	if err := os.WriteFile(prog, []byte(goldenProg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib, []byte(goldenLib), 0o644); err != nil {
		t.Fatal(err)
	}
	c := profile.NewCollector(profile.ModeStatement, 0)
	c.DeclareFunc("add", prog, 1, 1, [][2]int{{2, 5}, {3, 5}})
	c.DeclareFunc("", prog, 5, 1, nil)
	c.DeclareFunc("", prog, 6, 1, nil)
	c.DeclareFunc("pause", lib, 1, 8, [][2]int{{2, 5}})
	ms := time.Millisecond
	c.RecordStmt(prog, 5, 1, time.Microsecond, 0, time.Microsecond)
	c.RecordStmt(prog, 6, 1, ms, 0, 10*ms)
	for k := 0; k < 3; k++ {
		c.RecordStmt(prog, 7, 2, ms, 0, 3*ms)
		c.RecordStmt(prog, 2, 5, ms, 0, ms)
		c.RecordStmt(prog, 3, 5, ms/2, 0, ms/2)
	}
	for k := 0; k < 2; k++ {
		c.RecordStmt(lib, 2, 5, 0, 250*ms, 250*ms)
	}
	c.RecordTask()
	c.RecordTask()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetWall(t0, t0.Add(2*time.Second))
	v.WorkDir, v.SysDir = dir, sys
	c.SetView(v)
	return c
}

func goldenTable(t *testing.T, v profile.View) string {
	t.Helper()
	var b bytes.Buffer
	goldenCollector(t, v).Table(&b)
	return b.String()
}

const goldenHeader = `Jennifer statement profile: 2s wall, 3 tasks (main + 2 spawned)
SELF excludes nested statements and waits; WAIT is time blocked in a builtin that
waits (time.sleep, channel.recv, task.wait, httpd.accept, net reads); SELF, WAIT
and CUM are summed over tasks, so with spawned tasks they can exceed the wall time.

`

// The statement table names each row's function, shows its source line
// trimmed, and shows paths relative to the working directory or as <sys>/.
func TestGoldenStatementTable(t *testing.T) {
	want := goldenHeader + `  HITS   SELF   WAIT    CUM  FUNCTION     POSITION         SOURCE
     2     0s  500ms  500ms  pause        <sys>/lib.j:2:5  time.sleep(time.fromMilliseconds(250));
     1    1ms     0s   10ms  <top level>  prog.j:6:1       for (def k in 0..3) {
     3    3ms     0s    9ms  <top level>  prog.j:7:2       $t = add($t, $k);
     3    3ms     0s    3ms  add          prog.j:2:5       def s as int init $a + $b;
     3  1.5ms     0s  1.5ms  add          prog.j:3:5       return $s;
     1    1µs     0s    1µs  <top level>  prog.j:5:1       def t as int init 0;

6 positions, 13 statement executions, 8.501ms total self time, 500ms total wait (top-of-tree cum 500ms)
`
	if got := goldenTable(t, profile.View{}); got != want {
		t.Fatalf("statement table:\n%s\nwant:\n%s", got, want)
	}
}

// --by=function rolls the rows up per function: calls from the first body
// statement, self and wait summed, cum over the function's own statements.
func TestGoldenFunctionTable(t *testing.T) {
	want := goldenHeader + `  CALLS     SELF   WAIT       CUM  FUNCTION     POSITION
      2       0s  500ms     500ms  pause        <sys>/lib.j:1:8
      1  4.001ms     0s  10.001ms  <top level>  prog.j
      3    4.5ms     0s     4.5ms  add          prog.j:1:1

6 positions, 13 statement executions, 8.501ms total self time, 500ms total wait (top-of-tree cum 500ms)
`
	got := goldenTable(t, profile.View{ByFunction: true})
	if got != want {
		t.Fatalf("function table:\n%s\nwant:\n%s", got, want)
	}
}

// --top keeps the first rows; --focus keeps the rows whose path or function
// matches.
func TestGoldenTopAndFocus(t *testing.T) {
	got := goldenTable(t, profile.View{Top: 2})
	if !strings.Contains(got, "prog.j:6:1") || strings.Contains(got, "prog.j:7:2") ||
		!strings.Contains(got, "(showing 2 of 6 positions)") {
		t.Fatalf("--top 2:\n%s", got)
	}
	got = goldenTable(t, profile.View{Focus: regexp.MustCompile(`^add$`)})
	if !strings.Contains(got, "prog.j:2:5") || !strings.Contains(got, "prog.j:3:5") ||
		strings.Contains(got, "prog.j:7:2") || strings.Contains(got, "lib.j") {
		t.Fatalf("--focus add:\n%s", got)
	}
	got = goldenTable(t, profile.View{Focus: regexp.MustCompile(`<sys>/`)})
	if !strings.Contains(got, "<sys>/lib.j:2:5") || strings.Contains(got, "prog.j:") {
		t.Fatalf("--focus <sys>/:\n%s", got)
	}
}

// The pprof form names each location's function as the table does.
func TestPprofCarriesFunctionNames(t *testing.T) {
	var b bytes.Buffer
	if err := goldenCollector(t, profile.View{}).Pprof(&b); err != nil {
		t.Fatal(err)
	}
	raw := gunzip(t, b.Bytes())
	for _, want := range []string{"add", "pause", "<top level>", "wait"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("pprof string table missing %q", want)
		}
	}
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(gr)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
