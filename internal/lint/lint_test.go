// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint_test

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/lint"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// lintSrc lints src with the given checks enabled and returns the findings. It
// mirrors the CLI: parse from a copy (ParseTokens strips trivia in place), but
// hand the untouched token stream to Check so suppression directives survive.
func lintSrc(t *testing.T, src string, enabled map[string]bool, cfg lint.Config) []lint.Diagnostic {
	t.Helper()
	raw, err := lexer.TokenizeWithFile(src, "test.j")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	parseToks := make([]lexer.Token, len(raw))
	copy(parseToks, raw)
	prog, err := parser.ParseTokens(parseToks)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return lint.Check(prog, raw, src, "test.j", enabled, cfg)
}

// only builds an enabled-set with just the given IDs.
func only(ids ...string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// countID returns how many findings carry the given ID.
func countID(diags []lint.Diagnostic, id string) int {
	n := 0
	for _, d := range diags {
		if d.ID == id {
			n++
		}
	}
	return n
}

func TestUnusedLocal(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"unused flagged", `func f() { def used as int init 1; def dead as int init 2; return $used; }`, 1},
		{"all used", `func f() { def a as int init 1; def b as int init $a; return $b; }`, 0},
		{"spawn-local not flagged", `func f() { def t as task of int init spawn { def inner as int init 5; return 0; }; return task.wait($t); }`, 0},
		{"outer used only in spawn", `func f() { def outer as int init 5; def t as task of int init spawn { return $outer; }; return task.wait($t); }`, 0},
		{"global not flagged", `def top as int init 1;`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrc(t, c.src, only("L101"), cfg)
			if got := countID(diags, "L101"); got != c.want {
				t.Fatalf("L101 count = %d, want %d (%v)", got, c.want, diags)
			}
		})
	}
}

func TestDeadCode(t *testing.T) {
	diags := lintSrc(t, `func f() { return 1; def x as int init 2; }`, only("L102"), lint.DefaultConfig())
	if countID(diags, "L102") != 1 {
		t.Fatalf("expected one L102, got %v", diags)
	}
	clean := lintSrc(t, `func f() { def x as int init 2; return $x; }`, only("L102"), lint.DefaultConfig())
	if len(clean) != 0 {
		t.Fatalf("expected no L102, got %v", clean)
	}
}

func TestEmptyCatch(t *testing.T) {
	diags := lintSrc(t, `func f() { try { risky(); } catch (e) { } }`, only("L103"), lint.DefaultConfig())
	if countID(diags, "L103") != 1 {
		t.Fatalf("expected one L103, got %v", diags)
	}
	clean := lintSrc(t, `func f() { try { risky(); } catch (e) { handle($e); } }`, only("L103"), lint.DefaultConfig())
	if len(clean) != 0 {
		t.Fatalf("expected no L103, got %v", clean)
	}
}

func TestThrowNonError(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"string throw", `func f() { throw "boom"; }`, 1},
		{"error literal", `func f() { throw Error{kind: "x", message: "m", file: "f", line: 1, col: 1}; }`, 0},
		{"catch var rethrow", `func f() { try { risky(); } catch (e) { throw $e; } }`, 0},
		{"int throw", `func f() { throw 5; }`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrc(t, c.src, only("L104"), lint.DefaultConfig())
			if got := countID(diags, "L104"); got != c.want {
				t.Fatalf("L104 count = %d, want %d (%v)", got, c.want, diags)
			}
		})
	}
}

func TestMethodTooLong(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.MethodMaxStmts = 2
	src := `func f() { def a as int init 1; def b as int init 2; def c as int init 3; return $a; }`
	diags := lintSrc(t, src, only("L201"), cfg)
	if countID(diags, "L201") != 1 {
		t.Fatalf("expected one L201, got %v", diags)
	}
}

func TestNestingTooDeep(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.MaxNesting = 1
	src := `func f() { if (true) { if (true) { return 1; } } }`
	diags := lintSrc(t, src, only("L202"), cfg)
	if countID(diags, "L202") != 1 {
		t.Fatalf("expected one L202 (shallowest violating block), got %v", diags)
	}
}

