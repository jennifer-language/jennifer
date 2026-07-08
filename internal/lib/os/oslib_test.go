// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package oslib

import (
	stdos "os"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

func TestGetEnvReadsSetVariable(t *testing.T) {
	const key = "JENNIFER_TEST_GETENV_SET"
	stdos.Setenv(key, "hello")
	defer stdos.Unsetenv(key)
	v, err := getEnvFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(key)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindString || v.Str != "hello" {
		t.Errorf("got (%s, %q)", v.Kind, v.Str)
	}
}

func TestGetEnvReturnsEmptyWhenUnset(t *testing.T) {
	const key = "JENNIFER_TEST_GETENV_UNSET"
	stdos.Unsetenv(key)
	v, err := getEnvFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(key)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Str != "" {
		t.Errorf("expected empty string, got %q", v.Str)
	}
}

func TestGetEnvRejectsNonString(t *testing.T) {
	_, err := getEnvFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)})
	if err == nil {
		t.Fatal("expected type error, got nil")
	}
}

func TestPlatformLineEndingLinuxToday(t *testing.T) {
	// Jennifer ships Linux-only today. When cross-platform support
	// lands the matching update will make this test branch.
	if got := platformLineEnding(); got != "\n" {
		t.Errorf("got %q, want \\n on linux", got)
	}
}

func TestHasFlagFindsExactMatch(t *testing.T) {
	prev := userArgs
	defer func() { userArgs = prev }()
	SetUserArgs([]string{"prog.j", "--verbose", "--port", "8080"})
	v, err := hasFlagFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("--verbose")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Kind != interpreter.KindBool || !v.Bool {
		t.Errorf("got %+v, want true", v)
	}
}

func TestHasFlagMissingReturnsFalse(t *testing.T) {
	prev := userArgs
	defer func() { userArgs = prev }()
	SetUserArgs([]string{"prog.j", "--verbose"})
	v, err := hasFlagFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("--quiet")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Bool {
		t.Errorf("expected false, got true")
	}
}

func TestHasFlagDoesNotMatchEquals(t *testing.T) {
	// `--port=8080` is NOT a match for `--port` - exact only.
	prev := userArgs
	defer func() { userArgs = prev }()
	SetUserArgs([]string{"prog.j", "--port=8080"})
	v, err := hasFlagFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("--port")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Bool {
		t.Errorf("--port=8080 unexpectedly satisfied --port")
	}
}

func TestFlagReturnsFollowingValue(t *testing.T) {
	prev := userArgs
	defer func() { userArgs = prev }()
	SetUserArgs([]string{"prog.j", "--port", "8080"})
	v, err := flagFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("--port")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Str != "8080" {
		t.Errorf("got %q, want %q", v.Str, "8080")
	}
}

func TestFlagMissingReturnsEmpty(t *testing.T) {
	prev := userArgs
	defer func() { userArgs = prev }()
	SetUserArgs([]string{"prog.j", "--verbose"})
	v, err := flagFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("--port")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Str != "" {
		t.Errorf("got %q, want empty", v.Str)
	}
}

func TestFlagAtEndReturnsEmpty(t *testing.T) {
	// `--port` is present but no value follows.
	prev := userArgs
	defer func() { userArgs = prev }()
	SetUserArgs([]string{"prog.j", "--port"})
	v, err := flagFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("--port")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if v.Str != "" {
		t.Errorf("got %q, want empty (no value follows)", v.Str)
	}
}

func TestIsTerminalRegularFileIsNotTerminal(t *testing.T) {
	f, err := stdos.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isCharDevice(f) {
		t.Error("a regular file reported as a terminal")
	}
}

func TestIsTerminalReturnsBoolForStandardStreams(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr", "stdin"} {
		v, err := isTerminalFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(stream)})
		if err != nil {
			t.Fatalf("%s: %v", stream, err)
		}
		if v.Kind != interpreter.KindBool {
			t.Errorf("%s: got %s, want bool", stream, v.Kind)
		}
	}
}

func TestIsTerminalRejectsBadArgs(t *testing.T) {
	cases := [][]interpreter.Value{
		{interpreter.StringVal("foo")}, // unknown stream
		{interpreter.Null()},           // wrong type
		{},                             // wrong arity
		{interpreter.StringVal("stdout"), interpreter.StringVal("extra")}, // wrong arity
	}
	for i, args := range cases {
		if _, err := isTerminalFn(interpreter.BuiltinCtx{}, args); err == nil {
			t.Errorf("case %d: expected an error", i)
		}
	}
}
