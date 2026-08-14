// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/reqcheck"
)

// runFmt formats the named files per docs/user-guide/style-guide.md. Without a
// flag it writes a single file's canonical form to stdout; with -w / --write it
// rewrites each file in place (only touching files whose content actually
// changes); with -l / --check it lists the files that are not already formatted
// and exits non-zero, mutating nothing - the CI / publish-gate mode. File
// selection - globs, recursion - is the shell's job, so a directory argument is
// a usage error, not a tree walk.
//
// The formatter operates on the token stream rather than the AST, so it preserves
// `import "file.j";` statements verbatim (the preprocessor would otherwise inline
// them), the parentheses the user wrote (the AST erases redundant grouping), and
// comments / blank lines (the lexer emits them as trivia tokens re-placed by
// position; an in-expression `printf(/* note */ $x)` is positional only).
func runFmt(args []string) int {
	write := false
	check := false
	var files []string
	for _, a := range args {
		switch a {
		case "-w", "--write":
			write = true
		case "-l", "--check", "--list":
			check = true
		case "-":
			files = append(files, a)
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "jennifer fmt: unknown flag %q (use -w / --write to rewrite in place, or -l / --check to list unformatted files)\n", a)
				return 2
			}
			files = append(files, a)
		}
	}
	if write && check {
		fmt.Fprintln(os.Stderr, "jennifer fmt: -w / --write and -l / --check are mutually exclusive")
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "jennifer fmt: no input file")
		return 2
	}
	// `fmt` formats the files it is named - selecting files (globs, recursion)
	// is the shell's job, so a directory argument is a usage error rather than a
	// silent tree walk. See the globbing note in docs/technical/cli_fmt.md.
	for _, f := range files {
		if f == "-" {
			continue
		}
		if info, err := os.Stat(f); err == nil && info.IsDir() {
			fmt.Fprintf(os.Stderr, "jennifer fmt: %s is a directory; fmt takes files - let the shell select them (e.g. %s/**/*.j)\n", f, f)
			return 2
		}
	}
	if check {
		// Non-mutating gate: list the files whose canonical form differs, one path
		// per line to stdout, and exit non-zero when any does - so `jennifer fmt
		// --check` slots into a CI / publish gate without rewriting the tree.
		status := 0
		anyChanged := false
		for _, f := range files {
			if f == "-" {
				fmt.Fprintln(os.Stderr, "jennifer fmt: -l / --check cannot read stdin")
				status = 2
				continue
			}
			src, formatted, ok := formatFile(f)
			if !ok {
				// A read / lex failure is a broken invocation, not an "unformatted"
				// finding - exit 2, matching `jennifer lint`, so the combined gate
				// reads 0 clean / 1 needs-formatting / 2 broken uniformly.
				status = 2
				continue
			}
			if formatted != src {
				fmt.Println(f)
				anyChanged = true
			}
		}
		if status != 0 {
			return status
		}
		if anyChanged {
			return 1
		}
		return 0
	}
	if !write {
		if len(files) > 1 {
			fmt.Fprintln(os.Stderr, "jennifer fmt: formatting several files to stdout is ambiguous; pass -w to rewrite them in place")
			return 2
		}
		return fmtToStdout(files[0])
	}
	status := 0
	for _, f := range files {
		if f == "-" {
			fmt.Fprintln(os.Stderr, "jennifer fmt: -w cannot rewrite stdin")
			status = 2
			continue
		}
		if code := fmtInPlace(f); code != 0 {
			status = code
		}
	}
	return status
}

// formatFile lexes and formats one source, returning its canonical text. ok is
// false (with a diagnostic already printed) on a read or lex error.
func formatFile(path string) (src, formatted string, ok bool) {
	src, label, absPath, _, ok := loadProgramSource(path)
	if !ok {
		return "", "", false
	}
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return "", "", false
	}
	return src, formatTokens(tokens), true
}

func fmtToStdout(path string) int {
	_, formatted, ok := formatFile(path)
	if !ok {
		return 1
	}
	io.WriteString(os.Stdout, formatted)
	return 0
}

// fmtInPlace rewrites path with its canonical form. An already-canonical file is
// left untouched (no mtime churn); a changed file keeps its existing mode. The
// write is atomic - a temp file in the same directory then a rename - so a crash
// or a full disk mid-write can never truncate or corrupt the original source.
func fmtInPlace(path string) int {
	src, formatted, ok := formatFile(path)
	if !ok {
		return 1
	}
	if formatted == src {
		return 0
	}
	// Self-check: refuse to write if the format changed anything but whitespace.
	// fmt is token-preserving by design (implementation-note 8) and the tests pin
	// it, but -w mutates source in place, so re-verify per file - a future
	// formatter bug can then never silently corrupt a file.
	if !sameCodeTokens(src, formatted) {
		fmt.Fprintf(os.Stderr, "jennifer fmt: internal error - formatting %s would change its tokens; refusing to write (please report this)\n", path)
		return 1
	}
	// Resolve symlinks so `fmt -w link.j` rewrites the target rather than
	// replacing the link itself with a regular file (the atomic rename otherwise
	// clobbers the link).
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
		// Respect a read-only source (as gofmt does): the atomic rename only needs
		// write permission on the directory, so it would otherwise override a
		// `chmod -w` a user set deliberately.
		if mode&0o200 == 0 {
			fmt.Fprintf(os.Stderr, "jennifer fmt: %s is read-only; not rewriting (chmod +w to format it)\n", path)
			return 1
		}
	}
	if err := writeFileAtomic(target, []byte(formatted), mode); err != nil {
		fmt.Fprintf(os.Stderr, "jennifer fmt: writing %s: %v\n", path, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "formatted %s\n", path)
	return 0
}

