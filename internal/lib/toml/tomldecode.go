// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// TOML 1.0.0 decoder. `toml.decode(text)` parses a TOML document into the
// shared compact document tree json.decode also produces (internal/lib/
// doctree) - tables become maps (in document order), arrays become lists, and
// scalars map to the matching kind - then wraps it in an opaque toml.Value (a
// KindObject). The one type TOML has that JSON lacks, the four date-time forms,
// is a datetime node (text + form tag); `toml.asDatetime` turns it into a
// `time.Time`, and the encoder emits it bare. Tables are grown in place through
// a doctree.Builder (a [header] can reopen a table after its siblings) and the
// tree is sealed at the end. This file is the parser only; the encoder lives
// alongside it, the accessors and write surface in doctree.
package tomllib

import (
	"fmt"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lib/doctree"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// datetime form tags stored on a datetime node.
const (
	formOffsetDatetime uint8 = iota
	formLocalDatetime
	formLocalDate
	formLocalTime
)

// formName spells a datetime form for an error message.
func formName(form uint8) string {
	switch form {
	case formOffsetDatetime:
		return "offset-datetime"
	case formLocalDatetime:
		return "local-datetime"
	case formLocalDate:
		return "local-date"
	case formLocalTime:
		return "local-time"
	}
	return "datetime"
}

// node is a toml.Value tree node.
type node = doctree.Node

// decodeToml parses a full TOML document into the root table (a map).
func decodeToml(src string) (node, error) {
	d := &decoder{
		src:      src,
		defined:  map[string]bool{},
		arrays:   map[string]bool{},
		inline:   map[string]bool{},
		arrayIdx: map[string]int{},
	}
	return d.run()
}

// maxNestingDepth caps container recursion in parseValue and the segment
// count of a dotted key (inlineAssign recurses per segment, and headers /
// dotted keys build one tree level per segment). Each nesting level costs Go
// stack frames, and a Go stack overflow is fatal (not a catchable Jennifer
// error), so deeply-nested untrusted input could otherwise kill the whole
// process. The limit is shared with the language parser and the other decoders,
// and is build-tag split so it stays below the constrained TinyGo binary's
// fixed-stack crash point (see internal/limits).
const maxNestingDepth = limits.MaxNestingDepth

// maxDecodedNodes bounds how many value nodes one decode may materialise, so a
// flat document (e.g. a huge array) is a catchable error rather than unbounded
// memory. A package var so tests can lower it.
var maxDecodedNodes = limits.MaxDecodedNodes

type decoder struct {
	src   string
	pos   int
	line  int // 0-based; +1 for messages
	depth int // current container nesting, gated in parseValue
	nodes int // value nodes materialised so far, gated in parseValue

	// Table-definition state (keyed by encoded dotted path) enforces TOML 1.0's
	// MUST-error redefinition rules that a plain tree build would silently merge:
	//   defined  - explicitly created by a [header] (a second [header] errors)
	//   arrays   - created by a [[array-of-tables]] header (a plain [header] on
	//              one, or a static value shadowing one, errors)
	//   inline   - value came from an inline table { } (closed: no later dotted
	//              key or header may extend it)
	defined map[string]bool
	arrays  map[string]bool
	inline  map[string]bool
	// arrayIdx tracks, per array of tables (keyed by its own stateKey), the
	// index of its current (newest) element. stateKey embeds these indices so
	// redefinition state is scoped per element.
	arrayIdx map[string]int
	curPath  []string // dotted path of the current [header] context (for key paths)

	b doctree.Builder // grows tables in place; sealed when the parse ends
}

// stateKey encodes a dotted path for the redefinition-state maps. Each
// *proper* prefix that names an array of tables is tagged with the index of
// that array's current element, because every [[header]] opens a fresh
// element namespace: `[fruit.physical]` under the second [[fruit]] is a
// different table than under the first, and must not trip the
// defined-more-than-once check. The final segment carries no index - the key
// identifies the path itself, not a child of its newest element.
func (d *decoder) stateKey(segs []string) string {
	var b strings.Builder
	for i, seg := range segs {
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(seg)
		if i < len(segs)-1 {
			if idx, ok := d.arrayIdx[b.String()]; ok {
				// The \x00 separator can't appear in a bare key and the
				// \x00\x01 index tag is vanishingly unlikely in a quoted key,
				// so neither collides with a plain path.
				fmt.Fprintf(&b, "\x00\x01%d", idx)
			}
		}
	}
	return b.String()
}

func (d *decoder) errf(format string, a ...interface{}) error {
	return fmt.Errorf("toml: line %d: %s", d.line+1, fmt.Sprintf(format, a...))
}

func (d *decoder) peek() byte {
	if d.pos >= len(d.src) {
		return 0
	}
	return d.src[d.pos]
}

func (d *decoder) at(off int) byte {
	if d.pos+off >= len(d.src) {
		return 0
	}
	return d.src[d.pos+off]
}

func (d *decoder) advance() byte {
	c := d.src[d.pos]
	d.pos++
	if c == '\n' {
		d.line++
	}
	return c
}

// skipInlineWS consumes spaces and tabs only.
func (d *decoder) skipInlineWS() {
	for d.pos < len(d.src) && (d.src[d.pos] == ' ' || d.src[d.pos] == '\t') {
		d.pos++
	}
}

// skipComment consumes a `#` comment up to (not including) the newline.
func (d *decoder) skipComment() {
	for d.pos < len(d.src) && d.src[d.pos] != '\n' {
		d.pos++
	}
}

// consumeNewline consumes a single \n or \r\n; reports whether one was seen.
func (d *decoder) consumeNewline() bool {
	if d.peek() == '\r' && d.at(1) == '\n' {
		d.pos++
		d.advance()
		return true
	}
	if d.peek() == '\n' {
		d.advance()
		return true
	}
	return false
}

// skipBlank consumes runs of inline whitespace, comments, and newlines - the
// gaps between statements.
func (d *decoder) skipBlank() {
	for d.pos < len(d.src) {
		switch d.peek() {
		case ' ', '\t':
			d.pos++
		case '\r', '\n':
			if !d.consumeNewline() {
				d.pos++
			}
		case '#':
			d.skipComment()
		default:
			return
		}
	}
}

func (d *decoder) run() (node, error) {
	root := d.b.NewMap()
	current := root
	for {
		d.skipBlank()
		if d.pos >= len(d.src) {
			break
		}
		if d.peek() == '[' {
			var err error
			if d.at(1) == '[' {
				current, err = d.parseArrayTableHeader(root)
			} else {
				current, err = d.parseTableHeader(root)
			}
			if err != nil {
				return node{}, err
			}
		} else {
			if err := d.parseKeyValue(current); err != nil {
				return node{}, err
			}
		}
		// End of statement: inline whitespace then comment / newline / EOF.
		d.skipInlineWS()
		if d.pos >= len(d.src) {
			break
		}
		switch d.peek() {
		case '#':
			d.skipComment()
		case '\n', '\r':
			d.consumeNewline()
		default:
			return node{}, d.errf("unexpected %q after value", string(d.peek()))
		}
	}
	root = d.b.Seal(root)
	// Headers, dotted keys and inline values each cap their own nesting, but
	// together they can stack deeper; bound the whole document like the other
	// decoders do. The root table is implicit, so it does not count as a level.
	if root.ExceedsDepth(maxNestingDepth + 1) {
		return node{}, fmt.Errorf("toml: document nests deeper than %d levels", maxNestingDepth)
	}
	return root, nil
}

// ----- table headers -------------------------------------------------------

func (d *decoder) parseTableHeader(root node) (node, error) {
	d.advance() // '['
	d.skipInlineWS()
	segs, err := d.parseKey()
	if err != nil {
		return node{}, err
	}
	d.skipInlineWS()
	if d.peek() != ']' {
		return node{}, d.errf("expected ']' to close table header")
	}
	d.advance()
	key := d.stateKey(segs)
	dotted := strings.Join(segs, ".")
	// A second [header] for the same path, or a [header] on an existing
	// array-of-tables, is a TOML MUST-error.
	if d.defined[key] {
		return node{}, d.errf("table %q is defined more than once", dotted)
	}
	if d.arrays[key] {
		return node{}, d.errf("%q is an array of tables, not a table", dotted)
	}
	// A header may not extend a closed inline table, neither directly nor
	// through one ([a.b] with a = {x = 1}).
	for i := range segs {
		if d.inline[d.stateKey(segs[:i+1])] {
			return node{}, d.errf("cannot extend inline table %q with a header", strings.Join(segs[:i+1], "."))
		}
	}
	cur := root
	for i, seg := range segs {
		cur, err = d.descendTable(cur, seg, d.stateKey(segs[:i+1]))
		if err != nil {
			return node{}, err
		}
	}
	if cur.Kind() != doctree.KindMap {
		return node{}, d.errf("table header %q targets a non-table", dotted)
	}
	d.defined[key] = true
	d.curPath = segs
	return cur, nil
}

func (d *decoder) parseArrayTableHeader(root node) (node, error) {
	d.advance() // '['
	d.advance() // '['
	d.skipInlineWS()
	segs, err := d.parseKey()
	if err != nil {
		return node{}, err
	}
	d.skipInlineWS()
	if d.peek() != ']' || d.at(1) != ']' {
		return node{}, d.errf("expected ']]' to close array-of-tables header")
	}
	d.advance()
	d.advance()
	key := d.stateKey(segs)
	dotted := strings.Join(segs, ".")
	// A [[header]] whose path names a plain table ([header]) or a closed inline
	// table (directly or through one) is a MUST-error.
	if d.defined[key] {
		return node{}, d.errf("%q is a table, not an array of tables", dotted)
	}
	for i := range segs {
		if d.inline[d.stateKey(segs[:i+1])] {
			return node{}, d.errf("cannot extend inline table %q as an array of tables", strings.Join(segs[:i+1], "."))
		}
	}
	cur := root
	for i := 0; i < len(segs)-1; i++ {
		cur, err = d.descendTable(cur, segs[i], d.stateKey(segs[:i+1]))
		if err != nil {
			return node{}, err
		}
	}
	last := segs[len(segs)-1]
	// Find or create the array under `last`, then append a fresh table element.
	elem := d.b.NewMap()
	if pos := d.b.Lookup(cur, last); pos >= 0 {
		v := cur.Elem(pos)
		// The key must be an array *of tables* created by an earlier
		// [[header]] - not a static array (`a = []`) that is also a list.
		if v.Kind() != doctree.KindList || !d.arrays[key] {
			return node{}, d.errf("key %q is not an array of tables", last)
		}
		d.b.Push(v, elem)
		d.arrayIdx[key]++
		d.curPath = segs
		return elem, nil
	}
	arr := d.b.NewList()
	d.b.Push(arr, elem)
	d.b.Add(cur, last, arr)
	d.arrays[key] = true
	d.arrayIdx[key] = 0
	d.curPath = segs
	return elem, nil
}

// descendTable returns a pointer to the child table named key under cur,
// creating an empty table if absent. An array-of-tables child descends into its
// last element (so `[a.b]` after `[[a]]` targets the newest `a`). Lookups go
// through the Builder's index so building a wide table stays O(N), not O(N^2).
//
// stateKey is the dotted path's redefinition-state key: a list is descended
// only when it is an array of tables made by [[header]]s - a static array
// (`a = [{x = 1}]`) is closed, so `[a.y]` may not extend it (TOML 1.0).
func (d *decoder) descendTable(cur node, key, stateKey string) (node, error) {
	pos := d.b.Lookup(cur, key)
	if pos < 0 {
		t := d.b.NewMap()
		d.b.Add(cur, key, t)
		return t, nil
	}
	v := cur.Elem(pos)
	switch v.Kind() {
	case doctree.KindMap:
		return v, nil
	case doctree.KindList:
		if !d.arrays[stateKey] || v.Len() == 0 || v.Elem(v.Len()-1).Kind() != doctree.KindMap {
			return node{}, d.errf("key %q is not a table", key)
		}
		return v.Elem(v.Len() - 1), nil
	default:
		return node{}, d.errf("key %q is not a table", key)
	}
}

// ----- key/value pairs -----------------------------------------------------

func (d *decoder) parseKeyValue(current node) error {
	segs, err := d.parseKey()
	if err != nil {
		return err
	}
	d.skipInlineWS()
	if d.peek() != '=' {
		return d.errf("expected '=' after key")
	}
	d.advance()
	d.skipInlineWS()
	val, err := d.parseValue()
	if err != nil {
		return err
	}
	// Full path relative to the current [header] context, for redefinition state.
	full := append(append([]string{}, d.curPath...), segs...)
	cur := current
	for i := 0; i < len(segs)-1; i++ {
		// A dotted key may not reach through a closed inline table.
		if d.inline[d.stateKey(full[:len(d.curPath)+i+1])] {
			return d.errf("cannot extend inline table %q", strings.Join(full[:len(d.curPath)+i+1], "."))
		}
		cur, err = d.descendTable(cur, segs[i], d.stateKey(full[:len(d.curPath)+i+1]))
		if err != nil {
			return err
		}
	}
	last := segs[len(segs)-1]
	if d.b.Lookup(cur, last) >= 0 {
		return d.errf("duplicate key %q", last)
	}
	d.b.Add(cur, last, val)
	// A value that is itself an inline table is closed: no later dotted key or
	// header may extend it.
	if val.Kind() == doctree.KindMap {
		d.inline[d.stateKey(full)] = true
	}
	return nil
}

// parseKey reads a dotted key: one or more segments separated by '.', each a
// bare word, a basic string, or a literal string.
func (d *decoder) parseKey() ([]string, error) {
	var segs []string
	for {
		d.skipInlineWS()
		seg, err := d.parseKeySegment()
		if err != nil {
			return nil, err
		}
		segs = append(segs, seg)
		if len(segs) > maxNestingDepth {
			return nil, d.errf("key nesting exceeds %d segments", maxNestingDepth)
		}
		d.skipInlineWS()
		if d.peek() == '.' {
			d.advance()
			continue
		}
		return segs, nil
	}
}

func (d *decoder) parseKeySegment() (string, error) {
	switch c := d.peek(); {
	case c == '"':
		return d.parseBasicString()
	case c == '\'':
		return d.parseLiteralString()
	case isBareKeyChar(c):
		start := d.pos
		for d.pos < len(d.src) && isBareKeyChar(d.src[d.pos]) {
			d.pos++
		}
		return d.src[start:d.pos], nil
	default:
		return "", d.errf("expected a key")
	}
}

func isBareKeyChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// ----- values --------------------------------------------------------------

func (d *decoder) parseValue() (node, error) {
	d.nodes++
	if d.nodes > maxDecodedNodes {
		return node{}, d.errf("document expands to more than %d nodes (input rejected as a decode bomb)", maxDecodedNodes)
	}
	switch c := d.peek(); {
	case c == '"':
		if d.at(1) == '"' && d.at(2) == '"' {
			s, err := d.parseMultilineBasicString()
			return doctree.String(s), err
		}
		s, err := d.parseBasicString()
		return doctree.String(s), err
	case c == '\'':
		if d.at(1) == '\'' && d.at(2) == '\'' {
			s, err := d.parseMultilineLiteralString()
			return doctree.String(s), err
		}
		s, err := d.parseLiteralString()
		return doctree.String(s), err
	case c == '[':
		if d.depth >= maxNestingDepth {
			return node{}, d.errf("nesting exceeds %d levels", maxNestingDepth)
		}
		d.depth++
		v, err := d.parseArray()
		d.depth--
		return v, err
	case c == '{':
		if d.depth >= maxNestingDepth {
			return node{}, d.errf("nesting exceeds %d levels", maxNestingDepth)
		}
		d.depth++
		v, err := d.parseInlineTable()
		d.depth--
		return v, err
	case c == 't' || c == 'f':
		return d.parseBool()
	case c == '+' || c == '-' || c >= '0' && c <= '9' || c == 'i' || c == 'n':
		return d.parseNumberOrDate()
	default:
		return node{}, d.errf("unexpected value starting with %q", string(c))
	}
}

func (d *decoder) parseBool() (node, error) {
	if strings.HasPrefix(d.src[d.pos:], "true") {
		d.pos += 4
		return doctree.Bool(true), nil
	}
	if strings.HasPrefix(d.src[d.pos:], "false") {
		d.pos += 5
		return doctree.Bool(false), nil
	}
	return node{}, d.errf("invalid boolean")
}

// ----- strings -------------------------------------------------------------

// isTOMLControl reports whether c is a control character TOML forbids in string
// content: U+0000-U+001F (except tab) and U+007F (DEL). Newlines are handled by
// each scanner's own line rules, so callers check for them first.
func isTOMLControl(c byte) bool {
	return (c < 0x20 && c != '\t') || c == 0x7f
}

func (d *decoder) parseBasicString() (string, error) {
	d.advance() // opening "
	var sb strings.Builder
	for {
		if d.pos >= len(d.src) {
			return "", d.errf("unterminated string")
		}
		c := d.src[d.pos]
		if c == '"' {
			d.pos++
			return sb.String(), nil
		}
		if c == '\n' {
			return "", d.errf("newline in single-line string")
		}
		// A raw control byte (including a lone CR) is invalid in a basic string.
		if isTOMLControl(c) {
			return "", d.errf("control character U+%04X in string", c)
		}
		if c == '\\' {
			r, err := d.parseEscape()
			if err != nil {
				return "", err
			}
			sb.WriteString(r)
			continue
		}
		sb.WriteByte(c)
		d.pos++
	}
}

func (d *decoder) parseMultilineBasicString() (string, error) {
	d.pos += 3 // """
	// A newline immediately after the opening delimiter is trimmed.
	if d.peek() == '\r' && d.at(1) == '\n' {
		d.pos += 2
	} else if d.peek() == '\n' {
		d.advance()
	}
	var sb strings.Builder
	for {
		if d.pos >= len(d.src) {
			return "", d.errf("unterminated multiline string")
		}
		if d.peek() == '"' && d.at(1) == '"' && d.at(2) == '"' {
			d.pos += 3
			// At most two extra quotes may hug the closing delimiter; a third
			// would be a separate token, not string content.
			for extra := 0; extra < 2 && d.peek() == '"'; extra++ {
				sb.WriteByte('"')
				d.pos++
			}
			return sb.String(), nil
		}
		c := d.src[d.pos]
		// Allow tab and newlines (incl. CRLF); reject other raw control bytes,
		// including a lone CR not part of a CRLF pair.
		if isTOMLControl(c) && c != '\r' && c != '\n' {
			return "", d.errf("control character U+%04X in multiline string", c)
		}
		if c == '\r' && d.at(1) != '\n' {
			return "", d.errf("lone carriage return in multiline string")
		}
		if c == '\\' {
			// Line-ending backslash: trim the newline and following whitespace.
			j := d.pos + 1
			for j < len(d.src) && (d.src[j] == ' ' || d.src[j] == '\t' || d.src[j] == '\r') {
				j++
			}
			if j < len(d.src) && d.src[j] == '\n' {
				for d.pos < len(d.src) && (d.src[d.pos] == ' ' || d.src[d.pos] == '\t' || d.src[d.pos] == '\r' || d.src[d.pos] == '\n' || d.src[d.pos] == '\\') {
					if d.src[d.pos] == '\n' {
						d.line++
					}
					d.pos++
					if d.pos > 0 && d.src[d.pos-1] == '\\' {
						// consume trailing whitespace/newlines after the backslash
						for d.pos < len(d.src) && (d.src[d.pos] == ' ' || d.src[d.pos] == '\t' || d.src[d.pos] == '\r' || d.src[d.pos] == '\n') {
							if d.src[d.pos] == '\n' {
								d.line++
							}
							d.pos++
						}
						break
					}
				}
				continue
			}
			r, err := d.parseEscape()
			if err != nil {
				return "", err
			}
			sb.WriteString(r)
			continue
		}
		if c == '\n' {
			d.line++
		}
		sb.WriteByte(c)
		d.pos++
	}
}

func (d *decoder) parseLiteralString() (string, error) {
	d.advance() // opening '
	start := d.pos
	for {
		if d.pos >= len(d.src) {
			return "", d.errf("unterminated literal string")
		}
		c := d.src[d.pos]
		if c == '\'' {
			s := d.src[start:d.pos]
			d.pos++
			return s, nil
		}
		if c == '\n' {
			return "", d.errf("newline in single-line literal string")
		}
		// Literal strings allow tab but no other control byte (incl. lone CR).
		if isTOMLControl(c) {
			return "", d.errf("control character U+%04X in literal string", c)
		}
		d.pos++
	}
}

func (d *decoder) parseMultilineLiteralString() (string, error) {
	d.pos += 3 // '''
	if d.peek() == '\r' && d.at(1) == '\n' {
		d.pos += 2
	} else if d.peek() == '\n' {
		d.advance()
	}
	var sb strings.Builder
	for {
		if d.pos >= len(d.src) {
			return "", d.errf("unterminated multiline literal string")
		}
		if d.peek() == '\'' && d.at(1) == '\'' && d.at(2) == '\'' {
			d.pos += 3
			// At most two extra quotes may hug the closing delimiter.
			for extra := 0; extra < 2 && d.peek() == '\''; extra++ {
				sb.WriteByte('\'')
				d.pos++
			}
			return sb.String(), nil
		}
		c := d.src[d.pos]
		if isTOMLControl(c) && c != '\r' && c != '\n' {
			return "", d.errf("control character U+%04X in multiline literal string", c)
		}
		if c == '\r' && d.at(1) != '\n' {
			return "", d.errf("lone carriage return in multiline literal string")
		}
		if c == '\n' {
			d.line++
		}
		sb.WriteByte(c)
		d.pos++
	}
}

// parseEscape consumes a backslash escape (the backslash is at d.pos) and
// returns the decoded text.
func (d *decoder) parseEscape() (string, error) {
	d.pos++ // backslash
	if d.pos >= len(d.src) {
		return "", d.errf("dangling escape")
	}
	c := d.src[d.pos]
	d.pos++
	switch c {
	case 'b':
		return "\b", nil
	case 't':
		return "\t", nil
	case 'n':
		return "\n", nil
	case 'f':
		return "\f", nil
	case 'r':
		return "\r", nil
	case '"':
		return "\"", nil
	case '\\':
		return "\\", nil
	case 'u':
		return d.parseUnicodeEscape(4)
	case 'U':
		return d.parseUnicodeEscape(8)
	default:
		return "", d.errf("invalid escape \\%c", c)
	}
}

func (d *decoder) parseUnicodeEscape(n int) (string, error) {
	if d.pos+n > len(d.src) {
		return "", d.errf("truncated unicode escape")
	}
	hex := d.src[d.pos : d.pos+n]
	cp, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "", d.errf("invalid unicode escape %q", hex)
	}
	// TOML requires a valid Unicode scalar value: reject surrogates and
	// out-of-range code points rather than silently substituting U+FFFD.
	if cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
		return "", d.errf("unicode escape %q is not a valid scalar value", hex)
	}
	d.pos += n
	return string(rune(cp)), nil
}

