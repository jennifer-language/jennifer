// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package metalib implements Jennifer's `meta` library: a home for
// constants that describe the running Jennifer interpreter itself
// rather than the host environment (which lives in `os`) or any user
// data (which is the rest of the standard library).
//
// The library holds interpreter-identity constants (`meta.VERSION`,
// `meta.BUILD`, `meta.SYSMODDIR`) plus a small reflection surface
// (`meta.call` / `meta.defined`) for invoking a top-level user method by
// a runtime string name. The library exists because `meta.VERSION` and
// `meta.BUILD` are conceptually the same kind of fact ("identity of this
// interpreter binary") and historically `JENNIFER_VERSION` lived in
// `core` for lack of a better home. With `meta` shipping, `core` returns
// to its strict charter of polymorphic structural primitives. The 5+-name
// rule that normally gates new libraries is bent here because
// interpreter-identity and interpreter-reflection facts don't fit any
// existing library cleanly; further runtime introspection (git SHA, GC
// stats, scheduler info) has a natural home here too.
//
// All names are namespaced under the `meta.` prefix. Programs enable
// the library with `use meta;` like every other non-core library.
package metalib

import (
	"fmt"
	"runtime"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/version"
)

// LibraryName is the Jennifer name programs `use` to enable these
// constants, and doubles as the namespace prefix.
const LibraryName = "meta"

// sysmoddir holds the resolved system module directory. The CLI resolves
// it from its --sysmoddir / JENNIFER_SYSMODDIR / compile-time layers and
// hands the winning value to SetSysmoddir before Install runs, so
// meta.SYSMODDIR reflects the actual resolved directory (not a static
// Install-time constant). Empty until set.
var sysmoddir string

// SetSysmoddir records the resolved system module directory for the
// meta.SYSMODDIR constant. The CLI calls it once at startup, after
// resolving the layer precedence, before Install.
func SetSysmoddir(dir string) { sysmoddir = dir }

// Install registers the meta library's constants and reflection helpers.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedConst(LibraryName, "VERSION", interpreter.StringVal(version.Version))
	in.RegisterNamespacedConst(LibraryName, "BUILD", interpreter.StringVal(buildTag()))
	in.RegisterNamespacedConst(LibraryName, "SYSMODDIR", interpreter.StringVal(sysmoddir))

	// Dynamic dispatch: invoke a top-level user method by a runtime string
	// name. The general form of what `testing.run` does for tests - a
	// framework (the `web` module) matches a request to a handler name and
	// calls it here. Unlike `testing.run`, it does not catch `exit`: dispatch
	// is transparent, so every sentinel (runtime error, thrown Error, exit)
	// propagates to the caller, catchable with `try` / `catch`.
	in.RegisterNamespaced(LibraryName, "call", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) < 1 {
			return interpreter.Null(), fmt.Errorf("meta.call expects at least 1 argument (name[, args...]), got 0")
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("meta.call: name must be string, got %s", args[0].Kind)
		}
		return in.CallByNameWith(args[0].Str, args[1:]...)
	})

	// meta.defined(name) reports whether a top-level user method exists, so a
	// dispatcher can validate a handler name at registration time.
	in.RegisterNamespaced(LibraryName, "defined", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("meta.defined expects 1 argument (name), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("meta.defined: name must be string, got %s", args[0].Kind)
		}
		return interpreter.BoolVal(in.HasMethod(args[0].Str)), nil
	})

	// meta.callMain / meta.definedMain are the module-to-entry-program siblings
	// of call / defined: they resolve against the *entry program's* top-level
	// methods rather than the caller's. Modules run on isolated
	// sub-interpreters, so a framework module (like `web`) that dispatches to
	// handler methods defined in the program that imported it must reach across
	// that boundary - this is the one explicit way to do so. Called from the
	// entry program itself, `main` and the plain forms coincide (the entry
	// program is its own host).
	in.RegisterNamespaced(LibraryName, "callMain", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) < 1 {
			return interpreter.Null(), fmt.Errorf("meta.callMain expects at least 1 argument (name[, args...]), got 0")
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("meta.callMain: name must be string, got %s", args[0].Kind)
		}
		return in.CallHostWith(args[0].Str, args[1:]...)
	})

	in.RegisterNamespaced(LibraryName, "definedMain", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("meta.definedMain expects 1 argument (name), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("meta.definedMain: name must be string, got %s", args[0].Kind)
		}
		return interpreter.BoolVal(in.Host().HasMethod(args[0].Str)), nil
	})
}

// buildTag distinguishes which Go variant compiled the interpreter.
// TinyGo sets `runtime.Compiler` to `"tinygo"`; the standard `go`
// toolchain sets it to `"gc"`, which we normalise to `"go"` because
// it's the user-facing toolchain name even if `gc` is the internal
// compiler identifier. Any other value is passed through so future
// alternative compilers (gccgo, etc.) appear honestly.
func buildTag() string {
	switch runtime.Compiler {
	case "tinygo":
		return "tinygo"
	case "gc":
		return "go"
	default:
		return runtime.Compiler
	}
}
