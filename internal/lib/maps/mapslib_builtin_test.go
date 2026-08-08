// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mapslib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// These cover maps.has (previously 0%) and the error branches of the other
// builtins (wrong arity, non-map argument) that the happy-path tests skip.

func strIntMap(pairs ...[2]interface{}) interpreter.Value {
	entries := make([]interpreter.MapEntry, len(pairs))
	for i, p := range pairs {
		entries[i] = interpreter.MapEntry{
			Key:   interpreter.StringVal(p[0].(string)),
			Value: interpreter.IntVal(int64(p[1].(int))),
		}
	}
	return interpreter.MapVal(parser.PrimitiveType(parser.TypeString), parser.PrimitiveType(parser.TypeInt), entries)
}

func TestHasFn(t *testing.T) {
	m := strIntMap([2]interface{}{"a", 1}, [2]interface{}{"b", 2})
	present, err := hasFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, interpreter.StringVal("a")})
	if err != nil || present.Kind != interpreter.KindBool || !present.Bool {
		t.Errorf("has(m, \"a\") = %+v, err %v; want true", present, err)
	}
	absent, _ := hasFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, interpreter.StringVal("z")})
	if absent.Bool {
		t.Error("has(m, \"z\") should be false")
	}
	// Errors: arity, non-map first argument.
	if _, err := hasFn(interpreter.BuiltinCtx{}, []interpreter.Value{m}); err == nil {
		t.Error("has arity error expected")
	}
	if _, err := hasFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1), interpreter.StringVal("a")}); err == nil {
		t.Error("has(non-map) should error")
	}
}

func TestMapBuiltinArgumentErrors(t *testing.T) {
	m := strIntMap([2]interface{}{"a", 1})
	notMap := interpreter.IntVal(5)

	type call struct {
		name string
		fn   func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error)
	}
	single := []call{{"keys", keysFn}, {"values", valuesFn}}
	for _, c := range single {
		if _, err := c.fn(interpreter.BuiltinCtx{}, nil); err == nil {
			t.Errorf("%s() with no args should error", c.name)
		}
		if _, err := c.fn(interpreter.BuiltinCtx{}, []interpreter.Value{notMap}); err == nil {
			t.Errorf("%s(non-map) should error", c.name)
		}
	}

	// delete: arity + non-map first arg.
	if _, err := deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{m}); err == nil {
		t.Error("delete arity error expected")
	}
	if _, err := deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{notMap, interpreter.StringVal("a")}); err == nil {
		t.Error("delete(non-map) should error")
	}

	// merge: arity + a non-map on either side.
	if _, err := mergeFn(interpreter.BuiltinCtx{}, []interpreter.Value{m}); err == nil {
		t.Error("merge arity error expected")
	}
	if _, err := mergeFn(interpreter.BuiltinCtx{}, []interpreter.Value{notMap, m}); err == nil {
		t.Error("merge(non-map, map) should error")
	}
	if _, err := mergeFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, notMap}); err == nil {
		t.Error("merge(map, non-map) should error")
	}
}

func TestMergeAndDeleteResults(t *testing.T) {
	a := strIntMap([2]interface{}{"a", 1}, [2]interface{}{"b", 2})
	b := strIntMap([2]interface{}{"b", 20}, [2]interface{}{"c", 3})
	merged, err := mergeFn(interpreter.BuiltinCtx{}, []interpreter.Value{a, b})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	// b overlays a: a=1, b=20 (overwritten), c=3 (appended). Insertion order kept.
	if len(merged.Map) != 3 {
		t.Fatalf("merged has %d entries, want 3", len(merged.Map))
	}
	if merged.Map[0].Value.Int != 1 || merged.Map[1].Value.Int != 20 || merged.Map[2].Value.Int != 3 {
		t.Errorf("merge values = %d/%d/%d, want 1/20/3",
			merged.Map[0].Value.Int, merged.Map[1].Value.Int, merged.Map[2].Value.Int)
	}
	// The inputs are untouched (non-mutating).
	if a.Map[1].Value.Int != 2 {
		t.Error("merge mutated its first argument")
	}

	del, err := deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{a, interpreter.StringVal("a")})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(del.Map) != 1 || del.Map[0].Key.Str != "b" {
		t.Errorf("delete(a) left %+v, want just b", del.Map)
	}
	if len(a.Map) != 2 {
		t.Error("delete mutated its input")
	}
}
