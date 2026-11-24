// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package tomllib implements the `toml` system library: RFC-conformant TOML
// 1.0.0 decode / encode onto the same opaque, library-owned Value that `json`
// uses. `toml.decode(text)` returns a `toml.Value` (a KindObject holding the
// shared compact document tree, internal/lib/doctree); the read accessors
// (`typeOf` / `get` / `has` / `keys` / `length` / `as*`, addressed by JSON
// Pointer) and the non-mutating write surface (`map` / `list` / `set` /
// `insert` / `append` / `remove` / `move`) are doctree's, the same as `json`'s
// and `yaml`'s, so the libraries read the same. The one thing TOML has that
// JSON does not - the four date-time forms - is a datetime node, surfaced
// through `toml.asDatetime` (the offset form, as a `time.Time`) and
// `toml.asLocal` (the three local forms, as a `toml.Local`).
package tomllib

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`toml.decode`, `toml.get`, ...).
const LibraryName = "toml"

// lib is the toml library's handle on the shared surface: TOML has no null,
// a stored time.Time becomes an offset date-time, and typeOf names the local
// forms.
var lib = &doctree.Lib{Name: LibraryName, Display: display, TimeNode: timeNode, AsTime: asTime, DatetimeType: datetimeType}

// datetimeType names a datetime node's form for typeOf: "datetime" for an
// offset date-time (what asDatetime reads), else TOML 1.0's name of the local
// form.
func datetimeType(n node) string {
	if n.Form() == formOffsetDatetime {
		return "datetime"
	}
	return formName(n.Form())
}

// display renders a toml.Value inline (`{ a = 1 }`), for `%v` and the REPL.
func display(n node) (string, error) {
	var sb strings.Builder
	if err := encodeInline(&sb, n); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// timeNode stores a time.Time as an offset date-time.
func timeNode(v interpreter.Value) (node, bool) {
	text, ok := doctree.TimeRFC3339(v)
	if !ok {
		return node{}, false
	}
	return doctree.Datetime(text, formOffsetDatetime), true
}

// asTime reads an offset date-time as a Go time. A local form names no
// instant (it has no offset), so it is refused rather than read as UTC.
func asTime(n node) (stdtime.Time, error) {
	if n.Form() != formOffsetDatetime {
		return stdtime.Time{}, fmt.Errorf("node is a %s, which has no UTC offset; read it with toml.asLocal", formName(n.Form()))
	}
	return parseDatetimeText(n.Str(), n.Form())
}

// asLocalFn implements `toml.asLocal(v[, ptr]) -> toml.Local`: the fields of a
// local date-time, date or time; the parts the form lacks are 0.
func asLocalFn(args []interpreter.Value) (interpreter.Value, error) {
	const fn = "toml.asLocal"
	n, err := lib.NodeAt(fn, args)
	if err != nil {
		return interpreter.Null(), err
	}
	if n.Kind() != doctree.KindDatetime || n.Form() == formOffsetDatetime {
		hint := ""
		if n.Kind() == doctree.KindDatetime {
			hint = "; read it with toml.asDatetime"
		}
		return interpreter.Null(), fmt.Errorf("%s: node is a %s, not a local-datetime, local-date or local-time%s", fn, lib.NodeType(n), hint)
	}
	t, err := parseDatetimeText(n.Str(), n.Form())
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %w", fn, err)
	}
	var y, mo, d, h, mi, sec, ns int
	if n.Form() != formLocalTime {
		y, mo, d = t.Year(), int(t.Month()), t.Day()
	}
	if n.Form() != formLocalDate {
		h, mi, sec, ns = t.Hour(), t.Minute(), t.Second(), t.Nanosecond()
	}
	iv := func(v int) interpreter.Value { return interpreter.IntVal(int64(v)) }
	return interpreter.NamespacedStructVal(LibraryName, "Local", []interpreter.StructField{
		{Name: "year", Value: iv(y)}, {Name: "month", Value: iv(mo)}, {Name: "day", Value: iv(d)},
		{Name: "hour", Value: iv(h)}, {Name: "minute", Value: iv(mi)}, {Name: "second", Value: iv(sec)},
		{Name: "nanosecond", Value: iv(ns)},
	}), nil
}

func inf(neg bool) float64 {
	if neg {
		return math.Inf(-1)
	}
	return math.Inf(1)
}

func nan() float64 { return math.NaN() }

// Install registers the toml library on in.
func Install(in *interpreter.Interpreter) {
	// The toml.Value type, its accessors (with isDatetime / asDatetime) and
	// its write surface (internal/lib/doctree).
	lib.Install(in)

	intField := func(name string) parser.StructField {
		return parser.StructField{Name: name, Type: parser.PrimitiveType(parser.TypeInt)}
	}
	// A local date-time, date or time (no offset), as toml.asLocal reads it.
	in.RegisterNamespacedStruct(LibraryName, "Local", []parser.StructField{
		intField("year"), intField("month"), intField("day"),
		intField("hour"), intField("minute"), intField("second"), intField("nanosecond"),
	})
	in.RegisterNamespaced(LibraryName, "asLocal", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return asLocalFn(args)
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
		return lib.Wrap(tree), nil
	})
	// Caps for decodeWith, each at most toml.decode's and 0 for that default.
	in.RegisterNamespacedStruct(LibraryName, "DecodeOptions", doctree.DecodeOptionFields())
	in.RegisterNamespaced(LibraryName, "decodeWith", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		tree, err := decodeWithFn(args)
		if err != nil {
			return interpreter.Null(), err
		}
		return lib.Wrap(tree), nil
	})

	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})
}