// ----- arrays and inline tables --------------------------------------------

func (d *decoder) parseArray() (node, error) {
	d.advance() // '['
	elems := []node{}
	for {
		d.skipArrayGaps()
		if d.peek() == ']' {
			d.advance()
			return doctree.List(elems), nil
		}
		if d.pos >= len(d.src) {
			return node{}, d.errf("unterminated array")
		}
		v, err := d.parseValue()
		if err != nil {
			return node{}, err
		}
		elems = append(elems, v)
		d.skipArrayGaps()
		if d.peek() == ',' {
			d.advance()
			continue
		}
		if d.peek() == ']' {
			d.advance()
			return doctree.List(elems), nil
		}
		return node{}, d.errf("expected ',' or ']' in array")
	}
}

// skipArrayGaps skips whitespace, newlines, and comments allowed between array
// elements.
func (d *decoder) skipArrayGaps() {
	for d.pos < len(d.src) {
		switch d.peek() {
		case ' ', '\t':
			d.pos++
		case '\r', '\n':
			if !d.consumeNewline() {
				d.pos++
			}
		case '#':
			d.skipComment()
		default:
			return
		}
	}
}

func (d *decoder) parseInlineTable() (node, error) {
	d.advance() // '{'
	tbl := d.b.NewMap()
	d.skipInlineWS()
	if d.peek() == '}' {
		d.advance()
		return tbl, nil
	}
	for {
		d.skipInlineWS()
		segs, err := d.parseKey()
		if err != nil {
			return node{}, err
		}
		d.skipInlineWS()
		if d.peek() != '=' {
			return node{}, d.errf("expected '=' in inline table")
		}
		d.advance()
		d.skipInlineWS()
		val, err := d.parseValue()
		if err != nil {
			return node{}, err
		}
		if err := inlineAssign(tbl, segs, val, d); err != nil {
			return node{}, err
		}
		d.skipInlineWS()
		switch d.peek() {
		case ',':
			d.advance()
		case '}':
			d.advance()
			return tbl, nil
		default:
			return node{}, d.errf("expected ',' or '}' in inline table")
		}
	}
}

