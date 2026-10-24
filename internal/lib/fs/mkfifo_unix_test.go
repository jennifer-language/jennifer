// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMkfifoCarriesData: fs.mkfifo makes a named pipe (owner-only by default),
// and a program reads what another writer puts through it.
func TestMkfifoCarriesData(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "p")
	if _, err := runProg(t, fmt.Sprintf(`use fs; fs.mkfifo(%q);`, pipe)); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	info, err := os.Lstat(pipe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("mode %v is not a named pipe", info.Mode())
	}
	if perm := info.Mode().Perm(); perm&^0o600 != 0 {
		t.Errorf("default permissions %o grant more than 0o600", perm)
	}
	go func() {
		// Opening for write blocks until the program opens the read end.
		if f, err := os.OpenFile(pipe, os.O_WRONLY, 0); err == nil {
			f.WriteString("through the pipe")
			f.Close()
		}
	}()
	out, err := runProg(t, fmt.Sprintf(`use io; use fs; io.printf("%%s", fs.readString(%q));`, pipe))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "through the pipe" {
		t.Errorf("read %q, want %q", out, "through the pipe")
	}
}

func TestMkfifoMode(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "p")
	if _, err := runProg(t, fmt.Sprintf(`use fs; fs.mkfifo(%q, 0o640);`, pipe)); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	info, err := os.Lstat(pipe)
	if err != nil {
		t.Fatal(err)
	}
	// The umask can only clear bits, never add them.
	if perm := info.Mode().Perm(); perm&^0o640 != 0 || perm&0o600 != 0o600 {
		t.Errorf("permissions %o, want 0o640 less the umask", perm)
	}
}

func TestMkfifoErrors(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "here")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		fmt.Sprintf(`use fs; fs.mkfifo(%q);`, existing),
		fmt.Sprintf(`use fs; fs.mkfifo(%q, 0o1777);`, filepath.Join(dir, "a")),
		fmt.Sprintf(`use fs; fs.mkfifo(%q, -1);`, filepath.Join(dir, "b")),
		fmt.Sprintf(`use fs; fs.mkfifo(%q, "0600");`, filepath.Join(dir, "c")),
		fmt.Sprintf(`use fs; fs.mkfifo(%q);`, filepath.Join(dir, "missing", "d")),
	} {
		if _, err := runProg(t, src); err == nil {
			t.Errorf("%s: want a catchable error", src)
		}
	}
}
