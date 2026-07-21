// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
)

// runFmt formats path's source to stdout per docs/user-guide/style-guide.md. The
// formatter operates on the token stream rather than the AST so it can
// preserve `import "file.j";` statements verbatim (the preprocessor would
// otherwise inline them) and any parentheses the user wrote (the AST
// erases redundant grouping).
//
// Comments and blank lines now survive `fmt`. The lexer emits them
// as trivia tokens (TOKEN_COMMENT_*, TOKEN_BLANK_LINE); the formatter
// recognises each kind and re-emits it at the position it had in the
// source. Comments inside an expression (`printf(/* note */ $x)`) are
// supported only by-position - they are not parsed for attachment to
// any particular subexpression.
func runFmt(path string) int {
	src, label, absPath, _, ok := loadProgramSource(path)
	if !ok {
		return 1
	}
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	formatted := formatTokens(tokens)
	io.WriteString(os.Stdout, formatted)
	return 0
}

// formatTokens emits canonical Jennifer source from a complete token list
// (including the trailing EOF). Tests call this directly; the CLI wraps
// it with file I/O.
func formatTokens(tokens []lexer.Token) string {
	// File starts at column 1 of line 1 with no output yet, which is
	// effectively "line start" for separator purposes. That makes
	// a leading comment skip the spurious blank line that would
	// otherwise appear before it.
	f := &fmtState{indent: 0, atLineStart: true}
	for i, t := range tokens {
		if t.Type == lexer.TOKEN_EOF {
			break
		}
		f.emit(t, peekAt(tokens, i+1))
	}
	return f.finish()
}

// fmtState tracks the running output and the bookkeeping needed to decide
// what separator (space / newline / nothing) to put between consecutive
// tokens. The driver loop calls `emit(curr, next)` for every non-EOF
// token, then `finish()` once for the trailing newline.
type fmtState struct {
	out         strings.Builder
	indent      int // current block depth in indent units
	col         int // current output column (rune count since last '\n')
	prev        lexer.Token
	hasPrev     bool // false before the first token
	atLineStart bool // true right after newline+indent has been written
	// Token kinds that "begin an operand context" - i.e. when the next
	// token is `-`, that `-` is binary, not unary. Maintained on every
	// emit() so unary-vs-binary disambiguation stays a state lookup.
	prevIsOperand    bool
	prevIsUnaryMinus bool // true when prev was a `-` parsed as unary
	// braceStack records, for every open `{`, whether it's a block
	// (statements), a struct-decl body (one field per line), or a map
	// literal (key:value pairs). Matching `}` pops; the kind
	// determines indenting and newline behavior.
	braceStack []byte // 'b' block, 'm' map literal, 's' struct decl
	// lastBraceKind remembers the kind of the most recently emitted
	// `{` or `}` so the next token's separator logic can ask "was that
	// `}` a block close or a map close?" after the stack was already
	// popped. 0 if no brace has been seen yet.
	lastBraceKind byte
	// pendingTriviaSpace is set by emitTrivia after writing an inline
	// block comment that should not hug the next token. writeSeparator
	// consumes it, emitting a space before the next real token unless
	// that next token is itself tight-on-left (`)`, `,`, `;`, ...).
	// This is what makes `printf(/* note */ $x)` come out with the
	// expected internal space rather than `printf( /* note */$x)`.
	pendingTriviaSpace bool
	// pendingStructBrace flags "the next `{` opens a struct-decl body,
	// not a map literal". Set on emit of TOKEN_STRUCT; consumed on the
	// next TOKEN_LBRACE. Reset defensively on `;` so a stray `struct`
	// keyword doesn't corrupt later formatting.
	pendingStructBrace bool
}

const (
	braceBlock  = byte('b')
	braceMap    = byte('m')
	braceStruct = byte('s')
)

const fmtIndent = "    " // 4 spaces, per style-guide.md

// maxLineLength is the soft column limit. When a line grows past this
// and the next token is a binary joiner (`+`, `and`, `or`), the
// formatter breaks after the joiner and hangs subsequent lines one
// indent level in. Chosen to match docs/user-guide/style-guide.md.
const maxLineLength = 100