// inlineAssign sets a (possibly dotted) key inside an inline table, creating
// intermediate tables as needed. Duplicate detection goes through the
// Builder's index so a wide inline table builds in O(N).
func inlineAssign(tbl node, segs []string, val node, d *decoder) error {
	if len(segs) == 1 {
		if d.b.Lookup(tbl, segs[0]) >= 0 {
			return d.errf("duplicate key %q", segs[0])
		}
		d.b.Add(tbl, segs[0], val)
		return nil
	}
	// Dotted: descend/create a sub-table.
	pos := d.b.Lookup(tbl, segs[0])
	if pos < 0 {
		pos = d.b.Add(tbl, segs[0], d.b.NewMap())
	}
	sub := tbl.Elem(pos)
	if sub.Kind() != doctree.KindMap {
		return d.errf("key %q is not a table", segs[0])
	}
	return inlineAssign(sub, segs[1:], val, d)
}

// ----- numbers and datetimes -----------------------------------------------

// parseNumberOrDate classifies the token ahead as a date-time or a number. A
// date-time is recognised by its shape: four digits then '-' (a date), or two
// digits then ':' (a local time).
func (d *decoder) parseNumberOrDate() (node, error) {
	if d.looksLikeDate() || d.looksLikeTime() {
		return d.parseDatetime()
	}
	return d.parseNumber()
}