func TestConstantCondition(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"if true", `func f() { if (true) { return 1; } }`, 1},
		{"self compare", `func f(x as int) { if ($x == $x) { return 1; } }`, 1},
		{"self compare neq", `func f(x as int) { if ($x != $x) { return 1; } }`, 1},
		{"while true no escape", `func f() { while (true) { def a as int init 1; } }`, 1},
		{"while true with break", `func f() { while (true) { break; } }`, 0},
		{"while true with return", `func f() { while (true) { return 1; } }`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrc(t, c.src, only("L105"), lint.DefaultConfig())
			if got := countID(diags, "L105"); got != c.want {
				t.Fatalf("L105 count = %d, want %d (%v)", got, c.want, diags)
			}
		})
	}
}

func TestRemovedApi(t *testing.T) {
	diags := lintSrc(t, `use core;`, only("L302"), lint.DefaultConfig())
	if countID(diags, "L302") != 1 {
		t.Fatalf("expected one L302, got %v", diags)
	}
	clean := lintSrc(t, `use io;`, only("L302"), lint.DefaultConfig())
	if len(clean) != 0 {
		t.Fatalf("expected no L302 for a live library, got %v", clean)
	}
}

func TestLineTooLong(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.MaxLineLength = 20
	src := "def x as int init 123456789;\ndef y as int;\n"
	diags := lintSrc(t, src, only("L203"), cfg)
	if countID(diags, "L203") != 1 {
		t.Fatalf("expected one L203 (only the long line), got %v", diags)
	}
	if diags[0].Line != 1 {
		t.Fatalf("L203 line = %d, want 1", diags[0].Line)
	}
}

func TestSuppression(t *testing.T) {
	t.Run("line directive", func(t *testing.T) {
		src := "func f() { def dead as int init 2;   # lint-disable: L101\nreturn 0; }"
		diags := lintSrc(t, src, only("L101"), lint.DefaultConfig())
		if len(diags) != 0 {
			t.Fatalf("expected suppressed, got %v", diags)
		}
	})
	t.Run("file directive", func(t *testing.T) {
		src := "# lint-disable-file: L101\nfunc f() { def dead as int init 2; return 0; }"
		diags := lintSrc(t, src, only("L101"), lint.DefaultConfig())
		if len(diags) != 0 {
			t.Fatalf("expected file-suppressed, got %v", diags)
		}
	})
	t.Run("wrong id not suppressed", func(t *testing.T) {
		src := "func f() { def dead as int init 2;   # lint-disable: L102\nreturn 0; }"
		diags := lintSrc(t, src, only("L101"), lint.DefaultConfig())
		if countID(diags, "L101") != 1 {
			t.Fatalf("expected L101 still reported, got %v", diags)
		}
	})
	t.Run("unknown id is an L004 finding", func(t *testing.T) {
		// A bad directive is reported (L004) and suppresses nothing, so the
		// finding it meant to silence still shows too: continue-and-report.
		src := "func f() { def dead as int init 2;   # lint-disable: L999\nreturn 0; }"
		diags := lintSrc(t, src, only("L101"), lint.DefaultConfig())
		if countID(diags, "L004") != 1 {
			t.Fatalf("expected one L004 invalid-directive, got %v", diags)
		}
		if countID(diags, "L101") != 1 {
			t.Fatalf("the unsuppressed L101 should still show, got %v", diags)
		}
	})
	t.Run("empty directive is an L004 finding", func(t *testing.T) {
		src := "func f() { def dead as int init 2;   # lint-disable:\nreturn 0; }"
		diags := lintSrc(t, src, only("L101"), lint.DefaultConfig())
		if countID(diags, "L004") != 1 {
			t.Fatalf("expected one L004 for a directive naming no IDs, got %v", diags)
		}
	})
	t.Run("double-hash is a comment, not a directive", func(t *testing.T) {
		src := "func f() { def dead as int init 2;   # # lint-disable: L999\nreturn 0; }"
		diags := lintSrc(t, src, only("L101"), lint.DefaultConfig())
		if countID(diags, "L004") != 0 {
			t.Fatalf("a doubled-hash comment must not parse as a directive, got %v", diags)
		}
	})
}

