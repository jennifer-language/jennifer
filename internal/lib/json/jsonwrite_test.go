// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// jstr encodes a json.Value result to compact JSON for comparison.
func jstr(t *testing.T, v interpreter.Value) string {
	t.Helper()
	out, err := encodeFn([]interpreter.Value{v}, false)
	if err != nil {
		t.Fatalf("encode result: %v", err)
	}
	return out.Str
}

func TestConstructors(t *testing.T) {
	if got := jstr(t, call(t, mapFn)); got != "{}" {
		t.Errorf("json.map() = %q, want {}", got)
	}
	if got := jstr(t, call(t, listFn)); got != "[]" {
		t.Errorf("json.list() = %q, want []", got)
	}
}

func TestSetUpsertReplaceAndRoot(t *testing.T) {
	d := doc(t, `{"a":1,"xs":[10,20]}`)
	// upsert a new key
	if got := jstr(t, call(t, setFn, d, ptr("/b"), interpreter.IntVal(2))); got != `{"a":1,"xs":[10,20],"b":2}` {
		t.Errorf("upsert = %q", got)
	}
	// replace an existing key
	if got := jstr(t, call(t, setFn, d, ptr("/a"), interpreter.IntVal(9))); got != `{"a":9,"xs":[10,20]}` {
		t.Errorf("replace key = %q", got)
	}
	// replace an in-range list index
	if got := jstr(t, call(t, setFn, d, ptr("/xs/1"), interpreter.IntVal(99))); got != `{"a":1,"xs":[10,99]}` {
		t.Errorf("replace index = %q", got)
	}
	// empty pointer replaces the whole document
	if got := jstr(t, call(t, setFn, d, ptr(""), interpreter.StringVal("x"))); got != `"x"` {
		t.Errorf("replace root = %q", got)
	}
}

func TestWritesAreNonMutating(t *testing.T) {
	d := doc(t, `{"age":40}`)
	_ = call(t, setFn, d, ptr("/age"), interpreter.IntVal(41))
	if got := jstr(t, d); got != `{"age":40}` {
		t.Errorf("original mutated: %q", got)
	}
}

func TestInsertAppendRemove(t *testing.T) {
	d := doc(t, `{"xs":[1,2,3]}`)
	if got := jstr(t, call(t, insertFn, d, ptr("/xs/1"), interpreter.IntVal(9))); got != `{"xs":[1,9,2,3]}` {
		t.Errorf("insert = %q", got)
	}
	if got := jstr(t, call(t, insertFn, d, ptr("/xs/-"), interpreter.IntVal(9))); got != `{"xs":[1,2,3,9]}` {
		t.Errorf("insert at - = %q", got)
	}
	if got := jstr(t, call(t, appendFn, d, ptr("/xs"), interpreter.IntVal(9))); got != `{"xs":[1,2,3,9]}` {
		t.Errorf("append = %q", got)
	}
	if got := jstr(t, call(t, removeFn, d, ptr("/xs/0"))); got != `{"xs":[2,3]}` {
		t.Errorf("remove index = %q", got)
	}
	if got := jstr(t, call(t, removeFn, d, ptr("/xs"))); got != `{}` {
		t.Errorf("remove key = %q", got)
	}
}

func TestMoveRenames(t *testing.T) {
	d := doc(t, `{"city":"vienna","n":1}`)
	got := jstr(t, call(t, moveFn, d, ptr("/city"), ptr("/location")))
	if got != `{"n":1,"location":"vienna"}` {
		t.Errorf("move = %q", got)
	}
}

func TestToNodeNormalizes(t *testing.T) {
	d := doc(t, `{}`)
	// a struct normalizes to a map
	p := interpreter.NamespacedStructVal("", "P", []interpreter.StructField{
		{Name: "x", Value: interpreter.IntVal(1)},
		{Name: "y", Value: interpreter.IntVal(2)},
	})
	if got := jstr(t, call(t, setFn, d, ptr("/p"), p)); got != `{"p":{"x":1,"y":2}}` {
		t.Errorf("struct store = %q", got)
	}
	// a list literal stores as an array
	xs := interpreter.ListVal(parser.PrimitiveType(parser.TypeInt),
		[]interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(2)})
	if got := jstr(t, call(t, setFn, d, ptr("/xs"), xs)); got != `{"xs":[1,2]}` {
		t.Errorf("list store = %q", got)
	}
	// a json.Value stores as its tree (unwrapped)
	inner := doc(t, `{"k":true}`)
	if got := jstr(t, call(t, setFn, d, ptr("/j"), inner)); got != `{"j":{"k":true}}` {
		t.Errorf("json.Value store = %q", got)
	}
}

func TestWriteErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
		want string
	}{
		{"set key on null root", setFn, []interpreter.Value{doc(t, `null`), ptr("/a"), interpreter.IntVal(1)}, "cannot set a member of null"},
		{"missing intermediate", setFn, []interpreter.Value{doc(t, `{}`), ptr("/a/b"), interpreter.IntVal(1)}, "no key \"a\""},
		{"set index out of range", setFn, []interpreter.Value{doc(t, `[1,2]`), ptr("/5"), interpreter.IntVal(1)}, "out of range"},
		{"insert index out of range", insertFn, []interpreter.Value{doc(t, `[1]`), ptr("/9"), interpreter.IntVal(1)}, "out of range"},
		{"remove missing key", removeFn, []interpreter.Value{doc(t, `{"a":1}`), ptr("/z")}, "no key"},
		{"remove root", removeFn, []interpreter.Value{doc(t, `{"a":1}`), ptr("")}, "cannot remove the whole document"},
		{"insert into non-list", insertFn, []interpreter.Value{doc(t, `{"a":1}`), ptr("/a"), interpreter.IntVal(1)}, "expected a list"},
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

// TestBuildFromScratchRoundTrips builds a nested document with the write verbs
// and confirms it encodes to the expected JSON.
func TestBuildFromScratchRoundTrips(t *testing.T) {
	v := call(t, mapFn)
	v = call(t, setFn, v, ptr("/name"), interpreter.StringVal("ada"))
	v = call(t, setFn, v, ptr("/tags"), call(t, listFn))
	v = call(t, appendFn, v, ptr("/tags"), interpreter.StringVal("admin"))
	v = call(t, appendFn, v, ptr("/tags"), interpreter.StringVal("dev"))
	if got := jstr(t, v); got != `{"name":"ada","tags":["admin","dev"]}` {
		t.Errorf("built = %q", got)
	}
}
