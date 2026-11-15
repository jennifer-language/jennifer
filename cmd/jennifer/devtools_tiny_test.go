// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tinyBinFromEnv resolves a built TinyGo binary named by an environment
// variable, skipping the test when it is unset (a plain `go test ./...` needs
// no TinyGo) and failing it when it names a file that is not there.
func tinyBinFromEnv(t *testing.T, env string) string {
	t.Helper()
	bin := os.Getenv(env)
	if bin == "" {
		t.Skipf("%s not set; build the binary and point it there to run this test", env)
	}
	bin, err := filepath.Abs(bin)
	if err != nil {
		t.Fatalf("resolve %s: %v", env, err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("%s=%s: %v", env, bin, err)
	}
	return bin
}

// writeTinyOverlay writes a module and its co-located test overlay into a
// fresh directory and returns the overlay's path. The overlay reads a private
// name, so it only passes when `test` splices it onto the module.
func writeTinyOverlay(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calc.j"), []byte(
		"export func add(a as int, b as int) { return $a + $b; }\nfunc secret() { return 42; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, "calc_test.j")
	if err := os.WriteFile(overlay, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return overlay
}

// TestTinyDevBinaryRunsOverlay checks jennifer-tiny-dev (the TinyGo build with
// the `tinygo_devtools` tag, named by JENNIFER_TINY_DEV_BIN) runs `test` on a
// module overlay under the constrained runtime: a passing overlay exits 0, a
// failing one exits non-zero, and a bundled module's own overlay passes.
func TestTinyDevBinaryRunsOverlay(t *testing.T) {
	bin := tinyBinFromEnv(t, "JENNIFER_TINY_DEV_BIN")

	pass := writeTinyOverlay(t, "use testing;\n"+
		"func testPublic() { testing.assertEqual(add(2, 3), 5); }\n"+
		"func testPrivate() { testing.assertEqual(secret(), 42); }\n")
	out, err := exec.Command(bin, "test", pass).CombinedOutput()
	if err != nil {
		t.Fatalf("jennifer-tiny-dev test on a passing overlay: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "2 passed") {
		t.Errorf("want the two overlay tests reported as passed, got:\n%s", out)
	}

	fail := writeTinyOverlay(t, "use testing;\n"+
		"func testWrong() { testing.assertEqual(add(2, 2), 5); }\n")
	out, err = exec.Command(bin, "test", fail).CombinedOutput()
	if err == nil {
		t.Fatalf("jennifer-tiny-dev test on a failing overlay exited 0:\n%s", out)
	}

	// --isolated needs os/exec, which TinyGo lacks: refused up front rather
	// than failing every test with an empty message.
	out, err = exec.Command(bin, "test", "--isolated", pass).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--isolated starts a process per test") {
		t.Errorf("jennifer-tiny-dev test --isolated: want the refusal, got err=%v:\n%s", err, out)
	}

	root, err := filepath.Abs(filepath.Join("..", "..", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "test", filepath.Join(root, "semver_test.j"))
	cmd.Env = append(os.Environ(), "JENNIFER_SYSMODDIR="+root)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jennifer-tiny-dev test modules/semver_test.j: %v\n%s", err, out)
	}
}

// TestTinyBinaryRefusesTest checks the plain jennifer-tiny (JENNIFER_TINY_BIN)
// still refuses `test` with its usual pointer at the default binary: the
// `tinygo_devtools` tag must not leak into the shipped build.
func TestTinyBinaryRefusesTest(t *testing.T) {
	bin := tinyBinFromEnv(t, "JENNIFER_TINY_BIN")
	overlay := writeTinyOverlay(t, "use testing;\nfunc testPublic() { testing.assertEqual(add(2, 3), 5); }\n")
	cmd := exec.Command(bin, "test", overlay)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("jennifer-tiny test exited 0; want the development-feature refusal:\n%s", out)
	}
	if code := cmd.ProcessState.ExitCode(); code != 2 {
		t.Errorf("jennifer-tiny test exit code = %d, want 2", code)
	}
	want := "jennifer test is a development feature, not available in the constrained (TinyGo) build; use the default `jennifer` binary"
	if !strings.Contains(string(out), want) {
		t.Errorf("jennifer-tiny test output = %q, want it to contain %q", out, want)
	}
}
