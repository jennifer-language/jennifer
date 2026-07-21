// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// json.Value accessors. `json.decode` returns an opaque `json.Value` (a
// KindObject wrapping the decoded tree); the functions here are the only way
// to reach inside it, since operators, `[index]`, and `.field` all reject an
// object.
//
// Every accessor is (node, pointer)-shaped: an optional trailing JSON Pointer
// (RFC 6901) string, relative to the passed node, addressing exactly one
// sub-node ("" or omitted means the node itself). This mirrors the write
// surface, which addresses by the same pointer. `get` returns a child
// json.Value (so a walk stays opaque); the `as*` extractors return the leaf as
// an ordinary Jennifer value. Node types are reported in Jennifer's
// vocabulary - `list` / `map`, never "array" / "object" (see docs/glossary.md).
package jsonlib

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// jsonNodeType maps a decoded node's Value kind to its type name, in the same
// vocabulary convert.typeOf uses (list / map, not array / object).
func jsonNodeType(v interpreter.Value) string {
	switch v.Kind {
	case interpreter.KindNull:
		return "null"
	case interpreter.KindBool:
		return "bool"
	case interpreter.KindInt:
		return "int"
	case interpreter.KindFloat:
		return "float"
	case interpreter.KindString:
		return "string"
	case interpreter.KindList:
		return "list"
	case interpreter.KindMap:
		return "map"
	}
	return "unknown"
}

// takeNode unwraps a json.Value argument to its inner decoded tree, or reports
// a positioned type error naming the accessor.
func takeNode(fnName string, v interpreter.Value) (interpreter.Value, error) {
	inner, ok := v.AsObject(LibraryName, "Value")
	if !ok {
		return interpreter.Value{}, fmt.Errorf("%s: argument must be a json.Value, got %s", fnName, v.Kind)
	}
	return inner, nil
}

// wrap re-wraps a child tree as a json.Value so a walk stays opaque.
func wrap(inner interpreter.Value) interpreter.Value {
	return interpreter.ObjectVal(LibraryName, "Value", inner)
}

// nodeAt is the shape shared by every accessor: args[0] is a json.Value, an
// optional args[1] is a JSON Pointer string (default ""), and the result is
// the inner sub-node the pointer addresses.
func nodeAt(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Value{}, fmt.Errorf("%s expects 1 or 2 arguments (json.Value[, pointer]), got %d", fnName, len(args))
	}
	node, err := takeNode(fnName, args[0])
	if err != nil {
		return interpreter.Value{}, err
	}
	ptr := ""
	if len(args) == 2 {
		if args[1].Kind != interpreter.KindString {
			return interpreter.Value{}, fmt.Errorf("%s: pointer must be string, got %s", fnName, args[1].Kind)
		}
		ptr = args[1].Str
	}
	return resolvePointer(fnName, node, ptr)
}

// parsePointer splits a JSON Pointer (RFC 6901) into its unescaped reference
// tokens. "" yields no tokens (the node itself). A non-empty pointer must
// start with '/'. Escapes: `~1` -> `/`, `~0` -> `~` (in that order).
func parsePointer(fnName, ptr string) ([]string, error) {
	if ptr == "" {
		return nil, nil
	}
	if ptr[0] != '/' {
		return nil, fmt.Errorf("%s: JSON pointer %q must be empty or start with '/'", fnName, ptr)
	}
	parts := strings.Split(ptr[1:], "/")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~1", "/")
		p = strings.ReplaceAll(p, "~0", "~")
		parts[i] = p
	}
	return parts, nil
}

// walkPointer resolves reference tokens against node, returning the addressed
// sub-node. A missing key, an out-of-range or malformed list index, or a
// descent into a scalar is a positioned error naming the failing token.
func walkPointer(fnName string, node interpreter.Value, tokens []string) (interpreter.Value, error) {
	cur := node
	for _, tok := range tokens {
		switch cur.Kind {
		case interpreter.KindMap:
			found := false
			for _, e := range cur.Map {
				if e.Key.Str == tok {
					cur = e.Value
					found = true
					break
				}
			}
			if !found {
				return interpreter.Value{}, fmt.Errorf("%s: no key %q", fnName, tok)
			}
		case interpreter.KindList:
			idx, ok := arrayIndex(tok)
			if !ok {
				return interpreter.Value{}, fmt.Errorf("%s: %q is not a valid list index", fnName, tok)
			}
			if idx >= len(cur.List) {
				return interpreter.Value{}, fmt.Errorf("%s: list index %d out of range [0, %d)", fnName, idx, len(cur.List))
			}
			cur = cur.List[idx]
		default:
			return interpreter.Value{}, fmt.Errorf("%s: cannot descend into %s at %q", fnName, jsonNodeType(cur), tok)
		}
	}
	return cur, nil
}

