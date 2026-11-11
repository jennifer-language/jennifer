// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package jsonlib is the `json` library: RFC 8259 encode / decode. Hand-rolled
// (no `encoding/json`, no `reflect`) to stay TinyGo-clean, the same reason the
// AST-JSON emitter is hand-rolled. `decode` returns an opaque `json.Value` (a
// KindObject holding the shared compact node tree, internal/lib/doctree:
// objects become maps in document order, numbers int when integral else
// float); the shared accessors walk it, and there is no map-to-struct coercion - a typed
// target is rebuilt explicitly from the decoded value (see the library doc,
// and docs/technical/rejected.md). `encode` accepts ordinary typed values and
// json.Values alike (a decode/encode round-trip).
package jsonlib

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`json.`) and the `use` name.
const LibraryName = "json"

// Install registers the json surface. `decode` returns an opaque
// `json.Value` (a KindObject) that callers walk with the accessors below;
// `encode` accepts ordinary typed values and json.Values alike.
func Install(in *interpreter.Interpreter) {
	// The json.Value type (an uninitialised one holds JSON null), its
	// accessors and its write surface (internal/lib/doctree).
	lib.Install(in)

	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})
	// Options for encodeWith. The zero value is json.encode's output, so
	// `json.EncodeOptions{}` changes nothing and each field opts into one
	// difference.
	in.RegisterNamespacedStruct(LibraryName, "EncodeOptions", []parser.StructField{
		{Name: "indent", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "sortKeys", Type: parser.PrimitiveType(parser.TypeBool)},
		{Name: "rawHtml", Type: parser.PrimitiveType(parser.TypeBool)},
		{Name: "canonical", Type: parser.PrimitiveType(parser.TypeBool)},
	})
	in.RegisterNamespaced(LibraryName, "encodeWith", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeWithFn(args)
	})
	in.RegisterNamespaced(LibraryName, "mergePatch", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return mergePatchFn(args)
	})
	in.RegisterNamespaced(LibraryName, "decode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		tree, err := decodeFn(args)
		if err != nil {
			return interpreter.Null(), err
		}
		return wrap(tree), nil
	})
}

// encoder writes one value's JSON image. Its options are json.EncodeOptions:
// indent ("" is compact), sorted object keys, HTML-sensitive characters left
// unescaped, and RFC 8785 canonical form (which implies the previous two plus
// ECMAScript number text).
type encoder struct {
	sb        strings.Builder
	indent    string
	sortKeys  bool
	rawHtml   bool
	canonical bool
	pad       string // indent repeated, grown on demand by newline
}

// maxIndent bounds EncodeOptions.indent: wider is never useful and only
// multiplies the output.
const maxIndent = 16

// prettyIndent is encodePretty's indent.
const prettyIndent = "  "

func encodeFn(args []interpreter.Value, pretty bool) (interpreter.Value, error) {
	verb := "json.encode"
	e := &encoder{}
	if pretty {
		verb = "json.encodePretty"
		e.indent = prettyIndent
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (value), got %d", verb, len(args))
	}
	return e.run(verb, args[0])
}

// encodeWithFn is json.encodeWith(value, json.EncodeOptions).
func encodeWithFn(args []interpreter.Value) (interpreter.Value, error) {
	const verb = "json.encodeWith"
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (value, json.EncodeOptions), got %d", verb, len(args))
	}
	e, err := encoderFor(verb, args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	return e.run(verb, args[0])
}

// encoderFor reads a json.EncodeOptions. Settings that contradict each other
// are an error rather than one silently winning.
func encoderFor(verb string, v interpreter.Value) (*encoder, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "EncodeOptions" {
		return nil, fmt.Errorf("%s: options must be a json.EncodeOptions, got %s", verb, v.Kind)
	}
	e := &encoder{}
	indent := int64(0)
	for _, f := range v.Fields {
		switch f.Name {
		case "indent":
			indent = f.Value.Int
		case "sortKeys":
			e.sortKeys = f.Value.Bool
		case "rawHtml":
			e.rawHtml = f.Value.Bool
		case "canonical":
			e.canonical = f.Value.Bool
		}
	}
	if indent < 0 || indent > maxIndent {
		return nil, fmt.Errorf("%s: indent must be 0 (compact) to %d, got %d", verb, maxIndent, indent)
	}
	if e.canonical && indent != 0 {
		return nil, fmt.Errorf("%s: canonical output has no indentation; leave indent at 0", verb)
	}
	e.indent = strings.Repeat(" ", int(indent))
	if e.canonical {
		e.sortKeys, e.rawHtml = true, true
	}
	return e, nil
}

