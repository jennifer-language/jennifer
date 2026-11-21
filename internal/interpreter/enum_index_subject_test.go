// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// M29.28: a `match` subject that indexes a list or map resolves to the declared
// element / value type, so its arms can destructure enum payloads.

const indexSubjectEnum = `
use io;
def enum Tree { Leaf { v as int }, Node { l as Tree, r as Tree } };
def struct Holder { tree as Tree };
`

func TestMatchIndexedListSubject(t *testing.T) {
	out, err := run(t, indexSubjectEnum+`
def queue as list of Tree init [Tree.Leaf{ v: 1 }, Tree.Node{ l: Tree.Leaf{ v: 2 }, r: Tree.Leaf{ v: 3 } }];
def head as int init 0;
def sum as int init 0;
while ($head < len($queue)) {
    match ($queue[$head]) {
        when Leaf(lf) { $sum = $sum + $lf.v; }
        when Node(nd) { $queue[] = $nd.l; $queue[] = $nd.r; }
    }
    $head = $head + 1;
}
io.printf("%d\n", $sum);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "6\n" {
		t.Errorf("got %q, want %q", out, "6\n")
	}
}

func TestMatchIndexedMapAndNestedSubjects(t *testing.T) {
	out, err := run(t, indexSubjectEnum+`
func describe(byName as map of string to Holder, grid as list of list of Tree) {
    def n as int init 0;
    match ($byName["x"].tree) {
        when Leaf(lf) { $n = $lf.v * 10; }
        when Node(nd) { $n = -1; }
    }
    match ($grid[1][0]) {
        when Leaf(lf) { $n = $n + $lf.v; }
        when Node(nd) { $n = -2; }
    }
    return $n;
}
def m as map of string to Tree init {"a": Tree.Leaf{ v: 7 }};
match ($m["a"]) {
    when Leaf(lf) { io.printf("map %d\n", $lf.v); }
    when Node(nd) { io.printf("map node\n"); }
}
io.printf("%d\n", describe({"x": Holder{ tree: Tree.Leaf{ v: 4 } }}, [[Tree.Leaf{ v: 0 }], [Tree.Leaf{ v: 9 }]]));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "map 7\n49\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The subject is evaluated once, however many arms are tried.
func TestMatchIndexedSubjectEvaluatedOnce(t *testing.T) {
	out, err := run(t, indexSubjectEnum+`
def calls as int init 0;
func pick() {
    $calls = $calls + 1;
    return 1;
}
def xs as list of Tree init [Tree.Leaf{ v: 1 }, Tree.Node{ l: Tree.Leaf{ v: 2 }, r: Tree.Leaf{ v: 3 } }];
match ($xs[pick()]) {
    when Leaf(lf) { io.printf("leaf\n"); }
    when Node(nd) { io.printf("node\n"); }
}
io.printf("%d\n", $calls);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "node\n1\n" {
		t.Errorf("got %q, want %q", out, "node\n1\n")
	}
}

func TestMatchIndexedSubjectExhaustiveness(t *testing.T) {
	_, err := run(t, indexSubjectEnum+`
def xs as list of Tree init [Tree.Leaf{ v: 1 }];
match ($xs[0]) {
    when Leaf(lf) { io.printf("leaf\n"); }
}
`)
	if err == nil || !strings.Contains(err.Error(), "not exhaustive") || !strings.Contains(err.Error(), "Node") {
		t.Fatalf("expected the missing-variant error, got %v", err)
	}
}

func TestMatchIndexedSubjectInSpawn(t *testing.T) {
	out, runErr, _ := runSpawn(t, indexSubjectEnum+`
def grid as list of list of Tree init [[Tree.Leaf{ v: 5 }]];
spawn {
    def byName as map of string to Holder init {"x": Holder{ tree: Tree.Leaf{ v: 6 } }};
    match ($grid[0][0]) {
        when Leaf(lf) { io.printf("%d\n", $lf.v); }
        when Node(nd) { io.printf("node\n"); }
    }
    match ($byName["x"].tree) {
        when Leaf(lf) { io.printf("%d\n", $lf.v); }
        when Node(nd) { io.printf("node\n"); }
    }
};
`)
	if runErr != nil {
		t.Fatalf("err: %v", runErr)
	}
	if !strings.Contains(out, "5\n6\n") {
		t.Errorf("got %q, want it to contain 5 and 6", out)
	}
	_, err := run(t, indexSubjectEnum+`
def xs as list of Tree init [Tree.Leaf{ v: 1 }];
def t as task of string init spawn {
    match ($xs[0]) {
        when Leaf(lf) { return "leaf"; }
    }
    return "fell-through";
};
`)
	if err == nil || !strings.Contains(err.Error(), "not exhaustive") {
		t.Fatalf("expected a non-exhaustive error inside a spawn body, got %v", err)
	}
}

// A string index and a call result have no declared enum type: still refused
// with the assign-to-a-variable hint.
func TestMatchIndexedSubjectStillRefused(t *testing.T) {
	for name, src := range map[string]string{
		"string index": `
def s as string init "ab";
match ($s[0]) {
    when Leaf(lf) { io.printf("leaf\n"); }
    when Node(nd) { io.printf("node\n"); }
}
`,
		"string element": `
def ss as list of string init ["ab"];
match ($ss[0]) {
    when Leaf(lf) { io.printf("leaf\n"); }
    when Node(nd) { io.printf("node\n"); }
}
`,
		"call result": `
func mk() { return [Tree.Leaf{ v: 1 }]; }
match (mk()[0]) {
    when Leaf(lf) { io.printf("leaf\n"); }
    when Node(nd) { io.printf("node\n"); }
}
`,
	} {
		_, err := run(t, indexSubjectEnum+src)
		if err == nil || !strings.Contains(err.Error(), "assign the subject to a variable first") {
			t.Errorf("%s: expected the not-statically-an-enum error, got %v", name, err)
		}
	}
}
