// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package mapslib implements Jennifer's `maps` library (M9): the
// non-mutating manipulation helpers for `map of K to V` values.
// Every function returns a *new* map or list; the input is never
// modified. Callers commit results with `$m = maps.delete($m, k);`.
//
// All names are namespaced under the `maps.` prefix per M8's hybrid
// model. `maps.delete` would collide with future `set.delete`,
// `dict.delete`, etc., and namespacing keeps the call-site clear.
//
// The Go package is named mapslib to avoid colliding with Go 1.21's
// standard `maps` package.
package mapslib

import (
	"fmt"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these
// functions, and doubles as the namespace prefix.
const LibraryName = "maps"

// Install registers every M9 maps builtin with the interpreter.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "keys", keysFn)
	in.RegisterNamespaced(LibraryName, "values", valuesFn)
	in.RegisterNamespaced(LibraryName, "has", hasFn)
	in.RegisterNamespaced(LibraryName, "delete", deleteFn)
	in.RegisterNamespaced(LibraryName, "merge", mergeFn)
}

// hasFn reports whether the map contains the given key. The companion
// to the M6 decision that reads of missing keys are runtime errors:
// callers who need a non-erroring "does it exist?" check use
// `maps.has($m, key)`. Pre-M9 this lived in core as bare `has(...)`;
// it moved here because map-only membership is domain-specific and
// didn't fit core's "universally needed structural primitives"
// charter.
func hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("maps.has expects 2 arguments (map, key), got %d", len(args))
	}
	if err := requireMap("has", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	for _, e := range args[0].Map {
		if e.Key.Equal(args[1]) {
			return interpreter.BoolVal(true), nil
		}
	}
	return interpreter.BoolVal(false), nil
}

func requireMap(name string, v interpreter.Value, argpos string) error {
	if v.Kind != interpreter.KindMap {
		return fmt.Errorf("maps.%s: %s must be a map, got %s", name, argpos, v.Kind)
	}
	return nil
}

// keysFn returns the map's keys as a list, in insertion order.
func keysFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("maps.keys expects 1 argument, got %d", len(args))
	}
	if err := requireMap("keys", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	out := make([]interpreter.Value, len(args[0].Map))
	for i, e := range args[0].Map {
		out[i] = e.Key.Copy()
	}
	return interpreter.Value{Kind: interpreter.KindList, List: out}, nil
}

// valuesFn returns the map's values as a list, in insertion order.
func valuesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("maps.values expects 1 argument, got %d", len(args))
	}
	if err := requireMap("values", args[0], "argument"); err != nil {
		return interpreter.Null(), err
	}
	out := make([]interpreter.Value, len(args[0].Map))
	for i, e := range args[0].Map {
		out[i] = e.Value.Copy()
	}
	return interpreter.Value{Kind: interpreter.KindList, List: out}, nil
}

// deleteFn returns a new map without the entry for `key`. Missing key
// is a positioned runtime error - "strict at boundaries" matches the
// read-side behaviour where `$m[missing]` also errors. Callers who
// want best-effort delete should guard with `has($m, key)` first.
func deleteFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("maps.delete expects 2 arguments (map, key), got %d", len(args))
	}
	if err := requireMap("delete", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	for i, e := range args[0].Map {
		if e.Key.Equal(args[1]) {
			out := args[0].Copy()
			out.Map = append(out.Map[:i:i], out.Map[i+1:]...)
			return out, nil
		}
	}
	return interpreter.Null(), fmt.Errorf("maps.delete: map has no entry for key %s", args[1].Display())
}

// mergeFn returns a new map with `b`'s entries layered on top of
// `a`'s. Existing `a` keys are overwritten by `b`; new `b` keys are
// appended in `b`'s insertion order.
func mergeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("maps.merge expects 2 arguments, got %d", len(args))
	}
	if err := requireMap("merge", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	if err := requireMap("merge", args[1], "second argument"); err != nil {
		return interpreter.Null(), err
	}
	out := args[0].Copy()
	for _, be := range args[1].Map {
		replaced := false
		for i := range out.Map {
			if out.Map[i].Key.Equal(be.Key) {
				out.Map[i].Value = be.Value.Copy()
				replaced = true
				break
			}
		}
		if !replaced {
			out.Map = append(out.Map, interpreter.MapEntry{Key: be.Key.Copy(), Value: be.Value.Copy()})
		}
	}
	return out, nil
}