func (f *fmtState) emit(t, next lexer.Token) {
	// Trivia (comments, blank lines) is emitted by a dedicated
	// path that doesn't touch the prev/operand/brace state. That keeps
	// unary-vs-binary and brace classification working as if the
	// trivia weren't there.
	switch t.Type {
	case lexer.TOKEN_COMMENT_SHEBANG,
		lexer.TOKEN_COMMENT_LINE,
		lexer.TOKEN_COMMENT_BLOCK,
		lexer.TOKEN_BLANK_LINE:
		f.emitTrivia(t)
		return
	}
	// Classify `{` before writing it. Three kinds:
	//   - struct-decl body: the `def struct Name {` form (marked by
	//     pendingStructBrace, set when TOKEN_STRUCT was emitted).
	//   - block: after a token that begins a statement block
	//     (`)`, `else`, `try`, `spawn`, `repeat`).
	//   - map literal: anywhere else (the parser only allows `{` in
	//     expression position when it's a map literal, so any
	//     non-block, non-struct context must be a map).
	openBlock := false
	if t.Type == lexer.TOKEN_LBRACE {
		var kind byte
		switch {
		case f.pendingStructBrace:
			kind = braceStruct
			f.pendingStructBrace = false
		case f.hasPrev && isBlockOpener(f.prev.Type):
			kind = braceBlock
		default:
			kind = braceMap
		}
		f.braceStack = append(f.braceStack, kind)
		f.lastBraceKind = kind
		openBlock = kind == braceBlock || kind == braceStruct
	}
	// Closing brace: pop and find out what kind we're closing. For block
	// and struct `}` we dedent before writing so the brace lands at the
	// outer indent; for map literal `}` we don't touch indent. The
	// popped kind is also remembered in lastBraceKind so the next
	// token's separator logic can branch correctly.
	if t.Type == lexer.TOKEN_RBRACE && len(f.braceStack) > 0 {
		kind := f.braceStack[len(f.braceStack)-1]
		f.braceStack = f.braceStack[:len(f.braceStack)-1]
		f.lastBraceKind = kind
		if kind == braceBlock || kind == braceStruct {
			if f.indent > 0 {
				f.indent--
			}
		}
	}
	// Detect whether the `-` we're about to write is unary. A `-` is
	// unary when nothing operand-shaped came before it.
	isUnaryMinus := t.Type == lexer.TOKEN_MINUS && !f.prevIsOperand
	f.writeSeparator(t)
	f.writeToken(t, next)
	// Track "next `{` is a struct-decl body" across the intervening
	// identifier: set on STRUCT, reset by LBRACE (consumed above) or
	// by SEMI (defensive - keeps a stray `struct` from tainting later
	// braces).
	switch t.Type {
	case lexer.TOKEN_STRUCT:
		f.pendingStructBrace = true
	case lexer.TOKEN_SEMI:
		f.pendingStructBrace = false
	}
	f.prev = t
	f.hasPrev = true
	f.atLineStart = false
	f.prevIsOperand = isOperandToken(t)
	f.prevIsUnaryMinus = isUnaryMinus
	if openBlock {
		f.indent++
	}
}

