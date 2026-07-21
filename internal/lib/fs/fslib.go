// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package fslib implements Jennifer's `fs` library: blocking
// whole-file reads and writes, filesystem metadata, directory
// operations, and buffered file handles for line-oriented reads.
//
// The library is blocking on purpose - non-blocking use composes with
// `spawn` rather than duplicating each call as a `*Async`
// variant. See docs/milestones.md and docs/libraries/fs.md.
//
// Two-verbs pattern for recursion: `mkdir` / `mkdirAll` and `remove` /
// `removeAll` each ship as two names. The safe default keeps the same
// verb; the recursive form gets its own name so a code review can
// grep for it. This is Jennifer's "no footguns" stance applied at the
// API level.
//
// File handles (fs.open / fs.readLine / fs.close) use the
// integer-handle pattern: `fs.File{id as int}` on the Jennifer side
// indexes into a package-scope registry of Go `*os.File` state. See
// handles.go for the registry implementation.
package fslib

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "fs"

// Install registers the fs surface with the interpreter.
func Install(in *interpreter.Interpreter) {
	// fs.Stat mirrors the fields most callers want without dragging in
	// `time.Time` (fs stays decoupled from time at the Go-package
	// level). Callers who want a time.Time for the mtime write
	// `time.fromUnixNanos($stat.mtimeNanos)` explicitly.
	in.RegisterNamespacedStruct(LibraryName, "Stat", []parser.StructField{
		{Name: "path", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "size", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "isDir", Type: parser.PrimitiveType(parser.TypeBool)},
		{Name: "mtimeNanos", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "mode", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	// fs.File wraps an integer id into the handle registry. See
	// handles.go for the open/close/read/write/eof machinery.
	in.RegisterNamespacedStruct(LibraryName, "File", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	// One-shot ops. Several verbs (writeString, writeBytes, readBytes)
	// are polymorphic on argument shape - the dispatcher versions live
	// in handles.go and route to the one-shot helpers below when the
	// call matches the path-based form. Only the strictly-one-shot
	// verbs (readString, appendString, appendBytes) register directly
	// here.
	in.RegisterNamespaced(LibraryName, "readString", readStringFn)
	in.RegisterNamespaced(LibraryName, "appendString", appendStringFn)
	in.RegisterNamespaced(LibraryName, "appendBytes", appendBytesFn)

	// Metadata.
	in.RegisterNamespaced(LibraryName, "exists", existsFn)
	in.RegisterNamespaced(LibraryName, "isFile", isFileFn)
	in.RegisterNamespaced(LibraryName, "isDir", isDirFn)
	in.RegisterNamespaced(LibraryName, "stat", statFn)
	in.RegisterNamespaced(LibraryName, "chmod", chmodFn)
	in.RegisterNamespaced(LibraryName, "chown", chownFn)

	// Directory operations. Two verbs each for create + delete; the
	// recursive form is the second name so a code review sees which
	// call sites do the risky thing.
	in.RegisterNamespaced(LibraryName, "mkdir", mkdirFn)
	in.RegisterNamespaced(LibraryName, "mkdirAll", mkdirAllFn)
	in.RegisterNamespaced(LibraryName, "remove", removeFn)
	in.RegisterNamespaced(LibraryName, "removeAll", removeAllFn)
	in.RegisterNamespaced(LibraryName, "rename", renameFn)
	in.RegisterNamespaced(LibraryName, "list", listFn)
	in.RegisterNamespaced(LibraryName, "walk", walkFn)

	// Temporary-entry creation (temp.go): unique name, atomic create, path
	// returned. Strict - any OS failure is a catchable error.
	in.RegisterNamespaced(LibraryName, "makeTempFile", makeTempFileFn)
	in.RegisterNamespaced(LibraryName, "makeTempDir", makeTempDirFn)

	// File-handle surface lives in handles.go.
	installHandles(in)
}

// takeStringArg is the boundary check for a positional string argument.
func takeStringArg(fnName string, args []Value, idx int, role string) (string, error) {
	if args[idx].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Str, nil
}

// Value type-alias keeps signatures short.
type Value = interpreter.Value

// -------- One-shot reads --------

func readStringFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.readString expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.readString", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("fs.readString: %s: %v", path, err)
	}
	if !utf8.Valid(data) {
		return interpreter.Null(), fmt.Errorf("fs.readString: %s: not valid UTF-8", path)
	}
	return interpreter.StringVal(string(data)), nil
}

func readBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.readBytes expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.readBytes", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("fs.readBytes: %s: %v", path, err)
	}
	return interpreter.BytesVal(data), nil
}

