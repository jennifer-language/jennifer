// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import "testing"

// TestParseRunArgsEnv covers the --env run-profile flag: both spellings, its
// placement among the other interpreter flags, and that a token after the file
// is a program arg, never a flag.
func TestParseRunArgsEnv(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantFile string
		wantEnv  string
		wantUser []string
	}{
		{"equals form", []string{"--env=prod", "app.j"}, "app.j", "prod", []string{}},
		{"space form", []string{"--env", "staging", "app.j"}, "app.j", "staging", []string{}},
		{"no env flag", []string{"app.j"}, "app.j", "", []string{}},
		{"env among other flags", []string{"-I", "lib", "--env=dev", "app.j"}, "app.j", "dev", []string{}},
		{"flag after file is a user arg", []string{"app.j", "--env=nope"}, "app.j", "", []string{"--env=nope"}},
		{"user args preserved after file", []string{"--env=prod", "app.j", "a", "b"}, "app.j", "prod", []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file, _, _, env, _, userArgs, err := parseRunArgs(c.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if file != c.wantFile {
				t.Errorf("file = %q, want %q", file, c.wantFile)
			}
			if env != c.wantEnv {
				t.Errorf("env = %q, want %q", env, c.wantEnv)
			}
			if len(userArgs) != len(c.wantUser) {
				t.Fatalf("userArgs = %v, want %v", userArgs, c.wantUser)
			}
			for i := range c.wantUser {
				if userArgs[i] != c.wantUser[i] {
					t.Errorf("userArgs[%d] = %q, want %q", i, userArgs[i], c.wantUser[i])
				}
			}
		})
	}
}

// TestParseRunArgsEnvMissingValue: `--env` with no following token errors.
func TestParseRunArgsEnvMissingValue(t *testing.T) {
	if _, _, _, _, _, _, err := parseRunArgs([]string{"--env"}); err == nil {
		t.Fatal("expected an error for a bare --env with no profile")
	}
}

// TestValidRunProfile mirrors dotenv's validProfile (`^[A-Za-z0-9_-]{1,64}$`):
// the flag and the module must agree on what a profile may be spelled, and the
// strict shape blocks a `.env.<profile>` path traversal.
func TestValidRunProfile(t *testing.T) {
	valid := []string{"prod", "dev", "staging", "test-1", "a_b", "A1", "x", "e2e-smoke"}
	for _, s := range valid {
		if !validRunProfile(s) {
			t.Errorf("validRunProfile(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"",          // empty
		"../evil",   // path traversal
		"a/b",       // slash
		"a.b",       // dot (would splice a two-level .env.a.b)
		"has space", // space
		"tab\ttab",  // control
		"quote'd",   // quote
		"prod;rm",   // shell metachar
	}
	for _, s := range invalid {
		if validRunProfile(s) {
			t.Errorf("validRunProfile(%q) = true, want false", s)
		}
	}
	// exactly 64 chars is allowed; 65 is not.
	s64 := ""
	for i := 0; i < 64; i++ {
		s64 += "a"
	}
	if !validRunProfile(s64) {
		t.Error("a 64-char profile should be valid")
	}
	if validRunProfile(s64 + "a") {
		t.Error("a 65-char profile should be rejected")
	}
}
