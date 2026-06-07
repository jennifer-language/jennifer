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

// libConstantEntry records a constant that ships with a library (e.g. math.PI).
// Looked up at evaluation time when a bare-IDENT reference isn't found in the
// user environment - same gating rule as builtins (owning library must be
// `use`d).
type libConstantEntry struct {
	Lib   string
	Value Value
}

// Interpreter walks a parsed Program and runs it.
type Interpreter struct {
	Out          io.Writer // defaults to os.Stdout if nil
	Builtins     map[string]builtinEntry
	LibConstants map[string]libConstantEntry // library-provided constants (math.PI, ...)
	knownLibs    map[string]bool             // libraries with at least one registered builtin OR constant
	imported     map[string]bool             // libraries the program has `use`d
	methods      map[string]*parser.MethodDef
	global       *Environment // global scope where top-level statements live
}

func New() *Interpreter {
	in := &Interpreter{
		Out:          os.Stdout,
		Builtins:     map[string]builtinEntry{},
		LibConstants: map[string]libConstantEntry{},
		knownLibs:    map[string]bool{},
		imported:     map[string]bool{},
		methods:      map[string]*parser.MethodDef{},
	}
	// `core` is auto-loaded: its builtins and constants are visible without
	// the user writing `use core;`. The library itself registers via
	// corelib.Install (called by the CLI / tests, just like other libraries);
	// the auto-import here is what makes its names resolve without ceremony.
	// Explicit `use core;` in source is rejected at Run-time (see below).
	in.imported[CoreLibraryName] = true
	return in
}

// CoreLibraryName is the reserved name of the auto-loaded library. Lives
// here (rather than in internal/lib/core) so the interpreter can reject
// `use core;` without importing the library package and creating a cycle.
const CoreLibraryName = "core"

// Register attaches a builtin function under the given Jennifer library name.
// Libraries call this at install time; programs make those builtins callable
// by writing `use <lib>;`.
func (i *Interpreter) Register(lib, name string, fn Builtin) {
	i.Builtins[name] = builtinEntry{Lib: lib, Fn: fn}
	i.knownLibs[lib] = true
}

// RegisterConst attaches a library-provided constant under the given Jennifer
// library name. Programs reference it as a bare identifier (e.g. `PI`); the
// reference is only allowed after `use <lib>;`.
func (i *Interpreter) RegisterConst(lib, name string, value Value) {
	i.LibConstants[name] = libConstantEntry{Lib: lib, Value: value}
	i.knownLibs[lib] = true
}