// emitTrivia handles comments and blank lines. It writes only output -
// it does NOT touch prev/lastBraceKind/prevIsOperand/prevIsUnaryMinus
// so the surrounding state machine continues to see the most recent
// regular token. atLineStart IS updated because the separator logic
// for the next regular token reads it to decide whether to skip a
// leading separator.
func (f *fmtState) emitTrivia(t lexer.Token) {
	switch t.Type {
	case lexer.TOKEN_COMMENT_SHEBANG:
		// Shebang must be at file head, col 1. Re-emit verbatim and
		// move to a new line.
		f.writeString(t.Lexeme)
		f.writeByte('\n')
		f.atLineStart = true
	case lexer.TOKEN_COMMENT_LINE:
		// A line comment on the same source line as the previous real
		// token is a trailing comment; one on its own line is a leading
		// comment. Trailing: ` # ...`. Leading: indent + ` # ...`.
		// Either way, the line ends after the comment.
		if f.hasPrev && f.prev.Line == t.Line {
			f.writeByte(' ')
		} else if !f.atLineStart {
			f.newline()
		}
		f.writeString(t.Lexeme)
		f.writeByte('\n')
		// Next real token starts a fresh line at the current indent.
		for i := 0; i < f.indent; i++ {
			f.writeString(fmtIndent)
		}
		f.atLineStart = true
	case lexer.TOKEN_COMMENT_BLOCK:
		// A block comment on its own line (or at file start) is a
		// *leading* comment - typically a `/** ... */` doc comment
		// before a `func` / `def struct` / `def const`. Emit it on its
		// own line(s) and end the line, so the documented construct
		// starts fresh below it rather than being glued to the closing
		// `*/` (never `*/func`). A block comment on the same source
		// line as the previous real token is *inline*
		// (`printf(/* n */ $x)`) and keeps its surrounding spaces.
		//
		// A multi-line block comment re-emits its body verbatim - the
		// formatter doesn't re-indent the inner ` * ` lines (v1
		// limitation), which matches how doc comments are conventionally
		// written at the top level.
		if !f.hasPrev || f.prev.Line != t.Line {
			if f.hasPrev && !f.atLineStart {
				f.newline()
			}
			f.writeString(t.Lexeme)
			f.newline()
			return
		}
		// Inline: emit a space before the comment unless the previous
		// token was a tight-on-right operator (`(`, `[`, `.`), which
		// would normally hug the next token.
		needLeadingSpace := f.hasPrev && !f.atLineStart && !tightOnRight(f.prev.Type)
		if needLeadingSpace {
			f.writeByte(' ')
		}
		f.writeString(t.Lexeme)
		if strings.HasSuffix(t.Lexeme, "\n") {
			f.atLineStart = true
		} else {
			f.atLineStart = false
			// Force a space before the next real token (unless that
			// token is itself tight-on-left). Without this flag, the
			// next token's separator logic would still see prev=`(` /
			// `[` / `.` and skip the space.
			f.pendingTriviaSpace = true
		}
	case lexer.TOKEN_BLANK_LINE:
		// End the current line if we're mid-line, then add one blank.
		// The indent for the next real token is emitted lazily; we
		// leave the formatter at line-start so the next separator
		// decides what indent goes in.
		if !f.atLineStart {
			f.writeByte('\n')
		}
		f.writeByte('\n')
		for i := 0; i < f.indent; i++ {
			f.writeString(fmtIndent)
		}
		f.atLineStart = true
	}
}

// prevBraceKind reports the kind of the most recently emitted `{` or `}`
// token. The brace stack itself has already popped by the time the
// separator runs, so we cache the kind on emit.
func (f *fmtState) prevBraceKind() byte { return f.lastBraceKind }

