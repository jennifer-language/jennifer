// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestErrorCrossesModuleBoundary pins that the auto-injected `Error` struct
// crosses the module <-> entry-program boundary intact in both directions,
// rather than being stamped with the module's identity (which made it
// unbindable to an `as Error` parameter on the far side).
//
// Direction A: a module *returns* an Error and the entry program binds it as
// `Error`. Direction B: the entry program hands an Error *into* the module,
// which passes it straight back to a host method via `meta.callMain`, and that
// host method binds it as `Error`. Both paths run retagStructs over the value;
// before the fix, isOwnStruct("Error") was true, so the outward / callMain
// retag re-branded the module's Error and the `as Error` bind failed.
func TestErrorCrossesModuleBoundary(t *testing.T) {
	dir := t.TempDir()

	modSrc := `use meta;

# Direction A: build an Error inside the module and return it to the host.
export func makeErr() {
    def e as Error init Error{kind: "boom", message: "from-module", file: "", line: 0, col: 0};
    return $e;
}

# Direction B: accept an Error from the host and pass it straight back to a
# host method by name - the meta.callMain retag path.
export func bounce(e as Error) {
    return meta.callMain("hostSees", $e);
}
`
	modPath := filepath.Join(dir, "errmod.j")
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	prog := fmt.Sprintf(`use testing;
import %q as em;

# Host method the module reaches back into via meta.callMain.
func hostSees(e as Error) { return $e.message; }

# Direction A: module-returned Error binds as Error here.
def a as Error init em.makeErr();
testing.assertEqual($a.kind, "boom");
testing.assertEqual($a.message, "from-module");

# Direction B: host Error -> module -> back to hostSees, which binds it as Error.
def mine as Error init Error{kind: "k", message: "round-trip", file: "", line: 0, col: 0};
def echoed as string init em.bounce($mine);
testing.assertEqual($echoed, "round-trip");
`, modPath)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("Error boundary program failed with code %d", code)
	}
}
