// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"bytes"
	"fmt"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	jsonlib "jennifer-lang.dev/jennifer/internal/lib/json"
	listslib "jennifer-lang.dev/jennifer/internal/lib/lists"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// Values may nest at most limits.MaxNestingDepth container levels; a literal or
// write that would build a deeper one raises a catchable "limit" error, so no
// walk over a value (copy, ==, retag, display, encoders) can overrun the stack
// (M28.37). `N{kids: [...]}` is two levels per wrap (the struct and its list),
// and `N{kids: []}` is two levels on its own.

// nestPrelude declares the recursive struct and builds $n with `wraps` wraps
// of N{kids: []}, so $n nests 2 + 2*wraps levels.
func nestPrelude(wraps int) string {
	return fmt.Sprintf(`use io;
use json;
use lists;
def struct N {
    kids as list of N
};
def struct B {
    inner as N
};
def struct BL {
    items as list of N
};
def n as N init N{kids: []};
for (def i in 0..%d) {
    $n = N{kids: [$n]};
}
`, wraps)
}

func runNesting(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	jsonlib.Install(in)
	listslib.Install(in)
	err = in.Run(prog)
	return buf.String(), err
}

// atCapWraps is the wrap count that leaves $n exactly at the cap.
var atCapWraps = (limits.MaxNestingDepth - 2) / 2

func TestNestingAtCapBuildsAndWalks(t *testing.T) {
	if limits.MaxNestingDepth%2 != 0 {
		t.Skip("the fixtures assume an even cap")
	}
	out, err := runNesting(t, nestPrelude(atCapWraps)+`
def c as N init $n;
io.printf("eq %t\n", $c == $n);
$c.kids = [];
io.printf("copy %t %t\n", $c == $n, len($n.kids) == 1);
def s as string init json.encode($n);
io.printf("json %d\n", len($s));
`)
	if err != nil {
		t.Fatalf("a value at the cap must build and walk: %v", err)
	}
	// Each N encodes as {"kids":[...]}, 11 bytes, and $n holds atCapWraps+1.
	want := fmt.Sprintf("eq true\ncopy false true\njson %d\n", 11*(atCapWraps+1))
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestNestingOverCapIsCatchable covers each store that adds a level: a list,
// map and struct literal, an index write, a field write and an append, each
// one level past the cap from a value at the cap or one below it.
func TestNestingOverCapIsCatchable(t *testing.T) {
	if limits.MaxNestingDepth%2 != 0 {
		t.Skip("the fixtures assume an even cap")
	}
	cases := []struct {
		name, ok, over string
	}{
		// $n nests cap-2 levels: each "ok" store builds exactly the cap, each
		// "over" one level more.
		{"list literal", `def l as list of list of N init [[$n]];`, `def l as list of list of list of N init [[[$n]]];`},
		{"map literal", `def m as map of string to list of N init {"a": [$n]};`, `def m as map of string to list of list of N init {"a": [[$n]]};`},
		{"map key", `def m as map of list of N to int init {[$n]: 1};`, `def m as map of list of list of N to int init {[[$n]]: 1};`},
		{"struct literal", `def w as N init N{kids: [$n]};`, `def b as B init B{inner: N{kids: [$n]}};`},
		{"index write", `def l as list of list of N init [[]]; $l[0] = [$n];`, `def l as list of list of list of N init [[]]; $l[0] = [[$n]];`},
		{"field write", `def k as list of N init [$n]; def bl as BL init BL{items: []}; $bl.items = $k;`, `def k as list of N init [$n]; def b as B init B{inner: N{kids: []}}; $b.inner.kids = $k;`},
		{"append", `def w as N init N{kids: []}; $w.kids[] = $n;`, `def b as B init B{inner: N{kids: []}}; $b.inner.kids[] = $n;`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pre := nestPrelude(atCapWraps - 1)
			if _, err := runNesting(t, pre+c.ok); err != nil {
				t.Fatalf("at the cap: %v", err)
			}
			out, err := runNesting(t, pre+`try { `+c.over+` io.printf("no error\n"); } catch (e) { io.printf("%s: %s\n", $e.kind, $e.message); }
io.printf("after %d\n", len($n.kids));`)
			if err != nil {
				t.Fatalf("over the cap must be catchable: %v", err)
			}
			want := fmt.Sprintf("limit: value nests deeper than the %d-level limit\nafter 1\n", limits.MaxNestingDepth)
			if out != want {
				t.Fatalf("got %q, want %q", out, want)
			}
		})
	}
}

// TestNestingCachedHeightNeverStale covers the height a literal caches: a
// write into the value, or a builtin that derives its result from an argument
// (lists.push), makes it deeper than the cache said, so the next literal that
// embeds it must still see the real height.
func TestNestingCachedHeightNeverStale(t *testing.T) {
	if limits.MaxNestingDepth%2 != 0 {
		t.Skip("the fixtures assume an even cap")
	}
	cases := []struct{ name, grow, over string }{
		// $n nests cap-2; each grow leaves a shallow-cached value at cap-1 or cap.
		{"field write", `def w as N init N{kids: []}; $w.kids = [$n];`, `def x as list of N init [$w];`},
		{"append", `def w as N init N{kids: []}; $w.kids[] = $n;`, `def x as list of N init [$w];`},
		{"index write", `def w as list of N init [N{kids: []}]; $w[0] = $n;`, `def x as list of list of list of N init [[$w]];`},
		{"builtin", `def w as list of N init []; $w = lists.push($w, $n);`, `def x as list of list of list of N init [[$w]];`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runNesting(t, nestPrelude(atCapWraps-1)+c.grow+` try { `+c.over+` io.printf("no error\n"); } catch (e) { io.printf("%s\n", $e.kind); }`)
			if err != nil {
				t.Fatal(err)
			}
			if out != "limit\n" {
				t.Fatalf("got %q, want the limit error", out)
			}
		})
	}
}

// TestNestingOriginalRepro is the entry's case: about 1000 wraps, then a copy
// into a map literal. It used to crash jennifer-tiny and, at 100,000 wraps,
// overflow the default binary's stack; now the build loop stops at the cap.
func TestNestingOriginalRepro(t *testing.T) {
	for _, wraps := range []int{1000, 100000} {
		out, err := runNesting(t, `use io;
def struct N {
    kids as list of N
};
def n as N init N{kids: []};
try {
    for (def i in 0..`+fmt.Sprint(wraps)+`) {
        $n = N{kids: [$n]};
    }
    def m as map of string to N init {"a": $n};
} catch (e) {
    io.printf("%s\n", $e.kind);
}
def again as list of N init $n.kids;
io.printf("%t\n", $again[0] == $n.kids[0]);
`)
		if err != nil {
			t.Fatalf("%d wraps: %v", wraps, err)
		}
		if out != "limit\ntrue\n" {
			t.Fatalf("%d wraps: got %q", wraps, out)
		}
	}
}