// writeSeparator decides what (if anything) goes between f.prev and t,
// and writes it. Five outcomes: nothing, single space, newline+indent,
// or - in special cases - the chosen separator overrides on either side.
func (f *fmtState) writeSeparator(t lexer.Token) {
	if !f.hasPrev {
		return
	}
	if f.atLineStart {
		f.pendingTriviaSpace = false
		return
	}
	// A preceding inline block comment forces a space before this
	// token unless the token is itself tight-on-left (`)`, `,`, `;`,
	// `]`, `.`, `:`).
	if f.pendingTriviaSpace {
		f.pendingTriviaSpace = false
		switch t.Type {
		case lexer.TOKEN_RPAREN, lexer.TOKEN_COMMA, lexer.TOKEN_SEMI,
			lexer.TOKEN_DOT, lexer.TOKEN_RBRACKET, lexer.TOKEN_COLON:
			return
		}
		f.writeByte(' ')
		return
	}
	// Column-based reflow at binary joiners. When a line has already
	// grown past maxLineLength, or when the source itself put a break
	// at this point, wrap AFTER the joiner so the operator hangs at
	// end-of-line (matches the string-concat idiom in the wild). One
	// extra indent level per hanging continuation line, matching what
	// the style guide recommends.
	if isBinaryJoiner(f.prev.Type) && !f.prevIsUnaryMinus {
		if t.Line > f.prev.Line || f.col > maxLineLength {
			f.continuationLine()
			return
		}
	}
	// Statement terminator: ";" closes a statement; the next token starts
	// a new line at the current indent. Exception: the two `;`s inside
	// `for (...; ...; ...)` stay on the same line (handled by the
	// paren-depth check below).
	if f.prev.Type == lexer.TOKEN_SEMI {
		if !f.insideForHeader() {
			f.newline()
			return
		}
		f.writeByte(' ')
		return
	}
	// Struct-decl `,`: each field on its own line. The comma itself
	// was already emitted (tight-on-left); the next field's leading
	// token gets a newline here instead of a space.
	if f.prev.Type == lexer.TOKEN_COMMA && f.currentBraceKind() == braceStruct {
		f.newline()
		return
	}
	// Closing brace: block or struct `}` ends a multi-line container;
	// the next token starts a new line, except for the cuddled tail
	// forms - `} else`, `} elseif`, `} catch`, `} until`, and
	// `};` where the semicolon hugs the brace so a struct decl reads
	// as `};` on one line. Map-literal `}` stays inline (no newline).
	if f.prev.Type == lexer.TOKEN_RBRACE {
		if t.Type == lexer.TOKEN_SEMI {
			return
		}
		if f.prevBraceKind() == braceBlock || f.prevBraceKind() == braceStruct {
			switch t.Type {
			case lexer.TOKEN_ELSE, lexer.TOKEN_ELSEIF,
				lexer.TOKEN_CATCH, lexer.TOKEN_UNTIL:
				f.writeByte(' ')
				return
			}
			f.newline()
			return
		}
		// Map literal close: fall through to default-space-or-tight-rules.
	}
	// About-to-emit `}` for a struct decl: newline first so the closing
	// brace lands on its own line at the outer indent (indent was
	// already decremented in emit()).
	if t.Type == lexer.TOKEN_RBRACE && f.lastBraceKind == braceStruct {
		f.newline()
		return
	}
	// Opening brace: block and struct `{` trigger a newline so the body
	// starts on its own indented line. Map-literal `{` keeps the
	// contents inline with no padding (matches the existing `(` rule).
	if f.prev.Type == lexer.TOKEN_LBRACE {
		if f.prevBraceKind() == braceBlock || f.prevBraceKind() == braceStruct {
			f.newline()
			return
		}
		// Map literal: no padding inside.
		return
	}
	// No space between an IDENT or TYPE keyword and an opening `(` - call
	// sites (`printf(`) and type-conversion sites (`int(`) both use the
	// tight form. The leading keyword forms (`if (`, `while (`, `for (`,
	// `elseif (`) get a space, handled by the default case below since
	// those keyword types aren't in noSpaceBeforeLParen.
	if t.Type == lexer.TOKEN_LPAREN && noSpaceBeforeLParen(f.prev.Type) {
		return
	}
	// Index expressions hug their target: `$xs[0]`, `foo()[1]`,
	// `bar()[0][1]`. Any token that can stand at the end of an indexable
	// expression (IDENT, VARREF, RPAREN, RBRACKET, RBRACE-from-map) gets
	// tight binding to a following `[`.
	if t.Type == lexer.TOKEN_LBRACKET && noSpaceBeforeLBracket(f.prev.Type) {
		return
	}
	// No space before a map literal `}` (the closing brace was already
	// classified during emit and recorded in lastBraceKind).
	if t.Type == lexer.TOKEN_RBRACE && f.lastBraceKind == braceMap {
		return
	}
	// Tight punctuation: nothing between `(`/`[`/map-`{` and the next
	// token, and nothing between the previous token and the matching
	// close, comma, semi, dot, or `:` (map-literal key/value separator).
	switch t.Type {
	case lexer.TOKEN_RPAREN, lexer.TOKEN_COMMA, lexer.TOKEN_SEMI,
		lexer.TOKEN_DOT, lexer.TOKEN_RBRACKET, lexer.TOKEN_COLON:
		return
	}
	if f.prev.Type == lexer.TOKEN_LPAREN || f.prev.Type == lexer.TOKEN_DOT ||
		f.prev.Type == lexer.TOKEN_LBRACKET {
		return
	}
	// Unary minus hugs its operand: `-5`, `-$x`, `-foo()`. The state
	// machine recorded on the previous emit() whether the `-` it just
	// wrote was unary; if so, no separator on its right side.
	if f.prevIsUnaryMinus {
		return
	}
	// Default: single space.
	f.writeByte(' ')
}

