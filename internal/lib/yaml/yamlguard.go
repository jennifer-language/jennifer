// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// The pre-parse depth guard. yaml.v3 builds its node tree recursively, one
// Go call per collection level, on the stack of the calling goroutine. On
// jennifer-tiny that stack is fixed and overflows a few hundred levels down,
// and a stack overflow there is a fatal crash, not an error. So before
// yaml.v3 sees the text, scanDepth measures how deep its collections nest and
// guardDepth refuses anything past maxParseDepth.
//
// The measure has to be an upper bound for every input, hostile ones
// included, or the guard is a speed bump. A heuristic scan is not: whether a
// `'` opens a quoted scalar or sits inside a plain one (`it's`), whether a
// line continues a multi-line plain scalar, or where a block scalar ends all
// decide which brackets and indents are structure, and a scan that guesses
// can be steered into skipping real nesting. scanDepth therefore follows
// yaml.v3's own scanner (scannerc.go, v3.0.1) token by token, for the parts
// that decide structure: where each token starts and ends, the block indent
// stack (roll / unroll), simple keys (one slot per flow level), plain-scalar
// continuation lines, quoted and block scalars, comments, anchors, tags and
// document markers. Where yaml.v3 would stop with a scanner error the scan
// goes on regardless: yaml.v3 parses nothing past that point, so whatever the
// scan counts there can only raise the bound, never lower it.
// TestScanDepthBoundsParser checks the bound against yaml.v3's real tree.

package yamllib

import (
	"fmt"
	"unicode/utf8"
)

// maxParseDepth caps structural nesting before the input reaches yaml.v3:
// far above any real document and comfortably below jennifer-tiny's crash
// point (measured at ~350-400 levels).
const maxParseDepth = 128

// guardDepth rejects src when its collections nest deeper than
// maxParseDepth.
func guardDepth(src string) error {
	// yaml.v3 reads a UTF-16 byte-order mark and decodes the rest as UTF-16,
	// which this byte scan would misread. Such text is never valid UTF-8, and
	// neither is anything else a Jennifer string should hold, so it is
	// refused outright.
	if !utf8.ValidString(src) {
		return fmt.Errorf("yaml.decode: input is not valid UTF-8")
	}
	if scanDepth(src, maxParseDepth) > maxParseDepth {
		return fmt.Errorf("yaml.decode: nesting too deep (limit %d); refusing to parse to avoid a stack overflow", maxParseDepth)
	}
	return nil
}

// blockLevel is one entry of the block indent stack: a block collection
// opened at a column. indentless records that a sequence was opened at this
// mapping's own column (`key:` followed by `- item` at the key's
// indentation), a level the stack does not otherwise show.
type blockLevel struct {
	column     int
	isMap      bool
	indentless bool
}

// simpleKey is a possible implicit key: where it started, so a following `:`
// opens a mapping at that column. peak is the deepest nesting seen since it
// started: the mapping a `:` opens is inserted before the key, so it wraps
// everything the key itself nested (`[a, [b]]: c` is three levels deep).
type simpleKey struct {
	possible bool
	line     int
	column   int
	index    int
	peak     int
	flows    int // parser flow collections open where it started
}

// flowLevel is one flow collection open in yaml.v3's parser. A `:` or `?`
// inside a flow sequence makes an implicit single-pair mapping (`[a: b]` is a
// list holding a map), one more level, counted open until the next `,` or the
// closing bracket.
//
// The parser's flow nesting is tracked apart from the scanner's flow level
// (flow): they differ because of a parser quirk inherited from libyaml.
// Right after a `?` in a flow sequence, the parser skips a following `]`, `,`
// or `:` token outright (parse_flow_sequence_entry_mapping_key), so in
// `[?],[?],[?],]]]` each `]` lowers the scanner's level but closes nothing in
// the parser, which nests one level deeper per `[?],` while the brackets never
// go past two. Nesting is measured on the parser's side.
type flowLevel struct {
	isSeq bool
	pair  bool
}

