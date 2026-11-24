// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func numbersCall(fn func([]interpreter.Value) (interpreter.Value, error), args ...interpreter.Value) (interpreter.Value, error) {
	return fn(args)
}

// The bulk readers live in doctree's shared surface (json, toml and yaml all
// have them); these bind json's.
func asFloatsFn(args []interpreter.Value) (interpreter.Value, error) {
	return lib.Builtin("asFloats")(interpreter.BuiltinCtx{}, args)
}

func asIntsFn(args []interpreter.Value) (interpreter.Value, error) {
	return lib.Builtin("asInts")(interpreter.BuiltinCtx{}, args)
}

func asStringsFn(args []interpreter.Value) (interpreter.Value, error) {
	return lib.Builtin("asStrings")(interpreter.BuiltinCtx{}, args)
}

func TestAsStringsReadsList(t *testing.T) {
	d := doc(t, `{"tags": ["a", "é", ""], "e": []}`)
	got, err := numbersCall(asStringsFn, d, ptr("/tags"))
	if err != nil || len(got.List()) != 3 || got.List()[1].Str() != "é" || got.ElemTyp().Kind != parser.TypeString {
		t.Fatalf("asStrings = %v, %v", got, err)
	}
	if e, err := numbersCall(asStringsFn, d, ptr("/e")); err != nil || len(e.List()) != 0 {
		t.Errorf("empty list: %v, %v", e, err)
	}
	for _, bad := range []string{`["a", 1]`, `[null]`, `{"a": "b"}`, `"s"`} {
		if _, err := numbersCall(asStringsFn, doc(t, bad)); err == nil {
			t.Errorf("asStrings(%s) should fail", bad)
		}
	}
}

func TestAsFloatsReadsList(t *testing.T) {
	d := doc(t, `{"emb":{"v":[1, -2.5, 3e2, 0]},"e":[]}`)
	got, err := numbersCall(asFloatsFn, d, ptr("/emb/v"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != interpreter.KindList || got.ElemTyp().Kind != parser.TypeFloat {
		t.Fatalf("asFloats returned %s, want a list of float", got.Kind)
	}
	want := []float64{1, -2.5, 300, 0}
	if len(got.List()) != len(want) {
		t.Fatalf("len = %d, want %d", len(got.List()), len(want))
	}
	for i, w := range want {
		if got.List()[i].Kind != interpreter.KindFloat || got.List()[i].Float() != w {
			t.Errorf("[%d] = %v, want float %v", i, got.List()[i], w)
		}
	}
	// an empty list, and the node itself with no pointer
	if e, err := numbersCall(asFloatsFn, d, ptr("/e")); err != nil || len(e.List()) != 0 || e.ElemTyp().Kind != parser.TypeFloat {
		t.Errorf("empty asFloats = %v, %v", e, err)
	}
	if e, err := numbersCall(asFloatsFn, doc(t, `[7]`)); err != nil || len(e.List()) != 1 || e.List()[0].Float() != 7 {
		t.Errorf("asFloats without pointer = %v, %v", e, err)
	}
}

func TestAsIntsReadsList(t *testing.T) {
	d := doc(t, `{"a":[[0, 1, -9223372036854775808, 42]],"e":[]}`)
	got, err := numbersCall(asIntsFn, d, ptr("/a/0"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != interpreter.KindList || got.ElemTyp().Kind != parser.TypeInt {
		t.Fatalf("asInts returned %s, want a list of int", got.Kind)
	}
	want := []int64{0, 1, -9223372036854775808, 42}
	for i, w := range want {
		if got.List()[i].Kind != interpreter.KindInt || got.List()[i].Int() != w {
			t.Errorf("[%d] = %v, want int %d", i, got.List()[i], w)
		}
	}
	if e, err := numbersCall(asIntsFn, d, ptr("/e")); err != nil || len(e.List()) != 0 || e.ElemTyp().Kind != parser.TypeInt {
		t.Errorf("empty asInts = %v, %v", e, err)
	}
}

// TestBulkNumberErrors: a mixed list, a non-list node, a bad pointer and an
// integral float for asInts each fail as the single accessors do - a plain
// error (kind "runtime"), not a "limit" or other classified kind.
func TestBulkNumberErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   func([]interpreter.Value) (interpreter.Value, error)
		args []interpreter.Value
	}{
		{"asFloats string element", asFloatsFn, []interpreter.Value{doc(t, `[1, 2, "3"]`)}},
		{"asFloats null element", asFloatsFn, []interpreter.Value{doc(t, `[null]`)}},
		{"asFloats nested list", asFloatsFn, []interpreter.Value{doc(t, `[[1]]`)}},
		{"asFloats on map", asFloatsFn, []interpreter.Value{doc(t, `{"a":1}`)}},
		{"asFloats on scalar", asFloatsFn, []interpreter.Value{doc(t, `1.5`)}},
		{"asFloats missing key", asFloatsFn, []interpreter.Value{doc(t, `{"a":[1]}`), ptr("/b")}},
		{"asInts float element", asIntsFn, []interpreter.Value{doc(t, `[1, 2.5]`)}},
		{"asInts integral float", asIntsFn, []interpreter.Value{doc(t, `[1.0]`)}},
		{"asInts exponent", asIntsFn, []interpreter.Value{doc(t, `[1e2]`)}},
		{"asInts bool element", asIntsFn, []interpreter.Value{doc(t, `[true]`)}},
		{"asInts bad pointer", asIntsFn, []interpreter.Value{doc(t, `[1]`), ptr("0")}},
		{"asInts not a json.Value", asIntsFn, []interpreter.Value{interpreter.IntVal(1)}},
		{"asInts too many args", asIntsFn, []interpreter.Value{doc(t, `[1]`), ptr(""), ptr("")}},
	}
	for _, c := range cases {
		_, err := c.fn(c.args)
		if err == nil {
			t.Errorf("%s: expected an error", c.name)
			continue
		}
		var kinded interface{ ErrorKind() string }
		if errors.As(err, &kinded) {
			t.Errorf("%s: error has kind %q, want a plain (runtime) error like asFloat / asInt", c.name, kinded.ErrorKind())
		}
	}
}

// bigFloatDoc is a json list of n floats.
func bigFloatDoc(n int) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%d.%03d", i%997, i%1000)
	}
	sb.WriteByte(']')
	return sb.String()
}

