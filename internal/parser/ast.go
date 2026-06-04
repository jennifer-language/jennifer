// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import "fmt"

// Node is the root interface for all AST nodes. Pos returns the source line/col where the node starts.
type Node interface {
	Pos() (line, col int)
	astNode()
}

type Stmt interface {
	Node
	stmtNode()
}

type Expr interface {
	Node
	exprNode()
}

// pos carries source position; embedded into every node.
type pos struct {
	Line int
	Col  int
}

func (p pos) Pos() (int, int) { return p.Line, p.Col }
func (p pos) astNode()        {}

// Type is the declared static type of a variable or constant.
type Type int

const (
	TypeInvalid Type = iota
	TypeInt
	TypeString
)

func (t Type) String() string {
	switch t {
	case TypeInt:
		return "int"
	case TypeString:
		return "string"
	default:
		return "<invalid>"
	}
}

// ---- Top-level program ----

type Program struct {
	pos
	Imports []*ImportStmt
	Methods []*MethodDef
}

// ---- Statements ----

type ImportStmt struct {
	pos
	Name string
}

func (*ImportStmt) stmtNode() {}

type MethodDef struct {
	pos
	Name string
	Body *Block
}

func (*MethodDef) stmtNode() {}

type Block struct {
	pos
	Stmts []Stmt
}

func (*Block) stmtNode() {}

// DefineStmt: `define $x as int init <expr>;`
// M1 requires `init`; later milestones will allow defining without an initializer.
type DefineStmt struct {
	pos
	VarName  string
	VarType  Type
	InitExpr Expr // never nil in M1
}

func (*DefineStmt) stmtNode() {}

// ExprStmt: a bare expression terminated by `;` (used for calls like `printf(...)`).
type ExprStmt struct {
	pos
	Expr Expr
}

func (*ExprStmt) stmtNode() {}

// ---- Expressions ----

type IntLit struct {
	pos
	Value int64
}

func (*IntLit) exprNode() {}

type StringLit struct {
	pos
	Value string
}

func (*StringLit) exprNode() {}

type VarExpr struct {
	pos
	Name string // without the leading $
}

func (*VarExpr) exprNode() {}

type CallExpr struct {
	pos
	Callee string
	Args   []Expr
}

func (*CallExpr) exprNode() {}

type BinaryOp int

const (
	OpAdd BinaryOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
)

func (o BinaryOp) String() string {
	switch o {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	}
	return "?"
}

type BinaryExpr struct {
	pos
	Op       BinaryOp
	Left     Expr
	Right    Expr
}

func (*BinaryExpr) exprNode() {}

// Sprint produces a stable, readable representation of any AST node - used in tests.
func Sprint(n Node) string {
	switch v := n.(type) {
	case *Program:
		s := "Program{"
		for _, im := range v.Imports {
			s += Sprint(im) + " "
		}
		for _, m := range v.Methods {
			s += Sprint(m) + " "
		}
		return s + "}"
	case *ImportStmt:
		return fmt.Sprintf("Import(%s)", v.Name)
	case *MethodDef:
		return fmt.Sprintf("Method(%s, %s)", v.Name, Sprint(v.Body))
	case *Block:
		s := "Block["
		for i, st := range v.Stmts {
			if i > 0 {
				s += "; "
			}
			s += Sprint(st)
		}
		return s + "]"
	case *DefineStmt:
		return fmt.Sprintf("Define($%s as %s = %s)", v.VarName, v.VarType, Sprint(v.InitExpr))
	case *ExprStmt:
		return fmt.Sprintf("ExprStmt(%s)", Sprint(v.Expr))
	case *IntLit:
		return fmt.Sprintf("Int(%d)", v.Value)
	case *StringLit:
		return fmt.Sprintf("Str(%q)", v.Value)
	case *VarExpr:
		return fmt.Sprintf("Var($%s)", v.Name)
	case *CallExpr:
		s := fmt.Sprintf("Call(%s", v.Callee)
		for _, a := range v.Args {
			s += ", " + Sprint(a)
		}
		return s + ")"
	case *BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", Sprint(v.Left), v.Op, Sprint(v.Right))
	}
	return fmt.Sprintf("<unknown %T>", n)
}
