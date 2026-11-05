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

// A program that imports a module profiles successfully (rather than aborting
// with "module imports are not enabled"), and the module's own statements are
// attributed to the module file. Fix A (enable the loader under profile) + Fix B
// (instrument imported modules).
func TestProfileInstrumentsModules(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "greeter.j")
	if err := os.WriteFile(mod, []byte(`export func sumTo(n as int) {
    def total as int init 0;
    def i as int init 0;
    while ($i < $n) {
        $total = $total + $i;
        $i = $i + 1;
    }
    return $total;
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`import "./greeter.j" as g;
def r as int init g.sumTo(200);`), 0o644); err != nil {
		t.Fatal(err)
	}

	var code int
	out := captureStdout(t, func() { code = runProfile([]string{main}) })
	if code != 0 {
		t.Fatalf("profiling a module-using program should exit 0, got %d\n%s", code, out)
	}
	if !strings.Contains(out, "greeter.j:") {
		t.Errorf("module statements should be attributed to greeter.j:\n%s", out)
	}
	if strings.Contains(out, "0 positions") {
		t.Errorf("profile should not be empty:\n%s", out)
	}
}

// A load error that collects nothing writes no profile to stdout and exits
// non-zero - it must not leave a valid-but-empty pprof behind. Fix C.
func TestProfileNoEmptyProfileAfterLoadError(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`import "./does-not-exist.j" as x;
def r as int init x.f();`), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{"--format=pprof", main}) })
	if code == 0 {
		t.Fatalf("a failed import should exit non-zero")
	}
	if len(out) != 0 {
		t.Errorf("no profile bytes should reach stdout after a collected-nothing error, got %d bytes", len(out))
	}
}

// A program that runs some statements before erroring still emits its partial
// profile (the "profile up to the crash" behavior is preserved). Fix C only
// suppresses the empty case.
func TestProfilePartialProfileOnLateError(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`use io;
io.printf("");
def bad as int init 1 // 0;`), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{main}) })
	if code == 0 {
		t.Fatalf("a division-by-zero should exit non-zero")
	}
	if strings.Contains(out, "0 positions") || !strings.Contains(out, "main.j:") {
		t.Errorf("a run that executed statements before erroring should still emit a partial profile:\n%s", out)
	}
}

// A copy made binding a module function's parameter from another module's call
// (here the entry program's) is reported by --allocs, at the parameter's
// declaration; a read-only parameter is borrowed and reported nowhere.
func TestProfileAllocsReportsCrossModuleParamCopy(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "m.j")
	if err := os.WriteFile(mod, []byte(`export func writes(xs as list of int) { $xs[0] = 9; return $xs[0]; }
export func reads(ys as list of int) { return $ys[0]; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`import "./m.j" as m;
def g as list of int init [1, 2, 3];
func main() {
    for (def i in 0..4) { m.writes($g); m.reads($g); }
}
main();`), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{"--allocs", main}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "m.j:1:20") || !strings.Contains(out, "      4 ") {
		t.Errorf("want 4 copies at the writes parameter (m.j:1:20):\n%s", out)
	}
	if strings.Contains(out, "m.j:2:") {
		t.Errorf("the read-only parameter is borrowed and must not be reported:\n%s", out)
	}
}

// --allocs reports the rebuild of a value holding a module's own structs at the
// call site, inward for an argument and outward for a result, and reports
// nothing for a call whose values hold no module struct.
func TestProfileAllocsReportsModuleBoundaryRebuilds(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "m.j")
	if err := os.WriteFile(mod, []byte(`export def struct N { id as string };
export func make(n as int) {
    def out as list of N init [];
    for (def i in 1..$n) { $out[] = N{id: "x"}; }
    return $out;
}
export func count(ns as list of N) { return len($ns); }
export func one() { return N{id: "y"}; }
export func plain(m as map of string to string) { return len($m); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`import "./m.j" as m;
func main() {
    def ns as list of m.N init m.make(50);
    for (def i in 1..3) { def c as int init m.count($ns); }
    def p as int init m.plain({"a": "b"});
    def o as m.N init m.one();
}
main();`), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{"--allocs", main}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "Module-boundary rebuilds") {
		t.Fatalf("allocs table missing the module-boundary section:\n%s", out)
	}
	// A row is COUNT BYTES (a number and a unit) DIR TYPE... POSITION; the type
	// label may hold spaces, so it is everything between DIR and the position.
	rows := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 6 && (f[3] == "in" || f[3] == "out") {
			rows[filepath.Base(f[len(f)-1])] = f[0] + " " + strings.Join(f[3:len(f)-1], " ")
		}
	}
	// `1..3` runs twice: a range excludes its upper bound.
	if got := rows["main.j:3:32"]; got != "1 out list of m.N" {
		t.Errorf("want 1 outward rebuild of the make result at main.j:3:32, got %q:\n%s", got, out)
	}
	if got := rows["main.j:4:45"]; got != "2 in list of m.N" {
		t.Errorf("want 2 inward rebuilds of the count argument at main.j:4:45, got %q:\n%s", got, out)
	}
	// A struct built in the module and returned carries no namespace yet; the
	// row still names it as the caller does.
	if got := rows["main.j:6:23"]; got != "1 out m.N" {
		t.Errorf("want 1 outward rebuild of the one result at main.j:6:23, got %q:\n%s", got, out)
	}
	if len(rows) != 3 {
		t.Errorf("only the make, count and one calls rebuild a value, got %v:\n%s", rows, out)
	}
}

