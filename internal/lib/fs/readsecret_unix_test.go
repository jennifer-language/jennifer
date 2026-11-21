// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build unix

package fslib_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// secretKindOf runs fs.readSecret on path and returns what the program saw:
// the content on success, `kind: message` when it threw.
func secretKindOf(t *testing.T, path string) string {
	t.Helper()
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		try {
			io.printf("ok %%s", fs.readSecretString(%q));
		} catch (e) {
			io.printf("%%s: %%s", $e.kind, $e.message);
		}
	`, path))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out
}

func writeMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// TestReadSecretOwnerOnly: an owner-only file reads, as bytes and as text,
// and through a symlink, which is checked as the file it points at.
func TestReadSecretOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	writeMode(t, key, "s3cret", 0o600)
	if got := secretKindOf(t, key); got != "ok s3cret" {
		t.Errorf("0600: %q", got)
	}
	writeMode(t, key, "s3cret", 0o400)
	if got := secretKindOf(t, key); got != "ok s3cret" {
		t.Errorf("0400: %q", got)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(key, link); err != nil {
		t.Fatal(err)
	}
	if got := secretKindOf(t, link); got != "ok s3cret" {
		t.Errorf("symlink to 0400: %q", got)
	}
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use fs;
		io.printf("%%d", len(fs.readSecretBytes(%q)));
	`, key))
	if err != nil || out != "6" {
		t.Errorf("readSecret bytes: %q %v", out, err)
	}
}

// TestReadSecretRefusesLoosePermissions: any group or other bit is refused
// with kind "fs", a message naming the mode and the fix, and nothing read.
func TestReadSecretRefusesLoosePermissions(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	for _, mode := range []os.FileMode{0o640, 0o604, 0o620, 0o610, 0o644} {
		writeMode(t, key, "s3cret", mode)
		got := secretKindOf(t, key)
		want := fmt.Sprintf("fs: fs.readSecretString: %s has group or other permissions (mode %04o); run chmod 600 %s", key, mode, key)
		if got != want {
			t.Errorf("%04o: got %q\nwant %q", mode, got, want)
		}
	}
}

// TestReadSecretRefusesNonRegular: a directory and a FIFO are refused, the
// FIFO without waiting for a writer.
func TestReadSecretRefusesNonRegular(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := secretKindOf(t, sub); !strings.HasPrefix(got, "fs: ") || !strings.Contains(got, "is not a regular file") {
		t.Errorf("directory: %q", got)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	if got := secretKindOf(t, fifo); !strings.HasPrefix(got, "fs: ") || !strings.Contains(got, "is not a regular file") {
		t.Errorf("fifo: %q", got)
	}
}

// TestReadSecretCap: a file over 1 MiB is kind "limit", and a missing file
// is an ordinary fs error, not a refusal.
func TestReadSecretCap(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	writeMode(t, big, strings.Repeat("x", 1<<20+1), 0o600)
	if got := secretKindOf(t, big); !strings.HasPrefix(got, "limit: ") {
		t.Errorf("oversized: %q", got)
	}
	exact := filepath.Join(dir, "exact")
	writeMode(t, exact, strings.Repeat("x", 1<<20), 0o600)
	if got := secretKindOf(t, exact); !strings.HasPrefix(got, "ok ") {
		t.Errorf("exactly 1 MiB: %.40q", got)
	}
	if got := secretKindOf(t, filepath.Join(dir, "missing")); strings.HasPrefix(got, "fs: ") || !strings.Contains(got, "missing") {
		t.Errorf("missing: %q", got)
	}
}
