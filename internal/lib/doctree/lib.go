// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package doctree

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// Lib is one document library (`json`, `toml`, `yaml`) over the shared tree:
// its name, and the few behaviours that differ between the formats. Install
// registers the opaque `NAME.Value` type and the accessor and write surface the
// three libraries share; the library registers its own decode / encode.
//
// A Lib is a package-level value of its library, with every field set in its
// composite literal (not in an init function: package-level variables, such as
// a test's Builtin bindings, are initialised before init runs).
type Lib struct {
	// Name is the namespace (`json`) - the prefix of every builtin and of
	// the `NAME.Value` type.
	Name string
	// HasNull registers `isNull` (a format with a null value: json, yaml).
	HasNull bool
	// Display renders a document for `%v`, the REPL echo and
	// convert.toString (normally the compact encoding).
	Display func(Node) (string, error)
	// TimeNode, when set, stores a `time.Time` struct as a datetime node
	// (toml, yaml); without it a time.Time is an ordinary struct, stored as a
	// map of its fields (json). It also enables `isDatetime` / `asDatetime`.
	TimeNode func(v interpreter.Value) (Node, bool)
	// AsTime converts a datetime node to a Go time for `asDatetime`.
	AsTime func(n Node) (stdtime.Time, error)
}

// payload is the native payload of a `NAME.Value` handle: one node, the root
// of a document or any sub-tree of one.
type payload struct {
	root Node
	lib  *Lib
}

// EqualNative implements interpreter.NativeObject.
func (p *payload) EqualNative(other interpreter.NativeObject) bool {
	o, ok := other.(*payload)
	return ok && o.lib == p.lib && p.root.Equal(o.root)
}

// DisplayNative implements interpreter.NativeObject.
func (p *payload) DisplayNative() string {
	if p.lib.Display != nil {
		if s, err := p.lib.Display(p.root); err == nil {
			return s
		}
	}
	return "<" + p.lib.Name + ".Value>"
}

// TypeName is the qualified name of the library's value type (`json.Value`).
func (l *Lib) TypeName() string { return l.Name + ".Value" }

// Wrap makes a `NAME.Value` handle over n.
func (l *Lib) Wrap(n Node) interpreter.Value {
	return interpreter.NativeObjectVal(l.Name, "Value", &payload{root: n, lib: l})
}

// Zero is the value of an uninitialised `def v as NAME.Value;`: null.
func (l *Lib) Zero() interpreter.Value { return l.Wrap(Null()) }

// ValueType is the parser type of a `NAME.Value`, stamped on a returned
// `list of NAME.Value` so it binds to a declared one.
func (l *Lib) ValueType() parser.Type {
	return parser.Type{Kind: parser.TypeStruct, StructNS: l.Name, StructName: "Value"}
}

// AsNode returns the node a `NAME.Value` handle holds. A handle with no native
// payload (an uninitialised value built outside the registered zero) reads as
// null, the same as the registered zero.
func (l *Lib) AsNode(v interpreter.Value) (Node, bool) {
	if p, ok := v.AsNativeObject(l.Name, "Value"); ok {
		if dp, ok := p.(*payload); ok && dp.lib == l {
			return dp.root, true
		}
		return Node{}, false
	}
	if _, ok := v.AsObject(l.Name, "Value"); ok {
		return Null(), true
	}
	return Node{}, false
}

// Take unwraps a `NAME.Value` argument to its node, or reports a type error
// naming the builtin.
func (l *Lib) Take(fnName string, v interpreter.Value) (Node, error) {
	n, ok := l.AsNode(v)
	if !ok {
		return Node{}, fmt.Errorf("%s: argument must be a %s, got %s", fnName, l.TypeName(), v.Kind)
	}
	return n, nil
}

