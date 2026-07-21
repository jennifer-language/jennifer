// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"jennifer-lang.dev/jennifer/internal/parser"
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
	StructNS   string        // KindStruct: display namespace prefix (library name, or a module's file stem); empty for user-defined structs
	ModPath    string        // KindStruct: module identity - the module's canonical path; empty for library / user structs. Two module files that share a stem stay distinct types because their ModPath differs, while StructNS keeps the readable stem for display.
	ElemTyp    *parser.Type  // KindList: element type
	KeyTyp     *parser.Type  // KindMap:  key type
	ValTyp     *parser.Type  // KindMap:  value type
	Task       *TaskState    // KindTask: shared handle - copying a task copies the pointer
	Obj        *Value        // KindObject: wrapped opaque payload (e.g. json.Value's decoded tree); StructNS/StructName carry the owning type

	// mapIdx is an unexported acceleration cache for KindMap: encoded scalar
	// key -> position in Map. It turns the O(n) linear scan in indexInto /
	// writeIndexedSlot into an O(1) lookup, so building a map with
	// `$m[$k] = $v` in a loop is O(n) instead of O(n^2). It is *advisory*:
	// used only when mapIndexUsable() confirms it is a complete 1:1 index
	// (non-nil and len == len(Map)); otherwise every op falls back to the
	// linear scan, which is always correct. That length stamp is what makes
	// it safe under value semantics - a stale copy, a duplicate-key literal,
	// or a map holding a non-hashable key all fail the check and scan. Built
	// by DeepCopy / evalMapLit / lazily in writeIndexedSlot; never relied on.
	mapIdx map[string]int
}

// mapKeyEncode returns a canonical string encoding for a hashable scalar map
// key (string / int / bool / null), plus ok. It mirrors Value.Equal exactly
// (which requires equal Kind), so distinct keys never collide. Float is
// deliberately excluded - NaN != NaN, -0.0 == 0.0, and precision make a string
// encoding disagree with == - as is every compound kind; those fall back to a
// linear scan.
func mapKeyEncode(v Value) (string, bool) {
	switch v.Kind {
	case KindString:
		return "s" + v.Str, true
	case KindInt:
		return "i" + strconv.FormatInt(v.Int, 10), true
	case KindBool:
		if v.Bool {
			return "b1", true
		}
		return "b0", true
	case KindNull:
		return "n", true
	}
	return "", false
}

// mapIndexUsable reports whether mapIdx is a complete, position-accurate index
// of Map. A length mismatch (stale after a slice-only mutation, a duplicate-key
// literal, or a non-hashable key that was never indexed) means "do not trust
// it" - the caller linear-scans instead, which is always correct.
func (v Value) mapIndexUsable() bool {
	return v.mapIdx != nil && len(v.mapIdx) == len(v.Map)
}

// buildMapIndex (re)builds mapIdx from Map, or leaves it nil when any key is
// non-hashable or two keys collide (a duplicate-key literal) - either way the
// map keeps linear-scanning. O(n).
func (v *Value) buildMapIndex() {
	idx := make(map[string]int, len(v.Map))
	for i := range v.Map {
		enc, ok := mapKeyEncode(v.Map[i].Key)
		if !ok {
			v.mapIdx = nil
			return
		}
		idx[enc] = i
	}
	if len(idx) != len(v.Map) {
		v.mapIdx = nil
		return
	}
	v.mapIdx = idx
}

// LookupKey returns the position of key in a KindMap Value, or -1 if absent.
// It uses the O(1) hash index when it is trustworthy and falls back to a linear
// Equal scan otherwise. Exported so libraries (maps.has / delete / merge) avoid
// an O(n) scan per lookup. key.Copy() is the caller's responsibility if stored.
func (v Value) LookupKey(key Value) int {
	if v.Kind != KindMap {
		return -1
	}
	if enc, hashable := mapKeyEncode(key); hashable && v.mapIndexUsable() {
		if pos, ok := v.mapIdx[enc]; ok {
			return pos
		}
		return -1
	}
	for i := range v.Map {
		if v.Map[i].Key.Equal(key) {
			return i
		}
	}
	return -1
}

