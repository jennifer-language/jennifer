// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	listslib "github.com/mplx/jennifer-lang/internal/lib/lists"
	tasklib "github.com/mplx/jennifer-lang/internal/lib/task"
)

// runScopedModuleMain is like runModuleMain but installs the libraries the
// module scope / namespacing tests exercise (io + lists + task) into every
// module interpreter, so a module's `use io;` / `use task;` resolves.
func runScopedModuleMain(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	var buf bytes.Buffer
	setup := func(s *interpreter.Interpreter) {
		s.Out = &buf
		iolib.Install(s)
		listslib.Install(s)
		tasklib.Install(s)
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

// pointsModule is a small declarations-only module reused across tests: a
// struct, a constant, and functions that build and consume the struct.
const pointsModule = `
def struct Point { x as int, y as int };
def const DIM as int init 2;
func make(x as int, y as int) { return Point{x: $x, y: $y}; }
func getX(p as Point) { return $p.x; }
func sumXY(p as Point) { return $p.x + $p.y; }
`

func TestModuleQualifiedCallAndConst(t *testing.T) {
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as points;
io.printf("dim=%d x=%d sum=%d\n", points.DIM, points.getX(points.make(3, 4)), points.sumXY(points.make(3, 4)));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := strings.TrimSpace(out), "dim=2 x=3 sum=7"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestModuleStructMadeAndPassedBack(t *testing.T) {
	// A struct built by a module function and passed straight back to another
	// module function type-checks (the value keeps the module's identity).
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as points;
io.printf("%d\n", points.getX(points.make(41, 9)));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out); got != "41" {
		t.Errorf("output = %q, want 41", got)
	}
}

func TestModuleDeclarationsOnlyRejectsMutableDef(t *testing.T) {
	_, err := runScopedModuleMain(t, map[string]string{
		"bad.j":  `def x as int init 5;`,
		"main.j": `import "./bad.j";`,
	})
	if err == nil {
		t.Fatal("expected a declarations-only error for a mutable def")
	}
	if !strings.Contains(err.Error(), "mutable") {
		t.Errorf("error should call out the mutable def: %v", err)
	}
}

func TestModuleDeclarationsOnlyRejectsStatement(t *testing.T) {
	_, err := runScopedModuleMain(t, map[string]string{
		"bad.j":  `use io; io.printf("side effect\n");`,
		"main.j": `import "./bad.j";`,
	})
	if err == nil {
		t.Fatal("expected a declarations-only error for a free-standing statement")
	}
	if !strings.Contains(err.Error(), "only declarations") {
		t.Errorf("error should call out declarations-only: %v", err)
	}
}

func TestModuleUseIsNotTransitive(t *testing.T) {
	// The module's own `use io;` works inside the module, but the importer
	// (which never `use io;`) cannot call io.* itself.
	out, err := runScopedModuleMain(t, map[string]string{
		"logger.j": `use io;
func log(msg as string) { io.printf("[log] %s\n", $msg); return; }`,
		"main.j": `import "./logger.j" as logger;
logger.log("ok");
io.printf("leaked\n");`,
	})
	if !strings.Contains(out, "[log] ok") {
		t.Errorf("module's own io.printf should have run: %q", out)
	}
	if err == nil {
		t.Fatal("importer calling io.* without `use io;` should error")
	}
	if !strings.Contains(err.Error(), "use io") {
		t.Errorf("error should point at the missing `use io;`: %v", err)
	}
	if strings.Contains(out, "leaked") {
		t.Errorf("the leaked io.printf must not have run: %q", out)
	}
}

func TestModuleAliasCollidesWithLibrary(t *testing.T) {
	_, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io;
import "./points.j" as io;`,
	})
	if err == nil {
		t.Fatal("expected a collision error for a module alias shadowing a library")
	}
	if !strings.Contains(err.Error(), "collides") {
		t.Errorf("error should mention the collision: %v", err)
	}
}

func TestModuleMethodThrowIsCatchable(t *testing.T) {
	out, err := runScopedModuleMain(t, map[string]string{
		"thrower.j": `func boom() { throw Error{kind: "x", message: "boom", file: "", line: 0, col: 0}; return 0; }`,
		"main.j": `use io;
import "./thrower.j" as thrower;
try { def x as int init thrower.boom(); } catch (e) { io.printf("caught %s\n", $e.message); }`,
	})
	if err != nil {
		t.Fatalf("a caught module throw should not fail the program: %v", err)
	}
	if !strings.Contains(out, "caught boom") {
		t.Errorf("module throw was not caught at the consumer: %q", out)
	}
}

// A spawn body calling a module function must be race-free: the module's
// interpreter holds only immutable constants and read-only methods, so
// concurrent calls share no mutable state. Run under `go test -race`.
func TestModuleSpawnCallNoRace(t *testing.T) {
	out, err := runScopedModuleMain(t, map[string]string{
		"points.j": pointsModule,
		"main.j": `use io; use task;
import "./points.j" as points;
def tasks as list of task of int init [];
def w as int init 0;
while ($w < 8) {
    $tasks[] = spawn { return points.sumXY(points.make($w, $w)); };
    $w = $w + 1;
}
def results as list of int init task.waitAll($tasks);
io.printf("len=%d\n", len($results));`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "len=8") {
		t.Errorf("expected 8 worker results, got %q", out)
	}
}
