// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// DisplayFloat formats a float64 for human output. Unlike `strconv.FormatFloat`
// with verb 'g', it guarantees the result is recognisable as a float: if the
// shortest round-trip representation has no `.`, `e`, or `E`, we append `.0`.
// So `5.0` prints as "5.0", not "5" - the value's *type* stays visible even
// when its value happens to be a whole number.
func DisplayFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	// +Inf / -Inf / NaN have no `.eE`, but a `.0` suffix on them is wrong.
	if s == "+Inf" || s == "-Inf" || s == "NaN" {
		return s
	}
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
	KindBytes  // mutable byte sequence; elements are int in [0, 255]
	KindList   // ordered, mutable sequence (Go slice underneath)
	KindMap    // ordered key-value map; iteration is insertion order
	KindStruct // user-defined record; fields are an ordered list of (name, value)
	KindTask   // a pending or completed `spawn { ... }` computation
	KindObject // opaque, library-owned value (e.g. json.Value); wraps an inner tree in Obj
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
	case KindObject:
		return "object"
	}
	return "?"
}

// MapEntry is one entry in a Value of kind KindMap. Maps preserve
// insertion order (so iteration is deterministic), so they're stored as a
// parallel slice rather than a Go map[string]Value or similar.
type MapEntry struct {
	Key, Value Value
}

// StructField is one field in a Value of kind KindStruct.
// Fields are kept in declaration order (matching the StructDef's field
// ordering) so display output and assertions are deterministic.
type StructField struct {
	Name  string
	Value Value
}

// TaskState is the payload of a Value of kind KindTask. It holds
// the eventual result of a `spawn { ... }` block plus bookkeeping for the
// exit-time unwaited-error scan. The pointer is shared by every Value
// that points at the same logical task - copying a `task of T` Value
// copies the pointer, not the underlying state. This is the one
// exception to Jennifer's value-semantics rule: a task by definition
// represents a single underlying operation, and `task.wait` /
// `task.discard` need to see the same handle the spawn produced.
//
// Concurrency contract (Phase 2):
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
	Observed atomic.Bool   // flipped by task.wait (success or rethrow) and task.discard from whatever goroutine runs them (incl. spawn bodies); the exit-time registry scan reads it, so it must be atomic. Set: Done && Err != nil && !Observed.Load() reports.
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
// Value, not *Value). That matches the value-semantics decision:
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
	Bytes      []byte        // KindBytes: byte data
	Fields     []StructField // KindStruct: declaration-ordered field values
	StructName string        // KindStruct: name of the struct definition this value belongs to
	StructNS   string        // KindStruct: library namespace prefix; empty for user-defined structs
	ElemTyp    *parser.Type  // KindList: element type
	KeyTyp     *parser.Type  // KindMap:  key type
	ValTyp     *parser.Type  // KindMap:  value type
	Task       *TaskState    // KindTask: shared handle - copying a task copies the pointer
	Obj        *Value        // KindObject: wrapped opaque payload (e.g. json.Value's decoded tree); StructNS/StructName carry the owning type

	// shared is the aliasing marker for compound backings.
	// nil for freshly-constructed compounds and for scalars; set by
	// Share() to a *bool = true when the Value gets read via
	// evalExpr(VarExpr) (any variable reference is a potential alias
	// creator). Mutation sites call Ensure() to detach - if shared is
	// non-nil AND *shared is true, Ensure DeepCopies into a private
	// backing; otherwise it's an O(1) pass-through.
	//
	// The flag is one-directional: once set to true it never flips
	// back. That's pessimistic but correct: a Value that was ever
	// aliased will detach on next mutation even if the alias has
	// since gone out of scope. Refcounted COW is a possible future
	// optimisation.
	shared *bool
}

func Null() Value              { return Value{Kind: KindNull} }
func IntVal(n int64) Value     { return Value{Kind: KindInt, Int: n} }
func FloatVal(f float64) Value { return Value{Kind: KindFloat, Float: f} }
func StringVal(s string) Value { return Value{Kind: KindString, Str: s} }
func BoolVal(b bool) Value     { return Value{Kind: KindBool, Bool: b} }

// BytesVal constructs a bytes value with the given data. The slice is
// taken by reference; callers needing value-semantics guarantees
// (assignment, parameter binding) must call Value.Copy.
func BytesVal(data []byte) Value { return Value{Kind: KindBytes, Bytes: data} }

