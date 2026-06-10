// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package listslib

import (
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

func intList(vs ...int64) interpreter.Value {
	out := make([]interpreter.Value, len(vs))
	for i, v := range vs {
		out[i] = interpreter.IntVal(v)
	}
	return interpreter.Value{Kind: interpreter.KindList, List: out}
}

func TestPushIsNonMutating(t *testing.T) {
	a := intList(1, 2, 3)
	b, err := pushFn(interpreter.BuiltinCtx{}, []interpreter.Value{a, interpreter.IntVal(4)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(a.List) != 3 {
		t.Errorf("input was mutated: len=%d", len(a.List))
	}
	if len(b.List) != 4 || b.List[3].Int != 4 {
		t.Errorf("bad result: %+v", b.List)
	}
}

func TestPopEmptyErrors(t *testing.T) {
	_, err := popFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList()})
	if err == nil || !strings.Contains(err.Error(), "empty list") {
		t.Errorf("err = %v", err)
	}
}

func TestSortMixedKindRejected(t *testing.T) {
	mixed := interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
		interpreter.IntVal(1), interpreter.StringVal("a"),
	}}
	_, err := sortFn(interpreter.BuiltinCtx{}, []interpreter.Value{mixed})
	if err == nil || !strings.Contains(err.Error(), "mixed-kind") {
		t.Errorf("err = %v", err)
	}
}

func TestSortPromotesIntFloat(t *testing.T) {
	mixed := interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
		interpreter.IntVal(3), interpreter.FloatVal(1.5), interpreter.IntVal(2),
	}}
	out, err := sortFn(interpreter.BuiltinCtx{}, []interpreter.Value{mixed})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.List) != 3 {
		t.Fatalf("bad len: %d", len(out.List))
	}
	if !(out.List[0].Kind == interpreter.KindFloat && out.List[0].Float == 1.5) {
		t.Errorf("first = %+v", out.List[0])
	}
}

func TestContainsHaystackNeedleOrder(t *testing.T) {
	xs := intList(10, 20, 30)
	got, _ := containsFn(interpreter.BuiltinCtx{}, []interpreter.Value{xs, interpreter.IntVal(20)})
	if !got.Bool {
		t.Errorf("expected true")
	}
	got, _ = containsFn(interpreter.BuiltinCtx{}, []interpreter.Value{xs, interpreter.IntVal(99)})
	if got.Bool {
		t.Errorf("expected false")
	}
}

func TestSliceBoundsErrors(t *testing.T) {
	xs := intList(1, 2, 3)
	for _, c := range []struct {
		name       string
		start, end int64
	}{
		{"negative start", -1, 2},
		{"start past end", 4, 4},
		{"end before start", 2, 1},
		{"end past total", 0, 99},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := sliceFn(interpreter.BuiltinCtx{}, []interpreter.Value{
				xs, interpreter.IntVal(c.start), interpreter.IntVal(c.end),
			})
			if err == nil {
				t.Errorf("expected error")
			}
		})
	}
}