// depthScanner mirrors the yaml.v3 scanner state that decides nesting.
type depthScanner struct {
	s      string
	i      int // byte offset
	line   int
	column int // in characters, as yaml.v3 counts

	flow       int         // the scanner's flow level
	flows      []flowLevel // the parser's open flow collections
	pairs      int         // flows with an open implicit pair
	swallow    bool        // the parser skips the next `]`, `,` or `:`
	indent     int         // column of the innermost block collection, -1 at top level
	blocks     []blockLevel
	indentless int // blocks with indentless set
	keys       []simpleKey
	keyAllowed bool

	depth int // deepest nesting seen
	limit int // stop once depth passes this
}

// scanDepth returns an upper bound on the collection nesting yaml.v3 builds
// for src. It stops early, returning a value above limit, once the bound
// passes limit.
func scanDepth(src string, limit int) int {
	d := &depthScanner{s: src, indent: -1, keys: []simpleKey{{}}, keyAllowed: true, limit: limit}
	d.run()
	return d.depth
}

func (d *depthScanner) level() int { return len(d.blocks) + d.indentless + len(d.flows) + d.pairs }

func (d *depthScanner) note() {
	n := d.level()
	if n > d.depth {
		d.depth = n
	}
	for j := range d.keys {
		if d.keys[j].possible && n > d.keys[j].peak {
			d.keys[j].peak = n
		}
	}
}

// wrap accounts for a level inserted around everything since key k started.
// Every other open key started earlier, so its span holds k's and its peak
// rises too (`[[a]: b]: c` wraps twice).
func (d *depthScanner) wrap(k simpleKey) {
	n := k.peak + 1
	if n > d.depth {
		d.depth = n
	}
	for j := range d.keys {
		if d.keys[j].possible && n > d.keys[j].peak {
			d.keys[j].peak = n
		}
	}
}

// openPair opens the implicit pair of the innermost flow sequence.
func (d *depthScanner) openPair() { d.openPairIn(len(d.flows)) }

// openPairIn opens the implicit pair of the flow sequence that is the
// innermost of the first n open collections: a key's pair belongs to the
// collection the key started in, which the parser may still have collections
// open inside (the swallowed `]` above).
func (d *depthScanner) openPairIn(n int) {
	if n > 0 && n <= len(d.flows) {
		if f := &d.flows[n-1]; f.isSeq && !f.pair {
			f.pair = true
			d.pairs++
			d.note()
		}
	}
}

// closePair ends the innermost flow collection's implicit pair, if open.
func (d *depthScanner) closePair() {
	if len(d.flows) > 0 {
		if f := &d.flows[len(d.flows)-1]; f.pair {
			f.pair = false
			d.pairs--
		}
	}
}

// breakLen is the length of the line break at i (CR LF, CR, LF, NEL, LS, PS,
// the set yaml.v3's is_break accepts), or 0.
func (d *depthScanner) breakLen(i int) int {
	s := d.s
	if i >= len(s) {
		return 0
	}
	switch s[i] {
	case '\n':
		return 1
	case '\r':
		if i+1 < len(s) && s[i+1] == '\n' {
			return 2
		}
		return 1
	case 0xC2:
		if i+1 < len(s) && s[i+1] == 0x85 {
			return 2
		}
	case 0xE2:
		if i+2 < len(s) && s[i+1] == 0x80 && (s[i+2] == 0xA8 || s[i+2] == 0xA9) {
			return 3
		}
	}
	return 0
}

func (d *depthScanner) eof() bool { return d.i >= len(d.s) }

func (d *depthScanner) isBlank(i int) bool {
	return i < len(d.s) && (d.s[i] == ' ' || d.s[i] == '\t')
}

// isBlankz is yaml.v3's is_blankz: a blank, a break, or the end.
func (d *depthScanner) isBlankz(i int) bool {
	return i >= len(d.s) || d.isBlank(i) || d.breakLen(i) > 0 || d.s[i] == 0
}

// skip advances over one character (not a break).
func (d *depthScanner) skip() {
	c := d.s[d.i]
	d.i++
	if c >= 0x80 {
		for d.i < len(d.s) && d.s[d.i]&0xC0 == 0x80 {
			d.i++
		}
	}
	d.column++
}

