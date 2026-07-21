// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package testinglib_test

import "testing"

// The assertion vocabulary is exercised through testing.run so the thrown
// Error{kind:"assertion"} is caught and classified exactly as at runtime.
// runProg lives in testinglib_test.go.

func TestAssertEqualPassAndFail(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func good() { testing.assertEqual(2, 2); }
		func bad()  { testing.assertEqual(2, 3); }
		def g as testing.Result init testing.run("good");
		def b as testing.Result init testing.run("bad");
		io.printf("%t %t [%s] %s", $g.passed, $b.passed, $b.errorKind, $b.errorMessage);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "true false [assertion] assertEqual: 2 != 3"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestAssertEqualDeepStructural(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func lists()  { testing.assertEqual([1, 2, 3], [1, 2, 3]); }
		func nested() { testing.assertNotEqual([1, 2], [1, 3]); }
		def a as testing.Result init testing.run("lists");
		def b as testing.Result init testing.run("nested");
		io.printf("%t %t", $a.passed, $b.passed);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "true true" {
		t.Errorf("deep equality: got %q", out)
	}
}

func TestAssertContainsDispatch(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func str() { testing.assertContains("hello world", "world"); }
		func lst() { testing.assertContains([10, 20, 30], 20); }
		func mp()  { testing.assertContains({"a": 1, "b": 2}, "b"); }
		func miss(){ testing.assertContains([1, 2], 9); }
		def a as testing.Result init testing.run("str");
		def b as testing.Result init testing.run("lst");
		def c as testing.Result init testing.run("mp");
		def d as testing.Result init testing.run("miss");
		io.printf("%t %t %t %t [%s]", $a.passed, $b.passed, $c.passed, $d.passed, $d.errorKind);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "true true true false [assertion]"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestAssertTrueFalse(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func ok()  { testing.assertTrue(1 < 2); testing.assertFalse(1 > 2); }
		func bad() { testing.assertTrue(1 > 2); }
		def a as testing.Result init testing.run("ok");
		def b as testing.Result init testing.run("bad");
		io.printf("%t %t", $a.passed, $b.passed);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "true false" {
		t.Errorf("got %q", out)
	}
}

func TestAssertThrows(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func boom() { throw Error{kind: "boom", message: "x", file: "", line: 0, col: 0}; }
		func quiet() { return; }
		func matches()   { testing.assertThrows("boom", "boom"); }
		func wrongKind() { testing.assertThrows("boom", "other"); }
		func noThrow()   { testing.assertThrows("quiet", "boom"); }
		def a as testing.Result init testing.run("matches");
		def b as testing.Result init testing.run("wrongKind");
		def c as testing.Result init testing.run("noThrow");
		io.printf("%t %t %t", $a.passed, $b.passed, $c.passed);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "true false false" {
		t.Errorf("got %q", out)
	}
}

func TestRunWithBindsArgs(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func check(a as int, want as int) { testing.assertEqual($a + 1, $want); }
		def ok  as testing.Result init testing.runWith("check", [4, 5]);
		def bad as testing.Result init testing.runWith("check", [4, 9]);
		io.printf("%t %t %s", $ok.passed, $bad.passed, $bad.errorMessage);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "true false assertEqual: 5 != 9"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestAssertionCarriesCallSitePosition(t *testing.T) {
	// The thrown Error should point at the assertion call, not the throw
	// keyword or the call boundary.
	out, err := runProg(t, `
		use io; use testing;
		func bad() {
			testing.assertEqual(1, 2);
		}
		def r as testing.Result init testing.run("bad");
		io.printf("line=%d col=%d", $r.line, $r.col);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// The assertEqual( call is on line 4 of the source (1-based from the
	// leading newline), indented with tabs.
	if out != "line=4 col=4" {
		t.Errorf("position wrong: got %q", out)
	}
}
