// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mplx/jennifer-lang/internal/parser"
)

// Builtin is a Go-implemented library function callable from Jennifer source.
// `out` is where stdout-like effects (e.g. printf) write; the interpreter passes
// its configured writer in. Returning Null() for void-like calls is fine.
type Builtin func(out io.Writer, args []Value) (Value, error)

// builtinEntry records a registered builtin and the library that owns it.
// A call resolves a callee name to its entry; the call is only allowed if
// the owning library has been `use`d in the program.
type builtinEntry struct {
	Lib string
	Fn  Builtin
}

// Interpreter walks a parsed Program and runs it.
type Interpreter struct {
	Out       io.Writer // defaults to os.Stdout if nil
	Builtins  map[string]builtinEntry
	knownLibs map[string]bool // libraries that have at least one registered builtin
	imported  map[string]bool // libraries the program has `use`d
	methods   map[string]*parser.MethodDef
	global    *Environment // global scope where top-level statements live
}

func New() *Interpreter {
	return &Interpreter{
		Out:       os.Stdout,
		Builtins:  map[string]builtinEntry{},
		knownLibs: map[string]bool{},
		imported:  map[string]bool{},
		methods:   map[string]*parser.MethodDef{},
	}
}

// Register attaches a builtin function under the given Jennifer library name.
// Libraries call this at install time; programs make those builtins callable
// by writing `use <lib>;`.
func (i *Interpreter) Register(lib, name string, fn Builtin) {
	i.Builtins[name] = builtinEntry{Lib: lib, Fn: fn}
	i.knownLibs[lib] = true
}

