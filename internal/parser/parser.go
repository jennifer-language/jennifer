// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// ParseError carries source position so the caller can produce useful messages.
type ParseError struct {
	Msg  string
	File string
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("parse error at %s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("parse error at %d:%d: %s", e.Line, e.Col, e.Msg)
}

// Position implements the positioned-error interface used by the CLI.
func (e *ParseError) Position() (file string, line, col int) {
	return e.File, e.Line, e.Col
}

// Parse tokenizes the source and returns a *Program AST. Does NOT
// run the scope-analysis pass; call Resolve(prog) separately
// when preparing a program for execution. Splitting the two lets
// grammar-level tests focus on parse trees without wiring up scope
// context for every fragment.
func Parse(source string) (*Program, error) {
	toks, err := lexer.Tokenize(source)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: stripTrivia(toks)}
	return p.parseProgram()
}

// ParseTokens parses an already-lexed token stream. Trivia
// tokens (comments, blank lines) are stripped before parsing - they
// pass through the formatter on the raw lexer stream and don't reach
// here. Same scope-analysis contract as Parse: run Resolve(prog)
// yourself before execution.
func ParseTokens(toks []lexer.Token) (*Program, error) {
	stripped := stripTrivia(toks)
	// The parser indexes past the current token; a stream that is empty or does
	// not end with EOF would panic, so reject it as a parse error instead.
	if len(stripped) == 0 || stripped[len(stripped)-1].Type != lexer.TOKEN_EOF {
		return nil, &ParseError{Msg: "token stream must be non-empty and end with EOF"}
	}
	p := &parser{tokens: stripped}
	return p.parseProgram()
}

// stripTrivia drops comment and blank-line tokens. The parser doesn't
// model comments; the formatter consumes them from the raw lexer
// stream instead.
func stripTrivia(toks []lexer.Token) []lexer.Token {
	// Fresh slice: `toks[:0]` would compact the caller's backing array in place.
	out := make([]lexer.Token, 0, len(toks))
	for _, t := range toks {
		switch t.Type {
		case lexer.TOKEN_COMMENT_LINE,
			lexer.TOKEN_COMMENT_BLOCK,
			lexer.TOKEN_COMMENT_SHEBANG,
			lexer.TOKEN_BLANK_LINE:
			continue
		}
		out = append(out, t)
	}
	return out
}

type parser struct {
	tokens []lexer.Token
	pos    int
	// seed, when non-nil, is a pre-parsed primary that parsePrimaryAtom returns
	// instead of consuming tokens. It lets tryParseIndexAssign hand an
	// already-parsed lvalue chain back into the expression ladder (a non-`=`
	// statement) without re-parsing the chain from scratch.
	seed Expr
	// exprDepth tracks structural expression nesting so parseExpr can reject
	// input that would overflow the Go stack (the interpreter has no recover();
	// a stack overflow is a fatal, uncatchable crash). See parseExpr.
	exprDepth int
}

func (p *parser) peek() lexer.Token       { return p.tokens[p.pos] }
func (p *parser) peekN(n int) lexer.Token { return p.tokens[p.pos+n] }
func (p *parser) advance() lexer.Token {
	t := p.tokens[p.pos]
	if t.Type != lexer.TOKEN_EOF {
		p.pos++
	}
	return t
}

func (p *parser) check(tt lexer.TokenType) bool { return p.peek().Type == tt }

func (p *parser) match(tt lexer.TokenType) (lexer.Token, bool) {
	if p.check(tt) {
		return p.advance(), true
	}
	return lexer.Token{}, false
}

func (p *parser) expect(tt lexer.TokenType, ctx string) (lexer.Token, error) {
	t := p.peek()
	if t.Type != tt {
		return t, &ParseError{
			Msg:  fmt.Sprintf("expected %s %s, got %s (%q)", tt, ctx, t.Type, t.Lexeme),
			File: t.File, Line: t.Line, Col: t.Col,
		}
	}
	return p.advance(), nil
}

// ---- Grammar ----
//
//   program     := { importStmt | methodDef } EOF
//   importStmt  := "import" IDENT ";"
//   methodDef   := "def" IDENT "(" ")" block
//   block       := "{" { statement } "}"
//   statement   := defineStmt | exprStmt
//   defineStmt  := "define" VARREF "as" type "init" expr ";"
//   exprStmt    := expr ";"
//   type        := "int" | "string"
//   expr        := addExpr
//   addExpr     := mulExpr { ("+"|"-") mulExpr }
//   mulExpr     := unary { ("*"|"/"|"%") unary }
//   unary       := primary           (no prefix operators at this level)
//   primary     := INT | STRING | VARREF | call | "(" expr ")"
//   call        := IDENT "(" [ expr { "," expr } ] ")"

func (p *parser) parseProgram() (*Program, error) {
	prog := &Program{pos: pos{Line: 1, Col: 1}}
	for {
		t := p.peek()
		switch t.Type {
		case lexer.TOKEN_EOF:
			return prog, nil
		case lexer.TOKEN_USE:
			// Library import. After preprocessing, `import "file.j";`
			// statements are gone (spliced in place), so only `use NAME;`
			// reaches the parser.
			imp, err := p.parseImport()
			if err != nil {
				return nil, err
			}
			prog.Imports = append(prog.Imports, imp)
		case lexer.TOKEN_IMPORT:
			// `import "path.j" [as NAME];` - a module import (the preprocessor
			// passes it through; the interpreter's loader resolves and runs it).
			m, err := p.parseModuleImport()
			if err != nil {
				return nil, err
			}
			prog.ModuleImports = append(prog.ModuleImports, m)
		case lexer.TOKEN_EXPORT:
			// `export` publishes a module's `def const` / `def struct` /
			// `func`. It is only valid at the top level in front of one of
			// those three forms; anything else is a parse error.
			if err := p.parseExported(prog); err != nil {
				return nil, err
			}
		case lexer.TOKEN_FUNC:
			// `func NAME() { ... }` - methods are hoisted so they can be
			// called regardless of textual order.
			m, err := p.parseMethodDef()
			if err != nil {
				return nil, err
			}
			prog.Methods = append(prog.Methods, m)
		case lexer.TOKEN_DEFINE:
			// `def struct Name { ... };` is top-level only and hoisted
			// alongside methods. Plain `def NAME as T ...;` falls through
			// to parseStatement. Two-token lookahead distinguishes them.
			if p.peekN(1).Type == lexer.TOKEN_STRUCT {
				sd, err := p.parseStructDef()
				if err != nil {
					return nil, err
				}
				prog.Structs = append(prog.Structs, sd)
				continue
			}
			fallthrough
		default:
			// Any other top-level statement: def / assign / if / while / for / expr.
			st, err := p.parseStatement()
			if err != nil {
				return nil, err
			}
			prog.TopLevel = append(prog.TopLevel, st)
		}
	}
}

// parseImport parses a `use NAME;` or `use NAME as ALIAS;` library import.
// (File imports are handled by the preprocessor and never reach the parser.)
//
// The optional `as ALIAS` clause renames the namespace at the use site: after
// `use bio as b;`, only `b.translate(...)` resolves; `bio.` errors with a
// "did you mean `b`?" hint. The alias follows the same identifier rule as
// any other Jennifer name (letters only, no `_`).
// parseModuleImport parses `import "path.j" [as NAME];`. The path is a
// quoted, `.j`-suffixed string; the optional `as NAME` names the module's
// namespace prefix (bare form derives it from the file stem in the
// interpreter). The preprocessor has already rejected the unquoted mistake.
func (p *parser) parseModuleImport() (*ModuleImportStmt, error) {
	imp, _ := p.match(lexer.TOKEN_IMPORT)
	path, err := p.expect(lexer.TOKEN_STRING, "a quoted module path after `import`")
	if err != nil {
		return nil, err
	}
	// A deck reference (`@scope/package/`) need not end in `.j`: the trailing
	// slash expands to the package-named entry file at resolve time. Every other
	// module path is a file and must end in `.j`.
	if !strings.HasSuffix(path.Lexeme, ".j") && !strings.HasPrefix(path.Lexeme, "@") {
		return nil, &ParseError{Msg: fmt.Sprintf("module path %q must end in `.j`", path.Lexeme), File: path.File, Line: path.Line, Col: path.Col}
	}
	m := &ModuleImportStmt{pos: pos{File: imp.File, Line: imp.Line, Col: imp.Col}, Path: path.Lexeme}
	if _, ok := p.match(lexer.TOKEN_AS); ok {
		alias, err := p.expect(lexer.TOKEN_IDENT, "after `as` in `import`")
		if err != nil {
			return nil, err
		}
		if containsUnderscore(alias.Lexeme) {
			return nil, &ParseError{Msg: fmt.Sprintf("module alias %q may not contain `_`", alias.Lexeme), File: alias.File, Line: alias.Line, Col: alias.Col}
		}
		m.AsName = alias.Lexeme
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "after `import` statement"); err != nil {
		return nil, err
	}
	return m, nil
}

