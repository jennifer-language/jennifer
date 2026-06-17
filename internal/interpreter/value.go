// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"strconv"
	"strings"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// DisplayFloat formats a float64 for human output. Unlike `strconv.FormatFloat`
// with verb 'g', it guarantees the result is recognisable as a float: if the
// shortest round-trip representation has no `.`, `e`, or `E`, we append `.0`.
// So `5.0` prints as "5.0", not "5" - the value's *type* stays visible even
// when its value happens to be a whole number.
func DisplayFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// ValueKind tags a runtime value's type.
type ValueKind int

const (
	KindNull ValueKind = iota
	KindInt
	KindFloat
	KindString
	KindBool
	KindBytes  // M12: mutable byte sequence; elements are int in [0, 255]
	KindList   // M6: ordered, mutable sequence (Go slice underneath)
	KindMap    // M6: ordered key-value map; iteration is insertion order
	KindStruct // M13.1: user-defined record; fields are an ordered list of (name, value)
	KindTask   // M16.0: a pending or completed `spawn { ... }` computation
)

func (k ValueKind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindBytes:
		return "bytes"
	case KindList:
		return "list"
	case KindMap:
		return "map"
	case KindStruct:
		return "struct"
	case KindTask:
		return "task"
	}
	return "?"
}

// MapEntry is one entry in a Value of kind KindMap. Maps preserve
// insertion order (so iteration is deterministic), so they're stored as a
// parallel slice rather than a Go map[string]Value or similar.
type MapEntry struct {
	Key, Value Value
}

// StructField is one field in a Value of kind KindStruct (M13.1).
// Fields are kept in declaration order (matching the StructDef's field
// ordering) so display output and assertions are deterministic.
type StructField struct {
	Name  string
	Value Value
}

// TaskState is the payload of a Value of kind KindTask (M16.0). It holds
// the eventual result of a `spawn { ... }` block plus bookkeeping for the
// exit-time unwaited-error scan. The pointer is shared by every Value
// that points at the same logical task - copying a `task of T` Value
// copies the pointer, not the underlying state. This is the one
// exception to Jennifer's value-semantics rule: a task by definition
// represents a single underlying operation, and `task.wait` /
// `task.discard` need to see the same handle the spawn produced.
//
// Concurrency contract (M16.0 Phase 2):
//   - The spawning goroutine writes Result, Err, and Done exactly once,
//     in that order, before closing the Done channel. No further writes
//     to those fields occur after Done is closed.
//   - Other goroutines read Result / Err / Done only after observing
//     Done is closed (via channel receive or close-check). The channel
//     close establishes the happens-before edge that makes those reads
//     safe without an explicit mutex.
//   - Observed is read/written under the assumption Phase 2 has no
//     `task.wait` yet, so only the main goroutine flips it at exit.
//     Phase 3 ships task.wait/discard; if those run from background
//     goroutines, we promote Observed to an atomic.
type TaskState struct {
	Result   Value         // the body's return value when Err is nil; null otherwise
	Err      error         // any error thrown / surfaced by the body
	Done     chan struct{} // closed by the spawned goroutine after Result / Err are written
	Observed bool          // flipped by task.wait (success or rethrow) and task.discard; the registry scan at program exit reports tasks where Done && Err != nil && !Observed
	ElemTyp  *parser.Type  // the task's declared element type T (in `task of T`); used by Value.MatchesDeclared
}

// IsDone reports whether the spawned body has finished (Result / Err
// are safe to read). Non-blocking; used by the registry scan at exit
// to skip tasks that are still running.
func (s *TaskState) IsDone() bool {
	if s == nil || s.Done == nil {
		return false
	}
	select {
	case <-s.Done:
		return true
	default:
		return false
	}
}

// Value is a tagged union for all Jennifer runtime values.
// One concrete struct rather than a Go interface hierarchy keeps GC pressure
// low and avoids reflect - important when the binary is built with TinyGo.
//
// Compound kinds (List, Map) carry their declared element / key+value
// types alongside the data so runtime checks (assignment, parameter
// binding, index-write type matching) have the same information the
// declaration site recorded. Reading just the data isn't enough: an
// empty `list of int` and an empty `list of string` are both empty
// slices, but they aren't assignment-compatible.
//
// Sub-values are stored by value (List elements, Map keys/values are
// Value, not *Value). That matches the value-semantics decision from M6:
// `$ys = $xs;` and parameter binding both copy the whole structure;
// aliasing is impossible.
type Value struct {
	Kind       ValueKind
	Int        int64
	Float      float64
	Str        string
	Bool       bool
	List       []Value       // KindList: element data
	Map        []MapEntry    // KindMap:  insertion-ordered entries
	Bytes      []byte        // KindBytes (M12): byte data
	Fields     []StructField // KindStruct (M13.1): declaration-ordered field values
	StructName string        // KindStruct: name of the struct definition this value belongs to
	StructNS   string        // KindStruct (M15.2): library namespace prefix; empty for user-defined structs
	ElemTyp    *parser.Type  // KindList: element type
	KeyTyp     *parser.Type  // KindMap:  key type
	ValTyp     *parser.Type  // KindMap:  value type
	Task       *TaskState    // KindTask (M16.0): shared handle - copying a task copies the pointer
}