func (d *decoder) looksLikeDate() bool {
	return isDigit(d.at(0)) && isDigit(d.at(1)) && isDigit(d.at(2)) && isDigit(d.at(3)) && d.at(4) == '-'
}

func (d *decoder) looksLikeTime() bool {
	return isDigit(d.at(0)) && isDigit(d.at(1)) && d.at(2) == ':'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// parseDatetime consumes one of the four RFC 3339 / TOML date-time forms and
// returns a toml.Datetime node tagged with the form.
func (d *decoder) parseDatetime() (node, error) {
	start := d.pos
	if d.looksLikeTime() {
		if err := d.scanTime(); err != nil {
			return node{}, err
		}
		text := d.src[start:d.pos]
		if err := d.validateDatetime(text, formLocalTime); err != nil {
			return node{}, err
		}
		return doctree.Datetime(text, formLocalTime), nil
	}
	// Date: YYYY-MM-DD, validating each position rather than blindly advancing
	// (which could swallow a newline or non-digit into the token).
	for _, want := range []byte{'D', 'D', 'D', 'D', '-', 'D', 'D', '-', 'D', 'D'} {
		if want == 'D' {
			if err := d.consumeDigit("date"); err != nil {
				return node{}, err
			}
			continue
		}
		if err := d.consumeExact(want, "date"); err != nil {
			return node{}, err
		}
	}
	// A local date unless a time follows (separated by 'T', 't', or a space).
	sep := d.peek()
	hasTime := false
	if sep == 'T' || sep == 't' {
		d.pos++
		hasTime = true
	} else if sep == ' ' && isDigit(d.at(1)) && isDigit(d.at(2)) && d.at(3) == ':' {
		d.pos++
		hasTime = true
	}
	if !hasTime {
		text := d.src[start:d.pos]
		if err := d.validateDatetime(text, formLocalDate); err != nil {
			return node{}, err
		}
		return doctree.Datetime(text, formLocalDate), nil
	}
	if err := d.scanTime(); err != nil {
		return node{}, err
	}
	// Optional offset: Z, z, or +HH:MM / -HH:MM
	off := d.peek()
	if off == 'Z' || off == 'z' {
		d.pos++
		return d.datetimeChecked(d.src[start:d.pos], formOffsetDatetime)
	}
	if off == '+' || off == '-' {
		// +HH:MM: validate each position rather than a blind 6-byte advance.
		d.pos++ // sign
		for _, want := range []byte{'D', 'D', ':', 'D', 'D'} {
			if want == 'D' {
				if err := d.consumeDigit("date-time offset"); err != nil {
					return node{}, err
				}
				continue
			}
			if err := d.consumeExact(want, "date-time offset"); err != nil {
				return node{}, err
			}
		}
		return d.datetimeChecked(d.src[start:d.pos], formOffsetDatetime)
	}
	return d.datetimeChecked(d.src[start:d.pos], formLocalDatetime)
}

// datetimeChecked normalizes the raw token, verifies it parses under its form,
// and returns the node (or a positioned error for an out-of-range field).
func (d *decoder) datetimeChecked(raw string, form uint8) (node, error) {
	norm := normalizeDatetime(raw)
	if err := d.validateDatetime(norm, form); err != nil {
		return node{}, err
	}
	return doctree.Datetime(norm, form), nil
}

// validateDatetime re-parses a scanned token with Go's time package so
// out-of-range fields (month 13, day 99, hour 25) are rejected instead of
// stored as garbage that toml.encode would re-emit.
func (d *decoder) validateDatetime(text string, form uint8) error {
	if _, err := parseDatetimeText(text, form); err != nil {
		return d.errf("invalid %s %q", formName(form), text)
	}
	return nil
}

// consumeDigit advances over one digit, erroring (without swallowing a stray
// byte into the token) if the next character isn't 0-9.
func (d *decoder) consumeDigit(ctx string) error {
	if d.pos >= len(d.src) || !isDigit(d.src[d.pos]) {
		return d.errf("invalid %s: expected a digit", ctx)
	}
	d.pos++
	return nil
}

// consumeExact advances over one byte requiring it equals want. It never
// consumes a newline into a date-time token, so d.line stays accurate.
func (d *decoder) consumeExact(want byte, ctx string) error {
	if d.pos >= len(d.src) || d.src[d.pos] != want {
		return d.errf("invalid %s: expected %q", ctx, string(want))
	}
	d.pos++
	return nil
}

// scanTime consumes HH:MM:SS with an optional fractional part, validating each
// position rather than blindly advancing 8 bytes (which could swallow a newline
// or non-digit into the token and desync the line counter).
func (d *decoder) scanTime() error {
	for _, want := range []byte{'D', 'D', ':', 'D', 'D', ':', 'D', 'D'} {
		if want == 'D' {
			if err := d.consumeDigit("time"); err != nil {
				return err
			}
			continue
		}
		if err := d.consumeExact(want, "time"); err != nil {
			return err
		}
	}
	if d.peek() == '.' {
		d.pos++
		if !isDigit(d.peek()) {
			return d.errf("invalid time: fractional second needs at least one digit")
		}
		for isDigit(d.peek()) {
			d.pos++
		}
	}
	return nil
}

// normalizeDatetime canonicalises the date/time separator to 'T' and a trailing
// 'z' to 'Z' so the stored text is RFC 3339.
func normalizeDatetime(s string) string {
	if len(s) > 10 && (s[10] == ' ' || s[10] == 't') {
		s = s[:10] + "T" + s[11:]
	}
	if strings.HasSuffix(s, "z") {
		s = s[:len(s)-1] + "Z"
	}
	return s
}

// parseNumber consumes an integer or float token (no whitespace inside).
func (d *decoder) parseNumber() (node, error) {
	start := d.pos
	for d.pos < len(d.src) && isNumberChar(d.src[d.pos]) {
		d.pos++
	}
	tok := d.src[start:d.pos]
	if tok == "" {
		return node{}, d.errf("expected a number")
	}
	return classifyNumber(tok, d)
}

func isNumberChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c == '.' || c == '+' || c == '-' || c == '_'
}

