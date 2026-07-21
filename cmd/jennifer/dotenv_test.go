// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestDotenv drives the dotenv module's file + environment path: read a real
// .env off disk (parsing comments, export, quoting, inline comments), then load
// it and confirm the variables landed in the process environment via os.getEnv.
// A mismatch throws in the .j program and fails loadForTest.
func TestDotenv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := "# service config\n" +
		"DOTENVTESTPORT=8080\n" +
		"export DOTENVTESTNAME=\"ada\"\n" +
		"DOTENVTESTGREETING='hello world'\n" +
		"DOTENVTESTEMPTY=\n" +
		"DOTENVTESTNOTE=value # inline comment\n"
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dotenvMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "dotenv.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use os;
import %q as dotenv;
def m as map of string to string init dotenv.read(%q);
testing.assertEqual($m["DOTENVTESTPORT"], "8080");
testing.assertEqual($m["DOTENVTESTNAME"], "ada");
testing.assertEqual($m["DOTENVTESTGREETING"], "hello world");
testing.assertEqual($m["DOTENVTESTEMPTY"], "");
testing.assertEqual($m["DOTENVTESTNOTE"], "value");
def set as map of string to string init dotenv.load(%q);
testing.assertEqual(os.getEnv("DOTENVTESTPORT"), "8080");
testing.assertEqual(os.getEnv("DOTENVTESTNAME"), "ada");
testing.assertEqual(os.getEnv("DOTENVTESTGREETING"), "hello world");`, dotenvMod, envFile, envFile)
	progPath := filepath.Join(dir, "load.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("dotenv program failed with code %d", code)
	}
}