// ToNode converts a Jennifer value into a node so it can be stored or encoded:
// scalars map across, a struct becomes a map (a time.Time becomes a datetime
// where the library has one), `bytes` becomes a base64 string (or a bytes node
// with keepBytes, for an encoder that has a binary form), and the library's own
// Value contributes its node (shared, not copied - the tree is immutable).
// Any other kind, including another library's opaque value, is an error.
func (l *Lib) ToNode(v interpreter.Value, keepBytes bool) (Node, error) {
	switch v.Kind {
	case interpreter.KindNull:
		return Null(), nil
	case interpreter.KindBool:
		return Bool(v.Bool), nil
	case interpreter.KindInt:
		return Int(v.Int), nil
	case interpreter.KindFloat:
		return Float(v.Float), nil
	case interpreter.KindString:
		return String(v.Str), nil
	case interpreter.KindObject:
		if n, ok := l.AsNode(v); ok {
			return n, nil
		}
		return Node{}, fmt.Errorf("cannot convert an opaque %s.%s to a %s", v.StructNS, v.StructName, l.TypeName())
	case interpreter.KindBytes:
		if keepBytes {
			return Bytes(v.Bytes), nil
		}
		return String(base64.StdEncoding.EncodeToString(v.Bytes)), nil
	case interpreter.KindList:
		out := make([]Node, len(v.List))
		for i, e := range v.List {
			n, err := l.ToNode(e, keepBytes)
			if err != nil {
				return Node{}, err
			}
			out[i] = n
		}
		return List(out), nil
	case interpreter.KindMap:
		keys := make([]string, len(v.Map))
		elems := make([]Node, len(v.Map))
		for i, e := range v.Map {
			if e.Key.Kind != interpreter.KindString {
				return Node{}, fmt.Errorf("map key must be string, got %s", e.Key.Kind)
			}
			n, err := l.ToNode(e.Value, keepBytes)
			if err != nil {
				return Node{}, err
			}
			keys[i], elems[i] = e.Key.Str, n
		}
		return Map(keys, elems), nil
	case interpreter.KindStruct:
		if l.TimeNode != nil {
			if dt, ok := l.TimeNode(v); ok {
				return dt, nil
			}
		}
		keys := make([]string, len(v.Fields))
		elems := make([]Node, len(v.Fields))
		for i, f := range v.Fields {
			n, err := l.ToNode(f.Value, keepBytes)
			if err != nil {
				return Node{}, err
			}
			keys[i], elems[i] = f.Name, n
		}
		return Map(keys, elems), nil
	default:
		return Node{}, fmt.Errorf("cannot convert a %s value to a %s", v.Kind, l.TypeName())
	}
}

// TimeParts reads a `time.Time` struct's (nanos, offset), or reports that v is
// not one.
func TimeParts(v interpreter.Value) (nanos, offset int64, ok bool) {
	if v.Kind != interpreter.KindStruct || v.StructNS != "time" || v.StructName != "Time" {
		return 0, 0, false
	}
	for _, f := range v.Fields {
		switch f.Name {
		case "nanos":
			nanos = f.Value.Int
		case "offset":
			offset = f.Value.Int
		}
	}
	return nanos, offset, true
}

// TimeRFC3339 renders a `time.Time` struct as RFC 3339 text in its own offset.
func TimeRFC3339(v interpreter.Value) (string, bool) {
	nanos, offset, ok := TimeParts(v)
	if !ok {
		return "", false
	}
	loc := stdtime.FixedZone("", int(offset))
	return stdtime.Unix(0, nanos).In(loc).Format(stdtime.RFC3339Nano), true
}

