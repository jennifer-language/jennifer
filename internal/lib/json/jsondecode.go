// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/limits"
)

func decodeFn(args []interpreter.Value) (node, error) {
	if len(args) != 1 {
		return node{}, fmt.Errorf("json.decode expects 1 argument (string), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return node{}, fmt.Errorf("json.decode: argument must be string, got %s", args[0].Kind)
	}
	d := &decoder{s: args[0].Str}
	d.skipWS()
	v, err := d.parseValue()
	if err != nil {
		return node{}, fmt.Errorf("json.decode: %v", err)
	}
	d.skipWS()
	if d.pos < len(d.s) {
		return node{}, fmt.Errorf("json.decode: %v", d.errf("unexpected trailing content"))
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

// maxDecodedNodes bounds how many value nodes one decode may materialise, so a
// document of millions of tiny values is a catchable error rather than
// unbounded memory. A package var so tests can lower it.
var maxDecodedNodes = limits.MaxDecodedNodes

// decoder is a recursive-descent JSON reader over a byte-indexed string.
//
// Containers are collected on two shared stacks (elems / keys) rather than a
// growing slice each: a container pushes its members above whatever its
// ancestors have pushed, and on close copies exactly its own run into a
// right-sized slice and pops it. A document's containers therefore carry no
// append slack, and the decode allocates one slice per container instead of
// one per growth step.
type decoder struct {
	s     string
	elems []node
	keys  []string
	pos   int
	depth int // current container nesting, gated in parseValue
	nodes int // value nodes materialised so far, gated in parseValue
	// Memoized line/column for errf: lineOff is how far into s the (line, col)
	// pair has been advanced. d.pos moves forward during a decode, so errf walks
	// the cache from lineOff to d.pos instead of rescanning from the start each
	// time - O(n) total across any number of errf calls, not O(n) per call.
	lineOff int
	line    int
	col     int
}

// errf builds an error tagged with the line/column of the current position.
// It advances a memoized (line, col) from lineOff up to d.pos rather than
// rescanning from the start, so repeated calls stay O(n) in total. A backward
// move (should not happen in this decoder) falls back to a full rescan.
func (d *decoder) errf(format string, a ...any) error {
	if d.line == 0 || d.pos < d.lineOff {
		d.lineOff, d.line, d.col = 0, 1, 1
	}
	for d.lineOff < d.pos && d.lineOff < len(d.s) {
		if d.s[d.lineOff] == '\n' {
			d.line++
			d.col = 1
		} else {
			d.col++
		}
		d.lineOff++
	}
	return fmt.Errorf("%s at line %d, column %d", fmt.Sprintf(format, a...), d.line, d.col)
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

func (d *decoder) parseValue() (node, error) {
	if d.pos >= len(d.s) {
		return node{}, d.errf("unexpected end of input")
	}
	d.nodes++
	if d.nodes > maxDecodedNodes {
		return node{}, d.errf("document expands to more than %d nodes (input rejected as a decode bomb)", maxDecodedNodes)
	}
	switch c := d.s[d.pos]; {
	case c == '{':
		if d.depth >= maxNestingDepth {
			return node{}, d.errf("nesting exceeds %d levels", maxNestingDepth)
		}
		d.depth++
		v, err := d.parseObject()
		d.depth--
		return v, err
	case c == '[':
		if d.depth >= maxNestingDepth {
			return node{}, d.errf("nesting exceeds %d levels", maxNestingDepth)
		}
		d.depth++
		v, err := d.parseArray()
		d.depth--
		return v, err
	case c == '"':
		s, err := d.parseString()
		if err != nil {
			return node{}, err
		}
		return doctree.String(s), nil
	case c == 't' || c == 'f':
		return d.parseKeyword()
	case c == 'n':
		return d.parseKeyword()
	case c == '-' || (c >= '0' && c <= '9'):
		return d.parseNumber()
	default:
		return node{}, d.errf("unexpected character %q", string(c))
	}
}

// smallObject is the member count up to which duplicate keys are found by
// scanning the key stack rather than through a set.
const smallObject = 8

// parseObject decodes a JSON object into a map node, members in document
// order. A duplicate key is an error, as in `yaml` and `toml`: RFC 8259 leaves
// the behaviour open, and I-JSON (RFC 7493) forbids duplicates. Picking one
// copy silently would let two parsers of the same document disagree on its
// value (a parser-differential: a JWT with two `exp` claims, say).
func (d *decoder) parseObject() (node, error) {
	d.pos++ // '{'
	kbase, ebase := len(d.keys), len(d.elems)
	// seen detects duplicates in a large object in O(1) per key; a small one is
	// scanned directly on the key stack, which needs no allocation.
	var seen map[string]bool
	d.skipWS()
	if d.pos < len(d.s) && d.s[d.pos] == '}' {
		d.pos++
		return doctree.Map(nil, nil), nil
	}
	for {
		d.skipWS()
		if d.pos >= len(d.s) || d.s[d.pos] != '"' {
			return node{}, d.errf("expected string key in object")
		}
		keyStart := d.pos
		key, err := d.parseString()
		if err != nil {
			return node{}, err
		}
		n := len(d.keys) - kbase
		dup := false
		switch {
		case seen != nil:
			dup = seen[key]
			seen[key] = true
		case n < smallObject:
			for _, k := range d.keys[kbase:] {
				if k == key {
					dup = true
					break
				}
			}
		default:
			seen = make(map[string]bool, 2*n)
			for _, k := range d.keys[kbase:] {
				seen[k] = true
			}
			dup = seen[key]
			seen[key] = true
		}
		if dup {
			d.pos = keyStart
			return node{}, d.errf("duplicate key %q in object", key)
		}
		d.skipWS()
		if d.pos >= len(d.s) || d.s[d.pos] != ':' {
			return node{}, d.errf("expected ':' after object key")
		}
		d.pos++
		d.skipWS()
		// Push the key before parsing the value: a nested container pushes and
		// pops its own run above it, so this object's keys and values stay
		// aligned at [kbase, len) / [ebase, len).
		d.keys = append(d.keys, key)
		val, err := d.parseValue()
		if err != nil {
			return node{}, err
		}
		d.elems = append(d.elems, val)
		d.skipWS()
		if d.pos >= len(d.s) {
			return node{}, d.errf("unterminated object")
		}
		if d.s[d.pos] == ',' {
			d.pos++
			continue
		}
		if d.s[d.pos] == '}' {
			d.pos++
			keys := append([]string(nil), d.keys[kbase:]...)
			elems := append([]node(nil), d.elems[ebase:]...)
			d.keys = d.keys[:kbase]
			d.elems = d.elems[:ebase]
			return doctree.Map(keys, elems), nil
		}
		return node{}, d.errf("expected ',' or '}' in object")
	}
}

func (d *decoder) parseArray() (node, error) {
	d.pos++ // '['
	base := len(d.elems)
	d.skipWS()
	if d.pos < len(d.s) && d.s[d.pos] == ']' {
		d.pos++
		return doctree.List([]node{}), nil
	}
	for {
		d.skipWS()
		v, err := d.parseValue()
		if err != nil {
			return node{}, err
		}
		d.elems = append(d.elems, v)
		d.skipWS()
		if d.pos >= len(d.s) {
			return node{}, d.errf("unterminated array")
		}
		if d.s[d.pos] == ',' {
			d.pos++
			continue
		}
		if d.s[d.pos] == ']' {
			d.pos++
			elems := append([]node(nil), d.elems[base:]...)
			d.elems = d.elems[:base]
			return doctree.List(elems), nil
		}
		return node{}, d.errf("expected ',' or ']' in array")
	}
}

func (d *decoder) parseString() (string, error) {
	d.pos++ // opening quote
	// Fast path: a string with no escape is returned as a slice of the input,
	// with no copy or allocation - most keys and values in real documents. The
	// decoded tree then shares the input text, which stays alive while any of
	// those strings does; the tree is held in place of the text, so that is the
	// cheaper side of the trade.
	start := d.pos
	for i := d.pos; i < len(d.s); i++ {
		c := d.s[i]
		if c == '"' {
			d.pos = i + 1
			return d.s[start:i], nil
		}
		if c == '\\' || c < 0x20 {
			break
		}
	}
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
func (d *decoder) parseKeyword() (node, error) {
	switch {
	case strings.HasPrefix(d.s[d.pos:], "true"):
		d.pos += 4
		return doctree.Bool(true), nil
	case strings.HasPrefix(d.s[d.pos:], "false"):
		d.pos += 5
		return doctree.Bool(false), nil
	case strings.HasPrefix(d.s[d.pos:], "null"):
		d.pos += 4
		return doctree.Null(), nil
	}
	return node{}, d.errf("invalid literal")
}

// parseNumber scans a JSON number and returns int when it has no fractional
// or exponent part (and fits int64), else float. The grammar is walked
// explicitly rather than delegated to strconv, so json.org's rules are
// enforced strictly: no leading zeros (`01`), a fraction needs at least one
// digit (`1.` is invalid), and an exponent needs at least one digit (`1e`).
// strconv accepts all of those, so a permissive scan-then-parse would leak
// them through.
func (d *decoder) parseNumber() (node, error) {
	start := d.pos
	if d.pos < len(d.s) && d.s[d.pos] == '-' {
		d.pos++
	}
	// integer part: a lone 0, or a nonzero digit followed by more digits.
	if d.pos >= len(d.s) || !isDigit(d.s[d.pos]) {
		return node{}, d.errf("invalid number: expected a digit")
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
			return node{}, d.errf("invalid number: a fraction needs at least one digit")
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
			return node{}, d.errf("invalid number: an exponent needs at least one digit")
		}
		for d.pos < len(d.s) && isDigit(d.s[d.pos]) {
			d.pos++
		}
	}
	// A digit immediately after a lone leading 0 (`01`) never gets consumed
	// above, so it surfaces as trailing content at the call site; catch the
	// clearer message here instead.
	if d.pos < len(d.s) && isDigit(d.s[d.pos]) {
		return node{}, d.errf("invalid number: leading zeros are not allowed")
	}
	tok := d.s[start:d.pos]
	// `-0` (with no fraction/exponent) would parse to int 0 and lose its sign;
	// decode it as negative-zero float to preserve the sign, matching Go's
	// encoding/json.
	if !isFloat && tok == "-0" {
		return doctree.Float(math.Copysign(0, -1)), nil
	}
	if !isFloat {
		if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
			return doctree.Int(n), nil
		}
		// too big for int64: fall through to float
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return node{}, d.errf("invalid number %q", tok)
	}
	return doctree.Float(f), nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
