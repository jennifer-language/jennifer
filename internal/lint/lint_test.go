// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"fmt"
	"strings"
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
		{"used only as match subject", `func f(x as int) { def n as int init $x; match ($n) { when 1 { return 1; } else { return 0; } } return 0; }`, 0},
		{"used only in a match arm value", `func f(x as int) { def lim as int init $x; match (2) { when $lim { return 1; } else { return 0; } } return 0; }`, 0},
		{"used only in a match arm body", `func f(x as int) { def y as int init $x; match (1) { when 1 { return $y; } else { return 0; } } return 0; }`, 0},
		{"unused inside a match arm still flagged", `func f() { match (1) { when 1 { def dead as int init 2; return 0; } else { return 0; } } return 0; }`, 1},
		{"used only as a slice upper bound", `func f(xs as list of int) { def mid as int init 2; def s as list of int init $xs[0..$mid]; return len($s); }`, 0},
		{"used only as a slice lower bound", `func f(xs as list of int) { def m as int init 1; def s as list of int init $xs[$m..]; return len($s); }`, 0},
		{"used only in a range expression", `func f() { def hi as int init 3; def r as list of int init 0..$hi; return len($r); }`, 0},
		{"used only as a func-value callee", `func g() { return; } func f() { def fn as func init g; $fn(); return 0; }`, 0},
		{"used only as a func-value call arg", `func g(n as int) { return; } func f() { def fn as func init g; def a as int init 5; $fn($a); return 0; }`, 0},
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
		{"error-returning helper", `func wrap(e as Error) {
    if ($e.kind != "limit") { return $e; }
    return Error{kind: "x", message: "deep"};
}
func f() { try { risky(); } catch (e) { throw wrap($e); } }`, 0},
		{"helper via another helper", `func base() { return Error{kind: "x", message: "m"}; }
func wrap() { return base(); }
func f() { throw wrap(); }`, 0},
		{"helper with if/else returns", `func pick(a as bool) {
    if ($a) { return Error{kind: "a", message: "m"}; } else { return Error{kind: "b", message: "m"}; }
}
func f() { throw pick(true); }`, 0},
		{"helper returning a non-Error", `func wrap(e as Error) {
    if ($e.kind == "x") { return "boom"; }
    return $e;
}
func f() { try { risky(); } catch (e) { throw wrap($e); } }`, 1},
		{"helper that can fall off its end", `func wrap(a as bool) {
    if ($a) { return Error{kind: "x", message: "m"}; }
}
func f() { throw wrap(true); }`, 1},
		{"helper calling a non-Error helper", `func other() { return 5; }
func wrap() { return other(); }
func f() { throw wrap(); }`, 1},
		{"spawn return does not count", `func wrap() {
    def t as task of int init spawn { return 5; };
    return Error{kind: "x", message: "m"};
}
func f() { throw wrap(); }`, 0},
		{"unknown function", `func f() { throw nowhere(); }`, 1},
		{"error-shaped struct literal", `def struct HttpError { kind as string, message as string, status as int };
func f() { throw HttpError{kind: "api", message: "m", status: 404}; }`, 0},
		{"error-shaped struct variable", `def struct HttpError { kind as string, message as string, status as int };
func f(e as HttpError) { throw $e; }`, 0},
		{"helper returning an error-shaped struct", `def struct HttpError { kind as string, message as string, status as int };
func make(s as int) { return HttpError{kind: "api", message: "m", status: $s}; }
func f() { throw make(404); }`, 0},
		{"struct without message", `def struct Half { kind as string, status as int };
func f() { throw Half{kind: "api", status: 1}; }`, 1},
		{"struct with a non-string kind", `def struct Odd { kind as int, message as string };
func f() { throw Odd{kind: 1, message: "m"}; }`, 1},
		{"unloaded module literal naming kind and message", `import "./nowhere.j" as m;
func f() { throw m.E{kind: "x", message: "m", code: 3}; }`, 0},
		{"unloaded module literal without message", `import "./nowhere.j" as m;
func f() { throw m.E{kind: "x", code: 3}; }`, 1},
		{"library error struct variable", `use net;
func f(e as net.NetError) { throw $e; }`, 0},
		{"library error struct literal", `use net;
func f() { throw net.NetError{kind: "net", message: "m", op: "read", reason: "closed", sent: true, replied: false}; }`, 0},
		{"library struct that is no error", `use net;
func f(o as net.TLSOptions) { throw $o; }`, 1},
		{"enum variant", `def enum Fail { Bad { kind as string, message as string } };
func f() { throw Fail.Bad{kind: "x", message: "m"}; }`, 1},
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