// Install registers the `NAME.Value` type (its zero value is null) and the
// shared surface: the read accessors `typeOf get has keys length asInt asFloat
// asString asBool` (plus `isNull`, and `isDatetime` / `asDatetime` where the
// library has them) and the write verbs `map list set insert append remove
// move`.
func (l *Lib) Install(in *interpreter.Interpreter) {
	// The displayer covers a handle with no native payload (a wrapped Value,
	// in practice only a null an older zero path builds); a native payload
	// displays through payload.DisplayNative.
	in.RegisterNamespacedObject(l.Name, "Value", func(inner interpreter.Value) string {
		n, err := l.ToNode(inner, true)
		if err == nil {
			return (&payload{root: n, lib: l}).DisplayNative()
		}
		return "<" + l.TypeName() + ">"
	})
	in.SetNativeObjectZero(l.Name, "Value", &payload{root: Null(), lib: l})
	for _, b := range l.surface() {
		in.RegisterNamespaced(l.Name, b.name, l.builtin(b.name, b.fn))
	}
}

// accessor is one shared builtin before it is bound to its qualified name.
type accessor struct {
	name string
	fn   func(fnName string, args []interpreter.Value) (interpreter.Value, error)
}

// surface lists the shared builtins this library registers.
func (l *Lib) surface() []accessor {
	out := []accessor{
		{"typeOf", l.typeOf}, {"get", l.get}, {"has", l.has}, {"keys", l.keys},
		{"length", l.length}, {"asInt", l.asInt}, {"asFloat", l.asFloat},
		{"asString", l.asString}, {"asBool", l.asBool},
	}
	if l.HasNull {
		out = append(out, accessor{"isNull", l.isNull})
	}
	if l.TimeNode != nil {
		out = append(out, accessor{"isDatetime", l.isDatetime}, accessor{"asDatetime", l.asDatetime})
	}
	return append(out,
		accessor{"list", l.list}, accessor{"map", l.mapFn}, accessor{"set", l.set},
		accessor{"insert", l.insert}, accessor{"append", l.appendFn},
		accessor{"remove", l.remove}, accessor{"move", l.move})
}

func (l *Lib) builtin(name string, fn func(string, []interpreter.Value) (interpreter.Value, error)) interpreter.Builtin {
	fnName := l.Name + "." + name
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return fn(fnName, args)
	}
}

// Builtin returns one of the shared builtins by name, bound as Install
// registers it (nil for a name the library does not have) - for a library's
// own tests.
func (l *Lib) Builtin(name string) interpreter.Builtin {
	for _, b := range l.surface() {
		if b.name == name {
			return l.builtin(b.name, b.fn)
		}
	}
	return nil
}

// ----- read accessors ------------------------------------------------------
//
// Every accessor is (node, pointer)-shaped: an optional trailing JSON Pointer
// string, relative to the passed node, addressing exactly one sub-node (""
// or omitted means the node itself). `get` returns a child handle (so a walk
// stays opaque); the `as*` extractors return the leaf as an ordinary value.

// nodeAt unpacks (NAME.Value[, pointer]) and resolves the pointer.
func (l *Lib) nodeAt(fnName string, args []interpreter.Value) (Node, error) {
	if len(args) < 1 || len(args) > 2 {
		return Node{}, fmt.Errorf("%s expects 1 or 2 arguments (%s[, pointer]), got %d", fnName, l.TypeName(), len(args))
	}
	n, err := l.Take(fnName, args[0])
	if err != nil {
		return Node{}, err
	}
	ptr := ""
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindString {
			return Node{}, fmt.Errorf("%s: pointer must be string, got %s", fnName, args[1].Kind)
		}
		ptr = args[1].Str
	}
	return Resolve(fnName, n, ptr)
}

func (l *Lib) typeOf(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(n.TypeName()), nil
}

// get returns the addressed sub-node as a handle sharing the sub-tree.
func (l *Lib) get(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	return l.Wrap(n), nil
}

// has reports whether the pointer resolves. A malformed pointer is an error;
// a well-formed pointer that simply doesn't resolve is a plain false.
func (l *Lib) has(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (%s, pointer), got %d", fnName, l.TypeName(), len(args))
	}
	n, err := l.Take(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("%s: pointer must be string, got %s", fnName, args[1].Kind)
	}
	tokens, err := ParsePointer(fnName, args[1].Str)
	if err != nil {
		return interpreter.Null(), err
	}
	_, werr := Walk(fnName, n, tokens)
	return interpreter.BoolVal(werr == nil), nil
}