func (p *parser) parseImport() (*ImportStmt, error) {
	use, _ := p.match(lexer.TOKEN_USE)
	// `use task;` activates the task library. `task` is a type
	// keyword so it doesn't land in the IDENT bucket; accept it here
	// as a one-off so the library name matches the namespace prefix
	// (`task.wait` at call sites).
	if t, ok := p.match(lexer.TOKEN_TASK); ok {
		imp := &ImportStmt{pos: pos{File: use.File, Line: use.Line, Col: use.Col}, Name: "task"}
		if _, ok := p.match(lexer.TOKEN_AS); ok {
			alias, err := p.expect(lexer.TOKEN_IDENT, "after `as` in `use`")
			if err != nil {
				return nil, err
			}
			if containsUnderscore(alias.Lexeme) {
				return nil, &ParseError{Msg: fmt.Sprintf("library alias %q may not contain `_`", alias.Lexeme), File: alias.File, Line: alias.Line, Col: alias.Col}
			}
			imp.AsName = alias.Lexeme
		}
		if _, err := p.expect(lexer.TOKEN_SEMI, "after `use` statement"); err != nil {
			return nil, err
		}
		_ = t
		return imp, nil
	}
	name, err := p.expect(lexer.TOKEN_IDENT, "after `use`")
	if err != nil {
		return nil, err
	}
	if containsUnderscore(name.Lexeme) {
		return nil, &ParseError{Msg: fmt.Sprintf("library name %q may not contain `_`", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
	}
	imp := &ImportStmt{pos: pos{File: use.File, Line: use.Line, Col: use.Col}, Name: name.Lexeme}
	if _, ok := p.match(lexer.TOKEN_AS); ok {
		alias, err := p.expect(lexer.TOKEN_IDENT, "after `as` in `use`")
		if err != nil {
			return nil, err
		}
		if containsUnderscore(alias.Lexeme) {
			return nil, &ParseError{Msg: fmt.Sprintf("library alias %q may not contain `_`", alias.Lexeme), File: alias.File, Line: alias.Line, Col: alias.Col}
		}
		imp.AsName = alias.Lexeme
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "after `use` statement"); err != nil {
		return nil, err
	}
	return imp, nil
}

func (p *parser) parseMethodDef() (*MethodDef, error) {
	def, _ := p.match(lexer.TOKEN_FUNC)
	name, err := p.expect(lexer.TOKEN_IDENT, "after `func`")
	if err != nil {
		return nil, err
	}
	if containsUnderscore(name.Lexeme) {
		return nil, &ParseError{Msg: fmt.Sprintf("method name %q may not contain `_` (use camelCase: `myMethod`)", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
	}
	if _, err := p.expect(lexer.TOKEN_LPAREN, "after method name"); err != nil {
		return nil, err
	}
	var params []Param
	if !p.check(lexer.TOKEN_RPAREN) {
		for {
			// `name as TYPE`
			if v := p.peek(); v.Type == lexer.TOKEN_VARREF {
				return nil, &ParseError{
					Msg:  fmt.Sprintf("parameter name has no `$`: write `%s as TYPE`", v.Lexeme),
					File: v.File, Line: v.Line, Col: v.Col,
				}
			}
			pname, err := p.expect(lexer.TOKEN_IDENT, "for parameter name")
			if err != nil {
				return nil, err
			}
			if containsUnderscore(pname.Lexeme) {
				return nil, &ParseError{Msg: fmt.Sprintf("parameter name %q may not contain `_` (use camelCase)", pname.Lexeme), File: pname.File, Line: pname.Line, Col: pname.Col}
			}
			if _, err := p.expect(lexer.TOKEN_AS, "after parameter name"); err != nil {
				return nil, err
			}
			ptype, err := p.parseType()
			if err != nil {
				return nil, err
			}
			params = append(params, Param{Name: pname.Lexeme, Type: ptype, File: pname.File, Line: pname.Line, Col: pname.Col})
			if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
				break
			}
		}
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close parameter list"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &MethodDef{pos: pos{File: def.File, Line: def.Line, Col: def.Col}, Name: name.Lexeme, Params: params, Body: body}, nil
}

// parseExported consumes a leading `export` and the top-level declaration it
// marks - `func`, `def struct`, or `def const` - appending it to the program
// with Exported set. `export` in front of anything else (a mutable `def`, a
// statement, or nothing) is a positioned parse error. Whether a program may
// contain `export` at all (module vs script) is decided at load time, not
// here.
func (p *parser) parseExported(prog *Program) error {
	exp, _ := p.match(lexer.TOKEN_EXPORT)
	switch {
	case p.peek().Type == lexer.TOKEN_FUNC:
		m, err := p.parseMethodDef()
		if err != nil {
			return err
		}
		m.Exported = true
		prog.Methods = append(prog.Methods, m)
		return nil
	case p.peek().Type == lexer.TOKEN_DEFINE && p.peekN(1).Type == lexer.TOKEN_STRUCT:
		sd, err := p.parseStructDef()
		if err != nil {
			return err
		}
		sd.Exported = true
		prog.Structs = append(prog.Structs, sd)
		return nil
	case p.peek().Type == lexer.TOKEN_DEFINE && p.peekN(1).Type == lexer.TOKEN_CONST:
		st, err := p.parseDefineLike()
		if err != nil {
			return err
		}
		d, ok := st.(*DefineStmt)
		if !ok || !d.IsConst {
			return &ParseError{Msg: "only `def const`, `def struct`, and `func` can be exported", File: exp.File, Line: exp.Line, Col: exp.Col}
		}
		d.Exported = true
		prog.TopLevel = append(prog.TopLevel, st)
		return nil
	default:
		return &ParseError{Msg: "`export` must precede a top-level `def const`, `def struct`, or `func`", File: exp.File, Line: exp.Line, Col: exp.Col}
	}
}

// parseStructDef parses `def struct Name { field as type, ... };`.
// The leading `def` and `struct` tokens are at the head of
// the program token stream; we consume both, then the name, then a
// brace-delimited comma-separated field list, then the closing `;`.
// Top-level only - parseProgram does the dispatch.
func (p *parser) parseStructDef() (*StructDef, error) {
	def, _ := p.match(lexer.TOKEN_DEFINE)
	if _, err := p.expect(lexer.TOKEN_STRUCT, "after `def`"); err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.TOKEN_IDENT, "for struct name")
	if err != nil {
		return nil, err
	}
	if containsUnderscore(name.Lexeme) {
		return nil, &ParseError{Msg: fmt.Sprintf("struct name %q may not contain `_` (use PascalCase or camelCase)", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
	}
	if _, err := p.expect(lexer.TOKEN_LBRACE, "after struct name"); err != nil {
		return nil, err
	}
	var fields []StructField
	seen := map[string]bool{}
	if !p.check(lexer.TOKEN_RBRACE) {
		for {
			if v := p.peek(); v.Type == lexer.TOKEN_VARREF {
				return nil, &ParseError{
					Msg:  fmt.Sprintf("field name has no `$`: write `%s as TYPE`", v.Lexeme),
					File: v.File, Line: v.Line, Col: v.Col,
				}
			}
			fname, err := p.expectFieldName("for struct field name")
			if err != nil {
				return nil, err
			}
			if containsUnderscore(fname.Lexeme) {
				return nil, &ParseError{Msg: fmt.Sprintf("struct field name %q may not contain `_` (use camelCase)", fname.Lexeme), File: fname.File, Line: fname.Line, Col: fname.Col}
			}
			if seen[fname.Lexeme] {
				return nil, &ParseError{Msg: fmt.Sprintf("struct field %q declared twice", fname.Lexeme), File: fname.File, Line: fname.Line, Col: fname.Col}
			}
			seen[fname.Lexeme] = true
			if _, err := p.expect(lexer.TOKEN_AS, "after struct field name"); err != nil {
				return nil, err
			}
			ftype, err := p.parseType()
			if err != nil {
				return nil, err
			}
			fields = append(fields, StructField{Name: fname.Lexeme, Type: ftype, File: fname.File, Line: fname.Line, Col: fname.Col})
			if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
				break
			}
			// Trailing comma before `}` is allowed.
			if p.check(lexer.TOKEN_RBRACE) {
				break
			}
		}
	}
	if _, err := p.expect(lexer.TOKEN_RBRACE, "to close struct field list"); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate struct definition"); err != nil {
		return nil, err
	}
	return &StructDef{pos: pos{File: def.File, Line: def.Line, Col: def.Col}, Name: name.Lexeme, Fields: fields}, nil
}

func (p *parser) parseBlock() (*Block, error) {
	lb, err := p.expect(lexer.TOKEN_LBRACE, "to begin block")
	if err != nil {
		return nil, err
	}
	block := &Block{pos: pos{File: lb.File, Line: lb.Line, Col: lb.Col}}
	for !p.check(lexer.TOKEN_RBRACE) && !p.check(lexer.TOKEN_EOF) {
		st, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		block.Stmts = append(block.Stmts, st)
	}
	if _, err := p.expect(lexer.TOKEN_RBRACE, "to end block"); err != nil {
		return nil, err
	}
	return block, nil
}

func (p *parser) parseStatement() (Stmt, error) {
	switch p.peek().Type {
	case lexer.TOKEN_DEFINE:
		return p.parseDefineLike()
	case lexer.TOKEN_FUNC:
		t := p.peek()
		return nil, &ParseError{Msg: "methods can only be defined at the top level", File: t.File, Line: t.Line, Col: t.Col}
	case lexer.TOKEN_IF:
		return p.parseIf()
	case lexer.TOKEN_WHILE:
		return p.parseWhile()
	case lexer.TOKEN_FOR:
		return p.parseFor()
	case lexer.TOKEN_REPEAT:
		return p.parseRepeat()
	case lexer.TOKEN_BREAK:
		return p.parseBreak()
	case lexer.TOKEN_CONTINUE:
		return p.parseContinue()
	case lexer.TOKEN_RETURN:
		return p.parseReturn()
	case lexer.TOKEN_EXIT:
		return p.parseExit()
	case lexer.TOKEN_TRY:
		return p.parseTry()
	case lexer.TOKEN_THROW:
		return p.parseThrow()
	case lexer.TOKEN_DEFER, lexer.TOKEN_ERRDEFER:
		return p.parseDefer()
	case lexer.TOKEN_VARREF:
		// `$x = expr ;` is a simple assignment.
		if p.peekN(1).Type == lexer.TOKEN_ASSIGN {
			return p.parseAssign(true)
		}
		// `$xs[...] = expr ;` (or chained `$xs[i][j] = ...`) is an
		// index-assignment. `$p.field = expr;` (or chained) is a
		// field-assignment. Both share the same lvalue-chain
		// shape (VARREF followed by some mix of `[]` and `.field`),
		// so we use one tryParse for either.
		if p.peekN(1).Type == lexer.TOKEN_LBRACKET || p.peekN(1).Type == lexer.TOKEN_DOT {
			if stmt, ok, err := p.tryParseIndexAssign(); ok || err != nil {
				return stmt, err
			}
		}
	}
	// expression statement
	start := p.peek()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate statement"); err != nil {
		return nil, err
	}
	return &ExprStmt{pos: pos{File: start.File, Line: start.Line, Col: start.Col}, Expr: expr}, nil
}

// parseDefineLike handles `def` statements:
//
//	def NAME as T;
//	def NAME as T init EXPR;
//	def const NAME as T init EXPR;
//
// Methods use `func` and are handled by parseMethodDef.
func (p *parser) parseDefineLike() (Stmt, error) {
	def, _ := p.match(lexer.TOKEN_DEFINE)
	isConst := false
	if _, ok := p.match(lexer.TOKEN_CONST); ok {
		isConst = true
	}
	// The name is a bare IDENT. The `$` sigil is reserved for use-site
	// references to mutable variables. Catch the old-style `def $x ...`
	// here and produce a helpful error.
	if v := p.peek(); v.Type == lexer.TOKEN_VARREF {
		return nil, &ParseError{
			Msg:  fmt.Sprintf("drop the `$` here: write `def %s` (the `$` is only for use-site references)", v.Lexeme),
			File: v.File, Line: v.Line, Col: v.Col,
		}
	}
	if isConst {
		// constant: name is an IDENT, must be uppercase [A-Z]+
		name, err := p.expect(lexer.TOKEN_IDENT, "after `const`")
		if err != nil {
			return nil, err
		}
		if !isValidConstName(name.Lexeme) {
			return nil, &ParseError{Msg: fmt.Sprintf("constant name %q must be uppercase [A-Z]+ with single `_` separators (no leading, trailing, or consecutive `_`)", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
		}
		if _, err := p.expect(lexer.TOKEN_AS, "after constant name"); err != nil {
			return nil, err
		}
		tt, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TOKEN_INIT, "(constants require an `init` initializer)"); err != nil {
			return nil, err
		}
		initExpr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate constant define"); err != nil {
			return nil, err
		}
		return &DefineStmt{
			pos:      pos{File: def.File, Line: def.Line, Col: def.Col},
			IsConst:  true,
			VarName:  name.Lexeme,
			VarType:  tt,
			InitExpr: initExpr,
			Slot:     -1,
		}, nil
	}
	// variable - name is a bare IDENT
	name, err := p.expect(lexer.TOKEN_IDENT, "after `def`")
	if err != nil {
		return nil, err
	}
	if containsUnderscore(name.Lexeme) {
		return nil, &ParseError{Msg: fmt.Sprintf("variable name %q may not contain `_` (use camelCase; `_` is only allowed in constant names)", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
	}
	if _, err := p.expect(lexer.TOKEN_AS, "after variable name"); err != nil {
		return nil, err
	}
	tt, err := p.parseType()
	if err != nil {
		return nil, err
	}
	var initExpr Expr
	if _, ok := p.match(lexer.TOKEN_INIT); ok {
		initExpr, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate define"); err != nil {
		return nil, err
	}
	return &DefineStmt{
		pos:      pos{File: def.File, Line: def.Line, Col: def.Col},
		VarName:  name.Lexeme,
		VarType:  tt,
		InitExpr: initExpr,
		Slot:     -1,
	}, nil
}

// parseReturn parses `return;` or `return EXPR;`.
func (p *parser) parseReturn() (Stmt, error) {
	ret, _ := p.match(lexer.TOKEN_RETURN)
	stmt := &ReturnStmt{pos: pos{File: ret.File, Line: ret.Line, Col: ret.Col}}
	if !p.check(lexer.TOKEN_SEMI) {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		stmt.Value = expr
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate return"); err != nil {
		return nil, err
	}
	return stmt, nil
}

// parseBreak parses `break;`. Loop-scope validity is enforced at
// runtime (not here) so an error in a deeply-nested misuse can carry
// the body-statement's position.
func (p *parser) parseBreak() (Stmt, error) {
	bk, _ := p.match(lexer.TOKEN_BREAK)
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate break"); err != nil {
		return nil, err
	}
	return &BreakStmt{pos: pos{File: bk.File, Line: bk.Line, Col: bk.Col}}, nil
}

// parseContinue parses `continue;`. Same shape and validation rule as
// parseBreak.
func (p *parser) parseContinue() (Stmt, error) {
	ct, _ := p.match(lexer.TOKEN_CONTINUE)
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate continue"); err != nil {
		return nil, err
	}
	return &ContinueStmt{pos: pos{File: ct.File, Line: ct.Line, Col: ct.Col}}, nil
}

// parseRepeat parses `repeat { ... } until (cond);` - the post-test
// loop. Body runs at least once; cond is re-evaluated AFTER each pass
// and stops the loop when true.
func (p *parser) parseRepeat() (Stmt, error) {
	rp, _ := p.match(lexer.TOKEN_REPEAT)
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_UNTIL, "after `repeat { ... }` body"); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_LPAREN, "before `repeat ... until` condition"); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close `repeat ... until` condition"); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate `repeat ... until` statement"); err != nil {
		return nil, err
	}
	return &RepeatStmt{
		pos:  pos{File: rp.File, Line: rp.Line, Col: rp.Col},
		Body: body,
		Cond: cond,
	}, nil
}