// TestNestingCountsMatchArm confirms a match statement adds a nesting level and
// L202 counts blocks inside its arms (before the fix, match arms were skipped).
func TestNestingCountsMatchArm(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.MaxNesting = 1
	// match is one level; the if inside the arm is a second -> exceeds 1.
	src := `func f(x as int) { match ($x) { when 1 { if (true) { return 1; } } else { return 0; } } return 0; }`
	diags := lintSrc(t, src, only("L202"), cfg)
	if countID(diags, "L202") == 0 {
		t.Fatalf("expected L202 for nesting inside a match arm, got %v", diags)
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
		{"const cond inside a match arm is flagged", `func f(x as int) { match ($x) { when 1 { if (true) { return 1; } } else { return 0; } } return 0; }`, 1},
		{"while true escaping via a break in a match arm", `func f(x as int) { while (true) { match ($x) { when 1 { break; } else { } } } return 0; }`, 0},
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

func TestUnknownLibrary(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.Libraries = []string{"io", "math", "strings"}
	diags := lintSrc(t, "use io;\nuse strngs;\nuse core;\nuse math as m;\n", only("L115"), cfg)
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%s", d.Line, d.Message))
	}
	want := "2:unknown library `strngs` - did you mean `strings`? | 3:unknown library `core`"
	if strings.Join(got, " | ") != want {
		t.Fatalf("got %q, want %q", strings.Join(got, " | "), want)
	}
	// With no library list supplied the check is skipped, never guessed.
	if d := lintSrc(t, "use core;", only("L115"), lint.DefaultConfig()); len(d) != 0 {
		t.Fatalf("expected no L115 without a library list, got %v", d)
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

func TestInterpSlotComplexity(t *testing.T) {
	// A call in a slot is flagged; a variable / arithmetic slot is not.
	flagged := `use io;
func cost() { return 1; }
def n as int init 3;
io.printf("value {cost()}\n");
io.printf("ok {$n + 1}\n");
`
	diags := lintSrc(t, flagged, only("L204"), lint.DefaultConfig())
	if got := countID(diags, "L204"); got != 1 {
		t.Fatalf("expected one L204 (the call slot only), got %d: %v", got, diags)
	}
	// A field / index slot is trivial - no finding.
	clean := `use io;
def xs as list of int init [1, 2, 3];
io.printf("first {$xs[0]}\n");
`
	if d := lintSrc(t, clean, only("L204"), lint.DefaultConfig()); countID(d, "L204") != 0 {
		t.Fatalf("index-access slot should not be flagged, got %v", d)
	}
	// `len` is a keyword built-in, not a function call - no finding.
	lenSlot := `use io;
def steps as list of int init [1];
io.println("n={len($steps)}");
`
	if d := lintSrc(t, lenSlot, only("L204"), lint.DefaultConfig()); countID(d, "L204") != 0 {
		t.Fatalf("len() slot should not be flagged, got %v", d)
	}
	// A call inside the len operand still is.
	lenCall := `use io;
func xs() { return [1]; }
io.println("n={len(xs())}");
`
	if d := lintSrc(t, lenCall, only("L204"), lint.DefaultConfig()); countID(d, "L204") != 1 {
		t.Fatalf("call inside len() operand should be flagged once, got %v", d)
	}
}

// L116: a slot whose whole expression is a literal is the brace-quoting
// accident `["{", "}"]` (one string with the slot `", "`), never a real need.
func TestLiteralInterpSlot(t *testing.T) {
	flagged := []struct{ name, src string }{
		{"brace-list", `def braces as list of string init ["{", "}"];` + "\n"},
		// `f("{", "x")` alone is already a lex error (the slot runs on into `x"...`);
		// with a closing brace it lexes silently as f(<one string>, "x").
		{"call-args", "func f(a as string, b as string) { return $a + $b; }\ndef s as string init f(\"{\", \"}\", \"x\");\n"},
		{"raw-in-slot", "def s as string init \"a{'raw'}b\";\n"},
		{"int-in-slot", "def s as string init \"n{1}\";\n"},
	}
	for _, c := range flagged {
		t.Run(c.name, func(t *testing.T) {
			if d := lintSrc(t, c.src, only("L116"), lint.DefaultConfig()); countID(d, "L116") != 1 {
				t.Errorf("expected one L116, got %v", d)
			}
		})
	}
	clean := []struct{ name, src string }{
		{"variable", "def x as int init 1;\ndef s as string init \"{$x}\";\n"},
		{"arithmetic", "def a as int init 1;\ndef s as string init \"{$a + 1}\";\n"},
		{"raw-braces", "def braces as list of string init ['{', '}'];\n"},
		{"escaped-brace", "def braces as list of string init [\"\\{\", \"\\}\"];\n"},
		{"nested-interp", "def x as int init 1;\ndef s as string init \"a{\"b{$x}\"}\";\n"},
	}
	for _, c := range clean {
		t.Run(c.name, func(t *testing.T) {
			if d := lintSrc(t, c.src, only("L116"), lint.DefaultConfig()); countID(d, "L116") != 0 {
				t.Errorf("expected no L116, got %v", d)
			}
		})
	}
	// The finding says what was probably meant and how to write it.
	d := lintSrc(t, flagged[0].src, only("L116"), lint.DefaultConfig())
	if len(d) != 1 || !strings.Contains(d[0].Message, "`'{'`") || !strings.Contains(d[0].Message, "`\\{`") || d[0].Severity != lint.SeverityWarning {
		t.Fatalf("unexpected L116 finding: %v", d)
	}
}

func TestRequirementHeader(t *testing.T) {
	bad := []struct{ name, src string }{
		{"malformed", "# pragma-jennifer-version\nuse io;\n"},
		{"bad-version", "# pragma-jennifer-version: latest\nuse io;\n"},
		{"no-ge", "# pragma-jennifer-version: 0.25.0\nuse io;\n"},
		{"duplicate", "# pragma-jennifer-version: >=0.24.0\n# pragma-jennifer-version: >=0.25.0\nuse io;\n"},
		{"unknown-key", "# pragma-jennifer-platform: linux\nuse io;\n"},
		{"unknown-cap", "# pragma-jennifer-capability: nett\nuse io;\n"},
		{"caret-range", "# pragma-jennifer-version: ^0.25.0\nuse io;\n"},
		{"bound-not-above-floor", "# pragma-jennifer-version: >=0.26.0 <0.26.0\nuse io;\n"},
		{"inclusive-bound", "# pragma-jennifer-version: >=0.25.0 <=0.26.0\nuse io;\n"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if d := lintSrc(t, c.src, only("L303"), lint.DefaultConfig()); countID(d, "L303") != 1 {
				t.Errorf("expected one L303, got %v", d)
			}
		})
	}
	// A well-formed header and a pragma-free file are both clean (no false positive).
	clean := []string{
		"# pragma-jennifer-version: >=0.24.0\n# pragma-jennifer-capability: net\nuse io;\n",
		"# pragma-jennifer-version: >=0.25.0 <0.26.0\nuse io;\n",
		"# just an ordinary comment\nuse io;\n",
	}
	for i, src := range clean {
		if d := lintSrc(t, src, only("L303"), lint.DefaultConfig()); countID(d, "L303") != 0 {
			t.Errorf("clean case %d should have no L303, got %v", i, d)
		}
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

// Deep-nesting resolution: a global read only deep inside nested blocks (the
// O(1) global fast path) and a local read deep both resolve correctly, so
// neither is falsely flagged; an actually-unused local deep is still flagged.
func TestScopeResolutionDeepNesting(t *testing.T) {
	cfg := lint.DefaultConfig()
	used := `def g as int init 1;
func f() {
    def loc as int init 2;
    if (true) { if (true) { if (true) {
        def r as int init $g + $loc;
        return $r;
    } } }
    return 0;
}`
	if n := countID(lintSrc(t, used, only("L101"), cfg), "L101"); n != 0 {
		t.Errorf("all locals are used; expected 0 L101, got %d", n)
	}
	unused := `def g as int init 1;
func f() {
    if (true) { if (true) {
        def dead as int init $g;
        return 0;
    } }
    return 0;
}`
	if n := countID(lintSrc(t, unused, only("L101"), cfg), "L101"); n != 1 {
		t.Errorf("deep unused local should be flagged once, got %d", n)
	}
}

func TestUnusedImport(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"unused use flagged", `use io; use math; io.printf("hi");`, 1},
		{"all used", `use io; io.printf("%d", 3);`, 0},
		{"used only as a var type", `use time; def t as time.Time;`, 0},
		{"used only as a param type", `use time; func f(t as time.Time) { return; }`, 0},
		{"used only as a struct field type", `use time; def struct S { at as time.Time };`, 0},
		{"used only nested in a list type", `use time; def xs as list of time.Time;`, 0},
		{"used only as a namespaced value", `use json; def v as json.Value init json.map();`, 0},
		{"alias used", `use math as m; def x as float init m.sqrt(4.0);`, 0},
		{"alias unused flagged", `use math as m; def x as int init 1;`, 1},
		{"two unused", `use io; use math; use strings; def x as int init 1;`, 3},
		{"module import unused flagged", `import "./util.j" as u; def x as int init 1;`, 1},
		{"module import used", `import "./util.j" as u; def x as int init u.f();`, 0},
		{"module import used only as type", `import "./geo.j" as geo; func f(p as geo.Point) { return; }`, 0},
		{"bare module import unused flagged", `import "util.j"; def x as int init 1;`, 1},
		{"vendored deck never flagged", `import "@scope/pkg/"; def x as int init 1;`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrc(t, c.src, only("L106"), cfg)
			if got := countID(diags, "L106"); got != c.want {
				t.Errorf("L106 count = %d, want %d: %v", got, c.want, diags)
			}
		})
	}
}

func TestKnownIDs(t *testing.T) {
	if n := len(lint.KnownIDs()); n != 29 {
		t.Fatalf("expected 29 known IDs (4 source + 25 checks), got %d", n)
	}
	if len(lint.Catalog()) != 29 {
		t.Fatalf("catalog should list all 29 IDs")
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

func TestUndefinedCall(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("undefined bare call is flagged, defined one is not", func(t *testing.T) {
		src := `use io;
func known() { return 1; }
func caller() { io.printf("%d %d\n", known(), nope(1)); }`
		diags := lintSrc(t, src, only("L107"), cfg)
		if got := countID(diags, "L107"); got != 1 {
			t.Fatalf("L107 count = %d, want 1 (only `nope` is undefined): %v", got, diags)
		}
		if !strings.Contains(diags[0].Message, "nope") {
			t.Fatalf("L107 should name `nope`, got %q", diags[0].Message)
		}
	})
	t.Run("enum variant pattern in a match arm is not a call", func(t *testing.T) {
		// Unresolved here (lintSrc skips Resolve), so the arm stays a CallExpr -
		// the exact shape that must not be mistaken for an undefined function.
		src := `def enum Shape { Circle { r as int }, Square { s as int } };
func classify(sh as Shape) {
    match ($sh) {
        when Circle(c) { return 1; }
        when Square(s) { return 2; }
    }
    return 0;
}`
		if got := countID(lintSrc(t, src, only("L107"), cfg), "L107"); got != 0 {
			t.Fatalf("enum variant pattern flagged as undefined call: %d", got)
		}
	})
	t.Run("calling a known top-level binding is not an undefined call", func(t *testing.T) {
		src := `def const FOO as int init 5;
func c() { return FOO(); }`
		if got := countID(lintSrc(t, src, only("L107"), cfg), "L107"); got != 0 {
			t.Fatalf("call to a known top-level binding flagged as undefined: %d", got)
		}
	})
}

func TestUndefinedConstant(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("undefined constant read is flagged, defined one is not", func(t *testing.T) {
		src := `use io;
def const MAX as int init 10;
func f() { return MISSING_CONSTANT; }
func g() { return MAX; }`
		diags := lintSrc(t, src, only("L108"), cfg)
		if got := countID(diags, "L108"); got != 1 {
			t.Fatalf("L108 count = %d, want 1 (only MISSING_CONSTANT is undefined): %v", got, diags)
		}
		if !strings.Contains(diags[0].Message, "MISSING_CONSTANT") {
			t.Fatalf("L108 should name MISSING_CONSTANT, got %q", diags[0].Message)
		}
	})
	t.Run("a local constant and a func-value reference are not undefined", func(t *testing.T) {
		src := `func greet() { return 1; }
func a() { def const INNER as int init 2; return INNER; }
func b() { def h as func init greet; return $h(); }`
		if got := countID(lintSrc(t, src, only("L108"), cfg), "L108"); got != 0 {
			t.Fatalf("local const / func-value ref flagged as undefined constant: %d", got)
		}
	})
	t.Run("an undefined function used as a value is flagged", func(t *testing.T) {
		// The provider pattern: a struct of func values naming a method that was
		// never written runs until the line executes; lint must catch it.
		src := `def struct Table { greet as func };
func build() { return Table{ greet: greetingOf }; }
func f() { def g as func init notdefined; return $g(); }`
		diags := lintSrc(t, src, only("L108"), cfg)
		if got := countID(diags, "L108"); got != 2 {
			t.Fatalf("L108 count = %d, want 2 (greetingOf, notdefined): %v", got, diags)
		}
		for i, name := range []string{"greetingOf", "notdefined"} {
			if !strings.Contains(diags[i].Message, "undefined name `"+name+"`") {
				t.Errorf("finding %d should name %s as undefined, got %q", i, name, diags[i].Message)
			}
		}
	})
	t.Run("a bare variable or parameter gets the missing-sigil hint", func(t *testing.T) {
		src := `func f(count as int) { def total as int init 1; return count + total; }`
		diags := lintSrc(t, src, only("L108"), cfg)
		if got := countID(diags, "L108"); got != 2 {
			t.Fatalf("L108 count = %d, want 2: %v", got, diags)
		}
		for i, name := range []string{"count", "total"} {
			if !strings.Contains(diags[i].Message, "did you mean `$"+name+"`") {
				t.Errorf("finding %d should hint `$%s`, got %q", i, name, diags[i].Message)
			}
		}
	})
	t.Run("enum pattern binders and variant heads are not references", func(t *testing.T) {
		src := `def enum Shape { Empty, Circle { r as float } };
func area(s as Shape) {
    match ($s) {
        when Circle(c) { return $c.r; }
        when Empty { return 0.0; }
    }
    return 0.0;
}`
		if got := countID(lintSrc(t, src, only("L108"), cfg), "L108"); got != 0 {
			t.Fatalf("enum pattern binder / variant flagged as undefined: %d", got)
		}
	})
}

func TestUndeclaredNamespace(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("a namespace used without use/import is flagged once", func(t *testing.T) {
		src := `export func shout(s as string) { return strings.upper(strings.trim($s)); }`
		diags := lintSrc(t, src, only("L112"), cfg)
		if got := countID(diags, "L112"); got != 1 {
			t.Fatalf("L112 count = %d, want 1 (one report for `strings`): %v", got, diags)
		}
		if !strings.Contains(diags[0].Message, "strings") {
			t.Fatalf("L112 should name `strings`, got %q", diags[0].Message)
		}
	})
	t.Run("a declared use / import is not flagged", func(t *testing.T) {
		src := `use strings;
import "./lib.j" as lib;
func f() { return strings.upper(lib.pack("x")); }`
		if got := countID(lintSrc(t, src, only("L112"), cfg), "L112"); got != 0 {
			t.Fatalf("declared namespace flagged as undeclared: %d", got)
		}
	})
	t.Run("a local enum value and struct literal are not undeclared namespaces", func(t *testing.T) {
		src := `def enum Shape { Circle, Square };
def struct Point { x as int };
func f() { def s as Shape init Shape.Circle; def p as Point init Point{x: 1}; return $p.x; }`
		if got := countID(lintSrc(t, src, only("L112"), cfg), "L112"); got != 0 {
			t.Fatalf("local enum/struct prefix flagged as undeclared namespace: %d", got)
		}
	})
	t.Run("an aliased use is matched by its alias, and the canonical name is undeclared", func(t *testing.T) {
		src := `use strings as str;
func f() { return str.upper("x"); }`
		if got := countID(lintSrc(t, src, only("L112"), cfg), "L112"); got != 0 {
			t.Fatalf("aliased namespace flagged: %d", got)
		}
	})
}

func TestCallArity(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("too few and too many args are both flagged; a correct call is not", func(t *testing.T) {
		src := `func paint(title as string, rows as int) { return $title; }
func ok() { return paint("x", 3); }
func few() { return paint("x"); }
func many() { return paint("x", 3, 9); }`
		diags := lintSrc(t, src, only("L109"), cfg)
		if got := countID(diags, "L109"); got != 2 {
			t.Fatalf("L109 count = %d, want 2 (few + many): %v", got, diags)
		}
	})
	t.Run("a func-value binding call and an undefined call are not arity-checked", func(t *testing.T) {
		// A `def ... as func` call has no static arity; an undefined bare call is
		// L107's domain. Neither should produce an L109.
		src := `func greet(a as int) { return $a; }
func f() { def g as func init greet; return $g(1, 2, 3); }
func h() { return nope(1, 2); }`
		if got := countID(lintSrc(t, src, only("L109"), cfg), "L109"); got != 0 {
			t.Fatalf("func-value / undefined call flagged for arity: %d", got)
		}
	})
	t.Run("an enum variant pattern in a match arm is not an arity error", func(t *testing.T) {
		src := `def enum Shape { Circle { r as int }, Square { s as int } };
func classify(sh as Shape) {
    match ($sh) {
        when Circle(c) { return 1; }
        when Square(s) { return 2; }
    }
    return 0;
}`
		if got := countID(lintSrc(t, src, only("L109"), cfg), "L109"); got != 0 {
			t.Fatalf("enum variant pattern flagged for arity: %d", got)
		}
	})
}

func TestAccumulatorParam(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"append and return", `func f(out as list of int, x as int) { $out[] = $x; return $out; }`, 1},
		{"map index-write and return", `func f(acc as map of string to int, k as string, v as int) { $acc[$k] = $v; return $acc; }`, 1},
		{"reassign and return", `func f(xs as list of int) { $xs = [1]; return $xs; }`, 1},
		{"chained field-write and return", `func f(rows as list of int, i as int) { $rows[$i] = 9; return $rows; }`, 1},
		{"return fresh, not the param", `func f(xs as list of int) { def ys as list of int init []; for (def x in $xs) { $ys[] = $x; } return $ys; }`, 0},
		{"mutates but does not return it", `func f(out as list of int, n as int) { def i as int init 0; while ($i < $n) { $out[] = $i; $i = $i + 1; } }`, 0},
		{"returns but never mutates it", `func f(xs as list of int) { if (len($xs) > 0) { return $xs; } return [0]; }`, 0},
		{"scalar param not flagged", `func f(n as int) { return $n; }`, 0},
		{"write inside spawn body does not count", `func f(out as list of int) { def t as task of int init spawn { $out[] = 1; return 0; }; return $out; }`, 0},
		{"write in nested block counts", `func f(out as list of int, b as bool) { if ($b) { for (def i in 0..3) { $out[] = $i; } } return $out; }`, 1},
		// struct / bytes parameters count only when grown with `[] =`
		{"struct param grown and returned", `def struct Doc { items as list of int };
func f(d as Doc, x as int) { $d.items[] = $x; return $d; }`, 1},
		{"struct builder sets a field: not an accumulator", `def struct Client { timeout as int };
func withTimeout(c as Client, ms as int) { $c.timeout = $ms; return $c; }`, 0},
		{"bytes param grown and returned", `func put(b as bytes, v as int) { $b[] = $v; return $b; }`, 1},
		// copy-to-local: `def out init $param`, grow out, return out
		{"local copy of a list param grown and returned", `func f(xs as list of int, x as int) { def out as list of int init $xs; $out[] = $x; return $out; }`, 1},
		{"local copy of a struct grown through a field", `def struct Doc { items as list of int };
func merge(d as Doc, x as int) { def out as Doc init $d; $out.items[] = $x; return $out; }`, 1},
		{"local copy builder sets a field: not an accumulator", `def struct Client { timeout as int };
func withTimeout(c as Client, ms as int) { def out as Client init $c; $out.timeout = $ms; return $out; }`, 0},
		{"local copy grown but a fresh value returned", `func f(xs as list of int) { def out as list of int init $xs; $out[] = 1; return [len($out)]; }`, 0},
		{"local copy of a scalar param not tracked", `func f(n as int) { def m as int init $n; $m = $m + 1; return $m; }`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrc(t, c.src, only("L205"), cfg)
			if got := countID(diags, "L205"); got != c.want {
				t.Errorf("L205 count = %d, want %d", got, c.want)
			}
		})
	}
}

