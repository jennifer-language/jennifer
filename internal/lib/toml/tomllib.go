// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package tomllib implements the `toml` system library: RFC-conformant TOML
// 1.0.0 decode / encode onto the same opaque, library-owned Value that `json`
// uses. `toml.decode(text)` returns a `toml.Value` (a KindObject wrapping the
// decoded table tree); the read accessors (`typeOf` / `get` / `has` / `keys` /
// `length` / `as*`, addressed by JSON Pointer) and the non-mutating write
// surface (`map` / `list` / `set` / `insert` / `append` / `remove` / `move`)
// are deliberately the same shape as `json`'s, so the two libraries read the
// same. The one thing TOML has that JSON does not - the four date-time forms -
// is carried as an internal node and surfaced through `toml.asDatetime`, which
// returns a `time.Time`.
package tomllib

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// LibraryName is the namespace prefix (`toml.decode`, `toml.get`, ...).
const LibraryName = "toml"

func inf(neg bool) float64 {
	if neg {
		return math.Inf(-1)
	}
	return math.Inf(1)
}

func nan() float64 { return math.NaN() }

// Install registers the toml library on in.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedObject(LibraryName, "Value", func(inner interpreter.Value) string {
		var sb strings.Builder
		if err := encodeInline(&sb, inner); err != nil {
			return "<toml.Value>"
		}
		return sb.String()
	})

	in.RegisterNamespaced(LibraryName, "decode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("toml.decode expects 1 argument (text), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("toml.decode: argument must be string, got %s", args[0].Kind)
		}
		tree, err := decodeToml(args[0].Str)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.ObjectVal(LibraryName, "Value", tree), nil
	})

	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})

	// Read accessors (JSON Pointer shape, mirroring json).
	in.RegisterNamespaced(LibraryName, "typeOf", typeOfFn)
	in.RegisterNamespaced(LibraryName, "get", getFn)
	in.RegisterNamespaced(LibraryName, "has", hasFn)
	in.RegisterNamespaced(LibraryName, "keys", keysFn)
	in.RegisterNamespaced(LibraryName, "length", lengthFn)
	in.RegisterNamespaced(LibraryName, "asInt", asIntFn)
	in.RegisterNamespaced(LibraryName, "asFloat", asFloatFn)
	in.RegisterNamespaced(LibraryName, "asString", asStringFn)
	in.RegisterNamespaced(LibraryName, "asBool", asBoolFn)
	in.RegisterNamespaced(LibraryName, "asDatetime", asDatetimeFn)
	in.RegisterNamespaced(LibraryName, "isDatetime", isDatetimeFn)

	// Write surface (non-mutating; fresh handle each call, mirroring json).
	in.RegisterNamespaced(LibraryName, "list", listFn)
	in.RegisterNamespaced(LibraryName, "map", mapFn)
	in.RegisterNamespaced(LibraryName, "set", setFn)
	in.RegisterNamespaced(LibraryName, "insert", insertFn)
	in.RegisterNamespaced(LibraryName, "append", appendFn)
	in.RegisterNamespaced(LibraryName, "remove", removeFn)
	in.RegisterNamespaced(LibraryName, "move", moveFn)
}

// ----- encode --------------------------------------------------------------

func encodeFn(args []interpreter.Value, pretty bool) (interpreter.Value, error) {
	verb := "toml.encode"
	if pretty {
		verb = "toml.encodePretty"
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (value), got %d", verb, len(args))
	}
	s, err := encodeToml(args[0], pretty)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", verb, err)
	}
	return interpreter.StringVal(s), nil
}

