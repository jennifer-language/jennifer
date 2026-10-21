// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// The hand-rolled XML parser (no `encoding/xml`, no `reflect`, so TinyGo-clean
// - the same reason `json` and `toml` are hand-rolled). It reads a
// well-formed XML document char by char into the compact node tree defined in
// xmlnode.go: elements with ordered attributes and ordered children, text
// (character data), and CDATA (as text). Comments, processing instructions, an
// XML declaration, and a DOCTYPE are parsed and skipped. The five predefined
// entities and numeric character references are decoded; an unknown entity is
// an error. Namespace prefixes are kept verbatim in the element / attribute
// name (`prefix:local`); `xmlns` declarations are ordinary attributes.
package xmllib

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/limits"
)

type decoder struct {
	s   string
	pos int
	// Memoized line/column for errf, advanced from lineOff to d.pos so repeated
	// calls are O(n) total, not O(n) each. This decoder does reset pos backward
	// in a few places (entity rescans), so errf guards for that and rescans.
	lineOff int
	line    int
	col     int
	nodes   int // elements / attributes / text nodes built so far, gated by addNodes
	// Shared scratch stacks: an element's children and attributes are pushed
	// here while it is open and copied out at exactly their final size when it
	// closes, so no per-element slice is left with append's spare capacity.
	kids  []node
	attrs []attr
}

// maxDecodedNodes bounds how many nodes (elements, attributes, text runs) one
// decode may build, so a document of millions of tiny elements is a catchable
// error rather than unbounded memory. A package var so tests can lower it.
var maxDecodedNodes = limits.MaxDecodedNodes

// addNodes charges n materialised nodes and errors once the running total passes
// the cap.
func (d *decoder) addNodes(n int) error {
	d.nodes += n
	if d.nodes > maxDecodedNodes {
		return d.errf("document expands to more than %d nodes (input rejected as a decode bomb)", maxDecodedNodes)
	}
	return nil
}

// maxDepth caps element nesting. parseElement/parseContent recurse one Go frame
// per level, and a Go stack overflow is a fatal, uncatchable crash (the
// interpreter has no recover()), so an unbounded depth would make any untrusted
// document a remote kill switch. The limit is shared with json/toml and the
// language parser, and is build-tag split so it stays below the constrained
// TinyGo binary's fixed-stack crash point (see internal/limits).
const maxDepth = limits.MaxNestingDepth

// errf tags an error with the line/column of the current position. It advances
// a memoized (line, col) from lineOff to d.pos rather than rescanning from the
// start; a backward pos (entity rescan) falls back to a full rescan.
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

func (d *decoder) eof() bool  { return d.pos >= len(d.s) }
func (d *decoder) peek() byte { return d.s[d.pos] }

