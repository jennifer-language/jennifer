// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// TOML 1.0.0 decoder. `toml.decode(text)` parses a TOML document into the same
// generic Value tree json.decode produces - tables become KindMap (in document
// order), arrays become KindList, and scalars map to the matching Jennifer
// kind - then wraps it in an opaque toml.Value (a KindObject). The one type
// TOML has that JSON lacks, the four date-time forms, is carried as an internal
// `toml.Datetime` struct node (text + form); `toml.asDatetime` turns it into a
// `time.Time`, and the encoder emits it bare. This file is the parser only; the
// accessors, write surface, and encoder live alongside it.
package tomllib

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// datetime form tags stored on a toml.Datetime node's `form` field.
const (
	formOffsetDatetime = "offset-datetime"
	formLocalDatetime  = "local-datetime"
	formLocalDate      = "local-date"
	formLocalTime      = "local-time"
)

// datetimeVal builds the internal toml.Datetime node. It is never registered as
// a namespaced struct (it only ever lives inside the opaque toml.Value tree), so
// no `.j` code can name or construct it; the accessor and encoder recognise it
// by (StructNS, StructName).
func datetimeVal(text, form string) interpreter.Value {
	return interpreter.NamespacedStructVal("toml", "Datetime", []interpreter.StructField{
		{Name: "text", Value: interpreter.StringVal(text)},
		{Name: "form", Value: interpreter.StringVal(form)},
	})
}

// isDatetimeNode reports whether v is a toml.Datetime tree node, returning its
// text and form.
func isDatetimeNode(v interpreter.Value) (text, form string, ok bool) {
	if v.Kind != interpreter.KindStruct || v.StructNS != "toml" || v.StructName != "Datetime" {
		return "", "", false
	}
	for _, f := range v.Fields {
		switch f.Name {
		case "text":
			text = f.Value.Str
		case "form":
			form = f.Value.Str
		}
	}
	return text, form, true
}

// listVal / mapVal mirror the json tree constructors so the accessors and write
// surface (near-identical to json's) build nodes the same way.
func listVal(elems []interpreter.Value) interpreter.Value {
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeNull), elems)
}

func mapVal(entries []interpreter.MapEntry) interpreter.Value {
	return interpreter.MapVal(parser.PrimitiveType(parser.TypeString), parser.PrimitiveType(parser.TypeNull), entries)
}

// decodeToml parses a full TOML document into the root table (a KindMap).
func decodeToml(src string) (interpreter.Value, error) {
	d := &decoder{src: src}
	return d.run()
}