// writeByte / writeString are the only two paths that reach f.out;
// both keep f.col in sync so writeSeparator can consult the current
// column when deciding whether to reflow at a binary joiner. Column
// counts runes rather than bytes so non-ASCII string literals don't
// throw off the maxLineLength check.
func (f *fmtState) writeByte(b byte) {
	f.out.WriteByte(b)
	if b == '\n' {
		f.col = 0
	} else {
		f.col++
	}
}

func (f *fmtState) writeString(s string) {
	f.out.WriteString(s)
	for _, r := range s {
		if r == '\n' {
			f.col = 0
		} else {
			f.col++
		}
	}
}

// writeToken emits the token's text in its canonical form. Strings get
// their surrounding double quotes back; var refs get the `$` sigil.
func (f *fmtState) writeToken(t, _ lexer.Token) {
	switch t.Type {
	case lexer.TOKEN_VARREF:
		f.writeByte('$')
		f.writeString(t.Lexeme)
	case lexer.TOKEN_STRING:
		f.writeString(quoteJenniferString(t.Lexeme))
	default:
		f.writeString(canonicalLexeme(t))
	}
}

func (f *fmtState) newline() {
	f.writeByte('\n')
	for i := 0; i < f.indent; i++ {
		f.writeString(fmtIndent)
	}
	f.atLineStart = true
}

// continuationLine ends the current line and starts a new one at a
// hanging indent (one level in from the enclosing block). Used to
// reflow long expressions at `+ / and / or` joiners.
func (f *fmtState) continuationLine() {
	f.writeByte('\n')
	for i := 0; i < f.indent+1; i++ {
		f.writeString(fmtIndent)
	}
	f.atLineStart = true
}

func (f *fmtState) finish() string {
	s := f.out.String()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

// insideForHeader reports whether we're between the two `for (...;...;...)`
// semicolons. The check is approximate: it walks back through the output
// string looking for an unmatched `(` preceded by `for`. Cheap enough at
// formatter cadence and avoids carrying a token-aware paren stack.
//
// Used so the formatter writes a space (not a newline) after the two
// internal `for`-header semicolons.
func (f *fmtState) insideForHeader() bool {
	s := f.out.String()
	depth := 0
	// Walk backwards counting parens; a `(` with depth 0 is our enclosing
	// LPAREN. Check whether the keyword preceding it was `for`.
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				// Check the word that ends just before the `(`.
				j := i - 1
				for j >= 0 && s[j] == ' ' {
					j--
				}
				return j >= 2 && s[j-2:j+1] == "for"
			}
			depth--
		}
	}
	return false
}

// canonicalLexeme returns the source-form spelling of a token. For
// keywords and punctuation, the constant lexeme in the token is already
// canonical; for INT and FLOAT literals we use the lexeme as captured by
// the lexer (no normalization of float forms in v1).
func canonicalLexeme(t lexer.Token) string {
	if t.Lexeme != "" {
		return t.Lexeme
	}
	// Fallback for tokens whose lexeme field is empty (shouldn't normally
	// happen for anything we'd want to print, but keeps fmt total).
	return t.Type.String()
}

// quoteJenniferString re-quotes a string literal's *processed* value back
// into Jennifer-source form with double quotes and the standard escape
// sequences. Mirrors what the lexer's readString accepted on the way in.
func quoteJenniferString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case 0:
			b.WriteString("\\0")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// isBlockOpener reports whether tt introduces a `{` that starts a