func Null() Value              { return Value{Kind: KindNull} }
func IntVal(n int64) Value     { return Value{Kind: KindInt, Int: n} }
func FloatVal(f float64) Value { return Value{Kind: KindFloat, Float: f} }
func StringVal(s string) Value { return Value{Kind: KindString, Str: s} }
func BoolVal(b bool) Value     { return Value{Kind: KindBool, Bool: b} }

// BytesVal constructs a bytes value with the given data. The slice is
// taken by reference; callers needing value-semantics guarantees
// (assignment, parameter binding) must call Value.Copy. M12.
func BytesVal(data []byte) Value { return Value{Kind: KindBytes, Bytes: data} }

// StructVal constructs a user-defined struct value (M13.1).
func StructVal(name string, fields []StructField) Value {
	return Value{Kind: KindStruct, StructName: name, Fields: fields}
}

// NamespacedStructVal constructs a library-provided struct value
// behind the `<ns>.` prefix (M15.2). The runtime distinguishes
// `os.Result` from a user-defined `Result` via the namespace tag, so
// type matching at variable bindings keeps the two apart.
func NamespacedStructVal(ns, name string, fields []StructField) Value {
	return Value{Kind: KindStruct, StructNS: ns, StructName: name, Fields: fields}
}

// ListVal constructs a list value with the given element type and data.
// The data slice is taken by reference; callers that need value-semantics
// guarantees (assignment, parameter binding) must call Value.Copy.
func ListVal(elemT parser.Type, data []Value) Value {
	t := elemT
	return Value{Kind: KindList, List: data, ElemTyp: &t}
}

// TaskVal wraps an existing TaskState in a Value of kind KindTask. The
// state pointer is shared - subsequent copies of this Value reference
// the same underlying task (M16.0; see TaskState).
func TaskVal(elemT parser.Type, state *TaskState) Value {
	t := elemT
	state.ElemTyp = &t
	return Value{Kind: KindTask, Task: state}
}

// MapVal constructs a map value with the given key + value types and
// entries. Insertion order is whatever the entries slice expresses.
func MapVal(keyT, valT parser.Type, entries []MapEntry) Value {
	kt, vt := keyT, valT
	return Value{Kind: KindMap, Map: entries, KeyTyp: &kt, ValTyp: &vt}
}

// Copy returns a deep clone of v. Used everywhere value-semantics matter:
// variable assignment, function-parameter binding, returning a list/map
// from a function. Primitives are already value-typed so they fall
// through; lists and maps recurse so nested compound types are also
// independent. Cost is O(total elements); copy-on-write is a future
// optimization that won't change observable semantics.
func (v Value) Copy() Value {
	switch v.Kind {
	case KindList:
		out := make([]Value, len(v.List))
		for i, e := range v.List {
			out[i] = e.Copy()
		}
		return Value{Kind: KindList, List: out, ElemTyp: v.ElemTyp}
	case KindMap:
		out := make([]MapEntry, len(v.Map))
		for i, e := range v.Map {
			out[i] = MapEntry{Key: e.Key.Copy(), Value: e.Value.Copy()}
		}
		return Value{Kind: KindMap, Map: out, KeyTyp: v.KeyTyp, ValTyp: v.ValTyp}
	case KindBytes:
		// M12: same value-semantics as lists / maps - deep copy so the
		// callee can't surprise the caller by mutating a shared
		// underlying slice.
		out := make([]byte, len(v.Bytes))
		copy(out, v.Bytes)
		return Value{Kind: KindBytes, Bytes: out}
	case KindTask:
		// M16.0: tasks are the one exception to value-semantics. A task
		// represents a single underlying operation; copies of a `task
		// of T` Value share the same TaskState pointer so multiple
		// waiters see the same result and the observed bit is global to
		// the handle.
		return Value{Kind: KindTask, Task: v.Task}
	case KindStruct:
		// M13.1: deep copy fields so a struct passed into a method or
		// returned from it can't surprise its caller.
		out := make([]StructField, len(v.Fields))
		for i, f := range v.Fields {
			out[i] = StructField{Name: f.Name, Value: f.Value.Copy()}
		}
		return Value{Kind: KindStruct, StructNS: v.StructNS, StructName: v.StructName, Fields: out}
	}
	return v
}