// availableLibsString returns a sorted, comma-separated list of registered
// library names for use in error messages. "(none)" if nothing was installed.
func (i *Interpreter) availableLibsString() string {
	if len(i.knownLibs) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(i.knownLibs))
	for n := range i.knownLibs {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
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

// Run executes the program. It records imports, hoists method definitions so
// they can be called in any order, then runs the program's top-level
// statements in source order in a global environment. Methods see this
// global env as their outer scope (so top-level vars are visible inside
// methods, subject to the no-shadowing rule).
func (i *Interpreter) Run(prog *parser.Program) error {
	if i.Out == nil {
		i.Out = os.Stdout
	}
	// Imports: every `use NAME;` must refer to a library that has at least
	// one registered builtin. Silent acceptance of unknown libraries would
	// hide typos like `use stdio;` (instead of `io`).
	for _, imp := range prog.Imports {
		if !i.knownLibs[imp.Name] {
			line, col := imp.Pos()
			return &runtimeError{
				Msg:  fmt.Sprintf("unknown library %q (available: %s)", imp.Name, i.availableLibsString()),
				Line: line, Col: col,
			}
		}
		i.imported[imp.Name] = true
	}
	// Methods: collect first so call order doesn't matter
	for _, m := range prog.Methods {
		if _, exists := i.methods[m.Name]; exists {
			line, col := m.Pos()
			return &runtimeError{Msg: fmt.Sprintf("method %q is defined more than once", m.Name), Line: line, Col: col}
		}
		// No-shadowing rule for methods: a user method may not share a name
		// with a builtin from a library the program has `use`d. This mirrors
		// the variable no-shadowing rule. Without the relevant `use`, the
		// name is free for user code.
		if b, isBuiltin := i.Builtins[m.Name]; isBuiltin && i.imported[b.Lib] {
			line, col := m.Pos()
			return &runtimeError{
				Msg:  fmt.Sprintf("method %q shadows a builtin from `%s`; rename it or remove `use %s;`", m.Name, b.Lib, b.Lib),
				Line: line, Col: col,
			}
		}
		i.methods[m.Name] = m
	}
	i.global = NewEnvironment(nil)
	_, err := i.execStmts(prog.TopLevel, i.global)
	return err
}

// blockResult carries control flow info out of a block. M1 has no return/break,
// but the shape is ready for M2+ to plug control flow in.
type blockResult struct {
	hasReturn bool
	value     Value
}

// execBlock runs every statement of a block in a *new* child env so that
// vars declared inside the block don't leak out. The caller passes the
// enclosing env; nested blocks inherit through the parent chain.
func (i *Interpreter) execBlock(b *parser.Block, parent *Environment) (blockResult, error) {
	env := NewEnvironment(parent)
	return i.execStmts(b.Stmts, env)
}

func (i *Interpreter) execStmts(stmts []parser.Stmt, env *Environment) (blockResult, error) {
	for _, st := range stmts {
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
		return blockResult{}, i.execDefine(st, env)
	case *parser.AssignStmt:
		return blockResult{}, i.execAssign(st, env)
	case *parser.IfStmt:
		return i.execIf(st, env)
	case *parser.WhileStmt:
		return i.execWhile(st, env)
	case *parser.ForStmt:
		return i.execFor(st, env)
	case *parser.ReturnStmt:
		if st.Value == nil {
			return blockResult{hasReturn: true, value: Null()}, nil
		}
		v, err := i.evalExpr(st.Value, env)
		if err != nil {
			return blockResult{}, err
		}
		return blockResult{hasReturn: true, value: v}, nil
	case *parser.ExprStmt:
		if _, err := i.evalExpr(st.Expr, env); err != nil {
			return blockResult{}, err
		}
		return blockResult{}, nil
	}
	line, col := s.Pos()
	return blockResult{}, &runtimeError{Msg: fmt.Sprintf("unsupported statement type %T", s), Line: line, Col: col}
}

func (i *Interpreter) execDefine(st *parser.DefineStmt, env *Environment) error {
	var val Value
	if st.InitExpr != nil {
		v, err := i.evalExpr(st.InitExpr, env)
		if err != nil {
			return err
		}
		if !v.MatchesDeclared(st.VarType) {
			line, col := st.Pos()
			noun := "variable"
			if st.IsConst {
				noun = "constant"
			}
			return &runtimeError{Msg: fmt.Sprintf("cannot initialize %s %s %q with value of type %s", st.VarType, noun, st.VarName, v.Kind), Line: line, Col: col}
		}
		val = v
	} else {
		// Spec / M2 decision: uninitialized variables get the zero value of
		// their declared type. Constants must always be initialized (the
		// parser enforces this; the assertion below is defensive).
		if st.IsConst {
			line, col := st.Pos()
			return &runtimeError{Msg: "internal: constant without init reached interpreter", Line: line, Col: col}
		}
		val = ZeroFor(st.VarType)
	}
	if err := env.Define(st.VarName, val, st.VarType, st.IsConst); err != nil {
		line, col := st.Pos()
		return &runtimeError{Msg: err.Error(), Line: line, Col: col}
	}
	return nil
}

func (i *Interpreter) execAssign(st *parser.AssignStmt, env *Environment) error {
	val, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	if err := env.Assign(st.VarName, val); err != nil {
		line, col := st.Pos()
		return &runtimeError{Msg: err.Error(), Line: line, Col: col}
	}
	return nil
}

func (i *Interpreter) execIf(st *parser.IfStmt, env *Environment) (blockResult, error) {
	cond, err := i.evalBool(st.Cond, env, "`if` condition")
	if err != nil {
		return blockResult{}, err
	}
	if cond {
		return i.execBlock(st.Then, env)
	}
	for idx, c := range st.ElseIfs {
		ok, err := i.evalBool(c, env, "`elseif` condition")
		if err != nil {
			return blockResult{}, err
		}
		if ok {
			return i.execBlock(st.ElseIfBodies[idx], env)
		}
	}
	if st.Else != nil {
		return i.execBlock(st.Else, env)
	}
	return blockResult{}, nil
}

func (i *Interpreter) execWhile(st *parser.WhileStmt, env *Environment) (blockResult, error) {
	for {
		cond, err := i.evalBool(st.Cond, env, "`while` condition")
		if err != nil {
			return blockResult{}, err
		}
		if !cond {
			return blockResult{}, nil
		}
		res, err := i.execBlock(st.Body, env)
		if err != nil {
			return blockResult{}, err
		}
		if res.hasReturn {
			return res, nil
		}
	}
}

func (i *Interpreter) execFor(st *parser.ForStmt, env *Environment) (blockResult, error) {
	// for-statements introduce their own scope: the init's binding (if any)
	// is visible in cond/step/body, but NOT after the loop.
	forEnv := NewEnvironment(env)
	if st.Init != nil {
		if _, err := i.execStmt(st.Init, forEnv); err != nil {
			return blockResult{}, err
		}
	}
	for {
		if st.Cond != nil {
			cond, err := i.evalBool(st.Cond, forEnv, "`for` condition")
			if err != nil {
				return blockResult{}, err
			}
			if !cond {
				return blockResult{}, nil
			}
		}
		res, err := i.execBlock(st.Body, forEnv)
		if err != nil {
			return blockResult{}, err
		}
		if res.hasReturn {
			return res, nil
		}
		if st.Step != nil {
			if _, err := i.execStmt(st.Step, forEnv); err != nil {
				return blockResult{}, err
			}
		}
	}
}

// evalBool evaluates an expression that must yield a bool; otherwise it
// produces a positional runtime error referring to `ctx`.
func (i *Interpreter) evalBool(e parser.Expr, env *Environment, ctx string) (bool, error) {
	v, err := i.evalExpr(e, env)
	if err != nil {
		return false, err
	}
	if v.Kind != KindBool {
		line, col := e.Pos()
		return false, &runtimeError{Msg: fmt.Sprintf("%s must be bool, got %s", ctx, v.Kind), Line: line, Col: col}
	}
	return v.Bool, nil
}

func (i *Interpreter) evalExpr(e parser.Expr, env *Environment) (Value, error) {
	switch ex := e.(type) {
	case *parser.IntLit:
		return IntVal(ex.Value), nil
	case *parser.FloatLit:
		return FloatVal(ex.Value), nil
	case *parser.StringLit:
		return StringVal(ex.Value), nil
	case *parser.BoolLit:
		return BoolVal(ex.Value), nil
	case *parser.NullLit:
		return Null(), nil
	case *parser.VarExpr:
		v, err := env.Get(ex.Name)
		if err != nil {
			line, col := ex.Pos()
			return Value{}, &runtimeError{Msg: err.Error(), Line: line, Col: col}
		}
		return v, nil
	case *parser.ConstRefExpr:
		b, err := env.GetBinding(ex.Name)
		if err != nil {
			line, col := ex.Pos()
			return Value{}, &runtimeError{Msg: fmt.Sprintf("undefined name %q", ex.Name), Line: line, Col: col}
		}
		if !b.IsConst {
			line, col := ex.Pos()
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("%q is a variable; use `$%s` to reference it", ex.Name, ex.Name),
				Line: line, Col: col,
			}
		}
		return b.Value, nil
	case *parser.BinaryExpr:
		return i.evalBinary(ex, env)
	case *parser.UnaryExpr:
		return i.evalUnary(ex, env)
	case *parser.CallExpr:
		return i.evalCall(ex, env)
	}
	line, col := e.Pos()
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unsupported expression type %T", e), Line: line, Col: col}
}