// keys lists the addressed map's keys, in document order.
func (l *Lib) keys(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	if n.k != KindMap {
		return interpreter.Null(), fmt.Errorf("%s: expected a map, got a %s", fnName, n.TypeName())
	}
	out := make([]interpreter.Value, n.Len())
	for i, k := range n.c.keys {
		out[i] = interpreter.StringVal(strings.Clone(k)) // not a slice of the input
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// length is a list's element count or a map's member count.
func (l *Lib) length(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	if !n.IsContainer() {
		return interpreter.Null(), fmt.Errorf("%s: expected a list or map, got a %s", fnName, n.TypeName())
	}
	return interpreter.IntVal(int64(n.Len())), nil
}

// asInt is strict: a float node is not an int.
func (l *Lib) asInt(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	return l.asKind(fnName, args, KindInt, "an int")
}

// asFloat promotes an int node, since the formats' numbers are one family. A
// non-finite float (TOML's `inf` / `nan`, YAML's `.inf` / `.nan`) has no
// Jennifer value, so it is an error rather than a NaN leaking into arithmetic;
// typeOf still reports it as a float, and it encodes back unchanged.
func (l *Lib) asFloat(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch n.k {
	case KindFloat:
		f := n.FloatVal()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return interpreter.Null(), fmt.Errorf("%s: node is a non-finite float (%v), which has no Jennifer value", fnName, f)
		}
		return interpreter.FloatVal(f), nil
	case KindInt:
		return interpreter.FloatVal(float64(n.IntVal())), nil
	}
	return interpreter.Null(), fmt.Errorf("%s: node is a %s, not a number", fnName, n.TypeName())
}

func (l *Lib) asString(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	return l.asKind(fnName, args, KindString, "a string")
}

func (l *Lib) asBool(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	return l.asKind(fnName, args, KindBool, "a bool")
}

// asKind resolves the pointer and returns the node's scalar value when it has
// kind k.
func (l *Lib) asKind(fnName string, args []interpreter.Value, k Kind, what string) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	if n.k != k {
		return interpreter.Null(), fmt.Errorf("%s: node is a %s, not %s", fnName, n.TypeName(), what)
	}
	return n.ScalarValue(), nil
}

func (l *Lib) isNull(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(n.k == KindNull), nil
}

func (l *Lib) isDatetime(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(n.k == KindDatetime), nil
}

// asDatetime converts a datetime node to a `time.Time` (nanoseconds since the
// Unix epoch plus a zone offset).
func (l *Lib) asDatetime(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, err := l.nodeAt(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	if n.k != KindDatetime {
		return interpreter.Null(), fmt.Errorf("%s: node is a %s, not a datetime", fnName, n.TypeName())
	}
	t, err := l.AsTime(n)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fnName, err)
	}
	_, offset := t.Zone()
	return interpreter.NamespacedStructVal("time", "Time", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(t.UnixNano())},
		{Name: "offset", Value: interpreter.IntVal(int64(offset))},
	}), nil
}

// ----- write verbs ---------------------------------------------------------

// list returns a fresh empty list, the starting point for building one.
func (l *Lib) list(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("%s expects 0 arguments, got %d", fnName, len(args))
	}
	return l.Wrap(List([]Node{})), nil
}

// mapFn returns a fresh empty map, the starting point for building one.
func (l *Lib) mapFn(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("%s expects 0 arguments, got %d", fnName, len(args))
	}
	return l.Wrap(Map(nil, nil)), nil
}

