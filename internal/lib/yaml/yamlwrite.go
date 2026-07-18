// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// yaml.Value write surface. Every verb is non-mutating: it returns a fresh
// yaml.Value with the edit applied, leaving the input untouched, so the idiom is
// `$v = yaml.set($v, ...)` - the same shape json / toml use. Locations are
// addressed by JSON Pointer (RFC 6901). Writes are strict (no auto-vivification):
// `set` creates only the final pointer segment. Start a document from
// `yaml.map()` / `yaml.list()` and grow it a level at a time. A `time.Time`
// stored into the tree becomes a YAML timestamp.
package yamllib

import (
	"encoding/base64"
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// ----- constructors --------------------------------------------------------

func listFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("yaml.list expects 0 arguments, got %d", len(args))
	}
	return wrap(listVal([]interpreter.Value{})), nil
}

func mapFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("yaml.map expects 0 arguments, got %d", len(args))
	}
	return wrap(mapVal(nil)), nil
}

// ----- value normalization -------------------------------------------------

// toNode converts a Jennifer value into a tree node: scalars pass through, a
// struct becomes a map (a time.Time becomes a timestamp node), `bytes` becomes a
// base64 string, and a yaml.Value is unwrapped.
func toNode(v interpreter.Value) (interpreter.Value, error) {
	switch v.Kind {
	case interpreter.KindNull, interpreter.KindBool, interpreter.KindInt,
		interpreter.KindFloat, interpreter.KindString:
		return v, nil
	case interpreter.KindObject:
		if inner, ok := v.AsObject(LibraryName, "Value"); ok {
			return inner.DeepCopy(), nil
		}
		return interpreter.Value{}, fmt.Errorf("cannot store an opaque %s.%s in a yaml.Value", v.StructNS, v.StructName)
	case interpreter.KindBytes:
		return interpreter.StringVal(base64.StdEncoding.EncodeToString(v.Bytes)), nil
	case interpreter.KindList:
		out := make([]interpreter.Value, len(v.List))
		for i, e := range v.List {
			n, err := toNode(e)
			if err != nil {
				return interpreter.Value{}, err
			}
			out[i] = n
		}
		return listVal(out), nil
	case interpreter.KindMap:
		entries := make([]interpreter.MapEntry, len(v.Map))
		for i, e := range v.Map {
			if e.Key.Kind != interpreter.KindString {
				return interpreter.Value{}, fmt.Errorf("map key must be string, got %s", e.Key.Kind)
			}
			n, err := toNode(e.Value)
			if err != nil {
				return interpreter.Value{}, err
			}
			entries[i] = interpreter.MapEntry{Key: e.Key, Value: n}
		}
		return mapVal(entries), nil
	case interpreter.KindStruct:
		if _, ok := isDatetimeNode(v); ok {
			return v, nil // already a timestamp node
		}
		if dt, ok := timeToDatetimeNode(v); ok {
			return dt, nil
		}
		entries := make([]interpreter.MapEntry, len(v.Fields))
		for i, f := range v.Fields {
			n, err := toNode(f.Value)
			if err != nil {
				return interpreter.Value{}, err
			}
			entries[i] = interpreter.MapEntry{Key: interpreter.StringVal(f.Name), Value: n}
		}
		return mapVal(entries), nil
	default:
		return interpreter.Value{}, fmt.Errorf("cannot store a %s in a yaml.Value", v.Kind)
	}
}

// timeToDatetimeNode turns a `time.Time` struct into an internal timestamp node.
func timeToDatetimeNode(v interpreter.Value) (interpreter.Value, bool) {
	text, ok := timeToTimestamp(v)
	if !ok {
		return interpreter.Value{}, false
	}
	return datetimeVal(text), true
}

// ----- copy-on-write container helpers -------------------------------------

func mapGet(m interpreter.Value, key string) (interpreter.Value, bool) {
	for _, e := range m.Map {
		if e.Key.Str == key {
			return e.Value, true
		}
	}
	return interpreter.Value{}, false
}

func mapReplace(m interpreter.Value, key string, val interpreter.Value) interpreter.Value {
	entries := make([]interpreter.MapEntry, len(m.Map))
	copy(entries, m.Map)
	for i := range entries {
		if entries[i].Key.Str == key {
			entries[i].Value = val
			break
		}
	}
	return mapVal(entries)
}