// skipBreak advances over the break at d.i.
func (d *depthScanner) skipBreak() {
	d.i += d.breakLen(d.i)
	d.line++
	d.column = 0
}

func (d *depthScanner) skipLine() {
	for !d.eof() && d.breakLen(d.i) == 0 {
		d.skip()
	}
}

// docMarker reports a `---` or `...` document marker at d.i.
func (d *depthScanner) docMarker() bool {
	s, i := d.s, d.i
	return d.column == 0 && i+3 <= len(s) &&
		(s[i:i+3] == "---" || s[i:i+3] == "...") && d.isBlankz(i+3)
}

// ----- the indent stack ------------------------------------------------------

func (d *depthScanner) roll(column int, isMap bool) {
	if d.flow > 0 {
		return
	}
	if d.indent < column {
		d.blocks = append(d.blocks, blockLevel{column: column, isMap: isMap})
		d.indent = column
		d.note()
		return
	}
	// A sequence entry at a mapping's own column opens an indentless
	// sequence, one level the stack does not show.
	if !isMap && d.indent == column && len(d.blocks) > 0 {
		top := &d.blocks[len(d.blocks)-1]
		if top.isMap && !top.indentless {
			top.indentless = true
			d.indentless++
			d.note()
		}
	}
}

func (d *depthScanner) unroll(column int) {
	if d.flow > 0 {
		return
	}
	for d.indent > column {
		top := d.blocks[len(d.blocks)-1]
		if top.indentless {
			d.indentless--
		}
		d.blocks = d.blocks[:len(d.blocks)-1]
		d.indent = -1
		if len(d.blocks) > 0 {
			d.indent = d.blocks[len(d.blocks)-1].column
		}
	}
}

// ----- simple keys -----------------------------------------------------------

func (d *depthScanner) saveKey() {
	if d.keyAllowed {
		d.keys[len(d.keys)-1] = simpleKey{possible: true, line: d.line, column: d.column, index: d.i, peak: d.level(), flows: len(d.flows)}
	}
}

func (d *depthScanner) removeKey() { d.keys[len(d.keys)-1].possible = false }

// ----- the token loop --------------------------------------------------------

func (d *depthScanner) run() {
	for d.depth <= d.limit {
		d.toNextToken()
		if d.eof() {
			return
		}
		d.unroll(d.column)
		s, c := d.s, d.s[d.i]
		if d.column == 0 && c == '%' { // a directive
			d.skipLine()
			continue
		}
		if d.docMarker() {
			d.unroll(-1)
			d.removeKey()
			d.keyAllowed = false
			d.skip()
			d.skip()
			d.skip()
			continue
		}
		next, start := d.i+1, d.i
		swallowed := d.swallow
		d.swallow = false
		switch {
		case c == '[' || c == '{':
			d.saveKey()
			d.flow++
			d.flows = append(d.flows, flowLevel{isSeq: c == '['})
			d.keys = append(d.keys, simpleKey{})
			d.note()
			d.keyAllowed = true
			d.skip()
		case c == ']' || c == '}':
			d.removeKey()
			if d.flow > 0 {
				d.flow--
				d.keys = d.keys[:len(d.keys)-1]
			}
			if !swallowed && len(d.flows) > 0 {
				d.closePair()
				d.flows = d.flows[:len(d.flows)-1]
			}
			d.keyAllowed = false
			d.skip()
		case c == ',':
			if !swallowed {
				d.closePair()
			}
			d.removeKey()
			d.keyAllowed = true
			d.skip()
		case c == '-' && d.isBlankz(next):
			if d.flow == 0 {
				d.roll(d.column, false)
			}
			d.removeKey()
			d.keyAllowed = true
			d.skip()
		case c == '?' && (d.flow > 0 || d.isBlankz(next)):
			if d.flow == 0 {
				d.roll(d.column, true)
			}
			if len(d.flows) > 0 && d.flows[len(d.flows)-1].isSeq {
				d.openPair()
				d.swallow = true
			}
			d.removeKey()
			d.keyAllowed = d.flow == 0
			d.skip()
		case c == ':' && (d.flow > 0 || d.isBlankz(next)):
			k := &d.keys[len(d.keys)-1]
			if k.possible && k.line == d.line && k.index+1024 >= d.i {
				d.wrap(*k)
				d.openPairIn(k.flows)
				d.roll(k.column, true)
				k.possible = false
				d.keyAllowed = false
			} else {
				if d.flow == 0 {
					d.roll(d.column, true)
				}
				d.openPair()
				d.keyAllowed = d.flow == 0
			}
			d.skip()
		case c == '*' || c == '&':
			d.saveKey()
			d.keyAllowed = false
			d.skip()
			for !d.eof() && isAnchorChar(s[d.i]) {
				d.skip()
			}
		case c == '!':
			d.saveKey()
			d.keyAllowed = false
			d.tag()
		case (c == '|' || c == '>') && d.flow == 0:
			d.removeKey()
			d.keyAllowed = true
			d.blockScalar()
		case c == '\'' || c == '"':
			d.saveKey()
			d.keyAllowed = false
			d.quoted(c)
		case canStartPlain(s, d.i, d.flow, d.isBlankz(next)):
			d.saveKey()
			d.keyAllowed = false
			d.plain()
		}
		// A character yaml.v3 refuses (`@`, a tab where a token should
		// start): it stops there, so whatever this scan makes of the rest
		// only adds to the bound. Step over it and go on.
		if d.i == start && !d.eof() {
			if d.breakLen(d.i) > 0 {
				d.skipBreak()
			} else {
				d.skip()
			}
		}
	}
}