// ZeroFor returns the zero value for a declared parser.Type. Used when a
// variable is defined without an `init` clause. For lists and maps the
// zero value is an *empty* container of the declared element / key+value
// type, not null - that matches the spec's "zero value of the declared
// type" rule and keeps `len($xs)` immediately well-defined.
func ZeroFor(t parser.Type) Value {
	switch t.Kind {
	case parser.TypeInt:
		return IntVal(0)
	case parser.TypeFloat:
		return FloatVal(0)
	case parser.TypeString:
		return StringVal("")
	case parser.TypeBool:
		return BoolVal(false)
	case parser.TypeNull:
		return Null()
	case parser.TypeBytes:
		return BytesVal([]byte{})
	case parser.TypeStruct:
		// M13.1: a zero struct is constructed by the interpreter when it
		// has access to the StructDef registry. ZeroFor only sees the
		// type name (Type.StructName), not the field list, so it returns
		// a struct with empty Fields here; execDefine routes uninitialised
		// struct declarations through a dedicated path that consults the
		// interpreter's struct table.
		return StructVal(t.StructName, nil)
	case parser.TypeList:
		var et parser.Type
		if t.Element != nil {
			et = *t.Element
		}
		return ListVal(et, []Value{})
	case parser.TypeMap:
		var kt, vt parser.Type
		if t.KeyType != nil {
			kt = *t.KeyType
		}
		if t.ValType != nil {
			vt = *t.ValType
		}
		return MapVal(kt, vt, []MapEntry{})
	}
	return Null()
}

// Display formats the value the way `printf` should render it.
func (v Value) Display() string {
	switch v.Kind {
	case KindNull:
		return "null"
	case KindInt:
		return strconv.FormatInt(v.Int, 10)
	case KindFloat:
		return DisplayFloat(v.Float)
	case KindString:
		return v.Str
	case KindBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case KindList:
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range v.List {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(displayElement(e))
		}
		b.WriteByte(']')
		return b.String()
	case KindMap:
		var b strings.Builder
		b.WriteByte('{')
		for i, e := range v.Map {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(displayElement(e.Key))
			b.WriteString(": ")
			b.WriteString(displayElement(e.Value))
		}
		b.WriteByte('}')
		return b.String()
	case KindBytes:
		// M12: bytes display as hex pairs separated by spaces, wrapped
		// in `bytes[...]`. Picked over a string-decoded form because
		// bytes are explicitly not assumed to be valid UTF-8 - the
		// hex form is unambiguous and round-trippable in `%v` output.
		var b strings.Builder
		b.WriteString("bytes[")
		for i, by := range v.Bytes {
			if i > 0 {
				b.WriteByte(' ')
			}
			const hex = "0123456789abcdef"
			b.WriteByte(hex[by>>4])
			b.WriteByte(hex[by&0x0f])
		}
		b.WriteByte(']')
		return b.String()
	case KindStruct:
		// M13.1: struct display reuses the literal-shaped form so a
		// printed value reads as the source code the user would write
		// to reproduce it. `Name{field: value, ...}` for non-empty
		// fields; `Name{}` for empty.
		var b strings.Builder
		if v.StructNS != "" {
			b.WriteString(v.StructNS)
			b.WriteByte('.')
		}
		b.WriteString(v.StructName)
		b.WriteByte('{')
		for i, f := range v.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(f.Name)
			b.WriteString(": ")
			b.WriteString(displayElement(f.Value))
		}
		b.WriteByte('}')
		return b.String()
	case KindTask:
		// M16.0: tasks are opaque handles - the display form just
		// labels the task as pending or done, without exposing the
		// captured frame or the result (which printf already covers
		// via task.wait + a primitive print).
		if v.Task == nil {
			return "task<?>"
		}
		if !v.Task.IsDone() {
			return "task<pending>"
		}
		if v.Task.Err != nil {
			return "task<error>"
		}
		return "task<done>"
	}
	return "<unknown>"
}

// displayElement is Display() but with string values quoted, so that
// list / map representations are unambiguous (`[1, "2", 3]` rather than
// `[1, 2, 3]` when the middle entry is a string). Nested lists/maps
// recurse through the regular Display() so they stay unquoted.
func displayElement(v Value) string {
	if v.Kind == KindString {
		return strconv.Quote(v.Str)
	}
	return v.Display()
}

