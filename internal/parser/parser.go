// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import (
	"fmt"
	"strconv"

	"github.com/mplx/jennifer-lang/internal/lexer"
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

// Parse tokenizes the source and returns a *Program AST.
func Parse(source string) (*Program, error) {
	toks, err := lexer.Tokenize(source)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: toks}
	return p.parseProgram()
}

// ParseTokens parses an already-lexed token stream.
func ParseTokens(toks []lexer.Token) (*Program, error) {
	p := &parser{tokens: toks}
	return p.parseProgram()
}

type parser struct {
	tokens []lexer.Token
	pos    int
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

// ---- Grammar (M1) ----
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
//   unary       := primary           (M1: no prefix operators yet)
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
		case lexer.TOKEN_FUNC:
			// `func NAME() { ... }` - methods are hoisted so they can be
			// called regardless of textual order.
			m, err := p.parseMethodDef()
			if err != nil {
				return nil, err
			}
			prog.Methods = append(prog.Methods, m)
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
func (p *parser) parseImport() (*ImportStmt, error) {
	use, _ := p.match(lexer.TOKEN_USE)
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
	case lexer.TOKEN_RETURN:
		return p.parseReturn()
	case lexer.TOKEN_VARREF:
		// `$x = expr ;` is a simple assignment.
		if p.peekN(1).Type == lexer.TOKEN_ASSIGN {
			return p.parseAssign(true)
		}
		// `$xs[...] = expr ;` (or chained `$xs[i][j] = ...`) is an
		// index-assignment. We can't decide until we see the `=` after
		// the lvalue chain, so we save the parser position, attempt the
		// chain, and either commit (if `=` follows) or restore and fall
		// through to the expression-statement path below.
		if p.peekN(1).Type == lexer.TOKEN_LBRACKET {
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

// tryParseIndexAssign attempts to parse `$xs[i]...[j] = expr ;` as an
// IndexAssignStmt, or `$xs[] = expr ;` as an AppendStmt (M9).
// Returns (stmt, true, nil) on success. Returns (nil, false, nil) when
// the lvalue chain parses but no `=` follows (meaning the original
// token stream was an expression statement, not an assignment) - in
// that case the parser position is restored to the VARREF so the
// caller can re-parse it as an expression. Errors during the lvalue
// chain itself are propagated.
//
// The append form `$xs[] = expr ;` is detected first because it can't
// be mistaken for anything else: an empty index between a bare VARREF
// and `=` has no read meaning (you can't read past-the-end), so any
// occurrence is an unambiguous append.
func (p *parser) tryParseIndexAssign() (Stmt, bool, error) {
	saved := p.pos
	vref, _ := p.match(lexer.TOKEN_VARREF)
	// Detect the M9 append form `$xs[] = expr ;` before walking any
	// index chain - chained appends like `$xs[0][] = ...` aren't part
	// of M9 and would just confuse the chain-parsing loop below.
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
			Target: &VarExpr{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, Name: vref.Lexeme},
			Value:  val,
		}, true, nil
	}
	var target Expr = &VarExpr{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, Name: vref.Lexeme}
	// Consume one or more `[expr]` suffixes.
	for p.check(lexer.TOKEN_LBRACKET) {
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
	}
	if _, ok := p.match(lexer.TOKEN_ASSIGN); !ok {
		// Not an assignment; let the expression-statement path try again.
		p.pos = saved
		return nil, false, nil
	}
	val, err := p.parseExpr()
	if err != nil {
		return nil, false, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate assignment"); err != nil {
		return nil, false, err
	}
	idx, ok := target.(*IndexExpr)
	if !ok {
		// Defensive: tryParseIndexAssign is only entered when LBRACKET
		// follows the VARREF, so target should always end up an IndexExpr.
		return nil, false, &ParseError{Msg: "expected index expression on left of `=`", File: vref.File, Line: vref.Line, Col: vref.Col}
	}
	return &IndexAssignStmt{
		pos:    pos{File: vref.File, Line: vref.Line, Col: vref.Col},
		Target: idx,
		Value:  val,
	}, true, nil
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
	return &AssignStmt{pos: pos{File: vref.File, Line: vref.Line, Col: vref.Col}, VarName: vref.Lexeme, Value: val}, nil
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
		pos:     pos{File: ft.File, Line: ft.Line, Col: ft.Col},
		VarName: name.Lexeme,
		Coll:    coll,
		Body:    body,
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

func (p *parser) parseExpr() (Expr, error) {
	return p.parseOr()
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
func (p *parser) parseComparison() (Expr, error) {
	left, err := p.parseAdd()
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
	name, err := p.expect(lexer.TOKEN_IDENT, "after `.` in qualified call")
	if err != nil {
		return nil, err
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
	// Qualified constant reference: prefix.NAME. Same constant-name rule
	// as bare constants - uppercase chunks separated by single `_`.
	if !isValidConstName(name.Lexeme) {
		return nil, &ParseError{Msg: fmt.Sprintf("qualified constant name %q must be uppercase [A-Z]+ with single `_` separators", name.Lexeme), File: name.File, Line: name.Line, Col: name.Col}
	}
	return &QualifiedConstRefExpr{
		pos:    pos{File: prefix.File, Line: prefix.Line, Col: prefix.Col},
		Prefix: prefix.Lexeme,
		Name:   name.Lexeme,
	}, nil
}

// parseUnaryMinus handles the `-EXPR` prefix form. It sits between
// multiplicative and primary so that `-x * 2` parses as `(-x) * 2`.
// Right-associative: `--x` is `-(-x)`.
func (p *parser) parseUnaryMinus() (Expr, error) {
	if t, ok := p.match(lexer.TOKEN_MINUS); ok {
		operand, err := p.parseUnaryMinus()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Op: OpNeg, Operand: operand}, nil
	}
	return p.parsePrimary()
}

// parsePrimary parses an atom (literal, var ref, call, grouped expr,
// list/map literal) and then chains any number of `[index]` suffixes
// onto it. Returning the chained form from `primary` lets every level
// above (unary, mul, add, ...) treat `$xs[0]` exactly like any other
// expression without special-casing.
func (p *parser) parsePrimary() (Expr, error) {
	e, err := p.parsePrimaryAtom()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TOKEN_LBRACKET) {
		bra := p.peek()
		// Reject `e[]` reads early with a helpful message - the append form
		// `$xs[] = item;` is statement-level only (see tryParseIndexAssign);
		// any other `[]` is meaningless.
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
	}
	return e, nil
}

func (p *parser) parsePrimaryAtom() (Expr, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TOKEN_INT:
		p.advance()
		n, err := strconv.ParseInt(t.Lexeme, 10, 64)
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
	case lexer.TOKEN_VARREF:
		p.advance()
		return &VarExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Name: t.Lexeme}, nil
	case lexer.TOKEN_IDENT:
		// A bare IDENT in expression context is either:
		//   - a qualified call/constant `prefix.name(...)` / `prefix.NAME`,
		//   - a function call `name(...)`,
		//   - or a constant reference `MAX`.
		// The token immediately after the IDENT decides.
		p.advance()
		if p.check(lexer.TOKEN_DOT) {
			return p.parseQualifiedTail(t)
		}
		if !p.check(lexer.TOKEN_LPAREN) {
			return &ConstRefExpr{pos: pos{File: t.File, Line: t.Line, Col: t.Col}, Name: t.Lexeme}, nil
		}
		return p.parseCallTail(t)
	case lexer.TOKEN_INT_TYPE, lexer.TOKEN_FLOAT_TYPE, lexer.TOKEN_STRING_TYPE, lexer.TOKEN_BOOL_TYPE:
		// Type-name-as-function: `int(v)`, `float(v)`, `string(v)`, `bool(v)`.
		// These are calls to the `convert` library. Only valid here if
		// immediately followed by `(` - bare type keywords are still type
		// references handled by parseType.
		if p.peekN(1).Type != lexer.TOKEN_LPAREN {
			return nil, &ParseError{
				Msg:  fmt.Sprintf("type name %q can only appear in expression position when called as a conversion: %s(...)", t.Lexeme, t.Lexeme),
				File: t.File, Line: t.Line, Col: t.Col,
			}
		}
		p.advance() // consume the type keyword
		return p.parseCallTail(t)
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
