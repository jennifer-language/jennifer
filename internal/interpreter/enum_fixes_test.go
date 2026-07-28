// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// Single-file enum fixes: an enum type name spelled like a constant, the
// payload-less-with-braces form, and enum checking inside a spawn body.

// An enum whose type name is all uppercase (`RGB`) parses `RGB.Red` as field
// access on a constant - the `ORIGIN.x` deep-const form - because `RGB` matches
// the constant-name rule. It used to fail with "undefined name RGB" even though
// `def c as RGB;` and `match ($c)` both resolved the type fine.
func TestEnumCapsTypeNameConstructs(t *testing.T) {
	out, err := run(t, `
use io;
def enum RGB { Red { n as int }, Green, Blue };
def a as RGB init RGB.Red{ n: 5 };
def b as RGB init RGB.Blue;
def z as RGB;
io.printf("%v %v %v\n", $a, $b, $z);
match ($a) {
    when Red(p) { io.printf("red %d\n", $p.n); }
    when Green { io.printf("green\n"); }
    when Blue { io.printf("blue\n"); }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "RGB.Red{n: 5} RGB.Blue RGB.Red{n: 0}\nred 5\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The enum reading must not steal `CONST.field`, which is the older meaning of
// that shape: a real binding always wins.
func TestConstFieldAccessStillWorksAlongsideEnum(t *testing.T) {
	out, err := run(t, `
use io;
def struct Point { x as int };
def enum ORIGIN { x, y };
def const HOME as Point init Point{ x: 9 };
io.printf("%v %d\n", ORIGIN.x, HOME.x);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "ORIGIN.x 9\n" {
		t.Errorf("got %q, want %q", out, "ORIGIN.x 9\n")
	}
}

// A payload-less variant is written bare. Braces on it are the mirror of
// constructing a payloaded variant bare, and were silently accepted.
func TestEnumPayloadlessWithBracesRejected(t *testing.T) {
	_, err := run(t, `
def enum S { A { r as float }, Empty };
def e as S init S.Empty{};
`)
	if err == nil {
		t.Fatal("expected an error for braces on a payload-less variant")
	}
	if !strings.Contains(err.Error(), "has no payload") {
		t.Errorf("expected the no-payload error, got %v", err)
	}
}

// Spawn bodies are deliberately left unresolved for slot analysis. That must
// not extend to semantic analysis: a non-exhaustive enum match inside a spawn
// used to load and silently fall through.
func TestEnumSpawnMatchExhaustiveness(t *testing.T) {
	_, err := run(t, `
def enum S { A, B, C };
def t as task of string init spawn {
    def s as S init S.C;
    match ($s) {
        when A { return "a"; }
        when B { return "b"; }
    }
    return "fell-through";
};
`)
	if err == nil {
		t.Fatal("expected a non-exhaustive error inside a spawn body")
	}
	if !strings.Contains(err.Error(), "not exhaustive") || !strings.Contains(err.Error(), "C") {
		t.Errorf("expected the missing-variant error, got %v", err)
	}
}

// A misspelled variant inside a spawn is a load-time error naming the enum,
// not a runtime "undefined name".
func TestEnumSpawnUnknownVariant(t *testing.T) {
	_, err := run(t, `
def enum S { A, B };
def t as task of string init spawn {
    def s as S init S.A;
    match ($s) {
        when Nope { return "n"; }
        when B { return "b"; }
    }
    return "x";
};
`)
	if err == nil {
		t.Fatal("expected an unknown-variant error inside a spawn body")
	}
	if !strings.Contains(err.Error(), `"Nope" is not a variant`) {
		t.Errorf("expected the not-a-variant error, got %v", err)
	}
}

// The spawn walk must not disturb what already worked: a binder still binds
// (by name, since spawn bodies have no slots), and an exhaustive match runs.
func TestEnumSpawnBinderStillWorks(t *testing.T) {
	out, runErr, _ := runSpawn(t, `
use io;
def enum S { A { x as int }, B };
def s as S init S.A{ x: 42 };
spawn {
    match ($s) {
        when A(p) { io.printf("%d\n", $p.x); }
        when B { io.printf("-1\n"); }
    }
};
`)
	if runErr != nil {
		t.Fatalf("err: %v", runErr)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("got %q, want it to contain 42", out)
	}
}

// An ordinary value match inside a spawn keeps working - the enum walk only
// claims a match whose arms are all variant patterns.
func TestValueMatchInSpawnUnaffected(t *testing.T) {
	out, runErr, _ := runSpawn(t, `
use io;
def const TWO as int init 2;
spawn {
    def n as int init 2;
    match ($n) {
        when TWO { io.printf("two\n"); }
        when 3 { io.printf("three\n"); }
    }
};
`)
	if runErr != nil {
		t.Fatalf("err: %v", runErr)
	}
	if !strings.Contains(out, "two") {
		t.Errorf("got %q, want it to contain two", out)
	}
}