// sameCodeTokens reports whether a and b have the same non-trivia token stream -
// same type, same processed value (Lexeme), and same source spelling (Raw) - so
// they differ only in whitespace, comments, and blank lines. Comparing Raw as
// well as Lexeme means the check catches not just a semantic change (a wrong
// value or a dropped token) but also a surface-fidelity regression (a digit
// separator or quote style lost). It is the runtime proof that a format
// preserved the program, checked before an in-place write commits; a source that
// fails to lex compares unequal.
func sameCodeTokens(a, b string) bool {
	ta, oka := codeTokens(a)
	tb, okb := codeTokens(b)
	if !oka || !okb || len(ta) != len(tb) {
		return false
	}
	for i := range ta {
		if ta[i].Type != tb[i].Type || ta[i].Lexeme != tb[i].Lexeme || ta[i].Raw != tb[i].Raw {
			return false
		}
	}
	return true
}

// codeTokens lexes src and returns its tokens with all trivia (comments, blank
// lines) dropped; ok is false when src does not lex.
func codeTokens(src string) ([]lexer.Token, bool) {
	toks, err := lexer.TokenizeWithFile(src, "")
	if err != nil {
		return nil, false
	}
	out := make([]lexer.Token, 0, len(toks))
	for _, t := range toks {
		switch t.Type {
		case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK,
			lexer.TOKEN_COMMENT_SHEBANG, lexer.TOKEN_BLANK_LINE:
			continue
		}
		out = append(out, t)
	}
	return out, true
}