// -------- One-shot writes --------

// writeCommon is the shared "open with these flags + write payload"
// path for the four write / append verbs. Kept in one place so the
// error strings and modes stay consistent.
func writeCommon(fnName string, path string, payload []byte, flags int) error {
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %s: %v", fnName, path, err)
	}
	if _, werr := f.Write(payload); werr != nil {
		f.Close()
		return fmt.Errorf("%s: %s: %v", fnName, path, werr)
	}
	// Propagate the Close error explicitly: on NFS / quota volumes the write
	// can succeed into a buffer and the flush-on-close fail, so a deferred
	// Close that dropped the error would report a truncated file as written.
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("%s: %s: %v", fnName, path, cerr)
	}
	return nil
}

func writeStringFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.writeString expects 2 arguments (path, content), got %d", len(args))
	}
	path, err := takeStringArg("fs.writeString", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("fs.writeString: content must be string, got %s", args[1].Kind)
	}
	if err := writeCommon("fs.writeString", path, []byte(args[1].Str), os.O_WRONLY|os.O_CREATE|os.O_TRUNC); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

func writeBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.writeBytes expects 2 arguments (path, content), got %d", len(args))
	}
	path, err := takeStringArg("fs.writeBytes", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("fs.writeBytes: content must be bytes, got %s", args[1].Kind)
	}
	if err := writeCommon("fs.writeBytes", path, args[1].Bytes, os.O_WRONLY|os.O_CREATE|os.O_TRUNC); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

func appendStringFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.appendString expects 2 arguments (path, content), got %d", len(args))
	}
	path, err := takeStringArg("fs.appendString", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("fs.appendString: content must be string, got %s", args[1].Kind)
	}
	if err := writeCommon("fs.appendString", path, []byte(args[1].Str), os.O_WRONLY|os.O_CREATE|os.O_APPEND); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

func appendBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.appendBytes expects 2 arguments (path, content), got %d", len(args))
	}
	path, err := takeStringArg("fs.appendBytes", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("fs.appendBytes: content must be bytes, got %s", args[1].Kind)
	}
	if err := writeCommon("fs.appendBytes", path, args[1].Bytes, os.O_WRONLY|os.O_CREATE|os.O_APPEND); err != nil {
		return interpreter.Null(), err
	}
	return interpreter.Null(), nil
}

// -------- Metadata --------

func existsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.exists expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.exists", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		return interpreter.BoolVal(true), nil
	}
	if os.IsNotExist(statErr) {
		return interpreter.BoolVal(false), nil
	}
	// Permission denied and other unusual errors get surfaced. Users
	// who want "true/false only, never error" can wrap with try/catch.
	return interpreter.Null(), fmt.Errorf("fs.exists: %s: %v", path, statErr)
}

func isFileFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.isFile expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.isFile", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return interpreter.BoolVal(false), nil
		}
		return interpreter.Null(), fmt.Errorf("fs.isFile: %s: %v", path, statErr)
	}
	return interpreter.BoolVal(info.Mode().IsRegular()), nil
}

func isDirFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.isDir expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.isDir", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return interpreter.BoolVal(false), nil
		}
		return interpreter.Null(), fmt.Errorf("fs.isDir: %s: %v", path, statErr)
	}
	return interpreter.BoolVal(info.IsDir()), nil
}

// makeStat is the shared "os.FileInfo -> fs.Stat Value" conversion.
// `path` is what the caller passed in (or, for walk entries, the joined
// absolute-ish path used to reach the entry). Size is -1 for directories
// so callers can't accidentally interpret it as "empty directory."
func makeStat(path string, info os.FileInfo) Value {
	size := info.Size()
	if info.IsDir() {
		size = -1
	}
	return interpreter.NamespacedStructVal(LibraryName, "Stat", []interpreter.StructField{
		{Name: "path", Value: interpreter.StringVal(path)},
		{Name: "size", Value: interpreter.IntVal(size)},
		{Name: "isDir", Value: interpreter.BoolVal(info.IsDir())},
		{Name: "mtimeNanos", Value: interpreter.IntVal(info.ModTime().UnixNano())},
		{Name: "mode", Value: interpreter.IntVal(int64(info.Mode().Perm()))},
	})
}

func statFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.stat expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.stat", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.stat: %s: %v", path, statErr)
	}
	return makeStat(path, info), nil
}

