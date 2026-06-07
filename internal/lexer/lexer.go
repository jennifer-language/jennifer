// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package lexer

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer turns a Jennifer source string into a stream of tokens.
// It tracks line and column for error reporting; column is 1-based.
// File is attached to every produced token for cross-file diagnostics.
type Lexer struct {
	src  []rune
	pos  int
	line int
	col  int
	file string
}

func New(source string) *Lexer {
	return &Lexer{src: []rune(source), pos: 0, line: 1, col: 1}
}

// NewWithFile is like New but tags every produced token with the given file name.
func NewWithFile(source, file string) *Lexer {
	l := New(source)
	l.file = file
	return l
}

// LexError carries a source position so the parser/interpreter can surface useful messages.
type LexError struct {
	Msg  string
	File string
	Line int
	Col  int
}

func (e *LexError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("lex error at %s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("lex error at %d:%d: %s", e.Line, e.Col, e.Msg)
}

// Position implements the positioned-error interface used by the CLI to
// extract a file/line/col without parsing the error string.
func (e *LexError) Position() (file string, line, col int) {
	return e.File, e.Line, e.Col
}

// Tokenize runs the lexer to completion and returns the full token list (terminated by TOKEN_EOF).
func Tokenize(source string) ([]Token, error) {
	return TokenizeWithFile(source, "")
}

// TokenizeWithFile is like Tokenize but tags every produced token with the file name.
// Use it when the source comes from a known file so cross-file diagnostics work.
func TokenizeWithFile(source, file string) ([]Token, error) {
	l := NewWithFile(source, file)
	var out []Token
	for {
		tok, err := l.Next()
		if err != nil {
			return nil, err
		}
		if file != "" {
			tok.File = file
		}
		out = append(out, tok)
		if tok.Type == TOKEN_EOF {
			return out, nil
		}
	}
}

// Next returns the next token. At end of input it repeatedly returns TOKEN_EOF.
func (l *Lexer) Next() (Token, error) {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return Token{}, err
	}
	if l.pos >= len(l.src) {
		return Token{Type: TOKEN_EOF, Line: l.line, Col: l.col}, nil
	}

	startLine, startCol := l.line, l.col
	ch := l.src[l.pos]

	switch {
	case ch == '{':
		l.advance()
		return Token{Type: TOKEN_LBRACE, Lexeme: "{", Line: startLine, Col: startCol}, nil
	case ch == '}':
		l.advance()
		return Token{Type: TOKEN_RBRACE, Lexeme: "}", Line: startLine, Col: startCol}, nil
	case ch == '(':
		l.advance()
		return Token{Type: TOKEN_LPAREN, Lexeme: "(", Line: startLine, Col: startCol}, nil
	case ch == ')':
		l.advance()
		return Token{Type: TOKEN_RPAREN, Lexeme: ")", Line: startLine, Col: startCol}, nil
	case ch == ';':
		l.advance()
		return Token{Type: TOKEN_SEMI, Lexeme: ";", Line: startLine, Col: startCol}, nil
	case ch == ',':
		l.advance()
		return Token{Type: TOKEN_COMMA, Lexeme: ",", Line: startLine, Col: startCol}, nil
	case ch == '=':
		l.advance()
		if next, ok := l.peek(0); ok && next == '=' {
			l.advance()
			return Token{Type: TOKEN_EQ, Lexeme: "==", Line: startLine, Col: startCol}, nil
		}
		return Token{Type: TOKEN_ASSIGN, Lexeme: "=", Line: startLine, Col: startCol}, nil
	case ch == '<':
		l.advance()
		if next, ok := l.peek(0); ok && next == '=' {
			l.advance()
			return Token{Type: TOKEN_LE, Lexeme: "<=", Line: startLine, Col: startCol}, nil
		}
		return Token{Type: TOKEN_LT, Lexeme: "<", Line: startLine, Col: startCol}, nil
	case ch == '>':
		l.advance()
		if next, ok := l.peek(0); ok && next == '=' {
			l.advance()
			return Token{Type: TOKEN_GE, Lexeme: ">=", Line: startLine, Col: startCol}, nil
		}
		return Token{Type: TOKEN_GT, Lexeme: ">", Line: startLine, Col: startCol}, nil
	case ch == '+':
		l.advance()
		return Token{Type: TOKEN_PLUS, Lexeme: "+", Line: startLine, Col: startCol}, nil
	case ch == '-':
		l.advance()
		return Token{Type: TOKEN_MINUS, Lexeme: "-", Line: startLine, Col: startCol}, nil
	case ch == '*':
		l.advance()
		return Token{Type: TOKEN_STAR, Lexeme: "*", Line: startLine, Col: startCol}, nil
	case ch == '/':
		l.advance()
		return Token{Type: TOKEN_SLASH, Lexeme: "/", Line: startLine, Col: startCol}, nil
	case ch == '%':
		l.advance()
		return Token{Type: TOKEN_PERCENT, Lexeme: "%", Line: startLine, Col: startCol}, nil
	case ch == '.':
		l.advance()
		return Token{Type: TOKEN_DOT, Lexeme: ".", Line: startLine, Col: startCol}, nil
	case ch == '"' || ch == '\'':
		return l.readString(ch, startLine, startCol)
	case ch == '$':
		return l.readVarRef(startLine, startCol)
	case unicode.IsDigit(ch):
		return l.readNumber(startLine, startCol)
	case isIdentStart(ch):
		return l.readIdentifierOrKeyword(startLine, startCol)
	}

	return Token{}, &LexError{File: l.file, Msg: fmt.Sprintf("unexpected character %q", ch), Line: startLine, Col: startCol}
}

