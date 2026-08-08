// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package tomllib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestTomlBuiltinSurface exercises the (node, pointer) accessors and the
// non-mutating write surface through the registered builtins - the whole layer
// the decode-focused tests leave uncovered.
func TestTomlBuiltinSurface(t *testing.T) {
	in := interpreter.New()
	Install(in)
	fn := func(name string) interpreter.Builtin {
		b := in.LookupNamespacedBuiltin("toml", name)
		if b == nil {
			t.Fatalf("toml.%s is not registered", name)
		}
		return b
	}
	call := func(name string, args ...interpreter.Value) interpreter.Value {
		t.Helper()
		v, err := fn(name)(interpreter.BuiltinCtx{}, args)
		if err != nil {
			t.Fatalf("toml.%s: %v", name, err)
		}
		return v
	}
	callErr := func(name string, args ...interpreter.Value) {
		t.Helper()
		if _, err := fn(name)(interpreter.BuiltinCtx{}, args); err == nil {
			t.Errorf("toml.%s should error", name)
		}
	}
	s := interpreter.StringVal

	// --- read side: decode + accessors ---
	src := "" +
		"title = \"Jennifer\"\n" +
		"count = 42\n" +
		"ratio = 3.5\n" +
		"yes = true\n" +
		"when = 2024-01-02T03:04:05Z\n" +
		"\n[owner]\n" +
		"name = \"mplx\"\n" +
		"tags = [\"a\", \"b\", \"c\"]\n"

	node := call("decode", s(src))
	if node.Kind != interpreter.KindObject {
		t.Fatalf("decode returned %s, want object", node.Kind)
	}

	if v := call("typeOf", node); v.Kind != interpreter.KindString || v.Str == "" {
		t.Errorf("typeOf(root) = %+v", v)
	}
	if v := call("asString", node, s("/title")); v.Str != "Jennifer" {
		t.Errorf("asString(/title) = %q", v.Str)
	}
	if v := call("asInt", node, s("/count")); v.Int != 42 {
		t.Errorf("asInt(/count) = %d", v.Int)
	}
	if v := call("asFloat", node, s("/ratio")); v.Float != 3.5 {
		t.Errorf("asFloat(/ratio) = %v", v.Float)
	}
	if v := call("asBool", node, s("/yes")); !v.Bool {
		t.Error("asBool(/yes) should be true")
	}
	if v := call("isDatetime", node, s("/when")); !v.Bool {
		t.Error("isDatetime(/when) should be true")
	}
	if v := call("asDatetime", node, s("/when")); v.Kind == interpreter.KindNull {
		t.Error("asDatetime(/when) should return a datetime value")
	}
	if v := call("has", node, s("/owner/name")); !v.Bool {
		t.Error("has(/owner/name) should be true")
	}
	if v := call("has", node, s("/missing")); v.Bool {
		t.Error("has(/missing) should be false")
	}
	if v := call("keys", node); v.Kind != interpreter.KindList || len(v.List) == 0 {
		t.Errorf("keys(root) = %+v, want a non-empty list", v)
	}
	if v := call("length", node, s("/owner/tags")); v.Int != 3 {
		t.Errorf("length(/owner/tags) = %d, want 3", v.Int)
	}
	if v := call("get", node, s("/owner")); v.Kind != interpreter.KindObject {
		t.Errorf("get(/owner) = %s, want object", v.Kind)
	}
	// accessor errors: a missing pointer, and a type mismatch.
	callErr("get", node, s("/does/not/exist"))
	callErr("asInt", node, s("/title")) // a string is not an int

	// --- write side: build a document with the non-mutating edit surface ---
	m := call("map")
	m = call("set", m, s("/name"), s("widget"))
	m = call("set", m, s("/count"), interpreter.IntVal(3))
	m = call("set", m, s("/tags"), call("list")) // an empty array
	m = call("append", m, s("/tags"), s("b"))
	m = call("append", m, s("/tags"), s("c"))
	m = call("insert", m, s("/tags/0"), s("a")) // insert at the front

	if v := call("length", m, s("/tags")); v.Int != 3 {
		t.Errorf("built tags length = %d, want 3", v.Int)
	}
	if v := call("asString", m, s("/tags/0")); v.Str != "a" {
		t.Errorf("tags[0] = %q, want a (inserted at front)", v.Str)
	}

	m = call("remove", m, s("/count"))
	if v := call("has", m, s("/count")); v.Bool {
		t.Error("count should be removed")
	}

	m = call("move", m, s("/name"), s("/label"))
	if v := call("asString", m, s("/label")); v.Str != "widget" {
		t.Errorf("moved value = %q, want widget", v.Str)
	}
	if v := call("has", m, s("/name")); v.Bool {
		t.Error("/name should be gone after move")
	}

	// --- encode + encodePretty round-trip back through decode ---
	enc := call("encode", m)
	if enc.Kind != interpreter.KindString || enc.Str == "" {
		t.Fatalf("encode = %+v, want a non-empty string", enc)
	}
	if v := call("encodePretty", m); v.Kind != interpreter.KindString || v.Str == "" {
		t.Errorf("encodePretty = %+v", v)
	}
	reparsed := call("decode", enc)
	if v := call("asString", reparsed, s("/label")); v.Str != "widget" {
		t.Errorf("re-decoded label = %q, want widget", v.Str)
	}
}
