// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Temporary-file / temporary-directory creation. `fs.makeTempFile` and
// `fs.makeTempDir` create a fresh, uniquely-named entry and return its path.
// Creation is atomic (the OS reserves the name; the file is 0600, the directory
// 0700), so two concurrent callers never collide and a scratch file is not
// world-readable. Both are strict: any OS failure (read-only filesystem, no
// permission, a missing parent directory, a name the filesystem cannot hold)
// surfaces as a catchable error - they never fabricate a directory tree or
// mangle a name to fit.
//
// Arguments are positional and trailing-optional, `""` meaning the default:
//
//	fs.makeTempDir([dir[, prefix]])
//	fs.makeTempFile([dir[, prefix[, suffix]]])
//
// `dir == ""` uses the system temp directory (os.tempDir()); an explicit `dir`
// must already exist (only the final unique component is created - like
// `fs.mkdir`, not `fs.mkdirAll`; run `fs.mkdirAll` first if the parent is
// missing). `prefix` / `suffix` name the entry around the random component
// (`suffix` gives a file a real extension); neither may contain a path separator
// or NUL.
package fslib

import (
	"fmt"
	"os"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// tempArgs parses the trailing-optional (dir, prefix, suffix) string arguments,
// defaulting any that are omitted to "" and validating the name components.
func tempArgs(fnName string, args []Value, maxN int) (dir, prefix, suffix string, err error) {
	if len(args) > maxN {
		return "", "", "", fmt.Errorf("%s expects at most %d arguments, got %d", fnName, maxN, len(args))
	}
	parts := make([]string, maxN)
	for i := 0; i < len(args); i++ {
		if args[i].Kind != interpreter.KindString {
			return "", "", "", fmt.Errorf("%s: argument %d must be string, got %s", fnName, i+1, args[i].Kind)
		}
		parts[i] = args[i].Str
	}
	dir = parts[0]
	prefix = parts[1]
	if maxN >= 3 {
		suffix = parts[2]
	}
	// dir is a real path (separators allowed); prefix / suffix are single name
	// components, so a separator or NUL in them would escape the target directory
	// or be rejected by the OS with a cryptic message.
	if e := checkNameComponent(fnName, "prefix", prefix); e != nil {
		return "", "", "", e
	}
	if e := checkNameComponent(fnName, "suffix", suffix); e != nil {
		return "", "", "", e
	}
	return dir, prefix, suffix, nil
}

func checkNameComponent(fnName, role, s string) error {
	if strings.ContainsAny(s, "/\\\x00") {
		return fmt.Errorf("%s: %s must not contain a path separator or NUL, got %q", fnName, role, s)
	}
	return nil
}

// makeTempFileFn implements fs.makeTempFile([dir[, prefix[, suffix]]]) -> path.
// The file is created empty and closed; the caller writes it with the ordinary
// path-based verbs and removes it with fs.remove.
func makeTempFileFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	dir, prefix, suffix, err := tempArgs("fs.makeTempFile", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	// os.CreateTemp treats dir == "" as os.TempDir() and substitutes the random
	// string for the "*" between prefix and suffix.
	f, cerr := os.CreateTemp(dir, prefix+"*"+suffix)
	if cerr != nil {
		return interpreter.Null(), fmt.Errorf("fs.makeTempFile: %v", cerr)
	}
	name := f.Name()
	if clErr := f.Close(); clErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.makeTempFile: %s: %v", name, clErr)
	}
	return interpreter.StringVal(name), nil
}

// makeTempDirFn implements fs.makeTempDir([dir[, prefix]]) -> path.
func makeTempDirFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	dir, prefix, _, err := tempArgs("fs.makeTempDir", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	path, mkErr := os.MkdirTemp(dir, prefix+"*")
	if mkErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.makeTempDir: %v", mkErr)
	}
	return interpreter.StringVal(path), nil
}