func (e *encoder) run(verb string, v interpreter.Value) (interpreter.Value, error) {
	if err := e.value(v, 0); err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", verb, err)
	}
	return interpreter.StringVal(e.sb.String()), nil
}

// encodeNode writes n's compact JSON image to sb (the json.Value display).
func encodeNode(sb *strings.Builder, n node, pretty bool, depth int) error {
	e := &encoder{}
	if pretty {
		e.indent = prettyIndent
	}
	if err := e.node(n, depth); err != nil {
		return err
	}
	sb.WriteString(e.sb.String())
	return nil
}

// value writes v's JSON image. depth drives the indent.
func (e *encoder) value(v interpreter.Value, depth int) error {
	// The decoder caps nesting at MaxNestingDepth, but the json.set / insert /
	// append write API can grow a json.Value tree past it; without this the
	// symmetric encode recursion (one Go frame per level) is an uncatchable stack
	// overflow. Bound it here so encode / encodePretty / %v raise a catchable error.
	if depth > limits.MaxNestingDepth {
		return fmt.Errorf("value nests deeper than the %d-level limit", limits.MaxNestingDepth)
	}
	sb := &e.sb
	switch v.Kind {
	case interpreter.KindNull:
		sb.WriteString("null")
	case interpreter.KindBool:
		writeBool(sb, v.Bool)
	case interpreter.KindInt:
		return e.int(v.Int)
	case interpreter.KindFloat:
		return e.float(v.Float)
	case interpreter.KindString:
		e.string(v.Str)
	case interpreter.KindBytes:
		e.string(base64.StdEncoding.EncodeToString(v.Bytes))
	case interpreter.KindList:
		return e.seq(len(v.List), depth, '[', ']', nil, func(i int) error {
			return e.value(v.List[i], depth+1)
		})
	case interpreter.KindMap:
		for _, m := range v.Map {
			if m.Key.Kind != interpreter.KindString {
				return fmt.Errorf("map key must be string, got %s", m.Key.Kind)
			}
		}
		return e.seq(len(v.Map), depth, '{', '}', func(i int) string { return v.Map[i].Key.Str }, func(i int) error {
			e.member(v.Map[i].Key.Str)
			return e.value(v.Map[i].Value, depth+1)
		})
	case interpreter.KindStruct:
		return e.fields(v.Fields, depth)
	case interpreter.KindEnum:
		// Externally tagged, the conventional JSON encoding for a sum type: a
		// payload-less variant is its name as a string, a payloaded one is a
		// single-key object `{"Variant": {fields}}`. Both are unambiguous about
		// which variant is present, which a bare payload object would not be.
		// Decoding does not reconstruct an enum - json.decode yields the plain
		// string / map, and the program matches on it - because the wire form
		// carries no enum type identity.
		if len(v.Fields) == 0 {
			e.string(v.Variant)
			return nil
		}
		return e.seq(1, depth, '{', '}', nil, func(int) error {
			e.member(v.Variant)
			return e.fields(v.Fields, depth+1)
		})
	case interpreter.KindObject:
		// a json.Value round-trips: encode its node tree.
		if n, ok := asNode(v); ok {
			return e.node(n, depth)
		}
		return fmt.Errorf("cannot encode an opaque %s.%s value", v.StructNS, v.StructName)
	case interpreter.KindTask:
		return fmt.Errorf("cannot encode a task value")
	default:
		return fmt.Errorf("cannot encode value of kind %s", v.Kind)
	}
	return nil
}

