// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// Module-alias method calls are pre-resolved (stamped) by resolveQualifiedRefs
// so `m.fn(args)` dispatches straight through dispatchModuleMethod, skipping the
// per-call prefix / existence / export lookups. These tests pin the behaviour
// the stamping must preserve: correct dispatch (incl. the cross-boundary struct
// retag the fast path must still perform), with the unexported / missing /
// arity errors intact (an unstampable ref stays nil and takes the checked path).

const methodModule = `
export def struct Vec { x as int, y as int };
export func add(a as int, b as int) { return $a + $b; }
export func shift(v as Vec, dx as int) { return Vec{ x: $v.x + $dx, y: $v.y }; }
func secret() { return 1; }
`

func TestModuleMethodStampedInLoop(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"mod.j": methodModule,
		"main.j": `use io; import "./mod.j" as m;
def acc as int init 0;
def i as int init 0;
while ($i < 4) { $acc = m.add($acc, $i); $i = $i + 1; }
io.printf("%d", $acc);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "6" { // 0+0+1+2+3
		t.Fatalf("got %q, want %q", out, "6")
	}
}

// The stamped fast path must still retag a module struct passed in and returned
// out, so a Vec built in the consumer survives the round trip with the module's
// identity (field access on the result proves the retag).
func TestModuleMethodStructAcrossBoundary(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"mod.j": methodModule,
		"main.j": `use io; import "./mod.j" as m;
def v as m.Vec init m.shift(m.Vec{ x: 10, y: 20 }, 5);
io.printf("%d,%d", $v.x, $v.y);`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "15,20" {
		t.Fatalf("got %q, want %q", out, "15,20")
	}
}

func TestModuleMethodUnexportedStillErrors(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"mod.j":  methodModule,
		"main.j": `import "./mod.j" as m; m.secret();`,
	})
	if err == nil || !strings.Contains(err.Error(), "not exported") {
		t.Fatalf("expected not-exported error, got %v", err)
	}
}

func TestModuleMethodMissingStillErrors(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"mod.j":  methodModule,
		"main.j": `import "./mod.j" as m; m.nope();`,
	})
	if err == nil || !strings.Contains(err.Error(), "no method") {
		t.Fatalf("expected no-method error, got %v", err)
	}
}

func TestModuleMethodArityErrorAtCallSite(t *testing.T) {
	_, err := runModuleMain(t, map[string]string{
		"mod.j":  methodModule,
		"main.j": `import "./mod.j" as m; def x as int init m.add(1);`,
	})
	if err == nil || !strings.Contains(err.Error(), "takes 2 parameter") {
		t.Fatalf("expected arity error, got %v", err)
	}
}
