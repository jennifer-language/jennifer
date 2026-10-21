// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScreenModule drives the screen module through the real import path: build
// a cell buffer, draw into it, and check the output-only layer (get / box /
// text / diff) plus the pure key decoder. A failed assertion throws in the .j
// program and fails loadForTest. The term-backed event loop needs a TTY and is
// not exercised here.
func TestScreenModule(t *testing.T) {
	screenMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "screen.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as screen;

# Buffer drawing: box corners + text + clip.
def base as screen.Buffer init screen.newScreen(3, 6);
def drawn as screen.Buffer init screen.text(screen.box($base, 0, 0, 6, 3), 1, 1, "hi");
testing.assertEqual(screen.get($drawn, 0, 0), "┌");     # box corner
testing.assertEqual(screen.get($drawn, 1, 1), "h");
testing.assertEqual(screen.get($drawn, 2, 1), "i");
testing.assertEqual(screen.get(screen.set($base, 99, 99, "Z"), 0, 0), " ");   # clip

# The diff loop: identical is empty, a single change is not.
testing.assertEqual(screen.diff($drawn, $drawn), "");
testing.assertTrue(len(screen.diff($base, screen.set($base, 2, 1, "X"))) > 0);

# Pure key decoder across the main classes.
testing.assertEqual(screen.decodeKey([65]).name, "char");
testing.assertEqual(screen.decodeKey([65]).char, "A");
testing.assertEqual(screen.decodeKey([27, 91, 65]).name, "up");
testing.assertEqual(screen.decodeKey([27, 91, 51, 126]).name, "delete");
testing.assertEqual(screen.decodeKey([3]).name, "ctrl-c");
testing.assertEqual(screen.decodeKey([13]).name, "enter");
testing.assertEqual(screen.decodeKey([]).name, "eof");`, screenMod)
	progPath := filepath.Join(dir, "screen.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("screen program failed with code %d", code)
	}
}

// TestScreenNextKeyLoneEscape drives screen.nextKey over a real stdin pipe: a
// lone Escape (nothing follows within the wait) arrives as "escape" without
// swallowing the key pressed after it, and a complete sequence still decodes
// whole.
func TestScreenNextKeyLoneEscape(t *testing.T) {
	screenMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "screen.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as screen;

testing.assertEqual(screen.nextKey().name, "escape");
def k as screen.Key init screen.nextKey();
testing.assertEqual($k.name, "char");
testing.assertEqual($k.char, "x");
testing.assertEqual(screen.nextKey().name, "up");
testing.assertEqual(screen.nextKey().name, "eof");
`, screenMod)
	progPath := filepath.Join(dir, "keys.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin; r.Close() }()
	go func() {
		defer w.Close()
		_, _ = w.Write([]byte{27})
		time.Sleep(300 * time.Millisecond) // well past the Escape wait
		_, _ = w.Write([]byte("x\x1b[A"))
	}()
	if code := runFile(progPath, nil, ""); code != 0 {
		t.Fatalf("key program exited %d", code)
	}
}