// toNextToken skips blanks, comments and line breaks (yaml.v3's
// scan_to_next_token). A tab separates tokens only in flow context or where a
// simple key is not allowed.
func (d *depthScanner) toNextToken() {
	for !d.eof() {
		if d.column == 0 && d.i+3 <= len(d.s) && d.s[d.i:d.i+3] == "\xEF\xBB\xBF" {
			d.i += 3
		}
		for !d.eof() && (d.s[d.i] == ' ' || (d.flow > 0 || !d.keyAllowed) && d.s[d.i] == '\t') {
			d.skip()
		}
		if !d.eof() && d.s[d.i] == '#' {
			d.skipLine()
		}
		if d.breakLen(d.i) == 0 {
			return
		}
		d.skipBreak()
		if d.flow == 0 {
			d.keyAllowed = true
		}
	}
}

func isAlpha(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-'
}

// isAnchorChar is the anchor / alias name set yaml.v3 accepts.
func isAnchorChar(c byte) bool { return isAlpha(c) }

// isURIChar is the tag character set of yaml.v3's scan_tag_uri.
func isURIChar(c byte) bool {
	if isAlpha(c) {
		return true
	}
	switch c {
	case ';', '/', '?', ':', '@', '&', '=', '+', '$', ',', '.', '!', '~', '*', '\'', '(', ')', '[', ']', '%':
		return true
	}
	return false
}

// tag consumes a tag (`!`, `!!str`, `!x!y`, `!<verbatim>`); it must be
// followed by a blank or the end, as yaml.v3 requires.
func (d *depthScanner) tag() bool {
	d.skip() // '!'
	if !d.eof() && d.s[d.i] == '<' {
		for !d.eof() && d.s[d.i] != '>' && d.breakLen(d.i) == 0 {
			d.skip()
		}
		if d.eof() || d.s[d.i] != '>' {
			return false
		}
		d.skip()
	} else {
		for !d.eof() && isURIChar(d.s[d.i]) {
			d.skip()
		}
	}
	return d.isBlankz(d.i)
}