// arrayIndex parses a JSON Pointer list-index token: "0" or [1-9][0-9]* (no
// leading zeros, no sign, no `-` end-marker on the read side).
func arrayIndex(tok string) (int, bool) {
	if tok == "0" {
		return 0, true
	}
	if tok == "" || tok[0] < '1' || tok[0] > '9' {
		return 0, false
	}
	n := 0
	for i := 0; i < len(tok); i++ {
		if tok[i] < '0' || tok[i] > '9' {
			return 0, false
		}
		// Reject before the multiply/add can overflow int to a negative value
		// (a 19-20 digit token would otherwise wrap past a length check and
		// panic on a negative slice index).
		if n > (maxInt-9)/10 {
			return 0, false
		}
		n = n*10 + int(tok[i]-'0')
	}
	return n, true
}

// maxInt is the platform int max, used to reject overflowing pointer indices.
const maxInt = int(^uint(0) >> 1)

// resolvePointer parses ptr and walks it against node.
func resolvePointer(fnName string, node interpreter.Value, ptr string) (interpreter.Value, error) {
	tokens, err := parsePointer(fnName, ptr)
	if err != nil {
		return interpreter.Value{}, err
	}
	return walkPointer(fnName, node, tokens)
}

// typeOfFn implements json.typeOf(v[, pointer]) -> string: the JSON type of
// the addressed node (null / bool / int / float / string / list / map).
func typeOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.typeOf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(jsonNodeType(node)), nil
}

// getFn implements json.get(v[, pointer]) -> json.Value: the addressed
// sub-node, wrapped so a walk stays opaque. With no pointer, the node itself.
func getFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.get", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(node), nil
}

// hasFn implements json.has(v, pointer) -> bool: whether the pointer resolves
// to an existing node. A malformed pointer is an error; a well-formed pointer
// that simply doesn't resolve (missing key, out-of-range index, descent into a
// scalar) is a plain false.
func hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("json.has expects 2 arguments (json.Value, pointer), got %d", len(args))
	}
	node, err := takeNode("json.has", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("json.has: pointer must be string, got %s", args[1].Kind)
	}
	tokens, err := parsePointer("json.has", args[1].Str)
	if err != nil {
		return interpreter.Null(), err
	}
	_, werr := walkPointer("json.has", node, tokens)
	return interpreter.BoolVal(werr == nil), nil
}

// keysFn implements json.keys(v[, pointer]) -> list of string: the keys of the
// addressed map, in document order. Errors when the node is not a map.
func keysFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.keys", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindMap {
		return interpreter.Null(), fmt.Errorf("json.keys: expected a map, got a %s", jsonNodeType(node))
	}
	out := make([]interpreter.Value, len(node.Map))
	for i, e := range node.Map {
		out[i] = interpreter.StringVal(e.Key.Str)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// lengthFn implements json.length(v[, pointer]) -> int: element count of a
// list or entry count of a map. Errors on a scalar node.
func lengthFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.length", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch node.Kind {
	case interpreter.KindList:
		return interpreter.IntVal(int64(len(node.List))), nil
	case interpreter.KindMap:
		return interpreter.IntVal(int64(len(node.Map))), nil
	}
	return interpreter.Null(), fmt.Errorf("json.length: expected a list or map, got a %s", jsonNodeType(node))
}

// asIntFn implements json.asInt(v[, pointer]) -> int. Strict: a float node (a
// JSON number with a fractional part) is not an int and errors.
func asIntFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.asInt", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("json.asInt: node is a %s, not an int", jsonNodeType(node))
	}
	return node, nil
}

// asFloatFn implements json.asFloat(v[, pointer]) -> float. An integral JSON
// number (decoded as int) promotes to float, since JSON has one number type.
func asFloatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.asFloat", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch node.Kind {
	case interpreter.KindFloat:
		return node, nil
	case interpreter.KindInt:
		return interpreter.FloatVal(float64(node.Int)), nil
	}
	return interpreter.Null(), fmt.Errorf("json.asFloat: node is a %s, not a number", jsonNodeType(node))
}

// asStringFn implements json.asString(v[, pointer]) -> string.
func asStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.asString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("json.asString: node is a %s, not a string", jsonNodeType(node))
	}
	return node, nil
}

// asBoolFn implements json.asBool(v[, pointer]) -> bool.
func asBoolFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.asBool", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindBool {
		return interpreter.Null(), fmt.Errorf("json.asBool: node is a %s, not a bool", jsonNodeType(node))
	}
	return node, nil
}

// isNullFn implements json.isNull(v[, pointer]) -> bool: whether the addressed
// node is JSON null.
func isNullFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("json.isNull", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(node.Kind == interpreter.KindNull), nil
}