func TestResolveSelection(t *testing.T) {
	t.Run("default enables checks, not source errors", func(t *testing.T) {
		set, err := lint.ResolveSelection("", false, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !set["L101"] || !set["L302"] {
			t.Fatalf("default should enable the checks, got %v", set)
		}
		if set["L001"] || set["L004"] {
			t.Fatalf("default must not enable L0nn source errors, got %v", set)
		}
	})
	t.Run("exclude", func(t *testing.T) {
		set, err := lint.ResolveSelection("!L201,!L202", true, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !set["L101"] || set["L201"] || set["L202"] {
			t.Fatalf("exclude spec wrong: %v", set)
		}
	})
	t.Run("include only", func(t *testing.T) {
		set, err := lint.ResolveSelection("L101,L102", true, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !set["L101"] || !set["L102"] || set["L103"] {
			t.Fatalf("include spec wrong: %v", set)
		}
	})
	t.Run("mixed direction errors", func(t *testing.T) {
		if _, err := lint.ResolveSelection("L101,!L102", true, "", false); err == nil {
			t.Fatal("expected error mixing include and exclude")
		}
	})
	t.Run("unknown id errors", func(t *testing.T) {
		if _, err := lint.ResolveSelection("L999", true, "", false); err == nil {
			t.Fatal("expected error for unknown ID")
		}
	})
	t.Run("source-error id is not selectable", func(t *testing.T) {
		if _, err := lint.ResolveSelection("L001", true, "", false); err == nil {
			t.Fatal("expected error selecting an always-on source-error class")
		}
	})
	t.Run("dotfile used when no flag", func(t *testing.T) {
		set, err := lint.ResolveSelection("", false, "L101  # only this\n", true)
		if err != nil {
			t.Fatal(err)
		}
		if !set["L101"] || set["L102"] {
			t.Fatalf("dotfile spec wrong: %v", set)
		}
	})
}

func TestKnownIDs(t *testing.T) {
	if n := len(lint.KnownIDs()); n != 14 {
		t.Fatalf("expected 14 known IDs (4 source + 10 checks), got %d", n)
	}
	if len(lint.Catalog()) != 14 {
		t.Fatalf("catalog should list all 14 IDs")
	}
}

// TestDeferLint proves the lint walkers descend into `defer CALL(args);` - a
// variable read only inside a deferred call's arguments is a real use (no L101
// false positive on the canonical `defer fs.close($f);` pattern), suppression
// does not overreach, and a spawn hiding in a defer argument still counts for
// L202 nesting.
func TestDeferLint(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("use only in defer arg is a use", func(t *testing.T) {
		src := `func f() { def msg as string init "x"; defer g($msg); }`
		diags := lintSrc(t, src, only("L101"), cfg)
		if got := countID(diags, "L101"); got != 0 {
			t.Fatalf("L101 count = %d, want 0 (defer arg is a use): %v", got, diags)
		}
	})
	t.Run("unused var beside a defer still flagged", func(t *testing.T) {
		src := `func f() { def dead as int init 1; def msg as string init "x"; defer g($msg); }`
		diags := lintSrc(t, src, only("L101"), cfg)
		if got := countID(diags, "L101"); got != 1 {
			t.Fatalf("L101 count = %d, want 1 (dead is still unused): %v", got, diags)
		}
	})
	t.Run("spawn in defer arg counts for nesting", func(t *testing.T) {
		nestCfg := lint.DefaultConfig()
		nestCfg.MaxNesting = 1
		src := `func f() { if (true) { defer g(spawn { return 1; }); } }`
		diags := lintSrc(t, src, only("L202"), nestCfg)
		if got := countID(diags, "L202"); got != 1 {
			t.Fatalf("L202 count = %d, want 1 (spawn body nests inside the if): %v", got, diags)
		}
	})
}