func mapUpsert(m interpreter.Value, key string, val interpreter.Value) interpreter.Value {
	entries := make([]interpreter.MapEntry, len(m.Map))
	copy(entries, m.Map)
	for i := range entries {
		if entries[i].Key.Str == key {
			entries[i].Value = val
			return mapVal(entries)
		}
	}
	return mapVal(append(entries, interpreter.MapEntry{Key: interpreter.StringVal(key), Value: val}))
}

func mapRemove(m interpreter.Value, key string) (interpreter.Value, bool) {
	out := make([]interpreter.MapEntry, 0, len(m.Map))
	found := false
	for _, e := range m.Map {
		if e.Key.Str == key {
			found = true
			continue
		}
		out = append(out, e)
	}
	return mapVal(out), found
}

func listReplace(l interpreter.Value, idx int, val interpreter.Value) interpreter.Value {
	out := make([]interpreter.Value, len(l.List))
	copy(out, l.List)
	out[idx] = val
	return listVal(out)
}

func listInsert(l interpreter.Value, idx int, val interpreter.Value) interpreter.Value {
	out := make([]interpreter.Value, 0, len(l.List)+1)
	out = append(out, l.List[:idx]...)
	out = append(out, val)
	out = append(out, l.List[idx:]...)
	return listVal(out)
}

func listRemove(l interpreter.Value, idx int) interpreter.Value {
	out := make([]interpreter.Value, 0, len(l.List)-1)
	out = append(out, l.List[:idx]...)
	out = append(out, l.List[idx+1:]...)
	return listVal(out)
}

// ----- spine rebuild -------------------------------------------------------

func editAt(fnName string, node interpreter.Value, tokens []string, op func(container interpreter.Value, seg string) (interpreter.Value, error)) (interpreter.Value, error) {
	if len(tokens) == 1 {
		return op(node, tokens[0])
	}
	seg := tokens[0]
	switch node.Kind {
	case interpreter.KindMap:
		child, ok := mapGet(node, seg)
		if !ok {
			return interpreter.Value{}, fmt.Errorf("%s: no key %q", fnName, seg)
		}
		nc, err := editAt(fnName, child, tokens[1:], op)
		if err != nil {
			return interpreter.Value{}, err
		}
		return mapReplace(node, seg, nc), nil
	case interpreter.KindList:
		idx, ok := arrayIndex(seg)
		if !ok {
			return interpreter.Value{}, fmt.Errorf("%s: %q is not a valid list index", fnName, seg)
		}
		if idx >= len(node.List) {
			return interpreter.Value{}, fmt.Errorf("%s: list index %d out of range [0, %d)", fnName, idx, len(node.List))
		}
		nc, err := editAt(fnName, node.List[idx], tokens[1:], op)
		if err != nil {
			return interpreter.Value{}, err
		}
		return listReplace(node, idx, nc), nil
	default:
		return interpreter.Value{}, fmt.Errorf("%s: cannot descend into %s at %q", fnName, yamlNodeType(node), seg)
	}
}

func setInto(fnName string, node interpreter.Value, tokens []string, raw interpreter.Value) (interpreter.Value, error) {
	if len(tokens) == 0 {
		return raw, nil
	}
	return editAt(fnName, node, tokens, func(c interpreter.Value, seg string) (interpreter.Value, error) {
		switch c.Kind {
		case interpreter.KindMap:
			return mapUpsert(c, seg, raw), nil
		case interpreter.KindList:
			idx, ok := arrayIndex(seg)
			if !ok {
				return interpreter.Value{}, fmt.Errorf("%s: %q is not a valid list index (use append / insert to grow a list)", fnName, seg)
			}
			if idx >= len(c.List) {
				return interpreter.Value{}, fmt.Errorf("%s: list index %d out of range [0, %d); use append / insert to grow", fnName, idx, len(c.List))
			}
			return listReplace(c, idx, raw), nil
		default:
			return interpreter.Value{}, fmt.Errorf("%s: cannot set a member of %s", fnName, yamlNodeType(c))
		}
	})
}