// encodeToml renders a document. The root must be a table (a map / struct, or a
// toml.Value wrapping one); TOML has no top-level array or scalar form.
func encodeToml(v interpreter.Value, pretty bool) (string, error) {
	if inner, ok := v.AsObject(LibraryName, "Value"); ok {
		v = inner
	}
	root, err := asTable(v)
	if err != nil {
		return "", fmt.Errorf("a TOML document root must be a table, got %s", tomlNodeType(v))
	}
	var sb strings.Builder
	if err := emitTable(&sb, root, nil, pretty); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// tableEntry is a normalised (key, value) pair - a struct field or a map entry
// with a string key - so the encoder handles both container kinds uniformly.
type tableEntry struct {
	key string
	val interpreter.Value
}

// asTable normalises a map or struct to an ordered list of string-keyed
// entries, or reports that v is not a table.
func asTable(v interpreter.Value) ([]tableEntry, error) {
	switch v.Kind {
	case interpreter.KindMap:
		out := make([]tableEntry, len(v.Map))
		for i, e := range v.Map {
			if e.Key.Kind != interpreter.KindString {
				return nil, fmt.Errorf("table key must be string, got %s", e.Key.Kind)
			}
			out[i] = tableEntry{key: e.Key.Str, val: e.Value}
		}
		return out, nil
	case interpreter.KindStruct:
		if _, _, ok := isDatetimeNode(v); ok {
			return nil, fmt.Errorf("a datetime is not a table")
		}
		out := make([]tableEntry, len(v.Fields))
		for i, f := range v.Fields {
			out[i] = tableEntry{key: f.Name, val: f.Value}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("not a table")
	}
}

// isArrayOfTables reports whether v is a non-empty list whose every element is a
// table (so it renders as [[header]] sections rather than an inline array).
func isArrayOfTables(v interpreter.Value) bool {
	if v.Kind != interpreter.KindList || len(v.List) == 0 {
		return false
	}
	for _, e := range v.List {
		if _, err := asTable(e); err != nil {
			return false
		}
	}
	return true
}

// emitTable writes a table's contents: leaf keys first (so they attach to this
// table, not a following header), then sub-tables as [header] sections, then
// arrays of tables as [[header]] sections.
func emitTable(sb *strings.Builder, entries []tableEntry, path []string, pretty bool) error {
	for _, e := range entries {
		if e.val.Kind == interpreter.KindObject {
			if inner, ok := e.val.AsObject(LibraryName, "Value"); ok {
				e.val = inner
			}
		}
		if _, err := asTable(e.val); err == nil {
			continue // a sub-table: pass 2
		}
		if isArrayOfTables(e.val) {
			continue // pass 3
		}
		sb.WriteString(renderKey(e.key))
		sb.WriteString(" = ")
		if err := encodeInline(sb, e.val); err != nil {
			return err
		}
		sb.WriteByte('\n')
	}
	for _, e := range entries {
		val := e.val
		if inner, ok := val.AsObject(LibraryName, "Value"); ok {
			val = inner
		}
		sub, err := asTable(val)
		if err != nil {
			continue
		}
		childPath := append(append([]string{}, path...), e.key)
		if pretty && sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteByte('[')
		sb.WriteString(renderPath(childPath))
		sb.WriteString("]\n")
		if err := emitTable(sb, sub, childPath, pretty); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if !isArrayOfTables(e.val) {
			continue
		}
		childPath := append(append([]string{}, path...), e.key)
		for _, elem := range e.val.List {
			sub, _ := asTable(elem)
			if pretty && sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString("[[")
			sb.WriteString(renderPath(childPath))
			sb.WriteString("]]\n")
			if err := emitTable(sb, sub, childPath, pretty); err != nil {
				return err
			}
		}
	}
	return nil
}

// encodeInline renders a value in inline form: a scalar literal, a datetime
// bare, an inline array, or an inline table.
func encodeInline(sb *strings.Builder, v interpreter.Value) error {
	if inner, ok := v.AsObject(LibraryName, "Value"); ok {
		v = inner
	}
	switch v.Kind {
	case interpreter.KindNull:
		return fmt.Errorf("TOML has no null type")
	case interpreter.KindBool:
		if v.Bool {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case interpreter.KindInt:
		sb.WriteString(strconv.FormatInt(v.Int, 10))
	case interpreter.KindFloat:
		sb.WriteString(formatTomlFloat(v.Float))
	case interpreter.KindString:
		encodeBasicString(sb, v.Str)
	case interpreter.KindBytes:
		encodeBasicString(sb, base64.StdEncoding.EncodeToString(v.Bytes))
	case interpreter.KindStruct:
		if text, _, ok := isDatetimeNode(v); ok {
			sb.WriteString(text)
			return nil
		}
		return encodeInlineTable(sb, v)
	case interpreter.KindMap:
		return encodeInlineTable(sb, v)
	case interpreter.KindList:
		sb.WriteByte('[')
		for i, e := range v.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := encodeInline(sb, e); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
	case interpreter.KindTask:
		return fmt.Errorf("cannot encode a task value")
	default:
		return fmt.Errorf("cannot encode value of kind %s", v.Kind)
	}
	return nil
}

func encodeInlineTable(sb *strings.Builder, v interpreter.Value) error {
	entries, err := asTable(v)
	if err != nil {
		return err
	}
	sb.WriteByte('{')
	for i, e := range entries {
		if i > 0 {
			sb.WriteString(", ")
		} else {
			sb.WriteByte(' ')
		}
		sb.WriteString(renderKey(e.key))
		sb.WriteString(" = ")
		if err := encodeInline(sb, e.val); err != nil {
			return err
		}
	}
	if len(entries) > 0 {
		sb.WriteByte(' ')
	}
	sb.WriteByte('}')
	return nil
}

// formatTomlFloat renders a float in TOML syntax (inf / -inf / nan spelled out;
// a whole-valued float keeps a `.0` so it re-decodes as float).
func formatTomlFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// renderKey emits a bare key when it is a legal bare key, else a quoted basic
// string.
func renderKey(k string) string {
	if k == "" {
		return "\"\""
	}
	for i := 0; i < len(k); i++ {
		if !isBareKeyChar(k[i]) {
			var sb strings.Builder
			encodeBasicString(&sb, k)
			return sb.String()
		}
	}
	return k
}

// renderPath joins dotted-header segments, quoting any that need it.
func renderPath(segs []string) string {
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = renderKey(s)
	}
	return strings.Join(parts, ".")
}

// encodeBasicString writes s as a quoted, escaped TOML basic string.
func encodeBasicString(sb *strings.Builder, s string) {
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
