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

// TestSpawnModeTypoErrors: a stream value that is neither a keyword nor a path
// containing a separator is an error and creates no file.
func TestSpawnModeTypoErrors(t *testing.T) {
	skipIfNotLinux(t)
	dir := t.TempDir()
	wd, _ := stdos.Getwd()
	if err := stdos.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer stdos.Chdir(wd)
	_, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("true"), optsMap("stdout", "inhert")})
	if err == nil || !strings.HasPrefix(err.Error(), `os.spawn: unknown stdout mode "inhert"`) {
		t.Fatalf("err %v", err)
	}
	if _, serr := stdos.Stat(filepath.Join(dir, "inhert")); serr == nil {
		t.Fatal("the typo created a file")
	}
	// A bad stderr mode is reported before stdout's file is created.
	_, err = spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("true"), optsMap("stdout", "./out.log", "stderr", "discrad")})
	if err == nil || !strings.HasPrefix(err.Error(), `os.spawn: unknown stderr mode "discrad"`) {
		t.Fatalf("err %v", err)
	}
	if _, serr := stdos.Stat(filepath.Join(dir, "out.log")); serr == nil {
		t.Fatal("stdout file created despite the stderr error")
	}
	// "./name" is the explicit form for a file in the working directory.
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("echo", "hi"), optsMap("stdout", "./capture")})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p})
	if body, rerr := stdos.ReadFile(filepath.Join(dir, "capture")); rerr != nil || string(body) != "hi\n" {
		t.Fatalf("./capture: %q, %v", body, rerr)
	}
}

// TestSpawnUnknownOption: unknown keys and non-string values are errors, not
// silently ignored (which fell back to capture).
func TestSpawnUnknownOption(t *testing.T) {
	skipIfNotLinux(t)
	bad := []struct {
		opts interpreter.Value
		want string
	}{
		{optsMap("stdOut", "discard"), `os.spawn: unknown option "stdOut"; known: stdout, stderr`},
		{optsMap("stdin", "discard"), `os.spawn: unknown option "stdin"`},
		{interpreter.Value{Kind: interpreter.KindMap, Map: []interpreter.MapEntry{
			{Key: interpreter.StringVal("stdout"), Value: interpreter.IntVal(1)},
		}}, `os.spawn: option "stdout" must be a string, got int`},
	}
	for _, c := range bad {
		_, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{stringList("true"), c.opts})
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("err %v, want prefix %q", err, c.want)
		}
	}
}

// TestSpawnSameFileForBothStreams: stdout and stderr to one file (even spelled
// differently) share a descriptor, so neither stream overwrites the other.
func TestSpawnSameFileForBothStreams(t *testing.T) {
	skipIfNotLinux(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "x.log")
	p, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		stringList("sh", "-c", "echo OUT-LINE-ONE; echo ERR >&2; echo OUT-LINE-TWO"),
		optsMap("stdout", log, "stderr", dir+"/./x.log"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := waitFn(interpreter.BuiltinCtx{}, []interpreter.Value{p}); err != nil {
		t.Fatal(err)
	}
	body, err := stdos.ReadFile(log)
	if err != nil || string(body) != "OUT-LINE-ONE\nERR\nOUT-LINE-TWO\n" {
		t.Fatalf("log = %q, %v", body, err)
	}
}