// profileRows parses the statement table into position -> (self, cum).
func profileRows(t *testing.T, out string) map[string][2]time.Duration {
	t.Helper()
	rows := map[string][2]time.Duration{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 4 {
			continue
		}
		self, err1 := time.ParseDuration(f[1])
		cum, err2 := time.ParseDuration(f[2])
		if err1 != nil || err2 != nil {
			continue
		}
		rows[filepath.Base(f[3])] = [2]time.Duration{self, cum}
	}
	return rows
}

// A module call that re-enters the entry program through meta.callMain counts
// the handler's time once: the call site's self time is never negative, and the
// module statement that made the callMain call is not charged the handler's
// time as its own.
func TestProfileCallMainSelfTimeNotNegative(t *testing.T) {
	rows, out := profileCallMain(t, `m.run();`)
	call, mod := rows["main.j:7:1"], rows["m.j:3:5"]
	if call[1] == 0 || mod[1] == 0 {
		t.Fatalf("missing rows main.j:7:1 / m.j:3:5\n%s", out)
	}
	if call[0] < 0 || mod[0] > mod[1]/2 {
		t.Errorf("call self %v, m.j:3:5 self %v of cum %v: the handler is miscounted\n%s",
			call[0], mod[0], mod[1], out)
	}
}

// Parallel spawns whose handlers all re-enter the one host global through
// meta.callMain each keep their own accumulator, so no self time goes negative.
// (A worker's wait for the host lock is real wall-clock time and stays in the
// callMain statement's self time.)
func TestProfileConcurrentCallMainSelfTimeNotNegative(t *testing.T) {
	rows, out := profileCallMain(t, `def ts as list of task of null init [];
for (def w as int init 0; $w < 4; $w = $w + 1) { $ts[] = spawn { m.run(); return; }; }
task.waitAll($ts);`)
	for pos, r := range rows {
		if r[0] < 0 {
			t.Errorf("%s has negative self time %v\n%s", pos, r[0], out)
		}
	}
}

// profileCallMain profiles body after a module whose run() calls back into the
// entry program's work() through meta.callMain; body starts on line 7.
func profileCallMain(t *testing.T, body string) (map[string][2]time.Duration, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.j"), []byte(`use meta;
export func run() {
    meta.callMain("work");
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`use task;
import "./m.j" as m;
func work() {
    def s as int init 0;
    for (def i as int init 0; $i < 20000; $i = $i + 1) { $s = $s + $i; }
}
`+body), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{main}) })
	if code != 0 {
		t.Fatalf("profile exit %d\n%s", code, out)
	}
	return profileRows(t, out), out
}

// A recursive statement's cumulative time is counted once per stack, not once
// per activation, so it never exceeds the cumulative time of the call that
// started the recursion.
func TestProfileRecursiveCumCountedOnce(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.j")
	if err := os.WriteFile(main, []byte(`func rec(n as int) {
    if ($n == 0) { return 0; }
    def s as int init 0;
    for (def i as int init 0; $i < 2000; $i = $i + 1) { $s = $s + $i; }
    return rec($n - 1);
}
rec(20);`), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = runProfile([]string{main}) })
	if code != 0 {
		t.Fatalf("profile exit %d\n%s", code, out)
	}
	rows := profileRows(t, out)
	top, inner := rows["main.j:7:1"], rows["main.j:5:5"]
	if top[1] == 0 || inner[1] == 0 {
		t.Fatalf("missing rows\n%s", out)
	}
	if inner[1] > top[1] {
		t.Errorf("recursive return cum %v exceeds the outer call's cum %v\n%s", inner[1], top[1], out)
	}
}