// writeFileAtomic writes data to a temp file in path's directory, fsyncs and
// chmods it, then renames it over path. The rename is atomic on POSIX, so a
// reader (or a crash) never observes a partially written source file; the temp
// file is cleaned up on any error before the rename.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".jfmt-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// formatTokens emits canonical Jennifer source from a complete token list
// (including the trailing EOF). Tests call this directly; the CLI wraps
// it with file I/O.
func formatTokens(tokens []lexer.Token) string {
	// File starts at column 1 of line 1 with no output yet, which is
	// effectively "line start" for separator purposes. That makes
	// a leading comment skip the spurious blank line that would
	// otherwise appear before it.
	f := &fmtState{indent: 0, atLineStart: true, tokens: tokens}
	for i, t := range tokens {
		if t.Type == lexer.TOKEN_EOF {
			break
		}
		f.tokenIdx = i
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
	beforePrev  lexer.Token // the real token two back (trivia skipped); a `.` here marks f.prev as a namespaced member name
	hasPrev     bool        // false before the first token
	atLineStart bool        // true right after newline+indent has been written
	// Token kinds that "begin an operand context" - i.e. when the next
	// token is `-`, that `-` is binary, not unary. Maintained on every
	// emit() so unary-vs-binary disambiguation stays a state lookup.
	prevIsOperand    bool
	prevIsUnaryMinus bool // true when prev was a `-` parsed as unary
	// braceStack records, for every open `{`, whether it's a block
	// (statements), a struct-decl body (one field per line), or a map
	// literal (key:value pairs). Matching `}` pops; the kind
	// determines indenting and newline behavior.
	braceStack []byte // 'b' block, 'm' map literal, 's' struct decl, 'M' match body
	// parenDepth is the current `(`/`[` nesting depth. Used to tell the `{` that
	// opens a `match` body / `when` arm (paren depth 0) from a `{` inside a value
	// expression (a map literal at depth > 0).
	parenDepth int
	// pendingMatchBrace flags "the next block `{` at paren depth 0 opens a match
	// body". Set when TOKEN_MATCH is emitted; consumed by that `{`; reset on `;`.
	pendingMatchBrace bool
	// pendingArmBrace flags "the next block `{` at paren depth 0 opens a `when`
	// arm body". Set when TOKEN_WHEN is emitted; consumed by that `{`; reset on `;`.
	pendingArmBrace bool
	// inWhenValues is true while emitting a `when` value list (between `when` and
	// the arm `{`); a wrapped list aligns its continuation values under the first.
	inWhenValues bool
	whenValueCol int
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
	// tokens is the full token slice and tokenIdx the index of the token
	// currently being emitted, so emit() can look ahead over a whole
	// container (to the matching close bracket) to decide whether it wraps.
	tokens   []lexer.Token
	tokenIdx int
	// wrapStack mirrors every open `(` / `[` / `{` so the innermost
	// container's wrap state is known for a top-level `,` (which container
	// does this comma separate?). `wraps` is true when the container is
	// rendered multiline: its body is indented one level and its top-level
	// commas break to newlines. lastWrap caches the flag of the most
	// recently closed container (read when its `}` / `]` is emitted, after
	// the pop).
	wrapStack []wrapFrame
	lastWrap  bool
}

type wrapFrame struct {
	wraps bool
	// forHeader is true for the `(` of a `for (init; cond; step)` header, so the
	// two header `;`s stay inline. Set only on paren frames whose `(` follows a
	// `for` keyword; a stray identifier ending in "for" can never set it.
	forHeader bool
}

const (
	braceBlock     = byte('b')
	braceMap       = byte('m')
	braceStruct    = byte('s')
	braceMatch     = byte('M') // a `match (EXPR) { ... }` body (arms formatted like if/else branches)
	braceStructLit = byte('L') // a struct / enum literal `Name{...}` (tight brace + tight body, like a map)
	braceArm       = byte('a') // a `when` / `else` arm body (a block that may stay inline when short)
)

// maxInlineElements is the top-level element count above which a struct / enum /
// map / list literal wraps to one element per line regardless of width. Width
// (maxLineLength) is the dominant trigger; this is the secondary guard for a
// literal that fits under 100 columns but packs so many members it reads better
// stacked. Tunable; see docs/user-guide/style-guide.md.
const maxInlineElements = 6

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
		f.emitTrivia(t, next)
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
	// Classify `{` before writing it - the separator before a struct-literal
	// brace is tight, so the kind must be known first - and decide whether the
	// container it opens is rendered multiline. Kinds:
	//   - match body / arm body: paren depth 0, flagged by pendingMatch/Arm.
	//   - struct-decl body: `def struct Name {` (pendingStructBrace).
	//   - block: after `)` / `else` / `try` / `spawn` / `repeat`.
	//   - struct / enum literal: a `{` right after an IDENT (`Name{`).
	//   - map literal: anywhere else.
	if t.Type == lexer.TOKEN_LBRACE {
		var kind byte
		switch {
		case f.pendingMatchBrace && f.parenDepth == 0:
			kind = braceMatch
			f.pendingMatchBrace = false
		case f.pendingArmBrace && f.parenDepth == 0:
			kind = braceArm
			f.pendingArmBrace = false
			f.inWhenValues = false
		case f.pendingStructBrace:
			kind = braceStruct
			f.pendingStructBrace = false
		case f.hasPrev && f.prev.Type == lexer.TOKEN_ELSE && f.currentBraceKind() == braceMatch:
			// A `match` `else` is a peer arm (like `when`), not an if-else block,
			// so it may stay inline when it holds a single short statement.
			kind = braceArm
		case f.hasPrev && isBlockOpener(f.prev.Type):
			kind = braceBlock
		case f.hasPrev && f.prev.Type == lexer.TOKEN_IDENT:
			kind = braceStructLit
		default:
			kind = braceMap
		}
		var wraps bool
		switch kind {
		case braceBlock, braceMatch, braceStruct:
			wraps = true
		case braceArm:
			wraps = f.decideArmWrap(f.tokenIdx)
		case braceStructLit:
			wraps = f.decideWrap(f.tokenIdx, true)
		default: // braceMap
			wraps = f.decideWrap(f.tokenIdx, true)
		}
		f.braceStack = append(f.braceStack, kind)
		f.lastBraceKind = kind
		f.pushWrap(wraps)
	}
	// Opening `[` / `(`: a `[` in value position (not hugging an indexable
	// target) is a list literal that may wrap; an index / slice `[` and every
	// `(` never wrap. parenDepth still counts both, for match / arm detection.
	if t.Type == lexer.TOKEN_LBRACKET {
		wraps := false
		if !(f.hasPrev && noSpaceBeforeLBracket(f.prev.Type)) {
			// A list wraps on width only - a long list of short scalars reads
			// fine on one line, so the element-count trigger (for key:value
			// literals) does not apply.
			wraps = f.decideWrap(f.tokenIdx, false)
		}
		f.pushWrap(wraps)
		f.parenDepth++
	}
	if t.Type == lexer.TOKEN_LPAREN {
		// A call-argument `(` (one that hugs its callee) wraps one arg per line
		// when it is long; a control-flow `(` (`if (`, `while (`) and a grouping
		// `(` never wrap.
		wraps := false
		if f.hasPrev && (noSpaceBeforeLParen(f.prev.Type) || f.beforePrev.Type == lexer.TOKEN_DOT) {
			wraps = f.decideCallWrap(f.tokenIdx)
		}
		f.pushWrap(wraps)
		if f.hasPrev && f.prev.Type == lexer.TOKEN_FOR {
			f.wrapStack[len(f.wrapStack)-1].forHeader = true
		}
		f.parenDepth++
	}
	// Closing `}` / `]` / `)`: pop the container and, when it was multiline,
	// dedent before writing so the close bracket lands at the outer indent.
	if t.Type == lexer.TOKEN_RBRACE && len(f.braceStack) > 0 {
		kind := f.braceStack[len(f.braceStack)-1]
		f.braceStack = f.braceStack[:len(f.braceStack)-1]
		f.lastBraceKind = kind
		if f.popWrap() && f.indent > 0 {
			f.indent--
		}
	}
	if t.Type == lexer.TOKEN_RBRACKET {
		if f.popWrap() && f.indent > 0 {
			f.indent--
		}
		if f.parenDepth > 0 {
			f.parenDepth--
		}
	}
	if t.Type == lexer.TOKEN_RPAREN {
		if f.popWrap() && f.indent > 0 {
			f.indent--
		}
		if f.parenDepth > 0 {
			f.parenDepth--
		}
	}
	// Detect whether the `-` we're about to write is unary. A `-` is
	// unary when nothing operand-shaped came before it.
	isUnaryMinus := t.Type == lexer.TOKEN_MINUS && !f.prevIsOperand
	f.writeSeparator(t)
	f.writeToken(t, next)
	// Track "next `{` is a struct-decl / match / arm body" across the
	// intervening tokens; reset defensively on `;`.
	switch t.Type {
	case lexer.TOKEN_STRUCT, lexer.TOKEN_ENUM:
		f.pendingStructBrace = true
	case lexer.TOKEN_MATCH:
		f.pendingMatchBrace = true
	case lexer.TOKEN_WHEN:
		f.pendingArmBrace = true
		f.inWhenValues = true
		f.whenValueCol = f.col + 1 // first value column: one space past `when`
	case lexer.TOKEN_SEMI:
		f.pendingStructBrace = false
		f.pendingMatchBrace = false
		f.pendingArmBrace = false
		f.inWhenValues = false
	}
	// Indent the body of a container that just opened multiline. Driven off
	// the wrap frame so blocks, struct decls, match bodies, wrapped literals,
	// multiline arms, and wrapped call-argument lists indent uniformly (their
	// close dedents above).
	if (t.Type == lexer.TOKEN_LBRACE || t.Type == lexer.TOKEN_LBRACKET || t.Type == lexer.TOKEN_LPAREN) && f.curWraps() {
		f.indent++
	}
	f.beforePrev = f.prev
	f.prev = t
	f.hasPrev = true
	f.atLineStart = false
	f.prevIsOperand = isOperandToken(t)
	f.prevIsUnaryMinus = isUnaryMinus
}