// statement-bearing block (as opposed to a map-literal or struct-decl
// body). The parser lets `{` follow:
//   - `)` from the head of `if / while / for / func / catch`;
//   - `else` (unconditional else body);
//   - `try`;
//   - `spawn` (block primary expression);
//   - `repeat` (post-test loop).
//
// Struct-decl bodies are recognised through a separate one-shot flag
// (pendingStructBrace) because their `{` follows the struct's name
// identifier, not a keyword.
func isBlockOpener(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TOKEN_RPAREN, lexer.TOKEN_ELSE,
		lexer.TOKEN_TRY, lexer.TOKEN_SPAWN, lexer.TOKEN_REPEAT:
		return true
	}
	return false
}

// isBinaryJoiner reports whether tt is a binary joiner that the
// formatter is allowed to break AFTER when the current line has
// grown past maxLineLength. Keep the set small: only operators that
// commonly stitch expressions together across lines in real code
// (string concat with `+`, boolean chains with `and`/`or`). `-` is
// deliberately excluded because unary-minus disambiguation would
// otherwise need to be re-checked at every reflow point.
func isBinaryJoiner(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TOKEN_PLUS, lexer.TOKEN_AND, lexer.TOKEN_OR:
		return true
	}
	return false
}

// currentBraceKind returns the kind of the innermost `{` currently
// open, or 0 if none. Used by the separator logic to decide, for
// example, whether a `,` should be followed by a newline (struct
// decls) or a space (map literals, calls).
func (f *fmtState) currentBraceKind() byte {
	if len(f.braceStack) == 0 {
		return 0
	}
	return f.braceStack[len(f.braceStack)-1]
}

// isOperandToken reports whether t produces a value (so a following `-`
// is binary, not unary). The negation - "not an operand" - means t leaves
// the formatter in an expression-start context.
func isOperandToken(t lexer.Token) bool {
	switch t.Type {
	case lexer.TOKEN_INT, lexer.TOKEN_FLOAT, lexer.TOKEN_STRING,
		lexer.TOKEN_VARREF, lexer.TOKEN_TRUE, lexer.TOKEN_FALSE,
		lexer.TOKEN_NULL, lexer.TOKEN_IDENT,
		lexer.TOKEN_RPAREN, lexer.TOKEN_RBRACE:
		return true
	}
	return false
}

// noSpaceBeforeLParen lists the token types that hug a following `(`:
// function calls, type-conversion casts, and the `len` built-in (a
// keyword-shaped primary expression that syntactically behaves like
// a call).
func noSpaceBeforeLParen(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TOKEN_IDENT,
		lexer.TOKEN_INT_TYPE, lexer.TOKEN_FLOAT_TYPE,
		lexer.TOKEN_STRING_TYPE, lexer.TOKEN_BOOL_TYPE,
		lexer.TOKEN_LEN:
		return true
	}
	return false
}

// tightOnRight reports whether a token would normally hug the
// following token (i.e. no separator between it and what comes next).
// `(`, `[`, `.` are the only ones; the trivia path consults this so a
// block comment after `(` doesn't get a spurious leading space.
func tightOnRight(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TOKEN_LPAREN, lexer.TOKEN_LBRACKET, lexer.TOKEN_DOT:
		return true
	}
	return false
}

// noSpaceBeforeLBracket lists the token types that hug a following `[`
// (index expression target). Anything that can end an indexable
// expression: a variable reference, an identifier (when it's the
// callee of a call without args), a closing paren/bracket/brace
// (call result, list slice, map literal).
func noSpaceBeforeLBracket(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TOKEN_IDENT, lexer.TOKEN_VARREF,
		lexer.TOKEN_RPAREN, lexer.TOKEN_RBRACKET, lexer.TOKEN_RBRACE:
		return true
	}
	return false
}

func peekAt(tokens []lexer.Token, i int) lexer.Token {
	if i < 0 || i >= len(tokens) {
		return lexer.Token{}
	}
	return tokens[i]
}
