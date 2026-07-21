// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
	mapslib "jennifer-lang.dev/jennifer/internal/lib/maps"
	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
	regexlib "jennifer-lang.dev/jennifer/internal/lib/regex"
)

// runWithAnsiModule runs mainSrc with the real modules/ansi.j on the module
// search path, so `import "ansi.j" as ansi;` resolves the shipped file. It
// installs exactly the libraries ansi.j depends on (os / maps / convert /
// regex) plus io for output.
func runWithAnsiModule(t *testing.T, mainSrc string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.j"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	modulesDir, err := filepath.Abs(filepath.Join("..", "..", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	setup := func(s *interpreter.Interpreter) {
		s.Out = &buf
		iolib.Install(s)
		oslib.Install(s)
		mapslib.Install(s)
		convertlib.Install(s)
		regexlib.Install(s)
	}
	in := interpreter.New()
	setup(in)
	in.EnableModules(dir, []string{modulesDir}, moduleProgram, setup)
	prog, err := moduleProgram(filepath.Join(dir, "main.j"))
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	runErr := in.Run(prog)
	return buf.String(), runErr
}

func TestAnsiModuleWrapsAndStrips(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	out, err := runWithAnsiModule(t, `use io;
import "ansi.j" as ansi;
def r as string init ansi.bold(ansi.red("x"));
io.printf("wrapped=%t nested=%t stripped=%t\n", len($r) > 1, len($r) > len(ansi.red("x")), ansi.strip($r) == "x");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "wrapped=true nested=true stripped=true") {
		t.Errorf("wrap/nest/strip failed: %q", out)
	}
}

func TestAnsiModuleNoColorSuppresses(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out, err := runWithAnsiModule(t, `use io;
import "ansi.j" as ansi;
io.printf("plain=%t\n", ansi.red("x") == "x");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "plain=true") {
		t.Errorf("NO_COLOR should suppress escapes: %q", out)
	}
}

func TestAnsiModuleForceColorOverridesNonTTY(t *testing.T) {
	// Test stdout is not a TTY, so only FORCE_COLOR turns styling on.
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	off, err := runWithAnsiModule(t, `use io;
import "ansi.j" as ansi;
io.printf("%t\n", ansi.red("x") == "x");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(off, "true") {
		t.Errorf("non-TTY without FORCE_COLOR should be plain: %q", off)
	}
	t.Setenv("FORCE_COLOR", "1")
	on, err := runWithAnsiModule(t, `use io;
import "ansi.j" as ansi;
io.printf("%t\n", ansi.red("x") == "x");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(on, "false") {
		t.Errorf("FORCE_COLOR should emit escapes even off a TTY: %q", on)
	}
	// FORCE_COLOR=0 must force OFF (interpret the value, not just its presence):
	// CI commonly sets FORCE_COLOR=0 and must get plain output.
	t.Setenv("FORCE_COLOR", "0")
	zero, err := runWithAnsiModule(t, `use io;
import "ansi.j" as ansi;
io.printf("%t\n", ansi.red("x") == "x");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(zero, "true") {
		t.Errorf("FORCE_COLOR=0 should force plain output: %q", zero)
	}
}

func TestAnsiModuleUnknownColorThrows(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	_, err := runWithAnsiModule(t, `import "ansi.j" as ansi;
def r as string init ansi.color("x", "chartreuse");`)
	if err == nil {
		t.Fatal("an unknown colour name should throw")
	}
	if !strings.Contains(err.Error(), "unknown ansi") {
		t.Errorf("error should name the unknown colour: %v", err)
	}
}