// nextLineIndent is the indent level the line *after* this trivia should get.
// It is the current indent, minus one when the upcoming token is a `}` that
// closes a block / struct / match body: that `}` dedents in emit() *after* the
// trivia runs, so pre-emitting the full indent here would leave a closing brace
// that follows a trailing comment or a blank line over-indented. Only the token
// immediately after the trivia needs this; anything further is handled by its
// own separator (which runs after the dedent).
func (f *fmtState) nextLineIndent(next lexer.Token) int {
	ind := f.indent
	if (next.Type == lexer.TOKEN_RBRACE || next.Type == lexer.TOKEN_RBRACKET) && f.curWraps() {
		ind--
	}
	if ind < 0 {
		ind = 0
	}
	return ind
}

// emitTrivia handles comments and blank lines. It writes only output -
// it does NOT touch prev/lastBraceKind/prevIsOperand/prevIsUnaryMinus
// so the surrounding state machine continues to see the most recent
// regular token. atLineStart IS updated because the separator logic
// for the next regular token reads it to decide whether to skip a
// leading separator. `next` is the following regular token, used to size the
// indent of the line this trivia opens (see nextLineIndent).
func (f *fmtState) emitTrivia(t, next lexer.Token) {
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
		// A `# pragma-jennifer-*:` directive is canonicalized to its single-spaced
		// form so odd input spacing (`#pragma...`, wide gaps) renders uniformly;
		// every other comment is preserved verbatim.
		lex := t.Lexeme
		if canon, ok := reqcheck.CanonicalLine(lex); ok {
			lex = canon
		}
		f.writeString(lex)
		f.writeByte('\n')
		// Next real token starts a fresh line, indented for what it is (a
		// following `}` dedents, so it lands one level out).
		for i := 0; i < f.nextLineIndent(next); i++ {
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
			// End the comment line and indent for the next token (a following
			// `}` lands one level out).
			f.writeByte('\n')
			for i := 0; i < f.nextLineIndent(next); i++ {
				f.writeString(fmtIndent)
			}
			f.atLineStart = true
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
		for i := 0; i < f.nextLineIndent(next); i++ {
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
			lexer.TOKEN_DOT, lexer.TOKEN_DOTDOT, lexer.TOKEN_RBRACKET, lexer.TOKEN_COLON:
			return
		}
		f.writeByte(' ')
		return
	}
	// Tight brace of a struct / enum literal: `Name{` (no space, so the brace
	// reads as bound to the type name rather than a block opener).
	if t.Type == lexer.TOKEN_LBRACE && f.currentBraceKind() == braceStructLit {
		return
	}
	// Column-based reflow at binary joiners. When a line has already
	// grown past maxLineLength, or when the source itself put a break
	// at this point, wrap AFTER the joiner so the operator hangs at
	// end-of-line (matches the string-concat idiom in the wild). One
	// extra indent level per hanging continuation line, matching what
	// the style guide recommends.
	if isBinaryJoiner(f.prev.Type) && !f.prevIsUnaryMinus {
		// Break when the source broke here, when the line is already over the
		// limit, or when appending the next operand would push it over - the
		// last case fills the line before wrapping and is what stops a long
		// concat (whose overflowing operand is a single token) from printing
		// past the limit and tripping the line-length lint.
		opW, followW := f.nextOperandWidth(f.tokenIdx)
		// Reserve the leading space before the operand and, when another joiner
		// follows it, the ` +` / ` and` / ` or` that will hang at the end of this
		// line after the operand - so the hanging operator does not itself spill
		// past the limit.
		if t.Line > f.prev.Line || f.col > maxLineLength ||
			f.col+1+opW+followW > maxLineLength {
			f.continuationLine()
			return
		}
	}
	// Statement terminator: ";" closes a statement; the next token starts
	// a new line at the current indent. Exceptions: the two `;`s inside
	// `for (...; ...; ...)` stay inline, and an inline `when` / `else` arm
	// keeps its single statement's `;` on the same line as the closing `}`.
	if f.prev.Type == lexer.TOKEN_SEMI {
		if f.insideForHeader() {
			f.writeByte(' ')
			return
		}
		if t.Type == lexer.TOKEN_RBRACE && f.lastBraceKind == braceArm && !f.lastWrap {
			f.writeByte(' ')
			return
		}
		f.newline()
		return
	}
	// `when` value list `,`: keep on one line, but if the source wrapped it or
	// it grew too long, break after the comma and align the next value under
	// the first (like a wrapped call-arg list).
	if f.prev.Type == lexer.TOKEN_COMMA && f.inWhenValues {
		if t.Line > f.prev.Line || f.col > maxLineLength {
			f.newlineToCol(f.whenValueCol)
			return
		}
		f.writeByte(' ')
		return
	}
	// Top-level `,` of a multiline container - a struct decl, or a wrapped
	// struct / enum / map / list literal - puts each element on its own line.
	// prevContainerWraps looks past the current token's own frame when that
	// token is itself an opening bracket (a list whose element is a map).
	if f.prev.Type == lexer.TOKEN_COMMA && f.prevContainerWraps(t) {
		f.newline()
		return
	}
	// After a `}` that closed a block / struct-decl / match / multiline arm:
	// the next token starts a new line, except the cuddled tail forms
	// (`} else`, `} elseif`, `} catch`, `} until`, `};`). A `match`'s `else`
	// is a peer arm and does start its own line.
	if f.prev.Type == lexer.TOKEN_RBRACE {
		// A `}` hugs a following `;` / `,` / `)` - a statement terminator or a
		// separator / close after a block expression used as a value, e.g. a
		// `spawn { ... }` element in a list (`},`) or a call argument (`})`). A
		// following `]` is deliberately NOT here: a `]` closing a multiline list
		// lands on its own line, handled by the list-close rule below.
		switch t.Type {
		case lexer.TOKEN_SEMI, lexer.TOKEN_COMMA, lexer.TOKEN_RPAREN:
			return
		}
		k := f.prevBraceKind()
		if k == braceBlock || k == braceStruct || k == braceMatch || k == braceArm {
			switch t.Type {
			case lexer.TOKEN_ELSE, lexer.TOKEN_ELSEIF,
				lexer.TOKEN_CATCH, lexer.TOKEN_UNTIL:
				if t.Type == lexer.TOKEN_ELSE && f.currentBraceKind() == braceMatch {
					f.newline()
					return
				}
				f.writeByte(' ')
				return
			}
			f.newline()
			return
		}
		// A map / struct-literal `}` falls through to the tight rules (the
		// following `,` / `;` / `)` hugs it).
	}
	// About-to-emit a container close. Struct decls and multiline map /
	// struct-literal bodies put `}` on its own line; a multiline list puts `]`
	// on its own line. Inline struct-lit / map / list / index closes stay tight.
	if t.Type == lexer.TOKEN_RBRACE {
		switch f.lastBraceKind {
		case braceStruct:
			f.newline()
			return
		case braceStructLit, braceMap:
			if f.lastWrap {
				f.newline()
			}
			return
		}
	}
	if t.Type == lexer.TOKEN_RBRACKET {
		if f.lastWrap {
			f.newline()
		}
		return
	}
	// Opening a container body. Block / struct-decl / match `{` and a multiline
	// map / struct-lit newline; an inline arm keeps its statement on the line;
	// an inline struct-lit / map literal keeps its body tight (no padding).
	if f.prev.Type == lexer.TOKEN_LBRACE {
		k := f.prevBraceKind()
		if k == braceBlock || k == braceStruct || k == braceMatch {
			f.newline()
			return
		}
		if k == braceArm {
			if f.prevContainerWraps(t) {
				f.newline()
			} else {
				f.writeByte(' ')
			}
			return
		}
		if f.prevContainerWraps(t) {
			f.newline()
			return
		}
		// Inline map / struct-literal: no padding inside.
		return
	}
	// Opening a list literal `[`: a multiline list breaks after `[`; an inline
	// list (and every index / slice `[`) keeps its contents on the line.
	if f.prev.Type == lexer.TOKEN_LBRACKET {
		// The `t != ]` guard keeps an empty `[]` tight: its `[` frame is already
		// popped by the time its `]` reaches here, so prevContainerWraps would
		// otherwise read the enclosing (possibly wrapping) container.
		if t.Type != lexer.TOKEN_RBRACKET && f.prevContainerWraps(t) {
			f.newline()
			return
		}
		return
	}
	// Opening `(`: a wrapped call-argument list breaks after `(` (first arg on
	// its own indented line); an inline call / control-flow / grouping `(` keeps
	// its contents on the line. The `t != )` guard keeps an empty `()` tight
	// (same already-popped-frame reason as `[]` above).
	if f.prev.Type == lexer.TOKEN_LPAREN {
		if t.Type != lexer.TOKEN_RPAREN && f.prevContainerWraps(t) {
			f.newline()
			return
		}
		return
	}
	// No space between a callee and its opening `(`. Two cases: a bare call /
	// cast / `len` (`printf(`, `int(`, `len(`), and - the general one - any
	// `.`-qualified member name (`json.map(`, `strings.repeat(`), where the
	// member may be a keyword the lexer tokenised as such, so the whitelist
	// can't see it; the preceding `.` (two real tokens back) is the tell. The
	// leading keyword forms (`if (`, `while (`, `for (`, `match (`) get a space:
	// their keyword isn't in the whitelist and isn't dot-qualified.
	if t.Type == lexer.TOKEN_LPAREN &&
		(noSpaceBeforeLParen(f.prev.Type) || f.beforePrev.Type == lexer.TOKEN_DOT) {
		return
	}
	// Index expressions hug their target: `$xs[0]`, `foo()[1]`,
	// `bar()[0][1]`. Any token that can stand at the end of an indexable
	// expression (IDENT, VARREF, RPAREN, RBRACKET, RBRACE-from-map) gets
	// tight binding to a following `[`.
	if t.Type == lexer.TOKEN_LBRACKET && noSpaceBeforeLBracket(f.prev.Type) {
		return
	}
	// Tight punctuation: nothing between `(`/`[`/map-`{` and the next
	// token, and nothing between the previous token and the matching
	// close, comma, semi, dot, or `:` (map-literal key/value separator).
	switch t.Type {
	case lexer.TOKEN_RPAREN, lexer.TOKEN_COMMA, lexer.TOKEN_SEMI,
		lexer.TOKEN_DOT, lexer.TOKEN_DOTDOT, lexer.TOKEN_RBRACKET, lexer.TOKEN_COLON:
		return
	}
	if f.prev.Type == lexer.TOKEN_LPAREN || f.prev.Type == lexer.TOKEN_DOT ||
		f.prev.Type == lexer.TOKEN_DOTDOT || f.prev.Type == lexer.TOKEN_LBRACKET {
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

// nextOperandWidth returns the rendered width of the operand beginning at
// tokens[i] (the token right after a binary joiner), up to the next same-depth
// joiner or boundary (`;`, `,`, or a close bracket that ends the enclosing
// group), and followW: the width of ` <joiner>` when another binary joiner
// immediately follows the operand (0 at a boundary). Used to decide whether
// appending this operand - plus the operator that would hang after it - fits.
func (f *fmtState) nextOperandWidth(i int) (int, int) {
	depth := 0
	width := 0
	for j := i; j < len(f.tokens); j++ {
		t := f.tokens[j]
		switch t.Type {
		case lexer.TOKEN_LPAREN, lexer.TOKEN_LBRACKET, lexer.TOKEN_LBRACE:
			depth++
		case lexer.TOKEN_RPAREN, lexer.TOKEN_RBRACKET, lexer.TOKEN_RBRACE:
			if depth == 0 {
				return width, 0
			}
			depth--
		case lexer.TOKEN_SEMI, lexer.TOKEN_COMMA:
			if depth == 0 {
				return width, 0
			}
		case lexer.TOKEN_PLUS, lexer.TOKEN_AND, lexer.TOKEN_OR:
			if depth == 0 && j > i {
				return width, 1 + tokenWidth(t) // space + the hanging joiner
			}
		}
		if j > i {
			prevUnary := f.tokens[j-1].Type == lexer.TOKEN_MINUS &&
				(j-2 < 0 || !isOperandToken(f.tokens[j-2]))
			width += inlineSepWidth(f.tokens[j-1], t, prevUnary)
		}
		width += tokenWidth(t)
	}
	return width, 0
}

// prevContainerWraps reports whether the container we are currently *inside*
// (the one whose open bracket is f.prev) is being rendered multiline. When the
// current token cur is itself an opening bracket its own frame is already on the
// wrap stack, so the enclosing container is one below the top.
func (f *fmtState) prevContainerWraps(cur lexer.Token) bool {
	n := len(f.wrapStack)
	switch cur.Type {
	case lexer.TOKEN_LBRACE, lexer.TOKEN_LBRACKET, lexer.TOKEN_LPAREN:
		if n >= 2 {
			return f.wrapStack[n-2].wraps
		}
		return false
	}
	if n >= 1 {
		return f.wrapStack[n-1].wraps
	}
	return false
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
	case lexer.TOKEN_STRING, lexer.TOKEN_STRING_INTERP:
		// Emit the exact source spelling (quotes, escapes, embedded newlines, and
		// any `{expr}` interpolation slots verbatim) so a multi-line, specifically-
		// escaped, or interpolated string survives a format byte-for-byte; only a
		// token with no captured Raw (a hand-built AST in a test) falls back to
		// re-quoting the processed value.
		if t.Raw != "" {
			f.writeString(t.Raw)
		} else {
			f.writeString(quoteJenniferString(t.Lexeme))
		}
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

// newlineToCol ends the current line and pads the next with spaces to `col`,
// used to align a wrapped `when` value list under its first value.
func (f *fmtState) newlineToCol(col int) {
	f.writeByte('\n')
	for i := 0; i < col; i++ {
		f.writeByte(' ')
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
// semicolons, so the formatter writes a space (not a newline) after them. A
// header `;` sits directly inside the `for`'s `(` (any brackets in the init /
// cond expression are already balanced), so the innermost open container is that
// paren - an O(1) flag read on the wrap stack, set only when a `(` follows a
// `for` keyword (no output-string scan, no `waitfor`-style false match).
func (f *fmtState) insideForHeader() bool {
	return len(f.wrapStack) > 0 && f.wrapStack[len(f.wrapStack)-1].forHeader
}

// canonicalLexeme returns the source-form spelling of a token. For a numeric
// literal Raw carries the exact source (digit separators, base prefix), which
// the processed Lexeme has lost, so it wins; for keywords and punctuation the
// constant lexeme is already canonical.
func canonicalLexeme(t lexer.Token) string {
	if t.Raw != "" {
		return t.Raw
	}
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
	case lexer.TOKEN_INT, lexer.TOKEN_FLOAT, lexer.TOKEN_STRING, lexer.TOKEN_STRING_INTERP,
		lexer.TOKEN_VARREF, lexer.TOKEN_TRUE, lexer.TOKEN_FALSE,
		lexer.TOKEN_NULL, lexer.TOKEN_IDENT,
		lexer.TOKEN_RPAREN, lexer.TOKEN_RBRACE:
		return true
	}
	return false
}

// noSpaceBeforeLParen lists the token types that hug a following `(`:
// function calls, type-conversion casts, the `len` built-in (a keyword-shaped
// primary expression that behaves like a call), and a call through a function
// value - `$f(x)` (VARREF), `getFn()(x)` (RPAREN), `$fns[0](x)` (RBRACKET).
// Those three only precede a `(` as a first-class-function call (any such
// juxtaposition was a parse error before), so hugging them changes no existing
// program's formatting.
func noSpaceBeforeLParen(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TOKEN_IDENT,
		lexer.TOKEN_INT_TYPE, lexer.TOKEN_FLOAT_TYPE,
		lexer.TOKEN_STRING_TYPE, lexer.TOKEN_BOOL_TYPE,
		lexer.TOKEN_LEN,
		lexer.TOKEN_VARREF, lexer.TOKEN_RPAREN, lexer.TOKEN_RBRACKET:
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

// --- container wrap lookahead --------------------------------------------

func (f *fmtState) pushWrap(w bool) { f.wrapStack = append(f.wrapStack, wrapFrame{wraps: w}) }

// popWrap removes the innermost container frame, caching its wrap flag in
// lastWrap (read by the closing `}` / `]` separator, which runs after the pop).
func (f *fmtState) popWrap() bool {
	if len(f.wrapStack) == 0 {
		return false
	}
	w := f.wrapStack[len(f.wrapStack)-1].wraps
	f.wrapStack = f.wrapStack[:len(f.wrapStack)-1]
	f.lastWrap = w
	return w
}

// curWraps reports whether the innermost open container is being rendered
// multiline. Used to decide a top-level `,`'s separator and the token right
// after an opening bracket.
func (f *fmtState) curWraps() bool {
	if len(f.wrapStack) == 0 {
		return false
	}
	return f.wrapStack[len(f.wrapStack)-1].wraps
}

// spanEnd returns the index of the close bracket matching the open bracket at
// tokens[open] (`{`->`}` or `[`->`]`), or -1 if unbalanced. Nesting of the
// other bracket type is irrelevant: valid source is balanced, so counting one
// type finds the match.
func spanEnd(tokens []lexer.Token, open int) int {
	var openT, closeT lexer.TokenType
	switch tokens[open].Type {
	case lexer.TOKEN_LBRACE:
		openT, closeT = lexer.TOKEN_LBRACE, lexer.TOKEN_RBRACE
	case lexer.TOKEN_LBRACKET:
		openT, closeT = lexer.TOKEN_LBRACKET, lexer.TOKEN_RBRACKET
	case lexer.TOKEN_LPAREN:
		openT, closeT = lexer.TOKEN_LPAREN, lexer.TOKEN_RPAREN
	default:
		return -1
	}
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Type {
		case openT:
			depth++
		case closeT:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// topLevelElements counts the comma-separated elements directly inside the
// container tokens[open..close] (commas at the container's own depth, +1), or 0
// for an empty container.
func topLevelElements(tokens []lexer.Token, open, close int) int {
	if close <= open+1 {
		return 0
	}
	depth := 0
	commas := 0
	for i := open; i < close; i++ {
		switch tokens[i].Type {
		case lexer.TOKEN_LBRACE, lexer.TOKEN_LBRACKET, lexer.TOKEN_LPAREN:
			depth++
		case lexer.TOKEN_RBRACE, lexer.TOKEN_RBRACKET, lexer.TOKEN_RPAREN:
			depth--
		case lexer.TOKEN_COMMA:
			if depth == 1 {
				commas++
			}
		}
	}
	return commas + 1
}

// tokenWidth returns the rune width of a token's canonical spelling.
func tokenWidth(t lexer.Token) int {
	switch t.Type {
	case lexer.TOKEN_VARREF:
		return 1 + len([]rune(t.Lexeme))
	case lexer.TOKEN_STRING, lexer.TOKEN_STRING_INTERP:
		if t.Raw != "" {
			return len([]rune(t.Raw))
		}
		return len([]rune(quoteJenniferString(t.Lexeme)))
	default:
		return len([]rune(canonicalLexeme(t)))
	}
}

// inlineContainerWidth returns the rune width of tokens[open..close] rendered on
// one line (open is a `{` or `[`). Used to decide whether a container fits under
// the column limit; a small estimation error only nudges a borderline
// container's wrap decision, never correctness.
func inlineContainerWidth(tokens []lexer.Token, open, close int) int {
	width := 0
	for i := open; i <= close; i++ {
		t := tokens[i]
		if i > open {
			prevUnary := tokens[i-1].Type == lexer.TOKEN_MINUS &&
				(i-2 < open || !isOperandToken(tokens[i-2]))
			width += inlineSepWidth(tokens[i-1], t, prevUnary)
		}
		width += tokenWidth(t)
	}
	return width
}

// inlineSepWidth returns the separator width (0 or 1) the inline formatter would
// put between prev and cur, mirroring writeSeparator's non-newline branches.
// prevUnary is true when prev is a unary minus.
func inlineSepWidth(prev, cur lexer.Token, prevUnary bool) int {
	// Tight before a struct / enum literal brace: `Name{`.
	if cur.Type == lexer.TOKEN_LBRACE && prev.Type == lexer.TOKEN_IDENT {
		return 0
	}
	// Inline literal bodies are tight (no padding inside `{` / `}`).
	if prev.Type == lexer.TOKEN_LBRACE || cur.Type == lexer.TOKEN_RBRACE {
		return 0
	}
	switch cur.Type {
	case lexer.TOKEN_RPAREN, lexer.TOKEN_COMMA, lexer.TOKEN_SEMI,
		lexer.TOKEN_DOT, lexer.TOKEN_DOTDOT, lexer.TOKEN_RBRACKET, lexer.TOKEN_COLON:
		return 0
	}
	switch prev.Type {
	case lexer.TOKEN_LPAREN, lexer.TOKEN_DOT, lexer.TOKEN_DOTDOT, lexer.TOKEN_LBRACKET:
		return 0
	}
	if cur.Type == lexer.TOKEN_LPAREN && noSpaceBeforeLParen(prev.Type) {
		return 0
	}
	if cur.Type == lexer.TOKEN_LBRACKET && noSpaceBeforeLBracket(prev.Type) {
		return 0
	}
	if prevUnary {
		return 0
	}
	return 1
}

// decideWrap reports whether the literal container opening at tokens[open]
// (a `{` or `[`) should be rendered multiline: it wraps when embedded trivia
// forces it, when it has more than maxInlineElements top-level elements, or when
// its inline rendering would push the line past maxLineLength. f.col is the
// column the open bracket lands at, so the check is layout-aware.
// trailingWidth is the number of columns the emitted line carries immediately
// after a container's close bracket, so the fit test measures the whole line and
// not just the bracketed span. A hugging `;` / `,` / `)` / `]` is 1 column; a
// block-opening ` {` is 2 (the space plus the brace) - that is the `) {` of a
// `func` signature, which ends in ` {` rather than `);`, so without this its
// signature would be allowed two columns over the limit.
func trailingWidth(tokens []lexer.Token, end int) int {
	if end+1 >= len(tokens) {
		return 0
	}
	switch tokens[end+1].Type {
	case lexer.TOKEN_SEMI, lexer.TOKEN_COMMA, lexer.TOKEN_RPAREN, lexer.TOKEN_RBRACKET:
		return 1
	case lexer.TOKEN_LBRACE:
		return 2
	}
	return 0
}

func (f *fmtState) decideWrap(open int, applyCount bool) bool {
	end := spanEnd(f.tokens, open)
	if end < 0 || end == open+1 {
		return false
	}
	for i := open + 1; i < end; i++ {
		switch f.tokens[i].Type {
		case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK,
			lexer.TOKEN_COMMENT_SHEBANG, lexer.TOKEN_BLANK_LINE:
			// Embedded trivia can't sit on a joined line.
			return true
		case lexer.TOKEN_SPAWN:
			// A `spawn { ... }` block element is inherently multiline (the block
			// body always expands), so the container that holds it must wrap -
			// its inline form does not exist however narrow it looks.
			return true
		}
	}
	if applyCount && topLevelElements(f.tokens, open, end) > maxInlineElements {
		return true
	}
	w := inlineContainerWidth(f.tokens, open, end)
	trail := trailingWidth(f.tokens, end)
	return f.effectiveStartCol()+w+trail > maxLineLength
}

// decideCallWrap reports whether a call-argument list opening at tokens[open]
// (a call `(`) should wrap one argument per line: only with **two or more**
// arguments (a lone argument has no better shape, and a lone spawn/block arg
// expands on its own), and then when embedded trivia / a spawn block forces it
// or its inline rendering would pass the column limit. The close `)` hugs the
// last argument (handled by the RPAREN separator rules), so a wrapped call reads
// `foo(\n    a,\n    b)`.
func (f *fmtState) decideCallWrap(open int) bool {
	end := spanEnd(f.tokens, open)
	if end < 0 || end == open+1 {
		return false
	}
	if topLevelElements(f.tokens, open, end) < 2 {
		return false
	}
	for i := open + 1; i < end; i++ {
		switch f.tokens[i].Type {
		case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK,
			lexer.TOKEN_COMMENT_SHEBANG, lexer.TOKEN_BLANK_LINE, lexer.TOKEN_SPAWN:
			return true
		}
	}
	w := inlineContainerWidth(f.tokens, open, end)
	trail := trailingWidth(f.tokens, end)
	return f.effectiveStartCol()+w+trail > maxLineLength
}

// effectiveStartCol is the column the token about to be emitted will actually
// land at. It is f.col mid-line, but when the separator before it will be a
// newline (this container is an element of an enclosing multiline container, so
// it starts fresh at the body indent) the running column is irrelevant and the
// indent column is what matters.
func (f *fmtState) effectiveStartCol() int {
	if f.startsFreshLine() {
		return f.indent * len(fmtIndent)
	}
	// f.col is the column *before* the separator that will precede this bracket
	// is written, so add that separator's width to get the bracket's true
	// landing column (else a mid-line literal is under-measured by the space).
	return f.col + f.leadingSepWidth(f.tokenIdx)
}

// leadingSepWidth predicts the width (0 or 1) of the separator writeSeparator
// will put immediately before the bracket at tokens[open]: 0 when it hugs the
// previous token (a struct-literal `Name{`, or a `(` / `[` / `.` / index on the
// left), else a single space.
func (f *fmtState) leadingSepWidth(open int) int {
	if !f.hasPrev {
		return 0
	}
	cur := f.tokens[open]
	if cur.Type == lexer.TOKEN_LBRACE && f.prev.Type == lexer.TOKEN_IDENT {
		return 0
	}
	// A call `(` hugs its callee (`foo(`, `ns.name(`), so no separator precedes it.
	if cur.Type == lexer.TOKEN_LPAREN &&
		(noSpaceBeforeLParen(f.prev.Type) || f.beforePrev.Type == lexer.TOKEN_DOT) {
		return 0
	}
	switch f.prev.Type {
	case lexer.TOKEN_LPAREN, lexer.TOKEN_DOT, lexer.TOKEN_DOTDOT, lexer.TOKEN_LBRACKET:
		return 0
	}
	if cur.Type == lexer.TOKEN_LBRACKET && noSpaceBeforeLBracket(f.prev.Type) {
		return 0
	}
	return 1
}

// startsFreshLine predicts whether the token about to be emitted is preceded by
// a newline: the first / next element of an enclosing multiline container, or a
// fresh statement after `;` / a block `}`.
func (f *fmtState) startsFreshLine() bool {
	if !f.hasPrev {
		return false
	}
	switch f.prev.Type {
	case lexer.TOKEN_COMMA, lexer.TOKEN_LBRACKET, lexer.TOKEN_LBRACE:
		return f.curWraps()
	case lexer.TOKEN_SEMI:
		return !f.insideForHeader()
	case lexer.TOKEN_RBRACE:
		return true
	}
	return false
}

// decideArmWrap reports whether a `when` / `else` arm body opening at
// tokens[open] should expand across lines. A single-statement arm that fits the
// column limit stays inline (`when V { stmt; }`); anything else (multiple
// statements, an embedded comment, or an overflowing line) expands like a block.
func (f *fmtState) decideArmWrap(open int) bool {
	end := spanEnd(f.tokens, open)
	if end < 0 {
		return true
	}
	depth := 0
	semis := 0
	for i := open; i < end; i++ {
		switch f.tokens[i].Type {
		case lexer.TOKEN_LBRACE, lexer.TOKEN_LBRACKET, lexer.TOKEN_LPAREN:
			depth++
		case lexer.TOKEN_RBRACE, lexer.TOKEN_RBRACKET, lexer.TOKEN_RPAREN:
			depth--
		case lexer.TOKEN_SEMI:
			if depth == 1 {
				semis++
			}
		case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK,
			lexer.TOKEN_COMMENT_SHEBANG, lexer.TOKEN_BLANK_LINE:
			return true
		case lexer.TOKEN_SPAWN:
			// A `spawn { ... }` in the arm body forces the arm multiline.
			return true
		}
	}
	if semis != 1 {
		return true
	}
	w := inlineContainerWidth(f.tokens, open, end)
	return f.effectiveStartCol()+w > maxLineLength
}
