// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestFuncValueRunsInHomeContext pins that a func value called across a module
// boundary runs its body in its DEFINING interpreter's context, so the body
// resolves its own namespace imports - and that a struct crossing the boundary
// as an argument or return is retagged to the right identity on each side.
//
//   - the module calls a host func value that constructs one of the module's own
//     structs via the host's alias; the struct must arrive back inside the module
//     as its own bare type (a raw caller-context call would fail to resolve the
//     host's alias, and the struct would carry the wrong identity);
//   - the host calls a module func value whose body uses `strings.upper`, though
//     the host itself never `use`d strings.
func TestFuncValueRunsInHomeContext(t *testing.T) {
	dir := t.TempDir()

	modSrc := `use strings;
export def struct Item { label as string };
# Take a func value the caller supplies, call it (it builds one of OUR structs),
# and read the module-typed result.
export func apply(make as func) {
    def it as Item init $make();
    return $it.label;
}
# A module method that leans on the module's own ` + "`use strings`" + `.
export func shout(s as string) { return strings.upper($s); }
export func shouter() { def f as func init shout; return $f; }
`
	modPath := filepath.Join(dir, "fvmod.j")
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// The entry program deliberately does NOT `use strings`.
	prog := fmt.Sprintf(`use testing;
import %q as m;

# A host func value that constructs a MODULE struct through the host's alias.
func makeItem() { return m.Item{label: "widget"}; }

def mk as func init makeItem;
testing.assertEqual(m.apply($mk), "widget");

def sh as func init m.shouter();
testing.assertEqual($sh("hi"), "HI");
`, modPath)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("func-value home-context program failed with code %d", code)
	}
}
