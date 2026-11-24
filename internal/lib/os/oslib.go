// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package oslib implements Jennifer's `os` library: host-environment
// glue. Holds constants describing the host (platform, architecture,
// line ending, argv) and a handful of functions for environment-variable
// lookup and mutation and command-line flag inspection.
//
// External programs (`os.run` / `runBytes`, `os.spawn` / `wait` /
// `waitBytes` / `poll` / `kill` / `release`) live in exec.go; signal polling in signal_unix.go.
//
// Everything here is namespaced - the user writes `os.PLATFORM`,
// `os.getEnv("HOME")`, `os.hasFlag("--verbose")`, etc. None of these
// names are reachable bare.
//
// The Go package is named oslib to avoid colliding with Go's standard
// `os` package, which this implementation depends on.
package oslib

import (
	"fmt"
	stdos "os"
	"runtime"
	"strings"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these names,
// and doubles as the namespace prefix.
const LibraryName = "os"

// dataKind is the kind of outside text that is not valid UTF-8: a child's
// captured output (M29.21), an environment variable, an argument (M29.50).
const dataKind = "os"

// userArgs holds the command-line arguments the user program sees. By
// convention (matches Python sys.argv, Go os.Args), index 0 is the
// script path and the rest are user-supplied arguments. The CLI sets
// this before calling Install; if it's nil at Install time we fall
// back to the interpreter's own argv so library tests work without
// any setup.
var userArgs []string

// SetUserArgs lets the CLI hand the user program's argv to the
// library before Install registers the `os.ARGS` constant. Subsequent
// calls overwrite the previous value (the CLI calls this once per
// run; tests that need a clean slate can re-set it).
func SetUserArgs(args []string) {
	userArgs = append([]string(nil), args...)
}

// Install registers the `os` library. Constants describe immutable
// per-run host facts; functions cover the operations that genuinely
// take arguments.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "getEnv", getEnvFn)
	in.RegisterNamespaced(LibraryName, "getEnvBytes", getEnvBytesFn)
	in.RegisterNamespaced(LibraryName, "argsBytes", argsBytesFn)
	in.RegisterNamespaced(LibraryName, "setEnv", setEnvFn)
	in.RegisterNamespaced(LibraryName, "unsetEnv", unsetEnvFn)
	in.RegisterNamespaced(LibraryName, "hasEnv", hasEnvFn)
	in.RegisterNamespaced(LibraryName, "hasFlag", hasFlagFn)
	in.RegisterNamespaced(LibraryName, "flag", flagFn)
	in.RegisterNamespaced(LibraryName, "run", runFn)
	in.RegisterNamespaced(LibraryName, "runBytes", runBytesFn)
	in.RegisterNamespaced(LibraryName, "spawn", spawnFn)
	in.RegisterNamespaced(LibraryName, "spawnWith", spawnWithFn)
	in.RegisterNamespaced(LibraryName, "wait", waitFn)
	in.RegisterNamespaced(LibraryName, "waitBytes", waitBytesFn)
	in.RegisterNamespaced(LibraryName, "waitTimeout", waitTimeoutFn)
	in.RegisterNamespaced(LibraryName, "waitBytesTimeout", waitBytesTimeoutFn)
	in.RegisterNamespaced(LibraryName, "poll", pollFn)
	in.RegisterNamespaced(LibraryName, "kill", killFn)
	in.RegisterNamespaced(LibraryName, "release", releaseFn)
	in.RegisterNamespaced(LibraryName, "isTerminal", isTerminalFn)
	in.RegisterNamespaced(LibraryName, "cwd", cwdFn)
	in.RegisterNamespaced(LibraryName, "homeDir", homeDirFn)
	in.RegisterNamespaced(LibraryName, "tempDir", tempDirFn)
	in.RegisterNamespaced(LibraryName, "catchSignal", catchSignalFn)
	in.RegisterNamespaced(LibraryName, "gotSignal", gotSignalFn)

	in.RegisterNamespacedConst(LibraryName, "PLATFORM", interpreter.StringVal(runtime.GOOS))
	in.RegisterNamespacedConst(LibraryName, "ARCH", interpreter.StringVal(runtime.GOARCH))
	in.RegisterNamespacedConst(LibraryName, "NCPU", interpreter.IntVal(int64(runtime.NumCPU())))
	in.RegisterNamespacedConst(LibraryName, "EOL", interpreter.StringVal(platformLineEnding()))
	in.RegisterNamespacedConst(LibraryName, "DIRSEP", interpreter.StringVal(string(stdos.PathSeparator)))
	in.RegisterNamespacedConst(LibraryName, "PATHSEP", interpreter.StringVal(string(stdos.PathListSeparator)))
	if args, err := argsConstant(); err != nil {
		in.RegisterNamespacedConstError(LibraryName, "ARGS", err)
	} else {
		in.RegisterNamespacedConst(LibraryName, "ARGS", args)
	}

	// External-program execution result/handle types.
	str := parser.PrimitiveType(parser.TypeString)
	in.RegisterNamespacedStruct(LibraryName, "Result", []parser.StructField{
		{Name: "exitCode", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "stdout", Type: str},
		{Name: "stderr", Type: str},
	})
	byt := parser.PrimitiveType(parser.TypeBytes)
	in.RegisterNamespacedStruct(LibraryName, "BytesResult", []parser.StructField{
		{Name: "exitCode", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "stdout", Type: byt},
		{Name: "stderr", Type: byt},
		{Name: "truncated", Type: parser.PrimitiveType(parser.TypeBool)},
	})
	// Options of spawnWith: where each child stream goes. "" (the zero
	// value) or "capture" captures it for os.wait; "inherit", "discard", or
	// a file path containing a separator.
	in.RegisterNamespacedStruct(LibraryName, "SpawnOptions", []parser.StructField{
		{Name: "stdout", Type: str},
		{Name: "stderr", Type: str},
	})
	in.RegisterNamespacedStruct(LibraryName, "Process", []parser.StructField{
		{Name: "handle", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "pid", Type: parser.PrimitiveType(parser.TypeInt)},
	})
}