// chmodFn sets a file's permission bits: fs.chmod(path, mode). mode is the
// low 12 bits (rwx + setuid/setgid/sticky), e.g. 0o600 for an owner-only
// secret file - the piece the write verbs (fixed 0o644) can't express, so a
// token / key store can be tightened after writing. Rejects a mode outside
// [0, 0o7777]; any OS failure is a catchable error. Unix/Linux semantics: on
// Windows only the read-only bit is honoured (the platform target is Linux).
func chmodFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.chmod expects 2 arguments (path, mode), got %d", len(args))
	}
	path, err := takeStringArg("fs.chmod", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("fs.chmod: mode must be int, got %s", args[1].Kind)
	}
	mode := args[1].Int
	if mode < 0 || mode > 0o7777 {
		return interpreter.Null(), fmt.Errorf("fs.chmod: mode %d out of range [0, 0o7777]", mode)
	}
	if cerr := os.Chmod(path, os.FileMode(mode)); cerr != nil {
		return interpreter.Null(), fmt.Errorf("fs.chmod: %s: %v", path, cerr)
	}
	return interpreter.Null(), nil
}

// chownFn sets a file's owner and group: fs.chown(path, uid, gid). A uid or gid
// of -1 leaves that id unchanged (chown(2) semantics). Linux/Unix only - on
// Windows os.Chown always fails (the platform target is Linux); typically needs
// privilege. Any OS failure is a catchable error.
func chownFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("fs.chown expects 3 arguments (path, uid, gid), got %d", len(args))
	}
	path, err := takeStringArg("fs.chown", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("fs.chown: uid must be int, got %s", args[1].Kind)
	}
	if args[2].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("fs.chown: gid must be int, got %s", args[2].Kind)
	}
	if cerr := os.Chown(path, int(args[1].Int), int(args[2].Int)); cerr != nil {
		return interpreter.Null(), fmt.Errorf("fs.chown: %s: %v", path, cerr)
	}
	return interpreter.Null(), nil
}

// -------- Directory operations --------

func mkdirFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.mkdir expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.mkdir", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if mkErr := os.Mkdir(path, 0o755); mkErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.mkdir: %s: %v", path, mkErr)
	}
	return interpreter.Null(), nil
}

func mkdirAllFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.mkdirAll expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.mkdirAll", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if mkErr := os.MkdirAll(path, 0o755); mkErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.mkdirAll: %s: %v", path, mkErr)
	}
	return interpreter.Null(), nil
}

func removeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.remove expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.remove", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if rmErr := os.Remove(path); rmErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.remove: %s: %v", path, rmErr)
	}
	return interpreter.Null(), nil
}

func removeAllFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.removeAll expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.removeAll", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	if rmErr := os.RemoveAll(path); rmErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.removeAll: %s: %v", path, rmErr)
	}
	return interpreter.Null(), nil
}

func renameFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("fs.rename expects 2 arguments (old, new), got %d", len(args))
	}
	oldPath, err := takeStringArg("fs.rename", args, 0, "old path")
	if err != nil {
		return interpreter.Null(), err
	}
	newPath, err := takeStringArg("fs.rename", args, 1, "new path")
	if err != nil {
		return interpreter.Null(), err
	}
	if rnErr := os.Rename(oldPath, newPath); rnErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.rename: %s -> %s: %v", oldPath, newPath, rnErr)
	}
	return interpreter.Null(), nil
}

func listFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.list expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.list", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	entries, readErr := os.ReadDir(path)
	if readErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.list: %s: %v", path, readErr)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)
	out := make([]Value, len(names))
	for i, n := range names {
		out[i] = interpreter.StringVal(n)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// walkFn does a depth-first walk with sorted per-directory entries.
// Skips symlinks (no follow, no report) in v1 to sidestep symlink-loop
// pitfalls. The root itself is included as the first entry so callers
// can inspect it (matches Go's `filepath.Walk` behaviour).
func walkFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("fs.walk expects 1 argument (path), got %d", len(args))
	}
	path, err := takeStringArg("fs.walk", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	var results []Value
	walkErr := filepath.Walk(path, func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		// Skip symlinks entirely - Walk already dereferences them
		// via Lstat / Stat; we filter the result out.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		results = append(results, makeStat(p, info))
		return nil
	})
	if walkErr != nil {
		return interpreter.Null(), fmt.Errorf("fs.walk: %s: %v", path, walkErr)
	}
	statType := parser.NamespacedStructType(LibraryName, "Stat")
	return interpreter.ListVal(statType, results), nil
}