func (i *Interpreter) evalBinary(b *parser.BinaryExpr, env *Environment) (Value, error) {
	// Logical and/or evaluate the left operand first and short-circuit before
	// touching the right - important when the right has side effects (calls).
	if b.Op.IsLogical() {
		return i.evalLogical(b, env)
	}
	lv, err := i.evalExpr(b.Left, env)
	if err != nil {
		return Value{}, err
	}
	rv, err := i.evalExpr(b.Right, env)
	if err != nil {
		return Value{}, err
	}
	line, col := b.Pos()
	if b.Op.IsComparison() {
		return i.evalComparison(b.Op, lv, rv, line, col)
	}
	return i.evalArithmetic(b.Op, lv, rv, line, col)
}

// evalLogical implements short-circuit `and`/`or`. Both operands must be bool;
// the right operand is only evaluated when the left doesn't already decide.
func (i *Interpreter) evalLogical(b *parser.BinaryExpr, env *Environment) (Value, error) {
	lv, err := i.evalExpr(b.Left, env)
	if err != nil {
		return Value{}, err
	}
	if lv.Kind != KindBool {
		line, col := b.Left.Pos()
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("left operand of `%s` must be bool, got %s", b.Op, lv.Kind),
			Line: line, Col: col,
		}
	}
	// Short-circuit
	if b.Op == parser.OpAnd && !lv.Bool {
		return BoolVal(false), nil
	}
	if b.Op == parser.OpOr && lv.Bool {
		return BoolVal(true), nil
	}
	rv, err := i.evalExpr(b.Right, env)
	if err != nil {
		return Value{}, err
	}
	if rv.Kind != KindBool {
		line, col := b.Right.Pos()
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("right operand of `%s` must be bool, got %s", b.Op, rv.Kind),
			Line: line, Col: col,
		}
	}
	return BoolVal(rv.Bool), nil
}

