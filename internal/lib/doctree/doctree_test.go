// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package doctree

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// testLib is a library with every optional feature on.
var testLib = &Lib{
	Name:    "t",
	HasNull: true,
	TimeNode: func(v interpreter.Value) (Node, bool) {
		text, ok := TimeRFC3339(v)
		return Datetime(text, 7), ok
	},
}

// A large map keeps its sorted lookup index in step with its members through
// every write: keys added (in random order), replaced and removed must all stay
// findable, and removed ones must not.
func TestMapIndexMaintainedAcrossWrites(t *testing.T) {
	m := Map(nil, nil)
	want := map[string]int64{}
	r := rand.New(rand.NewSource(1))
	for _, i := range r.Perm(200) {
		k := "k" + strconv.Itoa(i)
		var err error
		if m, err = Set("set", m, []string{k}, Int(int64(i))); err != nil {
			t.Fatal(err)
		}
		want[k] = int64(i)
	}
	for _, i := range r.Perm(200)[:80] {
		k := "k" + strconv.Itoa(i)
		var err error
		if i%2 == 0 {
			m, err = Remove("remove", m, []string{k})
			delete(want, k)
		} else {
			m, err = Set("set", m, []string{k}, Int(-1))
			want[k] = -1
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if m.Len() != len(want) {
		t.Fatalf("length %d, want %d", m.Len(), len(want))
	}
	if m.c.order == nil {
		t.Error("a large map should carry a sorted index")
	}
	for i := 0; i < 200; i++ {
		k := "k" + strconv.Itoa(i)
		pos := m.Find(k)
		w, present := want[k]
		switch {
		case present && (pos < 0 || m.Elem(pos).IntVal() != w):
			t.Errorf("%s: pos %d, want value %d", k, pos, w)
		case !present && pos >= 0:
			t.Errorf("%s: removed key still found", k)
		}
	}
}

// Writes never touch the input tree: the original handle reads as before.
func TestWritesShareButNeverMutate(t *testing.T) {
	inner := List([]Node{Int(1), Int(2)})
	doc := Map([]string{"xs", "s"}, []Node{inner, String("a")})
	if _, err := Set("set", doc, []string{"xs", "0"}, Int(9)); err != nil {
		t.Fatal(err)
	}
	if _, err := Insert("insert", doc, []string{"xs", "-"}, Int(3)); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove("remove", doc, []string{"s"}); err != nil {
		t.Fatal(err)
	}
	if doc.Len() != 2 || doc.Elem(0).Len() != 2 || doc.Elem(0).Elem(0).IntVal() != 1 {
		t.Error("an edit changed the source tree")
	}
}

// The Builder grows maps past the index threshold with O(1) lookups, keeps
// handles valid while their containers grow, and Seal yields an indexed tree.
func TestBuilder(t *testing.T) {
	var b Builder
	root := b.NewMap()
	sub := b.NewMap()
	b.Add(root, "sub", sub)
	arr := b.NewList()
	b.Add(root, "arr", arr)
	for i := 0; i < 50; i++ {
		k := "k" + strconv.Itoa(i)
		if b.Lookup(sub, k) >= 0 {
			t.Fatalf("%s found before it was added", k)
		}
		b.Add(sub, k, Int(int64(i)))
		b.Push(arr, String(k))
	}
	if b.Lookup(sub, "k37") != 37 {
		t.Errorf("lookup k37 = %d", b.Lookup(sub, "k37"))
	}
	root = b.Seal(root)
	got, err := Resolve("get", root, "/sub/k49")
	if err != nil || got.IntVal() != 49 {
		t.Errorf("/sub/k49 = %v, %v", got.IntVal(), err)
	}
	if n, _ := Resolve("get", root, "/arr"); n.Len() != 50 || cap(n.c.elems) != 50 {
		t.Errorf("sealed list len %d cap %d, want 50 / 50", n.Len(), cap(n.c.elems))
	}
	if s, _ := Resolve("get", root, "/sub"); s.c.order == nil {
		t.Error("a sealed large map should carry a sorted index")
	}
}

func TestEquality(t *testing.T) {
	cases := []struct {
		a, b Node
		want bool
	}{
		{Map([]string{"a", "b"}, []Node{Int(1), Null()}), Map([]string{"b", "a"}, []Node{Null(), Int(1)}), true},
		{List([]Node{Int(1), Int(2)}), List([]Node{Int(2), Int(1)}), false},
		{Int(1), Float(1), true},
		{Int(1), Float(1.5), false},
		{String("x"), Bytes([]byte("x")), false},
		{Bytes([]byte("x")), Bytes([]byte("x")), true},
		{Datetime("1979-05-27", 2), Datetime("1979-05-27", 2), true},
		{Datetime("1979-05-27", 2), Datetime("1979-05-27", 3), false},
		{Datetime("1979-05-27", 2), String("1979-05-27"), false},
	}
	for i, c := range cases {
		if got := c.a.Equal(c.b); got != c.want {
			t.Errorf("case %d (%s vs %s): %t, want %t", i, c.a.TypeName(), c.b.TypeName(), got, c.want)
		}
	}
}

func TestExceedsDepth(t *testing.T) {
	n := Int(1)
	for i := 0; i < 10; i++ {
		n = List([]Node{n})
	}
	if n.ExceedsDepth(10) {
		t.Error("10 levels within a 10 budget")
	}
	if !n.ExceedsDepth(9) {
		t.Error("10 levels over a 9 budget")
	}
}

func TestToNode(t *testing.T) {
	tm := interpreter.NamespacedStructVal("time", "Time", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(0)},
		{Name: "offset", Value: interpreter.IntVal(3600)},
	})
	n, err := testLib.ToNode(tm, false)
	if err != nil || n.Kind() != KindDatetime || n.Str() != "1970-01-01T01:00:00+01:00" || n.Form() != 7 {
		t.Errorf("time.Time -> %s %q form %d (%v)", n.TypeName(), n.Str(), n.Form(), err)
	}
	plain := &Lib{Name: "p"}
	if n, _ := plain.ToNode(tm, false); n.Kind() != KindMap || n.Key(0) != "nanos" {
		t.Errorf("without TimeNode a time.Time is a map, got %s", n.TypeName())
	}
	b := interpreter.BytesVal([]byte("hi"))
	if n, _ := testLib.ToNode(b, false); n.Kind() != KindString || n.Str() != "aGk=" {
		t.Errorf("bytes -> %s %q, want base64 string", n.TypeName(), n.Str())
	}
	if n, _ := testLib.ToNode(b, true); n.Kind() != KindBytes || n.Str() != "hi" {
		t.Errorf("bytes keepBytes -> %s %q", n.TypeName(), n.Str())
	}
	m := interpreter.MapVal(parser.PrimitiveType(parser.TypeInt), parser.PrimitiveType(parser.TypeInt),
		[]interpreter.MapEntry{{Key: interpreter.IntVal(1), Value: interpreter.IntVal(2)}})
	if _, err := testLib.ToNode(m, false); err == nil || !strings.Contains(err.Error(), "key must be string") {
		t.Errorf("int-keyed map: %v", err)
	}
	other := (&Lib{Name: "o"}).Wrap(Int(1))
	if _, err := testLib.ToNode(other, false); err == nil || !strings.Contains(err.Error(), "opaque o.Value") {
		t.Errorf("another library's value: %v", err)
	}
	if n, _ := testLib.ToNode(testLib.Wrap(String("s")), false); n.Str() != "s" {
		t.Error("own value should unwrap")
	}
}

