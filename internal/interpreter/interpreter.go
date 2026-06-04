// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"
	"io"
	"os"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// Builtin is a Go-implemented stdlib function callable from Jennifer source.
// `out` is where stdout-like effects (e.g. printf) write; the interpreter passes
// its configured writer in. Returning Null() for void-like calls is fine.
type Builtin func(out io.Writer, args []Value) (Value, error)

// Interpreter walks a parsed Program and runs it.
type Interpreter struct {
	Out      io.Writer       // defaults to os.Stdout if nil
	Builtins map[string]Builtin
	imported map[string]bool // libraries the program has `import`ed
	methods  map[string]*parser.MethodDef
}

func New() *Interpreter {
	return &Interpreter{
		Out:      os.Stdout,
		Builtins: map[string]Builtin{},
		imported: map[string]bool{},
		methods:  map[string]*parser.MethodDef{},
	}
}

type runtimeError struct {
	Msg  string
	Line int
	Col  int
}

func (e *runtimeError) Error() string {
	if e.Line == 0 && e.Col == 0 {
		return "runtime error: " + e.Msg
	}
	return fmt.Sprintf("runtime error at %d:%d: %s", e.Line, e.Col, e.Msg)
}

// RuntimeError returns true if err is an interpreter runtime error.
func RuntimeError(err error) bool {
	_, ok := err.(*runtimeError)
	return ok
}

// Run executes the program. It collects `def`s, verifies `app()` exists, and calls it.
func (i *Interpreter) Run(prog *parser.Program) error {
	if i.Out == nil {
		i.Out = os.Stdout
	}
	// Imports
	for _, imp := range prog.Imports {
		i.imported[imp.Name] = true
	}
	// Methods: collect first so call order doesn't matter
	for _, m := range prog.Methods {
		if _, exists := i.methods[m.Name]; exists {
			line, col := m.Pos()
			return &runtimeError{Msg: fmt.Sprintf("method %q is defined more than once", m.Name), Line: line, Col: col}
		}
		i.methods[m.Name] = m
	}
	app, ok := i.methods["app"]
	if !ok {
		return &runtimeError{Msg: "program has no `app()` method"}
	}
	global := NewEnvironment(nil)
	_, err := i.execBlock(app.Body, global)
	return err
}

// blockResult carries control flow info out of a block. M1 has no return/break,
// but the shape is ready for M2+ to plug control flow in.
type blockResult struct {
	hasReturn bool
	value     Value
}

func (i *Interpreter) execBlock(b *parser.Block, env *Environment) (blockResult, error) {
	for _, st := range b.Stmts {
		if res, err := i.execStmt(st, env); err != nil {
			return blockResult{}, err
		} else if res.hasReturn {
			return res, nil
		}
	}
	return blockResult{}, nil
}

func (i *Interpreter) execStmt(s parser.Stmt, env *Environment) (blockResult, error) {
	switch st := s.(type) {
	case *parser.DefineStmt:
		val, err := i.evalExpr(st.InitExpr, env)
		if err != nil {
			return blockResult{}, err
		}
		if !val.MatchesDeclared(st.VarType) {
			line, col := st.Pos()
			return blockResult{}, &runtimeError{
				Msg:  fmt.Sprintf("cannot initialize %s variable %q with value of type %s", st.VarType, st.VarName, val.Kind),
				Line: line, Col: col,
			}
		}
		if err := env.Define(st.VarName, val, false); err != nil {
			line, col := st.Pos()
			return blockResult{}, &runtimeError{Msg: err.Error(), Line: line, Col: col}
		}
		return blockResult{}, nil
	case *parser.ExprStmt:
		if _, err := i.evalExpr(st.Expr, env); err != nil {
			return blockResult{}, err
		}
		return blockResult{}, nil
	}
	line, col := s.Pos()
	return blockResult{}, &runtimeError{Msg: fmt.Sprintf("unsupported statement type %T", s), Line: line, Col: col}
}

func (i *Interpreter) evalExpr(e parser.Expr, env *Environment) (Value, error) {
	switch ex := e.(type) {
	case *parser.IntLit:
		return IntVal(ex.Value), nil
	case *parser.StringLit:
		return StringVal(ex.Value), nil
	case *parser.VarExpr:
		v, err := env.Get(ex.Name)
		if err != nil {
			line, col := ex.Pos()
			return Value{}, &runtimeError{Msg: err.Error(), Line: line, Col: col}
		}
		return v, nil
	case *parser.BinaryExpr:
		return i.evalBinary(ex, env)
	case *parser.CallExpr:
		return i.evalCall(ex, env)
	}
	line, col := e.Pos()
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unsupported expression type %T", e), Line: line, Col: col}
}

func (i *Interpreter) evalBinary(b *parser.BinaryExpr, env *Environment) (Value, error) {
	lv, err := i.evalExpr(b.Left, env)
	if err != nil {
		return Value{}, err
	}
	rv, err := i.evalExpr(b.Right, env)
	if err != nil {
		return Value{}, err
	}
	line, col := b.Pos()
	// M1: only int arithmetic. String concatenation arrives in M2.
	if lv.Kind != KindInt || rv.Kind != KindInt {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("operator %s requires int operands, got %s and %s", b.Op, lv.Kind, rv.Kind), Line: line, Col: col}
	}
	switch b.Op {
	case parser.OpAdd:
		return IntVal(lv.Int + rv.Int), nil
	case parser.OpSub:
		return IntVal(lv.Int - rv.Int), nil
	case parser.OpMul:
		return IntVal(lv.Int * rv.Int), nil
	case parser.OpDiv:
		if rv.Int == 0 {
			return Value{}, &runtimeError{Msg: "integer division by zero", Line: line, Col: col}
		}
		return IntVal(lv.Int / rv.Int), nil
	case parser.OpMod:
		if rv.Int == 0 {
			return Value{}, &runtimeError{Msg: "integer modulo by zero", Line: line, Col: col}
		}
		return IntVal(lv.Int % rv.Int), nil
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown binary operator %s", b.Op), Line: line, Col: col}
}

func (i *Interpreter) evalCall(c *parser.CallExpr, env *Environment) (Value, error) {
	// User method?
	if m, ok := i.methods[c.Callee]; ok {
		if len(c.Args) != 0 {
			line, col := c.Pos()
			return Value{}, &runtimeError{Msg: fmt.Sprintf("method %q takes 0 arguments (M1), got %d", c.Callee, len(c.Args)), Line: line, Col: col}
		}
		// Per spec, methods get their own scope. Globals are not yet a thing in M1.
		callFrame := NewEnvironment(nil)
		res, err := i.execBlock(m.Body, callFrame)
		if err != nil {
			return Value{}, err
		}
		if res.hasReturn {
			return res.value, nil
		}
		return Null(), nil
	}
	// Builtin? Only callable if the owning library was imported.
	if fn, ok := i.Builtins[c.Callee]; ok {
		if !i.imported["stdlib"] {
			line, col := c.Pos()
			return Value{}, &runtimeError{Msg: fmt.Sprintf("`%s` requires `import stdlib;`", c.Callee), Line: line, Col: col}
		}
		args := make([]Value, 0, len(c.Args))
		for _, a := range c.Args {
			v, err := i.evalExpr(a, env)
			if err != nil {
				return Value{}, err
			}
			args = append(args, v)
		}
		v, err := fn(i.Out, args)
		if err != nil {
			line, col := c.Pos()
			return Value{}, &runtimeError{Msg: err.Error(), Line: line, Col: col}
		}
		return v, nil
	}
	line, col := c.Pos()
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown function %q", c.Callee), Line: line, Col: col}
}
