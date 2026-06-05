// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import "fmt"

// Node is the root interface for all AST nodes. Pos returns the source line/col
// where the node starts; Filename returns the source file (or "" if unknown).
type Node interface {
	Pos() (line, col int)
	Filename() string
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
	File string
	Line int
	Col  int
}

func (p pos) Pos() (int, int)  { return p.Line, p.Col }
func (p pos) Filename() string { return p.File }
func (p pos) astNode()         {}

// Type is the declared static type of a variable or constant.
type Type int

const (
	TypeInvalid Type = iota
	TypeInt
	TypeFloat
	TypeString
	TypeBool
	TypeNull
)

func (t Type) String() string {
	switch t {
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeString:
		return "string"
	case TypeBool:
		return "bool"
	case TypeNull:
		return "null"
	default:
		return "<invalid>"
	}
}

// ---- Top-level program ----

type Program struct {
	pos
	Imports  []*ImportStmt
	Methods  []*MethodDef
	TopLevel []Stmt // top-level statements executed in source order after method hoisting
}

// ---- Statements ----

type ImportStmt struct {
	pos
	Name string
}

func (*ImportStmt) stmtNode() {}

// Param is one formal parameter of a method.
type Param struct {
	Name string
	Type Type
	File string
	Line int
	Col  int
}

type MethodDef struct {
	pos
	Name   string
	Params []Param
	Body   *Block
}

func (*MethodDef) stmtNode() {}

type Block struct {
	pos
	Stmts []Stmt
}

func (*Block) stmtNode() {}

// DefineStmt: `define $x as int [init <expr>];` or `define const NAME as int init <expr>;`
// InitExpr is nil for uninitialized variables (interpreter uses ZeroFor in that case).
// Constants always have InitExpr != nil (the parser enforces this).
type DefineStmt struct {
	pos
	IsConst  bool
	VarName  string // for variables: the $-name; for constants: the UPPERCASE name
	VarType  Type
	InitExpr Expr // may be nil for uninitialized variables; never nil for constants
}

func (*DefineStmt) stmtNode() {}

// AssignStmt: `$x = <expr>;`
type AssignStmt struct {
	pos
	VarName string
	Value   Expr
}

func (*AssignStmt) stmtNode() {}

// IfStmt: `if (cond) { body } [elseif (cond) { body }]* [else { body }]?`
// ElseIfs is parallel to ElseIfBodies. Else may be nil.
type IfStmt struct {
	pos
	Cond         Expr
	Then         *Block
	ElseIfs      []Expr
	ElseIfBodies []*Block
	Else         *Block // nil if absent
}

func (*IfStmt) stmtNode() {}

// WhileStmt: `while (cond) { body }`
type WhileStmt struct {
	pos
	Cond Expr
	Body *Block
}

func (*WhileStmt) stmtNode() {}

// ForStmt: `for (init; cond; step) { body }`
// Init may be a DefineStmt or an AssignStmt; Step is an AssignStmt.
// Any field may be nil (e.g. `for (; cond ;) { ... }` would mean empty init/step).
type ForStmt struct {
	pos
	Init Stmt // *DefineStmt or *AssignStmt, or nil
	Cond Expr // must produce bool; nil means "true forever"
	Step Stmt // *AssignStmt or nil
	Body *Block
}

func (*ForStmt) stmtNode() {}

// ReturnStmt: `return;` (returns null) or `return EXPR;`.
type ReturnStmt struct {
	pos
	Value Expr // nil for bare `return;`
}

func (*ReturnStmt) stmtNode() {}

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

type FloatLit struct {
	pos
	Value float64
}

func (*FloatLit) exprNode() {}

type StringLit struct {
	pos
	Value string
}

func (*StringLit) exprNode() {}

type BoolLit struct {
	pos
	Value bool
}

func (*BoolLit) exprNode() {}

type NullLit struct {
	pos
}

func (*NullLit) exprNode() {}

type VarExpr struct {
	pos
	Name string // without the leading $
}

func (*VarExpr) exprNode() {}

// ConstRefExpr is a bare-identifier reference in an expression context
// (e.g. `printf(MAX)`). The interpreter resolves it to a constant in scope.
// A bare-identifier reference to a variable is rejected at runtime with a hint
// to use the `$` sigil.
type ConstRefExpr struct {
	pos
	Name string
}

func (*ConstRefExpr) exprNode() {}

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
	OpDiv      // `/` - always returns float
	OpFloorDiv // `div` - floor division (int//int -> int; involves float -> float floor)
	OpMod
	OpLt
	OpGt
	OpLe
	OpGe
	OpEq
	OpAnd
	OpOr
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
	case OpFloorDiv:
		return "div"
	case OpMod:
		return "%"
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpLe:
		return "<="
	case OpGe:
		return ">="
	case OpEq:
		return "=="
	case OpAnd:
		return "and"
	case OpOr:
		return "or"
	}
	return "?"
}