func insertInto(fnName string, node interpreter.Value, tokens []string, raw interpreter.Value) (interpreter.Value, error) {
	if len(tokens) == 0 {
		return interpreter.Value{}, fmt.Errorf("%s: pointer must end in a list index (or `-`)", fnName)
	}
	return editAt(fnName, node, tokens, func(c interpreter.Value, seg string) (interpreter.Value, error) {
		if c.Kind != interpreter.KindList {
			return interpreter.Value{}, fmt.Errorf("%s: expected a list, got a %s", fnName, yamlNodeType(c))
		}
		idx := len(c.List)
		if seg != "-" {
			i, ok := arrayIndex(seg)
			if !ok {
				return interpreter.Value{}, fmt.Errorf("%s: %q is not a valid list index", fnName, seg)
			}
			if i > len(c.List) {
				return interpreter.Value{}, fmt.Errorf("%s: list index %d out of range [0, %d]", fnName, i, len(c.List))
			}
			idx = i
		}
		return listInsert(c, idx, raw), nil
	})
}

// ----- verbs ---------------------------------------------------------------

func setFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, tokens, raw, err := writeArgs("yaml.set", args)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := setInto("yaml.set", node, tokens, raw)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(out), nil
}

func insertFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, tokens, raw, err := writeArgs("yaml.insert", args)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := insertInto("yaml.insert", node, tokens, raw)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(out), nil
}

func appendFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, tokens, raw, err := writeArgs("yaml.append", args)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := insertInto("yaml.append", node, append(tokens, "-"), raw)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(out), nil
}

func removeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("yaml.remove expects 2 arguments (yaml.Value, pointer), got %d", len(args))
	}
	node, err := takeNode("yaml.remove", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	tokens, err := pointerArg("yaml.remove", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	if len(tokens) == 0 {
		return interpreter.Null(), fmt.Errorf("yaml.remove: cannot remove the whole document")
	}
	out, err := removeFromTree("yaml.remove", node, tokens)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(out), nil
}

func moveFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("yaml.move expects 3 arguments (yaml.Value, from, to), got %d", len(args))
	}
	node, err := takeNode("yaml.move", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	fromTokens, err := pointerArg("yaml.move", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	toTokens, err := pointerArg("yaml.move", args[2])
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fromTokens) == 0 {
		return interpreter.Null(), fmt.Errorf("yaml.move: cannot move the whole document")
	}
	moved, err := walkPointer("yaml.move", node, fromTokens)
	if err != nil {
		return interpreter.Null(), err
	}
	moved = moved.DeepCopy()
	removed, err := removeFromTree("yaml.move", node, fromTokens)
	if err != nil {
		return interpreter.Null(), err
	}
	out, err := setInto("yaml.move", removed, toTokens, moved)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(out), nil
}

func removeFromTree(fnName string, node interpreter.Value, tokens []string) (interpreter.Value, error) {
	return editAt(fnName, node, tokens, func(c interpreter.Value, seg string) (interpreter.Value, error) {
		switch c.Kind {
		case interpreter.KindMap:
			m, ok := mapRemove(c, seg)
			if !ok {
				return interpreter.Value{}, fmt.Errorf("%s: no key %q", fnName, seg)
			}
			return m, nil
		case interpreter.KindList:
			idx, ok := arrayIndex(seg)
			if !ok {
				return interpreter.Value{}, fmt.Errorf("%s: %q is not a valid list index", fnName, seg)
			}
			if idx >= len(c.List) {
				return interpreter.Value{}, fmt.Errorf("%s: list index %d out of range [0, %d)", fnName, idx, len(c.List))
			}
			return listRemove(c, idx), nil
		default:
			return interpreter.Value{}, fmt.Errorf("%s: cannot remove a member of %s", fnName, yamlNodeType(c))
		}
	})
}

// ----- argument helpers ----------------------------------------------------

func writeArgs(fnName string, args []interpreter.Value) (interpreter.Value, []string, interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Value{}, nil, interpreter.Value{}, fmt.Errorf("%s expects 3 arguments (yaml.Value, pointer, value), got %d", fnName, len(args))
	}
	node, err := takeNode(fnName, args[0])
	if err != nil {
		return interpreter.Value{}, nil, interpreter.Value{}, err
	}
	tokens, err := pointerArg(fnName, args[1])
	if err != nil {
		return interpreter.Value{}, nil, interpreter.Value{}, err
	}
	raw, err := toNode(args[2])
	if err != nil {
		return interpreter.Value{}, nil, interpreter.Value{}, fmt.Errorf("%s: %v", fnName, err)
	}
	return node, tokens, raw, nil
}

func pointerArg(fnName string, arg interpreter.Value) ([]string, error) {
	if arg.Kind != interpreter.KindString {
		return nil, fmt.Errorf("%s: pointer must be string, got %s", fnName, arg.Kind)
	}
	return parsePointer(fnName, arg.Str)
}