// TestAccumulatorParamCallSites covers L205's call-site exemption, which reads
// the resolver's MoveArg / ReturnsOwned: a function whose every call is the
// in-place `$v = f(..., $v, ...)` form (or passes a constant literal) is not
// reported; any other call keeps the finding.
func TestAccumulatorParamCallSites(t *testing.T) {
	cfg := lint.DefaultConfig()
	const flatten = `def enum Item { Num { v as int }, Nest { items as list of Item } };
func flatten(item as Item, out as list of int) {
    match ($item) {
        when Num(num) { $out[] = $num.v; }
        when Nest(nest) { for (def i in $nest.items) { $out = flatten($i, $out); } }
    }
    return $out;
}
`
	const add = "func add(acc as list of int, x as int) { $acc[] = $x; return $acc; }\n"
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"recursive, called with a fresh literal", flatten + `def input as Item init Item.Nest{ items: [Item.Num{ v: 1 }] };
def r as list of int init flatten($input, []);`, 0},
		{"every call moves the accumulator", add + `func run(n as int) {
    def acc as list of int init [];
    for (def i in 0..$n) { $acc = add($acc, $i); }
    return $acc;
}
def r as list of int init run(3);`, 0},
		{"a struct literal of constants", `def struct Acc { xs as list of int };
func grow(a as Acc, x as int) { $a.xs[] = $x; return $a; }
def r as Acc init grow(Acc{ xs: [1, 2] }, 3);`, 0},
		{"never called: callers unseen", add, 1},
		{"top-level loop threads a global", add + `def acc as list of int init [];
for (def i in 0..3) { $acc = add($acc, $i); }`, 1},
		{"inside a try", add + `func run(n as int) {
    def acc as list of int init [];
    try { for (def i in 0..$n) { $acc = add($acc, $i); } } catch (e) { return []; }
    return $acc;
}`, 1},
		{"a global accumulator inside a function", add + `def acc as list of int init [];
func run(n as int) { for (def i in 0..$n) { $acc = add($acc, $i); } }`, 1},
		{"result stored elsewhere", add + `func run(xs as list of int) { def ys as list of int init add($xs, 1); return $ys; }`, 1},
		{"a literal reading a variable", add + `func run(x as int) { return add([$x], 1); }`, 1},
		{"used as a func value", add + `func run() { def g as func init add; return add([], 1); }`, 1},
		{"exported", "export " + add + `func run() { return add([], 1); }`, 1},
		{"recursive call inside a spawn", flatten + `def input as Item init Item.Num{ v: 1 };
def t as task of list of int init spawn { return flatten($input, []); };`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L205"), cfg)
			if got := countID(diags, "L205"); got != c.want {
				t.Errorf("L205 count = %d, want %d: %v", got, c.want, diags)
			}
		})
	}
}