// IsComparison reports whether the op is a comparison (result type is bool).
func (o BinaryOp) IsComparison() bool {
	switch o {
	case OpLt, OpGt, OpLe, OpGe, OpEq:
		return true
	}
	return false
}

// IsLogical reports whether the op is a short-circuit logical operator.
func (o BinaryOp) IsLogical() bool {
	return o == OpAnd || o == OpOr
}

// UnaryOp identifies a prefix-unary operator.
type UnaryOp int

const (
	OpNeg UnaryOp = iota // -x  (numeric)
	OpNot                // not x  (bool)
)

func (o UnaryOp) String() string {
	switch o {
	case OpNeg:
		return "-"
	case OpNot:
		return "not"
	}
	return "?"
}

// UnaryExpr is a prefix-unary expression: `-EXPR` or `not EXPR`.
type UnaryExpr struct {
	pos
	Op      UnaryOp
	Operand Expr
}

func (*UnaryExpr) exprNode() {}

type BinaryExpr struct {
	pos
	Op    BinaryOp
	Left  Expr
	Right Expr
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
		for _, st := range v.TopLevel {
			s += Sprint(st) + " "
		}
		return s + "}"
	case *ImportStmt:
		return fmt.Sprintf("Import(%s)", v.Name)
	case *MethodDef:
		params := ""
		for i, p := range v.Params {
			if i > 0 {
				params += ", "
			}
			params += fmt.Sprintf("%s as %s", p.Name, p.Type)
		}
		return fmt.Sprintf("Method(%s(%s), %s)", v.Name, params, Sprint(v.Body))
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
		kind := "Define"
		if v.IsConst {
			kind = "Const"
		}
		name := "$" + v.VarName
		if v.IsConst {
			name = v.VarName
		}
		if v.InitExpr == nil {
			return fmt.Sprintf("%s(%s as %s)", kind, name, v.VarType)
		}
		return fmt.Sprintf("%s(%s as %s = %s)", kind, name, v.VarType, Sprint(v.InitExpr))
	case *AssignStmt:
		return fmt.Sprintf("Assign($%s = %s)", v.VarName, Sprint(v.Value))
	case *IfStmt:
		s := fmt.Sprintf("If(%s, %s", Sprint(v.Cond), Sprint(v.Then))
		for i, c := range v.ElseIfs {
			s += fmt.Sprintf(", ElseIf(%s, %s)", Sprint(c), Sprint(v.ElseIfBodies[i]))
		}
		if v.Else != nil {
			s += fmt.Sprintf(", Else(%s)", Sprint(v.Else))
		}
		return s + ")"
	case *WhileStmt:
		return fmt.Sprintf("While(%s, %s)", Sprint(v.Cond), Sprint(v.Body))
	case *ForStmt:
		initS := "<nil>"
		if v.Init != nil {
			initS = Sprint(v.Init)
		}
		condS := "<nil>"
		if v.Cond != nil {
			condS = Sprint(v.Cond)
		}
		stepS := "<nil>"
		if v.Step != nil {
			stepS = Sprint(v.Step)
		}
		return fmt.Sprintf("For(%s; %s; %s, %s)", initS, condS, stepS, Sprint(v.Body))
	case *ReturnStmt:
		if v.Value == nil {
			return "Return"
		}
		return fmt.Sprintf("Return(%s)", Sprint(v.Value))
	case *ExprStmt:
		return fmt.Sprintf("ExprStmt(%s)", Sprint(v.Expr))
	case *IntLit:
		return fmt.Sprintf("Int(%d)", v.Value)
	case *FloatLit:
		return fmt.Sprintf("Float(%g)", v.Value)
	case *StringLit:
		return fmt.Sprintf("Str(%q)", v.Value)
	case *BoolLit:
		return fmt.Sprintf("Bool(%t)", v.Value)
	case *NullLit:
		return "Null"
	case *VarExpr:
		return fmt.Sprintf("Var($%s)", v.Name)
	case *ConstRefExpr:
		return fmt.Sprintf("Const(%s)", v.Name)
	case *CallExpr:
		s := fmt.Sprintf("Call(%s", v.Callee)
		for _, a := range v.Args {
			s += ", " + Sprint(a)
		}
		return s + ")"
	case *BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", Sprint(v.Left), v.Op, Sprint(v.Right))
	case *UnaryExpr:
		return fmt.Sprintf("(%s %s)", v.Op, Sprint(v.Operand))
	}
	return fmt.Sprintf("<unknown %T>", n)
}