func classifyNumber(tok string, d *decoder) (node, error) {
	switch tok {
	case "inf", "+inf":
		return doctree.Float(inf(false)), nil
	case "-inf":
		return doctree.Float(inf(true)), nil
	case "nan", "+nan", "-nan":
		return doctree.Float(nan()), nil
	}
	// Underscores must sit between digits (strconv silently strips them, so
	// without this `1__0`, `_1`, `1_` would slip through).
	if !validateUnderscores(tok) {
		return node{}, d.errf("invalid number %q: misplaced underscore", tok)
	}
	// Prefixed integers: hex / octal / binary. No sign, no underscores adjacent
	// to the prefix (we strip all underscores after the check above).
	body := strings.ReplaceAll(tok, "_", "")
	if len(body) > 2 && body[0] == '0' {
		switch body[1] {
		case 'x', 'X':
			n, err := strconv.ParseInt(body[2:], 16, 64)
			return intOrErr(n, err, tok, d)
		case 'o', 'O':
			n, err := strconv.ParseInt(body[2:], 8, 64)
			return intOrErr(n, err, tok, d)
		case 'b', 'B':
			n, err := strconv.ParseInt(body[2:], 2, 64)
			return intOrErr(n, err, tok, d)
		}
	}
	// A plain decimal integer or float. strconv accepts leading zeros (`042`),
	// bare-dot floats (`.5`, `5.`, `3.e2`), and empty exponents, all of which
	// TOML rejects - validate the grammar explicitly first.
	if !validateDecimalGrammar(body) {
		return node{}, d.errf("invalid number %q", tok)
	}
	// Float if it carries a '.', or an exponent, or is inf/nan (handled above).
	if strings.ContainsAny(body, ".eE") && !isDecimalInteger(body) {
		f, err := strconv.ParseFloat(body, 64)
		if err != nil {
			return node{}, d.errf("invalid float %q", tok)
		}
		return doctree.Float(f), nil
	}
	n, err := strconv.ParseInt(body, 10, 64)
	if err != nil {
		// TOML 1.0 integers are 64-bit signed; a value past that range is a
		// decode error, not a silent lossy-float downgrade. (`json` keeps its
		// deliberate integral-past-int64 -> float fallback; TOML does not.)
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return node{}, d.errf("integer %q is out of range for a 64-bit signed TOML integer", tok)
		}
		return node{}, d.errf("invalid integer %q", tok)
	}
	return doctree.Int(n), nil
}

