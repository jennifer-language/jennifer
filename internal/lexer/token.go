// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lexer

import "fmt"

type TokenType int

const (
	TOKEN_EOF TokenType = iota
	TOKEN_ILLEGAL

	// Literals
	TOKEN_INT
	TOKEN_FLOAT
	TOKEN_STRING
	TOKEN_IDENT  // method names like app, printf, stdlib
	TOKEN_VARREF // $name

	// Keywords
	TOKEN_DEFINE // `def` keyword; introduces a variable or constant
	TOKEN_FUNC   // `func` keyword; introduces a method
	TOKEN_AS
	TOKEN_INIT
	TOKEN_CONST
	TOKEN_INCLUDE // textual file splice: `include "name.j";`
	TOKEN_IMPORT  // reserved word, no live syntax (file splice moved to `include`; reserved for the module system)
	TOKEN_USE     // library import: `use io;`
	TOKEN_RETURN
	TOKEN_IF
	TOKEN_ELSEIF
	TOKEN_ELSE
	TOKEN_WHILE
	TOKEN_FOR
	TOKEN_REPEAT   // `repeat { ... } until (cond);` post-test loop
	TOKEN_UNTIL    // companion to `repeat`
	TOKEN_BREAK    // exit innermost loop
	TOKEN_CONTINUE // skip to next iteration of innermost loop
	TOKEN_EXIT     // terminate the program; optional int expression sets exit code
	TOKEN_TRY      // `try { ... } catch (NAME) { ... }` catchable error block
	TOKEN_CATCH    // companion to `try`; binds the thrown value
	TOKEN_THROW    // `throw EXPR;` raises a catchable error
	TOKEN_DEFER    // `defer CALL(args);` schedules a call for block exit
	TOKEN_ERRDEFER // `errdefer CALL(args);` schedules a call for error exit only
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_NULL
	TOKEN_AND
	TOKEN_OR
	TOKEN_NOT
	TOKEN_DIV         // `//` operator: floor (integer) division; `/` is true division (Python 3 style)
	TOKEN_INT_TYPE    // the word "int" used as a type
	TOKEN_FLOAT_TYPE  // the word "float" used as a type
	TOKEN_STRING_TYPE // the word "string" used as a type
	TOKEN_BOOL_TYPE   // the word "bool" used as a type
	TOKEN_BYTES_TYPE  // the word "bytes" used as a type

	// Compound-type keywords
	TOKEN_LIST   // the word "list" used as a type
	TOKEN_MAP    // the word "map" used as a type
	TOKEN_OF     // "of" - element-type separator: `list of int`, `map of K to V`
	TOKEN_TO     // "to" - K/V separator inside `map of K to V`
	TOKEN_IN     // "in" - for-each iterator: `for (def x in $coll)`
	TOKEN_STRUCT // `def struct Name { field as type, ... };`
	TOKEN_LEN    // `len(EXPR)` polymorphic structural-length built-in
	TOKEN_TASK   // the word "task" used as a type: `task of T`
	TOKEN_SPAWN  // `spawn { ... }` block primary expression producing a `task of T`
	TOKEN_EXPORT // `export` marker before a module's public `def const` / `def struct` / `func`

	// Punctuation
	TOKEN_LBRACE   // {
	TOKEN_RBRACE   // }
	TOKEN_LPAREN   // (
	TOKEN_RPAREN   // )
	TOKEN_LBRACKET // [ (list literals + index expressions)
	TOKEN_RBRACKET // ]
	TOKEN_SEMI     // ;
	TOKEN_COMMA    // ,
	TOKEN_COLON    // : (map literal key-value separator)
	TOKEN_ASSIGN   // =
	TOKEN_DOT      // . (reserved; future namespacing / field access)

	// Arithmetic operators
	TOKEN_PLUS    // +
	TOKEN_MINUS   // -
	TOKEN_STAR    // *
	TOKEN_SLASH   // /
	TOKEN_PERCENT // %

	// Comparison operators
	TOKEN_LT  // <
	TOKEN_GT  // >
	TOKEN_LE  // <=
	TOKEN_GE  // >=
	TOKEN_EQ  // ==
	TOKEN_NEQ // !=

	// Bitwise operators
	TOKEN_BIT_AND // & - bitwise AND on int
	TOKEN_BIT_OR  // | - bitwise OR on int
	TOKEN_BIT_XOR // ^ - bitwise XOR on int
	TOKEN_BIT_NOT // ~ - bitwise NOT on int (unary)
	TOKEN_SHL     // <<
	TOKEN_SHR     // >>

	// Trivia tokens: comments and blank lines are emitted as
	// tokens so the formatter can round-trip them. The parser skips
	// them at statement boundaries via a wrapper around peek/advance
	// (see parser.skipTrivia); attachment to AST nodes is done at
	// statement boundaries as part of the parsing loop. Lexeme holds
	// the comment text including its delimiters (`#...`, `/* ... */`,
	// `#!...`); TOKEN_BLANK_LINE has empty Lexeme.
	TOKEN_COMMENT_LINE    // `# ...` to end of line
	TOKEN_COMMENT_BLOCK   // `/* ... */`, may be nested
	TOKEN_COMMENT_SHEBANG // `#!...` on line 1 only
	TOKEN_BLANK_LINE      // one or more blank lines collapsed into one
)