// UpsertKey sets key -> val in a KindMap Value in place, appending a new entry
// when the key is absent, and keeps the hash index in lock-step so a sequence
// of upserts (e.g. maps.merge) stays O(n) rather than O(n^2). The receiver must
// be addressable; the maps library calls it on its own working copy. key / val
// are stored as given (the caller copies for value semantics).
func (v *Value) UpsertKey(key, val Value) {
	if v.Kind != KindMap {
		return
	}
	if enc, hashable := mapKeyEncode(key); hashable {
		if !v.mapIndexUsable() {
			v.buildMapIndex()
		}
		if v.mapIndexUsable() {
			if pos, ok := v.mapIdx[enc]; ok {
				v.Map[pos].Value = val
				return
			}
			v.Map = append(v.Map, MapEntry{Key: key, Value: val})
			v.mapIdx[enc] = len(v.Map) - 1
			return
		}
		// buildMapIndex declined (a non-hashable key elsewhere): fall to scan.
	}
	for i := range v.Map {
		if v.Map[i].Key.Equal(key) {
			v.Map[i].Value = val
			return
		}
	}
	v.Map = append(v.Map, MapEntry{Key: key, Value: val})
	v.mapIdx = nil
}

// DropMapIndex clears the hash index of a KindMap Value: positions became
// stale (e.g. after an entry was removed), so the next indexed op rebuilds it
// or falls back to the linear scan.
func (v *Value) DropMapIndex() { v.mapIdx = nil }

// Value semantics rest entirely on eager deep copies at every binding site:
// execDefine / execAssign (via eagerCopy), parameter binding (bindParamValue),
// and the spawn snapshot (DeepCopy) all take a private copy, and library
// builtins Copy() before they mutate. No two live bindings ever share a
// compound backing, so the mutation sites (execAppend / execIndexAssign /
// execFieldAssign) can mutate a binding's own backing in place - which is what
// keeps append-in-a-loop amortised O(N). A copy-on-write marker was tried here
// and removed: it was inert (a value receiver plus by-value Environment reads
// meant the flag never reached the stored binding, so it never detached). The
// write-through alternative is recorded in docs/technical/rejected.md.

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

// Copy returns a deep clone of v (kept as the public
// deep-copy alias). Libraries and other callers whose pattern is
// "Copy then mutate freely" (e.g. lists.shuffle, lists.reverse)
// keep working unchanged. Interpreter sites that can safely alias
// use Share; mutation sites use Ensure.
func (v Value) Copy() Value {
	return v.DeepCopy()
}

// DeepCopy recursively clones list elements, map entries, struct fields, and
// bytes so no backing slices / maps are shared with the source. It is the
// engine behind Copy() (every eager copy at a binding site) and behind
// snapshotForSpawn's value-semantics capture across goroutine boundaries.
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
		nv := Value{Kind: KindMap, Map: out, KeyTyp: v.KeyTyp, ValTyp: v.ValTyp}
		// A deep copy owns a fresh backing, so give it a fresh index rather
		// than sharing the source's (which the length stamp would reject
		// anyway once the copies diverge).
		nv.buildMapIndex()
		return nv
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
		return Value{Kind: KindStruct, StructNS: v.StructNS, ModPath: v.ModPath, StructName: v.StructName, Fields: out}
	case KindObject:
		// The wrapped payload is immutable - the library write surface returns
		// fresh handles, nothing mutates the tree in place - so share the Obj
		// pointer instead of deep-copying the whole document at every bind.
		// Aliasing is unobservable, and a large decoded document is no longer
		// multiplied into every call / assignment.
		return Value{Kind: KindObject, StructNS: v.StructNS, StructName: v.StructName, Obj: v.Obj}
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
		return (v.Kind == KindStruct || v.Kind == KindObject) && v.StructName == t.StructName && v.StructNS == t.StructNS && v.ModPath == t.ModPath
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

