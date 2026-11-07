// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mapslib

import (
	"fmt"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func bigMap(n int) interpreter.Value {
	pairs := make([]interpreter.Value, 0, 2*n)
	for i := 0; i < n; i++ {
		pairs = append(pairs, interpreter.StringVal(fmt.Sprintf("k%d", i)), interpreter.IntVal(int64(i)))
	}
	m := mapOf(pairs...)
	m.BuildMapIndex()
	return m
}

// TestDeleteSingleCopy: delete copies the entries once. Before the fix it
// deep-copied the map, built a hash index (one allocation per key), then
// reallocated the entry slice: ~20,000 allocations for 20,000 entries.
func TestDeleteSingleCopy(t *testing.T) {
	m := bigMap(20000)
	key := interpreter.StringVal("k100")
	allocs := testing.AllocsPerRun(5, func() {
		if _, err := deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, key}); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 10 {
		t.Fatalf("delete on 20000 entries: %.0f allocations", allocs)
	}
}

// TestDeleteResultLookups: the result answers lookups for every remaining
// key, before and after a later insertion, and does not share nested values
// with its source.
func TestDeleteResultLookups(t *testing.T) {
	m := bigMap(100)
	m.Map[5].Value = interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{interpreter.IntVal(1)}}
	out, err := deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, interpreter.StringVal("k3")})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Map) != 99 || out.LookupKey(interpreter.StringVal("k3")) >= 0 {
		t.Fatalf("k3 still present or wrong size %d", len(out.Map))
	}
	for i := 0; i < 100; i++ {
		if i == 3 {
			continue
		}
		j := out.LookupKey(interpreter.StringVal(fmt.Sprintf("k%d", i)))
		if j < 0 || out.Map[j].Value.Kind == interpreter.KindInt && out.Map[j].Value.Int != int64(i) {
			t.Fatalf("k%d: index %d", i, j)
		}
	}
	out.UpsertKey(interpreter.StringVal("new"), interpreter.IntVal(7))
	if j := out.LookupKey(interpreter.StringVal("new")); j != 99 {
		t.Fatalf("inserted key at %d", j)
	}
	if j := out.LookupKey(interpreter.StringVal("k99")); j != 98 || out.Map[j].Value.Int != 99 {
		t.Fatalf("k99 after insert: %d", j)
	}
	out.Map[4].Value.List[0] = interpreter.IntVal(2) // k5 shifted to position 4
	if m.Map[5].Value.List[0].Int != 1 {
		t.Fatal("result shares a nested list with its source")
	}
	if len(m.Map) != 100 {
		t.Fatal("source modified")
	}
}
