// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
