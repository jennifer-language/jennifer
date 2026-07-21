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
	"jennifer-lang.dev/jennifer/internal/lexer"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
)

// moduleProgram lexes, preprocesses, and parses a module file - the same
// pipeline the CLI's loader runs, so the tests exercise real path resolution.
func moduleProgram(path string) (*parser.Program, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	toks, err := lexer.TokenizeWithFile(string(src), path)
	if err != nil {
		return nil, err
	}
	toks, err = preproc.Process(toks, filepath.Dir(path), path)
	if err != nil {
		return nil, err
	}
	return parser.ParseTokens(toks)
}

// runModuleMain writes the named files into a temp dir, then runs main.j with
// the module system enabled. Every module interpreter shares one output
// buffer so init order is observable. Returns the combined output and the run
// error.
func runModuleMain(t *testing.T, files map[string]string) (string, error) {
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
	}
	in := interpreter.New()
	setup(in)
	in.EnableModules(dir, nil, moduleProgram, setup)

	mainPath := filepath.Join(dir, "main.j")
	prog, err := moduleProgram(mainPath)
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// logHelper is a per-module method that prints its name; each module carries
// its own copy (a fresh sub-interpreter has its own scope, so the name never
// collides across modules).
const logHelper = ` func log(n as string) { io.printf("%s ", $n); return 0; } `