// StructVal constructs a user-defined struct value.
func StructVal(name string, fields []StructField) Value {
	return Value{Kind: KindStruct, StructName: name, Fields: fields}
}

// NamespacedStructVal constructs a library-provided struct value
// behind the `<ns>.` prefix. The runtime distinguishes
// `os.Result` from a user-defined `Result` via the namespace tag, so
// type matching at variable bindings keeps the two apart.
func NamespacedStructVal(ns, name string, fields []StructField) Value {
	return Value{Kind: KindStruct, StructNS: ns, StructName: name, Fields: fields}
}

// ObjectVal wraps an inner value tree as an opaque, library-owned
// KindObject (e.g. json.Value). ns/name identify the owning type; the
// language rejects operators / [index] / .field on it, so only that
// library's accessors reach into the wrapped tree.
func ObjectVal(ns, name string, inner Value) Value {
	return Value{Kind: KindObject, StructNS: ns, StructName: name, Obj: &inner}
}

// AsObject unwraps a KindObject of the given ns/name, returning its inner
// tree. Reports false for any other value, so a builtin can reject a
// wrong-typed argument cleanly.
func (v Value) AsObject(ns, name string) (Value, bool) {
	if v.Kind == KindObject && v.StructNS == ns && v.StructName == name {
		if v.Obj != nil {
			return *v.Obj, true
		}
		return Null(), true
	}
	return Value{}, false
}

// ObjectDisplayer renders an opaque object's inner tree for Display() (the
// REPL echo, `%v`, error messages). A library supplies one when it registers
// its object type; without one, Display falls back to the bare `<ns.name>`
// form.
type ObjectDisplayer func(inner Value) string

// objectDisplayers is keyed by "ns.name". It is package-level because
// Value.Display() has no Interpreter reference, and safe as shared state: the
// registered function is a stateless renderer that a library re-registers
// identically per interpreter. A sync.Map keeps registration (setup time) and
// lookup (possibly from spawn goroutines) race-free.
var objectDisplayers sync.Map

// registerObjectDisplayer records the renderer for an opaque object type.
func registerObjectDisplayer(ns, name string, d ObjectDisplayer) {
	if d != nil {
		objectDisplayers.Store(ns+"."+name, d)
	}
}