// fields writes a struct's (or an enum payload's) fields as an object.
func (e *encoder) fields(fs []interpreter.StructField, depth int) error {
	return e.seq(len(fs), depth, '{', '}', func(i int) string { return fs[i].Name }, func(i int) error {
		e.member(fs[i].Name)
		return e.value(fs[i].Value, depth+1)
	})
}

// node writes a json.Value node's JSON image, with the same depth guard and
// number / string forms as value.
func (e *encoder) node(n node, depth int) error {
	if depth > limits.MaxNestingDepth {
		return fmt.Errorf("value nests deeper than the %d-level limit", limits.MaxNestingDepth)
	}
	switch n.Kind() {
	case doctree.KindNull:
		e.sb.WriteString("null")
	case doctree.KindBool:
		writeBool(&e.sb, n.BoolVal())
	case doctree.KindInt:
		return e.int(n.IntVal())
	case doctree.KindFloat:
		return e.float(n.FloatVal())
	case doctree.KindString:
		e.string(n.Str())
	case doctree.KindList:
		return e.seq(n.Len(), depth, '[', ']', nil, func(i int) error {
			return e.node(n.Elem(i), depth+1)
		})
	case doctree.KindMap:
		return e.seq(n.Len(), depth, '{', '}', n.Key, func(i int) error {
			e.member(n.Key(i))
			return e.node(n.Elem(i), depth+1)
		})
	default:
		return fmt.Errorf("cannot encode a %s node", n.TypeName())
	}
	return nil
}

func writeBool(sb *strings.Builder, b bool) {
	if b {
		sb.WriteString("true")
	} else {
		sb.WriteString("false")
	}
}

// maxExactInt is 2^53: every integer up to it in magnitude is exactly a
// float64, the one number type RFC 8785 allows.
const maxExactInt = 1 << 53

// int writes n in decimal without allocating. Canonical form numbers are
// IEEE doubles, so an integer that is not exactly one is an error there, not
// a silently different number.
func (e *encoder) int(n int64) error {
	if e.canonical && (n > maxExactInt || n < -maxExactInt) {
		return fmt.Errorf("canonical form cannot hold the integer %d exactly (beyond +/-2^53); encode it as a string", n)
	}
	var buf [20]byte
	e.sb.Write(strconv.AppendInt(buf[:0], n, 10))
	return nil
}

// float writes a finite float. The form is the one JavaScript and Go's
// encoding/json print: plain digits for 1e-6 <= |f| < 1e21 (`1000000.0`, not
// `1e+06`), an exponent outside it, with no zero padding in the exponent
// (`1e-7`, not `1e-07`). Outside canonical form an integral float gets a
// `.0`, so it decodes back as a float; canonical form is ECMAScript's text
// exactly (`5`, and `0` for -0).
func (e *encoder) float(f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("cannot encode non-finite float")
	}
	if e.canonical && f == 0 {
		e.sb.WriteByte('0')
		return nil
	}
	var buf [32]byte
	form := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		form = 'e'
	}
	b := strconv.AppendFloat(buf[:0], f, form, -1, 64)
	if form == 'e' {
		// e-07 -> e-7
		if n := len(b); n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
		e.sb.Write(b)
		return nil
	}
	e.sb.Write(b)
	if !e.canonical && !bytes.ContainsRune(b, '.') {
		e.sb.WriteString(".0")
	}
	return nil
}

// seq writes an array or object body, calling item(i) for each element.
// item is responsible for writing the element (and, for objects, its key and
// colon). key, for an object, names member i; with sortKeys the members are
// written in key order. Empty sequences collapse to `[]` / `{}` even when
// indented.
func (e *encoder) seq(n, depth int, open, close byte, key func(int) string, item func(int) error) error {
	sb := &e.sb
	sb.WriteByte(open)
	if n == 0 {
		sb.WriteByte(close)
		return nil
	}
	var order []int
	if e.sortKeys && key != nil && n > 1 {
		order = make([]int, n)
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return lessUTF16(key(order[a]), key(order[b])) })
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		e.newline(depth + 1)
		at := i
		if order != nil {
			at = order[i]
		}
		if err := item(at); err != nil {
			return err
		}
	}
	e.newline(depth)
	sb.WriteByte(close)
	return nil
}

