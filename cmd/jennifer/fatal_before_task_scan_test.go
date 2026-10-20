// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A program that dies with an uncaught error while a spawned task is still
// running reports the error at once and exits without waiting for the task: an
// early end abandons tasks still in flight, so a script whose worker loops
// forever - a server's accept loop - neither hangs nor hides the explanation.
// The task here sleeps far longer than the test waits.
func TestFatalErrorPrintsBeforeTaskScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fatal.j")
	src := `use time;
use task;
def worker as task of null init spawn {
    time.sleep(time.fromMilliseconds(10000));
    return;
};
def x as int init 1 // 0;
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = oldErr }()

	// Drain stderr concurrently so a full pipe can never block runFile.
	output := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		output <- string(b)
	}()

	done := make(chan int, 1)
	go func() { done <- runFile(path, nil, "") }()

	// Returning well inside the task's 10 s sleep is the "does not wait" half;
	// the error then being on stderr is the "reports it" half. Ordering the two
	// against each other would race: runFile may return before the reader has
	// seen the line it already wrote.
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("exit code %d, want 1", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runFile waited for the still-running task after a fatal error")
	}
	w.Close()
	if got := <-output; !strings.Contains(got, "division by zero") {
		t.Errorf("stderr lacks the fatal error:\n%s", got)
	}
}

// `exit` ends the program without waiting for a task still running - here one
// blocked in net.accept, which no cancellation reaches - and keeps its code.
func TestExitDoesNotWaitForRunningTask(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exit.j")
	src := `use net;
def l as net.Listener init net.listen("127.0.0.1:0");
def t as task of null init spawn {
    while (true) {
        net.accept($l);
    }
    return;
};
exit 3;
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() { done <- runFile(path, nil, "") }()
	select {
	case code := <-done:
		if code != 3 {
			t.Errorf("exit code %d, want 3", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exit waited for a task blocked in net.accept")
	}
}
