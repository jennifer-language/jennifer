// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package preproc handles Jennifer's textual-splice preprocessor.
//
// A file splice has the form `include "name.j";` and is replaced, at the
// location it appears, by the tokens of the referenced file. The path is
// resolved relative to the directory of the file that contains the
// include. Includes are processed recursively, with a cycle check to
// prevent infinite inclusion.
//
// `import "name.j" [as NAME];` is a module import - a real statement
// handled by the parser and interpreter, not a textual splice. Its tokens
// pass through the preprocessor unchanged (like `use`); only the common
// unquoted mistake (`import foo;`) is caught here with a positioned hint.
//
// Library imports use the `use` keyword (e.g. `use io;`) and are left in
// place; the parser turns them into ImportStmt nodes.
package preproc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// errReason returns a filesystem error's reason without the (possibly huge)
// path it embeds. An *os.PathError's Error() is "op path: reason"; the path is
// already shown truncated elsewhere in the message, so only the reason is kept.
func errReason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return truncPath(err.Error())
}

// PreprocessError carries context across files.
type PreprocessError struct {
	Msg  string
	File string
	Line int
	Col  int
}

func (e *PreprocessError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("preprocess error at %d:%d: %s", e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("preprocess error at %s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
}

// Position implements the positioned-error interface used by the CLI.
func (e *PreprocessError) Position() (file string, line, col int) {
	return e.File, e.Line, e.Col
}

// Process expands all file imports in `tokens`.
// `baseDir` is the directory used to resolve relative `.j` filenames.
// `selfPath`, if non-empty, is the absolute path of the file that produced
// `tokens`; it is added to the visited set so a file can't import itself
// transitively.
func Process(tokens []lexer.Token, baseDir, selfPath string) ([]lexer.Token, error) {
	visited := map[string]bool{}
	if selfPath != "" {
		visited[canonicalPath(selfPath)] = true
	}
	// Trivia tokens (comments, blank lines) carry no semantic
	// content. Drop them here so the include / use / import
	// recognizers can rely on adjacent tokens being meaningful. The
	// formatter doesn't go through the preprocessor; the parser
	// strips any survivors as a defensive last step.
	tokens = stripTrivia(tokens)
	return processTokens(tokens, baseDir, visited, &expandCtx{cache: map[string][]lexer.Token{}})
}

// maxSplicedTokens bounds the total tokens spliced in across every `include`
// expansion. Per-path cycle detection is an undo-stack (add on entry, remove on
// exit), not a global set, so a diamond re-expands correctly - but a chain where each file includes
// the previous one twice doubles at every level, so ~30 tiny files reach 2^30
// tokens and exhaust memory. Counting each file's own tokens once per inclusion
// makes that exponential growth trip this bound early, while a linear or
// diamond include tree (each file's tokens counted per actual inclusion) stays
// well under it. Untrusted *code* is outside the threat model; this catches the
// accidental blow-up in a generated-source pipeline.
const maxSplicedTokens = 2_000_000

// maxSourceBytes is the per-included-file size cap. It defaults to
// limits.MaxSourceBytes and is a package var (not the const directly) only so a
// test can lower it without writing a 64 MiB file; production code never writes it.
var maxSourceBytes = int64(limits.MaxSourceBytes)

// expandCtx carries state across the whole include expansion: the running count
// of spliced tokens (against maxSplicedTokens) and a per-path token cache so a
// file included many times (a diamond, or the pathological doubling chain) is
// read and tokenized once, not once per inclusion. The cache also keeps the
// bound cheap to reach: an exponential chain trips it from memory rather than
// after hundreds of thousands of re-reads.
type expandCtx struct {
	total int
	depth int                      // current include-recursion depth (guards the Go stack)
	cache map[string][]lexer.Token // absPath -> stripped raw (pre-expansion) tokens
}

// truncPath bounds a path for an error message: an `include` path is a source
// string literal (and the resolved absolute path derives from it), so echoing it
// whole would turn a 1 MiB path into a 1 MiB diagnostic. Keeps the first ~256
// runes and marks the truncation; the rune loop stops at the cap, so it never
// materialises a big []rune.
func truncPath(p string) string {
	const maxRunes = 256
	n := 0
	for i := range p {
		if n == maxRunes {
			return p[:i] + "..."
		}
		n++
	}
	return p
}

// canonicalPath resolves a path to its symlink-free absolute form, the key used
// for cycle detection and the read-once cache. Keying on the canonical physical
// file (not the lexical spelling) means two spellings of one file - a symlink and
// its target, or two symlinks to it - dedupe: the file is read, counted against
// the token budget, and cached once, and a self-include through an alias is still
// caught as circular. EvalSymlinks needs the file to exist (and errors on a
// symlink loop); when it can't resolve, fall back to the absolute lexical path so
// the subsequent read still reports a clean "cannot read" error.
func canonicalPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		return resolved
	}
	return abs
}