// parseExit parses `exit;` or `exit EXPR;`. The optional EXPR is the
// process exit code; defaults to 0 when bare. Distinct from `return`
// (which is method-scoped); exit terminates the whole program.
func (p *parser) parseExit() (Stmt, error) {
	ex, _ := p.match(lexer.TOKEN_EXIT)
	stmt := &ExitStmt{pos: pos{File: ex.File, Line: ex.Line, Col: ex.Col}}
	if !p.check(lexer.TOKEN_SEMI) {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		stmt.Code = expr
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate exit"); err != nil {
		return nil, err
	}
	return stmt, nil
}

// parseTry parses `try { ... } catch (NAME) { ... }`. The catch
// binding is a bare IDENT following the iteration-variable rule (no
// `_`, letters-only). No `finally` in v1.
func (p *parser) parseTry() (Stmt, error) {
	tk, _ := p.match(lexer.TOKEN_TRY)
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	catchTok, err := p.expect(lexer.TOKEN_CATCH, "after `try { ... }` body")
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_LPAREN, "before catch binding"); err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.TOKEN_IDENT, "for catch binding name")
	if err != nil {
		return nil, err
	}
	if containsUnderscore(name.Lexeme) {
		return nil, &ParseError{
			Msg:  fmt.Sprintf("catch variable name %q may not contain `_` (use camelCase)", name.Lexeme),
			File: name.File, Line: name.Line, Col: name.Col,
		}
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close catch binding"); err != nil {
		return nil, err
	}
	handler, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &TryStmt{
		pos:       pos{File: tk.File, Line: tk.Line, Col: tk.Col},
		Body:      body,
		CatchName: name.Lexeme,
		CatchBody: handler,
		CatchFile: catchTok.File,
		CatchLine: catchTok.Line,
		CatchCol:  catchTok.Col,
		CatchSlot: -1,
	}, nil
}

