// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A shebang tool symlinked into a bin directory (`/usr/local/bin/tool ->
// /opt/tool/tool.j`) resolves its local import and include against the real
// file's directory, not the symlink's - the entry path is canonicalized the
// same way every imported module already is.
func TestRunResolvesSymlinkedEntryImports(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "helper.j"),
		[]byte(`export func tag() { return "real-dir"; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "data.j"),
		[]byte(`def const MARK as string init "included-ok";`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A real shebang tool: no .j on the installed name, so the `#!` line is what
	// makes it runnable (sourceExtensionOK), and it imports/includes siblings.
	if err := os.WriteFile(filepath.Join(real, "tool.j"), []byte(`#!/usr/bin/env -S jennifer run
import "./helper.j" as h;
include "./data.j";
use io;
io.printf("%s %s", h.tag(), MARK);`), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	link := filepath.Join(bin, "tool") // installed name, no extension
	if err := os.Symlink(filepath.Join(real, "tool.j"), link); err != nil {
		t.Skipf("cannot create symlink (unprivileged Windows?): %v", err)
	}

	// Run from a cwd unrelated to either directory, through the symlink.
	restore := chdirTemp(t)
	defer restore()

	var code int
	out := captureStdout(t, func() { code = runFile(link, nil, "") })
	if code != 0 {
		t.Fatalf("a symlinked entry should resolve its siblings, got exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "real-dir") || !strings.Contains(out, "included-ok") {
		t.Errorf("import/include did not resolve against the real directory:\n%s", out)
	}
}

// chdirTemp switches to a fresh temp directory and returns a restore func, so
// the test proves resolution does not depend on the working directory.
func chdirTemp(t *testing.T) func() {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return func() { os.Chdir(prev) }
}
