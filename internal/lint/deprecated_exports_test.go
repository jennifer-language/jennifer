// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestDeprecatedModuleExports: L301 reports a call to a module function whose
// docblock carries `@deprecated`, its text (continuation lines joined) the
// message; a function without the tag, or with it on a private function or a
// detached comment, is not reported.
func TestDeprecatedModuleExports(t *testing.T) {
	const modSrc = `/**
 * Old.
 * @param a {int} a
 * @deprecated use ` + "`m.fresh`" + ` with options;
 *   removed in 0.29.0
 * @return {int} a
 */
export func old(a as int) { return $a; }

/**
 * Bare.
 * @deprecated
 */
export func bare() { return 1; }

/**
 * Fresh.
 */
export func fresh(a as int) { return $a; }

/**
 * @deprecated detached
 */

export func detached() { return 1; }
`
	mod, err := parser.Parse(modSrc)
	if err != nil {
		t.Fatal(err)
	}
	cfg := lint.DefaultConfig()
	cfg.Modules = map[string]*parser.Program{"m": mod}
	cfg.DeprecatedExports = lint.ModuleDeprecations(cfg.Modules, func(string) ([]byte, error) {
		return []byte(modSrc), nil
	})
	src := `import "./m.j" as m;
def a as int init m.old(1);
def b as int init m.bare();
def c as int init m.fresh(1);
def d as int init m.detached();`
	diags := lintSrc(t, src, only("L301"), cfg)
	var got []string
	for _, d := range diags {
		got = append(got, d.Message)
	}
	want := []string{
		"`m.old` is deprecated: use `m.fresh` with options; removed in 0.29.0",
		"`m.bare` is deprecated",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