// validateUnderscores rejects TOML underscore misuse: a `_` must sit between
// two digits *of the token's base*. strconv strips underscores before
// parsing, so `1__0`, `_1`, and `1_` would otherwise parse fine - and the
// digit class must match the base, or decimal `1_e5` slips through with `e`
// counting as a hex digit.
func validateUnderscores(tok string) bool {
	digit := func(c byte) bool { return c >= '0' && c <= '9' }
	if len(tok) > 2 && tok[0] == '0' {
		switch tok[1] {
		case 'x', 'X':
			digit = func(c byte) bool {
				return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
			}
		case 'o', 'O':
			digit = func(c byte) bool { return c >= '0' && c <= '7' }
		case 'b', 'B':
			digit = func(c byte) bool { return c == '0' || c == '1' }
		}
	}
	for i := 0; i < len(tok); i++ {
		if tok[i] != '_' {
			continue
		}
		if i == 0 || i == len(tok)-1 {
			return false
		}
		if !digit(tok[i-1]) || !digit(tok[i+1]) {
			return false
		}
	}
	return true
}

// validateDecimalGrammar enforces TOML's decimal integer/float rules on an
// underscore-stripped token: a leading digit (no bare-dot `.5`), no leading
// zeros on a multi-digit integer part (`042`), a digit on both sides of the
// decimal point (`5.`, `3.e2`), and digits in the exponent (`1e`).
func validateDecimalGrammar(body string) bool {
	i := 0
	if i < len(body) && (body[i] == '+' || body[i] == '-') {
		i++
	}
	intStart := i
	for i < len(body) && body[i] >= '0' && body[i] <= '9' {
		i++
	}
	intLen := i - intStart
	if intLen == 0 {
		return false // needs an integer part (rejects ".5")
	}
	if intLen > 1 && body[intStart] == '0' {
		return false // no leading zeros (rejects "042")
	}
	if i < len(body) && body[i] == '.' {
		i++
		fracStart := i
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		if i-fracStart == 0 {
			return false // fraction needs a digit (rejects "5." and "3.e2")
		}
	}
	if i < len(body) && (body[i] == 'e' || body[i] == 'E') {
		i++
		if i < len(body) && (body[i] == '+' || body[i] == '-') {
			i++
		}
		expStart := i
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		if i-expStart == 0 {
			return false // exponent needs a digit (rejects "1e")
		}
	}
	return i == len(body) // no trailing junk
}

// isDecimalInteger reports whether body is a plain base-10 integer (so a token
// like "1e5" is treated as float but "10" is not misrouted).
func isDecimalInteger(body string) bool {
	i := 0
	if i < len(body) && (body[i] == '+' || body[i] == '-') {
		i++
	}
	if i >= len(body) {
		return false
	}
	for ; i < len(body); i++ {
		if body[i] < '0' || body[i] > '9' {
			return false
		}
	}
	return true
}

func intOrErr(n int64, err error, tok string, d *decoder) (node, error) {
	if err != nil {
		return node{}, d.errf("invalid integer %q", tok)
	}
	return doctree.Int(n), nil
}
