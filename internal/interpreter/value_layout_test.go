// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"math"
	"sync"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// The scalar word is shared by int, float and bool; each accessor answers
// only for its own kind, so a value never reads another kind's bits.
func TestScalarAccessorsAreKindGated(t *testing.T) {
	f := FloatVal(1.5)
	if f.Float() != 1.5 || f.Int() != 0 || f.Bool() {
		t.Errorf("float 1.5: Float %v Int %v Bool %v", f.Float(), f.Int(), f.Bool())
	}
	n := IntVal(-7)
	if n.Int() != -7 || n.Float() != 0 || n.Bool() {
		t.Errorf("int -7: Int %v Float %v Bool %v", n.Int(), n.Float(), n.Bool())
	}
	b := BoolVal(true)
	if !b.Bool() || b.Int() != 0 || b.Float() != 0 {
		t.Errorf("bool true: Bool %v Int %v Float %v", b.Bool(), b.Int(), b.Float())
	}
	if v := FloatVal(math.Inf(-1)); v.Float() != math.Inf(-1) {
		t.Errorf("-Inf round trip: %v", v.Float())
	}
	var w Value
	w.Kind = KindInt
	w.SetInt(math.MinInt64)
	if w.Int() != math.MinInt64 {
		t.Errorf("SetInt(MinInt64) = %d", w.Int())
	}
}

// The slice header is shared by every slice payload; the tag, not Kind,
// decides which one it holds, so a rewritten Kind never reinterprets memory.
func TestSlicePayloadIsTagGated(t *testing.T) {
	l := GenericListVal([]Value{IntVal(1), IntVal(2)})
	if len(l.List()) != 2 || l.Str() != "" || l.Bytes() != nil || l.Map() != nil || l.Fields() != nil {
		t.Fatalf("list exposes another payload")
	}
	l.Kind = KindBytes // a forged kind
	if l.Bytes() != nil {
		t.Fatalf("a list's backing read as bytes")
	}
	s := StringVal("héllo")
	if s.Str() != "héllo" || s.RuneLen() != 5 || s.List() != nil {
		t.Fatalf("string payload: %q %d", s.Str(), s.RuneLen())
	}
	if e := StringVal(""); e.Str() != "" || e.RuneLen() != 0 {
		t.Fatalf("empty string payload")
	}
	if GenericListVal(nil).List() != nil {
		t.Errorf("nil list reads back non-nil")
	}
	if got := GenericListVal([]Value{}).List(); got == nil || len(got) != 0 {
		t.Errorf("empty list reads back %v", got)
	}
}

// List shares the backing (element writes land in the value) and keeps the
// capacity, so append-and-store stays amortised O(1).
func TestListAccessorSharesBackingAndCapacity(t *testing.T) {
	v := GenericListVal(make([]Value, 1, 8))
	v.List()[0] = IntVal(9)
	if v.List()[0].Int() != 9 {
		t.Fatalf("element write did not land")
	}
	if cap(v.List()) != 8 {
		t.Fatalf("cap = %d, want 8", cap(v.List()))
	}
	before := &v.List()[0]
	v.SetList(append(v.List(), IntVal(10)))
	if &v.List()[0] != before || len(v.List()) != 2 {
		t.Fatalf("append within capacity moved the backing")
	}
}

// Equal identities intern to one pointer, so identity comparison is a pointer
// comparison and a variant is part of it.
func TestIdentityInterning(t *testing.T) {
	a := InternIdentity("m", "/x/m.j", "Point", "")
	b := InternIdentity("m", "/x/m.j", "Point", "")
	if a != b {
		t.Fatalf("equal identities not shared")
	}
	if a == InternIdentity("m", "/y/m.j", "Point", "") {
		t.Fatalf("same stem, different module path shares an identity")
	}
	if InternIdentity("", "", "", "") != nil {
		t.Fatalf("all-empty identity is not nil")
	}
	s := ModuleStructVal("m", "/x/m.j", "Point", nil)
	if s.Identity() != a || s.StructNS() != "m" || s.ModPath() != "/x/m.j" || s.StructName() != "Point" {
		t.Fatalf("struct identity: %+v", *s.Identity())
	}
	red := EnumVal("", "", "Color", "Red", nil)
	if red.Equal(EnumVal("", "", "Color", "Green", nil)) || !red.Equal(EnumVal("", "", "Color", "Red", nil)) {
		t.Fatalf("enum equality by variant")
	}
	o := ObjectVal("json", "Value", Null())
	if o.StructNS() != "json" || o.StructName() != "Value" || o.Obj() == nil {
		t.Fatalf("object identity")
	}
}

// Interning from many goroutines at once hands every caller the same pointer.
func TestIdentityInterningConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	got := make([]*Identity, 16)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = InternIdentity("c", "", "Concurrent", "")
		}()
	}
	wg.Wait()
	for _, p := range got {
		if p != got[0] {
			t.Fatalf("concurrent interning returned two pointers")
		}
	}
}

// A map's types and key index live in one descriptor; replacing either never
// writes the descriptor a copy still shares.
func TestMapDescriptorIsNotWrittenInPlace(t *testing.T) {
	kt, vt := parser.Type{Kind: parser.TypeString}, parser.Type{Kind: parser.TypeInt}
	m := MapVal(kt, vt, nil)
	m.UpsertKey(StringVal("a"), IntVal(1))
	if !m.mapIndexUsable() || m.LookupKey(StringVal("a")) != 0 {
		t.Fatalf("upsert did not index")
	}
	shared := m // a by-value copy shares the descriptor
	m.DropMapIndex()
	if m.mapIndexUsable() || !shared.mapIndexUsable() {
		t.Fatalf("dropping the index reached the copy")
	}
	if m.KeyTyp() == nil || m.KeyTyp().Kind != parser.TypeString || m.ValTyp().Kind != parser.TypeInt {
		t.Fatalf("dropping the index lost the types")
	}
	other := parser.Type{Kind: parser.TypeFloat}
	m.SetMapTypes(m.KeyTyp(), &other)
	if shared.ValTyp().Kind != parser.TypeInt {
		t.Fatalf("retyping reached the copy")
	}
	c := shared.DeepCopy()
	if c.mapDesc() == shared.mapDesc() || !c.mapIndexUsable() || c.ValTyp() != shared.ValTyp() {
		t.Fatalf("deep copy: fresh descriptor and index, shared types")
	}
}

// Func values keep their method and home interpreter out of line.
func TestFuncValueHandle(t *testing.T) {
	z := FuncVal(nil)
	if z.Fn() != nil || z.FnHome() != nil || !z.Equal(Null()) {
		t.Fatalf("zero func value")
	}
	m := &parser.MethodDef{Name: "f"}
	in := &Interpreter{}
	f := FuncValIn(m, in)
	if f.Fn() != m || f.FnHome() != in || !f.Equal(FuncVal(m)) || f.Display() != "<func f>" {
		t.Fatalf("func value handle")
	}
}
