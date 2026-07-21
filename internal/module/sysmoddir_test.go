// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSysmoddirPrecedence(t *testing.T) {
	env := func(k string) string {
		if k == SysmoddirEnv {
			return "/from/env"
		}
		return ""
	}
	noEnv := func(string) string { return "" }

	// CLI wins over env and compile.
	if got := ResolveSysmoddir("/from/cli", env); got.Dir != "/from/cli" || got.Source != SourceCLI {
		t.Errorf("cli precedence: got %+v", got)
	}
	// Env wins over compile when no CLI flag.
	if got := ResolveSysmoddir("", env); got.Dir != "/from/env" || got.Source != SourceEnv {
		t.Errorf("env precedence: got %+v", got)
	}
	// Compile default when neither is set.
	if got := ResolveSysmoddir("", noEnv); got.Source != SourceCompile || got.Dir != compileDefaultSysmoddir {
		t.Errorf("compile default: got %+v", got)
	}
}

func TestValidateSysmoddir(t *testing.T) {
	realDir := t.TempDir()
	realFile := filepath.Join(realDir, "f")
	if err := os.WriteFile(realFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// CLI / env sources must exist and be directories.
	if err := (Sysmoddir{Dir: realDir, Source: SourceCLI}).Validate(os.Stat); err != nil {
		t.Errorf("existing dir should validate: %v", err)
	}
	if err := (Sysmoddir{Dir: "/no/such/dir", Source: SourceEnv}).Validate(os.Stat); err == nil {
		t.Error("missing env dir should fail validation")
	}
	if err := (Sysmoddir{Dir: realFile, Source: SourceCLI}).Validate(os.Stat); err == nil {
		t.Error("a file (not dir) should fail validation")
	}
	// The compile default is best-effort: never validated, even if absent.
	if err := (Sysmoddir{Dir: "/no/such/dir", Source: SourceCompile}).Validate(os.Stat); err != nil {
		t.Errorf("compile default should not be validated: %v", err)
	}
}