func (i *Interpreter) evalUnary(u *parser.UnaryExpr, env *Environment) (Value, error) {
	v, err := i.evalExpr(u.Operand, env)
	if err != nil {
		return Value{}, err
	}
	line, col := u.Pos()
	switch u.Op {
	case parser.OpNeg:
		switch v.Kind {
		case KindInt:
			return IntVal(-v.Int), nil
		case KindFloat:
			return FloatVal(-v.Float), nil
		}
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("unary `-` requires int or float, got %s", v.Kind),
			Line: line, Col: col,
		}
	case parser.OpNot:
		if v.Kind != KindBool {
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("unary `not` requires bool, got %s", v.Kind),
				Line: line, Col: col,
			}
		}
		return BoolVal(!v.Bool), nil
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown unary operator %s", u.Op), Line: line, Col: col}
}

func (i *Interpreter) evalComparison(op parser.BinaryOp, lv, rv Value, line, col int) (Value, error) {
	// `==` works for any same-kind comparison (and across int/float). Other
	// comparisons require numeric operands.
	if op == parser.OpEq {
		return BoolVal(lv.Equal(rv)), nil
	}
	a, aok := lv.AsFloat()
	b, bok := rv.AsFloat()
	if !aok || !bok {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("operator %s requires numeric operands, got %s and %s", op, lv.Kind, rv.Kind), Line: line, Col: col}
	}
	switch op {
	case parser.OpLt:
		return BoolVal(a < b), nil
	case parser.OpGt:
		return BoolVal(a > b), nil
	case parser.OpLe:
		return BoolVal(a <= b), nil
	case parser.OpGe:
		return BoolVal(a >= b), nil
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown comparison %s", op), Line: line, Col: col}
}

func (i *Interpreter) evalArithmetic(op parser.BinaryOp, lv, rv Value, line, col int) (Value, error) {
	// String concatenation with `+`
	if op == parser.OpAdd && lv.Kind == KindString && rv.Kind == KindString {
		return StringVal(lv.Str + rv.Str), nil
	}
	// Pure-int fast path keeps int results exact for +, -, *, div, %.
	// `/` is NOT in the int fast path: per Python 3 semantics it always
	// returns float (see the mixed/float section below).
	if lv.Kind == KindInt && rv.Kind == KindInt {
		switch op {
		case parser.OpAdd:
			return IntVal(lv.Int + rv.Int), nil
		case parser.OpSub:
			return IntVal(lv.Int - rv.Int), nil
		case parser.OpMul:
			return IntVal(lv.Int * rv.Int), nil
		case parser.OpFloorDiv:
			if rv.Int == 0 {
				return Value{}, &runtimeError{Msg: "integer division by zero", Line: line, Col: col}
			}
			// Go's `/` on ints is truncate-toward-zero. Python-style `div`
			// (floor) only differs when signs differ; align with Python here.
			q := lv.Int / rv.Int
			if (lv.Int%rv.Int != 0) && ((lv.Int < 0) != (rv.Int < 0)) {
				q--
			}
			return IntVal(q), nil
		case parser.OpMod:
			if rv.Int == 0 {
				return Value{}, &runtimeError{Msg: "integer modulo by zero", Line: line, Col: col}
			}
			return IntVal(lv.Int % rv.Int), nil
		}
	}
	// Mixed or float operands: promote both to float (modulo is rejected for
	// floats; `div` returns a float that is the floor of the true quotient).
	a, aok := lv.AsFloat()
	b, bok := rv.AsFloat()
	if !aok || !bok {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("operator %s requires numeric operands, got %s and %s", op, lv.Kind, rv.Kind), Line: line, Col: col}
	}
	switch op {
	case parser.OpAdd:
		return FloatVal(a + b), nil
	case parser.OpSub:
		return FloatVal(a - b), nil
	case parser.OpMul:
		return FloatVal(a * b), nil
	case parser.OpDiv:
		if b == 0 {
			return Value{}, &runtimeError{Msg: "division by zero", Line: line, Col: col}
		}
		return FloatVal(a / b), nil
	case parser.OpFloorDiv:
		if b == 0 {
			return Value{}, &runtimeError{Msg: "division by zero", Line: line, Col: col}
		}
		return FloatVal(floorDiv(a, b)), nil
	case parser.OpMod:
		return Value{}, &runtimeError{Msg: "operator % requires int operands, got float", Line: line, Col: col}
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown binary operator %s", op), Line: line, Col: col}
}