// argsConstant materialises a Jennifer `list of string` for `os.ARGS`
// from argvSource. An argument that is not valid UTF-8 makes it an error
// (kind "os", naming the argument index and byte offset), which every read of
// `os.ARGS` throws; `os.argsBytes()` returns the raw arguments.
func argsConstant() (interpreter.Value, error) {
	src := argvSource()
	data := make([]interpreter.Value, len(src))
	for i, a := range src {
		if off := limits.InvalidUTF8At(a); off >= 0 {
			return interpreter.Null(), dataerr.New(dataKind, "os.ARGS: argument %d: invalid UTF-8 at byte %d; use os.argsBytes()", i, off)
		}
		data[i] = interpreter.StringVal(a)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), data), nil
}

// argsBytesFn implements `os.argsBytes() -> list of bytes`: the arguments of
// os.ARGS as raw bytes, for a program that must take any argument.
func argsBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("os.argsBytes expects 0 arguments, got %d", len(args))
	}
	src := argvSource()
	data := make([]interpreter.Value, len(src))
	for i, a := range src {
		data[i] = interpreter.BytesVal([]byte(a))
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeBytes), data), nil
}

// getEnvFn reads an environment variable. Unset variables produce the
// empty string (no error) so callers can use `==`/`!=` against `""` to
// branch. A value that is not valid UTF-8 throws kind "os" naming the
// variable and the byte offset; os.getEnvBytes returns it raw.
func getEnvFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.getEnv expects 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.getEnv: variable name must be string, got %s", args[0].Kind)
	}
	v := stdos.Getenv(args[0].Str)
	if off := limits.InvalidUTF8At(v); off >= 0 {
		return interpreter.Null(), dataerr.New(dataKind, "os.getEnv: %s: invalid UTF-8 at byte %d; use os.getEnvBytes", args[0].Str, off)
	}
	return interpreter.StringVal(v), nil
}

// getEnvBytesFn implements `os.getEnvBytes(name) -> bytes`: the variable's
// raw value, empty when it is unset (like os.getEnv; os.hasEnv tells the two
// apart).
func getEnvBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.getEnvBytes expects 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.getEnvBytes: variable name must be string, got %s", args[0].Kind)
	}
	return interpreter.BytesVal([]byte(stdos.Getenv(args[0].Str))), nil
}

// setEnvFn sets an environment variable for the current process (and any
// child process it later spawns via `os.run` / `os.spawn`). Mirrors Go's
// `os.Setenv`; an invalid name (empty, or containing '=' or a NUL byte) is a
// positioned error. Returns null.
func setEnvFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("os.setEnv expects 2 arguments (name, value), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.setEnv: variable name must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.setEnv: value must be string, got %s", args[1].Kind)
	}
	if err := stdos.Setenv(args[0].Str, args[1].Str); err != nil {
		return interpreter.Null(), fmt.Errorf("os.setEnv: %v", err)
	}
	return interpreter.Null(), nil
}

// envName checks the name argument of an env builtin: a string that is a legal
// variable name (non-empty, no '=' or NUL byte), the rule os.setEnv enforces.
func envName(fn string, v interpreter.Value) (string, error) {
	if v.Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: variable name must be string, got %s", fn, v.Kind)
	}
	if v.Str == "" || strings.ContainsAny(v.Str, "=\x00") {
		return "", fmt.Errorf("%s: invalid variable name %q", fn, v.Str)
	}
	return v.Str, nil
}

// unsetEnvFn removes an environment variable from the current process (and so
// from any child it later spawns), unlike `os.setEnv(name, "")`, which leaves
// the variable set to the empty string. Removing a variable that is not set is
// a no-op. Returns null.
func unsetEnvFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.unsetEnv expects 1 argument (name), got %d", len(args))
	}
	name, err := envName("os.unsetEnv", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := stdos.Unsetenv(name); err != nil {
		return interpreter.Null(), fmt.Errorf("os.unsetEnv: %v", err)
	}
	return interpreter.Null(), nil
}