// decodeWithFn implements `toml.decodeWith(text, toml.DecodeOptions)`: decode
// under lower caps than toml.decode's.
func decodeWithFn(args []interpreter.Value) (node, error) {
	const fn = "toml.decodeWith"
	if len(args) != 2 {
		return node{}, fmt.Errorf("%s expects 2 arguments (string, toml.DecodeOptions), got %d", fn, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return node{}, fmt.Errorf("%s: argument 1 must be string, got %s", fn, args[0].Kind)
	}
	caps, err := doctree.ReadDecodeOptions(fn, LibraryName, args[1], defaultCaps())
	if err != nil {
		return node{}, err
	}
	return decodeTomlCaps(fn, args[0].Str, caps)
}

// parseDatetimeText parses stored RFC 3339 text into a Go time.Time, choosing
// the layout by the recorded form. Zone-less forms parse as UTC: a local date
// is taken at midnight, a local time on the Unix epoch date.
func parseDatetimeText(text string, form uint8) (stdtime.Time, error) {
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
		return stdtime.Time{}, fmt.Errorf("unknown datetime form %d", form)
	}
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
		return interpreter.Null(), fmt.Errorf("%s: %w", verb, err)
	}
	return interpreter.StringVal(s), nil
}

// encodeToml renders a document: a toml.Value, or any value that converts to
// one (a map / struct; `bytes` as a base64 string, a time.Time as an offset
// date-time). The root must be a table; TOML has no top-level array or scalar
// form.
func encodeToml(v interpreter.Value, pretty bool) (string, error) {
	root, err := lib.ToNode(v, false)
	if err != nil {
		return "", err
	}
	if root.Kind() != doctree.KindMap {
		return "", fmt.Errorf("a TOML document root must be a table, got %s", root.TypeName())
	}
	var sb strings.Builder
	if err := emitTable(&sb, root, nil, pretty); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// isArrayOfTables reports whether n is a non-empty list whose every element is
// a table (so it renders as [[header]] sections rather than an inline array).
func isArrayOfTables(n node) bool {
	if n.Kind() != doctree.KindList || n.Len() == 0 {
		return false
	}
	for _, e := range n.Elems() {
		if e.Kind() != doctree.KindMap {
			return false
		}
	}
	return true
}

// emitTable writes a table's contents: leaf keys first (so they attach to this
// table, not a following header), then sub-tables as [header] sections, then
// arrays of tables as [[header]] sections.
func emitTable(sb *strings.Builder, t node, path []string, pretty bool) error {
	for i, v := range t.Elems() {
		if v.Kind() == doctree.KindMap || isArrayOfTables(v) {
			continue // passes 2 and 3
		}
		sb.WriteString(renderKey(t.Key(i)))
		sb.WriteString(" = ")
		if err := encodeInline(sb, v); err != nil {
			return err
		}
		sb.WriteByte('\n')
	}
	for i, v := range t.Elems() {
		if v.Kind() != doctree.KindMap {
			continue
		}
		childPath := append(append([]string{}, path...), t.Key(i))
		if pretty && sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteByte('[')
		sb.WriteString(renderPath(childPath))
		sb.WriteString("]\n")
		if err := emitTable(sb, v, childPath, pretty); err != nil {
			return err
		}
	}
	for i, v := range t.Elems() {
		if !isArrayOfTables(v) {
			continue
		}
		childPath := append(append([]string{}, path...), t.Key(i))
		for _, elem := range v.Elems() {
			if pretty && sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString("[[")
			sb.WriteString(renderPath(childPath))
			sb.WriteString("]]\n")
			if err := emitTable(sb, elem, childPath, pretty); err != nil {
				return err
			}
		}
	}
	return nil
}

// encodeInline renders a node in inline form: a scalar literal, a datetime
// bare, an inline array, or an inline table.
func encodeInline(sb *strings.Builder, n node) error {
	switch n.Kind() {
	case doctree.KindNull:
		return fmt.Errorf("TOML has no null type")
	case doctree.KindBool:
		if n.BoolVal() {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case doctree.KindInt:
		sb.WriteString(strconv.FormatInt(n.IntVal(), 10))
	case doctree.KindBigInt:
		return fmt.Errorf("TOML integers are 64-bit; %s does not fit (encode it as a string)", n.Str())
	case doctree.KindFloat:
		sb.WriteString(formatTomlFloat(n.FloatVal()))
	case doctree.KindString:
		encodeBasicString(sb, n.Str())
	case doctree.KindBytes:
		encodeBasicString(sb, base64.StdEncoding.EncodeToString([]byte(n.Str())))
	case doctree.KindDatetime:
		sb.WriteString(n.Str())
	case doctree.KindMap:
		sb.WriteByte('{')
		for i, v := range n.Elems() {
			if i > 0 {
				sb.WriteString(", ")
			} else {
				sb.WriteByte(' ')
			}
			sb.WriteString(renderKey(n.Key(i)))
			sb.WriteString(" = ")
			if err := encodeInline(sb, v); err != nil {
				return err
			}
		}
		if n.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteByte('}')
	case doctree.KindList:
		sb.WriteByte('[')
		for i, e := range n.Elems() {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := encodeInline(sb, e); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
	}
	return nil
}

// formatTomlFloat renders a float in TOML syntax: inf / -inf / nan spelled
// out, else doctree.FormatFloat (`1000000.0`, `1e-7`).
func formatTomlFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	return doctree.FormatFloat(f)
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
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(sb, `\u%04x`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}
