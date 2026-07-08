// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package oslib implements Jennifer's `os` library: host-environment
// glue. Holds constants describing the host (platform, architecture,
// line ending, argv) and a handful of functions for environment-variable
// lookup and command-line flag inspection.
//
// External-program execution (`os.run`, `os.spawn`, `os.wait`, etc.)
// is deferred to a later sub-milestone since it needs a
// library-provided struct mechanism the language doesn't yet have.
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

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these names,
// and doubles as the namespace prefix.
const LibraryName = "os"

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
	in.RegisterNamespaced(LibraryName, "hasFlag", hasFlagFn)
	in.RegisterNamespaced(LibraryName, "flag", flagFn)
	in.RegisterNamespaced(LibraryName, "run", runFn)
	in.RegisterNamespaced(LibraryName, "spawn", spawnFn)
	in.RegisterNamespaced(LibraryName, "wait", waitFn)
	in.RegisterNamespaced(LibraryName, "poll", pollFn)
	in.RegisterNamespaced(LibraryName, "kill", killFn)
	in.RegisterNamespaced(LibraryName, "isTerminal", isTerminalFn)

	in.RegisterNamespacedConst(LibraryName, "PLATFORM", interpreter.StringVal(runtime.GOOS))
	in.RegisterNamespacedConst(LibraryName, "ARCH", interpreter.StringVal(runtime.GOARCH))
	in.RegisterNamespacedConst(LibraryName, "EOL", interpreter.StringVal(platformLineEnding()))
	in.RegisterNamespacedConst(LibraryName, "DIRSEP", interpreter.StringVal(string(stdos.PathSeparator)))
	in.RegisterNamespacedConst(LibraryName, "PATHSEP", interpreter.StringVal(string(stdos.PathListSeparator)))
	in.RegisterNamespacedConst(LibraryName, "ARGS", argsConstant())

	// External-program execution result/handle types.
	str := parser.PrimitiveType(parser.TypeString)
	in.RegisterNamespacedStruct(LibraryName, "Result", []parser.StructField{
		{Name: "exitCode", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "stdout", Type: str},
		{Name: "stderr", Type: str},
	})
	in.RegisterNamespacedStruct(LibraryName, "Process", []parser.StructField{
		{Name: "pid", Type: parser.PrimitiveType(parser.TypeInt)},
	})
}

// argsConstant materialises a Jennifer `list of string` for `os.ARGS`.
// Uses the user-args slice set by SetUserArgs when available; falls
// back to the interpreter binary's own argv when nothing was set (so
// library tests work without the CLI's hand-off).
func argsConstant() interpreter.Value {
	src := userArgs
	if src == nil {
		src = stdos.Args
	}
	data := make([]interpreter.Value, len(src))
	for i, a := range src {
		data[i] = interpreter.StringVal(a)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), data)
}

// getEnvFn reads an environment variable. Unset variables produce the
// empty string (no error) so callers can use `==`/`!=` against `""` to
// branch. Mirrors Go's `os.Getenv`.
func getEnvFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("os.getEnv expects 1 argument, got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("os.getEnv: variable name must be string, got %s", args[0].Kind)
	}
	return interpreter.StringVal(stdos.Getenv(args[0].Str)), nil
}

// hasFlagFn returns true if `name` appears anywhere in `os.ARGS` as an
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
// standard stream ("stdout" / "stderr" / "stdin") an interactive terminal?
// Detected via the character-device mode bit (pure stdlib - no x/term
// dependency, which stays CLI-scoped - and TinyGo-clean). A stream that can't
// be stat'd (closed, or a runtime without terminal introspection like
// jennifer-tiny) conservatively reports false, since the point of the check is
// to suppress terminal escapes on anything that isn't an interactive terminal.
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
	return interpreter.BoolVal(isCharDevice(f)), nil
}

// isCharDevice reports whether f is a character device - the terminal
// heuristic. A pipe or a regular-file redirect is not (reports false); a
// terminal is. /dev/null is also a character device and reads true, which is
// harmless for the color-gating use case (escapes written there are
// discarded). A stat error reports false.
func isCharDevice(f *stdos.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&stdos.ModeCharDevice != 0
}

// flagFn returns the argument that immediately follows `name` in
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
