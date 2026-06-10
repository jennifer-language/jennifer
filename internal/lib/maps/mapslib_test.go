// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package mapslib

import (
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

func mapOf(pairs ...interpreter.Value) interpreter.Value {
	if len(pairs)%2 != 0 {
		panic("mapOf needs pairs")
	}
	entries := make([]interpreter.MapEntry, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		entries = append(entries, interpreter.MapEntry{Key: pairs[i], Value: pairs[i+1]})
	}
	return interpreter.Value{Kind: interpreter.KindMap, Map: entries}
}

func TestKeysAndValuesPreserveInsertionOrder(t *testing.T) {
	m := mapOf(
		interpreter.StringVal("b"), interpreter.IntVal(2),
		interpreter.StringVal("a"), interpreter.IntVal(1),
		interpreter.StringVal("c"), interpreter.IntVal(3),
	)
	keys, _ := keysFn(interpreter.BuiltinCtx{}, []interpreter.Value{m})
	if got := []string{keys.List[0].Str, keys.List[1].Str, keys.List[2].Str}; !(got[0] == "b" && got[1] == "a" && got[2] == "c") {
		t.Errorf("keys out of order: %v", got)
	}
	vals, _ := valuesFn(interpreter.BuiltinCtx{}, []interpreter.Value{m})
	if got := []int64{vals.List[0].Int, vals.List[1].Int, vals.List[2].Int}; !(got[0] == 2 && got[1] == 1 && got[2] == 3) {
		t.Errorf("values out of order: %v", got)
	}
}

func TestDeleteIsNonMutating(t *testing.T) {
	m := mapOf(interpreter.StringVal("x"), interpreter.IntVal(1))
	_, _ = deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, interpreter.StringVal("x")})
	if len(m.Map) != 1 {
		t.Errorf("input mutated, len=%d", len(m.Map))
	}
}

func TestDeleteMissingErrors(t *testing.T) {
	m := mapOf(interpreter.StringVal("x"), interpreter.IntVal(1))
	_, err := deleteFn(interpreter.BuiltinCtx{}, []interpreter.Value{m, interpreter.StringVal("y")})
	if err == nil || !strings.Contains(err.Error(), "no entry for key") {
		t.Errorf("err = %v", err)
	}
}

func TestMergeOverlaysBOverA(t *testing.T) {
	a := mapOf(
		interpreter.StringVal("x"), interpreter.IntVal(1),
		interpreter.StringVal("y"), interpreter.IntVal(2),
	)
	b := mapOf(
		interpreter.StringVal("y"), interpreter.IntVal(99),
		interpreter.StringVal("z"), interpreter.IntVal(3),
	)
	out, err := mergeFn(interpreter.BuiltinCtx{}, []interpreter.Value{a, b})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.Map) != 3 {
		t.Fatalf("len=%d", len(out.Map))
	}
	if out.Map[0].Value.Int != 1 || out.Map[1].Value.Int != 99 || out.Map[2].Value.Int != 3 {
		t.Errorf("got %+v", out.Map)
	}
}