func (d *decoder) startsWith(prefix string) bool {
	return strings.HasPrefix(d.s[d.pos:], prefix)
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// isXMLChar reports whether r is allowed by XML 1.0's Char production: tab, LF,
// CR, then #x20..#xD7FF, #xE000..#xFFFD, #x10000..#x10FFFF. This excludes NUL
// and the other C0 controls (which utf8.ValidRune would otherwise accept), so a
// numeric reference like `&#0;` is rejected rather than injecting a control byte.
func isXMLChar(r rune) bool {
	switch {
	case r == 0x9 || r == 0xA || r == 0xD:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	default:
		return false
	}
}

func (d *decoder) skipSpace() {
	for !d.eof() && isSpace(d.peek()) {
		d.pos++
	}
}

// decodeXML parses a full document: an optional prolog, exactly one root
// element, and optional trailing misc.
func decodeXML(src string) (node, error) {
	d := &decoder{s: src}
	if err := d.skipMisc(true); err != nil {
		return node{}, err
	}
	if d.eof() || d.peek() != '<' {
		return node{}, d.errf("expected a root element")
	}
	root, err := d.parseElement(0)
	if err != nil {
		return node{}, err
	}
	if err := d.skipMisc(false); err != nil {
		return node{}, err
	}
	if !d.eof() {
		return node{}, d.errf("unexpected content after the root element")
	}
	return root, nil
}

// skipMisc skips whitespace, comments, and processing instructions between
// nodes. In the prolog (allowDoctype) it also skips an XML declaration and a
// DOCTYPE.
func (d *decoder) skipMisc(allowDoctype bool) error {
	for !d.eof() {
		switch {
		case isSpace(d.peek()):
			d.pos++
		case d.startsWith("<!--"):
			if err := d.skipComment(); err != nil {
				return err
			}
		case d.startsWith("<?"):
			if err := d.skipPI(); err != nil {
				return err
			}
		case allowDoctype && d.startsWith("<!DOCTYPE"):
			if err := d.skipDoctype(); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return nil
}

func (d *decoder) skipComment() error {
	end := strings.Index(d.s[d.pos+4:], "-->")
	if end < 0 {
		return d.errf("unterminated comment")
	}
	d.pos += 4 + end + 3
	return nil
}

func (d *decoder) skipPI() error {
	end := strings.Index(d.s[d.pos+2:], "?>")
	if end < 0 {
		return d.errf("unterminated processing instruction")
	}
	d.pos += 2 + end + 2
	return nil
}

// skipDoctype skips `<!DOCTYPE ... >`, honouring a bracketed internal subset so
// a '>' inside `[...]` does not close it early.
func (d *decoder) skipDoctype() error {
	i := d.pos + len("<!DOCTYPE")
	depth := 0
	for i < len(d.s) {
		switch d.s[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '>':
			if depth == 0 {
				d.pos = i + 1
				return nil
			}
		}
		i++
	}
	return d.errf("unterminated DOCTYPE")
}

// parseElement parses a start tag, its content, and its end tag (or a
// self-closing tag). d.pos is at the opening '<'. depth is the current nesting
// level, capped to keep the recursion from overflowing the Go stack.
func (d *decoder) parseElement(depth int) (node, error) {
	if depth >= maxDepth {
		return node{}, d.errf("element nesting exceeds %d levels", maxDepth)
	}
	if err := d.addNodes(1); err != nil {
		return node{}, err
	}
	d.pos++ // '<'
	name, err := d.parseName()
	if err != nil {
		return node{}, err
	}
	attrs, selfClose, err := d.parseAttributes()
	if err != nil {
		return node{}, err
	}
	if selfClose {
		return elementNode(name, attrs, nil), nil
	}
	children, err := d.parseContent(name, depth)
	if err != nil {
		return node{}, err
	}
	return elementNode(name, attrs, children), nil
}

// parseName reads an element or attribute name: a run of characters up to
// whitespace or one of `/ > =`. A name may not contain `< & " '` (which
// terminate other syntax) - accepting them would let a malformed name round-trip
// through encode as un-escaped markup (e.g. a tag name `a&b` re-emitted as
// `<a&b>`, where `&b` reads as an entity to the next consumer).
func (d *decoder) parseName() (string, error) {
	start := d.pos
	for !d.eof() {
		c := d.peek()
		if isSpace(c) || c == '>' || c == '/' || c == '=' {
			break
		}
		if c == '<' || c == '&' || c == '"' || c == '\'' {
			return "", d.errf("invalid character %q in a name", string(c))
		}
		d.pos++
	}
	if d.pos == start {
		return "", d.errf("expected a name")
	}
	return d.s[start:d.pos], nil
}

// parseAttributes reads zero or more `name="value"` pairs, returning whether
// the tag was self-closing (`/>`). The pairs are collected on the shared
// d.attrs stack and copied out at their final size.
func (d *decoder) parseAttributes() (attrs []attr, selfClose bool, err error) {
	base := len(d.attrs)
	// seen finds duplicates in O(1) once an element has many attributes; a
	// few are scanned directly on the stack, which needs no allocation.
	var seen map[string]bool
	defer func() { d.attrs = d.attrs[:base] }()
	for {
		d.skipSpace()
		if d.eof() {
			return nil, false, d.errf("unterminated start tag")
		}
		switch d.peek() {
		case '>':
			d.pos++
			return d.takeAttrs(base), false, nil
		case '/':
			if d.pos+1 < len(d.s) && d.s[d.pos+1] == '>' {
				d.pos += 2
				return d.takeAttrs(base), true, nil
			}
			return nil, false, d.errf("expected '>' after '/'")
		}
		aname, e := d.parseName()
		if e != nil {
			return nil, false, e
		}
		d.skipSpace()
		if d.eof() || d.peek() != '=' {
			return nil, false, d.errf("expected '=' after attribute %q", aname)
		}
		d.pos++
		d.skipSpace()
		aval, e := d.parseAttValue()
		if e != nil {
			return nil, false, e
		}
		if d.hasAttr(base, aname, &seen) {
			return nil, false, d.errf("duplicate attribute %q", aname)
		}
		if err := d.addNodes(1); err != nil {
			return nil, false, err
		}
		d.attrs = append(d.attrs, attr{name: aname, value: aval})
	}
}

// hasAttr reports whether the open element (attributes from base on) already
// has an attribute called name, and records name. Up to manyAttrs attributes
// it scans the stack; past that it builds *seen and looks up there, so an
// element with a huge attribute list stays linear.
func (d *decoder) hasAttr(base int, name string, seen *map[string]bool) bool {
	open := d.attrs[base:]
	if *seen == nil && len(open) < manyAttrs {
		for _, a := range open {
			if a.name == name {
				return true
			}
		}
		return false
	}
	if *seen == nil {
		*seen = make(map[string]bool, 2*len(open))
		for _, a := range open {
			(*seen)[a.name] = true
		}
	}
	if (*seen)[name] {
		return true
	}
	(*seen)[name] = true
	return false
}

// takeAttrs copies the open element's attributes off the shared stack.
func (d *decoder) takeAttrs(base int) []attr {
	if len(d.attrs) == base {
		return nil
	}
	return append([]attr(nil), d.attrs[base:]...)
}

// parseAttValue reads a quoted, entity-decoded attribute value.
func (d *decoder) parseAttValue() (string, error) {
	if d.eof() || (d.peek() != '"' && d.peek() != '\'') {
		return "", d.errf("attribute value must be quoted")
	}
	q := d.peek()
	d.pos++
	// Fast path: a value with no reference and no whitespace to normalize is a
	// plain substring of the input.
	for i := d.pos; i < len(d.s); i++ {
		c := d.s[i]
		if c == q {
			v := d.s[d.pos:i]
			d.pos = i + 1
			return v, nil
		}
		if c == '&' || c == '<' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
	}
	var b strings.Builder
	for {
		if d.eof() {
			return "", d.errf("unterminated attribute value")
		}
		c := d.peek()
		switch {
		case c == q:
			d.pos++
			return b.String(), nil
		case c == '<':
			return "", d.errf("'<' is not allowed in an attribute value")
		case c == '&':
			r, e := d.parseReference()
			if e != nil {
				return "", e
			}
			b.WriteString(r)
		case c == '\t' || c == '\n' || c == ' ':
			// Attribute-value normalization: a literal whitespace character
			// becomes a space (a whitespace char *reference* is left as-is,
			// handled by the '&' case above).
			b.WriteByte(' ')
			d.pos++
		case c == '\r':
			// A literal CR (or CR-LF) normalizes to a single space.
			b.WriteByte(' ')
			d.pos++
			if !d.eof() && d.peek() == '\n' {
				d.pos++
			}
		default:
			b.WriteByte(c)
			d.pos++
		}
	}
}

// parseContent reads an element's children up to and including its `</name>`.
// depth is the nesting level of the owning element, passed to child elements.
// The children are collected on the shared d.kids stack (a child element's own
// children sit above them while it is parsed) and copied out at their final size.
func (d *decoder) parseContent(name string, depth int) ([]node, error) {
	base := len(d.kids)
	defer func() { d.kids = d.kids[:base] }()
	for {
		if d.eof() {
			return nil, d.errf("element <%s> is not closed", name)
		}
		if d.peek() != '<' {
			txt, err := d.parseText()
			if err != nil {
				return nil, err
			}
			if err := d.addNodes(1); err != nil {
				return nil, err
			}
			d.kids = append(d.kids, textNode(txt))
			continue
		}
		switch {
		case d.startsWith("</"):
			if err := d.parseCloseTag(name); err != nil {
				return nil, err
			}
			if len(d.kids) == base {
				return nil, nil
			}
			return append([]node(nil), d.kids[base:]...), nil
		case d.startsWith("<!--"):
			if err := d.skipComment(); err != nil {
				return nil, err
			}
		case d.startsWith("<![CDATA["):
			txt, err := d.parseCDATA()
			if err != nil {
				return nil, err
			}
			if err := d.addNodes(1); err != nil {
				return nil, err
			}
			d.kids = append(d.kids, textNode(txt))
		case d.startsWith("<?"):
			if err := d.skipPI(); err != nil {
				return nil, err
			}
		case d.startsWith("<!"):
			return nil, d.errf("unexpected declaration inside element <%s>", name)
		default:
			child, err := d.parseElement(depth + 1)
			if err != nil {
				return nil, err
			}
			d.kids = append(d.kids, child)
		}
	}
}

func (d *decoder) parseCloseTag(name string) error {
	d.pos += 2 // "</"
	cname, err := d.parseName()
	if err != nil {
		return err
	}
	d.skipSpace()
	if d.eof() || d.peek() != '>' {
		return d.errf("expected '>' in closing tag </%s>", cname)
	}
	d.pos++
	if cname != name {
		return d.errf("closing tag </%s> does not match <%s>", cname, name)
	}
	return nil
}

func (d *decoder) parseCDATA() (string, error) {
	const open = "<![CDATA["
	end := strings.Index(d.s[d.pos+len(open):], "]]>")
	if end < 0 {
		return "", d.errf("unterminated CDATA section")
	}
	txt := d.s[d.pos+len(open) : d.pos+len(open)+end]
	d.pos += len(open) + end + 3
	return txt, nil
}

// parseText reads character data up to the next '<', decoding entities.
func (d *decoder) parseText() (string, error) {
	// Fast path: a run with no reference is a plain substring of the input.
	start := d.pos
	end := strings.IndexByte(d.s[start:], '<')
	if end < 0 {
		end = len(d.s) - start
	}
	if strings.IndexByte(d.s[start:start+end], '&') < 0 {
		d.pos = start + end
		return d.s[start:d.pos], nil
	}
	var b strings.Builder
	for !d.eof() {
		c := d.peek()
		if c == '<' {
			break
		}
		if c == '&' {
			r, e := d.parseReference()
			if e != nil {
				return "", e
			}
			b.WriteString(r)
			continue
		}
		b.WriteByte(c)
		d.pos++
	}
	return b.String(), nil
}

// parseReference decodes an entity or character reference starting at '&'.
func (d *decoder) parseReference() (string, error) {
	amp := d.pos
	rel := strings.IndexByte(d.s[amp:], ';')
	if rel < 0 {
		return "", d.errf("unterminated entity reference")
	}
	ent := d.s[amp+1 : amp+rel]
	d.pos = amp + rel + 1
	switch ent {
	case "lt":
		return "<", nil
	case "gt":
		return ">", nil
	case "amp":
		return "&", nil
	case "apos":
		return "'", nil
	case "quot":
		return "\"", nil
	}
	if strings.HasPrefix(ent, "#") {
		var code int64
		var err error
		if strings.HasPrefix(ent, "#x") || strings.HasPrefix(ent, "#X") {
			code, err = strconv.ParseInt(ent[2:], 16, 32)
		} else {
			code, err = strconv.ParseInt(ent[1:], 10, 32)
		}
		if err != nil || code < 0 || code > utf8.MaxRune || !isXMLChar(rune(code)) {
			d.pos = amp
			return "", d.errf("invalid character reference &%s;", ent)
		}
		return string(rune(code)), nil
	}
	d.pos = amp
	return "", d.errf("unknown entity &%s;", ent)
}