func isTrivia(t lexer.TokenType) bool {
	switch t {
	case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK,
		lexer.TOKEN_COMMENT_SHEBANG, lexer.TOKEN_BLANK_LINE:
		return true
	}
	return false
}

func stripTrivia(toks []lexer.Token) []lexer.Token {
	// Fast path: a file with no trivia (comment-free generated source) needs no
	// filtering, so return the slice as-is rather than copying it whole. Callers
	// only read the result, so sharing the backing array is safe here.
	hasTrivia := false
	for _, t := range toks {
		if isTrivia(t.Type) {
			hasTrivia = true
			break
		}
	}
	if !hasTrivia {
		return toks
	}
	// Allocate a fresh slice rather than compacting in place (out := toks[:0]):
	// an in-place compaction corrupts any caller that still holds the original
	// slice.
	out := make([]lexer.Token, 0, len(toks))
	for _, t := range toks {
		if isTrivia(t.Type) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func processTokens(tokens []lexer.Token, baseDir string, visited map[string]bool, ctx *expandCtx) ([]lexer.Token, error) {
	out := make([]lexer.Token, 0, len(tokens))
	i := 0
	for i < len(tokens) {
		tok := tokens[i]

		// `include "path/file.j";` - textual file splice
		if tok.Type == lexer.TOKEN_INCLUDE {
			expanded, advance, err := handleInclude(tokens, i, baseDir, visited, ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded...)
			i += advance
			continue
		}

		// `import "path.j" [as NAME];` - a module import. It is a real
		// statement (parser + interpreter), not a preprocessor splice like
		// `include`, so the tokens pass through unchanged to the parser.
		// The one thing checked here is the common unquoted mistake.
		if tok.Type == lexer.TOKEN_IMPORT {
			if err := validateModuleImport(tokens, i); err != nil {
				return nil, err
			}
			out = append(out, tok)
			i++
			continue
		}

		// `use NAME ;` - library import. Check for a common mistake
		// (`use file.j;`) and produce a helpful error.
		if tok.Type == lexer.TOKEN_USE {
			if err := validateUse(tokens, i); err != nil {
				return nil, err
			}
			// pass through unchanged
			out = append(out, tok)
			i++
			continue
		}

		out = append(out, tok)
		i++
	}
	return out, nil
}

// handleInclude processes an `include` token. Possible shapes:
//
//	include "path.j" ;     (canonical textual file splice)
//	include NAME ;          (looks like a library form - error: use `use NAME;`)
//	include NAME.j ;        (old unquoted form - error: quote the path)
//
// Returns the spliced tokens (empty if it was a pass-through) and the number
// of input tokens consumed.
func handleInclude(tokens []lexer.Token, i int, baseDir string, visited map[string]bool, ctx *expandCtx) ([]lexer.Token, int, error) {
	inc := tokens[i]
	// Defensive: `include` is never the final token today (the lexer always
	// appends TOKEN_EOF), but reading tokens[i+1] on that invariant held at a
	// distance is a panic-by-refactor risk. Guard it, mirroring validateModuleImport.
	if i+1 >= len(tokens) {
		return nil, 0, &PreprocessError{Msg: "`include` must be followed by a quoted path", File: inc.File, Line: inc.Line, Col: inc.Col}
	}
	next := tokens[i+1]

	// `include "path.j" ;`
	if next.Type == lexer.TOKEN_STRING && i+2 < len(tokens) && tokens[i+2].Type == lexer.TOKEN_SEMI {
		path := next.Lexeme
		if !strings.HasSuffix(path, ".j") {
			return nil, 0, &PreprocessError{
				Msg:  fmt.Sprintf("include path %q must end with `.j`", truncPath(path)),
				File: next.File, Line: next.Line, Col: next.Col,
			}
		}
		spliced, err := spliceFile(path, baseDir, visited, next, ctx)
		if err != nil {
			return nil, 0, err
		}
		return spliced, 3, nil // include STRING ;
	}

	// `include NAME ;` - looks like the library form
	if next.Type == lexer.TOKEN_IDENT && i+2 < len(tokens) && tokens[i+2].Type == lexer.TOKEN_SEMI {
		return nil, 0, &PreprocessError{
			Msg:  fmt.Sprintf("`include` is for files; use `use %s;` for system libraries", next.Lexeme),
			File: inc.File, Line: inc.Line, Col: inc.Col,
		}
	}

	// `include NAME.j ;` - the old unquoted file form
	if next.Type == lexer.TOKEN_IDENT && i+4 < len(tokens) &&
		tokens[i+2].Type == lexer.TOKEN_DOT &&
		tokens[i+3].Type == lexer.TOKEN_IDENT &&
		tokens[i+4].Type == lexer.TOKEN_SEMI {
		return nil, 0, &PreprocessError{
			Msg:  fmt.Sprintf("file splices take a string literal: `include \"%s.%s\";`", next.Lexeme, tokens[i+3].Lexeme),
			File: inc.File, Line: inc.Line, Col: inc.Col,
		}
	}

	return nil, 0, &PreprocessError{
		Msg:  "expected `include \"path.j\";`",
		File: inc.File, Line: inc.Line, Col: inc.Col,
	}
}

// validateModuleImport catches the common unquoted mistake right after an
// `import` keyword: a module path must be a quoted string. A quoted path
// passes through to the parser, which builds the ModuleImportStmt.
func validateModuleImport(tokens []lexer.Token, i int) error {
	imp := tokens[i]
	if i+1 >= len(tokens) {
		return nil // a truncated statement; the parser reports it
	}
	next := tokens[i+1]
	if next.Type == lexer.TOKEN_IDENT {
		// `import foo.j;` (unquoted path) vs `import foo;` (looks like a use).
		if i+3 < len(tokens) && tokens[i+2].Type == lexer.TOKEN_DOT && tokens[i+3].Type == lexer.TOKEN_IDENT {
			return &PreprocessError{
				Msg:  fmt.Sprintf("module paths are quoted: `import \"%s.%s\";`", next.Lexeme, tokens[i+3].Lexeme),
				File: imp.File, Line: imp.Line, Col: imp.Col,
			}
		}
		return &PreprocessError{
			Msg:  fmt.Sprintf("`import` takes a quoted module path (`import \"%s.j\";`); for a system library use `use %s;`", next.Lexeme, next.Lexeme),
			File: imp.File, Line: imp.Line, Col: imp.Col,
		}
	}
	return nil
}

func spliceFile(path, baseDir string, visited map[string]bool, originTok lexer.Token, ctx *expandCtx) ([]lexer.Token, error) {
	fullPath := path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(baseDir, path)
	}
	absPath := canonicalPath(fullPath)
	if visited[absPath] {
		return nil, &PreprocessError{
			Msg:  fmt.Sprintf("circular import: %s is already being included", truncPath(absPath)),
			File: originTok.File, Line: originTok.Line, Col: originTok.Col,
		}
	}
	// A deep *linear* chain (a -> b -> c -> ...) slips under the token budget
	// (each file adds only its own few tokens) but recurses one Go frame per
	// level. Uncapped, a generated chain overflows the fixed TinyGo goroutine
	// stack into a fatal, uncatchable crash; cap it like the parser's nesting.
	if ctx.depth >= limits.MaxNestingDepth {
		return nil, &PreprocessError{
			Msg:  fmt.Sprintf("include nesting exceeds %d levels", limits.MaxNestingDepth),
			File: originTok.File, Line: originTok.Line, Col: originTok.Col,
		}
	}
	ctx.depth++
	defer func() { ctx.depth-- }()
	// Read + tokenize each file once, then serve every later inclusion from the
	// cache. The file content is fixed for the run, so a cached copy is exact;
	// this is what makes a diamond cheap and the pathological doubling chain
	// trip the bound below from memory rather than after N re-reads.
	incToks, cached := ctx.cache[absPath]
	if !cached {
		// Reject an oversized file from its stat before os.ReadFile slurps the
		// whole thing into memory (the token budget bounds the token stream, but a
		// huge non-token-dense file would still be committed by the read).
		if info, serr := os.Stat(fullPath); serr == nil && info.Size() > maxSourceBytes {
			return nil, &PreprocessError{
				Msg:  fmt.Sprintf("imported file %q is too large (%d bytes; limit %d)", truncPath(fullPath), info.Size(), maxSourceBytes),
				File: originTok.File, Line: originTok.Line, Col: originTok.Col,
			}
		}
		srcBytes, rerr := os.ReadFile(fullPath)
		if rerr != nil {
			return nil, &PreprocessError{
				Msg:  fmt.Sprintf("cannot read imported file %q: %s", truncPath(fullPath), errReason(rerr)),
				File: originTok.File, Line: originTok.Line, Col: originTok.Col,
			}
		}
		toks, terr := lexer.TokenizeWithFile(string(srcBytes), fullPath)
		if terr != nil {
			return nil, terr
		}
		// Strip the included file's trivia (comments, blank lines) so the
		// include / use / import recognizers see meaningful adjacent tokens -
		// the same normalization the top-level Process applies.
		incToks = stripTrivia(toks)
		ctx.cache[absPath] = incToks
	}
	// Count this file's own tokens for every inclusion, before recursing. An
	// exponential include chain trips the bound here early (a file at depth k
	// included twice per level is counted 2^k times); a linear / diamond tree
	// accrues only its actual inclusions.
	ctx.total += len(incToks)
	if ctx.total > maxSplicedTokens {
		return nil, &PreprocessError{
			Msg:  fmt.Sprintf("include expansion is too large (over %d spliced tokens); a chain of files that each `include` the previous more than once expands exponentially", maxSplicedTokens),
			File: originTok.File, Line: originTok.Line, Col: originTok.Col,
		}
	}
	// Mark this file as on the current include path, recurse, then unmark on the
	// way out - an undo-stack rather than a per-inclusion copy of the whole
	// visited set. A downstream sibling include then sees a clean set (a diamond
	// re-includes correctly), while a cycle back to this file is still caught.
	// O(1) per inclusion instead of O(depth), so a deep chain is linear, not
	// quadratic in time and memory.
	visited[absPath] = true
	expanded, err := processTokens(incToks, filepath.Dir(fullPath), visited, ctx)
	delete(visited, absPath)
	if err != nil {
		return nil, err
	}
	// drop trailing EOF
	out := make([]lexer.Token, 0, len(expanded))
	for _, t := range expanded {
		if t.Type == lexer.TOKEN_EOF {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// validateUse catches the common mistake `use foo.j;` (using `use` for a file).
// `use NAME;` is fine and passes through unchanged.
func validateUse(tokens []lexer.Token, i int) error {
	if i+4 >= len(tokens) {
		return nil
	}
	if tokens[i+1].Type == lexer.TOKEN_IDENT &&
		tokens[i+2].Type == lexer.TOKEN_DOT &&
		tokens[i+3].Type == lexer.TOKEN_IDENT &&
		tokens[i+4].Type == lexer.TOKEN_SEMI {
		t := tokens[i]
		return &PreprocessError{
			Msg:  fmt.Sprintf("`use` is for system libraries; for files use `include \"%s.%s\";`", tokens[i+1].Lexeme, tokens[i+3].Lexeme),
			File: t.File, Line: t.Line, Col: t.Col,
		}
	}
	return nil
}
