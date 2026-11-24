// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"math"
	"sync"
	"unsafe"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// The packed layout of Value and its accessors. Value holds Kind and two small
// caches, one scalar word, one slice header and one pointer; a tag byte says
// what the slice header and the pointer hold. Everything that is not read on
// every evaluation step lives behind the pointer:
//
//	struct / enum   *Identity      interned (ns, modPath, name, variant)
//	list            *parser.Type   the element type (nil = generic)
//	map             *mapDesc       key / value types and the key index
//	task            *TaskState     shared handle
//	channel         *ChannelState  shared handle
//	object          *ObjectBox     identity plus the immutable payload
//	func            *funcRef       the method and its home interpreter
//
// Every read goes through a method here that checks the tag before it
// reinterprets the untyped words, and returns the zero value of the field
// otherwise, which is what the old one-field-per-payload struct returned for a
// field the value's kind did not use.

// Slice payload tags (low nibble of Value.tag).
const (
	sliceNone uint8 = iota
	sliceStr
	sliceList
	sliceMap
	sliceBytes
	sliceFields
)

// Out-of-line payload tags (high nibble of Value.tag).
const (
	refNone uint8 = iota
	refIdent
	refElem
	refMap
	refTask
	refChan
	refObj
	refFunc
)

func (v *Value) setSliceTag(t uint8) { v.tag = v.tag&0xf0 | t }
func (v *Value) setRefTag(t uint8)   { v.tag = v.tag&0x0f | t<<4 }
func (v Value) sliceTag() uint8      { return v.tag & 0x0f }
func (v Value) refTag() uint8        { return v.tag >> 4 }

// ---- scalar word -----------------------------------------------------------

// Int returns the payload of a KindInt value, 0 for any other kind.
func (v Value) Int() int64 {
	if v.Kind != KindInt {
		return 0
	}
	return int64(v.word)
}

// SetInt stores n as the scalar payload. The value reads it back through Int
// only while its Kind is KindInt.
func (v *Value) SetInt(n int64) { v.word = uint64(n) }

// Float returns the payload of a KindFloat value, 0 for any other kind.
func (v Value) Float() float64 {
	if v.Kind != KindFloat {
		return 0
	}
	return math.Float64frombits(v.word)
}

// SetFloat stores f as the scalar payload (read back while Kind is KindFloat).
func (v *Value) SetFloat(f float64) { v.word = math.Float64bits(f) }

// Bool returns the payload of a KindBool value, false for any other kind.
func (v Value) Bool() bool { return v.Kind == KindBool && v.word != 0 }

// SetBool stores b as the scalar payload (read back while Kind is KindBool).
func (v *Value) SetBool(b bool) {
	v.word = 0
	if b {
		v.word = 1
	}
}

// ---- slice header ----------------------------------------------------------

// Str returns the string payload ("" when the value holds none).
func (v Value) Str() string {
	if v.sliceTag() != sliceStr {
		return ""
	}
	return unsafe.String((*byte)(v.sp), v.sn)
}

// SetStr stores s as the slice payload. It drops the rune-count cache; build a
// string value with StringVal to have it.
func (v *Value) SetStr(s string) {
	v.sp, v.sn, v.sc = unsafe.Pointer(unsafe.StringData(s)), len(s), len(s)
	v.strRunes = 0
	v.setSliceTag(sliceStr)
}

// List returns the element slice of a list value (nil when it holds none). The
// slice shares the value's backing: element writes through it are writes into
// the value, an append is not (store it back with SetList).
func (v Value) List() []Value {
	if v.sliceTag() != sliceList {
		return nil
	}
	return unsafe.Slice((*Value)(v.sp), v.sc)[:v.sn]
}

// SetList stores s as the list backing.
func (v *Value) SetList(s []Value) {
	v.sp, v.sn, v.sc = unsafe.Pointer(unsafe.SliceData(s)), len(s), cap(s)
	v.setSliceTag(sliceList)
}

// Map returns the entries of a map value (nil when it holds none), sharing the
// backing like List.
func (v Value) Map() []MapEntry {
	if v.sliceTag() != sliceMap {
		return nil
	}
	return unsafe.Slice((*MapEntry)(v.sp), v.sc)[:v.sn]
}

// SetMap stores s as the map entries. It does not touch the key index, whose
// length check rejects it once it no longer matches.
func (v *Value) SetMap(s []MapEntry) {
	v.sp, v.sn, v.sc = unsafe.Pointer(unsafe.SliceData(s)), len(s), cap(s)
	v.setSliceTag(sliceMap)
}

// Bytes returns the data of a bytes value (nil when it holds none), sharing the
// backing like List.
func (v Value) Bytes() []byte {
	if v.sliceTag() != sliceBytes {
		return nil
	}
	return unsafe.Slice((*byte)(v.sp), v.sc)[:v.sn]
}