// hasEnvFn reports whether an environment variable is set, even to the empty
// string - the distinction os.getEnv (which returns "" for both) cannot make,
// so a program can save a variable's state and restore it exactly.
func hasEnvFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.hasEnv expects 1 argument (name), got %d", len(args))
	}
	name, err := envName("os.hasEnv", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	_, ok := stdos.LookupEnv(name)
	return interpreter.BoolVal(ok), nil
}

// hasFlagFn (deprecated, M29.48: the args module) returns true if `name` appears anywhere in `os.ARGS` as an
// exact-match element. Useful for the "did the user pass `--verbose`?"
// pattern. Exact-match only: `--port=8080` does NOT satisfy
// `os.hasFlag("--port")` - that form is deliberately not supported
// here because real flag parsing belongs to a future `cli` library.
func hasFlagFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.hasFlag expects 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.hasFlag: flag name must be string, got %s", args[0].Kind)
	}
	name := args[0].Str
	for _, a := range argvSource() {
		if a == name {
			return interpreter.BoolVal(true), nil
		}
	}
	return interpreter.BoolVal(false), nil
}

// isTerminalFn implements `os.isTerminal(stream) -> bool`: is the named
// standard stream ("stdout" / "stderr" / "stdin") an interactive terminal? The
// default binary answers the real question via golang.org/x/term (already a
// dependency of the term library), so a /dev/null or /dev/zero redirect reads
// false - what an "am I interactive?" guard needs, not just a color gate.
// jennifer-tiny has no x/term and falls back to the character-device mode bit
// (right for a pty, wrong only for a non-terminal character device). Either way
// a stream that can't be introspected conservatively reports false, since the
// point of the check is to suppress a prompt or terminal escapes on anything
// that isn't an interactive terminal.
func isTerminalFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.isTerminal expects 1 argument (stream), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.isTerminal: stream must be string, got %s", args[0].Kind)
	}
	var f *stdos.File
	switch args[0].Str {
	case "stdout":
		f = stdos.Stdout
	case "stderr":
		f = stdos.Stderr
	case "stdin":
		f = stdos.Stdin
	default:
		return interpreter.Null(), fmt.Errorf("os.isTerminal: unknown stream %q; known: \"stdout\", \"stderr\", \"stdin\"", args[0].Str)
	}
	return interpreter.BoolVal(isTerminalFile(f)), nil
}

// cwdFn implements os.cwd() -> string: the process's current working
// directory, as an absolute path. Errors only if the directory can't be
// determined (e.g. it was removed out from under the process).
func cwdFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("os.cwd expects 0 arguments, got %d", len(args))
	}
	dir, err := stdos.Getwd()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("os.cwd: %v", err)
	}
	return interpreter.StringVal(dir), nil
}

// homeDirFn implements os.homeDir() -> string: the current user's home
// directory (`$HOME` on Unix, `%USERPROFILE%` on Windows). Errors if it
// can't be resolved (the relevant variable is unset).
func homeDirFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("os.homeDir expects 0 arguments, got %d", len(args))
	}
	dir, err := stdos.UserHomeDir()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("os.homeDir: %v", err)
	}
	return interpreter.StringVal(dir), nil
}

// tempDirFn implements os.tempDir() -> string: the directory for temporary
// files (`$TMPDIR` or `/tmp` on Unix, the `%TMP%` / `%TEMP%` location on
// Windows). Never errors - the host returns a platform default when the
// environment variables are unset. The directory is not created or
// checked for existence.
func tempDirFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("os.tempDir expects 0 arguments, got %d", len(args))
	}
	return interpreter.StringVal(stdos.TempDir()), nil
}

// flagFn (deprecated, M29.48: the args module) returns the argument that immediately follows `name` in
// `os.ARGS`, or `""` if `name` is absent or appears only at the end of
// the argv. Like `os.getEnv`, missing values are an empty string
// rather than an error so callers can compare against `""`. The
// `--foo=bar` syntax is not parsed here; callers who need it write
// `os.flag("--foo=bar")` (string-search the whole element) or reach
// for a future `cli` library.
func flagFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.flag expects 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.flag: flag name must be string, got %s", args[0].Kind)
	}
	name := args[0].Str
	argv := argvSource()
	for i, a := range argv {
		if a == name && i+1 < len(argv) {
			if off := limits.InvalidUTF8At(argv[i+1]); off >= 0 {
				return interpreter.Null(), dataerr.New(dataKind, "os.flag: argument %d: invalid UTF-8 at byte %d", i+1, off)
			}
			return interpreter.StringVal(argv[i+1]), nil
		}
	}
	return interpreter.StringVal(""), nil
}

// argvSource returns the slice both hasFlag and flag should scan: the
// user-args set by SetUserArgs when present, the interpreter binary's
// own argv otherwise.
func argvSource() []string {
	if userArgs != nil {
		return userArgs
	}
	return stdos.Args
}

// platformLineEnding returns the conventional line terminator for the
// host. Today Jennifer is Linux-only so this always resolves to "\n";
// the switch is in place so Windows support (when cross-compile lands)
// just changes one return value.
func platformLineEnding() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}