// set upserts a map key / replaces an in-range list index; "" replaces the
// whole document.
func (l *Lib) set(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, tokens, raw, err := l.writeArgs(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := Set(fnName, n, tokens, raw)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := CheckDepth(fnName, len(tokens), raw); err != nil {
		return interpreter.Null(), err
	}
	return l.Wrap(out), nil
}

// insert inserts into a list at an index or `-` (the end).
func (l *Lib) insert(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, tokens, raw, err := l.writeArgs(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := Insert(fnName, n, tokens, raw)
	if err != nil {
		return interpreter.Null(), err
	}
	if err := CheckDepth(fnName, len(tokens), raw); err != nil {
		return interpreter.Null(), err
	}
	return l.Wrap(out), nil
}

// appendFn pushes onto the list the pointer addresses (insert at ptr + "/-").
func (l *Lib) appendFn(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	n, tokens, raw, err := l.writeArgs(fnName, args)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := Insert(fnName, n, append(tokens, "-"), raw)
	if err != nil {
		return interpreter.Null(), err
	}
	// append targets the container at `tokens` and adds one level (the `-`
	// position), so the value lands at len(tokens)+1.
	if err := CheckDepth(fnName, len(tokens)+1, raw); err != nil {
		return interpreter.Null(), err
	}
	return l.Wrap(out), nil
}

// remove drops the addressed member / element.
func (l *Lib) remove(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (%s, pointer), got %d", fnName, l.TypeName(), len(args))
	}
	n, err := l.Take(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	tokens, err := pointerArg(fnName, args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	if len(tokens) == 0 {
		return interpreter.Null(), fmt.Errorf("%s: cannot remove the whole document", fnName)
	}
	out, err := Remove(fnName, n, tokens)
	if err != nil {
		return interpreter.Null(), err
	}
	return l.Wrap(out), nil
}

// move relocates the subtree at `from` to `to`: read `from`, remove it, then
// set `to` (upsert a map key / replace an in-range list index). To reorder
// within a list, use remove + insert.
func (l *Lib) move(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("%s expects 3 arguments (%s, from, to), got %d", fnName, l.TypeName(), len(args))
	}
	n, err := l.Take(fnName, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	fromTokens, err := pointerArg(fnName, args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	toTokens, err := pointerArg(fnName, args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fromTokens) == 0 {
		return interpreter.Null(), fmt.Errorf("%s: cannot move the whole document", fnName)
	}
	moved, err := Walk(fnName, n, fromTokens)
	if err != nil {
		return interpreter.Null(), err
	}
	removed, err := Remove(fnName, n, fromTokens)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := Set(fnName, removed, toTokens, moved)
	if err != nil {
		return interpreter.Null(), err
	}
	// Unlike set/insert/append (which add at most one level), move splices a
	// whole subtree under an arbitrary location, so the result can nest far
	// deeper than either operand - guard it too.
	if err := CheckDepth(fnName, len(toTokens), moved); err != nil {
		return interpreter.Null(), err
	}
	return l.Wrap(out), nil
}

// writeArgs unpacks the (NAME.Value, pointer, value) shape shared by set /
// insert / append, normalizing the value to a node.
func (l *Lib) writeArgs(fnName string, args []interpreter.Value) (Node, []string, Node, error) {
	if len(args) != 3 {
		return Node{}, nil, Node{}, fmt.Errorf("%s expects 3 arguments (%s, pointer, value), got %d", fnName, l.TypeName(), len(args))
	}
	n, err := l.Take(fnName, args[0])
	if err != nil {
		return Node{}, nil, Node{}, err
	}
	tokens, err := pointerArg(fnName, args[1])
	if err != nil {
		return Node{}, nil, Node{}, err
	}
	raw, err := l.ToNode(args[2], false)
	if err != nil {
		return Node{}, nil, Node{}, fmt.Errorf("%s: %v", fnName, err)
	}
	return n, tokens, raw, nil
}

// pointerArg validates a string pointer argument and parses it to tokens.
func pointerArg(fnName string, arg interpreter.Value) ([]string, error) {
	if arg.Kind != interpreter.KindString {
		return nil, fmt.Errorf("%s: pointer must be string, got %s", fnName, arg.Kind)
	}
	return ParsePointer(fnName, arg.Str)
}
