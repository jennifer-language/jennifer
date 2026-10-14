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
// running must report that error immediately, not after the exit-time task scan
// (which blocks on every in-flight task). Otherwise a script whose worker loops
// forever - a server's accept loop - hangs with the explanation never printed.
// The task here sleeps for a second, so the test can observe the error on
// stderr while runFile is still blocked on it, then let it finish cleanly.
func TestFatalErrorPrintsBeforeTaskScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fatal.j")
	src := `use time;
use task;
def worker as task of null init spawn {
    time.sleep(time.fromMilliseconds(1500));
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
	case <-done:
		t.Fatal("runFile returned before the spawned task finished; the test no longer exercises the scan")
	default:
	}

	if code := <-done; code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	w.Close()
}
