// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func mkMap(pairs ...[2]interpreter.Value) interpreter.Value {
	entries := make([]interpreter.MapEntry, len(pairs))
	for i, p := range pairs {
		entries[i] = interpreter.MapEntry{Key: p[0], Value: p[1]}
	}
	return interpreter.MapVal(parser.PrimitiveType(parser.TypeString), parser.PrimitiveType(parser.TypeInt), entries)
}

func kv(k string, v int64) [2]interpreter.Value {
	return [2]interpreter.Value{interpreter.StringVal(k), interpreter.IntVal(v)}
}

// Map equality must not be fooled by duplicate keys: a one-way containment
// check plus a length test reports {a,a} == {a,b}. Normal decode/literal
// paths reject duplicates, but Equal itself has to stay correct for any
// value a library might construct.
func TestMapEqualDuplicateKeys(t *testing.T) {
	cases := []struct {
		name string
		a, b interpreter.Value
		want bool
	}{
		{"same order", mkMap(kv("a", 1), kv("b", 2)), mkMap(kv("a", 1), kv("b", 2)), true},
		{"insertion order irrelevant", mkMap(kv("a", 1), kv("b", 2)), mkMap(kv("b", 2), kv("a", 1)), true},
		{"value differs", mkMap(kv("a", 1)), mkMap(kv("a", 2)), false},
		{"dup vs distinct", mkMap(kv("a", 1), kv("a", 1)), mkMap(kv("a", 1), kv("b", 2)), false},
		{"distinct vs dup", mkMap(kv("a", 1), kv("b", 2)), mkMap(kv("a", 1), kv("a", 1)), false},
		{"dup multiset equal", mkMap(kv("a", 1), kv("a", 2)), mkMap(kv("a", 2), kv("a", 1)), true},
		{"dup multiset differs", mkMap(kv("a", 1), kv("a", 1)), mkMap(kv("a", 1), kv("a", 2)), false},
	}
	for _, c := range cases {
		if got := c.a.Equal(c.b); got != c.want {
			t.Errorf("%s: Equal = %t, want %t", c.name, got, c.want)
		}
		if got := c.b.Equal(c.a); got != c.want {
			t.Errorf("%s (reversed): Equal = %t, want %t", c.name, got, c.want)
		}
	}
}
