// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func decodeFn(args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("json.decode expects 1 argument (string), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("json.decode: argument must be string, got %s", args[0].Kind)
	}
	d := &decoder{s: args[0].Str}
	d.skipWS()
	v, err := d.parseValue()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("json.decode: %v", err)
	}
	d.skipWS()
	if d.pos < len(d.s) {
		return interpreter.Null(), fmt.Errorf("json.decode: %v", d.errf("unexpected trailing content"))
	}
	return v, nil
}

// maxNestingDepth caps container recursion in parseValue. Each nesting level
// costs Go stack frames, and a Go stack overflow is fatal (not a catchable
// Jennifer error), so deeply-nested untrusted input could otherwise kill the
// whole process. The limit is shared with the language parser and the other
// decoders, and is build-tag split so it stays below the constrained TinyGo
// binary's fixed-stack crash point (see internal/limits).
const maxNestingDepth = limits.MaxNestingDepth

// decoder is a recursive-descent JSON reader over a byte-indexed string.
type decoder struct {
	s     string
	pos   int
	depth int // current container nesting, gated in parseValue
}

// errf builds an error tagged with the line/column of the current position.
func (d *decoder) errf(format string, a ...any) error {
	line, col := 1, 1
	for i := 0; i < d.pos && i < len(d.s); i++ {
		if d.s[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return fmt.Errorf("%s at line %d, column %d", fmt.Sprintf(format, a...), line, col)
}

func (d *decoder) skipWS() {
	for d.pos < len(d.s) {
		switch d.s[d.pos] {
		case ' ', '\t', '\n', '\r':
			d.pos++
		default:
			return
		}
	}
}

func (d *decoder) parseValue() (interpreter.Value, error) {
	if d.pos >= len(d.s) {
		return interpreter.Null(), d.errf("unexpected end of input")
	}
	switch c := d.s[d.pos]; {
	case c == '{':
		if d.depth >= maxNestingDepth {
			return interpreter.Null(), d.errf("nesting exceeds %d levels", maxNestingDepth)
		}
		d.depth++
		v, err := d.parseObject()
		d.depth--
		return v, err
	case c == '[':
		if d.depth >= maxNestingDepth {
			return interpreter.Null(), d.errf("nesting exceeds %d levels", maxNestingDepth)
		}
		d.depth++
		v, err := d.parseArray()
		d.depth--
		return v, err
	case c == '"':
		s, err := d.parseString()
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.StringVal(s), nil
	case c == 't' || c == 'f':
		return d.parseKeyword()
	case c == 'n':
		return d.parseKeyword()
	case c == '-' || (c >= '0' && c <= '9'):
		return d.parseNumber()
	default:
		return interpreter.Null(), d.errf("unexpected character %q", string(c))
	}
}

// parseObject decodes a JSON object into a generic `map of string to V`.
// Duplicate keys are last-wins so the map stays well-formed.
func (d *decoder) parseObject() (interpreter.Value, error) {
	d.pos++ // '{'
	var entries []interpreter.MapEntry
	// idx keys each seen key to its position so duplicate handling stays O(1)
	// per key: a linear scan per key made a large object O(N^2).
	idx := map[string]int{}
	d.skipWS()
	if d.pos < len(d.s) && d.s[d.pos] == '}' {
		d.pos++
		return mapVal(entries), nil
	}
	for {
		d.skipWS()
		if d.pos >= len(d.s) || d.s[d.pos] != '"' {
			return interpreter.Null(), d.errf("expected string key in object")
		}
		key, err := d.parseString()
		if err != nil {
			return interpreter.Null(), err
		}
		d.skipWS()
		if d.pos >= len(d.s) || d.s[d.pos] != ':' {
			return interpreter.Null(), d.errf("expected ':' after object key")
		}
		d.pos++
		d.skipWS()
		val, err := d.parseValue()
		if err != nil {
			return interpreter.Null(), err
		}
		if pos, seen := idx[key]; seen {
			entries[pos].Value = val // last-wins
		} else {
			idx[key] = len(entries)
			entries = append(entries, interpreter.MapEntry{Key: interpreter.StringVal(key), Value: val})
		}
		d.skipWS()
		if d.pos >= len(d.s) {
			return interpreter.Null(), d.errf("unterminated object")
		}
		if d.s[d.pos] == ',' {
			d.pos++
			continue
		}
		if d.s[d.pos] == '}' {
			d.pos++
			return mapVal(entries), nil
		}
		return interpreter.Null(), d.errf("expected ',' or '}' in object")
	}
}

func (d *decoder) parseArray() (interpreter.Value, error) {
	d.pos++ // '['
	elems := []interpreter.Value{}
	d.skipWS()
	if d.pos < len(d.s) && d.s[d.pos] == ']' {
		d.pos++
		return listVal(elems), nil
	}
	for {
		d.skipWS()
		v, err := d.parseValue()
		if err != nil {
			return interpreter.Null(), err
		}
		elems = append(elems, v)
		d.skipWS()
		if d.pos >= len(d.s) {
			return interpreter.Null(), d.errf("unterminated array")
		}
		if d.s[d.pos] == ',' {
			d.pos++
			continue
		}
		if d.s[d.pos] == ']' {
			d.pos++
			return listVal(elems), nil
		}
		return interpreter.Null(), d.errf("expected ',' or ']' in array")
	}
}

func (d *decoder) parseString() (string, error) {
	d.pos++ // opening quote
	var sb strings.Builder
	for d.pos < len(d.s) {
		c := d.s[d.pos]
		if c == '"' {
			d.pos++
			return sb.String(), nil
		}
		if c == '\\' {
			d.pos++
			if d.pos >= len(d.s) {
				return "", d.errf("unterminated escape")
			}
			switch d.s[d.pos] {
			case '"':
				sb.WriteByte('"')
				d.pos++
			case '\\':
				sb.WriteByte('\\')
				d.pos++
			case '/':
				sb.WriteByte('/')
				d.pos++
			case 'n':
				sb.WriteByte('\n')
				d.pos++
			case 't':
				sb.WriteByte('\t')
				d.pos++
			case 'r':
				sb.WriteByte('\r')
				d.pos++
			case 'b':
				sb.WriteByte('\b')
				d.pos++
			case 'f':
				sb.WriteByte('\f')
				d.pos++
			case 'u':
				r, err := d.parseUnicodeEscape()
				if err != nil {
					return "", err
				}
				sb.WriteRune(r)
			default:
				return "", d.errf("invalid escape \\%c", d.s[d.pos])
			}
			continue
		}
		if c < 0x20 {
			return "", d.errf("control character U+%04X in string", c)
		}
		sb.WriteByte(c)
		d.pos++
	}
	return "", d.errf("unterminated string")
}

// parseUnicodeEscape decodes a `\uXXXX` (or a high+low surrogate pair) with
// d.pos at the `u`, and leaves d.pos just past the consumed hex.
func (d *decoder) parseUnicodeEscape() (rune, error) {
	hi, err := d.readHex4()
	if err != nil {
		return 0, err
	}
	if utf16.IsSurrogate(rune(hi)) {
		if d.pos+1 < len(d.s) && d.s[d.pos] == '\\' && d.s[d.pos+1] == 'u' {
			d.pos++ // consume '\'
			lo, err := d.readHex4()
			if err != nil {
				return 0, err
			}
			r := utf16.DecodeRune(rune(hi), rune(lo))
			if r == utf8.RuneError {
				return 0, d.errf("invalid surrogate pair")
			}
			return r, nil
		}
		return 0, d.errf("unpaired surrogate")
	}
	return rune(hi), nil
}

// readHex4 expects d.pos at `u`, consumes it plus four hex digits, and returns
// the 16-bit value.
func (d *decoder) readHex4() (uint32, error) {
	d.pos++ // 'u'
	if d.pos+4 > len(d.s) {
		return 0, d.errf("incomplete \\u escape")
	}
	v, err := strconv.ParseUint(d.s[d.pos:d.pos+4], 16, 32)
	if err != nil {
		return 0, d.errf("invalid \\u escape")
	}
	d.pos += 4
	return uint32(v), nil
}

// parseKeyword handles the three bare literals true / false / null.
func (d *decoder) parseKeyword() (interpreter.Value, error) {
	switch {
	case strings.HasPrefix(d.s[d.pos:], "true"):
		d.pos += 4
		return interpreter.BoolVal(true), nil
	case strings.HasPrefix(d.s[d.pos:], "false"):
		d.pos += 5
		return interpreter.BoolVal(false), nil
	case strings.HasPrefix(d.s[d.pos:], "null"):
		d.pos += 4
		return interpreter.Null(), nil
	}
	return interpreter.Null(), d.errf("invalid literal")
}

// parseNumber scans a JSON number and returns int when it has no fractional
// or exponent part (and fits int64), else float. The grammar is walked
// explicitly rather than delegated to strconv, so json.org's rules are
// enforced strictly: no leading zeros (`01`), a fraction needs at least one
// digit (`1.` is invalid), and an exponent needs at least one digit (`1e`).
// strconv accepts all of those, so a permissive scan-then-parse would leak
// them through.
func (d *decoder) parseNumber() (interpreter.Value, error) {
	start := d.pos
	if d.pos < len(d.s) && d.s[d.pos] == '-' {
		d.pos++
	}
	// integer part: a lone 0, or a nonzero digit followed by more digits.
	if d.pos >= len(d.s) || !isDigit(d.s[d.pos]) {
		return interpreter.Null(), d.errf("invalid number: expected a digit")
	}
	if d.s[d.pos] == '0' {
		d.pos++ // a leading 0 stands alone; 01 / 00 are rejected below
	} else {
		for d.pos < len(d.s) && isDigit(d.s[d.pos]) {
			d.pos++
		}
	}
	isFloat := false
	if d.pos < len(d.s) && d.s[d.pos] == '.' {
		isFloat = true
		d.pos++
		if d.pos >= len(d.s) || !isDigit(d.s[d.pos]) {
			return interpreter.Null(), d.errf("invalid number: a fraction needs at least one digit")
		}
		for d.pos < len(d.s) && isDigit(d.s[d.pos]) {
			d.pos++
		}
	}
	if d.pos < len(d.s) && (d.s[d.pos] == 'e' || d.s[d.pos] == 'E') {
		isFloat = true
		d.pos++
		if d.pos < len(d.s) && (d.s[d.pos] == '+' || d.s[d.pos] == '-') {
			d.pos++
		}
		if d.pos >= len(d.s) || !isDigit(d.s[d.pos]) {
			return interpreter.Null(), d.errf("invalid number: an exponent needs at least one digit")
		}
		for d.pos < len(d.s) && isDigit(d.s[d.pos]) {
			d.pos++
		}
	}
	// A digit immediately after a lone leading 0 (`01`) never gets consumed
	// above, so it surfaces as trailing content at the call site; catch the
	// clearer message here instead.
	if d.pos < len(d.s) && isDigit(d.s[d.pos]) {
		return interpreter.Null(), d.errf("invalid number: leading zeros are not allowed")
	}
	tok := d.s[start:d.pos]
	// `-0` (with no fraction/exponent) would parse to int 0 and lose its sign;
	// decode it as negative-zero float to preserve the sign, matching Go's
	// encoding/json.
	if !isFloat && tok == "-0" {
		return interpreter.FloatVal(math.Copysign(0, -1)), nil
	}
	if !isFloat {
		if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
			return interpreter.IntVal(n), nil
		}
		// too big for int64: fall through to float
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return interpreter.Null(), d.errf("invalid number %q", tok)
	}
	return interpreter.FloatVal(f), nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// listVal / mapVal build generic decoded collections. Element / value types
// are left unset so the value is assignable to any declared list / map type
// through Value.MatchesDeclared's lenient path (the same leniency an empty
// literal gets); keys are always strings.
func listVal(elems []interpreter.Value) interpreter.Value {
	return interpreter.Value{Kind: interpreter.KindList, List: elems}
}

func mapVal(entries []interpreter.MapEntry) interpreter.Value {
	st := parser.PrimitiveType(parser.TypeString)
	return interpreter.Value{Kind: interpreter.KindMap, Map: entries, KeyTyp: &st}
}
