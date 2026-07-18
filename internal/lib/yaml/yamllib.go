// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package yamllib implements the `yaml` system library: YAML 1.2 decode / encode
// onto the same opaque, library-owned Value that `json` and `toml` use.
// `yaml.decode(text)` returns a `yaml.Value` (a KindObject wrapping the decoded
// tree); the read accessors (`typeOf` / `get` / `has` / `keys` / `length` /
// `as*` / `isNull`, addressed by JSON Pointer) and the non-mutating write surface
// (`map` / `list` / `set` / `insert` / `append` / `remove` / `move`) are
// deliberately the same shape as `json`'s and `toml`'s, so the config libraries
// read the same. `yaml.decodeAll` handles a multi-document stream. YAML's
// timestamp scalar is surfaced through `yaml.asDatetime` (a `time.Time`), the way
// `toml` surfaces its date-times.
//
// Unlike `json` / `toml` / `xml`, the parse is delegated to gopkg.in/yaml.v3:
// full YAML (anchors / aliases, flow and block styles, implicit typing,
// multi-document streams) is impractical to hand-roll and has no Go stdlib
// equivalent. The dependency is verified TinyGo-clean, so the library builds on
// both binaries (`jennifer` and `jennifer-tiny`).
package yamllib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`yaml.decode`, `yaml.get`, ...).
const LibraryName = "yaml"

// Install registers the yaml library on in.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedObject(LibraryName, "Value", func(inner interpreter.Value) string {
		s, err := encodeYaml(inner, true)
		if err != nil {
			return "<yaml.Value>"
		}
		// yaml.Marshal always terminates with a newline; trim it for a tidy
		// single-line REPL echo / %v rendering.
		for len(s) > 0 && s[len(s)-1] == '\n' {
			s = s[:len(s)-1]
		}
		return s
	})

	in.RegisterNamespaced(LibraryName, "decode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("yaml.decode expects 1 argument (text), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("yaml.decode: argument must be string, got %s", args[0].Kind)
		}
		tree, err := decodeYaml(args[0].Str)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.ObjectVal(LibraryName, "Value", tree), nil
	})

	in.RegisterNamespaced(LibraryName, "decodeAll", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("yaml.decodeAll expects 1 argument (text), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("yaml.decodeAll: argument must be string, got %s", args[0].Kind)
		}
		docs, err := decodeAllYaml(args[0].Str)
		if err != nil {
			return interpreter.Null(), err
		}
		out := make([]interpreter.Value, len(docs))
		for i, d := range docs {
			out[i] = interpreter.ObjectVal(LibraryName, "Value", d)
		}
		return interpreter.ListVal(valueType(), out), nil
	})

	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})

	// Read accessors (JSON Pointer shape, mirroring json / toml).
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
	in.RegisterNamespaced(LibraryName, "asDatetime", asDatetimeFn)
	in.RegisterNamespaced(LibraryName, "isDatetime", isDatetimeFn)

	// Write surface (non-mutating; fresh handle each call, mirroring json / toml).
	in.RegisterNamespaced(LibraryName, "list", listFn)
	in.RegisterNamespaced(LibraryName, "map", mapFn)
	in.RegisterNamespaced(LibraryName, "set", setFn)
	in.RegisterNamespaced(LibraryName, "insert", insertFn)
	in.RegisterNamespaced(LibraryName, "append", appendFn)
	in.RegisterNamespaced(LibraryName, "remove", removeFn)
	in.RegisterNamespaced(LibraryName, "move", moveFn)
}

// valueType is the parser type of a yaml.Value, used to type the list
// yaml.decodeAll returns.
func valueType() parser.Type {
	return parser.Type{Kind: parser.TypeStruct, StructNS: LibraryName, StructName: "Value"}
}

// encodeFn renders args[0] to YAML text (flow = compact vs block = pretty).
func encodeFn(args []interpreter.Value, flow bool) (interpreter.Value, error) {
	verb := "yaml.encode"
	if !flow {
		verb = "yaml.encodePretty"
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (value), got %d", verb, len(args))
	}
	s, err := encodeYaml(args[0], flow)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", verb, err)
	}
	return interpreter.StringVal(s), nil
}
