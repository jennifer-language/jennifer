// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// node decodes s and wraps it as a json.Value the accessors accept.
func node(t *testing.T, s string) interpreter.Value {
	t.Helper()
	return interpreter.ObjectVal(LibraryName, "Value", dec(t, s))
}

// ptr is a JSON Pointer string argument.
func ptr(s string) interpreter.Value { return interpreter.StringVal(s) }

// call invokes an accessor and fails on error.
func call(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	out, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("accessor error: %v", err)
	}
	return out
}

func TestTypeOfAtPointer(t *testing.T) {
	doc := node(t, `{"n": 5, "f": 4.2, "s": "hi", "b": true, "z": null, "xs": [1], "m": {}}`)
	cases := map[string]string{
		"":    "map", // the node itself
		"/n":  "int",
		"/f":  "float",
		"/s":  "string",
		"/b":  "bool",
		"/z":  "null",
		"/xs": "list",
		"/m":  "map",
	}
	for p, want := range cases {
		got := call(t, typeOfFn, doc, ptr(p))
		if got.Str != want {
			t.Errorf("typeOf(%q) = %q, want %q", p, got.Str, want)
		}
	}
	// no pointer arg -> the root type.
	if got := call(t, typeOfFn, doc); got.Str != "map" {
		t.Errorf("typeOf(root) = %q, want map", got.Str)
	}
}

func TestDeepWalkAndLeaves(t *testing.T) {
	doc := node(t, `{"user": {"name": "ada", "age": 36, "roles": ["admin", "dev"], "ok": true, "z": null}}`)

	if got := call(t, asStringFn, doc, ptr("/user/name")); got.Str != "ada" {
		t.Errorf("name = %q, want ada", got.Str)
	}
	if got := call(t, asIntFn, doc, ptr("/user/age")); got.Int != 36 {
		t.Errorf("age = %d, want 36", got.Int)
	}
	// an integral number promotes to float via asFloat.
	if got := call(t, asFloatFn, doc, ptr("/user/age")); got.Float != 36.0 {
		t.Errorf("age asFloat = %v, want 36.0", got.Float)
	}
	if got := call(t, asStringFn, doc, ptr("/user/roles/1")); got.Str != "dev" {
		t.Errorf("roles/1 = %q, want dev", got.Str)
	}
	if got := call(t, asBoolFn, doc, ptr("/user/ok")); !got.Bool {
		t.Errorf("ok = %v, want true", got.Bool)
	}
	if got := call(t, isNullFn, doc, ptr("/user/z")); !got.Bool {
		t.Errorf("z isNull = %v, want true", got.Bool)
	}
	if got := call(t, lengthFn, doc, ptr("/user/roles")); got.Int != 2 {
		t.Errorf("roles length = %d, want 2", got.Int)
	}
}

func TestGetReturnsSubtreeAndComposesRelative(t *testing.T) {
	doc := node(t, `{"user": {"name": "ada"}}`)
	user := call(t, getFn, doc, ptr("/user"))
	// get returns a json.Value; a relative pointer walks from it.
	if _, ok := user.AsObject(LibraryName, "Value"); !ok {
		t.Fatalf("get did not return a json.Value")
	}
	if got := call(t, asStringFn, user, ptr("/name")); got.Str != "ada" {
		t.Errorf("relative name = %q, want ada", got.Str)
	}
	// get with no pointer returns the node itself.
	same := call(t, getFn, doc)
	if got := call(t, asStringFn, same, ptr("/user/name")); got.Str != "ada" {
		t.Errorf("get(root) then walk = %q, want ada", got.Str)
	}
}

func TestHasAndKeys(t *testing.T) {
	doc := node(t, `{"a": 1, "b": {"c": 2}}`)
	trueCases := []string{"/a", "/b", "/b/c"}
	for _, p := range trueCases {
		if got := call(t, hasFn, doc, ptr(p)); !got.Bool {
			t.Errorf("has(%q) = false, want true", p)
		}
	}
	falseCases := []string{"/z", "/b/z", "/a/x"} // missing key, missing nested, descend into scalar
	for _, p := range falseCases {
		if got := call(t, hasFn, doc, ptr(p)); got.Bool {
			t.Errorf("has(%q) = true, want false", p)
		}
	}
	keys := call(t, keysFn, doc)
	if len(keys.List) != 2 || keys.List[0].Str != "a" || keys.List[1].Str != "b" {
		t.Errorf("keys = %v, want [a b] in order", keys.List)
	}
}

