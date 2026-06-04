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

// parseImport parses a `use NAME;` library import. (File imports are handled
// by the preprocessor and never reach the parser.)
func (p *parser) parseImport() (*ImportStmt, error) {
	use, _ := p.match(lexer.TOKEN_USE)
	name, err := p.expect(lexer.TOKEN_IDENT, "after `use`")
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_SEMI, "after `use` statement"); err != nil {
		return nil, err
	}
	return &ImportStmt{pos: pos{Line: use.Line, Col: use.Col}, Name: name.Lexeme}, nil
}

func (p *parser) parseMethodDef() (*MethodDef, error) {
	def, _ := p.match(lexer.TOKEN_FUNC)
	name, err := p.expect(lexer.TOKEN_IDENT, "after `func`")
	if err != nil {
		return nil, err
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
					Line: v.Line, Col: v.Col,
				}
			}
			pname, err := p.expect(lexer.TOKEN_IDENT, "for parameter name")
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TOKEN_AS, "after parameter name"); err != nil {
				return nil, err
			}
			ptype, err := p.parseType()
			if err != nil {
				return nil, err
			}
			params = append(params, Param{Name: pname.Lexeme, Type: ptype, Line: pname.Line, Col: pname.Col})
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
	return &MethodDef{pos: pos{Line: def.Line, Col: def.Col}, Name: name.Lexeme, Params: params, Body: body}, nil
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
	switch p.peek().Type {
	case lexer.TOKEN_DEFINE:
		return p.parseDefineLike()
	case lexer.TOKEN_FUNC:
		t := p.peek()
		return nil, &ParseError{Msg: "methods can only be defined at the top level", Line: t.Line, Col: t.Col}
	case lexer.TOKEN_IF:
		return p.parseIf()
	case lexer.TOKEN_WHILE:
		return p.parseWhile()
	case lexer.TOKEN_FOR:
		return p.parseFor()
	case lexer.TOKEN_RETURN:
		return p.parseReturn()
	case lexer.TOKEN_VARREF:
		// `$x = expr ;` is an assignment; anything else starting with $x
		// is an expression statement (rare, but possible if VARREF is used
		// in a call - which it currently isn't - kept open for safety).
		if p.peekN(1).Type == lexer.TOKEN_ASSIGN {
			return p.parseAssign(true)
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

// parseDefineLike handles `def` statements:
//   def NAME as T;
//   def NAME as T init EXPR;
//   def const NAME as T init EXPR;
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
			Line: v.Line, Col: v.Col,
		}
	}
	if isConst {
		// constant: name is an IDENT, must be uppercase [A-Z]+
		name, err := p.expect(lexer.TOKEN_IDENT, "after `const`")
		if err != nil {
			return nil, err
		}
		if !isUpperOnly(name.Lexeme) {
			return nil, &ParseError{Msg: fmt.Sprintf("constant name %q must use [A-Z] only", name.Lexeme), Line: name.Line, Col: name.Col}
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
			pos:      pos{Line: def.Line, Col: def.Col},
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
		pos:      pos{Line: def.Line, Col: def.Col},
		VarName:  name.Lexeme,
		VarType:  tt,
		InitExpr: initExpr,
	}, nil
}

// parseReturn parses `return;` or `return EXPR;`.
func (p *parser) parseReturn() (Stmt, error) {
	ret, _ := p.match(lexer.TOKEN_RETURN)
	stmt := &ReturnStmt{pos: pos{Line: ret.Line, Col: ret.Col}}
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
	return &AssignStmt{pos: pos{Line: vref.Line, Col: vref.Col}, VarName: vref.Lexeme, Value: val}, nil
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
	stmt := &IfStmt{pos: pos{Line: ift.Line, Col: ift.Col}, Cond: cond, Then: body}
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
	return &WhileStmt{pos: pos{Line: wt.Line, Col: wt.Col}, Cond: cond, Body: body}, nil
}