type decoder struct {
	src  string
	pos  int
	line int // 0-based; +1 for messages
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

func (d *decoder) run() (interpreter.Value, error) {
	root := mapVal(nil)
	current := &root
	for {
		d.skipBlank()
		if d.pos >= len(d.src) {
			break
		}
		if d.peek() == '[' {
			var err error
			if d.at(1) == '[' {
				current, err = d.parseArrayTableHeader(&root)
			} else {
				current, err = d.parseTableHeader(&root)
			}
			if err != nil {
				return interpreter.Value{}, err
			}
		} else {
			if err := d.parseKeyValue(current); err != nil {
				return interpreter.Value{}, err
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
			return interpreter.Value{}, d.errf("unexpected %q after value", string(d.peek()))
		}
	}
	return root, nil
}

// ----- table headers -------------------------------------------------------

func (d *decoder) parseTableHeader(root *interpreter.Value) (*interpreter.Value, error) {
	d.advance() // '['
	d.skipInlineWS()
	segs, err := d.parseKey()
	if err != nil {
		return nil, err
	}
	d.skipInlineWS()
	if d.peek() != ']' {
		return nil, d.errf("expected ']' to close table header")
	}
	d.advance()
	cur := root
	for _, seg := range segs {
		cur, err = d.descendTable(cur, seg)
		if err != nil {
			return nil, err
		}
	}
	if cur.Kind != interpreter.KindMap {
		return nil, d.errf("table header %q targets a non-table", strings.Join(segs, "."))
	}
	return cur, nil
}

func (d *decoder) parseArrayTableHeader(root *interpreter.Value) (*interpreter.Value, error) {
	d.advance() // '['
	d.advance() // '['
	d.skipInlineWS()
	segs, err := d.parseKey()
	if err != nil {
		return nil, err
	}
	d.skipInlineWS()
	if d.peek() != ']' || d.at(1) != ']' {
		return nil, d.errf("expected ']]' to close array-of-tables header")
	}
	d.advance()
	d.advance()
	cur := root
	for i := 0; i < len(segs)-1; i++ {
		cur, err = d.descendTable(cur, segs[i])
		if err != nil {
			return nil, err
		}
	}
	last := segs[len(segs)-1]
	// Find or create the array under `last`, then append a fresh table element.
	for i := range cur.Map {
		if cur.Map[i].Key.Str == last {
			v := &cur.Map[i].Value
			if v.Kind != interpreter.KindList {
				return nil, d.errf("key %q is not an array of tables", last)
			}
			v.List = append(v.List, mapVal(nil))
			return &v.List[len(v.List)-1], nil
		}
	}
	cur.Map = append(cur.Map, interpreter.MapEntry{Key: interpreter.StringVal(last), Value: listVal([]interpreter.Value{mapVal(nil)})})
	v := &cur.Map[len(cur.Map)-1].Value
	return &v.List[0], nil
}

// descendTable returns a pointer to the child table named key under cur,
// creating an empty table if absent. An array-of-tables child descends into its
// last element (so `[a.b]` after `[[a]]` targets the newest `a`).
func (d *decoder) descendTable(cur *interpreter.Value, key string) (*interpreter.Value, error) {
	for i := range cur.Map {
		if cur.Map[i].Key.Str == key {
			v := &cur.Map[i].Value
			switch v.Kind {
			case interpreter.KindMap:
				return v, nil
			case interpreter.KindList:
				if len(v.List) == 0 || v.List[len(v.List)-1].Kind != interpreter.KindMap {
					return nil, d.errf("key %q is not a table", key)
				}
				return &v.List[len(v.List)-1], nil
			default:
				return nil, d.errf("key %q is not a table", key)
			}
		}
	}
	cur.Map = append(cur.Map, interpreter.MapEntry{Key: interpreter.StringVal(key), Value: mapVal(nil)})
	return &cur.Map[len(cur.Map)-1].Value, nil
}

// ----- key/value pairs -----------------------------------------------------

func (d *decoder) parseKeyValue(current *interpreter.Value) error {
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
	cur := current
	for i := 0; i < len(segs)-1; i++ {
		cur, err = d.descendTable(cur, segs[i])
		if err != nil {
			return err
		}
	}
	last := segs[len(segs)-1]
	for i := range cur.Map {
		if cur.Map[i].Key.Str == last {
			return d.errf("duplicate key %q", last)
		}
	}
	cur.Map = append(cur.Map, interpreter.MapEntry{Key: interpreter.StringVal(last), Value: val})
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

func (d *decoder) parseValue() (interpreter.Value, error) {
	switch c := d.peek(); {
	case c == '"':
		if d.at(1) == '"' && d.at(2) == '"' {
			s, err := d.parseMultilineBasicString()
			return interpreter.StringVal(s), err
		}
		s, err := d.parseBasicString()
		return interpreter.StringVal(s), err
	case c == '\'':
		if d.at(1) == '\'' && d.at(2) == '\'' {
			s, err := d.parseMultilineLiteralString()
			return interpreter.StringVal(s), err
		}
		s, err := d.parseLiteralString()
		return interpreter.StringVal(s), err
	case c == '[':
		return d.parseArray()
	case c == '{':
		return d.parseInlineTable()
	case c == 't' || c == 'f':
		return d.parseBool()
	case c == '+' || c == '-' || c >= '0' && c <= '9' || c == 'i' || c == 'n':
		return d.parseNumberOrDate()
	default:
		return interpreter.Value{}, d.errf("unexpected value starting with %q", string(c))
	}
}

func (d *decoder) parseBool() (interpreter.Value, error) {
	if strings.HasPrefix(d.src[d.pos:], "true") {
		d.pos += 4
		return interpreter.BoolVal(true), nil
	}
	if strings.HasPrefix(d.src[d.pos:], "false") {
		d.pos += 5
		return interpreter.BoolVal(false), nil
	}
	return interpreter.Value{}, d.errf("invalid boolean")
}

// ----- strings -------------------------------------------------------------

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
			// Up to two extra quotes may hug the closing delimiter.
			for d.peek() == '"' {
				sb.WriteByte('"')
				d.pos++
			}
			return sb.String(), nil
		}
		c := d.src[d.pos]
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
			for d.peek() == '\'' {
				sb.WriteByte('\'')
				d.pos++
			}
			return sb.String(), nil
		}
		c := d.src[d.pos]
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
	d.pos += n
	return string(rune(cp)), nil
}

