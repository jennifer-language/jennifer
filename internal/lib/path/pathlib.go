// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package pathlib implements Jennifer's `path` library: OS-aware filesystem path
// manipulation (base / dir / join / ext / clean / ...), the string-only
// counterpart to what `fs` does for I/O. Every function is a pure string
// transform over Go's `path/filepath` - no call touches the disk, so unlike
// `fs` / `net` the library needs no build-tag split and is TinyGo-clean. Paths
// use the host separator (`/` on Linux, `\` on Windows), so a program that
// builds paths with `path.join` stays portable instead of hardcoding `/`.
//
// Note: `path.base` is path logic, not sanitization - on Linux it does not strip
// a `\` (a legal Unix filename byte), so it is not a safe way to neutralize an
// untrusted filename.
package pathlib

import (
	"fmt"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "path"

// Value type-alias keeps signatures short.
type Value = interpreter.Value

// Install registers the path library functions on an interpreter. Namespaced:
// every name lives behind the `path.` prefix.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "base", baseFn)
	in.RegisterNamespaced(LibraryName, "dir", dirFn)
	in.RegisterNamespaced(LibraryName, "ext", extFn)
	in.RegisterNamespaced(LibraryName, "stem", stemFn)
	in.RegisterNamespaced(LibraryName, "join", joinFn)
	in.RegisterNamespaced(LibraryName, "clean", cleanFn)
	in.RegisterNamespaced(LibraryName, "isAbs", isAbsFn)
	in.RegisterNamespaced(LibraryName, "split", splitFn)
}

// one reads the single string argument of a one-arg path function.
func one(name string, args []Value) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("path.%s expects 1 argument (path), got %d", name, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return "", fmt.Errorf("path.%s: path must be string, got %s", name, args[0].Kind)
	}
	return args[0].Str, nil
}

// baseFn: path.base(p) - the last element of p (filepath.Base).
func baseFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("base", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(filepath.Base(p)), nil
}

// dirFn: path.dir(p) - all but the last element of p, cleaned (filepath.Dir).
func dirFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("dir", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(filepath.Dir(p)), nil
}

// extFn: path.ext(p) - the file extension incl. the leading dot, or "".
func extFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("ext", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(filepath.Ext(p)), nil
}

// stemFn: path.stem(p) - the base name of p without its extension.
func stemFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("stem", args)
	if err != nil {
		return interpreter.Null(), err
	}
	b := filepath.Base(p)
	return interpreter.StringVal(strings.TrimSuffix(b, filepath.Ext(b))), nil
}

// cleanFn: path.clean(p) - the shortest equivalent path (filepath.Clean).
func cleanFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("clean", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(filepath.Clean(p)), nil
}

// isAbsFn: path.isAbs(p) - whether p is an absolute path (filepath.IsAbs).
func isAbsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("isAbs", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(filepath.IsAbs(p)), nil
}

// joinFn: path.join(a, b, ...) - join any number of elements into one path,
// cleaned; empty elements are dropped (filepath.Join). At least one argument.
func joinFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 {
		return interpreter.Null(), fmt.Errorf("path.join expects at least 1 argument, got 0")
	}
	parts := make([]string, len(args))
	for i, a := range args {
		if a.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("path.join: argument %d must be string, got %s", i+1, a.Kind)
		}
		parts[i] = a.Str
	}
	return interpreter.StringVal(filepath.Join(parts...)), nil
}

// splitFn: path.split(p) - [dir, file], where dir keeps its trailing separator
// (filepath.Split): dir + file == p.
func splitFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	p, err := one("split", args)
	if err != nil {
		return interpreter.Null(), err
	}
	dir, file := filepath.Split(p)
	out := []Value{interpreter.StringVal(dir), interpreter.StringVal(file)}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}