func TestModulePostOrderRunOnce(t *testing.T) {
	// main -> a -> {b -> c, c}. c is imported twice but must init once, and
	// every module must init before the code that imports it: c, b, a, main.
	out, err := runModuleMain(t, map[string]string{
		"c.j":    `use io; def const C as int init log("c");` + logHelper,
		"b.j":    `use io; import "./c.j"; def const B as int init log("b");` + logHelper,
		"a.j":    `use io; import "./b.j"; import "./c.j"; def const A as int init log("a");` + logHelper,
		"main.j": `use io; import "./a.j"; io.printf("main");`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out, "c b a main"; got != want {
		t.Errorf("init order = %q, want %q", got, want)
	}
}

func TestModuleCycleRejected(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"x.j":    `import "./y.j"; def const X as int init 1;`,
		"y.j":    `import "./x.j"; def const Y as int init 1;`,
		"main.j": `import "./x.j";`,
	})
	if err == nil {
		t.Fatal("expected a module-cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "module cycle") {
		t.Errorf("error should name the cycle: %v", err)
	}
	// The chain names every edge that closes the loop.
	if !strings.Contains(err.Error(), "x.j") || !strings.Contains(err.Error(), "y.j") {
		t.Errorf("cycle chain should name both modules: %v", err)
	}
}

// runModuleMainTree is runModuleMain but the file map keys may contain
// subdirectories (e.g. "a/util.j"), so two modules can share a basename while
// living in different directories.
func runModuleMainTree(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	var buf bytes.Buffer
	setup := func(s *interpreter.Interpreter) {
		s.Out = &buf
		iolib.Install(s)
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

// Two module files sharing a basename (both `util.j`) are distinct types keyed
// by their canonical path, so importing both under different aliases works: each
// module's struct round-trips through its own methods, and the two `Thing`
// types never cross-satisfy each other.
func TestSameStemModulesCoexist(t *testing.T) {
	files := map[string]string{
		"a/util.j": `export def struct Thing { x as int };
			export func make() { return Thing{ x: 1 }; }
			export func getx(t as Thing) { return $t.x; }`,
		"b/util.j": `export def struct Thing { y as int };
			export func make() { return Thing{ y: 2 }; }
			export func gety(t as Thing) { return $t.y; }`,
	}
	// Happy path: each module's struct passes back into its own method.
	ok := make(map[string]string, len(files))
	for k, v := range files {
		ok[k] = v
	}
	ok["main.j"] = `import "./a/util.j" as u; import "./b/util.j" as v; use io;
		def a as u.Thing init u.make();
		def b as v.Thing init v.make();
		io.printf("%d %d\n", u.getx($a), v.gety($b));`
	out, err := runModuleMainTree(t, ok)
	if err != nil {
		t.Fatalf("same-stem modules should coexist: %v", err)
	}
	if strings.TrimSpace(out) != "1 2" {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), "1 2")
	}

	// The two Thing types are distinct: u's Thing must not satisfy v's Thing.
	bad := make(map[string]string, len(files))
	for k, v := range files {
		bad[k] = v
	}
	bad["main.j"] = `import "./a/util.j" as u; import "./b/util.j" as v;
		def a as v.Thing init u.make();`
	if _, err := runModuleMainTree(t, bad); err == nil {
		t.Error("assigning u.Thing to a v.Thing variable should be a type error")
	}
}

// Field writes and zero-init on a same-stem module struct must resolve
// through the module's canonical path (the identity), not the stem: a
// stem-keyed lookup is ambiguous when two loaded modules share a basename,
// which would break `$x.field = ...` and `def y as u.Thing;` for both.
func TestSameStemModuleFieldWriteAndZeroInit(t *testing.T) {
	out, err := runModuleMainTree(t, map[string]string{
		"a/util.j": `export def struct Thing { x as int };
			export func make() { return Thing{ x: 1 }; }`,
		"b/util.j": `export def struct Gadget { y as int };
			export func make() { return Gadget{ y: 2 }; }`,
		"main.j": `import "./a/util.j" as u; import "./b/util.j" as v; use io;
			def t as u.Thing init u.make();
			$t.x = 5;
			def z as u.Thing;
			def g as v.Gadget init v.make();
			$g.y = 7;
			def h as v.Gadget;
			io.printf("%d %d %d %d\n", $t.x, $z.x, $g.y, $h.y);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "5 0 7 0" {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), "5 0 7 0")
	}
}

// A consumer-defined bare struct that shares a name with a module's struct must
// not impersonate it across the inward call boundary: passing the consumer's own
// Point to a module method that takes the module's Point is a positioned error,
// not silently accepted.
func TestModuleRejectsImpostorStruct(t *testing.T) {
	_, err := runModuleMainTree(t, map[string]string{
		"geo.j": `export def struct Point { x as int };
			export func px(p as Point) { return $p.x; }`,
		"main.j": `import "./geo.j" as g; use io;
			def struct Point { name as string };
			def mine as Point init Point{ name: "nope" };
			io.printf("%d\n", g.px($mine));`,
	})
	if err == nil {
		t.Fatal("expected an impostor-struct error")
	}
	if !strings.Contains(err.Error(), "impersonate") {
		t.Errorf("error should mention impersonation: %v", err)
	}
	// A genuine module struct (constructed via the alias) is accepted.
	out, err := runModuleMainTree(t, map[string]string{
		"geo.j": `export def struct Point { x as int };
			export func px(p as Point) { return $p.x; }`,
		"main.j": `import "./geo.j" as g; use io;
			def p as g.Point init g.Point{ x: 42 };
			io.printf("%d\n", g.px($p));`,
	})
	if err != nil {
		t.Fatalf("genuine module struct should be accepted: %v", err)
	}
	if strings.TrimSpace(out) != "42" {
		t.Errorf("got %q, want 42", strings.TrimSpace(out))
	}
}

// A map whose keys are a module's own struct must retag those keys on the way
// out, so the consumer's lookup with a same-identity key matches.
func TestModuleRetagsMapStructKeys(t *testing.T) {
	out, err := runModuleMainTree(t, map[string]string{
		"reg.j": `export def struct K { id as int };
			export func build() { def m as map of K to int init {K{ id: 1 }: 100, K{ id: 2 }: 200}; return $m; }`,
		"main.j": `import "./reg.j" as r; use io;
			def m as map of r.K to int init r.build();
			io.printf("%d %d\n", $m[r.K{ id: 1 }], $m[r.K{ id: 2 }]);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "100 200" {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), "100 200")
	}
}

// Distinct stems are fine, and importing the same module file twice (a run-once
// cache hit) is not a collision.
func TestModuleDistinctStemsAndReimportOK(t *testing.T) {
	out, err := runModuleMainTree(t, map[string]string{
		"a/one.j": `export func v() { return 1; }`,
		"b/two.j": `export func v() { return 2; }`,
		"main.j":  `import "./a/one.j" as o; import "./b/two.j" as t; import "./a/one.j" as oo; use io; io.printf("%d\n", o.v() + t.v() + oo.v());`,
	})
	if err != nil {
		t.Fatalf("distinct stems / reimport should be fine: %v", err)
	}
	if strings.TrimSpace(out) != "4" {
		t.Errorf("got %q, want 4", strings.TrimSpace(out))
	}
}

func TestModuleParseErrorPositioned(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"bad.j":  `def x as `, // truncated - parse error inside the module
		"main.j": `import "./bad.j";`,
	})
	if err == nil {
		t.Fatal("expected a parse error from the imported module, got nil")
	}
	if !strings.Contains(err.Error(), "bad.j") {
		t.Errorf("parse error should be positioned in the imported file: %v", err)
	}
}