// isNumeric reports whether v is an int or float.
func (v Value) isNumeric() bool {
	return v.Kind == KindInt || v.Kind == KindFloat
}

// Equal implements Jennifer's `==` semantics. Same-kind: deep equal. Mixed
// int/float: compared exactly (not via a lossy float promotion). Any other
// cross-kind pairing is not equal. Lists and maps recurse.
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
			if v.StructName != o.StructName || v.StructNS != o.StructNS || v.ModPath != o.ModPath {
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
			// Fast path: index o's keys. Requires o's keys hashable and
			// unique (the normal case - duplicate map literals are
			// rejected); each v entry must then hit a distinct o entry with
			// an equal value. O(n) instead of O(n*m).
			oIdx := make(map[string]int, len(o.Map))
			indexable := true
			for i := range o.Map {
				enc, ok := mapKeyEncode(o.Map[i].Key)
				if !ok {
					indexable = false
					break
				}
				if _, dup := oIdx[enc]; dup {
					indexable = false
					break
				}
				oIdx[enc] = i
			}
			if indexable {
				seen := make([]bool, len(o.Map))
				for _, a := range v.Map {
					enc, ok := mapKeyEncode(a.Key)
					if !ok {
						// A non-hashable (compound) key cannot equal any of
						// o's hashable scalar keys.
						return false
					}
					pos, ok := oIdx[enc]
					if !ok || seen[pos] || !a.Value.Equal(o.Map[pos].Value) {
						return false
					}
					seen[pos] = true
				}
				return true
			}
			// Degenerate maps (duplicate or non-hashable keys): a one-way
			// containment check plus the length test is fooled by duplicates
			// ({a,a} vs {a,b}), so check containment in BOTH directions.
			for _, a := range v.Map {
				if !mapContainsEntry(o, a) {
					return false
				}
			}
			for _, b := range o.Map {
				if !mapContainsEntry(v, b) {
					return false
				}
			}
			return true
		}
	}
	// Cross-kind int/float: compare exactly. Promoting the int to float64
	// (the old path) loses precision above 2^53, so e.g.
	// 9007199254740993 == 9007199254740992.0 wrongly reported true.
	if v.Kind == KindInt && o.Kind == KindFloat {
		return compareIntFloat(v.Int, o.Float) == 0
	}
	if v.Kind == KindFloat && o.Kind == KindInt {
		return compareIntFloat(o.Int, v.Float) == 0
	}
	return false
}

// mapContainsEntry reports whether m holds an entry with an equal key AND an
// equal value. Linear; only used by Equal's degenerate-map fallback.
func mapContainsEntry(m Value, e MapEntry) bool {
	for i := range m.Map {
		if m.Map[i].Key.Equal(e.Key) && m.Map[i].Value.Equal(e.Value) {
			return true
		}
	}
	return false
}

// compareIntFloat returns the sign of (n - f) exactly: -1 if n < f, 0 if equal,
// +1 if n > f. It avoids converting n to float64 (lossy above 2^53). f is
// assumed finite (Jennifer forbids NaN / Inf floats).
func compareIntFloat(n int64, f float64) int {
	// f beyond the int64 range decides by magnitude alone.
	if f >= 9223372036854775808.0 { // >= 2^63, above MaxInt64
		return -1
	}
	if f < -9223372036854775808.0 { // < -2^63
		return 1
	}
	// f is now within (-2^63, 2^63), so int64(f) truncates it exactly.
	tf := int64(f)
	if n < tf {
		return -1
	}
	if n > tf {
		return 1
	}
	// Integer parts match; the fractional part of f (if any) breaks the tie.
	// float64(tf) is exact here: |tf| <= 2^53 makes it representable, and a
	// larger f has no fractional part (it is already integral).
	frac := f - float64(tf)
	if frac > 0 {
		return -1
	}
	if frac < 0 {
		return 1
	}
	return 0
}
