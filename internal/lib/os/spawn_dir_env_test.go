// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	stdos "os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.57: os.SpawnOptions dir, env and cleanEnv.

// envMap builds a map of string to string from key, value pairs.
func envMap(kv ...string) interpreter.Value {
	var es []interpreter.MapEntry
	for i := 0; i+1 < len(kv); i += 2 {
		es = append(es, interpreter.MapEntry{Key: interpreter.StringVal(kv[i]), Value: interpreter.StringVal(kv[i+1])})
	}
	return interpreter.GenericMapVal(es)
}

// spawnOptsWith builds an os.SpawnOptions holding only the given fields, as a
// literal that leaves the others out does.
func spawnOptsWith(fields ...interpreter.StructField) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "SpawnOptions", fields)
}

// spawnWithOut runs argv with opts and returns its captured stdout.
func spawnWithOut(t *testing.T, opts interpreter.Value, argv ...string) string {
	t.Helper()
	ctx := interpreter.BuiltinCtx{}
	p, err := spawnWithFn(ctx, []interpreter.Value{stringList(argv...), opts})
	if err != nil {
		t.Fatal(err)
	}
	r, err := waitFn(ctx, []interpreter.Value{p})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := resultStdout(r)
	return out
}

func TestSpawnWithDir(t *testing.T) {
	skipIfNotLinux(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out := spawnWithOut(t, spawnOptsWith(interpreter.StructField{Name: "dir", Value: interpreter.StringVal(dir)}), "pwd"); out != dir+"\n" {
		t.Errorf("pwd = %q, want %q", out, dir+"\n")
	}
	wd, _ := stdos.Getwd()
	if out := spawnWithOut(t, spawnOptsWith(interpreter.StructField{Name: "dir", Value: interpreter.StringVal("")}), "pwd"); out != wd+"\n" {
		t.Errorf("dir \"\": pwd = %q, want %q", out, wd+"\n")
	}
	file := filepath.Join(dir, "f")
	if err := stdos.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "out.log")
	for _, c := range []struct{ dir, want string }{
		{filepath.Join(dir, "missing"), `os.spawnWith: dir "` + filepath.Join(dir, "missing") + `" does not exist`},
		{file, `os.spawnWith: dir "` + file + `" is not a directory`},
	} {
		_, err := spawnWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("true"), spawnOptsWith(
			interpreter.StructField{Name: "stdout", Value: interpreter.StringVal(log)},
			interpreter.StructField{Name: "dir", Value: interpreter.StringVal(c.dir)},
		)})
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("dir %q: err %v, want prefix %q", c.dir, err, c.want)
		}
		if _, serr := stdos.Stat(log); serr == nil {
			t.Fatalf("dir %q: the stdout file was created despite the error", c.dir)
		}
	}
}

func TestSpawnWithEnv(t *testing.T) {
	skipIfNotLinux(t)
	t.Setenv("JTEST_PARENT", "parent")
	t.Setenv("JTEST_KEEP", "kept")
	script := `printf '%s|%s|%s' "$JTEST_PARENT" "$JTEST_KEEP" "$JTEST_NEW"`
	// Inherited, one overridden, one added.
	out := spawnWithOut(t, spawnOptsWith(interpreter.StructField{Name: "env", Value: envMap("JTEST_PARENT", "child", "JTEST_NEW", "new")}), "sh", "-c", script)
	if out != "child|kept|new" {
		t.Errorf("env on top: %q", out)
	}
	// The zero value (an empty map) inherits everything unchanged.
	out = spawnWithOut(t, spawnOptsWith(interpreter.StructField{Name: "env", Value: envMap()}), "sh", "-c", script)
	if out != "parent|kept|" {
		t.Errorf("empty env: %q", out)
	}
	// cleanEnv: only env reaches the child.
	clean := interpreter.StructField{Name: "cleanEnv", Value: interpreter.BoolVal(true)}
	out = spawnWithOut(t, spawnOptsWith(clean, interpreter.StructField{Name: "env", Value: envMap("A", "1")}), "env")
	if out != "A=1\n" {
		t.Errorf("cleanEnv + env: %q", out)
	}
	if out = spawnWithOut(t, spawnOptsWith(clean), "env"); out != "" {
		t.Errorf("cleanEnv alone: %q", out)
	}
}

func TestSpawnWithEnvRefusesBadNames(t *testing.T) {
	skipIfNotLinux(t)
	for _, c := range []struct {
		env  interpreter.Value
		want string
	}{
		{envMap("", "x"), "os.spawnWith: os.SpawnOptions.env: empty variable name"},
		{envMap("A=B", "x"), `os.spawnWith: os.SpawnOptions.env: variable name "A=B" contains '=' or NUL`},
		{envMap("A\x00B", "x"), `os.spawnWith: os.SpawnOptions.env: variable name "A\x00B" contains '=' or NUL`},
		{envMap("A", "x\x00y"), `os.spawnWith: os.SpawnOptions.env: the value of "A" contains NUL`},
	} {
		_, err := spawnWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("true"), spawnOptsWith(interpreter.StructField{Name: "env", Value: c.env})})
		if err == nil || err.Error() != c.want {
			t.Errorf("err %v, want %q", err, c.want)
		}
	}
}