func TestModuleInitThrowFailsAtLoad(t *testing.T) {
	// A throwing initializer fails at load, before the importer's body runs.
	out, err := runModuleMain(t, map[string]string{
		"boom.j": `def const B as int init blow();
			func blow() { throw Error{kind: "x", message: "boom", file: "", line: 0, col: 0}; return 0; }`,
		"main.j": `use io; import "./boom.j"; io.printf("AFTER");`,
	})
	if err == nil {
		t.Fatal("expected the throwing initializer to fail the load, got nil")
	}
	if strings.Contains(out, "AFTER") {
		t.Errorf("importer body ran despite a load-time throw: %q", out)
	}
}

func TestModuleImportsWithoutEnableError(t *testing.T) {
	// A program with module imports but no EnableModules (an embedding that
	// never wired the module system) fails with a clear message rather than
	// silently ignoring the import.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dep.j"), []byte(`def const D as int init 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.j")
	if err := os.WriteFile(mainPath, []byte(`import "./dep.j";`), 0o644); err != nil {
		t.Fatal(err)
	}
	prog, err := moduleProgram(mainPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	iolib.Install(in)
	if err := in.Run(prog); err == nil {
		t.Fatal("expected an error when modules are not enabled, got nil")
	}
}

// TestModuleStructZeroValueAndFieldWrite exercises the cross-module struct
// resolution lookupStructDef / zeroStructFor gained: `def x as m.Struct;`
// (zero value, no init) and `$x.field = ...` both resolve an imported module
// struct's definition, and a nested module struct zeroes and writes through.
// Regression for the "unknown struct" (zero value) and "definition missing"
// (field write) errors those paths raised because module structs live in the
// module's own interpreter, not i.NSStructs.
func TestModuleStructZeroValueAndFieldWrite(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"m.j": `export def struct Inner { n as int };
export def struct Point { tag as string, inner as Inner };`,
		"main.j": `use io;
import "./m.j" as m;
def p as m.Point;
io.printf("zero tag=[%s] n=%d\n", $p.tag, $p.inner.n);
$p.tag = "hi";
$p.inner.n = 7;
io.printf("wrote tag=[%s] n=%d\n", $p.tag, $p.inner.n);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "zero tag=[] n=0\nwrote tag=[hi] n=7\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The impostor check must recurse: a consumer struct impersonating a module
// struct hidden inside a list (or map / struct field) crosses the boundary
// just as unchecked as a top-level one - and would be re-branded to the
// module's identity on the way back out.
func TestModuleRejectsNestedImpostorStruct(t *testing.T) {
	geo := `export def struct Point { x as int };
		export func firstX(ps as list of Point) { return $ps[0]; }`
	_, err := runModuleMainTree(t, map[string]string{
		"geo.j": geo,
		"main.j": `import "./geo.j" as g; use io;
			def struct Point { name as string };
			def xs as list of Point init [Point{ name: "nope" }];
			def back as g.Point init g.firstX($xs);
			io.printf("%s\n", "unreachable");`,
	})
	if err == nil {
		t.Fatal("expected a nested impostor-struct error")
	}
	if !strings.Contains(err.Error(), "impersonate") {
		t.Errorf("error should mention impersonation: %v", err)
	}
	// Genuine module structs nested in a list still cross fine.
	out, err := runModuleMainTree(t, map[string]string{
		"geo.j": geo,
		"main.j": `import "./geo.j" as g; use io;
			def xs as list of g.Point init [g.Point{ x: 7 }];
			def back as g.Point init g.firstX($xs);
			io.printf("%d\n", $back.x);`,
	})
	if err != nil {
		t.Fatalf("genuine nested module struct should be accepted: %v", err)
	}
	if strings.TrimSpace(out) != "7" {
		t.Errorf("got %q, want 7", strings.TrimSpace(out))
	}
}
