// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
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

	done := make(chan int, 1)
	go func() { done <- runFile(path, nil, "") }()

	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	deadline := time.After(1 * time.Second)
	for seen := false; !seen; {
		select {
		case line := <-lines:
			if strings.Contains(line, "division by zero") {
				seen = true
			}
		case code := <-done:
			t.Fatalf("runFile returned (code %d) before the fatal error reached stderr", code)
		case <-deadline:
			t.Fatal("the fatal error was not printed while the spawned task was still running")
		}
	}
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("exit code %d, want 1", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runFile waited for the still-running task after a fatal error")
	}
	w.Close()
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