// lintSrcResolved is lintSrc after parser.Resolve, as `jennifer lint` runs it.
func lintSrcResolved(t *testing.T, src string, enabled map[string]bool, cfg lint.Config) []lint.Diagnostic {
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
	if err := parser.Resolve(prog); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return lint.Check(prog, raw, src, "test.j", enabled, cfg)
}

func TestStructLitFields(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("an unknown and a missing field are flagged; a correct literal is not", func(t *testing.T) {
		src := `def struct P { x as int, z as int };
func ok() { return P{ x: 1, z: 2 }; }
func extra(flag as bool) {
    if ($flag) {
        return P{ x: 1, z: 2, y: 3 };
    }
    return P{ x: 0 };
}`
		diags := lintSrc(t, src, only("L113"), cfg)
		if got := countID(diags, "L113"); got != 2 {
			t.Fatalf("L113 count = %d, want 2 (y unknown + z missing): %v", got, diags)
		}
	})
	t.Run("enum variant payloads and variant names are checked", func(t *testing.T) {
		src := `def enum Shape { Empty, Circle { r as int } };
func ok() { return Shape.Circle{ r: 1 }; }
func bad() { return Shape.Circle{ radius: 1 }; }
func none() { return Shape.Square{ r: 1 }; }
func bare() { return Shape.Empty; }`
		if got := countID(lintSrc(t, src, only("L113"), cfg), "L113"); got != 3 {
			t.Fatalf("L113 count = %d, want 3 (radius unknown, r missing, no Square)", got)
		}
	})
	t.Run("library and module struct literals are left to the load-time check", func(t *testing.T) {
		src := `use os;
import "./m.j" as m;
func f() { return os.Result{ exitCode: 1 }; }
func g() { return m.Point{ q: 1 }; }`
		if got := countID(lintSrc(t, src, only("L113"), cfg), "L113"); got != 0 {
			t.Fatalf("a non-local literal was flagged: %d", got)
		}
	})
}