// floorDiv computes math.Floor(a/b) without importing math (TinyGo size).
// Equivalent to math.Floor(a / b) for finite, non-zero b.
func floorDiv(a, b float64) float64 {
	q := a / b
	// Round toward negative infinity. The intrinsic `math.Floor` is fine but
	// we avoid the import for TinyGo binary size.
	if q < 0 && q != float64(int64(q)) {
		return float64(int64(q) - 1)
	}
	return float64(int64(q))
}

func (i *Interpreter) evalCall(c *parser.CallExpr, env *Environment) (Value, error) {
	// User method?
	if m, ok := i.methods[c.Callee]; ok {
		if len(c.Args) != len(m.Params) {
			line, col := c.Pos()
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("method %q takes %d argument(s), got %d", c.Callee, len(m.Params), len(c.Args)),
				Line: line, Col: col,
			}
		}
		// Evaluate args in the caller's env, then bind them in a fresh
		// call frame that inherits from globals. Each arg is type-checked
		// against the parameter's declared type.
		args := make([]Value, len(c.Args))
		for idx, a := range c.Args {
			v, err := i.evalExpr(a, env)
			if err != nil {
				return Value{}, err
			}
			if !v.MatchesDeclared(m.Params[idx].Type) {
				line, col := a.Pos()
				return Value{}, &runtimeError{
					Msg:  fmt.Sprintf("argument %d to %q must be %s, got %s", idx+1, c.Callee, m.Params[idx].Type, v.Kind),
					Line: line, Col: col,
				}
			}
			args[idx] = v
		}
		callFrame := NewEnvironment(i.global)
		for idx, p := range m.Params {
			if err := callFrame.Define(p.Name, args[idx], p.Type, false); err != nil {
				return Value{}, &runtimeError{Msg: err.Error(), Line: p.Line, Col: p.Col}
			}
		}
		res, err := i.execBlock(m.Body, callFrame)
		if err != nil {
			return Value{}, err
		}
		if res.hasReturn {
			return res.value, nil
		}
		return Null(), nil
	}
	// Builtin? Only callable if the owning library was `use`d.
	if b, ok := i.Builtins[c.Callee]; ok {
		if !i.imported[b.Lib] {
			line, col := c.Pos()
			return Value{}, &runtimeError{Msg: fmt.Sprintf("`%s` requires `use %s;`", c.Callee, b.Lib), Line: line, Col: col}
		}
		args := make([]Value, 0, len(c.Args))
		for _, a := range c.Args {
			v, err := i.evalExpr(a, env)
			if err != nil {
				return Value{}, err
			}
			args = append(args, v)
		}
		v, err := b.Fn(i.Out, args)
		if err != nil {
			line, col := c.Pos()
			return Value{}, &runtimeError{Msg: err.Error(), Line: line, Col: col}
		}
		return v, nil
	}
	line, col := c.Pos()
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown function %q", c.Callee), Line: line, Col: col}
}