// SetBytes stores s as the bytes backing.
func (v *Value) SetBytes(s []byte) {
	v.sp, v.sn, v.sc = unsafe.Pointer(unsafe.SliceData(s)), len(s), cap(s)
	v.setSliceTag(sliceBytes)
}

// Fields returns the fields of a struct value or an enum variant's payload
// (nil when it holds none), sharing the backing like List.
func (v Value) Fields() []StructField {
	if v.sliceTag() != sliceFields {
		return nil
	}
	return unsafe.Slice((*StructField)(v.sp), v.sc)[:v.sn]
}

// SetFields stores s as the field backing.
func (v *Value) SetFields(s []StructField) {
	v.sp, v.sn, v.sc = unsafe.Pointer(unsafe.SliceData(s)), len(s), cap(s)
	v.setSliceTag(sliceFields)
}

// ---- type identity ---------------------------------------------------------

// Identity is the type identity of a struct, enum or opaque object value: the
// display namespace (a library name or a module's file stem), the module's
// canonical path (empty for a library or user type), the type name and, for
// an enum value, the active variant. Identities are interned and immutable, so
// every value of one type (and variant) shares one *Identity, comparing two of
// them is a pointer comparison, and a copy of a value copies one pointer.
type Identity struct {
	ns, modPath, name, variant string
}

// NS returns the display namespace ("" for a user type).
func (t *Identity) NS() string {
	if t == nil {
		return ""
	}
	return t.ns
}

// ModPath returns the canonical path of the defining module ("" for a library
// or user type).
func (t *Identity) ModPath() string {
	if t == nil {
		return ""
	}
	return t.modPath
}

// Name returns the struct, enum or object type name.
func (t *Identity) Name() string {
	if t == nil {
		return ""
	}
	return t.name
}

// Variant returns an enum value's active variant ("" for any other type).
func (t *Identity) Variant() string {
	if t == nil {
		return ""
	}
	return t.variant
}

var identities struct {
	sync.RWMutex
	m map[Identity]*Identity
}

// InternIdentity returns the shared identity for (ns, modPath, name, variant),
// creating it on first use; nil for the all-empty identity. Safe from any
// goroutine. The table grows with the number of distinct types a program
// uses, which is bounded by its declarations.
func InternIdentity(ns, modPath, name, variant string) *Identity {
	key := Identity{ns: ns, modPath: modPath, name: name, variant: variant}
	if key == (Identity{}) {
		return nil
	}
	identities.RLock()
	t := identities.m[key]
	identities.RUnlock()
	if t != nil {
		return t
	}
	identities.Lock()
	defer identities.Unlock()
	if t = identities.m[key]; t != nil {
		return t
	}
	if identities.m == nil {
		identities.m = make(map[Identity]*Identity)
	}
	t = &key
	identities.m[key] = t
	return t
}

// Identity returns the type identity of a struct, enum or object value (nil
// for any other value).
func (v Value) Identity() *Identity {
	switch v.refTag() {
	case refIdent:
		return (*Identity)(v.ref)
	case refObj:
		if v.ref != nil {
			return (*ObjectBox)(v.ref).id
		}
	}
	return nil
}

// SetIdentity gives a struct or enum value the identity t. On an object value
// it rewraps the payload under the new identity (the box is shared, so it is
// never written).
func (v *Value) SetIdentity(t *Identity) {
	if v.refTag() == refObj && v.ref != nil {
		b := *(*ObjectBox)(v.ref)
		b.id = t
		v.ref = unsafe.Pointer(&b)
		return
	}
	v.ref = unsafe.Pointer(t)
	v.setRefTag(refIdent)
}

// StructName returns the struct, enum or object type name ("" otherwise).
func (v Value) StructName() string { return v.Identity().Name() }

// StructNS returns the type's display namespace ("" for a user type).
func (v Value) StructNS() string { return v.Identity().NS() }

// ModPath returns the canonical path of the module that defines the type (""
// for a library or user type). Two module files that share a stem stay
// distinct types because their ModPath differs.
func (v Value) ModPath() string { return v.Identity().ModPath() }

// Variant returns an enum value's active variant ("" otherwise).
func (v Value) Variant() string { return v.Identity().Variant() }

// sameType reports whether two values carry the same struct / enum / object
// identity (variant included). Interning makes it a pointer comparison.
func sameType(a, b Value) bool { return a.Identity() == b.Identity() }

// ---- collection element types ----------------------------------------------

// ElemTyp returns a list's recorded element type, nil for a generic list (a
// fresh literal or a decoder result) and for any other value.
func (v Value) ElemTyp() *parser.Type {
	if v.refTag() != refElem {
		return nil
	}
	return (*parser.Type)(v.ref)
}