// parseThrow parses `throw EXPR;`. The EXPR may produce any value;
// convention is an `Error` struct.
func (p *parser) parseThrow() (Stmt, error) {
	tk, _ := p.match(lexer.TOKEN_THROW)
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate throw"); err != nil {
		return nil, err
	}
	return &ThrowStmt{
		pos:   pos{File: tk.File, Line: tk.Line, Col: tk.Col},
		Value: expr,
	}, nil
}

// parseDefer parses `defer CALL(args);` and its error-path variant
// `errdefer CALL(args);`. The deferred thing must be a call - a user method
// (`defer cleanup();`) or a namespaced / module call (`defer fs.close($f);`).
// Anything else (a bare value, an operator expression, an index/field access)
// is a parse error: `defer` schedules a call, and the single-call form is what
// keeps a `return` / `throw` from hiding inside it. `errdefer` differs only at
// runtime (the call runs solely when the block exits with an error), so both
// keywords share this parse and the DeferStmt node.
func (p *parser) parseDefer() (Stmt, error) {
	onError := p.peek().Type == lexer.TOKEN_ERRDEFER
	keyword := "defer"
	if onError {
		keyword = "errdefer"
	}
	tk := p.advance()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	switch expr.(type) {
	case *CallExpr, *QualifiedCallExpr:
		// ok: a method call or a namespaced / module call.
	default:
		l, c := expr.Pos()
		return nil, &ParseError{
			Msg:  fmt.Sprintf("`%s` requires a function call, e.g. `%s fs.close($f);`", keyword, keyword),
			File: expr.Filename(), Line: l, Col: c,
		}
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate "+keyword); err != nil {
		return nil, err
	}
	return &DeferStmt{
		pos:     pos{File: tk.File, Line: tk.Line, Col: tk.Col},
		Call:    expr,
		OnError: onError,
	}, nil
}

// tryParseIndexAssign attempts to parse `$xs[i]...[j] = expr ;` as an
// IndexAssignStmt, or `$xs[] = expr ;` as an AppendStmt.
// Returns (stmt, true, nil) on success. When the lvalue chain parses but no
// `=` follows, the chain was actually the start of an expression statement;
// rather than restore the position and re-parse, it continues the expression
// from the already-built chain (via parseExprFrom) and returns that ExprStmt.
// Errors during the lvalue chain itself are propagated.
//
// The append form `$xs[] = expr ;` is detected first because it can't
// be mistaken for anything else: an empty index between a bare VARREF
// and `=` has no read meaning (you can't read past-the-end), so any
// occurrence is an unambiguous append.
func (p *parser) tryParseIndexAssign() (Stmt, bool, error) {
	vref, _ := p.match(lexer.TOKEN_VARREF)
	// Detect the append form `$xs[] = expr ;` before walking any
	// index chain - chained appends like `$xs[0][] = ...` aren't
	// supported and would just confuse the chain-parsing loop below.
	if p.check(lexer.TOKEN_LBRACKET) && p.peekN(1).Type == lexer.TOKEN_RBRACKET {
		p.advance() // [
		p.advance() // ]
		if _, ok := p.match(lexer.TOKEN_ASSIGN); !ok {
			// `$xs[]` not followed by `=` is a parse error - reads of the
			// append-slot have no meaning. Don't restore the position; this
			// is a clean, actionable diagnostic.
			return nil, false, &ParseError{
				Msg:  fmt.Sprintf("`$%s[]` is write-only (append form); reads are not allowed", vref.Lexeme),
				File: vref.File, Line: vref.Line, Col: vref.Col,
			}
		}
		val, err := p.parseExpr()
		if err != nil {
			return nil, false, err
		}
		if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate append"); err != nil {
			return nil, false, err
		}
		return &AppendStmt{
			pos:    pos{File: vref.File, Line: vref.Line, Col: vref.Col},
			Target: &VarExpr{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, Name: vref.Lexeme, Depth: -1, Slot: -1},
			Value:  val,
		}, true, nil
	}
	var target Expr = &VarExpr{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, Name: vref.Lexeme, Depth: -1, Slot: -1}
	// Consume any mix of `[expr]` and `.field` suffixes.
	for {
		switch p.peek().Type {
		case lexer.TOKEN_LBRACKET:
			bra := p.peek()
			p.advance()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, false, err
			}
			if _, err := p.expect(lexer.TOKEN_RBRACKET, "to close index expression"); err != nil {
				return nil, false, err
			}
			target = &IndexExpr{
				pos:    pos{File: bra.File, Line: bra.Line, Col: bra.Col},
				Target: target,
				Index:  idx,
			}
			continue
		case lexer.TOKEN_DOT:
			dot := p.peek()
			p.advance()
			name, err := p.expectFieldName("after `.` for field name")
			if err != nil {
				return nil, false, err
			}
			target = &FieldAccessExpr{
				pos:    pos{File: dot.File, Line: dot.Line, Col: dot.Col},
				Target: target,
				Field:  name.Lexeme,
			}
			continue
		}
		break
	}
	if _, ok := p.match(lexer.TOKEN_ASSIGN); !ok {
		// Not an assignment: the lvalue chain we parsed is actually the start of
		// an expression statement. Continue the expression from the chain we
		// already built (seeding the ladder) instead of restoring the position
		// and re-parsing it from scratch.
		expr, err := p.parseExprFrom(target)
		if err != nil {
			return nil, false, err
		}
		if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate statement"); err != nil {
			return nil, false, err
		}
		return &ExprStmt{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, Expr: expr}, true, nil
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, false, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate assignment"); err != nil {
		return nil, false, err
	}
	// Pick the right stmt kind based on the leaf node we landed on.
	switch leaf := target.(type) {
	case *IndexExpr:
		return &IndexAssignStmt{
			pos:    pos{File: vref.File, Line: vref.Line, Col: vref.Col},
			Target: leaf,
			Value:  val,
		}, true, nil
	case *FieldAccessExpr:
		return &FieldAssignStmt{
			pos:    pos{File: vref.File, Line: vref.Line, Col: vref.Col},
			Target: leaf,
			Value:  val,
		}, true, nil
	}
	return nil, false, &ParseError{Msg: "expected index or field expression on left of `=`", File: vref.File, Line: vref.Line, Col: vref.Col}
}

// parseAssign parses `$x = expr;`. If consumeSemi is false (e.g. inside a
// for-loop step), the trailing `;` is not consumed.
func (p *parser) parseAssign(consumeSemi bool) (Stmt, error) {
	vref, _ := p.match(lexer.TOKEN_VARREF)
	if _, err := p.expect(lexer.TOKEN_ASSIGN, "in assignment"); err != nil {
		return nil, err
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if consumeSemi {
		if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate assignment"); err != nil {
			return nil, err
		}
	}
	return &AssignStmt{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, VarName: vref.Lexeme, Value: val, Depth: -1, Slot: -1}, nil
}

func (p *parser) parseIf() (Stmt, error) {
	ift, _ := p.match(lexer.TOKEN_IF)
	cond, err := p.parseParenCond("if")
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	stmt := &IfStmt{pos: pos{File: ift.File, Line: ift.Line, Col: ift.Col}, Cond: cond, Then: body}
	for p.check(lexer.TOKEN_ELSEIF) {
		p.advance()
		c, err := p.parseParenCond("elseif")
		if err != nil {
			return nil, err
		}
		b, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		stmt.ElseIfs = append(stmt.ElseIfs, c)
		stmt.ElseIfBodies = append(stmt.ElseIfBodies, b)
	}
	if p.check(lexer.TOKEN_ELSE) {
		p.advance()
		b, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		stmt.Else = b
	}
	return stmt, nil
}

func (p *parser) parseWhile() (Stmt, error) {
	wt, _ := p.match(lexer.TOKEN_WHILE)
	cond, err := p.parseParenCond("while")
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &WhileStmt{pos: pos{File: wt.File, Line: wt.Line, Col: wt.Col}, Cond: cond, Body: body}, nil
}