func (p *parser) parseFor() (Stmt, error) {
	ft, _ := p.match(lexer.TOKEN_FOR)
	if _, err := p.expect(lexer.TOKEN_LPAREN, "after `for`"); err != nil {
		return nil, err
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
		return nil, &ParseError{Msg: fmt.Sprintf("expected for-init (define or assignment), got %s (%q)", t.Type, t.Lexeme), Line: t.Line, Col: t.Col}
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
		return nil, &ParseError{Msg: fmt.Sprintf("expected for-step (assignment) or `)`, got %s (%q)", t.Type, t.Lexeme), Line: t.Line, Col: t.Col}
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN, "to close for-header"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ForStmt{pos: pos{Line: ft.Line, Col: ft.Col}, Init: initStmt, Cond: cond, Step: step, Body: body}, nil
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

func (p *parser) parseType() (Type, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TOKEN_INT_TYPE:
		p.advance()
		return TypeInt, nil
	case lexer.TOKEN_FLOAT_TYPE:
		p.advance()
		return TypeFloat, nil
	case lexer.TOKEN_STRING_TYPE:
		p.advance()
		return TypeString, nil
	case lexer.TOKEN_BOOL_TYPE:
		p.advance()
		return TypeBool, nil
	case lexer.TOKEN_NULL:
		p.advance()
		return TypeNull, nil
	}
	return TypeInvalid, &ParseError{Msg: fmt.Sprintf("expected type, got %s (%q)", t.Type, t.Lexeme), Line: t.Line, Col: t.Col}
}

func isUpperOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
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
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: OpOr, Left: left, Right: right}
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
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: OpAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (Expr, error) {
	if t, ok := p.match(lexer.TOKEN_NOT); ok {
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{pos: pos{Line: t.Line, Col: t.Col}, Op: OpNot, Operand: operand}, nil
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
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: op, Left: left, Right: right}
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
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: op, Left: left, Right: right}
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
		left = &BinaryExpr{pos: pos{Line: l, Col: c}, Op: op, Left: left, Right: right}
	}
}

// parseCallTail parses the `(args...)` portion of a call expression. The
// callee token (already consumed) is passed in so we can attach the right
// position and callee lexeme.
func (p *parser) parseCallTail(callee lexer.Token) (Expr, error) {
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
	return &CallExpr{pos: pos{Line: callee.Line, Col: callee.Col}, Callee: callee.Lexeme, Args: args}, nil
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
		return &UnaryExpr{pos: pos{Line: t.Line, Col: t.Col}, Op: OpNeg, Operand: operand}, nil
	}
	return p.parsePrimary()
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
	case lexer.TOKEN_FLOAT:
		p.advance()
		f, err := strconv.ParseFloat(t.Lexeme, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid float literal %q: %v", t.Lexeme, err), Line: t.Line, Col: t.Col}
		}
		return &FloatLit{pos: pos{Line: t.Line, Col: t.Col}, Value: f}, nil
	case lexer.TOKEN_STRING:
		p.advance()
		return &StringLit{pos: pos{Line: t.Line, Col: t.Col}, Value: t.Lexeme}, nil
	case lexer.TOKEN_TRUE:
		p.advance()
		return &BoolLit{pos: pos{Line: t.Line, Col: t.Col}, Value: true}, nil
	case lexer.TOKEN_FALSE:
		p.advance()
		return &BoolLit{pos: pos{Line: t.Line, Col: t.Col}, Value: false}, nil
	case lexer.TOKEN_NULL:
		p.advance()
		return &NullLit{pos: pos{Line: t.Line, Col: t.Col}}, nil
	case lexer.TOKEN_VARREF:
		p.advance()
		return &VarExpr{pos: pos{Line: t.Line, Col: t.Col}, Name: t.Lexeme}, nil
	case lexer.TOKEN_IDENT:
		// A bare IDENT in expression context is either a function call
		// (`name(...)`) or a constant reference (`MAX`). Look at the next
		// token to decide.
		p.advance()
		if !p.check(lexer.TOKEN_LPAREN) {
			return &ConstRefExpr{pos: pos{Line: t.Line, Col: t.Col}, Name: t.Lexeme}, nil
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
				Line: t.Line, Col: t.Col,
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
	}
	return nil, &ParseError{Msg: fmt.Sprintf("unexpected token %s (%q) in expression", t.Type, t.Lexeme), Line: t.Line, Col: t.Col}
}
