// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"os"
	"path/filepath"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
)

// evalRepl mirrors the REPL's per-input pipeline (lex -> preproc -> parse ->
// EvalInteractive) so module `import` statements reach loadModuleImports the
// same way a typed line does.
func evalRepl(t *testing.T, in *interpreter.Interpreter, cwd, src string) (interpreter.Value, error) {
	t.Helper()
	toks, err := lexer.TokenizeWithFile(src, "<repl>")
	if err != nil {
		t.Fatalf("lex %q: %v", src, err)
	}
	toks, err = preproc.Process(toks, cwd, "<repl>")
	if err != nil {
		t.Fatalf("preproc %q: %v", src, err)
	}
	prog, err := parser.ParseTokens(toks)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return in.EvalInteractive(prog)
}

// replInterpWithModules builds a REPL-style interpreter with the module system
// enabled against `dir` (local imports resolve there), matching runRepl.
func replInterpWithModules(dir string) *interpreter.Interpreter {
	setup := func(s *interpreter.Interpreter) { iolib.Install(s) }
	in := interpreter.New()
	setup(in)
	in.EnableModules(dir, nil, moduleProgram, setup)
	return in
}

// TestEvalInteractiveModuleImport is the REPL counterpart of the batch module
// tests: an `import "./dep.j";` typed at the prompt binds the module's stem
// namespace so a later `dep.member` / `dep.Struct` resolves - the fix for the
// REPL previously accepting the import yet reporting "unknown namespace".
func TestEvalInteractiveModuleImport(t *testing.T) {
	dir := t.TempDir()
	dep := `export def struct Box { n as int };
export func make(v as int) { return Box{ n: $v }; }
export def const K as int init 7;`
	if err := os.WriteFile(filepath.Join(dir, "dep.j"), []byte(dep), 0o644); err != nil {
		t.Fatal(err)
	}
	in := replInterpWithModules(dir)

	// No alias: the namespace defaults to the file stem "dep".
	if _, err := evalRepl(t, in, dir, `import "./dep.j";`); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	// A module function call resolves across submissions.
	got, err := evalRepl(t, in, dir, `dep.make(3).n;`)
	if err != nil {
		t.Fatalf("dep.make failed: %v", err)
	}
	if got.Kind != interpreter.KindInt || got.Int != 3 {
		t.Errorf("dep.make(3).n = %v %v, want int 3", got.Kind, got)
	}
	// A module constant resolves.
	got, err = evalRepl(t, in, dir, `dep.K;`)
	if err != nil {
		t.Fatalf("dep.K failed: %v", err)
	}
	if got.Kind != interpreter.KindInt || got.Int != 7 {
		t.Errorf("dep.K = %v %v, want int 7", got.Kind, got)
	}
	// A module struct type resolves in a declaration.
	if _, err := evalRepl(t, in, dir, `def b as dep.Box init dep.make(9);`); err != nil {
		t.Fatalf("def as dep.Box failed: %v", err)
	}
	got, err = evalRepl(t, in, dir, `$b.n;`)
	if err != nil {
		t.Fatalf("read $b.n failed: %v", err)
	}
	if got.Kind != interpreter.KindInt || got.Int != 9 {
		t.Errorf("$b.n = %v %v, want int 9", got.Kind, got)
	}
}

// TestEvalInteractiveModuleReimport confirms re-typing the same import is a
// silent no-op (a module is run-once / cached), while binding the alias to a
// different module is still a collision.
func TestEvalInteractiveModuleReimport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dep.j"), []byte(`export def const K as int init 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.j"), []byte(`export def const K as int init 2;`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := replInterpWithModules(dir)

	if _, err := evalRepl(t, in, dir, `import "./dep.j";`); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	// Re-importing the same module under the same (stem) alias: no error.
	if _, err := evalRepl(t, in, dir, `import "./dep.j";`); err != nil {
		t.Errorf("re-import of the same module should be a no-op, got: %v", err)
	}
	// Binding the alias "dep" to a *different* module is a real collision.
	if _, err := evalRepl(t, in, dir, `import "./other.j" as dep;`); err == nil {
		t.Error("expected a collision binding alias `dep` to a different module")
	}
}

// TestEvalInteractiveImportWithoutEnableModules confirms that if the REPL never
// enabled modules, an `import` reports the clear "not enabled" error rather
// than silently registering nothing.
func TestEvalInteractiveImportWithoutEnableModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dep.j"), []byte(`export def const K as int init 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := interpreter.New()
	iolib.Install(in) // libraries, but no EnableModules
	if _, err := evalRepl(t, in, dir, `import "./dep.j";`); err == nil {
		t.Error("expected an error importing a module without EnableModules")
	}
}
