// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A file with no test methods (or a --filter that matches none) fails the run
// with the runner-error code, so a CI or publish gate that only checks the exit
// status cannot pass an empty overlay. --allow-empty accepts it.
func TestTestEmptySuiteExitCode(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "m_test.j")
	if err := os.WriteFile(empty, []byte("use testing;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := filepath.Join(dir, "s_test.j")
	if err := os.WriteFile(suite, []byte("use testing;\nfunc testOne() { testing.assertTrue(true); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no test methods", []string{empty}, testExitFailure},
		{"no test methods, --allow-empty", []string{"--allow-empty", empty}, testExitPass},
		{"filter matches nothing", []string{"--filter=nomatch", suite}, testExitFailure},
		{"filter matches nothing, --allow-empty", []string{"--allow-empty", "--filter=nomatch", suite}, testExitPass},
		{"a real suite still passes", []string{suite}, testExitPass},
	}
	for _, c := range cases {
		var code int
		captureStdout(t, func() { code = runTest(c.args) })
		if code != c.want {
			t.Errorf("%s: exit %d, want %d", c.name, code, c.want)
		}
	}
}