// MatchesDeclared reports whether v's runtime kind matches a declared
// parser.Type. For lists and maps the inner types are compared too -
// `list of int` does not accept a value built as `list of string`. Empty
// containers with the right shape pass.
func (v Value) MatchesDeclared(t parser.Type) bool {
	switch t.Kind {
	case parser.TypeInt:
		return v.Kind == KindInt
	case parser.TypeFloat:
		return v.Kind == KindFloat
	case parser.TypeString:
		return v.Kind == KindString
	case parser.TypeBool:
		return v.Kind == KindBool
	case parser.TypeNull:
		return v.Kind == KindNull
	case parser.TypeBytes:
		return v.Kind == KindBytes
	case parser.TypeStruct:
		// M13.1 / M15.2: struct types match by (namespace, name). A
		// struct value with empty fields and matching name+ns matches
		// the declared type; that's how ZeroFor's placeholder gets
		// accepted before the interpreter materialises the real field
		// set in execDefine.
		return v.Kind == KindStruct && v.StructName == t.StructName && v.StructNS == t.StructNS
	case parser.TypeList:
		if v.Kind != KindList {
			return false
		}
		// Empty list with no recorded element type is assignable to any
		// list type. Otherwise element types must match recursively.
		if t.Element == nil || v.ElemTyp == nil {
			return true
		}
		return t.Element.Equal(*v.ElemTyp)
	case parser.TypeMap:
		if v.Kind != KindMap {
			return false
		}
		if t.KeyType == nil || t.ValType == nil || v.KeyTyp == nil || v.ValTyp == nil {
			return true
		}
		return t.KeyType.Equal(*v.KeyTyp) && t.ValType.Equal(*v.ValTyp)
	case parser.TypeTask:
		// M16.0: `task of T` matches a Value of KindTask whose recorded
		// element type equals T. A task value without a recorded element
		// type (shouldn't happen post-construction, but defensive) is
		// considered compatible with any task type.
		if v.Kind != KindTask {
			return false
		}
		if t.Element == nil || v.Task == nil || v.Task.ElemTyp == nil {
			return true
		}
		return t.Element.Equal(*v.Task.ElemTyp)
	}
	return false
}

// AsFloat returns the value as float64 if it's int or float; ok=false otherwise.
// Used by arithmetic and comparison to implement int->float promotion.
func (v Value) AsFloat() (float64, bool) {
	switch v.Kind {
	case KindInt:
		return float64(v.Int), true
	case KindFloat:
		return v.Float, true
	}
	return 0, false
}

// Equal implements Jennifer's `==` semantics. Same-kind: deep equal. Mixed
// int/float: compared as floats (per the int->float promotion rule). Any
// other cross-kind pairing is not equal. Lists and maps recurse.
func (v Value) Equal(o Value) bool {
	if v.Kind == o.Kind {
		switch v.Kind {
		case KindNull:
			return true
		case KindInt:
			return v.Int == o.Int
		case KindFloat:
			return v.Float == o.Float
		case KindString:
			return v.Str == o.Str
		case KindBool:
			return v.Bool == o.Bool
		case KindBytes:
			if len(v.Bytes) != len(o.Bytes) {
				return false
			}
			for i := range v.Bytes {
				if v.Bytes[i] != o.Bytes[i] {
					return false
				}
			}
			return true
		case KindStruct:
			// M13.1 / M15.2: structs compare equal if they have the
			// same (namespace, name) and every field's value matches in
			// declaration order. The namespace tag keeps a library
			// `os.Result` distinct from a user-defined `Result`.
			if v.StructName != o.StructName || v.StructNS != o.StructNS {
				return false
			}
			if len(v.Fields) != len(o.Fields) {
				return false
			}
			for i := range v.Fields {
				if v.Fields[i].Name != o.Fields[i].Name {
					return false
				}
				if !v.Fields[i].Value.Equal(o.Fields[i].Value) {
					return false
				}
			}
			return true
		case KindList:
			if len(v.List) != len(o.List) {
				return false
			}
			for i := range v.List {
				if !v.List[i].Equal(o.List[i]) {
					return false
				}
			}
			return true
		case KindMap:
			// Maps compare equal if they hold the same key->value mapping;
			// insertion order is *not* considered semantically significant
			// for equality (it only matters for iteration order).
			if len(v.Map) != len(o.Map) {
				return false
			}
			for _, a := range v.Map {
				found := false
				for _, b := range o.Map {
					if a.Key.Equal(b.Key) && a.Value.Equal(b.Value) {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
			return true
		}
	}
	// numeric promotion
	if a, ok := v.AsFloat(); ok {
		if b, ok := o.AsFloat(); ok {
			return a == b
		}
	}
	return false
}
