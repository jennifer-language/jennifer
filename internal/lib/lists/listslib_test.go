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

// ---- M15.0: shuffle + range ----

func TestRangeAscendingHalfOpen(t *testing.T) {
	// Half-open: range(1, 5) yields 1, 2, 3, 4 (4 elements; 5 excluded).
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(5)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []int64{1, 2, 3, 4}
	if len(out.List) != len(want) {
		t.Fatalf("len=%d, want %d (%+v)", len(out.List), len(want), out.List)
	}
	for i, w := range want {
		if out.List[i].Int != w {
			t.Errorf("[%d] = %d, want %d", i, out.List[i].Int, w)
		}
	}
}

func TestRangeCoincidentBoundsEmpty(t *testing.T) {
	// Half-open with start == end is empty.
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(5), interpreter.IntVal(5)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.List) != 0 {
		t.Errorf("got %+v, want []", out.List)
	}
}

func TestRangeIndexAlignment(t *testing.T) {
	// The motivating use case: range(0, n) yields valid 0-based indices
	// into an n-element list.
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(0), interpreter.IntVal(5)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []int64{0, 1, 2, 3, 4}
	if len(out.List) != len(want) {
		t.Fatalf("got %+v, want %v", out.List, want)
	}
	for i, w := range want {
		if out.List[i].Int != w {
			t.Errorf("[%d] = %d, want %d", i, out.List[i].Int, w)
		}
	}
}

func TestRangeCompositionInvariant(t *testing.T) {
	// concat(range(a, b), range(b, c)) == range(a, c) - the composability
	// guarantee that drove the choice of half-open.
	a, b, c := int64(2), int64(7), int64(12)
	left, _ := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(a), interpreter.IntVal(b)})
	right, _ := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(b), interpreter.IntVal(c)})
	whole, _ := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(a), interpreter.IntVal(c)})
	combined := append([]interpreter.Value{}, left.List...)
	combined = append(combined, right.List...)
	if len(combined) != len(whole.List) {
		t.Fatalf("len mismatch: combined=%d whole=%d", len(combined), len(whole.List))
	}
	for i := range combined {
		if combined[i].Int != whole.List[i].Int {
			t.Errorf("[%d]: combined=%d whole=%d", i, combined[i].Int, whole.List[i].Int)
		}
	}
}

func TestRangeStepHalfOpen(t *testing.T) {
	// Half-open stepping: 0, 3, 6 (9 excluded).
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(0), interpreter.IntVal(9), interpreter.IntVal(3)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []int64{0, 3, 6}
	if len(out.List) != len(want) {
		t.Fatalf("len=%d, want %d (%+v)", len(out.List), len(want), out.List)
	}
	for i, w := range want {
		if out.List[i].Int != w {
			t.Errorf("[%d] = %d, want %d", i, out.List[i].Int, w)
		}
	}
}

func TestRangeStepStopsBeforeEnd(t *testing.T) {
	// Same "stops before end" rule regardless of whether the step would
	// have landed: range(1, 9, 3) → 1, 4, 7 (10 > 9 anyway).
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(9), interpreter.IntVal(3)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []int64{1, 4, 7}
	if len(out.List) != len(want) {
		t.Fatalf("len=%d, want %d (%+v)", len(out.List), len(want), out.List)
	}
	for i, w := range want {
		if out.List[i].Int != w {
			t.Errorf("[%d] = %d, want %d", i, out.List[i].Int, w)
		}
	}
}

func TestRangeDescendingHalfOpen(t *testing.T) {
	// Descending half-open: end is excluded too. range(10, 1, -3) emits
	// while current > 1, so 10, 7, 4 (1 is the exclusive end).
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(10), interpreter.IntVal(1), interpreter.IntVal(-3)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []int64{10, 7, 4}
	if len(out.List) != len(want) {
		t.Fatalf("len=%d, want %d (%+v)", len(out.List), len(want), out.List)
	}
	for i, w := range want {
		if out.List[i].Int != w {
			t.Errorf("[%d] = %d, want %d", i, out.List[i].Int, w)
		}
	}
}

func TestRangeDescendingDefaultStep(t *testing.T) {
	out, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(3), interpreter.IntVal(0)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []int64{3, 2, 1}
	if len(out.List) != len(want) {
		t.Fatalf("got %+v, want %v", out.List, want)
	}
}

func TestRangeStepZeroErrors(t *testing.T) {
	_, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(9), interpreter.IntVal(0)})
	if err == nil || !strings.Contains(err.Error(), "non-zero") {
		t.Errorf("err = %v", err)
	}
}

func TestRangeStepDirectionMismatch(t *testing.T) {
	_, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(9), interpreter.IntVal(-1)})
	if err == nil || !strings.Contains(err.Error(), "positive step") {
		t.Errorf("ascending+neg: err = %v", err)
	}
	_, err = rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(9), interpreter.IntVal(1), interpreter.IntVal(1)})
	if err == nil || !strings.Contains(err.Error(), "negative step") {
		t.Errorf("descending+pos: err = %v", err)
	}
}

func TestShuffleIsNonMutating(t *testing.T) {
	a := intList(1, 2, 3, 4, 5)
	b, err := shuffleFn(interpreter.BuiltinCtx{}, []interpreter.Value{a})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for i, want := range []int64{1, 2, 3, 4, 5} {
		if a.List[i].Int != want {
			t.Errorf("input mutated at %d: %d != %d", i, a.List[i].Int, want)
		}
	}
	if len(b.List) != 5 {
		t.Errorf("result len=%d", len(b.List))
	}
	// All five elements preserved (sum invariant).
	sum := int64(0)
	for _, v := range b.List {
		sum += v.Int
	}
	if sum != 15 {
		t.Errorf("sum=%d, want 15", sum)
	}
}

func TestShuffleEmpty(t *testing.T) {
	out, err := shuffleFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList()})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.List) != 0 {
		t.Errorf("expected empty, got %+v", out.List)
	}
}

func TestShuffleSingleElement(t *testing.T) {
	out, err := shuffleFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(42)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.List) != 1 || out.List[0].Int != 42 {
		t.Errorf("got %+v, want [42]", out.List)
	}
}
