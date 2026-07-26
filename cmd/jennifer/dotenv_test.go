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

// TestDotenvCascade drives the layered loaders: cascade order (later file wins),
// cross-file ${VAR} interpolation, multi-line double-quoted values from a real
// file, real-OS-env-wins in resolve / loadCascade, profile-traversal rejection,
// and autoload picking the profile from JENNIFER_ENV.
func TestDotenvCascade(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Base: some keys, a shared key a real env var will win over, a multi-line
	// value, and a key the profile layer overrides.
	write(".env", "DOTCA=base\nDOTCC=base\nDOTCSHARED=filevalue\nDOTCML=\"one\ntwo\"\n")
	write(".env.local", "DOTCB=local\n")
	// Profile layer overrides DOTCA and interpolates against it (already merged).
	write(".env.production", "DOTCA=prod\nDOTCURL=http://${DOTCA}/api\n")

	// A real environment variable that must win over the file's DOTCSHARED, plus
	// the profile selector autoload reads.
	t.Setenv("DOTCSHARED", "realsecret")
	t.Setenv("JENNIFER_ENV", "production")

	dotenvMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "dotenv.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use os;
import %q as dotenv;

def dir as string init %q;

# readCascade: pure file merge, later layer wins, cross-file interpolation,
# multi-line - and it does NOT consult the OS env (DOTCSHARED stays the file value).
def rc as map of string to string init dotenv.readCascade($dir, "production");
testing.assertEqual($rc["DOTCA"], "prod");             # .env.production overrides .env
testing.assertEqual($rc["DOTCB"], "local");            # from .env.local
testing.assertEqual($rc["DOTCC"], "base");             # only in .env
testing.assertEqual($rc["DOTCURL"], "http://prod/api");# ${DOTCA} interpolated
testing.assertEqual($rc["DOTCML"], "one\ntwo");        # multi-line double-quoted
testing.assertEqual($rc["DOTCSHARED"], "filevalue");   # readCascade ignores the env

# resolve: real OS env wins over the file value; a key absent from the env keeps
# its file value.
def rv as map of string to string init dotenv.resolve($dir, "production");
testing.assertEqual($rv["DOTCSHARED"], "realsecret");  # real env wins
testing.assertEqual($rv["DOTCA"], "prod");             # not in env -> file value

# loadCascade: sets only keys not already in the real env; never clobbers one
# that is. Returns the file map.
def lc as map of string to string init dotenv.loadCascade($dir, "production");
testing.assertEqual($lc["DOTCA"], "prod");
testing.assertEqual(os.getEnv("DOTCA"), "prod");         # was unset -> now set
testing.assertEqual(os.getEnv("DOTCSHARED"), "realsecret"); # real env NOT overridden

# Profile traversal is rejected up front.
def rejected as bool init false;
try { dotenv.readCascade($dir, "../../etc"); } catch (e) { $rejected = true; testing.assertEqual($e.kind, "dotenv"); }
testing.assertTrue($rejected);

# autoload reads JENNIFER_ENV (=production here) for the profile.
def al as map of string to string init dotenv.autoload($dir);
testing.assertEqual($al["DOTCA"], "prod");
testing.assertEqual($al["DOTCURL"], "http://prod/api");
`, dotenvMod, dir)
	progPath := filepath.Join(dir, "cascade.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("dotenv cascade program failed with code %d", code)
	}
}