func (l *Lexer) advance() {
	if l.pos >= len(l.src) {
		return
	}
	if l.src[l.pos] == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.pos++
}

func (l *Lexer) peek(offset int) (rune, bool) {
	idx := l.pos + offset
	if idx < 0 || idx >= len(l.src) {
		return 0, false
	}
	return l.src[idx], true
}

func (l *Lexer) skipWhitespaceAndComments() error {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		if ch == '/' {
			if next, ok := l.peek(1); ok && next == '/' {
				// line comment
				for l.pos < len(l.src) && l.src[l.pos] != '\n' {
					l.advance()
				}
				continue
			}
			if next, ok := l.peek(1); ok && next == '*' {
				startLine, startCol := l.line, l.col
				l.advance() // /
				l.advance() // *
				for {
					if l.pos >= len(l.src) {
						return &LexError{File: l.file, Msg: "unterminated block comment", Line: startLine, Col: startCol}
					}
					if l.src[l.pos] == '*' {
						if next, ok := l.peek(1); ok && next == '/' {
							l.advance()
							l.advance()
							break
						}
					}
					l.advance()
				}
				continue
			}
		}
		return nil
	}
	return nil
}

func (l *Lexer) readString(quote rune, startLine, startCol int) (Token, error) {
	l.advance() // opening quote
	var b strings.Builder
	for {
		if l.pos >= len(l.src) {
			return Token{}, &LexError{File: l.file, Msg: "unterminated string literal", Line: startLine, Col: startCol}
		}
		ch := l.src[l.pos]
		if ch == quote {
			l.advance()
			return Token{Type: TOKEN_STRING, Lexeme: b.String(), Line: startLine, Col: startCol}, nil
		}
		if ch == '\\' {
			l.advance()
			if l.pos >= len(l.src) {
				return Token{}, &LexError{File: l.file, Msg: "unterminated escape in string", Line: startLine, Col: startCol}
			}
			esc := l.src[l.pos]
			switch esc {
			case 'n':
				b.WriteRune('\n')
			case 'r':
				b.WriteRune('\r')
			case 't':
				b.WriteRune('\t')
			case '\\':
				b.WriteRune('\\')
			case '"':
				b.WriteRune('"')
			case '\'':
				b.WriteRune('\'')
			case '0':
				b.WriteRune(0)
			default:
				return Token{}, &LexError{File: l.file, Msg: fmt.Sprintf("unknown escape sequence \\%c", esc), Line: l.line, Col: l.col}
			}
			l.advance()
			continue
		}
		b.WriteRune(ch)
		l.advance()
	}
}

