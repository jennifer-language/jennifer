// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package yamllib implements the `yaml` system library: YAML 1.2 decode / encode
// onto the same opaque, library-owned Value that `json` and `toml` use.
// `yaml.decode(text)` returns a `yaml.Value` (a KindObject holding the shared
// compact document tree, internal/lib/doctree); the read accessors (`typeOf` /
// `get` / `has` / `keys` / `length` / `as*` / `isNull`, addressed by JSON
// Pointer) and the non-mutating write surface (`map` / `list` / `set` /
// `insert` / `append` / `remove` / `move`) are doctree's, the same as `json`'s
// and `toml`'s, so the config libraries read the same. `yaml.decodeAll`
// handles a multi-document stream. YAML's timestamp scalar is surfaced through
// `yaml.asDatetime` (a `time.Time`), the way `toml` surfaces its date-times.
//
// Unlike `json` / `toml` / `xml`, the parse is delegated to gopkg.in/yaml.v3:
// full YAML (anchors / aliases, flow and block styles, implicit typing,
// multi-document streams) is impractical to hand-roll and has no Go stdlib
// equivalent. The dependency is verified TinyGo-clean, so the library builds on
// both binaries (`jennifer` and `jennifer-tiny`).
package yamllib

import (
	"fmt"
	stdtime "time"

	"gopkg.in/yaml.v3"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
)

// LibraryName is the namespace prefix (`yaml.decode`, `yaml.get`, ...).
const LibraryName = "yaml"

// node is a yaml.Value tree node.
type node = doctree.Node

// lib is the yaml library's handle on the shared surface: YAML has null, and
// a stored time.Time becomes a timestamp.
var lib = &doctree.Lib{Name: LibraryName, HasNull: true, Display: display, TimeNode: timeNode, AsTime: asTime}

// display renders a yaml.Value in flow style, for `%v` and the REPL.
func display(n node) (string, error) {
	s, err := encodeNode(n, true)
	if err != nil {
		return "", err
	}
	// yaml.Marshal always terminates with a newline; trim it for a tidy
	// single-line REPL echo / %v rendering.
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s, nil
}

// timeNode stores a time.Time as a timestamp.
func timeNode(v interpreter.Value) (node, bool) {
	text, ok := doctree.TimeRFC3339(v)
	if !ok {
		return node{}, false
	}
	return doctree.Datetime(text, 0), true
}

// asTime reparses a timestamp's verbatim text through yaml.v3 (the same
// resolver that tagged it), so every YAML timestamp spelling is accepted.
func asTime(n node) (stdtime.Time, error) {
	var t stdtime.Time
	// Route through the same depth guard as decode. The text is a timestamp
	// scalar (no nesting), so this never rejects a real value; it just keeps
	// every yaml.Unmarshal call behind the guard.
	if err := guardDepth(n.Str()); err != nil {
		return t, err
	}
	err := yaml.Unmarshal([]byte(n.Str()), &t)
	return t, err
}

// Install registers the yaml library on in.
func Install(in *interpreter.Interpreter) {
	// The yaml.Value type, its accessors (with isNull, isDatetime /
	// asDatetime) and its write surface (internal/lib/doctree).
	lib.Install(in)

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
		return lib.Wrap(tree), nil
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
			out[i] = lib.Wrap(d)
		}
		return interpreter.ListVal(lib.ValueType(), out), nil
	})

	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})
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