func (p *parser) parseFor() (Stmt, error) {
	ft, _ := p.match(lexer.TOKEN_FOR)
	if _, err := p.expect(lexer.TOKEN_LPAREN, "after `for`"); err != nil {
		return nil, err
	}
	// Disambiguate `for (def NAME in EXPR)` (for-each) from the C-style
	// `for (def NAME as TYPE init EXPR; cond; step)`. Both start with
	// `def IDENT`; the token after the IDENT decides:
	//   - `in`  -> for-each
	//   - `as`  -> regular for
	// Anything else is a parse error inside parseDefineLike or here.
	if p.check(lexer.TOKEN_DEFINE) && p.peekN(1).Type == lexer.TOKEN_IDENT && p.peekN(2).Type == lexer.TOKEN_IN {
		return p.parseForEachTail(ft)
	}
	// init: define-stmt | assign | empty
	var initStmt Stmt
	if p.check(lexer.TOKEN_DEFINE) {
		s, err := p.parseDefineLike() // consumes its own `;`
		if err != nil {
			return nil, err
		}
		initStmt = s
	} else if p.check(lexer.TOKEN_VARREF) && p.peekN(1).Type == lexer.TOKEN_ASSIGN {
		s, err := p.parseAssign(true) // consumes `;`
		if err != nil {
			return nil, err
		}
		initStmt = s
	} else if !p.check(lexer.TOKEN_SEMI) {
		t := p.peek()
		return nil, &ParseError{Msg: fmt.Sprintf("expected for-init (define or assignment), got %s (%q)", t.Type, t.Lexeme), File: t.File, Line: t.Line, Col: t.Col}
	} else {
		p.advance() // consume `;` for empty init
	}
	// cond: expr | empty
	var cond Expr
	if !p.check(lexer.TOKEN_SEMI) {
		c, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		cond = c
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "after for-condition"); err != nil {
		return nil, err
	}
	// step: assign | empty   (no trailing `;`, the `)` terminates it)
	var step Stmt
	if p.check(lexer.TOKEN_VARREF) && p.peekN(1).Type == lexer.TOKEN_ASSIGN {
		s, err := p.parseAssign(false)
		if err != nil {
			return nil, err
		}
		step = s
	} else if !p.check(lexer.TOKEN_RPAREN) {
		t := p.peek()
		return nil, &ParseError{Msg: fmt.Sprintf("expected for-step (assignment) or `)`, got %s (%q)", t.Type, t.Lexeme), File: t.File, Line: t.Line, Col: t.Col}
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close for-header"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ForStmt{pos: pos{File: ft.File, Line: ft.Line, Col: ft.Col}, Init: initStmt, Cond: cond, Step: step, Body: body}, nil
}

// parseForEachTail finishes a `for (def NAME in EXPR) { body }` after
// the `for` and `(` have already been consumed by parseFor. The
// iteration variable name uses the same rule as a regular def (no
// underscores, [A-Za-z]{1,64}); the collection expression is anything
// that evaluates to a list or map at runtime.
func (p *parser) parseForEachTail(ft lexer.Token) (Stmt, error) {
	// Consume `def NAME in`.
	def, _ := p.match(lexer.TOKEN_DEFINE)
	_ = def
	name, err := p.expect(lexer.TOKEN_IDENT, "after `def` in for-each")
	if err != nil {
		return nil, err
	}
	if containsUnderscore(name.Lexeme) {
		return nil, &ParseError{
			Msg:  fmt.Sprintf("iteration variable name %q may not contain `_` (use camelCase)", name.Lexeme),
			File: name.File, Line: name.Line, Col: name.Col,
		}
	}
	if _, err := p.expect(lexer.TOKEN_IN, "after for-each variable"); err != nil {
		return nil, err
	}
	coll, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close for-each header"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ForEachStmt{
		pos:      pos{File: ft.File, Line: ft.Line, Col: ft.Col},
		VarName:  name.Lexeme,
		Coll:     coll,
		Body:     body,
		IterSlot: -1,
	}, nil
}

func (p *parser) parseParenCond(ctx string) (Expr, error) {
	if _, err := p.expect(lexer.TOKEN_LPAREN, "after `"+ctx+"`"); err != nil {
		return nil, err
	}
	c, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close `"+ctx+"` condition"); err != nil {
		return nil, err
	}
	return c, nil
}

// parseType consumes a type expression. Primitives are a single keyword
// (`int`, `float`, `string`, `bool`, `null`). Compound types are
// recursive:
//
//	list of <TYPE>
//	map  of <TYPE> to <TYPE>
//
// Nesting falls out naturally because each recursive call reads one
// fresh <TYPE>; there is no depth cap.
func (p *parser) parseType() (Type, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TOKEN_INT_TYPE:
		p.advance()
		return PrimitiveType(TypeInt), nil
	case lexer.TOKEN_FLOAT_TYPE:
		p.advance()
		return PrimitiveType(TypeFloat), nil
	case lexer.TOKEN_STRING_TYPE:
		p.advance()
		return PrimitiveType(TypeString), nil
	case lexer.TOKEN_BOOL_TYPE:
		p.advance()
		return PrimitiveType(TypeBool), nil
	case lexer.TOKEN_BYTES_TYPE:
		p.advance()
		return PrimitiveType(TypeBytes), nil
	case lexer.TOKEN_NULL:
		p.advance()
		return PrimitiveType(TypeNull), nil
	case lexer.TOKEN_LIST:
		p.advance()
		if _, err := p.expect(lexer.TOKEN_OF, "after `list`"); err != nil {
			return Type{}, err
		}
		elem, err := p.parseType()
		if err != nil {
			return Type{}, err
		}
		return ListType(elem), nil
	case lexer.TOKEN_MAP:
		p.advance()
		if _, err := p.expect(lexer.TOKEN_OF, "after `map`"); err != nil {
			return Type{}, err
		}
		keyT, err := p.parseType()
		if err != nil {
			return Type{}, err
		}
		if _, err := p.expect(lexer.TOKEN_TO, "after map key type"); err != nil {
			return Type{}, err
		}
		valT, err := p.parseType()
		if err != nil {
			return Type{}, err
		}
		return MapType(keyT, valT), nil
	case lexer.TOKEN_TASK:
		p.advance()
		if _, err := p.expect(lexer.TOKEN_OF, "after `task`"); err != nil {
			return Type{}, err
		}
		elem, err := p.parseType()
		if err != nil {
			return Type{}, err
		}
		return TaskType(elem), nil
	case lexer.TOKEN_IDENT:
		// Struct type reference. Bare IDENT (`def x as Name;`) names a
		// top-level user struct; `IDENT.IDENT` (`def x as os.Result;`)
		// names a library-registered namespaced struct. Both
		// resolve at run time against their respective hoisted tables,
		// producing a positioned error if the name isn't known.
		p.advance()
		if p.check(lexer.TOKEN_DOT) && p.peekN(1).Type == lexer.TOKEN_IDENT {
			p.advance() // .
			name := p.advance()
			return NamespacedStructType(t.Lexeme, name.Lexeme), nil
		}
		return StructType(t.Lexeme), nil
	}
	return Type{}, &ParseError{Msg: fmt.Sprintf("expected type, got %s (%q)", t.Type, t.Lexeme), File: t.File, Line: t.Line, Col: t.Col}
}

// isValidConstName reports whether s matches the constant naming rule:
// `[A-Z]+(_[A-Z]+)*` - one or more uppercase chunks separated by single
// `_` characters. Equivalently: every `_` must be immediately followed by
// `[A-Z]`, never another `_` and never the end of the name. The lexer
// already refuses identifiers that start with `_` or end with `_`; this
// function additionally enforces the uppercase requirement and the
// "no consecutive `_`" rule.
//
// Accepted:  A, MAX, MAX_RETRIES, HTTP_OK, A_B_C
// Rejected:  _MAX, MAX_, max_int, MAX__INT, MAX___RETRIES
func isValidConstName(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '_' || s[len(s)-1] == '_' {
		return false
	}
	prevUnderscore := false
	for _, r := range s {
		if r == '_' {
			if prevUnderscore {
				return false
			}
			prevUnderscore = true
			continue
		}
		if r < 'A' || r > 'Z' {
			return false
		}
		prevUnderscore = false
	}
	return true
}

// containsUnderscore is used by variable / method / parameter validation:
// those kinds keep the letters-only rule `[A-Za-z]{1,64}`, so any `_` is
// a parse error. We check here rather than in the lexer because the lexer
// accepts `_` in bare IDENTs to support constants.
func containsUnderscore(s string) bool {
	for _, r := range s {
		if r == '_' {
			return true
		}
	}
	return false
}

