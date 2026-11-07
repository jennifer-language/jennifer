// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mapslib

import (
	"fmt"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func strList(keys ...string) interpreter.Value {
	out := make([]interpreter.Value, len(keys))
	for i, k := range keys {
		out[i] = interpreter.StringVal(k)
	}
	return interpreter.Value{Kind: interpreter.KindList, List: out}
}

// TestWithout: removes every listed key, keeps the others in order, leaves
// the source intact; a duplicate key is removed once.
func TestWithout(t *testing.T) {
	m := mapOf(interpreter.StringVal("a"), interpreter.IntVal(1), interpreter.StringVal("b"), interpreter.IntVal(2),
		interpreter.StringVal("c"), interpreter.IntVal(3), interpreter.StringVal("d"), interpreter.IntVal(4))
	out, err := withoutFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, strList("c", "a", "c")})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Display(); got != `{"b": 2, "d": 4}` {
		t.Fatalf("got %s", got)
	}
	if len(m.Map) != 4 {
		t.Fatalf("source changed: %s", m.Display())
	}
	if out.LookupKey(interpreter.StringVal("d")) != 1 || out.LookupKey(interpreter.StringVal("a")) >= 0 {
		t.Fatalf("lookups on result")
	}
	empty, err := withoutFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, strList()})
	if err != nil || len(empty.Map) != 4 {
		t.Fatalf("no keys: %v %v", empty.Display(), err)
	}
	all, err := withoutFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, strList("a", "b", "c", "d")})
	if err != nil || len(all.Map) != 0 {
		t.Fatalf("all keys: %v %v", all.Display(), err)
	}
}

// TestWithoutErrors: a missing key, a non-map or non-list argument and a
// wrong argument count are errors.
func TestWithoutErrors(t *testing.T) {
	m := mapOf(interpreter.StringVal("a"), interpreter.IntVal(1))
	cases := []struct {
		args []interpreter.Value
		msg  string
	}{
		{[]interpreter.Value{m, strList("a", "zz")}, "maps.without: map has no entry for key zz"},
		{[]interpreter.Value{interpreter.IntVal(1), strList("a")}, "maps.without: first argument must be a map"},
		{[]interpreter.Value{m, interpreter.StringVal("a")}, "maps.without: second argument must be a list"},
		{[]interpreter.Value{m}, "maps.without expects 2 arguments"},
	}
	for i, c := range cases {
		if _, err := withoutFn(interpreter.BuiltinCtx{}, c.args); err == nil || !strings.HasPrefix(err.Error(), c.msg) {
			t.Errorf("case %d: got %v, want %q", i, err, c.msg)
		}
	}
}

// TestWithoutSingleCopy: removing 1,000 of 20,000 keys copies the entries
// once; the only per-key allocation is the index lookup's key encoding.
// Repeated delete would allocate ~20,000 entry slices.
func TestWithoutSingleCopy(t *testing.T) {
	m := bigMap(20000)
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i*20)
	}
	kl := strList(keys...)
	var out interpreter.Value
	allocs := testing.AllocsPerRun(5, func() {
		var err error
		if out, err = withoutFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, kl}); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > float64(len(keys)+10) {
		t.Fatalf("without on 20000 entries: %.0f allocations", allocs)
	}
	if len(out.Map) != 19000 {
		t.Fatalf("result has %d entries", len(out.Map))
	}
}