// objectDisplayer looks up the renderer for an opaque object type, or nil.
func objectDisplayer(ns, name string) ObjectDisplayer {
	if d, ok := objectDisplayers.Load(ns + "." + name); ok {
		return d.(ObjectDisplayer)
	}
	return nil
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
// the same underlying task (see TaskState).
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

// Share marks v as an aliased view of its compound backing
// and returns v unchanged (same slice headers, no allocation). Called
// from evalExpr for VarExpr so any variable reference records that
// the underlying data might now have multiple readers. A future
// mutation goes through Ensure and pays the deep-copy cost only if
// the shared flag is set.
//
// For scalars and KindTask (which shares state by design), Share is
// a no-op. The scalar fast path is a single range compare
// (`KindBytes <= Kind <= KindStruct`) so the Go inliner can fold the
// call at every VarExpr eval site, eliminating the function-call
// overhead for the common scalar-variable read.
func (v Value) Share() Value {
	if v.Kind < KindBytes || v.Kind > KindStruct {
		return v
	}
	if v.shared == nil {
		t := true
		v.shared = &t
	} else {
		*v.shared = true
	}
	return v
}

// Ensure is the mutation-site detach. If v was Shared
// (sharedTag non-nil and *shared true), return a DeepCopy with a
// private backing that the caller can safely mutate. Otherwise
// return v as-is - the common append/rebind loop where nothing else
// references the value.
//
// The Ensure/DeepCopy protocol replaces the previous
// "binding.Value.Copy()" at every mutation site, cutting the
// append-in-a-loop pattern from O(N^2) to amortised O(N).
func (v Value) Ensure() Value {
	if v.shared != nil && *v.shared {
		return v.DeepCopy()
	}
	return v
}

// Copy returns a deep clone of v (kept as the public
// deep-copy alias). Libraries and other callers whose pattern is
// "Copy then mutate freely" (e.g. lists.shuffle, lists.reverse)
// keep working unchanged. Interpreter sites that can safely alias
// use Share; mutation sites use Ensure.
func (v Value) Copy() Value {
	return v.DeepCopy()
}

// DeepCopy is the historical Copy() behaviour: recursively clone list
// elements, map entries, struct fields, and bytes so no shared
// backings remain. Called by Ensure at mutation time and by
// snapshotForSpawn's value-semantics capture across goroutine
// boundaries. The returned Value's shared marker is nil - owned by
// nobody else.
func (v Value) DeepCopy() Value {
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
		// same value-semantics as lists / maps - deep copy so the
		// callee can't surprise the caller by mutating a shared
		// underlying slice.
		out := make([]byte, len(v.Bytes))
		copy(out, v.Bytes)
		return Value{Kind: KindBytes, Bytes: out}
	case KindTask:
		// tasks are the one exception to value-semantics. A task
		// represents a single underlying operation; copies of a `task
		// of T` Value share the same TaskState pointer so multiple
		// waiters see the same result and the observed bit is global to
		// the handle.
		return Value{Kind: KindTask, Task: v.Task}
	case KindStruct:
		// deep copy fields so a struct passed into a method or
		// returned from it can't surprise its caller.
		out := make([]StructField, len(v.Fields))
		for i, f := range v.Fields {
			out[i] = StructField{Name: f.Name, Value: f.Value.Copy()}
		}
		return Value{Kind: KindStruct, StructNS: v.StructNS, StructName: v.StructName, Fields: out}
	case KindObject:
		// opaque, immutable payload; deep-copy the wrapped tree so value
		// semantics hold even though there are no mutation paths on it.
		if v.Obj == nil {
			return Value{Kind: KindObject, StructNS: v.StructNS, StructName: v.StructName}
		}
		inner := v.Obj.DeepCopy()
		return Value{Kind: KindObject, StructNS: v.StructNS, StructName: v.StructName, Obj: &inner}
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
		// a zero struct is constructed by the interpreter when it
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
		// bytes display as hex pairs separated by spaces, wrapped
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
		// struct display reuses the literal-shaped form so a
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
		// tasks are opaque handles - the display form just
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
	case KindObject:
		// opaque - the payload is reachable only through the owning
		// library's accessors, but a library may register a displayer so
		// `$v` at the REPL and `%v` show its content instead of the bare
		// type name.
		if d := objectDisplayer(v.StructNS, v.StructName); d != nil {
			inner := Null()
			if v.Obj != nil {
				inner = *v.Obj
			}
			return d(inner)
		}
		return "<" + v.StructNS + "." + v.StructName + ">"
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
		// struct types match by (namespace, name). This also covers the
		// opaque KindObject types (json.Value): they register like a
		// namespaced struct for parsing / declaration and carry the same
		// ns+name, but their runtime kind is KindObject.
		return (v.Kind == KindStruct || v.Kind == KindObject) && v.StructName == t.StructName && v.StructNS == t.StructNS
	case parser.TypeList:
		if v.Kind != KindList {
			return false
		}
		// A value with a recorded element type compares by that type. A generic
		// value - one with no recorded element type, i.e. a fresh literal or a
		// json.decode result - is validated element by element against the
		// declared type, so a heterogeneous or mismatched collection can't slip
		// past a typed binding. An empty list matches any list type either way.
		if v.ElemTyp != nil {
			return t.Element == nil || t.Element.Equal(*v.ElemTyp)
		}
		for _, e := range v.List {
			if t.Element != nil && !e.MatchesDeclared(*t.Element) {
				return false
			}
		}
		return true
	case parser.TypeMap:
		if v.Kind != KindMap {
			return false
		}
		// Recorded key+value types compare directly; a generic map - no recorded
		// value type, i.e. a literal or a json.decode result - is validated entry
		// by entry against the declared key/value types. An empty map matches any
		// map type either way.
		if v.KeyTyp != nil && v.ValTyp != nil {
			return (t.KeyType == nil || t.KeyType.Equal(*v.KeyTyp)) &&
				(t.ValType == nil || t.ValType.Equal(*v.ValTyp))
		}
		for _, e := range v.Map {
			if t.KeyType != nil && !e.Key.MatchesDeclared(*t.KeyType) {
				return false
			}
			if t.ValType != nil && !e.Value.MatchesDeclared(*t.ValType) {
				return false
			}
		}
		return true
	case parser.TypeTask:
		// `task of T` matches a Value of KindTask whose recorded
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
			// structs compare equal if they have the
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
