// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// toml.Value read accessors. `toml.decode` returns an opaque toml.Value (a
// KindObject wrapping the decoded tree); these functions are the only way to
// reach inside it. Every accessor is (node, pointer)-shaped: an optional
// trailing JSON Pointer (RFC 6901) string addresses one sub-node, exactly the
// json accessor shape - TOML has no pointer syntax of its own, and a JSON
// Pointer sidesteps the dotted-path ambiguity a key containing '.' would create.
// Node types are reported in Jennifer's vocabulary (list / map, not
// array / table), plus `datetime` for the one type TOML adds over JSON.
package tomllib

import (
	"fmt"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// tomlNodeType maps a tree node to its type name (list / map / datetime plus the
// scalar kinds).
func tomlNodeType(v interpreter.Value) string {
	if _, _, ok := isDatetimeNode(v); ok {
		return "datetime"
	}
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

func takeNode(fnName string, v interpreter.Value) (interpreter.Value, error) {
	inner, ok := v.AsObject(LibraryName, "Value")
	if !ok {
		return interpreter.Value{}, fmt.Errorf("%s: argument must be a toml.Value, got %s", fnName, v.Kind)
	}
	return inner, nil
}

func wrap(inner interpreter.Value) interpreter.Value {
	return interpreter.ObjectVal(LibraryName, "Value", inner)
}

func nodeAt(fnName string, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Value{}, fmt.Errorf("%s expects 1 or 2 arguments (toml.Value[, pointer]), got %d", fnName, len(args))
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
			return interpreter.Value{}, fmt.Errorf("%s: cannot descend into %s at %q", fnName, tomlNodeType(cur), tok)
		}
	}
	return cur, nil
}

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
		// Reject before overflow wraps int negative and panics a slice index.
		if n > (maxInt-9)/10 {
			return 0, false
		}
		n = n*10 + int(tok[i]-'0')
	}
	return n, true
}

// maxInt is the platform int max, used to reject overflowing pointer indices.
const maxInt = int(^uint(0) >> 1)

func resolvePointer(fnName string, node interpreter.Value, ptr string) (interpreter.Value, error) {
	tokens, err := parsePointer(fnName, ptr)
	if err != nil {
		return interpreter.Value{}, err
	}
	return walkPointer(fnName, node, tokens)
}

func typeOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.typeOf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.StringVal(tomlNodeType(node)), nil
}

func getFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.get", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return wrap(node), nil
}

func hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("toml.has expects 2 arguments (toml.Value, pointer), got %d", len(args))
	}
	node, err := takeNode("toml.has", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("toml.has: pointer must be string, got %s", args[1].Kind)
	}
	tokens, err := parsePointer("toml.has", args[1].Str)
	if err != nil {
		return interpreter.Null(), err
	}
	_, werr := walkPointer("toml.has", node, tokens)
	return interpreter.BoolVal(werr == nil), nil
}

func keysFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.keys", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindMap {
		return interpreter.Null(), fmt.Errorf("toml.keys: expected a map, got a %s", tomlNodeType(node))
	}
	out := make([]interpreter.Value, len(node.Map))
	for i, e := range node.Map {
		out[i] = interpreter.StringVal(e.Key.Str)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

func lengthFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.length", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch node.Kind {
	case interpreter.KindList:
		return interpreter.IntVal(int64(len(node.List))), nil
	case interpreter.KindMap:
		return interpreter.IntVal(int64(len(node.Map))), nil
	}
	return interpreter.Null(), fmt.Errorf("toml.length: expected a list or map, got a %s", tomlNodeType(node))
}

func asIntFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.asInt", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("toml.asInt: node is a %s, not an int", tomlNodeType(node))
	}
	return node, nil
}

func asFloatFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.asFloat", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch node.Kind {
	case interpreter.KindFloat:
		return node, nil
	case interpreter.KindInt:
		return interpreter.FloatVal(float64(node.Int)), nil
	}
	return interpreter.Null(), fmt.Errorf("toml.asFloat: node is a %s, not a number", tomlNodeType(node))
}

func asStringFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.asString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("toml.asString: node is a %s, not a string", tomlNodeType(node))
	}
	return node, nil
}

func asBoolFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.asBool", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if node.Kind != interpreter.KindBool {
		return interpreter.Null(), fmt.Errorf("toml.asBool: node is a %s, not a bool", tomlNodeType(node))
	}
	return node, nil
}

// isDatetimeFn implements toml.isDatetime(v[, pointer]) -> bool.
func isDatetimeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.isDatetime", args)
	if err != nil {
		return interpreter.Null(), err
	}
	_, _, ok := isDatetimeNode(node)
	return interpreter.BoolVal(ok), nil
}

// asDatetimeFn implements toml.asDatetime(v[, pointer]) -> time.Time. It turns
// any of the four TOML date-time forms into a time.Time (nanoseconds since the
// Unix epoch plus a zone offset). A local date is taken at midnight UTC; a local
// time is placed on the zero date; a local datetime carries a zero offset.
func asDatetimeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, err := nodeAt("toml.asDatetime", args)
	if err != nil {
		return interpreter.Null(), err
	}
	text, form, ok := isDatetimeNode(node)
	if !ok {
		return interpreter.Null(), fmt.Errorf("toml.asDatetime: node is a %s, not a datetime", tomlNodeType(node))
	}
	t, err := parseDatetimeText(text, form)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("toml.asDatetime: %v", err)
	}
	_, offset := t.Zone()
	return interpreter.NamespacedStructVal("time", "Time", []interpreter.StructField{
		{Name: "nanos", Value: interpreter.IntVal(t.UnixNano())},
		{Name: "offset", Value: interpreter.IntVal(int64(offset))},
	}), nil
}

// parseDatetimeText parses stored RFC 3339 text into a Go time.Time, choosing
// the layout by the recorded form. Zone-less forms parse as UTC.
func parseDatetimeText(text, form string) (stdtime.Time, error) {
	switch form {
	case formOffsetDatetime:
		return stdtime.Parse(stdtime.RFC3339Nano, text)
	case formLocalDatetime:
		return stdtime.Parse("2006-01-02T15:04:05.999999999", text)
	case formLocalDate:
		return stdtime.Parse("2006-01-02", text)
	case formLocalTime:
		t, err := stdtime.Parse("15:04:05.999999999", text)
		if err != nil {
			return stdtime.Time{}, err
		}
		// A time-only layout parses onto year 0, whose UnixNano overflows int64
		// (~-6.2e19). Anchor to the Unix epoch date so the clock is preserved
		// and the stored nanos are representable.
		return stdtime.Date(1970, 1, 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), stdtime.UTC), nil
	default:
		return stdtime.Time{}, fmt.Errorf("unknown datetime form %q", form)
	}
}