var tokenNames = map[TokenType]string{
	TOKEN_EOF:         "EOF",
	TOKEN_ILLEGAL:     "ILLEGAL",
	TOKEN_INT:         "INT",
	TOKEN_FLOAT:       "FLOAT",
	TOKEN_STRING:      "STRING",
	TOKEN_IDENT:       "IDENT",
	TOKEN_VARREF:      "VARREF",
	TOKEN_DEFINE:      "DEF",
	TOKEN_FUNC:        "FUNC",
	TOKEN_AS:          "AS",
	TOKEN_INIT:        "INIT",
	TOKEN_CONST:       "CONST",
	TOKEN_INCLUDE:     "INCLUDE",
	TOKEN_IMPORT:      "IMPORT",
	TOKEN_USE:         "USE",
	TOKEN_RETURN:      "RETURN",
	TOKEN_IF:          "IF",
	TOKEN_ELSEIF:      "ELSEIF",
	TOKEN_ELSE:        "ELSE",
	TOKEN_WHILE:       "WHILE",
	TOKEN_FOR:         "FOR",
	TOKEN_REPEAT:      "REPEAT",
	TOKEN_UNTIL:       "UNTIL",
	TOKEN_BREAK:       "BREAK",
	TOKEN_CONTINUE:    "CONTINUE",
	TOKEN_EXIT:        "EXIT",
	TOKEN_TRY:         "TRY",
	TOKEN_CATCH:       "CATCH",
	TOKEN_THROW:       "THROW",
	TOKEN_DEFER:       "DEFER",
	TOKEN_ERRDEFER:    "ERRDEFER",
	TOKEN_TRUE:        "TRUE",
	TOKEN_FALSE:       "FALSE",
	TOKEN_NULL:        "NULL",
	TOKEN_AND:         "AND",
	TOKEN_OR:          "OR",
	TOKEN_NOT:         "NOT",
	TOKEN_DIV:         "DIV",
	TOKEN_INT_TYPE:    "INT_TYPE",
	TOKEN_FLOAT_TYPE:  "FLOAT_TYPE",
	TOKEN_BYTES_TYPE:  "BYTES_TYPE",
	TOKEN_STRING_TYPE: "STRING_TYPE",
	TOKEN_BOOL_TYPE:   "BOOL_TYPE",
	TOKEN_LIST:        "LIST",
	TOKEN_MAP:         "MAP",
	TOKEN_OF:          "OF",
	TOKEN_TO:          "TO",
	TOKEN_IN:          "IN",
	TOKEN_STRUCT:      "STRUCT",
	TOKEN_LEN:         "LEN",
	TOKEN_TASK:        "TASK",
	TOKEN_SPAWN:       "SPAWN",
	TOKEN_EXPORT:      "EXPORT",
	TOKEN_LBRACE:      "LBRACE",
	TOKEN_RBRACE:      "RBRACE",
	TOKEN_LPAREN:      "LPAREN",
	TOKEN_RPAREN:      "RPAREN",
	TOKEN_LBRACKET:    "LBRACKET",
	TOKEN_RBRACKET:    "RBRACKET",
	TOKEN_SEMI:        "SEMI",
	TOKEN_COMMA:       "COMMA",
	TOKEN_COLON:       "COLON",
	TOKEN_ASSIGN:      "ASSIGN",
	TOKEN_DOT:         "DOT",
	TOKEN_PLUS:        "PLUS",
	TOKEN_MINUS:       "MINUS",
	TOKEN_STAR:        "STAR",
	TOKEN_SLASH:       "SLASH",
	TOKEN_PERCENT:     "PERCENT",
	TOKEN_LT:          "LT",
	TOKEN_GT:          "GT",
	TOKEN_LE:          "LE",
	TOKEN_GE:          "GE",
	TOKEN_EQ:          "EQ",
	TOKEN_NEQ:         "NEQ",
	TOKEN_BIT_AND:     "BIT_AND",
	TOKEN_BIT_OR:      "BIT_OR",
	TOKEN_BIT_XOR:     "BIT_XOR",
	TOKEN_BIT_NOT:     "BIT_NOT",
	TOKEN_SHL:         "SHL",
	TOKEN_SHR:         "SHR",

	TOKEN_COMMENT_LINE:    "COMMENT_LINE",
	TOKEN_COMMENT_BLOCK:   "COMMENT_BLOCK",
	TOKEN_COMMENT_SHEBANG: "COMMENT_SHEBANG",
	TOKEN_BLANK_LINE:      "BLANK_LINE",
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
	"def":      TOKEN_DEFINE,
	"func":     TOKEN_FUNC,
	"as":       TOKEN_AS,
	"init":     TOKEN_INIT,
	"const":    TOKEN_CONST,
	"include":  TOKEN_INCLUDE,
	"import":   TOKEN_IMPORT,
	"use":      TOKEN_USE,
	"return":   TOKEN_RETURN,
	"if":       TOKEN_IF,
	"elseif":   TOKEN_ELSEIF,
	"else":     TOKEN_ELSE,
	"while":    TOKEN_WHILE,
	"for":      TOKEN_FOR,
	"repeat":   TOKEN_REPEAT,
	"until":    TOKEN_UNTIL,
	"break":    TOKEN_BREAK,
	"continue": TOKEN_CONTINUE,
	"exit":     TOKEN_EXIT,
	"try":      TOKEN_TRY,
	"catch":    TOKEN_CATCH,
	"throw":    TOKEN_THROW,
	"defer":    TOKEN_DEFER,
	"errdefer": TOKEN_ERRDEFER,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
	"null":     TOKEN_NULL,
	"and":      TOKEN_AND,
	"or":       TOKEN_OR,
	"not":      TOKEN_NOT,
	"int":      TOKEN_INT_TYPE,
	"float":    TOKEN_FLOAT_TYPE,
	"string":   TOKEN_STRING_TYPE,
	"bool":     TOKEN_BOOL_TYPE,
	"bytes":    TOKEN_BYTES_TYPE,
	"list":     TOKEN_LIST,
	"map":      TOKEN_MAP,
	"of":       TOKEN_OF,
	"to":       TOKEN_TO,
	"in":       TOKEN_IN,
	"struct":   TOKEN_STRUCT,
	"len":      TOKEN_LEN,
	"task":     TOKEN_TASK,
	"spawn":    TOKEN_SPAWN,
	"export":   TOKEN_EXPORT,
}

func lookupKeyword(ident string) (TokenType, bool) {
	tt, ok := keywords[ident]
	return tt, ok
}