// canStartPlain is yaml.v3's test for a plain scalar's first character, after
// every indicator has been ruled out.
func canStartPlain(s string, i, flow int, nextBlankz bool) bool {
	switch c := s[i]; c {
	case '-':
		return !nextBlankz
	case '?', ':':
		return flow == 0 && !nextBlankz
	case ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`', ' ', '\t', 0:
		return false
	}
	return true
}

// quoted consumes a single- or double-quoted scalar, line breaks included.
func (d *depthScanner) quoted(q byte) bool {
	d.skip()
	for {
		if d.docMarker() {
			return false // "found unexpected document indicator"
		}
		if d.eof() {
			return false // "found unexpected end of stream"
		}
		c := d.s[d.i]
		switch {
		case q == '\'' && c == '\'' && d.i+1 < len(d.s) && d.s[d.i+1] == '\'':
			d.skip()
			d.skip()
		case c == q:
			d.skip()
			return true
		case q == '"' && c == '\\':
			d.skip()
			if d.breakLen(d.i) > 0 {
				d.skipBreak()
			} else if !d.eof() {
				d.skip()
			}
		case d.breakLen(d.i) > 0:
			d.skipBreak()
		default:
			d.skip()
		}
	}
}

// plain consumes a plain scalar, following its continuation lines exactly as
// yaml.v3's scan_plain_scalar does: in block context a line continues the
// scalar while it is indented past the enclosing block collection.
func (d *depthScanner) plain() {
	need := d.indent + 1
	leadingBlanks := false
	for {
		if d.docMarker() || d.eof() || d.s[d.i] == '#' {
			break
		}
		for !d.isBlankz(d.i) {
			c := d.s[d.i]
			if c == ':' && d.isBlankz(d.i+1) {
				break
			}
			if d.flow > 0 && (c == ',' || c == '?' || c == '[' || c == ']' || c == '{' || c == '}') {
				break
			}
			d.skip()
		}
		if !(d.isBlank(d.i) || d.breakLen(d.i) > 0) {
			break
		}
		for d.isBlank(d.i) || d.breakLen(d.i) > 0 {
			if d.isBlank(d.i) {
				d.skip()
			} else {
				d.skipBreak()
				leadingBlanks = true
			}
		}
		if d.flow == 0 && d.column < need {
			break
		}
	}
	if leadingBlanks {
		d.keyAllowed = true
	}
}

// blockScalar consumes a `|` / `>` scalar: its header, then every line
// indented at least as far as its content indent, which is explicit or
// detected from the first non-empty line, as in yaml.v3's scan_block_scalar.
func (d *depthScanner) blockScalar() bool {
	d.skip() // '|' or '>'
	increment := 0
	for k := 0; k < 2 && !d.eof(); k++ {
		c := d.s[d.i]
		switch {
		case c == '+' || c == '-':
			d.skip()
		case c >= '1' && c <= '9' && increment == 0:
			increment = int(c - '0')
			d.skip()
		case c == '0':
			return false // "found an indentation indicator equal to 0"
		}
	}
	for d.isBlank(d.i) {
		d.skip()
	}
	if !d.eof() && d.s[d.i] == '#' {
		d.skipLine()
	}
	if d.eof() {
		return true
	}
	if d.breakLen(d.i) == 0 {
		return false // "did not find expected comment or line break"
	}
	d.skipBreak()
	indent := 0
	if increment > 0 {
		if d.indent >= 0 {
			indent = d.indent + increment
		} else {
			indent = increment
		}
	}
	if !d.blockBreaks(&indent) {
		return false
	}
	for d.column == indent && !d.eof() {
		for !d.eof() && d.breakLen(d.i) == 0 {
			d.skip()
		}
		if d.eof() {
			break
		}
		d.skipBreak()
		if !d.blockBreaks(&indent) {
			return false
		}
	}
	return true
}

// blockBreaks skips a block scalar's indentation and blank lines
// (scan_block_scalar_breaks), fixing an undetected indent from them.
func (d *depthScanner) blockBreaks(indent *int) bool {
	maxIndent := 0
	for {
		for (*indent == 0 || d.column < *indent) && !d.eof() && d.s[d.i] == ' ' {
			d.skip()
		}
		if d.column > maxIndent {
			maxIndent = d.column
		}
		if (*indent == 0 || d.column < *indent) && !d.eof() && d.s[d.i] == '\t' {
			return false // "found a tab character where an indentation space is expected"
		}
		if d.breakLen(d.i) == 0 {
			break
		}
		d.skipBreak()
	}
	if *indent == 0 {
		*indent = maxIndent
		if *indent < d.indent+1 {
			*indent = d.indent + 1
		}
		if *indent < 1 {
			*indent = 1
		}
	}
	return true
}