// ----- arrays and inline tables --------------------------------------------

func (d *decoder) parseArray() (interpreter.Value, error) {
	d.advance() // '['
	elems := []interpreter.Value{}
	for {
		d.skipArrayGaps()
		if d.peek() == ']' {
			d.advance()
			return listVal(elems), nil
		}
		if d.pos >= len(d.src) {
			return interpreter.Value{}, d.errf("unterminated array")
		}
		v, err := d.parseValue()
		if err != nil {
			return interpreter.Value{}, err
		}
		elems = append(elems, v)
		d.skipArrayGaps()
		if d.peek() == ',' {
			d.advance()
			continue
		}
		if d.peek() == ']' {
			d.advance()
			return listVal(elems), nil
		}
		return interpreter.Value{}, d.errf("expected ',' or ']' in array")
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

func (d *decoder) parseInlineTable() (interpreter.Value, error) {
	d.advance() // '{'
	entries := []interpreter.MapEntry{}
	d.skipInlineWS()
	if d.peek() == '}' {
		d.advance()
		return mapVal(entries), nil
	}
	for {
		d.skipInlineWS()
		segs, err := d.parseKey()
		if err != nil {
			return interpreter.Value{}, err
		}
		d.skipInlineWS()
		if d.peek() != '=' {
			return interpreter.Value{}, d.errf("expected '=' in inline table")
		}
		d.advance()
		d.skipInlineWS()
		val, err := d.parseValue()
		if err != nil {
			return interpreter.Value{}, err
		}
		entries, err = inlineAssign(entries, segs, val, d)
		if err != nil {
			return interpreter.Value{}, err
		}
		d.skipInlineWS()
		switch d.peek() {
		case ',':
			d.advance()
		case '}':
			d.advance()
			return mapVal(entries), nil
		default:
			return interpreter.Value{}, d.errf("expected ',' or '}' in inline table")
		}
	}
}

// inlineAssign sets a (possibly dotted) key inside an inline table's entry list,
// creating intermediate tables as needed.
func inlineAssign(entries []interpreter.MapEntry, segs []string, val interpreter.Value, d *decoder) ([]interpreter.MapEntry, error) {
	if len(segs) == 1 {
		for i := range entries {
			if entries[i].Key.Str == segs[0] {
				return nil, d.errf("duplicate key %q", segs[0])
			}
		}
		return append(entries, interpreter.MapEntry{Key: interpreter.StringVal(segs[0]), Value: val}), nil
	}
	// Dotted: descend/create a sub-table.
	for i := range entries {
		if entries[i].Key.Str == segs[0] {
			if entries[i].Value.Kind != interpreter.KindMap {
				return nil, d.errf("key %q is not a table", segs[0])
			}
			sub, err := inlineAssign(entries[i].Value.Map, segs[1:], val, d)
			if err != nil {
				return nil, err
			}
			entries[i].Value = mapVal(sub)
			return entries, nil
		}
	}
	sub, err := inlineAssign(nil, segs[1:], val, d)
	if err != nil {
		return nil, err
	}
	return append(entries, interpreter.MapEntry{Key: interpreter.StringVal(segs[0]), Value: mapVal(sub)}), nil
}

// ----- numbers and datetimes -----------------------------------------------

// parseNumberOrDate classifies the token ahead as a date-time or a number. A
// date-time is recognised by its shape: four digits then '-' (a date), or two
// digits then ':' (a local time).
func (d *decoder) parseNumberOrDate() (interpreter.Value, error) {
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
func (d *decoder) parseDatetime() (interpreter.Value, error) {
	start := d.pos
	if d.looksLikeTime() {
		d.scanTime()
		return datetimeVal(d.src[start:d.pos], formLocalTime), nil
	}
	// Date: YYYY-MM-DD
	d.pos += 10
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
		return datetimeVal(d.src[start:d.pos], formLocalDate), nil
	}
	d.scanTime()
	// Optional offset: Z, z, or ±HH:MM
	off := d.peek()
	if off == 'Z' || off == 'z' {
		d.pos++
		return datetimeVal(normalizeDatetime(d.src[start:d.pos]), formOffsetDatetime), nil
	}
	if off == '+' || off == '-' {
		d.pos += 6 // ±HH:MM
		return datetimeVal(normalizeDatetime(d.src[start:d.pos]), formOffsetDatetime), nil
	}
	return datetimeVal(normalizeDatetime(d.src[start:d.pos]), formLocalDatetime), nil
}

// scanTime consumes HH:MM:SS with an optional fractional part.
func (d *decoder) scanTime() {
	d.pos += 8 // HH:MM:SS
	if d.peek() == '.' {
		d.pos++
		for isDigit(d.peek()) {
			d.pos++
		}
	}
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
func (d *decoder) parseNumber() (interpreter.Value, error) {
	start := d.pos
	for d.pos < len(d.src) && isNumberChar(d.src[d.pos]) {
		d.pos++
	}
	tok := d.src[start:d.pos]
	if tok == "" {
		return interpreter.Value{}, d.errf("expected a number")
	}
	return classifyNumber(tok, d)
}

func isNumberChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c == '.' || c == '+' || c == '-' || c == '_'
}

func classifyNumber(tok string, d *decoder) (interpreter.Value, error) {
	switch tok {
	case "inf", "+inf":
		return interpreter.FloatVal(inf(false)), nil
	case "-inf":
		return interpreter.FloatVal(inf(true)), nil
	case "nan", "+nan", "-nan":
		return interpreter.FloatVal(nan()), nil
	}
	// Prefixed integers: hex / octal / binary. No sign, no underscores adjacent
	// to the prefix (we strip all underscores leniently).
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
	// Float if it carries a '.', or an exponent, or is inf/nan (handled above).
	if strings.ContainsAny(body, ".eE") && !isDecimalInteger(body) {
		f, err := strconv.ParseFloat(body, 64)
		if err != nil {
			return interpreter.Value{}, d.errf("invalid float %q", tok)
		}
		return interpreter.FloatVal(f), nil
	}
	n, err := strconv.ParseInt(body, 10, 64)
	if err != nil {
		// A leading-zero decimal or other malformed integer.
		if f, ferr := strconv.ParseFloat(body, 64); ferr == nil {
			return interpreter.FloatVal(f), nil
		}
		return interpreter.Value{}, d.errf("invalid integer %q", tok)
	}
	return interpreter.IntVal(n), nil
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

func intOrErr(n int64, err error, tok string, d *decoder) (interpreter.Value, error) {
	if err != nil {
		return interpreter.Value{}, d.errf("invalid integer %q", tok)
	}
	return interpreter.IntVal(n), nil
}
