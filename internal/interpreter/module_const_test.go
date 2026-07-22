// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// Module-alias constants are pre-resolved (stamped) by resolveQualifiedRefs so
// `m.CONST` returns the value directly instead of a per-access GetBinding slot
// scan. These tests pin the behaviour the stamping must preserve: correct
// values (scalar and struct, the latter exercising the cross-boundary retag),
// stable across repeated access, with the unexported / missing errors intact
// (an unstampable ref stays nil and falls through to the runtime error path).

const constModule = `
export def const GREETING as string init "hello";
export def const LIMIT as int init 42;
def const SECRET as int init 99;
export def struct Point { x as int, y as int };
export def const ORIGIN as Point init Point{ x: 1, y: 2 };
`

func TestModuleConstStampedScalarAndStruct(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"mod.j": constModule,
		"main.j": `use io; import "./mod.j" as m;
def i as int init 0;
def sum as int init 0;
while ($i < 3) { $sum = $sum + m.LIMIT; $i = $i + 1; }
io.printf("%s/%d/%d/%d,%d", m.GREETING, m.LIMIT, $sum, m.ORIGIN.x, m.ORIGIN.y);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "hello/42/126/1,2"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// A struct-valued module const survives value semantics: copying it into a local
// and mutating that local must not disturb the shared, stamped const value.
func TestModuleConstStructIsValueSemantic(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"mod.j": constModule,
		"main.j": `use io; import "./mod.j" as m;
def p as m.Point init m.ORIGIN;
$p.x = 999;
io.printf("%d,%d/%d,%d", $p.x, $p.y, m.ORIGIN.x, m.ORIGIN.y);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "999,2/1,2"; out != want {
		t.Fatalf("got %q, want %q (stamped const must not alias a local copy)", out, want)
	}
}

func TestModuleConstUnexportedStillErrors(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"mod.j":  constModule,
		"main.j": `use io; import "./mod.j" as m; io.printf("%d", m.SECRET);`,
	})
	if err == nil || !strings.Contains(err.Error(), "not exported") {
		t.Fatalf("expected not-exported error, got %v", err)
	}
}

func TestModuleConstMissingStillErrors(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"mod.j":  constModule,
		"main.j": `use io; import "./mod.j" as m; io.printf("%d", m.NOPE);`,
	})
	if err == nil || !strings.Contains(err.Error(), "no constant") {
		t.Fatalf("expected no-constant error, got %v", err)
	}
}
