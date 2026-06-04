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
	Line int
	Col  int
}

func (e *LexError) Error() string {
	return fmt.Sprintf("lex error at %d:%d: %s", e.Line, e.Col, e.Msg)
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
		return Token{Type: TOKEN_ASSIGN, Lexeme: "=", Line: startLine, Col: startCol}, nil
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

	return Token{}, &LexError{Msg: fmt.Sprintf("unexpected character %q", ch), Line: startLine, Col: startCol}
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
						return &LexError{Msg: "unterminated block comment", Line: startLine, Col: startCol}
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
			return Token{}, &LexError{Msg: "unterminated string literal", Line: startLine, Col: startCol}
		}
		ch := l.src[l.pos]
		if ch == quote {
			l.advance()
			return Token{Type: TOKEN_STRING, Lexeme: b.String(), Line: startLine, Col: startCol}, nil
		}
		if ch == '\\' {
			l.advance()
			if l.pos >= len(l.src) {
				return Token{}, &LexError{Msg: "unterminated escape in string", Line: startLine, Col: startCol}
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
				return Token{}, &LexError{Msg: fmt.Sprintf("unknown escape sequence \\%c", esc), Line: l.line, Col: l.col}
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
		return Token{}, &LexError{Msg: "expected identifier after '$'", Line: startLine, Col: startCol}
	}
	var b strings.Builder
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		b.WriteRune(l.src[l.pos])
		l.advance()
	}
	name := b.String()
	if len(name) > 64 {
		return Token{}, &LexError{Msg: fmt.Sprintf("variable name %q exceeds 64 characters", name), Line: startLine, Col: startCol}
	}
	return Token{Type: TOKEN_VARREF, Lexeme: name, Line: startLine, Col: startCol}, nil
}

func (l *Lexer) readNumber(startLine, startCol int) (Token, error) {
	var b strings.Builder
	for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
		b.WriteRune(l.src[l.pos])
		l.advance()
	}
	return Token{Type: TOKEN_INT, Lexeme: b.String(), Line: startLine, Col: startCol}, nil
}

func (l *Lexer) readIdentifierOrKeyword(startLine, startCol int) (Token, error) {
	var b strings.Builder
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		b.WriteRune(l.src[l.pos])
		l.advance()
	}
	name := b.String()
	if tt, ok := lookupKeyword(name); ok {
		return Token{Type: tt, Lexeme: name, Line: startLine, Col: startCol}, nil
	}
	if len(name) > 64 {
		return Token{}, &LexError{Msg: fmt.Sprintf("identifier %q exceeds 64 characters", name), Line: startLine, Col: startCol}
	}
	return Token{Type: TOKEN_IDENT, Lexeme: name, Line: startLine, Col: startCol}, nil
}

// Spec: variable/constant/method names use [A-Za-z]. Digits and underscores are NOT allowed.
func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentPart(r rune) bool {
	return isIdentStart(r)
}
