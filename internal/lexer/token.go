// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package lexer

import "fmt"

type TokenType int

const (
	TOKEN_EOF TokenType = iota
	TOKEN_ILLEGAL

	// Literals
	TOKEN_INT
	TOKEN_STRING
	TOKEN_IDENT  // method names like app, printf, stdlib
	TOKEN_VARREF // $name

	// Keywords
	TOKEN_DEFINE // both `define` and `def` produce this token; they are synonyms
	TOKEN_AS
	TOKEN_INIT
	TOKEN_IMPORT
	TOKEN_INT_TYPE    // the word "int" used as a type
	TOKEN_STRING_TYPE // the word "string" used as a type

	// Punctuation
	TOKEN_LBRACE // {
	TOKEN_RBRACE // }
	TOKEN_LPAREN // (
	TOKEN_RPAREN // )
	TOKEN_SEMI   // ;
	TOKEN_COMMA  // ,
	TOKEN_ASSIGN // =
	TOKEN_DOT    // . (used in file-import paths, e.g. `import name.j;`)

	// Operators
	TOKEN_PLUS    // +
	TOKEN_MINUS   // -
	TOKEN_STAR    // *
	TOKEN_SLASH   // /
	TOKEN_PERCENT // %
)

var tokenNames = map[TokenType]string{
	TOKEN_EOF:         "EOF",
	TOKEN_ILLEGAL:     "ILLEGAL",
	TOKEN_INT:         "INT",
	TOKEN_STRING:      "STRING",
	TOKEN_IDENT:       "IDENT",
	TOKEN_VARREF:      "VARREF",
	TOKEN_DEFINE:      "DEFINE",
	TOKEN_AS:          "AS",
	TOKEN_INIT:        "INIT",
	TOKEN_IMPORT:      "IMPORT",
	TOKEN_INT_TYPE:    "INT_TYPE",
	TOKEN_STRING_TYPE: "STRING_TYPE",
	TOKEN_LBRACE:      "LBRACE",
	TOKEN_RBRACE:      "RBRACE",
	TOKEN_LPAREN:      "LPAREN",
	TOKEN_RPAREN:      "RPAREN",
	TOKEN_SEMI:        "SEMI",
	TOKEN_COMMA:       "COMMA",
	TOKEN_ASSIGN:      "ASSIGN",
	TOKEN_DOT:         "DOT",
	TOKEN_PLUS:        "PLUS",
	TOKEN_MINUS:       "MINUS",
	TOKEN_STAR:        "STAR",
	TOKEN_SLASH:       "SLASH",
	TOKEN_PERCENT:     "PERCENT",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// Token is one lexeme produced by the scanner.
// Lexeme holds the literal text for identifiers, numbers, and unprocessed strings.
// For TOKEN_STRING, Lexeme holds the already-escape-processed value (no surrounding quotes).
// For TOKEN_VARREF, Lexeme holds the variable name without the leading "$".
// File is the source filename the token came from (empty if unknown / from a string).
type Token struct {
	Type   TokenType
	Lexeme string
	Line   int
	Col    int
	File   string
}

func (t Token) String() string {
	if t.File != "" {
		return fmt.Sprintf("%s(%q) @%s:%d:%d", t.Type, t.Lexeme, t.File, t.Line, t.Col)
	}
	return fmt.Sprintf("%s(%q) @%d:%d", t.Type, t.Lexeme, t.Line, t.Col)
}

var keywords = map[string]TokenType{
	"define": TOKEN_DEFINE,
	"def":    TOKEN_DEFINE, // synonym
	"as":     TOKEN_AS,
	"init":   TOKEN_INIT,
	"import": TOKEN_IMPORT,
	"int":    TOKEN_INT_TYPE,
	"string": TOKEN_STRING_TYPE,
}

func lookupKeyword(ident string) (TokenType, bool) {
	tt, ok := keywords[ident]
	return tt, ok
}
