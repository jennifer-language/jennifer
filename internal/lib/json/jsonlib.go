// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package jsonlib is the `json` library: RFC 8259 encode / decode. Hand-rolled
// (no `encoding/json`, no `reflect`) to stay TinyGo-clean, the same reason the
// AST-JSON emitter is hand-rolled. `decode` returns an opaque `json.Value` (a
// KindObject wrapping the decoded tree: objects become `map of string to V`,
// numbers become int when integral else float); the accessors in
// jsonaccess.go walk it, and there is no map-to-struct coercion - a typed
// target is rebuilt explicitly from the decoded value (see the library doc,
// and docs/technical/rejected.md). `encode` accepts ordinary typed values and
// json.Values alike (a decode/encode round-trip).
package jsonlib

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the namespace prefix (`json.`) and the `use` name.
const LibraryName = "json"

// Install registers the json surface. `decode` returns an opaque
// `json.Value` (a KindObject) that callers walk with the accessors below;
// `encode` accepts ordinary typed values and json.Values alike.
func Install(in *interpreter.Interpreter) {
	// A json.Value displays as its compact JSON, so `$v` at the REPL and
	// `%v` show the document rather than an opaque `<json.Value>`.
	in.RegisterNamespacedObject(LibraryName, "Value", func(inner interpreter.Value) string {
		var sb strings.Builder
		if err := encodeValue(&sb, inner, false, 0); err != nil {
			return "<json.Value>"
		}
		return sb.String()
	})

	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})
	in.RegisterNamespaced(LibraryName, "decode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		tree, err := decodeFn(args)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.ObjectVal(LibraryName, "Value", tree), nil
	})

	// json.Value accessors (jsonaccess.go).
	in.RegisterNamespaced(LibraryName, "typeOf", typeOfFn)
	in.RegisterNamespaced(LibraryName, "get", getFn)
	in.RegisterNamespaced(LibraryName, "has", hasFn)
	in.RegisterNamespaced(LibraryName, "keys", keysFn)
	in.RegisterNamespaced(LibraryName, "length", lengthFn)
	in.RegisterNamespaced(LibraryName, "asInt", asIntFn)
	in.RegisterNamespaced(LibraryName, "asFloat", asFloatFn)
	in.RegisterNamespaced(LibraryName, "asString", asStringFn)
	in.RegisterNamespaced(LibraryName, "asBool", asBoolFn)
	in.RegisterNamespaced(LibraryName, "isNull", isNullFn)

	// json.Value write surface (jsonwrite.go) - all non-mutating, returning
	// a fresh handle; the idiom is `$v = json.set($v, ...)`.
	in.RegisterNamespaced(LibraryName, "list", listFn)
	in.RegisterNamespaced(LibraryName, "map", mapFn)
	in.RegisterNamespaced(LibraryName, "set", setFn)
	in.RegisterNamespaced(LibraryName, "insert", insertFn)
	in.RegisterNamespaced(LibraryName, "append", appendFn)
	in.RegisterNamespaced(LibraryName, "remove", removeFn)
	in.RegisterNamespaced(LibraryName, "move", moveFn)
}

func encodeFn(args []interpreter.Value, pretty bool) (interpreter.Value, error) {
	verb := "json.encode"
	if pretty {
		verb = "json.encodePretty"
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (value), got %d", verb, len(args))
	}
	var sb strings.Builder
	if err := encodeValue(&sb, args[0], pretty, 0); err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", verb, err)
	}
	return interpreter.StringVal(sb.String()), nil
}

// encodeValue writes v's JSON image to sb. depth drives the pretty indent.
func encodeValue(sb *strings.Builder, v interpreter.Value, pretty bool, depth int) error {
	switch v.Kind {
	case interpreter.KindNull:
		sb.WriteString("null")
	case interpreter.KindBool:
		if v.Bool {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case interpreter.KindInt:
		sb.WriteString(strconv.FormatInt(v.Int, 10))
	case interpreter.KindFloat:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			return fmt.Errorf("cannot encode non-finite float")
		}
		s := strconv.FormatFloat(v.Float, 'g', -1, 64)
		// Keep the float kind round-trippable: a bare "1" would decode back as
		// int, so give a float without a fractional/exponent part one.
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		sb.WriteString(s)
	case interpreter.KindString:
		encodeString(sb, v.Str)
	case interpreter.KindBytes:
		encodeString(sb, base64.StdEncoding.EncodeToString(v.Bytes))
	case interpreter.KindList:
		return encodeSeq(sb, len(v.List), pretty, depth, '[', ']', func(i int) error {
			return encodeValue(sb, v.List[i], pretty, depth+1)
		})
	case interpreter.KindMap:
		for _, e := range v.Map {
			if e.Key.Kind != interpreter.KindString {
				return fmt.Errorf("map key must be string, got %s", e.Key.Kind)
			}
		}
		return encodeSeq(sb, len(v.Map), pretty, depth, '{', '}', func(i int) error {
			encodeString(sb, v.Map[i].Key.Str)
			sb.WriteString(colon(pretty))
			return encodeValue(sb, v.Map[i].Value, pretty, depth+1)
		})
	case interpreter.KindStruct:
		return encodeSeq(sb, len(v.Fields), pretty, depth, '{', '}', func(i int) error {
			encodeString(sb, v.Fields[i].Name)
			sb.WriteString(colon(pretty))
			return encodeValue(sb, v.Fields[i].Value, pretty, depth+1)
		})
	case interpreter.KindObject:
		// a json.Value round-trips: unwrap it and encode its inner tree.
		if inner, ok := v.AsObject(LibraryName, "Value"); ok {
			return encodeValue(sb, inner, pretty, depth)
		}
		return fmt.Errorf("cannot encode an opaque %s.%s value", v.StructNS, v.StructName)
	case interpreter.KindTask:
		return fmt.Errorf("cannot encode a task value")
	default:
		return fmt.Errorf("cannot encode value of kind %s", v.Kind)
	}
	return nil
}

// encodeSeq writes an array or object body, calling item(i) for each element.
// item is responsible for writing the element (and, for objects, its key and
// colon). Empty sequences collapse to `[]` / `{}` even when pretty.
func encodeSeq(sb *strings.Builder, n int, pretty bool, depth int, open, close byte, item func(int) error) error {
	sb.WriteByte(open)
	if n == 0 {
		sb.WriteByte(close)
		return nil
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		newlineIndent(sb, pretty, depth+1)
		if err := item(i); err != nil {
			return err
		}
	}
	newlineIndent(sb, pretty, depth)
	sb.WriteByte(close)
	return nil
}

func colon(pretty bool) string {
	if pretty {
		return ": "
	}
	return ":"
}

// newlineIndent writes a newline plus 2*depth spaces when pretty; nothing
// otherwise.
func newlineIndent(sb *strings.Builder, pretty bool, depth int) {
	if !pretty {
		return
	}
	sb.WriteByte('\n')
	for i := 0; i < depth*2; i++ {
		sb.WriteByte(' ')
	}
}

// encodeString writes s as a quoted, escaped JSON string. Input is valid
// UTF-8 (a Go string) and emitted as-is except for the mandatory escapes and
// control characters below U+0020.
func encodeString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(sb, `\u%04x`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}