// TestAsFloatsMatchesLoop: a 1M-element list read in one call equals the
// per-element asFloat loop it replaces.
func TestAsFloatsMatchesLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("1M-element document")
	}
	const n = 1_000_000
	d := doc(t, bigFloatDoc(n))
	got, err := numbersCall(asFloatsFn, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List()) != n {
		t.Fatalf("len = %d, want %d", len(got.List()), n)
	}
	for i := 0; i < n; i += 9973 {
		one := call(t, asFloatFn, d, ptr(fmt.Sprintf("/%d", i)))
		if got.List()[i].Float() != one.Float() {
			t.Fatalf("[%d] = %v, asFloat = %v", i, got.List()[i].Float(), one.Float())
		}
	}
}

// The M29.8 measurement: 3,072-element embeddings, one call against a loop of
// asFloat with a formatted pointer each.
func BenchmarkAsFloatsEmbedding(b *testing.B) {
	d := wrap(mustDecode(b, bigFloatDoc(3072)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := asFloatsFn([]interpreter.Value{d}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAsFloatLoopEmbedding(b *testing.B) {
	d := wrap(mustDecode(b, bigFloatDoc(3072)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out := make([]interpreter.Value, 0, 3072)
		for j := 0; j < 3072; j++ {
			v, err := asFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{d, interpreter.StringVal("/" + fmt.Sprint(j))})
			if err != nil {
				b.Fatal(err)
			}
			out = append(out, v)
		}
		_ = out
	}
}

func mustDecode(b *testing.B, s string) node {
	b.Helper()
	n, err := decodeFn([]interpreter.Value{interpreter.StringVal(s)})
	if err != nil {
		b.Fatal(err)
	}
	return n
}