func (l *Lexer) readVarRef(startLine, startCol int) (Token, error) {
	l.advance() // $
	if l.pos >= len(l.src) || !isIdentStart(l.src[l.pos]) {
		return Token{}, &LexError{File: l.file, Msg: "expected identifier after '$'", Line: startLine, Col: startCol}
	}
	var b strings.Builder
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		b.WriteRune(l.src[l.pos])
		l.advance()
	}
	name := b.String()
	if len(name) > 64 {
		return Token{}, &LexError{File: l.file, Msg: fmt.Sprintf("variable name %q exceeds 64 characters", name), Line: startLine, Col: startCol}
	}
	return Token{Type: TOKEN_VARREF, Lexeme: name, Line: startLine, Col: startCol}, nil
}

func (l *Lexer) readNumber(startLine, startCol int) (Token, error) {
	var b strings.Builder
	for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
		b.WriteRune(l.src[l.pos])
		l.advance()
	}
	// Float? Require a digit after `.` so that `3.j` (file-import-ish) still
	// lexes as INT(3) DOT IDENT(j). A trailing dot with no digit is also left
	// to the caller's interpretation.
	if l.pos+1 < len(l.src) && l.src[l.pos] == '.' && unicode.IsDigit(l.src[l.pos+1]) {
		b.WriteRune('.')
		l.advance()
		for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
			b.WriteRune(l.src[l.pos])
			l.advance()
		}
		return Token{Type: TOKEN_FLOAT, Lexeme: b.String(), Line: startLine, Col: startCol}, nil
	}
	return Token{Type: TOKEN_INT, Lexeme: b.String(), Line: startLine, Col: startCol}, nil
}

func (l *Lexer) readIdentifierOrKeyword(startLine, startCol int) (Token, error) {
	var b strings.Builder
	// Bare IDENTs may include `_` in the middle to support constant names
	// like MAX_RETRIES. The parser enforces per-context rules: constants
	// accept `[A-Z][A-Z_]*[A-Z]` (or single [A-Z]); variables, methods and
	// parameters reject `_` entirely.
	for l.pos < len(l.src) && isIdentPartLoose(l.src[l.pos]) {
		b.WriteRune(l.src[l.pos])
		l.advance()
	}
	name := b.String()
	if tt, ok := lookupKeyword(name); ok {
		return Token{Type: tt, Lexeme: name, Line: startLine, Col: startCol}, nil
	}
	if len(name) > 64 {
		return Token{}, &LexError{File: l.file, Msg: fmt.Sprintf("identifier %q exceeds 64 characters", name), Line: startLine, Col: startCol}
	}
	// Trailing `_` is never legal in any identifier kind.
	if name[len(name)-1] == '_' {
		return Token{}, &LexError{File: l.file, Msg: fmt.Sprintf("identifier %q may not end with `_`", name), Line: startLine, Col: startCol}
	}
	return Token{Type: TOKEN_IDENT, Lexeme: name, Line: startLine, Col: startCol}, nil
}

// isIdentStart: identifiers (variable / method / constant / parameter
// names, plus library names) must start with a letter. Digits and `_`
// are explicitly rejected as the first character.
func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// isIdentPart: variable references (`$name`) and similar contexts where
// the spec keeps names letters-only. Constant identifiers use the looser
// rule below.
func isIdentPart(r rune) bool {
	return isIdentStart(r)
}

// isIdentPartLoose: bare-IDENT continuation. Accepts `_` so the lexer can
// produce constant names like MAX_RETRIES as a single token. The trailing-
// `_` and per-context rules (uppercase-only for constants, no `_` at all
// for variables / methods / parameters) are enforced by the lexer's
// trailing-character check and by the parser at the relevant def sites.
func isIdentPartLoose(r rune) bool {
	return isIdentStart(r) || r == '_'
}