// availableLibsString returns a sorted, comma-separated list of registered
// library names for use in error messages. `core` is excluded because it's
// auto-loaded - suggesting `use core;` to a user who typoed something would
// just lead them to a second, different error. "(none)" if nothing else
// was installed.
func (i *Interpreter) availableLibsString() string {
	names := make([]string, 0, len(i.knownLibs))
	for n := range i.knownLibs {
		if n == CoreLibraryName {
			continue
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return "(none)"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

type runtimeError struct {
	Msg  string
	File string
	Line int
	Col  int
}

func (e *runtimeError) Error() string {
	if e.Line == 0 && e.Col == 0 {
		return "runtime error: " + e.Msg
	}
	if e.File != "" {
		return fmt.Sprintf("runtime error at %s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("runtime error at %d:%d: %s", e.Line, e.Col, e.Msg)
}

// RuntimeError returns true if err is an interpreter runtime error.
func RuntimeError(err error) bool {
	_, ok := err.(*runtimeError)
	return ok
}

// Position implements the positioned-error interface used by the CLI.
func (e *runtimeError) Position() (file string, line, col int) {
	return e.File, e.Line, e.Col
}

// posFor extracts (file, line, col) from any AST node. Used to construct
// positioned runtime errors that point at the right source file - important
// when an error originates inside an imported `.j` file.
func posFor(n parser.Node) (file string, line, col int) {
	line, col = n.Pos()
	return n.Filename(), line, col
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
	// hide typos like `use stdio;` (instead of `io`). The `core` library is
	// a special case: it's auto-loaded by New(), so an explicit `use core;`
	// is rejected to keep the source honest about which libraries are
	// expressly invoked.
	for _, imp := range prog.Imports {
		if imp.Name == CoreLibraryName {
			file, line, col := posFor(imp)
			return &runtimeError{
				Msg:  fmt.Sprintf("library %q is automatically available; remove this `use %s;` statement", imp.Name, imp.Name),
				File: file, Line: line, Col: col,
			}
		}
		if !i.knownLibs[imp.Name] {
			file, line, col := posFor(imp)
			return &runtimeError{
				Msg:  fmt.Sprintf("unknown library %q (available: %s)", imp.Name, i.availableLibsString()),
				File: file, Line: line, Col: col,
			}
		}
		i.imported[imp.Name] = true
	}
	// Methods: collect first so call order doesn't matter
	for _, m := range prog.Methods {
		if _, exists := i.methods[m.Name]; exists {
			file, line, col := posFor(m)
			return &runtimeError{Msg: fmt.Sprintf("method %q is defined more than once", m.Name), File: file, Line: line, Col: col}
		}
		// No-shadowing rule for methods: a user method may not share a name
		// with a builtin from a library the program has `use`d. This mirrors
		// the variable no-shadowing rule. Without the relevant `use`, the
		// name is free for user code.
		if b, isBuiltin := i.Builtins[m.Name]; isBuiltin && i.imported[b.Lib] {
			file, line, col := posFor(m)
			return &runtimeError{
				Msg:  fmt.Sprintf("method %q shadows a builtin from `%s`; rename it or remove `use %s;`", m.Name, b.Lib, b.Lib),
				File: file, Line: line, Col: col,
			}
		}
		i.methods[m.Name] = m
	}
	i.global = NewEnvironment(nil)
	_, err := i.execStmts(prog.TopLevel, i.global)
	return err
}

// EvalInteractive runs a parsed Program in REPL mode. It differs from Run in
// three ways:
//
//  1. The global env is initialized lazily on the first call and preserved
//     across calls, so vars and consts defined in one REPL input remain
//     visible in the next.
//  2. Methods and imports already present are silently overwritten / no-oped
//     rather than producing "defined more than once" errors. The
//     builtin-shadowing rule still applies for new methods.
//  3. If the program's final TopLevel statement is a bare ExprStmt, the
//     value of that expression is returned to the caller so the REPL can
//     print it. For non-expression-ending input, the returned Value is null.
//
// EvalInteractive is intended for the REPL only; ordinary CLI runs use Run.
func (i *Interpreter) EvalInteractive(prog *parser.Program) (Value, error) {
	if i.Out == nil {
		i.Out = os.Stdout
	}
	for _, imp := range prog.Imports {
		if imp.Name == CoreLibraryName {
			file, line, col := posFor(imp)
			return Null(), &runtimeError{
				Msg:  fmt.Sprintf("library %q is automatically available; remove this `use %s;` statement", imp.Name, imp.Name),
				File: file, Line: line, Col: col,
			}
		}
		if !i.knownLibs[imp.Name] {
			file, line, col := posFor(imp)
			return Null(), &runtimeError{
				Msg:  fmt.Sprintf("unknown library %q (available: %s)", imp.Name, i.availableLibsString()),
				File: file, Line: line, Col: col,
			}
		}
		i.imported[imp.Name] = true
	}
	for _, m := range prog.Methods {
		if b, isBuiltin := i.Builtins[m.Name]; isBuiltin && i.imported[b.Lib] {
			file, line, col := posFor(m)
			return Null(), &runtimeError{
				Msg:  fmt.Sprintf("method %q shadows a builtin from `%s`; rename it or remove `use %s;`", m.Name, b.Lib, b.Lib),
				File: file, Line: line, Col: col,
			}
		}
		i.methods[m.Name] = m
	}
	if i.global == nil {
		i.global = NewEnvironment(nil)
	}
	last := Null()
	for _, st := range prog.TopLevel {
		if es, ok := st.(*parser.ExprStmt); ok {
			v, err := i.evalExpr(es.Expr, i.global)
			if err != nil {
				return Null(), err
			}
			last = v
			continue
		}
		last = Null()
		if _, err := i.execStmt(st, i.global); err != nil {
			return Null(), err
		}
	}
	return last, nil
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
	file, line, col := posFor(s)
	return blockResult{}, &runtimeError{Msg: fmt.Sprintf("unsupported statement type %T", s), File: file, Line: line, Col: col}
}

func (i *Interpreter) execDefine(st *parser.DefineStmt, env *Environment) error {
	var val Value
	if st.InitExpr != nil {
		v, err := i.evalExpr(st.InitExpr, env)
		if err != nil {
			return err
		}
		if !v.MatchesDeclared(st.VarType) {
			file, line, col := posFor(st)
			noun := "variable"
			if st.IsConst {
				noun = "constant"
			}
			return &runtimeError{Msg: fmt.Sprintf("cannot initialize %s %s %q with value of type %s", st.VarType, noun, st.VarName, v.Kind), File: file, Line: line, Col: col}
		}
		val = v
	} else {
		// Spec / M2 decision: uninitialized variables get the zero value of
		// their declared type. Constants must always be initialized (the
		// parser enforces this; the assertion below is defensive).
		if st.IsConst {
			file, line, col := posFor(st)
			return &runtimeError{Msg: "internal: constant without init reached interpreter", File: file, Line: line, Col: col}
		}
		val = ZeroFor(st.VarType)
	}
	if err := env.Define(st.VarName, val, st.VarType, st.IsConst); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

func (i *Interpreter) execAssign(st *parser.AssignStmt, env *Environment) error {
	val, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	if err := env.Assign(st.VarName, val); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
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
		file, line, col := posFor(e)
		return false, &runtimeError{Msg: fmt.Sprintf("%s must be bool, got %s", ctx, v.Kind), File: file, Line: line, Col: col}
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
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		return v, nil
	case *parser.ConstRefExpr:
		// 1. User scope first (variables and `def const`).
		b, err := env.GetBinding(ex.Name)
		if err == nil {
			if !b.IsConst {
				file, line, col := posFor(ex)
				return Value{}, &runtimeError{
					Msg:  fmt.Sprintf("%q is a variable; use `$%s` to reference it", ex.Name, ex.Name),
					File: file, Line: line, Col: col,
				}
			}
			return b.Value, nil
		}
		// 2. Library-provided constants (e.g. math.PI), only when the
		// owning library has been `use`d.
		if c, ok := i.LibConstants[ex.Name]; ok {
			if !i.imported[c.Lib] {
				file, line, col := posFor(ex)
				return Value{}, &runtimeError{Msg: fmt.Sprintf("`%s` requires `use %s;`", ex.Name, c.Lib), File: file, Line: line, Col: col}
			}
			return c.Value, nil
		}
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("undefined name %q", ex.Name), File: file, Line: line, Col: col}
	case *parser.BinaryExpr:
		return i.evalBinary(ex, env)
	case *parser.UnaryExpr:
		return i.evalUnary(ex, env)
	case *parser.CallExpr:
		return i.evalCall(ex, env)
	}
	file, line, col := posFor(e)
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unsupported expression type %T", e), File: file, Line: line, Col: col}
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
	file, line, col := posFor(b)
	if b.Op.IsComparison() {
		return i.evalComparison(b.Op, lv, rv, file, line, col)
	}
	return i.evalArithmetic(b.Op, lv, rv, file, line, col)
}

// evalLogical implements short-circuit `and`/`or`. Both operands must be bool;
// the right operand is only evaluated when the left doesn't already decide.
func (i *Interpreter) evalLogical(b *parser.BinaryExpr, env *Environment) (Value, error) {
	lv, err := i.evalExpr(b.Left, env)
	if err != nil {
		return Value{}, err
	}
	if lv.Kind != KindBool {
		file, line, col := posFor(b.Left)
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("left operand of `%s` must be bool, got %s", b.Op, lv.Kind),
			File: file, Line: line, Col: col,
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
		file, line, col := posFor(b.Right)
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("right operand of `%s` must be bool, got %s", b.Op, rv.Kind),
			File: file, Line: line, Col: col,
		}
	}
	return BoolVal(rv.Bool), nil
}

func (i *Interpreter) evalUnary(u *parser.UnaryExpr, env *Environment) (Value, error) {
	v, err := i.evalExpr(u.Operand, env)
	if err != nil {
		return Value{}, err
	}
	file, line, col := posFor(u)
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
			File: file, Line: line, Col: col,
		}
	case parser.OpNot:
		if v.Kind != KindBool {
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("unary `not` requires bool, got %s", v.Kind),
				File: file, Line: line, Col: col,
			}
		}
		return BoolVal(!v.Bool), nil
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown unary operator %s", u.Op), File: file, Line: line, Col: col}
}

func (i *Interpreter) evalComparison(op parser.BinaryOp, lv, rv Value, file string, line, col int) (Value, error) {
	// `==` works for any same-kind comparison (and across int/float). Other
	// comparisons require numeric operands.
	if op == parser.OpEq {
		return BoolVal(lv.Equal(rv)), nil
	}
	a, aok := lv.AsFloat()
	b, bok := rv.AsFloat()
	if !aok || !bok {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("operator %s requires numeric operands, got %s and %s", op, lv.Kind, rv.Kind), File: file, Line: line, Col: col}
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
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown comparison %s", op), File: file, Line: line, Col: col}
}

func (i *Interpreter) evalArithmetic(op parser.BinaryOp, lv, rv Value, file string, line, col int) (Value, error) {
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
				return Value{}, &runtimeError{Msg: "integer division by zero", File: file, Line: line, Col: col}
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
				return Value{}, &runtimeError{Msg: "integer modulo by zero", File: file, Line: line, Col: col}
			}
			return IntVal(lv.Int % rv.Int), nil
		}
	}
	// Mixed or float operands: promote both to float (modulo is rejected for
	// floats; `div` returns a float that is the floor of the true quotient).
	a, aok := lv.AsFloat()
	b, bok := rv.AsFloat()
	if !aok || !bok {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("operator %s requires numeric operands, got %s and %s", op, lv.Kind, rv.Kind), File: file, Line: line, Col: col}
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
			return Value{}, &runtimeError{Msg: "division by zero", File: file, Line: line, Col: col}
		}
		return FloatVal(a / b), nil
	case parser.OpFloorDiv:
		if b == 0 {
			return Value{}, &runtimeError{Msg: "division by zero", File: file, Line: line, Col: col}
		}
		return FloatVal(floorDiv(a, b)), nil
	case parser.OpMod:
		return Value{}, &runtimeError{Msg: "operator % requires int operands, got float", File: file, Line: line, Col: col}
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown binary operator %s", op), File: file, Line: line, Col: col}
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
			file, line, col := posFor(c)
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("method %q takes %d argument(s), got %d", c.Callee, len(m.Params), len(c.Args)),
				File: file, Line: line, Col: col,
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
				file, line, col := posFor(a)
				return Value{}, &runtimeError{
					Msg:  fmt.Sprintf("argument %d to %q must be %s, got %s", idx+1, c.Callee, m.Params[idx].Type, v.Kind),
					File: file, Line: line, Col: col,
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
			file, line, col := posFor(c)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("`%s` requires `use %s;`", c.Callee, b.Lib), File: file, Line: line, Col: col}
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
			file, line, col := posFor(c)
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		return v, nil
	}
	file, line, col := posFor(c)
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown function %q", c.Callee), File: file, Line: line, Col: col}
}
