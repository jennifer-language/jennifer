// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestArgsDispatchCrossBoundary drives args.dispatch from a real entry program:
// the handler func values live in the entry program and read their args with the
// `args.` alias, while dispatch itself runs inside the args module. This
// exercises the func-value home-context path (the handler resolves its own
// `args` import) and the args.Result struct crossing the module boundary in both
// directions - the practical payoff of first-class cross-module func values.
func TestArgsDispatchCrossBoundary(t *testing.T) {
	argsMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "args.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use convert;
import %q as args;

func cmdGreet(r as args.Result) { return "greet:" + args.asString($r, "name"); }
func cmdBye(r as args.Result) { return "bye:" + args.asString($r, "name"); }

def p as args.Parser init args.parser("tool", "a demo tool");
def g as args.Parser init args.parser("greet", "greet someone");
$g = args.positional($g, "name", "who to greet");
$p = args.command($p, "greet", "", $g);
def b as args.Parser init args.parser("bye", "say goodbye");
$b = args.positional($b, "name", "who to farewell");
$p = args.command($p, "bye", "", $b);

def handlers as map of string to func init {"greet": cmdGreet, "bye": cmdBye};

def r1 as args.Result init args.parse($p, ["tool", "greet", "ada"]);
testing.assertEqual(args.dispatch($r1, $handlers), "greet:ada");

def r2 as args.Result init args.parse($p, ["tool", "bye", "bob"]);
testing.assertEqual(args.dispatch($r2, $handlers), "bye:bob");

# --help sets done: dispatch is a no-op (returns null), the app prints helpText.
def rh as args.Result init args.parse($p, ["tool", "--help"]);
testing.assertTrue($rh.done);
testing.assertEqual(convert.typeOf(args.dispatch($rh, $handlers)), "null");
`, argsMod)
	progPath := filepath.Join(dir, "cli.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("args dispatch program failed with code %d", code)
	}
}