// Each library registers only the optional accessors it has.
func TestOptionalSurface(t *testing.T) {
	plain := &Lib{Name: "p"}
	for _, name := range []string{"isNull", "isDatetime", "asDatetime"} {
		if plain.Builtin(name) != nil {
			t.Errorf("a library without the feature registered %s", name)
		}
		if testLib.Builtin(name) == nil {
			t.Errorf("a library with the feature lacks %s", name)
		}
	}
	got, err := testLib.Builtin("typeOf")(interpreter.BuiltinCtx{}, []interpreter.Value{testLib.Wrap(Datetime("x", 0))})
	if err != nil || got.Str != "datetime" {
		t.Errorf("typeOf datetime = %q, %v", got.Str, err)
	}
	if _, err := testLib.Builtin("get")(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil || !strings.Contains(err.Error(), "t.get: argument must be a t.Value") {
		t.Errorf("wrong argument: %v", err)
	}
}

// The zero value is null, and equals a null from the library but not another
// library's null.
func TestZeroAndCrossLibraryEquality(t *testing.T) {
	if !testLib.Zero().Equal(testLib.Wrap(Null())) {
		t.Error("zero should equal null")
	}
	other := &Lib{Name: "t"} // same namespace, different library
	if testLib.Wrap(Null()).Obj == nil || (&payload{root: Null(), lib: testLib}).EqualNative(&payload{root: Null(), lib: other}) {
		t.Error("payloads of different libraries should not be equal")
	}
}

// asFloat refuses a non-finite float (a TOML `inf`, a YAML `.nan`): Jennifer
// has no such value. typeOf still reports it.
func TestAsFloatRefusesNonFinite(t *testing.T) {
	for _, f := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		v := testLib.Wrap(Float(f))
		if _, err := testLib.Builtin("asFloat")(interpreter.BuiltinCtx{}, []interpreter.Value{v}); err == nil || !strings.Contains(err.Error(), "non-finite") {
			t.Errorf("asFloat(%v): %v", f, err)
		}
		if got, _ := testLib.Builtin("typeOf")(interpreter.BuiltinCtx{}, []interpreter.Value{v}); got.Str != "float" {
			t.Errorf("typeOf(%v) = %q", f, got.Str)
		}
	}
}

// Height is carried, so a depth check stays O(1) on a tree whose subtrees are
// shared many times (each step below doubles the expanded size).
func TestHeightOnSharedSubtrees(t *testing.T) {
	d := List(nil)
	for i := 0; i < 200; i++ {
		d = List([]Node{d, d})
	}
	if d.Height() != 201 || !d.ExceedsDepth(200) || d.ExceedsDepth(201) {
		t.Errorf("height = %d", d.Height())
	}
	var b Builder
	root := b.NewMap()
	inner := b.NewList()
	b.Add(root, "x", inner)
	b.Push(inner, List([]Node{Int(1)}))
	if root = b.Seal(root); root.Height() != 3 {
		t.Errorf("sealed height = %d, want 3", root.Height())
	}
}