// A line directive also covers every line of a multi-line token that has no
// room for one: a string or block comment ending on the directive's line, and a
// docblock right below a directive standing alone. A line comment may end in a
// directive; an embedded marker that does not parse is ordinary prose.
func TestSuppressionReachesMultiLineTokens(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.MaxLineLength = 40
	long := strings.Repeat("x", 50)
	cases := []struct {
		name, src string
		want      int
	}{
		{"multi-line string, directive where it ends",
			"def t as string init '\n" + long + "\n'; # lint-disable: L203\n", 0},
		{"multi-line string without a directive",
			"def t as string init '\n" + long + "\n';\n", 1},
		{"docblock below a lone directive",
			"# lint-disable: L203\n/**\n * " + long + "\n */\ndef const A as int init 1;\n", 0},
		{"docblock after the closing line of a block comment",
			"/**\n * " + long + "\n */ # lint-disable: L203\ndef const A as int init 1;\n", 0},
		{"a trailing directive does not reach the next docblock",
			"def const A as int init 1; # lint-disable: L203\n/**\n * " + long + "\n */\ndef const B as int init 1;\n", 1},
		{"directive at the end of a long line comment",
			"# " + long + " # lint-disable: L203\ndef const A as int init 1;\n", 0},
		{"an embedded file directive is not honoured",
			"def const A as int init 1; # note # lint-disable-file: L203\ndef const B as string init \"" + long + "\";\n", 2},
		{"prose mentioning a directive stays a comment",
			"# write # lint-disable: L203 at the end " + long + "\ndef const A as int init 1;\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrc(t, c.src, only("L203"), cfg)
			if got := countID(diags, "L203"); got != c.want {
				t.Errorf("L203 count = %d, want %d: %v", got, c.want, diags)
			}
			if countID(diags, "L004") != 0 {
				t.Errorf("unexpected L004: %v", diags)
			}
		})
	}
}

// L203 does not flag a comment line that cannot be wrapped: a Markdown table
// row, or a line whose overflow is one URL or code span. Code, and comment
// prose that could wrap, are still flagged.
func TestLineTooLongExemptions(t *testing.T) {
	cfg := lint.DefaultConfig()
	cfg.MaxLineLength = 30
	exempt := []string{
		"/**\n * | a | b | c | d | e | f | g | h | i |\n */\ndef const A as int init 1;\n",
		"# see https://example.com/a/long/path/here\ndef const A as int init 1;\n",
		"# run `jennifer lint --checks=L203`\ndef const A as int init 1;\n",
	}
	for i, src := range exempt {
		if d := lintSrc(t, src, only("L203"), cfg); countID(d, "L203") != 0 {
			t.Errorf("exempt case %d flagged: %v", i, d)
		}
	}
	flagged := []string{
		"# a long comment that could simply be wrapped\ndef const A as int init 1;\n",
		// a short URL or code span that would fit on a line of its own
		"# prose running past the limit https://e.x/y\ndef const A as int init 1;\n",
		"# prose running past the limit, then `a b`\ndef const A as int init 1;\n",
		"def u as string init \"https://example.com/a/long/path\";\n",
	}
	for i, src := range flagged {
		if d := lintSrc(t, src, only("L203"), cfg); countID(d, "L203") != 1 {
			t.Errorf("flagged case %d: %v", i, d)
		}
	}
}

// Calls and literals into an imported module are checked against the module's
// exported surface (Config.Modules): arity (L109), struct fields and enum
// variants (L113). A private or unknown member is L114.
func TestModuleSurfaceChecks(t *testing.T) {
	mod, err := parser.Parse(`export func two(a as int, b as int) { return $a + $b; }
func hidden(a as int) { return $a; }
export def struct P { x as int };
export def enum Shape { Dot, Circle { r as int } };`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := lint.DefaultConfig()
	cfg.Modules = map[string]*parser.Program{"m": mod}
	src := `import "./m.j" as m;
def a as int init m.two(1);
def b as int init m.two(1, 2);
def c as int init m.hidden(1, 2, 3);
def p as m.P init m.P{x: 1, y: 2};
def q as m.P init m.P{};
def s as m.Shape init m.Shape.Circle{r: 1, q: 2};
def u as m.Shape init m.Shape.Square{w: 1};
def v as m.Shape init m.Shape.Dot;`
	diags := lintSrc(t, src, only("L109", "L113"), cfg)
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%s", d.Line, d.ID))
	}
	want := []string{"2:L109", "5:L113", "6:L113", "7:L113", "8:L113"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("findings %v, want %v\n%v", got, want, diags)
	}
	// Without the module's program nothing is checked.
	if n := len(lintSrc(t, src, only("L109", "L113"), lint.DefaultConfig())); n != 0 {
		t.Errorf("unloaded module: %d findings, want 0", n)
	}
}

// A struct of a module in the system module dir (Config.Bundled) may leave
// fields out, as the interpreter allows, so L113 reports only an unknown field.
func TestBundledModuleLitMayLeaveFieldsOut(t *testing.T) {
	mod, err := parser.Parse(`export def struct P { x as int, y as int };`)
	if err != nil {
		t.Fatal(err)
	}
	src := `import "m.j";
def a as m.P init m.P{x: 1};
def b as m.P init m.P{x: 1, z: 2};`
	for _, c := range []struct {
		bundled bool
		want    string
	}{
		{true, "3:L113"},
		{false, "2:L113 3:L113 3:L113"},
	} {
		cfg := lint.DefaultConfig()
		cfg.Modules = map[string]*parser.Program{"m": mod}
		cfg.Bundled = map[string]bool{"m": c.bundled}
		var got []string
		for _, d := range lintSrc(t, src, only("L113"), cfg) {
			got = append(got, fmt.Sprintf("%d:%s", d.Line, d.ID))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("bundled=%t: findings %v, want %s", c.bundled, got, c.want)
		}
	}
}