// SetElemTyp records a list's element type. The type is shared, never written.
func (v *Value) SetElemTyp(t *parser.Type) {
	v.ref = unsafe.Pointer(t)
	v.setRefTag(refElem)
}

// mapDesc is the out-of-line part of a map value: the recorded key and value
// types (both nil for a generic map) and idx, an acceleration cache of encoded
// scalar key -> position in the entries. idx turns the O(n) linear scan in
// indexInto / writeIndexedSlot into an O(1) lookup, so building a map with
// `$m[$k] = $v` in a loop is O(n) instead of O(n^2). It is *advisory*: used
// only when mapIndexUsable() confirms it is a complete 1:1 index (non-nil and
// as long as the entries); otherwise every op falls back to the linear scan,
// which is always correct. That length stamp is what makes it safe under value
// semantics - a stale copy, a duplicate-key literal, or a map holding a
// non-hashable key all fail the check and scan. Built by DeepCopy /
// evalMapLit / lazily in writeIndexedSlot; never relied on.
//
// A by-value copy of a map shares its mapDesc, as it shared the index before:
// the fields are never reassigned in place (a new index or new types take a
// new mapDesc), only the index's entries are added to, which the length check
// covers.
type mapDesc struct {
	key, val *parser.Type
	idx      map[string]int
}

func (v Value) mapDesc() *mapDesc {
	if v.refTag() != refMap {
		return nil
	}
	return (*mapDesc)(v.ref)
}

// KeyTyp returns a map's recorded key type (nil for a generic map).
func (v Value) KeyTyp() *parser.Type {
	if d := v.mapDesc(); d != nil {
		return d.key
	}
	return nil
}

// ValTyp returns a map's recorded value type (nil for a generic map).
func (v Value) ValTyp() *parser.Type {
	if d := v.mapDesc(); d != nil {
		return d.val
	}
	return nil
}

// SetMapTypes records a map's key and value types, keeping its key index.
func (v *Value) SetMapTypes(key, val *parser.Type) {
	d := v.mapDesc()
	if d != nil && d.key == key && d.val == val {
		return
	}
	nd := &mapDesc{key: key, val: val}
	if d != nil {
		nd.idx = d.idx
	}
	v.ref = unsafe.Pointer(nd)
	v.setRefTag(refMap)
}

// mapIndex returns the map's key index (nil when it has none).
func (v Value) mapIndex() map[string]int {
	if d := v.mapDesc(); d != nil {
		return d.idx
	}
	return nil
}

// setMapIndex replaces the map's key index, keeping its types.
func (v *Value) setMapIndex(idx map[string]int) {
	d := v.mapDesc()
	if d == nil {
		if idx == nil {
			return
		}
		d = &mapDesc{}
	}
	if d.idx == nil && idx == nil {
		return
	}
	nd := &mapDesc{key: d.key, val: d.val, idx: idx}
	v.ref = unsafe.Pointer(nd)
	v.setRefTag(refMap)
}

// ---- handles ---------------------------------------------------------------

// handleVal builds a value whose payload is the out-of-line pointer p.
func handleVal(k ValueKind, refT uint8, p unsafe.Pointer) Value {
	return Value{Kind: k, tag: refT << 4, ref: p}
}

// Task returns a task value's shared state (nil otherwise).
func (v Value) Task() *TaskState {
	if v.refTag() != refTask {
		return nil
	}
	return (*TaskState)(v.ref)
}

// Chan returns a channel value's shared state (nil otherwise).
func (v Value) Chan() *ChannelState {
	if v.refTag() != refChan {
		return nil
	}
	return (*ChannelState)(v.ref)
}

// Obj returns an object value's payload box (nil otherwise).
func (v Value) Obj() *ObjectBox {
	if v.refTag() != refObj {
		return nil
	}
	return (*ObjectBox)(v.ref)
}

// funcRef is the out-of-line part of a func value: the referenced top-level
// method and the interpreter that defined it (its home context; a func value
// called from another interpreter dispatches back there so the body resolves
// its own namespace imports, nil = the caller's). Immutable, shared on copy.
type funcRef struct {
	fn   *parser.MethodDef
	home *Interpreter
}

// Fn returns the method a func value references (nil for the uninitialized
// zero and for any other value).
func (v Value) Fn() *parser.MethodDef {
	if v.refTag() != refFunc || v.ref == nil {
		return nil
	}
	return (*funcRef)(v.ref).fn
}

// FnHome returns the interpreter that defined a func value's method (nil when
// unknown or for any other value).
func (v Value) FnHome() *Interpreter {
	if v.refTag() != refFunc || v.ref == nil {
		return nil
	}
	return (*funcRef)(v.ref).home
}

// taskVal wraps a task state without touching its element type.
func taskVal(state *TaskState) Value { return handleVal(KindTask, refTask, unsafe.Pointer(state)) }
