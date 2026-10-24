// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
)

// A call into a module method is a hazard only when that method can re-enter:
// the module ran the analysis when it loaded and stamped ReentryFree, and the
// importer's scan reads it. So a module function calling a ReentryFree one stays
// GlobalSafe (its read-only parameters borrow), while one calling a re-entering
// function (a func-value call here) does not, and neither does its caller. The
// spawn-capture pass reads the same stamp.
func TestModuleCallReentryFree(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"b.j": `export func echo(s as string) { return $s; }
export func call(f as func) { return $f(); }`,
		"a.j": `import "./b.j" as b;
export func viaEcho(xs as list of int) { b.echo("x"); return $xs[0]; }
export func viaCall(xs as list of int, f as func) { b.call($f); return $xs[0]; }
export func outer(xs as list of int, f as func) { return viaCall($xs, $f); }`,
		"main.j": `import "./a.j" as a;
def g as list of int init [1];
def h as list of int init [2];
func one() { return 1; }
def r as int init a.viaEcho($g);
def t1 as task of int init spawn { return a.viaEcho($g); };
def t2 as task of int init spawn { return a.viaCall($g, one); };`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	load := func(path string) (*parser.Program, error) {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		toks, err := lexer.TokenizeWithFile(string(src), path)
		if err != nil {
			return nil, err
		}
		if toks, err = preproc.Process(toks, filepath.Dir(path), path); err != nil {
			return nil, err
		}
		return parser.ParseTokens(toks)
	}
	var buf bytes.Buffer
	setup := func(s *Interpreter) { s.Out = &buf }
	in := New()
	setup(in)
	in.EnableModules(dir, nil, load, setup)
	prog, err := load(filepath.Join(dir, "main.j"))
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	// A spawn calling a ReentryFree module function captures only the globals
	// it names; one calling a re-entering function copies them all.
	spawns := in.scanGlobalRefs(prog.TopLevel, nil).spawns
	if len(spawns) != 2 {
		t.Fatalf("want 2 spawns, found %d", len(spawns))
	}
	if c := spawns[0].Captures; c == nil || c.AllGlobals || len(c.Globals) != 1 || !c.Globals["g"] {
		t.Errorf("spawn calling a.viaEcho: captures %+v, want just g", c)
	}
	if c := spawns[1].Captures; c == nil || !c.AllGlobals {
		t.Errorf("spawn calling a.viaCall: captures %+v, want all globals", c)
	}
	a := in.moduleAliases["a"].interp
	b := a.moduleAliases["b"].interp
	for _, c := range []struct {
		in          *Interpreter
		name        string
		globalSafe  bool
		reentryFree bool
	}{
		{b, "echo", true, true},
		{b, "call", false, false},
		{a, "viaEcho", true, true},
		{a, "viaCall", false, false},
		{a, "outer", false, false},
	} {
		m := c.in.methods[c.name]
		if m.GlobalSafe != c.globalSafe || m.ReentryFree != c.reentryFree {
			t.Errorf("%s: GlobalSafe=%v ReentryFree=%v, want %v %v", c.name, m.GlobalSafe, m.ReentryFree, c.globalSafe, c.reentryFree)
		}
	}
}
