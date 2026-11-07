// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package mapslib implements Jennifer's `maps` library: the
// non-mutating manipulation helpers for `map of K to V` values.
// Every function returns a *new* map or list; the input is never
// modified. Callers commit results with `$m = maps.delete($m, k);`.
//
// All names are namespaced under the `maps.` prefix per the hybrid
// model. `maps.delete` would collide with future `set.delete`,
// `dict.delete`, etc., and namespacing keeps the call-site clear.
//
// The Go package is named mapslib to avoid colliding with Go 1.21's
// standard `maps` package.
package mapslib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the Jennifer name programs `use` to enable these
// functions, and doubles as the namespace prefix.
const LibraryName = "maps"

// Install registers every maps builtin with the interpreter.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "keys", keysFn)
	in.RegisterNamespaced(LibraryName, "values", valuesFn)
	in.RegisterNamespaced(LibraryName, "has", hasFn)
	in.RegisterNamespaced(LibraryName, "delete", deleteFn)
	in.RegisterNamespaced(LibraryName, "without", withoutFn)
	in.RegisterNamespaced(LibraryName, "merge", mergeFn)
}

// hasFn reports whether the map contains the given key. The companion
// to the decision that reads of missing keys are runtime errors:
// callers who need a non-erroring "does it exist?" check use
// `maps.has($m, key)`. This lived in core as bare `has(...)`;
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
	// Index-aware: O(1) when the map's hash index is usable, O(n) fallback
	// otherwise (so a has() in a loop is O(n), not O(n^2)).
	return interpreter.BoolVal(args[0].LookupKey(args[1]) >= 0), nil
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
	// Index-aware lookup, then one pass that deep-copies every other entry
	// into a fresh backing. Positions shift after the removal, so the source's
	// index does not carry over; the result starts without one (built on next
	// indexed use, or the linear fallback). Copy() would deep-copy all entries
	// and build an index only to discard both.
	if i := args[0].LookupKey(args[1]); i >= 0 {
		src := args[0].Map
		out := args[0]
		out.Map = make([]interpreter.MapEntry, 0, len(src)-1)
		for j, e := range src {
			if j != i {
				out.Map = append(out.Map, interpreter.MapEntry{Key: e.Key.Copy(), Value: e.Value.Copy()})
			}
		}
		out.DropMapIndex()
		return out, nil
	}
	return interpreter.Null(), fmt.Errorf("maps.delete: map has no entry for key %s", args[1].Display())
}

// withoutFn returns a new map without the entries for every key in the list
// `keys`, built with one copy (O(n + k) with the map's index). Strict like
// delete: a key the map lacks is an error naming it. A key listed twice is
// removed once.
func withoutFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("maps.without expects 2 arguments (map, keys), got %d", len(args))
	}
	if err := requireMap("without", args[0], "first argument"); err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("maps.without: second argument must be a list, got %s", args[1].Kind)
	}
	src := args[0].Map
	drop := make([]bool, len(src))
	n := 0
	for _, k := range args[1].List {
		i := args[0].LookupKey(k)
		if i < 0 {
			return interpreter.Null(), fmt.Errorf("maps.without: map has no entry for key %s", k.Display())
		}
		if !drop[i] {
			drop[i] = true
			n++
		}
	}
	out := args[0]
	out.Map = make([]interpreter.MapEntry, 0, len(src)-n)
	for j, e := range src {
		if !drop[j] {
			out.Map = append(out.Map, interpreter.MapEntry{Key: e.Key.Copy(), Value: e.Value.Copy()})
		}
	}
	out.DropMapIndex()
	return out, nil
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
	// Index-aware upsert keeps merge O(A+B) instead of O(A*B) (a linear scan
	// of `out` per entry of `b`).
	for _, be := range args[1].Map {
		out.UpsertKey(be.Key.Copy(), be.Value.Copy())
	}
	return out, nil
}