// A library builtin's arity comes from its signature table (internal/libsig),
// optional and variadic parameters included, under the alias it is used by.
func TestCallArityLibrary(t *testing.T) {
	src := `use strings as s;
use io;
func f() {
    io.printf("%d %d\n", 1, 2);
    io.printf("x\n");
    def a as int init s.indexOf("ab", "b");
    def b as int init s.indexOf("ab", "b", 1);
    def c as int init s.indexOf("ab");
    def d as string init s.upper("a", "b");
    return $a + $b + $c + len($d);
}`
	diags := lintSrc(t, src, only("L109"), lint.DefaultConfig())
	var lines []int
	for _, d := range diags {
		lines = append(lines, d.Line)
	}
	if len(lines) != 2 || lines[0] != 8 || lines[1] != 9 {
		t.Fatalf("L109 lines = %v, want [8 9]: %v", lines, diags)
	}
}

// L110 compares an argument's static type to the parameter type of a same-file
// method or a library builtin, and only where the argument's type is certain.
func TestArgTypes(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("certain mismatches are flagged", func(t *testing.T) {
		src := `use strings;
use binary;
func take(s as string, xs as list of int) { return $s; }
func f(b as bytes, n as int) {
    take($b, [1]);
    take("x", [1, "two"]);
    take("x" + "y", 0..3);
    def u as string init strings.upper($n);
    def k as bool init binary.contains("s", $b);
    return $u + convert($k);
}
func convert(k as bool) { return "x"; }`
		diags := lintSrc(t, src, only("L110"), cfg)
		var lines []int
		for _, d := range diags {
			lines = append(lines, d.Line)
		}
		if fmt.Sprint(lines) != "[5 6 8 9]" {
			t.Fatalf("L110 lines = %v, want [5 6 8 9]: %v", lines, diags)
		}
	})
	t.Run("unknown or possibly-fitting arguments are not flagged", func(t *testing.T) {
		// A user method's result, a func-value call, a union-returning builtin,
		// an `any` parameter, and a stamped list of another element type handed
		// to a builtin (which checks elements one by one, so an empty one passes)
		// all stay silent.
		src := `use strings;
use math;
use io;
func take(s as string) { return $s; }
func g() { return "x"; }
func f(h as func, ints as list of int) {
    take(g());
    take($h());
    io.printf("%v\n", math.abs(1));
    io.printf("%v %v\n", 1, [1]);
    def j as string init strings.join($ints, ",");
    for (def x in $ints) { io.printf("%d\n", $x); }
    return $j;
}`
		if diags := lintSrc(t, src, only("L110"), cfg); len(diags) != 0 {
			t.Fatalf("unexpected L110: %v", diags)
		}
	})
	t.Run("a catch binding holds whatever was thrown", func(t *testing.T) {
		// A catch binding is untyped, as the interpreter binds a non-Error
		// value: handing it to a parameter of a module's error struct type
		// is no L110, and rethrowing it is no L104.
		src := `def struct HttpError { kind as string, message as string, status as int };
func status(e as HttpError) { return $e.status; }
func f() {
    try {
        throw HttpError{kind: "api.http", message: "HTTP 404", status: 404};
    } catch (e) {
        if (status($e) == 500) {
            throw $e;
        }
    }
    return 0;
}`
		if diags := lintSrc(t, src, only("L110", "L104"), cfg); len(diags) != 0 {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
	})
}

// L111 compares a stored value's static type to the declared type of its slot.
func TestBindingTypes(t *testing.T) {
	cfg := lint.DefaultConfig()
	t.Run("certain mismatches are flagged", func(t *testing.T) {
		src := `use strings;
def struct P { x as int, tags as list of string };
def enum Sh { Empty, Circle { r as float } };
def const LIMIT as string init 3;
func f(p as P, names as list of string) {
    def a as float init 1 + 2;
    def b as list of int init [1, 2.5];
    def c as Sh init Sh.Circle{r: 1};
    def d as int init strings.split("a b", " ");
    $p.x = "one";
    $names[0] = 1;
    $names[] = true;
    def e as Error init Error{kind: 7, message: "m"};
    def q as P init P{x: 1, tags: [1]};
    for (def n in $names) { def k as int init $n; }
    $a = "x";
    return $a;
}`
		diags := lintSrc(t, src, only("L111"), cfg)
		var lines []int
		for _, d := range diags {
			lines = append(lines, d.Line)
		}
		if fmt.Sprint(lines) != "[4 6 7 8 9 10 11 12 13 14 15 16]" {
			t.Fatalf("L111 lines = %v, want [4 6 7 8 9 10 11 12 13 14 15 16]: %v", lines, diags)
		}
	})
	t.Run("matching or unknown values are not flagged", func(t *testing.T) {
		src := `def struct P { x as int };
def enum Sh { Empty, Circle { r as float } };
func g() { return 1; }
func f(h as func) {
    def a as int init g();
    def b as float init 1.0 // 2;
    def c as list of int init [];
    def d as map of string to int init {};
    def e as Sh init Sh.Empty;
    def k as func init g;
    def t as task of int init spawn { return 1; };
    def p as P init P{x: $h()};
    def r as list of int init 0..3;
    def s as Error init Error{kind: "k", message: "m"};
    return $a;
}`
		if diags := lintSrc(t, src, only("L111"), cfg); len(diags) != 0 {
			t.Fatalf("unexpected L111: %v", diags)
		}
	})
	t.Run("a bytes loop binds ints", func(t *testing.T) {
		src := `func f(b as bytes) {
    for (def x in $b) {
        def n as int init $x;
        def s as string init $x;
    }
}`
		diags := lintSrc(t, src, only("L111"), cfg)
		if len(diags) != 1 || diags[0].Line != 4 {
			t.Fatalf("L111 = %v, want one on line 4 (the string)", diags)
		}
	})
}

// L114 flags a `ns.name` its namespace does not have: a library builtin missing
// from the signature table, a constant or type missing from the registered
// tables, and a module member that is unknown or not exported. Each surface is
// checked only when it is available.
func TestUnknownMember(t *testing.T) {
	mod, err := parser.Parse(`export def const K as int init 1;
def const HIDDEN as int init 2;
export def struct P { x as int };
export func two() { return 2; }
func hidden() { return 3; }`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := lint.DefaultConfig()
	cfg.Modules = map[string]*parser.Program{"m": mod}
	cfg.LibConsts = map[string]parser.Type{"math.PI": {Kind: parser.TypeFloat}}
	cfg.LibStructs = map[string]*parser.StructDef{"time.Time": {Name: "Time"}}
	src := `use strings as s;
use math;
use time;
import "./m.j" as m;
func f(a as time.Tme, b as list of m.Q) {
    def x as string init s.uper("a");
    def y as float init math.PII + math.PI;
    def z as time.Time init time.Tme{};
    def w as int init m.two() + m.hidden() + m.K + m.HIDDEN + m.NOPE;
    def p as m.P init m.P{x: 1};
    return s.upper($x);
}`
	diags := lintSrc(t, src, only("L114"), cfg)
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%d", d.Line, d.Col))
	}
	want := "5:8 5:23 6:26 7:25 8:29 9:33 9:52 9:63"
	if strings.Join(got, " ") != want {
		t.Fatalf("L114 at %v, want %s\n%v", got, want, diags)
	}
	for _, frag := range []string{"did you mean `s.upper`?", "did you mean `math.PI`?", "did you mean `time.Time`?", "`m.hidden` is not exported", "`m.HIDDEN` is not exported", "constant `m.NOPE` does not exist"} {
		found := false
		for _, d := range diags {
			found = found || strings.Contains(d.Message, frag)
		}
		if !found {
			t.Errorf("no finding mentions %q: %v", frag, diags)
		}
	}
	// Without the module program and the library tables, only the signature
	// table's builtins are checked.
	diags = lintSrc(t, src, only("L114"), lint.DefaultConfig())
	if len(diags) != 1 || diags[0].Line != 6 {
		t.Errorf("surfaces unavailable: %v, want only the s.uper call", diags)
	}
}

// A parameter may share its name with a top-level variable (only a local `def`
// may not), and the interpreter binds the parameter: lint must type `$x` as the
// parameter's int, not the global's float, and count the read as the
// parameter's, not the global's.
func TestParamShadowingGlobalResolvesToParam(t *testing.T) {
	cfg := lint.DefaultConfig()
	src := `func g(v as int) { return $v; }
func f(x as int) { return g($x); }
def x as float init 0.5;
g(f(3));
`
	if diags := lintSrc(t, src, only("L110"), cfg); len(diags) != 0 {
		t.Fatalf("unexpected L110: %v", diags)
	}
	// A wrong-typed argument through the shadowing parameter still reports.
	src = `func g(v as float) { return $v; }
func f(x as int) { return g($x); }
def x as float init 0.5;
g(f(3));
`
	if diags := lintSrc(t, src, only("L110"), cfg); countID(diags, "L110") != 1 {
		t.Fatalf("want one L110, got %v", diags)
	}
}

// TestRemovedModules: L302 reports an import of a module removed from the
// bundle when it resolves to nothing (a file of that name on the search path
// is the user's and is left alone), and a call to a removed function of a
// bundled module, instead of L114; the same name in a non-bundled module, a
// local path and a current function are left alone.
func TestRemovedModules(t *testing.T) {
	kv, err := parser.Parse(`export func inProcessStore() { return 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	own, err := parser.Parse(`export func version() { return 1; }`)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		src     string
		modules map[string]*parser.Program
		bundled map[string]bool
		want    int
	}{
		{"a removed module, unresolved", "import \"gotify.j\" as gotify;\ndef v as int init gotify.version();", nil, nil, 1},
		{"a removed module by its stem", "import \"slack.j\";\ndef v as int init slack.version();", nil, nil, 1},
		{"a file of the user's own on the search path", "import \"gotify.j\" as gotify;\ndef v as int init gotify.version();", map[string]*parser.Program{"gotify": own}, nil, 0},
		{"a local path", "import \"./gotify.j\" as gotify;\ndef v as int init gotify.version();", nil, nil, 0},
		{"a removed function of a bundled module", "import \"kvstore.j\" as kvstore;\ndef v as int init kvstore.redisStore(1);", map[string]*parser.Program{"kvstore": kv}, map[string]bool{"kvstore": true}, 1},
		{"the same name in a module of the user's", "import \"kvstore.j\" as kvstore;\ndef v as int init kvstore.redisStore(1);", map[string]*parser.Program{"kvstore": kv}, nil, 0},
		{"a current function", "import \"kvstore.j\" as kvstore;\ndef v as int init kvstore.inProcessStore();", map[string]*parser.Program{"kvstore": kv}, map[string]bool{"kvstore": true}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := lint.DefaultConfig()
			cfg.Modules = c.modules
			cfg.Bundled = c.bundled
			diags := lintSrcResolved(t, c.src, only("L302", "L114"), cfg)
			if got := countID(diags, "L302"); got != c.want {
				t.Errorf("L302 count = %d, want %d: %v", got, c.want, diags)
			}
			if c.want > 0 && countID(diags, "L114") != 0 {
				t.Errorf("L114 also reported: %v", diags)
			}
		})
	}
}

// TestRemovedBuiltins: L302 reports a builtin removed in 0.28.0 (through an
// alias too) and the removed call shapes: one value to printf that is
// certainly not a string, range with two bounds, toString with a radix. A
// format string alone, a step, a value and current builtins are left alone.
// It is a warning, and the unknown-member (L114), arity (L109) and
// argument-type (L110) checks leave these calls to it.
func TestRemovedBuiltins(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"substring", "use strings;\ndef s as string init strings.substring(\"abc\", 1, 2);", 1},
		{"aliased lists.head", "use lists as l;\ndef xs as list of int init l.head([1, 2], 1);", 1},
		{"tail, pop and binary.slice", "use lists;\nuse binary;\ndef a as list of int init lists.tail([1], 1);\ndef b as list of int init lists.pop([1]);\ndef c as bytes init binary.slice(binary.make(2), 0, 1);", 3},
		{"printf of an int", "use io;\nio.printf(5);", 1},
		{"sprintf of a typed list", "use io;\ndef xs as list of int init [1];\ndef s as string init io.sprintf($xs);", 1},
		{"printf of a format only", "use io;\nio.printf(\"hi\\n\");", 0},
		{"printf of a string variable", "use io;\ndef s as string init \"x\";\nio.printf($s);", 0},
		{"printf with arguments", "use io;\nio.printf(\"%d\\n\", 5);", 0},
		{"range with two bounds", "use lists;\ndef xs as list of int init lists.range(0, 3);", 1},
		{"range with a step", "use lists;\ndef xs as list of int init lists.range(9, 0, -3);", 0},
		{"deriveKey's predecessors", "use crypto;\ndef k as bytes init crypto.pbkdf2(\"sha256\", crypto.randBytes(8), crypto.randBytes(16), 1000, 32);", 1},
		{"toString with a radix", "use convert;\ndef s as string init convert.toString(255, 16);", 1},
		{"toString of a value", "use convert;\ndef s as string init convert.toString(255);", 0},
		{"convert pair", "use convert;\ndef b as bytes init convert.bytesFromString(\"utf-8\", \"x\");", 1},
		{"current builtins", "use strings;\nuse lists;\ndef s as string init strings.trim(\" a \");\ndef xs as list of int init lists.reverse([1]);", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L302", "L114", "L109", "L110"), cfg)
			if got := countID(diags, "L302"); got != c.want {
				t.Errorf("L302 count = %d, want %d: %v", got, c.want, diags)
			}
			if n := countID(diags, "L114") + countID(diags, "L109") + countID(diags, "L110"); n != 0 {
				t.Errorf("L114/L109/L110 also reported: %v", diags)
			}
			for _, d := range diags {
				if d.ID == "L302" && d.Severity != lint.SeverityWarning {
					t.Errorf("L302 severity = %v, want warning", d.Severity)
				}
			}
		})
	}
}

// TestDeprecatedNetShapes: L301 reports a deprecated builtin (`net.eof`) and
// the deprecated `net` call shapes: a trailing timeoutMs on connect,
// connectTLS and startTLS (a lone second argument only when it is certainly
// an int), and the trailing idle timeout of readAll / readN. It is info, and
// the arity (L109) and argument-type (L110) checks accept the old shapes.
func TestDeprecatedNetShapes(t *testing.T) {
	cfg := lint.DefaultConfig()
	const head = "use net;\ndef c as net.Conn init net.connect(\"h:1\");\ndef o as net.TLSOptions;\n"
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"connect with a timeout", "use net;\ndef c as net.Conn init net.connect(\"h:1\", 500);", 1, "`net.connect(addr, timeoutMs)` is deprecated and removed in 0.29.0; use `net.connectWith(addr, net.ConnectOptions{timeoutMs: ...})`"},
		{"aliased connect with a timeout", "use net as n;\ndef c as n.Conn init n.connect(\"h:1\", 500);", 1, "`n.connectWith("},
		{"connect", "use net;\ndef c as net.Conn init net.connect(\"h:1\");", 0, ""},
		{"connectWith", "use net;\ndef c as net.Conn init net.connectWith(\"h:1\", net.ConnectOptions{timeoutMs: 500, tls: true});", 0, ""},
		{"connectTLS with an int", "use net;\ndef c as net.Conn init net.connectTLS(\"h:1\", 500);", 1, "`net.connectTLS(..., timeoutMs)`"},
		{"connectTLS with options", head + "def d as net.Conn init net.connectTLS(\"h:1\", $o);", 0, ""},
		{"connectTLS with options and an int", head + "def d as net.Conn init net.connectTLS(\"h:1\", $o, 500);", 1, "`net.connectTLS(..., timeoutMs)`"},
		{"startTLS with an int variable", head + "def ms as int init 500;\nnet.startTLS($c, $ms);", 1, "`net.startTLSWith(conn,"},
		{"startTLS with options", head + "net.startTLS($c, $o);", 0, ""},
		{"startTLS with options and an int", head + "net.startTLS($c, $o, 500);", 1, "`net.startTLS(..., timeoutMs)`"},
		{"startTLSWith", head + "net.startTLSWith($c, net.ConnectOptions{timeoutMs: 500});", 0, ""},
		{"readAll with an idle timeout", head + "def b as bytes init net.readAll($c, 100, 500);", 1, "`net.setIdleTimeout(conn, ms)` before `net.readAll(conn, maxBytes)`"},
		{"readAll with a cap", head + "def b as bytes init net.readAll($c, 100);", 0, ""},
		{"readN with an idle timeout", head + "def b as bytes init net.readN($c, 4, 500);", 1, "`net.readN(conn, n, idleTimeoutMs)`"},
		{"readN", head + "def b as bytes init net.readN($c, 4);", 0, ""},
		{"eof", head + "def e as bool init net.eof($c);", 1, "`net.eof` is deprecated and removed in 0.29.0; use an empty `net.readBytes`"},
		{"setIdleTimeout", head + "net.setIdleTimeout($c, 500);", 0, ""},
		{"lookupWith", "use net;\ndef a as list of string init net.lookupWith(\"localhost\", net.LookupOptions{timeoutMs: 500});", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L301", "L114", "L109", "L110"), cfg)
			if got := countID(diags, "L301"); got != c.want {
				t.Errorf("L301 count = %d, want %d: %v", got, c.want, diags)
			}
			if n := countID(diags, "L114") + countID(diags, "L109") + countID(diags, "L110"); n != 0 {
				t.Errorf("L114/L109/L110 also reported: %v", diags)
			}
			for _, d := range diags {
				if d.ID != "L301" {
					continue
				}
				if d.Severity != lint.SeverityInfo {
					t.Errorf("L301 severity = %v, want info", d.Severity)
				}
				if !strings.Contains(d.Message, c.frag) {
					t.Errorf("L301 message %q lacks %q", d.Message, c.frag)
				}
			}
		})
	}
}

// TestPrintfFormat: L117 parses a literal printf format as printf does at
// call time: a malformed verb or modifier, a verb count other than the value
// count, and a value whose static type its verb does not take are reported;
// a well-formed format, `%%`, `null=`, `%v` and a non-literal format are not.
func TestPrintfFormat(t *testing.T) {
	cfg := lint.DefaultConfig()
	cases := []struct {
		name string
		src  string
		want int
		msg  string
	}{
		{"well formed", "use io;\nio.printf(\"%d|pad=4 %s %f|prec=2 %% %t\\n\", 1, \"a\", 1.5, true);", 0, ""},
		{"C width", "use io;\nio.printf(\"%-20s\\n\", \"a\");", 1, "unknown format verb `%-`"},
		{"unknown verb", "use io;\nio.printf(\"%x\\n\", 1);", 1, "unknown format verb `%x`"},
		{"modifier the verb does not take", "use io;\nio.printf(\"%d|prec=2\\n\", 1);", 1, "`io.printf` format: "},
		{"bad base", "use io;\ndef s as string init io.sprintf(\"%d|base=3\", 1);", 1, "`io.sprintf` format: "},
		{"duplicate modifier", "use io;\nio.eprintf(\"%d|pad=2|pad=3\\n\", 1);", 1, "specified twice"},
		{"dangling percent", "use io;\nio.printf(\"100%\");", 1, "dangling"},
		{"too few values", "use io;\nio.printf(\"%d %d\\n\", 1);", 1, "2 verb(s) for 1 value(s)"},
		{"too many values", "use io;\nio.printf(\"%d\\n\", 1, 2);", 1, "1 verb(s) for 2 value(s)"},
		{"no verb, a value", "use io;\nio.printf(\"hi\\n\", 1);", 1, "0 verb(s) for 1 value(s)"},
		{"type mismatch", "use io;\nio.printf(\"%d %s\\n\", \"a\", 1);", 2, "` in the `io.printf` format takes "},
		{"float for %d", "use io;\ndef f as float init 1.5;\nio.printf(\"%d\\n\", $f);", 1, "takes int, got float"},
		{"%a of a list", "use io;\nio.printf(\"%a\\n\", [1, 2]);", 0, ""},
		{"%a of an int", "use io;\nio.printf(\"%a\\n\", 1);", 1, "takes a list or map, got int"},
		{"%v takes anything", "use io;\nio.printf(\"%v %v\\n\", 1, [1]);", 0, ""},
		{"null with null=", "use io;\nio.printf(\"%d|null=empty\\n\", null);", 0, ""},
		{"non-literal format", "use io;\ndef f as string init \"%x\";\nio.printf($f, 1);", 0, ""},
		{"interpolated format", "use io;\ndef n as int init 1;\nio.printf(\"{$n} %d\\n\", 2);", 0, ""},
		{"untyped value", "use io;\nuse json;\nio.printf(\"%d\\n\", json.asInt(json.decode(\"1\")));", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L117"), cfg)
			if got := countID(diags, "L117"); got != c.want {
				t.Fatalf("L117 count = %d, want %d: %v", got, c.want, diags)
			}
			for _, d := range diags {
				if !strings.Contains(d.Message, c.msg) {
					t.Errorf("message %q does not contain %q", d.Message, c.msg)
				}
				if d.Severity != lint.SeverityWarning {
					t.Errorf("severity = %v, want warning", d.Severity)
				}
			}
		})
	}
}
