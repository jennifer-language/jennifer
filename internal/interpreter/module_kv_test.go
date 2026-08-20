// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	convertlib "jennifer-lang.dev/jennifer/internal/lib/convert"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	kvlib "jennifer-lang.dev/jennifer/internal/lib/kv"
)

// runWithKvModule runs mainSrc with modSrc written as a local module the entry
// program imports. Both interpreters install io + kv, so a kv.Store opened in
// one is exercised across the module boundary in the other.
func runWithKvModule(t *testing.T, mainSrc, modSrc string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.j"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kvmod.j"), []byte(modSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	setup := func(s *interpreter.Interpreter) {
		s.Out = &buf
		iolib.Install(s)
		convertlib.Install(s)
		kvlib.Install(s)
	}
	in := interpreter.New()
	setup(in)
	in.EnableModules(dir, nil, moduleProgram, setup)
	prog, err := moduleProgram(filepath.Join(dir, "main.j"))
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// TestKvStoreCrossesModuleBoundary: a kv.Store handle opened in the entry
// program resolves inside an imported module (the registry is shared across the
// run, not closed over per interpreter). The module both reads a key the entry
// program set and writes one the entry program then reads back.
func TestKvStoreCrossesModuleBoundary(t *testing.T) {
	mod := `use kv;
export func peek(s as kv.Store, key as string) {
    return kv.get($s, $key);
}
export func poke(s as kv.Store, key as string, val as string) {
    kv.set($s, $key, $val, 0);
    return;
}`
	main := `use io;
use kv;
import "./kvmod.j" as m;
def s as kv.Store init kv.open();
kv.set($s, "fromMain", "hello", 0);
io.printf("moduleReads=%s\n", m.peek($s, "fromMain"));
m.poke($s, "fromModule", "world");
io.printf("mainReads=%s\n", kv.get($s, "fromModule"));`
	out, err := runWithKvModule(t, main, mod)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "moduleReads=hello") {
		t.Errorf("module could not read a store opened in the entry program: %q", out)
	}
	if !strings.Contains(out, "mainReads=world") {
		t.Errorf("entry program could not read a write the module made: %q", out)
	}
}