// isPostDotName reports whether `t` can appear as the name slot of a
// qualified call (`prefix.NAME`) or qualified constant ref. After a
// `.` no statement keyword can start, so a reserved word is
// unambiguously a name there - `strings.repeat`, `strings.split`,
// `lists.for` (if anyone wrote that) all read fine. Lets reserved
// words coexist with library names that happen to spell the same way.
//
// The same rule covers struct field names: `def struct Line {
// from as Point, to as Point };` works even though `to` is a keyword
// (the `to` in `map of K to V`), because the field-name position is
// unambiguous between the surrounding `{` / `,` / `as` / `}` tokens.
func isPostDotName(t lexer.Token) bool {
	if t.Type == lexer.TOKEN_IDENT {
		return true
	}
	// Any token whose lexeme is a non-empty identifier-shaped word.
	// Built-in keywords all carry their spelling in t.Lexeme; type
	// tokens (TOKEN_INT_TYPE etc.) and operator tokens (`and`, `or`,
	// `not`) likewise. Punctuation tokens have lexemes like "{" or ","
	// that fail the first-letter check, so they're excluded.
	if t.Lexeme == "" {
		return false
	}
	c := t.Lexeme[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// expectFieldName consumes an identifier-shaped token for a struct
// field name. Accepts IDENT plus any keyword whose lexeme
// looks like an identifier - the field-name position is contextually
// unambiguous so reserved-word collisions don't apply.
func (p *parser) expectFieldName(ctx string) (lexer.Token, error) {
	t := p.peek()
	if isPostDotName(t) {
		p.advance()
		return t, nil
	}
	return lexer.Token{}, &ParseError{Msg: fmt.Sprintf("expected identifier %s, got %s (%q)", ctx, t.Type, t.Lexeme), File: t.File, Line: t.Line, Col: t.Col}
}

func (p *parser) parseExpr() (Expr, error) {
	// parseExpr is the single re-entry point for every nested expression - list
	// and map elements, struct-literal field values, call arguments, and grouped
	// parens all recurse back through here - so one counter bounds structural
	// nesting for all of them. The cap is the depth the constrained TinyGo
	// binary's fixed 4 MB stack survives (the decoders share the same limit); an
	// uncapped deeply-nested literal from stdin or a downloaded script would
	// otherwise crash the process outright rather than raise a catchable error.
	if p.exprDepth >= limits.MaxNestingDepth {
		t := p.peek()
		return nil, &ParseError{
			Msg:  fmt.Sprintf("expression nesting exceeds %d levels", limits.MaxNestingDepth),
			File: t.File, Line: t.Line, Col: t.Col,
		}
	}
	p.exprDepth++
	e, err := p.parseOr()
	p.exprDepth--
	return e, err
}

// Precedence (lowest to highest):
//   or, and, not, comparison, addition, multiplication, unary -, primary.
// `not` is right-associative (so `not not x` works). Unary `-` is also
// right-associative.

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TOKEN_OR) {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: OpOr, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TOKEN_AND) {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: OpAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (Expr, error) {
	// A pending seed (parseExprFrom) is the already-parsed leading operand,
	// so the next token sits in binary-operator position: `$x[0] - 1` must
	// not read the `-` as a prefix operator. Skip the prefix match and let
	// the descent reach parsePrimaryAtom, which consumes the seed.
	if p.seed != nil {
		return p.parseComparison()
	}
	if t, ok := p.match(lexer.TOKEN_NOT); ok {
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Op: OpNot, Operand: operand}, nil
	}
	return p.parseComparison()
}

// Comparison is lower-precedence than additive, non-associative-ish: we accept
// a chain (`a < b == c`) syntactically but it's almost certainly a bug; for
// now we parse left-associatively and let semantics reject mixed-kind ops.
//
// Four rungs sit between comparison and additive for the bit
// operators (`|`, `^`, `&`, `<<` / `>>`) following Python's precedence:
//
//	comparison
//	  |   (bitwise OR)
//	  ^   (bitwise XOR)
//	  &   (bitwise AND)
//	  << >> (shifts)
//	additive (+ -)
//	multiplicative (* / // %)
//	unary (- ~ not)
//
// Each level recurses into the next tighter one. Python ordering avoids
// the C `x & 0xff == 0` footgun (which C parses as `x & (0xff == 0)`).
func (p *parser) parseComparison() (Expr, error) {
	left, err := p.parseBitOr()
	if err != nil {
		return nil, err
	}
	for {
		var op BinaryOp
		t := p.peek()
		switch t.Type {
		case lexer.TOKEN_LT:
			op = OpLt
		case lexer.TOKEN_GT:
			op = OpGt
		case lexer.TOKEN_LE:
			op = OpLe
		case lexer.TOKEN_GE:
			op = OpGe
		case lexer.TOKEN_EQ:
			op = OpEq
		case lexer.TOKEN_NEQ:
			op = OpNeq
		default:
			return left, nil
		}
		p.advance()
		right, err := p.parseBitOr()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseBitOr() (Expr, error) {
	left, err := p.parseBitXor()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == lexer.TOKEN_BIT_OR {
		p.advance()
		right, err := p.parseBitXor()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: OpBitOr, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseBitXor() (Expr, error) {
	left, err := p.parseBitAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == lexer.TOKEN_BIT_XOR {
		p.advance()
		right, err := p.parseBitAnd()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: OpBitXor, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseBitAnd() (Expr, error) {
	left, err := p.parseShift()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == lexer.TOKEN_BIT_AND {
		p.advance()
		right, err := p.parseShift()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: OpBitAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseShift() (Expr, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for {
		var op BinaryOp
		switch p.peek().Type {
		case lexer.TOKEN_SHL:
			op = OpShl
		case lexer.TOKEN_SHR:
			op = OpShr
		default:
			return left, nil
		}
		p.advance()
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseAdd() (Expr, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		var op BinaryOp
		t := p.peek()
		switch t.Type {
		case lexer.TOKEN_PLUS:
			op = OpAdd
		case lexer.TOKEN_MINUS:
			op = OpSub
		default:
			return left, nil
		}
		p.advance()
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseMul() (Expr, error) {
	left, err := p.parseUnaryMinus()
	if err != nil {
		return nil, err
	}
	for {
		var op BinaryOp
		t := p.peek()
		switch t.Type {
		case lexer.TOKEN_STAR:
			op = OpMul
		case lexer.TOKEN_SLASH:
			op = OpDiv
		case lexer.TOKEN_DIV:
			op = OpFloorDiv
		case lexer.TOKEN_PERCENT:
			op = OpMod
		default:
			return left, nil
		}
		p.advance()
		right, err := p.parseUnaryMinus()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{File: left.Filename(), Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

// parseCallTail parses the `(args...)` portion of a call expression. The
// callee token (already consumed) is passed in so we can attach the right
// position and callee lexeme.
func (p *parser) parseCallTail(callee lexer.Token) (Expr, error) {
	// Type-keyword callers (int(...), float(...), etc.) come through here too
	// with TOKEN_INT_TYPE / TOKEN_FLOAT_TYPE / .. tokens - their lexemes are
	// always `int`/`float`/`string`/`bool` and never contain `_`. The check
	// is therefore only meaningful for actual IDENT callees.
	if callee.Type == lexer.TOKEN_IDENT && containsUnderscore(callee.Lexeme) {
		return nil, &ParseError{Msg: fmt.Sprintf("method name %q may not contain `_` (use camelCase)", callee.Lexeme), File: callee.File, Line: callee.Line, Col: callee.Col}
	}
	p.advance() // consume LPAREN
	var args []Expr
	if !p.check(lexer.TOKEN_RPAREN) {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
				break
			}
		}
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close call argument list"); err != nil {
		return nil, err
	}
	return &CallExpr{pos: pos{File: callee.File, Line: callee.Line, Col: callee.Col}, Callee: callee.Lexeme, Args: args}, nil
}

// parseStructLitTail parses the `{ field: expr, ... }` portion of a
// struct literal expression, having already consumed the struct name
// IDENT (passed as `name`). The interpreter resolves the name against
// the program's struct table and enforces "every declared field must
// appear exactly once" plus the per-field type check. Trailing comma
// before `}` is allowed; empty `Name{}` is allowed and means
// "zero-initialised", which the interpreter accepts only when the
// struct has no fields - otherwise every field is required.
func (p *parser) parseStructLitTail(name lexer.Token) (Expr, error) {
	p.advance() // consume LBRACE
	var fields []StructLitField
	seen := map[string]bool{}
	if !p.check(lexer.TOKEN_RBRACE) {
		for {
			fname, err := p.expectFieldName("for struct field name in literal")
			if err != nil {
				return nil, err
			}
			if seen[fname.Lexeme] {
				return nil, &ParseError{Msg: fmt.Sprintf("field %q assigned twice in struct literal", fname.Lexeme), File: fname.File, Line: fname.Line, Col: fname.Col}
			}
			seen[fname.Lexeme] = true
			if _, err := p.expect(lexer.TOKEN_COLON, "after struct field name"); err != nil {
				return nil, err
			}
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			fields = append(fields, StructLitField{Name: fname.Lexeme, Expr: val, File: fname.File, Line: fname.Line, Col: fname.Col})
			if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
				break
			}
			if p.check(lexer.TOKEN_RBRACE) {
				break
			}
		}
	}
	if _, err := p.expect(lexer.TOKEN_RBRACE, "to close struct literal"); err != nil {
		return nil, err
	}
	return &StructLit{pos: pos{File: name.File, Line: name.Line, Col: name.Col}, Name: name.Lexeme, Fields: fields}, nil
}

// parseQualifiedTail parses the `. IDENT (args)?` portion of a qualified
// reference, having already consumed the prefix IDENT (passed as
// `prefix`). When followed by `(`, returns a QualifiedCallExpr;
// otherwise a QualifiedConstRefExpr. The post-DOT name follows the
// regular method-name rule (letters only, no `_`); qualified constants
// follow the constant-name rule (`[A-Z]+(_[A-Z]+)*`). The interpreter
// resolves (prefix, name) against the namespaced-builtin / constant
// registry, gated by `use prefix;` (or the alias-aware equivalent).
func (p *parser) parseQualifiedTail(prefix lexer.Token) (Expr, error) {
	p.advance() // consume DOT
	// After a `.` the name slot is unambiguously a callee or constant -
	// no statement keyword can start there - so a token that lexes as a
	// reserved word (`repeat`, `until`, `for`, ...) reads as a name in
	// this position. Accept any identifier-shaped keyword in addition
	// to a plain IDENT. This preserves library names like
	// `strings.repeat` even though `repeat` is also a loop keyword,
	// and pre-emptively dodges similar collisions for any
	// future keyword.
	var name lexer.Token
	if isPostDotName(p.peek()) {
		name = p.peek()
		p.advance()
	} else {
		var err error
		name, err = p.expect(lexer.TOKEN_IDENT, "after `.` in qualified call")
		if err != nil {
			return nil, err
		}
	}
	if p.check(lexer.TOKEN_LPAREN) {
		if containsUnderscore(name.Lexeme) {
			return nil, &ParseError{Msg: fmt.Sprintf("method name %q may not contain `_` (use camelCase)", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
		}
		p.advance() // consume LPAREN
		var args []Expr
		if !p.check(lexer.TOKEN_RPAREN) {
			for {
				arg, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
					break
				}
			}
		}
		if _, err := p.expect(lexer.TOKEN_RPAREN, "to close qualified call argument list"); err != nil {
			return nil, err
		}
		return &QualifiedCallExpr{
			pos:    pos{File: prefix.File, Line: prefix.Line, Col: prefix.Col},
			Prefix: prefix.Lexeme,
			Callee: name.Lexeme,
			Args:   args,
		}, nil
	}
	// Namespaced struct literal: prefix.Name { field: expr, ... }.
	// Checked before the qualified-const path so a struct's
	// PascalCase / camelCase name doesn't trip the constant-name rule.
	if p.check(lexer.TOKEN_LBRACE) {
		lit, err := p.parseStructLitTail(name)
		if err != nil {
			return nil, err
		}
		sl := lit.(*StructLit)
		sl.NS = prefix.Lexeme
		sl.pos = pos{File: prefix.File, Line: prefix.Line, Col: prefix.Col}
		return sl, nil
	}
	// Qualified constant reference: prefix.NAME. Same constant-name rule
	// as bare constants - uppercase chunks separated by single `_`.
	if !isValidConstName(name.Lexeme) {
		// A constant-named prefix with a non-const post-dot name is field
		// access on a (deep const) struct: `ORIGIN.x`. Fall back to a
		// FieldAccessExpr so the field is reachable directly, not only via the
		// `(ORIGIN).x` parenthesis workaround. The postfix loop in parsePrimary
		// continues any further `.field` / `[i]` chain.
		if isValidConstName(prefix.Lexeme) {
			base := pos{File: prefix.File, Line: prefix.Line, Col: prefix.Col}
			return &FieldAccessExpr{
				pos:    base,
				Target: &ConstRefExpr{pos: base, Name: prefix.Lexeme, Depth: -1, Slot: -1},
				Field:  name.Lexeme,
			}, nil
		}
		return nil, &ParseError{Msg: fmt.Sprintf("qualified constant name %q must be uppercase [A-Z]+ with single `_` separators", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
	}
	return &QualifiedConstRefExpr{
		pos:    pos{File: prefix.File, Line: prefix.Line, Col: prefix.Col},
		Prefix: prefix.Lexeme,
		Name:   name.Lexeme,
	}, nil
}

// parseUnaryMinus handles the `-EXPR` and `~EXPR` prefix forms. Sits
// between multiplicative and primary so `-x * 2` parses as `(-x) * 2`.
// Right-associative: `--x` is `-(-x)`, `~~x` is `~(~x)`. Mixing is
// allowed: `-~x` is `-(~x)`.
func (p *parser) parseUnaryMinus() (Expr, error) {
	// With a seed pending the leading operand already exists; `-` / `~` here
	// would be a binary operator (or an error), never a prefix. See parseNot.
	if p.seed != nil {
		return p.parsePrimary()
	}
	if t, ok := p.match(lexer.TOKEN_MINUS); ok {
		// The most-negative int literal, e.g. -9223372036854775808 (=
		// math.MinInt64) and its 0x/0o/0b forms, has magnitude 2^63 - one past
		// MaxInt64 - so it exists only as a *negated* literal; parsePrimaryAtom's
		// ParseInt would reject the bare magnitude. Fold the sign here for that
		// one case; every other magnitude takes the normal path (fits, or a
		// truly out-of-range magnitude still errors in parsePrimaryAtom).
		if p.peek().Type == lexer.TOKEN_INT {
			if lit := p.foldMostNegativeInt(); lit != nil {
				return lit, nil
			}
		}
		operand, err := p.parseUnaryMinus()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Op: OpNeg, Operand: operand}, nil
	}
	if t, ok := p.match(lexer.TOKEN_BIT_NOT); ok {
		operand, err := p.parseUnaryMinus()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Op: OpBitNot, Operand: operand}, nil
	}
	return p.parsePrimary()
}

// foldMostNegativeInt consumes the pending TOKEN_INT and returns an
// IntLit(math.MinInt64) when its magnitude is exactly 2^63 (the only value that
// is out of range as a positive literal but in range once negated). Returns nil
// without consuming for any other magnitude, so the caller's normal path runs.
func (p *parser) foldMostNegativeInt() Expr {
	t := p.peek()
	lex := t.Lexeme
	base := 10
	if strings.HasPrefix(lex, "0x") || strings.HasPrefix(lex, "0o") || strings.HasPrefix(lex, "0b") {
		base = 0
	}
	u, err := strconv.ParseUint(lex, base, 64)
	if err != nil || u != 1<<63 {
		return nil
	}
	p.advance()
	return &IntLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Value: math.MinInt64}
}

// parsePrimary parses an atom (literal, var ref, call, grouped expr,
// list/map/struct literal) and then chains any number of `[index]` /
// `.field` suffixes onto it. Returning the chained form from `primary`
// lets every level above (unary, mul, add, ...) treat `$xs[0]` and
// `$p.field` exactly like any other expression without special-casing.
// parseExprFrom continues parsing an expression whose leading primary has
// already been built (by tryParseIndexAssign). It seeds the ladder so the
// operator-precedence climb runs once over the pre-parsed lvalue chain instead
// of the caller restoring the position and re-parsing it.
func (p *parser) parseExprFrom(primary Expr) (Expr, error) {
	p.seed = primary
	return p.parseExpr()
}

func (p *parser) parsePrimary() (Expr, error) {
	e, err := p.parsePrimaryAtom()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().Type {
		case lexer.TOKEN_LBRACKET:
			bra := p.peek()
			// Reject `e[]` reads early with a helpful message - the
			// append form `$xs[] = item;` is statement-level only (see
			// tryParseIndexAssign); any other `[]` is meaningless.
			if p.peekN(1).Type == lexer.TOKEN_RBRACKET {
				return nil, &ParseError{
					Msg:  "`[]` is the M9 append form and only valid as a write target: `$xs[] = item;`",
					File: bra.File, Line: bra.Line, Col: bra.Col,
				}
			}
			p.advance() // consume [
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TOKEN_RBRACKET, "to close index expression"); err != nil {
				return nil, err
			}
			e = &IndexExpr{
				pos:    pos{File: bra.File, Line: bra.Line, Col: bra.Col},
				Target: e,
				Index:  idx,
			}
		case lexer.TOKEN_DOT:
			// Struct field access `e.field`. The qualified-call
			// path that takes IDENT.IDENT is handled in parsePrimaryAtom
			// before we get here; this branch only fires after a
			// non-IDENT atom or further down a chain like `$p.q.r`.
			dot := p.peek()
			p.advance() // consume .
			name, err := p.expectFieldName("after `.` for field name")
			if err != nil {
				return nil, err
			}
			e = &FieldAccessExpr{
				pos:    pos{File: dot.File, Line: dot.Line, Col: dot.Col},
				Target: e,
				Field:  name.Lexeme,
			}
		default:
			return e, nil
		}
	}
}

func (p *parser) parsePrimaryAtom() (Expr, error) {
	// A seeded primary (from parseExprFrom) is returned once, consuming no
	// tokens; parsePrimary's postfix loop then continues any further chain.
	if p.seed != nil {
		s := p.seed
		p.seed = nil
		return s, nil
	}
	t := p.peek()
	switch t.Type {
	case lexer.TOKEN_INT:
		p.advance()
		// Literal may be prefixed `0x`/`0o`/`0b` for non-decimal
		// bases. Pass base=0 to ParseInt so it picks the base from the
		// prefix (or 10 if none). 64-bit signed range applies in every
		// base.
		lex := t.Lexeme
		base := 0
		if !strings.HasPrefix(lex, "0x") && !strings.HasPrefix(lex, "0o") && !strings.HasPrefix(lex, "0b") {
			base = 10
		}
		n, err := strconv.ParseInt(lex, base, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid int literal %q: %v", t.Lexeme, err), File: t.File, Line: t.Line, Col: t.Col}
		}
		return &IntLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Value: n}, nil
	case lexer.TOKEN_FLOAT:
		p.advance()
		f, err := strconv.ParseFloat(t.Lexeme, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid float literal %q: %v", t.Lexeme, err), File: t.File, Line: t.Line, Col: t.Col}
		}
		return &FloatLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Value: f}, nil
	case lexer.TOKEN_STRING:
		p.advance()
		return &StringLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Value: t.Lexeme}, nil
	case lexer.TOKEN_TRUE:
		p.advance()
		return &BoolLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Value: true}, nil
	case lexer.TOKEN_FALSE:
		p.advance()
		return &BoolLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Value: false}, nil
	case lexer.TOKEN_NULL:
		p.advance()
		return &NullLit{pos: pos{File: t.File, Line: t.Line, Col: t.Col}}, nil
	case lexer.TOKEN_LEN:
		// `len(EXPR)` is a language built-in primary, not a
		// library function. Parses as a fixed shape with exactly one
		// argument; any other arity is a parse-time error rather than
		// the historical runtime arity check.
		p.advance()
		if _, err := p.expect(lexer.TOKEN_LPAREN, "after `len`"); err != nil {
			return nil, err
		}
		operand, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TOKEN_RPAREN, "to close `len` argument"); err != nil {
			return nil, err
		}
		return &LenExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Operand: operand}, nil
	case lexer.TOKEN_SPAWN:
		// `spawn { body }` is a block primary expression that
		// returns a `task of T`. The body is a statement list - same
		// shape as a method body or a `try` block. Phase 1 runs the
		// body inline; Phase 2 lowers it to a goroutine.
		p.advance()
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &SpawnExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Body: body.Stmts}, nil
	case lexer.TOKEN_TASK:
		// `task` is primarily a type keyword (`task of T`),
		// but the `task.wait` / `task.poll` / ... library exposes
		// builtins behind the `task.` namespace prefix at call sites.
		// In expression position, `task.IDENT` parses as a qualified
		// call or constant reference with prefix `"task"`. Anything
		// else after `task` in expression position is a syntax error
		// pointing the user at the two valid uses.
		p.advance()
		if !p.check(lexer.TOKEN_DOT) {
			return nil, &ParseError{
				Msg:  "`task` in expression position must be followed by `.method(...)` (the `task` library); for the type, write `task of T` in a declaration",
				File: t.File, Line: t.Line, Col: t.Col,
			}
		}
		// Manufacture a synthetic IDENT token for the qualified-tail
		// parser; its Lexeme is what becomes the QualifiedCall's
		// Prefix string.
		ident := lexer.Token{Type: lexer.TOKEN_IDENT, Lexeme: "task", File: t.File, Line: t.Line, Col: t.Col}
		return p.parseQualifiedTail(ident)
	case lexer.TOKEN_VARREF:
		p.advance()
		return &VarExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Name: t.Lexeme, Depth: -1, Slot: -1}, nil
	case lexer.TOKEN_IDENT:
		// A bare IDENT in expression context is either:
		//   - a qualified call/constant `prefix.name(...)` / `prefix.NAME`,
		//   - a function call `name(...)`,
		//   - a struct literal `Name{ field: expr, ... }`,
		//   - or a constant reference `MAX`.
		// The token immediately after the IDENT decides.
		p.advance()
		if p.check(lexer.TOKEN_DOT) {
			return p.parseQualifiedTail(t)
		}
		if p.check(lexer.TOKEN_LBRACE) {
			return p.parseStructLitTail(t)
		}
		if !p.check(lexer.TOKEN_LPAREN) {
			return &ConstRefExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Name: t.Lexeme, Depth: -1, Slot: -1}, nil
		}
		return p.parseCallTail(t)
	case lexer.TOKEN_INT_TYPE, lexer.TOKEN_FLOAT_TYPE, lexer.TOKEN_STRING_TYPE, lexer.TOKEN_BOOL_TYPE, lexer.TOKEN_BYTES_TYPE:
		// Type keywords have no expression-position meaning. The
		// `int(v)` / `float(v)` / `string(v)` / `bool(v)` conversion-call
		// shortcut lives in the convert library under names that avoid
		// the keyword collision: `convert.toInt(v)`, `convert.toFloat(v)`,
		// `convert.toString(v)`, `convert.toBool(v)`. The `bytes`
		// type doesn't take a single-arg conversion (it ships
		// `convert.bytesFromString(s, codec)` instead); rejecting it
		// here keeps the type-keyword treatment uniform.
		hint := ""
		if p.peekN(1).Type == lexer.TOKEN_LPAREN && t.Type != lexer.TOKEN_BYTES_TYPE {
			toName := "to" + strings.ToUpper(t.Lexeme[:1]) + t.Lexeme[1:]
			hint = fmt.Sprintf(" (the bare-call form was removed in M10; use `convert.%s(...)` instead)", toName)
		} else if t.Type == lexer.TOKEN_BYTES_TYPE {
			hint = " (use `convert.bytesFromString(s, codec)` to build bytes from a string)"
		} else {
			hint = " (type names belong after `as` in a declaration)"
		}
		return nil, &ParseError{
			Msg:  fmt.Sprintf("type name %q cannot appear in expression position%s", t.Lexeme, hint),
			File: t.File, Line: t.Line, Col: t.Col,
		}
	case lexer.TOKEN_LPAREN:
		p.advance()
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TOKEN_RPAREN, "to close grouped expression"); err != nil {
			return nil, err
		}
		return e, nil
	case lexer.TOKEN_LBRACKET:
		return p.parseListLit()
	case lexer.TOKEN_LBRACE:
		return p.parseMapLit()
	}
	return nil, &ParseError{Msg: fmt.Sprintf("unexpected token %s (%q) in expression", t.Type, t.Lexeme), File: t.File, Line: t.Line, Col: t.Col}
}