// member writes an object key and its colon.
func (e *encoder) member(key string) {
	e.string(key)
	if e.indent != "" {
		e.sb.WriteString(": ")
	} else {
		e.sb.WriteByte(':')
	}
}

// newline writes a newline plus the indent depth times when indenting;
// nothing otherwise.
func (e *encoder) newline(depth int) {
	if e.indent == "" {
		return
	}
	n := depth * len(e.indent)
	for len(e.pad) < n {
		e.pad += e.pad + e.indent
	}
	e.sb.WriteByte('\n')
	e.sb.WriteString(e.pad[:n])
}

// lessUTF16 orders strings by their UTF-16 code units, the key order RFC 8785
// prescribes (and JavaScript's default sort). It differs from byte (code
// point) order only between characters above U+FFFF, which sort as surrogates
// (0xD800..), and U+E000..U+FFFF.
func lessUTF16(a, b string) bool {
	for a != "" && b != "" {
		if a[0] < utf8.RuneSelf && b[0] < utf8.RuneSelf {
			if a[0] != b[0] {
				return a[0] < b[0]
			}
			a, b = a[1:], b[1:]
			continue
		}
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if ra != rb {
			ua, ub := firstUnit(ra), firstUnit(rb)
			if ua != ub {
				return ua < ub
			}
			return ra < rb // same high surrogate: the low ones order as the runes do
		}
		a, b = a[na:], b[nb:]
	}
	return a == "" && b != ""
}

// firstUnit is the first UTF-16 code unit of r.
func firstUnit(r rune) rune {
	if r < 0x10000 {
		return r
	}
	hi, _ := utf16.EncodeRune(r)
	return hi
}

// string writes s as a quoted, escaped JSON string. Input is valid UTF-8 (a
// Go string) and emitted as-is except for the mandatory escapes and control
// characters below U+0020. Unless rawHtml is set, HTML-sensitive characters
// (`<` `>` `&` U+2028 U+2029) are escaped to their `\u00xx` form too. That
// set matches Go's encoding/json default (SetEscapeHTML(true)): JSON is
// routinely embedded in an HTML `<script>` block or an attribute, and an
// unescaped `</script>` or line separator would break out. The escapes stay
// valid JSON and decode to the identical string, so a round-trip through
// json.decode is unchanged. A byte that is not valid UTF-8 (which a Jennifer
// string should not hold) is written as U+FFFD.
//
// Runs of bytes that need no escape are copied in one write.
func (e *encoder) string(s string) {
	sb := &e.sb
	table := &htmlEscape
	if e.rawHtml {
		table = &plainEscape
	}
	sb.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if !table[c] {
				i++
				continue
			}
			sb.WriteString(s[start:i])
			switch c {
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
			default: // other control characters and < > &
				sb.WriteString(`\u00`)
				sb.WriteByte(hexDigits[c>>4])
				sb.WriteByte(hexDigits[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\u2028' && !e.rawHtml:
			sb.WriteString(s[start:i])
			sb.WriteString(`\u2028`)
		case r == '\u2029' && !e.rawHtml:
			sb.WriteString(s[start:i])
			sb.WriteString(`\u2029`)
		case r == utf8.RuneError && size == 1:
			sb.WriteString(s[start:i])
			sb.WriteString("\uFFFD")
		default:
			i += size
			continue
		}
		i += size
		start = i
	}
	sb.WriteString(s[start:])
	sb.WriteByte('"')
}

const hexDigits = "0123456789abcdef"

// plainEscape marks the ASCII bytes JSON requires escaped: control
// characters, `"` and `\`. htmlEscape adds the HTML-sensitive `<` `>` `&`.
var plainEscape, htmlEscape = func() (plain, html [utf8.RuneSelf]bool) {
	for c := 0; c < 0x20; c++ {
		plain[c] = true
	}
	plain['"'], plain['\\'] = true, true
	html = plain
	html['<'], html['>'], html['&'] = true, true, true
	return plain, html
}()