func TestPointerEscaping(t *testing.T) {
	// RFC 6901: ~1 -> "/", ~0 -> "~".
	doc := node(t, `{"m~n": 8, "a/b": 0, "e^f": 3}`)
	if got := call(t, asIntFn, doc, ptr("/a~1b")); got.Int != 0 {
		t.Errorf("/a~1b = %d, want 0", got.Int)
	}
	if got := call(t, asIntFn, doc, ptr("/m~0n")); got.Int != 8 {
		t.Errorf("/m~0n = %d, want 8", got.Int)
	}
	if got := call(t, asIntFn, doc, ptr("/e^f")); got.Int != 3 {
		t.Errorf("/e^f = %d, want 3", got.Int)
	}
}

func TestAccessorErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
		want string
	}{
		{"missing key", asIntFn, []interpreter.Value{node(t, `{"a":1}`), ptr("/z")}, "no key"},
		{"index out of range", asIntFn, []interpreter.Value{node(t, `[1]`), ptr("/5")}, "out of range"},
		{"dash index on read", asIntFn, []interpreter.Value{node(t, `[1,2,3]`), ptr("/-")}, "not a valid list index"},
		{"leading-zero index", asIntFn, []interpreter.Value{node(t, `[1,2,3]`), ptr("/01")}, "not a valid list index"},
		{"descend into scalar", asIntFn, []interpreter.Value{node(t, `{"a":1}`), ptr("/a/b")}, "cannot descend into int"},
		{"pointer without slash", asIntFn, []interpreter.Value{node(t, `{"a":1}`), ptr("a")}, "must be empty or start with '/'"},
		{"asInt on float", asIntFn, []interpreter.Value{node(t, `4.2`)}, "not an int"},
		{"asString on int", asStringFn, []interpreter.Value{node(t, `1`)}, "not a string"},
		{"keys on non-map", keysFn, []interpreter.Value{node(t, `[1]`)}, "expected a map"},
		{"length on scalar", lengthFn, []interpreter.Value{node(t, `1`)}, "expected a list or map"},
		{"non-json.Value arg", asIntFn, []interpreter.Value{interpreter.IntVal(1)}, "must be a json.Value"},
	}
	for _, c := range cases {
		_, err := c.fn(interpreter.BuiltinCtx{}, c.args)
		if err == nil {
			t.Errorf("%s: expected an error", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
		}
	}
}

// TestDisplayRendersJSON confirms the registered displayer makes a json.Value
// render as its compact JSON (so `$v` at the REPL and `%v` show content, not
// the opaque `<json.Value>`). Install wires the displayer into the
// package-level registry.
func TestDisplayRendersJSON(t *testing.T) {
	Install(interpreter.New())

	if got := node(t, `{"a":[1,2],"b":null,"s":"hi"}`).Display(); got != `{"a":[1,2],"b":null,"s":"hi"}` {
		t.Errorf("Display = %q, want compact JSON", got)
	}
	// an empty (uninitialized) json.Value is a null node.
	empty := interpreter.ObjectVal(LibraryName, "Value", interpreter.Null())
	if got := empty.Display(); got != "null" {
		t.Errorf("empty Display = %q, want null", got)
	}
}

// TestRoundTripThroughObject confirms json.encode accepts a json.Value and
// reproduces the source document (a decode/encode round-trip).
func TestRoundTripThroughObject(t *testing.T) {
	src := `{"name":"ada","nums":[1,2,3],"ok":true,"z":null}`
	out, err := encodeFn([]interpreter.Value{node(t, src)}, false)
	if err != nil {
		t.Fatalf("encode(json.Value): %v", err)
	}
	if out.Str != src {
		t.Errorf("round-trip = %q, want %q", out.Str, src)
	}
}