// parseListLit reads `[ expr, expr, ..., ]`. The trailing comma is
// allowed (consistent with multi-line struct/map literal habits in other
// languages, and makes diffs cleaner). Empty `[]` is legal and produces
// an empty list whose element type is decided by the enclosing context.
func (p *parser) parseListLit() (Expr, error) {
	bra, _ := p.match(lexer.TOKEN_LBRACKET)
	var elems []Expr
	for !p.check(lexer.TOKEN_RBRACKET) {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, e)
		if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
			break
		}
	}
	if _, err := p.expect(lexer.TOKEN_RBRACKET, "to close list literal"); err != nil {
		return nil, err
	}
	return &ListLit{pos: pos{File: bra.File, Line: bra.Line, Col: bra.Col}, Elements: elems}, nil
}

// parseMapLit reads `{ key: value, key: value, ..., }`. Trailing comma
// allowed. Empty `{}` produces an empty map whose key/value types come
// from the enclosing context.
//
// Care: `{` also starts blocks in statement position. parseMapLit is
// only reached through parsePrimary, which is called from expression
// context, so the ambiguity is already resolved by the caller.
func (p *parser) parseMapLit() (Expr, error) {
	brace, _ := p.match(lexer.TOKEN_LBRACE)
	var keys, vals []Expr
	for !p.check(lexer.TOKEN_RBRACE) {
		key, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TOKEN_COLON, "between map key and value"); err != nil {
			return nil, err
		}
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
		vals = append(vals, val)
		if _, ok := p.match(lexer.TOKEN_COMMA); !ok {
			break
		}
	}
	if _, err := p.expect(lexer.TOKEN_RBRACE, "to close map literal"); err != nil {
		return nil, err
	}
	return &MapLit{pos: pos{File: brace.File, Line: brace.Line, Col: brace.Col}, Keys: keys, Values: vals}, nil
}
