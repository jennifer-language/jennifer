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
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at %d:%d: %s", e.Line, e.Col, e.Msg)
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

func (p *parser) peek() lexer.Token         { return p.tokens[p.pos] }
func (p *parser) peekN(n int) lexer.Token   { return p.tokens[p.pos+n] }
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
			Line: t.Line, Col: t.Col,
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
		case lexer.TOKEN_IMPORT:
			imp, err := p.parseImport()
			if err != nil {
				return nil, err
			}
			prog.Imports = append(prog.Imports, imp)
		case lexer.TOKEN_DEFINE:
			// `define`/`def` introduces either a method (followed by IDENT)
			// or a variable (followed by VARREF). Variable defines aren't
			// allowed at the top level in M1 - that's global state and we
			// don't have an evaluation strategy for it yet.
			next := p.peekN(1)
			if next.Type == lexer.TOKEN_IDENT {
				m, err := p.parseMethodDef()
				if err != nil {
					return nil, err
				}
				prog.Methods = append(prog.Methods, m)
				continue
			}
			if next.Type == lexer.TOKEN_VARREF {
				return nil, &ParseError{
					Msg:  "variable definitions are not allowed at the top level (M1)",
					Line: t.Line, Col: t.Col,
				}
			}
			return nil, &ParseError{
				Msg:  fmt.Sprintf("expected method name or variable reference after `%s`, got %s (%q)", t.Lexeme, next.Type, next.Lexeme),
				Line: next.Line, Col: next.Col,
			}
		default:
			return nil, &ParseError{
				Msg:  fmt.Sprintf("expected `import`, `def`, or `define` at top level, got %s (%q)", t.Type, t.Lexeme),
				Line: t.Line, Col: t.Col,
			}
		}
	}
}

func (p *parser) parseImport() (*ImportStmt, error) {
	imp, _ := p.match(lexer.TOKEN_IMPORT)
	name, err := p.expect(lexer.TOKEN_IDENT, "after `import`")
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "after import statement"); err != nil {
		return nil, err
	}
	return &ImportStmt{pos: pos{Line: imp.Line, Col: imp.Col}, Name: name.Lexeme}, nil
}

func (p *parser) parseMethodDef() (*MethodDef, error) {
	def, _ := p.match(lexer.TOKEN_DEFINE)
	name, err := p.expect(lexer.TOKEN_IDENT, "after `def`/`define`")
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_LPAREN, "after method name"); err != nil {
		return nil, err
	}
	// M1: no parameters yet
	if _, err := p.expect(lexer.TOKEN_RPAREN, "(M1 only supports zero-arg methods)"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &MethodDef{pos: pos{Line: def.Line, Col: def.Col}, Name: name.Lexeme, Body: body}, nil
}

func (p *parser) parseBlock() (*Block, error) {
	lb, err := p.expect(lexer.TOKEN_LBRACE, "to begin block")
	if err != nil {
		return nil, err
	}
	block := &Block{pos: pos{Line: lb.Line, Col: lb.Col}}
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
	if p.check(lexer.TOKEN_DEFINE) {
		// Disambiguate variable vs. method definition.
		// M1 only allows variable definitions inside blocks.
		next := p.peekN(1)
		if next.Type == lexer.TOKEN_VARREF {
			return p.parseDefine()
		}
		if next.Type == lexer.TOKEN_IDENT {
			t := p.peek()
			return nil, &ParseError{
				Msg:  "methods can only be defined at the top level (M1)",
				Line: t.Line, Col: t.Col,
			}
		}
		t := p.peek()
		return nil, &ParseError{
			Msg:  fmt.Sprintf("expected variable reference after `%s`, got %s (%q)", t.Lexeme, next.Type, next.Lexeme),
			Line: next.Line, Col: next.Col,
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
	return &ExprStmt{pos: pos{Line: start.Line, Col: start.Col}, Expr: expr}, nil
}

func (p *parser) parseDefine() (Stmt, error) {
	def, _ := p.match(lexer.TOKEN_DEFINE)
	vref, err := p.expect(lexer.TOKEN_VARREF, "after `define`")
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_AS, "after variable name"); err != nil {
		return nil, err
	}
	tt, err := p.parseType()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_INIT, "(M1 requires `init` initializer)"); err != nil {
		return nil, err
	}
	initExpr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "to terminate define"); err != nil {
		return nil, err
	}
	return &DefineStmt{
		pos:      pos{Line: def.Line, Col: def.Col},
		VarName:  vref.Lexeme,
		VarType:  tt,
		InitExpr: initExpr,
	}, nil
}

func (p *parser) parseType() (Type, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TOKEN_INT_TYPE:
		p.advance()
		return TypeInt, nil
	case lexer.TOKEN_STRING_TYPE:
		p.advance()
		return TypeString, nil
	}
	return TypeInvalid, &ParseError{Msg: fmt.Sprintf("expected type, got %s (%q)", t.Type, t.Lexeme), Line: t.Line, Col: t.Col}
}

func (p *parser) parseExpr() (Expr, error) {
	return p.parseAdd()
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
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseMul() (Expr, error) {
	left, err := p.parsePrimary()
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
		case lexer.TOKEN_PERCENT:
			op = OpMod
		default:
			return left, nil
		}
		p.advance()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		l, c := left.Pos()
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parsePrimary() (Expr, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TOKEN_INT:
		p.advance()
		n, err := strconv.ParseInt(t.Lexeme, 10, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid int literal %q: %v", t.Lexeme, err), Line: t.Line, Col: t.Col}
		}
		return &IntLit{pos: pos{Line: t.Line, Col: t.Col}, Value: n}, nil
	case lexer.TOKEN_STRING:
		p.advance()
		return &StringLit{pos: pos{Line: t.Line, Col: t.Col}, Value: t.Lexeme}, nil
	case lexer.TOKEN_VARREF:
		p.advance()
		return &VarExpr{pos: pos{Line: t.Line, Col: t.Col}, Name: t.Lexeme}, nil
	case lexer.TOKEN_IDENT:
		// function call: ident "(" args ")"
		p.advance()
		if _, err := p.expect(lexer.TOKEN_LPAREN, "after function name"); err != nil {
			return nil, err
		}
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
		return &CallExpr{pos: pos{Line: t.Line, Col: t.Col}, Callee: t.Lexeme, Args: args}, nil
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
	}
	return nil, &ParseError{Msg: fmt.Sprintf("unexpected token %s (%q) in expression", t.Type, t.Lexeme), Line: t.Line, Col: t.Col}
}
