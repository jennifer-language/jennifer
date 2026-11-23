// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix && !tinygo

package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// profileBinary builds the jennifer binary for a signal test, which needs a
// process of its own to signal.
func profileBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "jennifer")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// startProfiled runs `jennifer profile --out OUT prog` and waits until the
// program prints "ready" on its stdout (with --out it keeps its stdout).
func startProfiled(t *testing.T, src string) (*exec.Cmd, string) {
	t.Helper()
	bin := profileBinary(t)
	dir := t.TempDir()
	prog := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "prof.txt")
	cmd := exec.Command(bin, "profile", "--out", out, prog)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "ready" {
				ready <- true
				break
			}
		}
		for sc.Scan() {
		}
	}()
	select {
	case <-ready:
	case <-time.After(20 * time.Second):
		t.Fatal("program never printed ready")
	}
	return cmd, out
}

// exitCode waits for cmd and returns its exit status.
func exitCode(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	case <-time.After(20 * time.Second):
		t.Fatal("profiled program did not exit")
		return -1
	}
}

// statementRows maps each row's POSITION of a statement table to its HITS.
func statementRows(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]int{}
	in := false
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == "HITS" {
			in = true
			continue
		}
		if in && len(f) == 0 {
			break
		}
		if in && len(f) >= 6 {
			hits, err := strconv.Atoi(f[0])
			if err != nil {
				t.Fatalf("bad row %q", line)
			}
			// FUNCTION may be "<top level>" (two fields): POSITION is the
			// first field holding a .j position.
			for _, x := range f[4:] {
				if strings.Contains(x, ".j:") {
					rows[x] = hits
					break
				}
			}
		}
	}
	return rows
}

// A server loop stopped with SIGINT writes a complete report and exits 130.
// The main goroutine is blocked in httpd.accept, so the report is written
// after the stop grace, without waiting for a checkpoint.
func TestProfileSigintServerLoopWritesReport(t *testing.T) {
	cmd, out := startProfiled(t, `use httpd;
use io;
func handle(r as httpd.Request) { httpd.respond($r, 200, "ok"); }
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
io.printf("ready\n");
while (true) {
    handle(httpd.accept($srv));
}
`)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(t, cmd); code != 130 {
		t.Fatalf("exit code %d, want 130", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no report written: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "Jennifer statement profile") || !strings.Contains(s, "statement executions") {
		t.Fatalf("incomplete report:\n%s", s)
	}
	if !strings.Contains(s, "prog.j:4:") {
		t.Fatalf("report misses the statements that ran:\n%s", s)
	}
}

// A compute loop stopped with SIGINT unwinds at its next loop checkpoint, so
// the loop statement itself (still running when the signal came) is in the
// report; SIGUSR1 twice writes two snapshots, the second a superset.
func TestProfileSigusr1SnapshotsAndSigint(t *testing.T) {
	cmd, out := startProfiled(t, `use io;
use time;
func work(n as int) {
    def t as int init 0;
    for (def k in 0..$n) { $t = $t + $k; }
    return $t;
}
def total as int init 0;
io.printf("ready\n");
while (true) {
    $total = $total + work(100);
    time.sleep(time.fromMilliseconds(2));
}
`)
	for n := 1; n <= 2; n++ {
		time.Sleep(200 * time.Millisecond)
		if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
			t.Fatal(err)
		}
		snap := out + "." + strconv.Itoa(n)
		deadline := time.Now().Add(10 * time.Second)
		for {
			if data, err := os.ReadFile(snap); err == nil && strings.Contains(string(data), "statement executions") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("snapshot %s not written", snap)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(t, cmd); code != 130 {
		t.Fatalf("exit code %d, want 130", code)
	}
	first, second, final := statementRows(t, out+".1"), statementRows(t, out+".2"), statementRows(t, out)
	if len(first) == 0 {
		t.Fatal("first snapshot has no rows")
	}
	for pos, hits := range first {
		if second[pos] < hits {
			t.Errorf("second snapshot is not a superset: %s has %d hits, was %d", pos, second[pos], hits)
		}
	}
	// The while loop (line 10) was running at every snapshot; it finishes its
	// record only when SIGINT unwinds it.
	found := false
	for pos := range final {
		if strings.HasSuffix(pos, "prog.j:10:1") {
			found = true
		}
	}
	if !found {
		t.Errorf("final report misses the interrupted loop statement: %v", final)
	}
}

// A program that catches SIGINT itself keeps it: it is not cut short, ends
// on its own terms, and the report is written at its exit.
func TestProfileProgramCatchingSigintKeepsIt(t *testing.T) {
	cmd, out := startProfiled(t, `use io;
use os;
use time;
os.catchSignal("int");
io.printf("ready\n");
while (not os.gotSignal("int")) {
    time.sleep(time.fromMilliseconds(5));
}
io.printf("caught\n");
time.sleep(time.fromMilliseconds(1500));
exit 3;
`)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(t, cmd); code != 3 {
		t.Fatalf("exit code %d, want the program's own 3", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no report written: %v", err)
	}
	if !strings.Contains(string(data), "prog.j:11:1") {
		t.Fatalf("the program should have run to its exit statement:\n%s", data)
	}
}
