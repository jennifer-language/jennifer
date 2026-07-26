// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// Enums declared in one file and consumed in another. The resolver cannot see a
// module's enum at parse time, so these paths run through the deferred-match
// machinery: the resolver rewrites the arms and records the match, and the
// interpreter validates it once the module is loaded. Each test here pins a bug
// where that hand-off used to be missing.

const enumShapesModule = `
export def enum Shape { Circle { r as float }, Rect { w as float, h as float }, Empty };
export func mk(r as float) { return Shape.Circle{ r: $r }; }
`

// A cross-module match must be able to destructure the payload. This used to
// fail at parse time with "undefined variable c": the arms were left as value
// arms, so the binder was never introduced and the body's `$c` dangled.
func TestModuleEnumMatchBindsPayload(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		"main.j": `
use io;
import "./shapes.j" as sh;
def a as sh.Shape init sh.Shape.Rect{ w: 2.0, h: 3.0 };
match ($a) {
    when Circle(c) { io.printf("circle %f\n", $c.r); }
    when Rect(rc) { io.printf("rect %f\n", $rc.w * $rc.h); }
    when Empty { io.printf("empty\n"); }
}
`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "rect 6.0\n" {
		t.Errorf("got %q, want %q", out, "rect 6.0\n")
	}
}

// Exhaustiveness has to hold across a module boundary too. This used to load
// and run, matching nothing and falling silently through - the exact failure
// the check exists to prevent.
func TestModuleEnumMatchExhaustiveness(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		"main.j": `
use io;
import "./shapes.j" as sh;
def a as sh.Shape init sh.Shape.Empty;
match ($a) {
    when Circle(c) { io.printf("circle\n"); }
    when Rect(rc) { io.printf("rect\n"); }
}
`,
	})
	if err == nil {
		t.Fatal("expected a non-exhaustive error for a module enum match")
	}
	if !strings.Contains(err.Error(), "not exhaustive") || !strings.Contains(err.Error(), "Empty") {
		t.Errorf("expected the missing-variant error, got %v", err)
	}
}

// An `else` still discharges exhaustiveness across the boundary.
func TestModuleEnumMatchElseIsExhaustive(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		"main.j": `
use io;
import "./shapes.j" as sh;
def a as sh.Shape init sh.Shape.Empty;
match ($a) {
    when Circle(c) { io.printf("circle\n"); }
    else { io.printf("other\n"); }
}
`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "other\n" {
		t.Errorf("got %q, want %q", out, "other\n")
	}
}

// A misspelled variant is a load-time error naming the enum, not a runtime
// "undefined name" pointing at the arm head.
func TestModuleEnumUnknownVariant(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		"main.j": `
use io;
import "./shapes.j" as sh;
def a as sh.Shape init sh.Shape.Empty;
match ($a) {
    when Circle(c) { io.printf("c\n"); }
    when Nonsense { io.printf("n\n"); }
    when Empty { io.printf("e\n"); }
}
`,
	})
	if err == nil {
		t.Fatal("expected an unknown-variant error")
	}
	if !strings.Contains(err.Error(), `"Nonsense" is not a variant`) {
		t.Errorf("expected the not-a-variant error, got %v", err)
	}
}

// Type identity is (namespace, name, module path), not the bare name. A local
// enum sharing a module enum's name used to hijack the match: the arms were
// checked against the local declaration, so a valid program was rejected...
func TestModuleEnumNotHijackedByLocalEnum(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		"main.j": `
use io;
import "./shapes.j" as sh;
def enum Shape { Alpha, Beta };
def a as sh.Shape init sh.Shape.Circle{ r: 1.0 };
def b as Shape init Shape.Beta;
match ($a) {
    when Circle(c) { io.printf("circle %f\n", $c.r); }
    when Rect(rc) { io.printf("rect\n"); }
    when Empty { io.printf("empty\n"); }
}
match ($b) {
    when Alpha { io.printf("alpha\n"); }
    when Beta { io.printf("beta\n"); }
}
`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "circle 1.0\nbeta\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// ...and, with a same-named variant carrying a different payload, a program was
// accepted whose binder had the wrong shape and blew up at runtime.
func TestModuleEnumLocalDecoyPayloadShape(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		"main.j": `
use io;
import "./shapes.j" as sh;
def enum Shape { Circle { x as int } };
def a as sh.Shape init sh.Shape.Circle{ r: 2.5 };
match ($a) {
    when Circle(c) { io.printf("r=%f\n", $c.r); }
    when Rect(rc) { io.printf("rect\n"); }
    when Empty { io.printf("empty\n"); }
}
`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "r=2.5\n" {
		t.Errorf("got %q, want %q", out, "r=2.5\n")
	}
}

// The deferred path must reject a variant-pattern match whose subject is a
// module *struct* rather than an enum.
func TestModuleStructRejectsVariantPatterns(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"pt.j": `
export def struct Point { x as int };
export func mk() { return Point{ x: 1 }; }
`,
		"main.j": `
use io;
import "./pt.j" as pt;
def p as pt.Point init pt.mk();
match ($p) {
    when Circle(c) { io.printf("c\n"); }
    when Empty { io.printf("e\n"); }
}
`,
	})
	if err == nil {
		t.Fatal("expected an error for variant patterns over a non-enum subject")
	}
	if !strings.Contains(err.Error(), "is not an enum") {
		t.Errorf("expected the not-an-enum error, got %v", err)
	}
}

// A module enum matched inside a spawn body also gets checked, even though
// spawn bodies are deliberately left unresolved for slot purposes.
func TestModuleEnumMatchInSpawnIsChecked(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"shapes.j": enumShapesModule,
		// The check runs at load, so the task never has to be awaited (and the
		// harness installs only `io`).
		"main.j": `
use io;
import "./shapes.j" as sh;
def t as task of string init spawn {
    def s as sh.Shape init sh.Shape.Empty;
    match ($s) {
        when Circle(c) { return "c"; }
        when Rect(rc) { return "r"; }
    }
    return "fell-through";
};
io.printf("loaded\n");
`,
	})
	if err == nil {
		t.Fatal("expected a non-exhaustive error inside a spawn body")
	}
	if !strings.Contains(err.Error(), "not exhaustive") {
		t.Errorf("expected the missing-variant error, got %v", err)
	}
}

// `export def enum` is only meaningful to an importer; a script has none. The
// script check covered methods and structs but skipped enums.
func TestExportEnumRejectedInScript(t *testing.T) {
	_, err := run(t, `
use io;
export def enum E { A };
io.printf("loaded\n");
`)
	if err == nil {
		t.Fatal("expected `export` in a script to be rejected")
	}
	if !strings.Contains(err.Error(), "only allowed in a module") {
		t.Errorf("expected the export-in-script error, got %v", err)
	}
}

// A local enum whose name collides with a module alias must not capture that
// alias's struct literals: `m.Opt{...}` is a module struct, not a variant.
func TestModuleAliasWinsOverSameNamedEnum(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"opts.j": `
export def struct Opt { a as int };
export func show(o as Opt) { return $o.a; }
`,
		"main.j": `
use io;
import "./opts.j" as m;
def enum m { Opt, Zed };
def o as m.Opt init m.Opt{ a: 7 };
io.printf("%d\n", m.show($o));
`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "7\n" {
		t.Errorf("got %q, want %q", out, "7\n")
	}
}
