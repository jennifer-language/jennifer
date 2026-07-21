// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"os"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
)

// ANSI SGR foreground codes for each token category. Foreground-only (no
// backgrounds), so colouring the whitespace that trails a token is invisible
// and the scheme stays readable on both light and dark terminals. The
// categories mirror the editor highlight definitions in editors/.
const (
	sgrKeyword = "35" // magenta - def / func / if / return / use / import / ...
	sgrType    = "36" // cyan    - int / float / string / list / map / task / ...
	sgrString  = "32" // green   - "..." / '...'
	sgrNumber  = "33" // yellow  - int / float literals, true / false / null
	sgrVar     = "94" // bright blue - $variable references
	sgrComment = "90" // bright black (grey) - # ... and /* ... */
)

// colorForToken returns the SGR parameter for a token type, or "" for tokens
// that render in the terminal's default colour (identifiers, operators,
// punctuation).
func colorForToken(t lexer.TokenType) string {
	switch t {
	case lexer.TOKEN_INT, lexer.TOKEN_FLOAT,
		lexer.TOKEN_TRUE, lexer.TOKEN_FALSE, lexer.TOKEN_NULL:
		return sgrNumber
	case lexer.TOKEN_STRING:
		return sgrString
	case lexer.TOKEN_VARREF:
		return sgrVar
	case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK, lexer.TOKEN_COMMENT_SHEBANG:
		return sgrComment
	case lexer.TOKEN_INT_TYPE, lexer.TOKEN_FLOAT_TYPE, lexer.TOKEN_STRING_TYPE,
		lexer.TOKEN_BOOL_TYPE, lexer.TOKEN_BYTES_TYPE,
		lexer.TOKEN_LIST, lexer.TOKEN_MAP, lexer.TOKEN_TASK:
		return sgrType
	case lexer.TOKEN_DEFINE, lexer.TOKEN_FUNC, lexer.TOKEN_AS, lexer.TOKEN_INIT,
		lexer.TOKEN_CONST, lexer.TOKEN_INCLUDE, lexer.TOKEN_IMPORT, lexer.TOKEN_USE,
		lexer.TOKEN_RETURN, lexer.TOKEN_IF, lexer.TOKEN_ELSEIF, lexer.TOKEN_ELSE,
		lexer.TOKEN_WHILE, lexer.TOKEN_FOR, lexer.TOKEN_REPEAT, lexer.TOKEN_UNTIL,
		lexer.TOKEN_BREAK, lexer.TOKEN_CONTINUE, lexer.TOKEN_EXIT, lexer.TOKEN_TRY,
		lexer.TOKEN_CATCH, lexer.TOKEN_THROW, lexer.TOKEN_AND, lexer.TOKEN_OR,
		lexer.TOKEN_NOT, lexer.TOKEN_OF, lexer.TOKEN_TO, lexer.TOKEN_IN,
		lexer.TOKEN_STRUCT, lexer.TOKEN_LEN, lexer.TOKEN_SPAWN:
		return sgrKeyword
	default:
		return ""
	}
}

// highlightLine returns src wrapped in ANSI colour escapes, one span per
// token. It operates on a single physical line (the REPL reads one line per
// readLine call), using each token's 1-based rune column to slice the source
// so the user's exact spacing is preserved. A token's span runs to the start
// of the next token, so trailing whitespace inherits the token's colour -
// invisible for foreground-only codes. On any lex error the input is returned
// unchanged, so a partial or malformed line still echoes verbatim.
func highlightLine(src string) string {
	toks, err := lexer.Tokenize(src)
	if err != nil {
		return src
	}
	runes := []rune(src)
	var b strings.Builder
	prevEnd := 0
	for i, t := range toks {
		if t.Type == lexer.TOKEN_EOF {
			break
		}
		start := t.Col - 1
		if start < 0 {
			start = 0
		}
		if start > len(runes) {
			start = len(runes)
		}
		// Leading text before the first token (indentation) stays plain.
		if start > prevEnd {
			b.WriteString(string(runes[prevEnd:start]))
		}
		// The span ends where the next non-EOF token begins, or at line end.
		end := len(runes)
		if i+1 < len(toks) {
			if nc := toks[i+1].Col - 1; nc >= start && nc <= len(runes) {
				end = nc
			}
		}
		if end < start {
			end = start
		}
		seg := string(runes[start:end])
		if sgr := colorForToken(t.Type); sgr != "" {
			b.WriteString("\x1b[")
			b.WriteString(sgr)
			b.WriteString("m")
			b.WriteString(seg)
			b.WriteString("\x1b[0m")
		} else {
			b.WriteString(seg)
		}
		prevEnd = end
	}
	if prevEnd < len(runes) {
		b.WriteString(string(runes[prevEnd:]))
	}
	return b.String()
}

// colorEnabled reports whether REPL output should carry ANSI colour: stdout
// must be a terminal and the NO_COLOR convention (https://no-color.org) must
// not be set. The line editor itself only runs when stdin is a TTY; this adds
// the stdout-side checks so redirected output stays clean.
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTTY(os.Stdout)
}
