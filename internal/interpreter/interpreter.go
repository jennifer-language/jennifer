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

// BuiltinCtx is the I/O context the interpreter passes to every builtin.
// `Out` is where stdout-like effects write (e.g. `printf`); `In` is the
// reader stdin-consuming builtins read from (e.g. `readLine`); `InREPL`
// is true when the call originates inside the interactive REPL, so
// stdin-consuming builtins can refuse rather than racing the line
// editor for input.
type BuiltinCtx struct {
	Out    io.Writer
	In     io.Reader
	InREPL bool
}

// Builtin is a Go-implemented library function callable from Jennifer source.
// The interpreter passes a populated BuiltinCtx; functions that don't need
// I/O can ignore it. Returning Null() for void-like calls is fine.
type Builtin func(ctx BuiltinCtx, args []Value) (Value, error)

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

// nsKey identifies a namespaced builtin or constant by (namespace,
// name). Used as a map key so a single map covers `bio.translate`,
// `os.platform`, etc. without nested maps. The namespace doubles as the
// owning library's name; the user enables the namespace via
// `use <lib>;` and addresses callees as `<lib>.name(...)`.
type nsKey struct {
	NS   string
	Name string
}

// Interpreter walks a parsed Program and runs it.
type Interpreter struct {
	Out             io.Writer // defaults to os.Stdout if nil
	In              io.Reader // defaults to os.Stdin if nil
	InREPL          bool      // set by the REPL so stdin-consuming builtins refuse
	Builtins        map[string]builtinEntry
	LibConstants    map[string]libConstantEntry // library-provided constants (math.PI, ...)
	NSBuiltins      map[nsKey]Builtin           // namespaced builtins: os.platform, bio.translate, ...
	NSConstants     map[nsKey]Value             // namespaced constants: os.JENNIFER_LF, ...
	knownLibs       map[string]bool             // libraries with at least one registered builtin OR constant
	knownNamespaces map[string]bool             // libraries that registered through the namespaced API
	imported        map[string]bool             // libraries the program has `use`d
	nsPrefixes      map[string]string           // active call-site prefix -> canonical namespace (after aliasing)
	nsAliasedAway   map[string]string           // canonical namespace -> alias chosen by `use NAME as ALIAS;`
	methods         map[string]*parser.MethodDef
	global          *Environment // global scope where top-level statements live
}

func New() *Interpreter {
	in := &Interpreter{
		Out:             os.Stdout,
		In:              os.Stdin,
		Builtins:        map[string]builtinEntry{},
		LibConstants:    map[string]libConstantEntry{},
		NSBuiltins:      map[nsKey]Builtin{},
		NSConstants:     map[nsKey]Value{},
		knownLibs:       map[string]bool{},
		knownNamespaces: map[string]bool{},
		imported:        map[string]bool{},
		nsPrefixes:      map[string]string{},
		nsAliasedAway:   map[string]string{},
		methods:         map[string]*parser.MethodDef{},
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

// RegisterNamespaced attaches a namespaced builtin. The library
// name doubles as the namespace prefix: `in.RegisterNamespaced("os",
// "platform", fn)` makes the function callable as `os.platform()` once
// the program writes `use os;`. A library may register both flat
// builtins (via Register) and namespaced builtins, but the essential
// libraries (io, convert, math, strings, core) intentionally stay flat.
func (i *Interpreter) RegisterNamespaced(lib, name string, fn Builtin) {
	i.NSBuiltins[nsKey{NS: lib, Name: name}] = fn
	i.knownLibs[lib] = true
	i.knownNamespaces[lib] = true
}

// RegisterNamespacedConst attaches a namespaced constant. Same
// gating model as RegisterNamespaced - the constant is reachable only as
// `<lib>.NAME` and only after `use <lib>;`.
func (i *Interpreter) RegisterNamespacedConst(lib, name string, value Value) {
	i.NSConstants[nsKey{NS: lib, Name: name}] = value
	i.knownLibs[lib] = true
	i.knownNamespaces[lib] = true
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
	if i.In == nil {
		i.In = os.Stdin
	}
	if err := i.processImports(prog, false); err != nil {
		return err
	}
	// Methods: collect first so call order doesn't matter
	for _, m := range prog.Methods {
		if _, exists := i.methods[m.Name]; exists {
			file, line, col := posFor(m)
			return &runtimeError{Msg: fmt.Sprintf("method %q is defined more than once", m.Name), File: file, Line: line, Col: col}
		}
		if err := i.checkMethodNoShadow(m); err != nil {
			return err
		}
		i.methods[m.Name] = m
	}
	i.global = NewEnvironment(nil)
	_, err := i.execStmts(prog.TopLevel, i.global)
	return err
}

// processImports walks `use NAME [as ALIAS];` statements and updates the
// interpreter's import / namespace tables. Shared by Run (one-shot batch
// mode) and EvalInteractive (REPL); the `repl` flag tunes a couple of
// behaviours - the REPL silently re-imports a library a user already
// `use`d (so re-running a snippet works), while batch mode would too,
// since `imported` is just a set.
//
// Namespace-aware rules:
//   - `use os;` activates prefix "os" -> namespace "os".
//   - `use os as o;` activates prefix "o" -> namespace "os" and records
//     that the canonical name "os" has been aliased. After the alias,
//     `os.foo()` is rejected with a "did you mean `o`?" hint.
//   - The prefix has to be unique among active namespaces; two libs
//     fighting for the same prefix is a positioned error.
//   - `core` is always auto-loaded; explicit `use core;` is rejected.
func (i *Interpreter) processImports(prog *parser.Program, repl bool) error {
	_ = repl // currently unused; kept for documentation symmetry with the REPL path
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
		// Namespace bookkeeping. Only libraries that registered through the
		// namespaced API participate in qualified-call resolution; a
		// `use math;` for a flat-only library has no namespace effect even
		// if the user writes the optional `as ALIAS` clause (which would
		// be pointless for a flat lib).
		if !i.knownNamespaces[imp.Name] {
			if imp.AsName != "" {
				file, line, col := posFor(imp)
				return &runtimeError{
					Msg:  fmt.Sprintf("library %q has no namespaced builtins; `as %s` aliasing is meaningless here", imp.Name, imp.AsName),
					File: file, Line: line, Col: col,
				}
			}
			continue
		}
		prefix := imp.Name
		if imp.AsName != "" {
			prefix = imp.AsName
			i.nsAliasedAway[imp.Name] = imp.AsName
		}
		if existingNS, taken := i.nsPrefixes[prefix]; taken && existingNS != imp.Name {
			file, line, col := posFor(imp)
			return &runtimeError{
				Msg:  fmt.Sprintf("namespace prefix %q is already bound to library %q", prefix, existingNS),
				File: file, Line: line, Col: col,
			}
		}
		i.nsPrefixes[prefix] = imp.Name
	}
	return nil
}

// checkMethodNoShadow enforces the no-shadowing rules that apply to a
// top-level method definition:
//
//   - A method may not share its name with a flat builtin from a library
//     the program has `use`d (the existing rule).
//   - A method may not share its name with an active namespace prefix
//     (`func os() {}` is rejected after `use os;` because `os.foo()`
//     would then collide with a regular call to `os()`).
//
// Run() and EvalInteractive() share this so the REPL stays consistent
// with batch mode.
func (i *Interpreter) checkMethodNoShadow(m *parser.MethodDef) error {
	if b, isBuiltin := i.Builtins[m.Name]; isBuiltin && i.imported[b.Lib] {
		file, line, col := posFor(m)
		return &runtimeError{
			Msg:  fmt.Sprintf("method %q shadows a builtin from `%s`; rename it or remove `use %s;`", m.Name, b.Lib, b.Lib),
			File: file, Line: line, Col: col,
		}
	}
	if _, isPrefix := i.nsPrefixes[m.Name]; isPrefix {
		file, line, col := posFor(m)
		return &runtimeError{
			Msg:  fmt.Sprintf("method %q shadows imported namespace %q", m.Name, m.Name),
			File: file, Line: line, Col: col,
		}
	}
	return nil
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
	if i.In == nil {
		i.In = os.Stdin
	}
	if err := i.processImports(prog, true); err != nil {
		return Null(), err
	}
	for _, m := range prog.Methods {
		if err := i.checkMethodNoShadow(m); err != nil {
			return Null(), err
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
	case *parser.IndexAssignStmt:
		return blockResult{}, i.execIndexAssign(st, env)
	case *parser.AppendStmt:
		return blockResult{}, i.execAppend(st, env)
	case *parser.IfStmt:
		return i.execIf(st, env)
	case *parser.WhileStmt:
		return i.execWhile(st, env)
	case *parser.ForStmt:
		return i.execFor(st, env)
	case *parser.ForEachStmt:
		return i.execForEach(st, env)
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
		// Value semantics + type stamping: take an independent copy so the
		// initializer expression can't alias into this binding, and stamp
		// the declared element / key+value type onto the (possibly empty
		// or untyped) container so subsequent `$x[i] = ...` writes can
		// enforce the declared inner type.
		val = stampDeclaredType(v.Copy(), st.VarType)
	} else {
		// Spec / M2 decision: uninitialized variables get the zero value of
		// their declared type. Constants must always be initialized (the
		// parser enforces this; the assertion below is defensive).
		if st.IsConst {
			file, line, col := posFor(st)
			return &runtimeError{Msg: "internal: constant without init reached interpreter", File: file, Line: line, Col: col}
		}
		val = stampDeclaredType(ZeroFor(st.VarType), st.VarType)
	}
	if err := env.Define(st.VarName, val, st.VarType, st.IsConst); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// stampDeclaredType walks v and writes the declared inner-type pointers
// (Element for lists, KeyType/ValType for maps) onto the value and,
// recursively, onto every nested compound element. After this, an index
// chain has the type info it needs at each level to type-check writes -
// even though the literal expression that built v didn't carry any.
//
// Called at every binding boundary: Define, Assign, parameter pass,
// for-each iteration variable. Operates in place on v (caller is
// expected to have already Copy()'d if independence matters).
func stampDeclaredType(v Value, declType parser.Type) Value {
	switch declType.Kind {
	case parser.TypeList:
		if v.Kind != KindList {
			return v
		}
		v.ElemTyp = declType.Element
		if declType.Element != nil {
			et := declType.Element
			if et.Kind == parser.TypeList || et.Kind == parser.TypeMap {
				for i := range v.List {
					v.List[i] = stampDeclaredType(v.List[i], *et)
				}
			}
		}
	case parser.TypeMap:
		if v.Kind != KindMap {
			return v
		}
		v.KeyTyp = declType.KeyType
		v.ValTyp = declType.ValType
		if declType.ValType != nil {
			vt := declType.ValType
			if vt.Kind == parser.TypeList || vt.Kind == parser.TypeMap {
				for k := range v.Map {
					v.Map[k].Value = stampDeclaredType(v.Map[k].Value, *vt)
				}
			}
		}
	}
	return v
}

func (i *Interpreter) execAssign(st *parser.AssignStmt, env *Environment) error {
	val, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	// Value semantics + type stamping for compound assignments. The
	// destination's declared type tells us the inner shape; we re-stamp
	// because the right-hand-side may be a literal that's not yet
	// stamped. Primitives skip the stamp branch entirely.
	b, err := env.GetBinding(st.VarName)
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	val = stampDeclaredType(val.Copy(), b.DeclType)
	if err := env.Assign(st.VarName, val); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// execIndexAssign handles `$xs[i] = ...;`, `$m["k"] = ...;`, and chained
// forms. The const-target check fires once against the root binding; the
// rest is walking the index chain to find the slot and writing newVal.
// We operate on a Copy of the root binding so that intermediate slice
// aliasing doesn't leak out - the only thing visible to the caller is the
// final env.Assign at the end.
func (i *Interpreter) execIndexAssign(st *parser.IndexAssignStmt, env *Environment) error {
	rootVar := findIndexRoot(st.Target)
	if rootVar == nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: "internal: index-assign target has no root variable", File: file, Line: line, Col: col}
	}
	binding, err := env.GetBinding(rootVar.Name)
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("undefined variable %q", rootVar.Name), File: file, Line: line, Col: col}
	}
	if binding.IsConst {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("cannot mutate contents of constant %q (const is deep)", rootVar.Name), File: file, Line: line, Col: col}
	}

	newVal, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}

	// Evaluate indices outermost-first into a slice the chain walker can
	// consume. For `$xs[i][j]` the AST nests j on the outside and i on the
	// inside, so we collect inner-to-outer and reverse.
	var indices []Value
	for cur := st.Target; cur != nil; {
		v, err := i.evalExpr(cur.Index, env)
		if err != nil {
			return err
		}
		indices = append([]Value{v}, indices...)
		if next, ok := cur.Target.(*parser.IndexExpr); ok {
			cur = next
		} else {
			break
		}
	}

	// Work on a fresh copy of the root so writes don't accidentally
	// alias other live values; commit it back to the binding at the end.
	rootCopy := binding.Value.Copy()
	if err := applyIndexAssign(&rootCopy, indices, newVal, st); err != nil {
		return err
	}
	if err := env.Assign(rootVar.Name, rootCopy); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// execAppend handles `$xs[] = expr;` (M9). Same shape as execIndexAssign
// without an index chain: copy the target binding, append the new value,
// commit it back. Element-type check, const-target rejection, and
// non-list rejection all live here.
func (i *Interpreter) execAppend(st *parser.AppendStmt, env *Environment) error {
	binding, err := env.GetBinding(st.Target.Name)
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("undefined variable %q", st.Target.Name), File: file, Line: line, Col: col}
	}
	if binding.IsConst {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("cannot mutate contents of constant %q (const is deep)", st.Target.Name), File: file, Line: line, Col: col}
	}
	if binding.Value.Kind != KindList {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("`$%s[] = ...` requires a list, got %s", st.Target.Name, binding.Value.Kind), File: file, Line: line, Col: col}
	}
	newVal, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	if binding.Value.ElemTyp != nil && !newVal.MatchesDeclared(*binding.Value.ElemTyp) {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("cannot append %s to list of declared element type %s", newVal.Kind, binding.Value.ElemTyp), File: file, Line: line, Col: col}
	}
	rootCopy := binding.Value.Copy()
	rootCopy.List = append(rootCopy.List, newVal.Copy())
	if err := env.Assign(st.Target.Name, rootCopy); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// findIndexRoot walks an IndexExpr chain back to the underlying VarExpr.
// The parser guarantees that an IndexAssignStmt's Target is rooted on a
// VarExpr (the chain bottom), but production code shouldn't trust that
// invariant blindly: nil indicates "no usable root" and the caller
// surfaces an internal error.
func findIndexRoot(ix *parser.IndexExpr) *parser.VarExpr {
	var cur parser.Expr = ix
	for {
		switch n := cur.(type) {
		case *parser.IndexExpr:
			cur = n.Target
		case *parser.VarExpr:
			return n
		default:
			return nil
		}
	}
}

// applyIndexAssign walks the evaluated index path through a (mutable)
// root copy and writes newVal at the leaf. Intermediate steps go through
// indexInto, which returns a *Value pointing into the structure - so the
// final writeIndexedSlot writes through the chain without explicit
// writeback bookkeeping.
func applyIndexAssign(rootCopy *Value, indices []Value, newVal Value, st *parser.IndexAssignStmt) error {
	if len(indices) == 0 {
		file, line, col := posFor(st)
		return &runtimeError{Msg: "internal: index-assign with no indices", File: file, Line: line, Col: col}
	}
	cur := rootCopy
	for k := 0; k < len(indices)-1; k++ {
		next, err := indexInto(cur, indices[k], st)
		if err != nil {
			return err
		}
		cur = next
	}
	return writeIndexedSlot(cur, indices[len(indices)-1], newVal, st)
}

// indexInto returns a *Value pointing at the slot designated by idx
// within parent. Used by both reads (in evalExpr's IndexExpr case) and
// intermediate steps of index-assign chains. Out-of-bounds list indices
// and missing map keys both error positionally.
func indexInto(parent *Value, idx Value, st parser.Node) (*Value, error) {
	switch parent.Kind {
	case KindList:
		if idx.Kind != KindInt {
			file, line, col := posFor(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("list index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		if n < 0 || n >= len(parent.List) {
			file, line, col := posFor(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("list index %d out of bounds (len %d)", n, len(parent.List)), File: file, Line: line, Col: col}
		}
		return &parent.List[n], nil
	case KindMap:
		for k := range parent.Map {
			if parent.Map[k].Key.Equal(idx) {
				return &parent.Map[k].Value, nil
			}
		}
		file, line, col := posFor(st)
		return nil, &runtimeError{Msg: fmt.Sprintf("map has no entry for key %s", idx.Display()), File: file, Line: line, Col: col}
	}
	file, line, col := posFor(st)
	return nil, &runtimeError{Msg: fmt.Sprintf("cannot index into %s", parent.Kind), File: file, Line: line, Col: col}
}

// writeIndexedSlot sets parent[idx] = newVal. Lists: in-bounds only.
// Maps: existing key updates in place, missing key extends the map
// (insertion order is preserved). Element/value-type mismatches error.
func writeIndexedSlot(parent *Value, idx Value, newVal Value, st *parser.IndexAssignStmt) error {
	switch parent.Kind {
	case KindList:
		if idx.Kind != KindInt {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("list index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		if n < 0 || n >= len(parent.List) {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("list index %d out of bounds (len %d)", n, len(parent.List)), File: file, Line: line, Col: col}
		}
		if parent.ElemTyp != nil && !newVal.MatchesDeclared(*parent.ElemTyp) {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("cannot assign %s to list element of declared type %s", newVal.Kind, parent.ElemTyp), File: file, Line: line, Col: col}
		}
		parent.List[n] = newVal.Copy()
		return nil
	case KindMap:
		if parent.KeyTyp != nil && !idx.MatchesDeclared(*parent.KeyTyp) {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("map key must be %s, got %s", parent.KeyTyp, idx.Kind), File: file, Line: line, Col: col}
		}
		if parent.ValTyp != nil && !newVal.MatchesDeclared(*parent.ValTyp) {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("cannot assign %s to map value of declared type %s", newVal.Kind, parent.ValTyp), File: file, Line: line, Col: col}
		}
		for k := range parent.Map {
			if parent.Map[k].Key.Equal(idx) {
				parent.Map[k].Value = newVal.Copy()
				return nil
			}
		}
		// New key: append, preserving insertion order.
		parent.Map = append(parent.Map, MapEntry{Key: idx.Copy(), Value: newVal.Copy()})
		return nil
	}
	file, line, col := posFor(st)
	return &runtimeError{Msg: fmt.Sprintf("cannot index-assign into %s", parent.Kind), File: file, Line: line, Col: col}
}

// execForEach runs the body once per element (lists) or once per key
// (maps), binding the iteration variable in a fresh per-iteration scope
// so the binding doesn't leak out and `def` re-bindings don't accumulate.
func (i *Interpreter) execForEach(st *parser.ForEachStmt, env *Environment) (blockResult, error) {
	coll, err := i.evalExpr(st.Coll, env)
	if err != nil {
		return blockResult{}, err
	}
	// Surface the iteration variable's declared type to the binding so
	// MatchesDeclared works in the body. For lists it's the element type;
	// for maps it's the key type.
	var iterType parser.Type
	switch coll.Kind {
	case KindList:
		if coll.ElemTyp != nil {
			iterType = *coll.ElemTyp
		}
	case KindMap:
		if coll.KeyTyp != nil {
			iterType = *coll.KeyTyp
		}
	default:
		file, line, col := posFor(st)
		return blockResult{}, &runtimeError{Msg: fmt.Sprintf("for-each requires a list or map, got %s", coll.Kind), File: file, Line: line, Col: col}
	}

	emit := func(iter Value) (blockResult, error) {
		// Each iteration opens its own scope so the binding is fresh.
		iterEnv := NewEnvironment(env)
		if err := iterEnv.Define(st.VarName, iter.Copy(), iterType, false); err != nil {
			file, line, col := posFor(st)
			return blockResult{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		return i.execStmts(st.Body.Stmts, iterEnv)
	}

	switch coll.Kind {
	case KindList:
		for _, elem := range coll.List {
			res, err := emit(elem)
			if err != nil {
				return blockResult{}, err
			}
			if res.hasReturn {
				return res, nil
			}
		}
	case KindMap:
		for _, entry := range coll.Map {
			res, err := emit(entry.Key)
			if err != nil {
				return blockResult{}, err
			}
			if res.hasReturn {
				return res, nil
			}
		}
	}
	return blockResult{}, nil
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
	case *parser.QualifiedCallExpr:
		return i.evalQualifiedCall(ex, env)
	case *parser.QualifiedConstRefExpr:
		return i.evalQualifiedConst(ex)
	case *parser.ListLit:
		return i.evalListLit(ex, env)
	case *parser.MapLit:
		return i.evalMapLit(ex, env)
	case *parser.IndexExpr:
		return i.evalIndex(ex, env)
	}
	file, line, col := posFor(e)
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unsupported expression type %T", e), File: file, Line: line, Col: col}
}

// evalListLit builds a runtime list from a literal. Element types come
// from the values; the declared element type is set later by the
// surrounding Define/Assign when MatchesDeclared runs. The "list of T"
// constraint is enforced at assignment time, not literal time.
func (i *Interpreter) evalListLit(ex *parser.ListLit, env *Environment) (Value, error) {
	out := make([]Value, 0, len(ex.Elements))
	for _, e := range ex.Elements {
		v, err := i.evalExpr(e, env)
		if err != nil {
			return Value{}, err
		}
		out = append(out, v.Copy())
	}
	// Element type is left unset on the raw literal; the receiving
	// binding's MatchesDeclared check stamps it on via type inference at
	// the Define site. This keeps `[1, 2, 3]` usable as both `list of int`
	// and `list of int`-element nesting without re-parsing.
	return Value{Kind: KindList, List: out}, nil
}

// evalMapLit builds a runtime map from a literal. Insertion order is
// preserved (the entries slice is built in source order); deduplication
// is *not* performed here - duplicate keys are caught when the value is
// assigned to a typed binding, or simply produce extra entries the
// reader can spot.
func (i *Interpreter) evalMapLit(ex *parser.MapLit, env *Environment) (Value, error) {
	entries := make([]MapEntry, 0, len(ex.Keys))
	for k, keyExpr := range ex.Keys {
		key, err := i.evalExpr(keyExpr, env)
		if err != nil {
			return Value{}, err
		}
		val, err := i.evalExpr(ex.Values[k], env)
		if err != nil {
			return Value{}, err
		}
		entries = append(entries, MapEntry{Key: key.Copy(), Value: val.Copy()})
	}
	return Value{Kind: KindMap, Map: entries}, nil
}

// evalIndex implements read access for `$xs[i]`, `$m["k"]`, or arbitrary
// nesting. Reads of out-of-bounds list indices and missing map keys are
// positioned runtime errors (no null fallback - that's the M6 decision
// from milestones.md).
func (i *Interpreter) evalIndex(ex *parser.IndexExpr, env *Environment) (Value, error) {
	parent, err := i.evalExpr(ex.Target, env)
	if err != nil {
		return Value{}, err
	}
	idx, err := i.evalExpr(ex.Index, env)
	if err != nil {
		return Value{}, err
	}
	slot, err := indexInto(&parent, idx, ex)
	if err != nil {
		return Value{}, err
	}
	return *slot, nil
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
			// Value semantics: arguments copy into the call frame, so
			// callee mutations don't leak back to the caller. Stamp the
			// declared parameter type so compound parameters know their
			// element / key+value type for index-write checks.
			bound := stampDeclaredType(args[idx].Copy(), p.Type)
			if err := callFrame.Define(p.Name, bound, p.Type, false); err != nil {
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
		ctx := BuiltinCtx{Out: i.Out, In: i.In, InREPL: i.InREPL}
		v, err := b.Fn(ctx, args)
		if err != nil {
			file, line, col := posFor(c)
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		return v, nil
	}
	file, line, col := posFor(c)
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown function %q", c.Callee), File: file, Line: line, Col: col}
}

// resolveNamespacePrefix turns the source-level prefix on a qualified
// reference (`os` in `os.platform()`) into the canonical namespace tag
// the namespaced-builtin registry is keyed on, applying following rules:
//
//   - active prefix: the canonical namespace is returned, no error.
//   - canonical name that's been aliased away: error with the alias as
//     a "did you mean?" hint.
//   - canonical name of a namespaced lib the program hasn't `use`d:
//     error with the `use <lib>;` reminder.
//   - anything else: error as an unknown namespace.
//
// The caller decorates the returned error with positional info.
func (i *Interpreter) resolveNamespacePrefix(prefix string) (string, error) {
	if ns, ok := i.nsPrefixes[prefix]; ok {
		return ns, nil
	}
	if alias, aliased := i.nsAliasedAway[prefix]; aliased {
		return "", fmt.Errorf("namespace %q is aliased; did you mean `%s`?", prefix, alias)
	}
	if i.knownNamespaces[prefix] {
		return "", fmt.Errorf("namespace %q requires `use %s;`", prefix, prefix)
	}
	return "", fmt.Errorf("unknown namespace %q", prefix)
}

// evalQualifiedCall handles `prefix.callee(args)`. The prefix is
// resolved to a namespace (alias-aware), then the (namespace, callee)
// pair is looked up in the namespaced-builtin registry.
func (i *Interpreter) evalQualifiedCall(c *parser.QualifiedCallExpr, env *Environment) (Value, error) {
	ns, err := i.resolveNamespacePrefix(c.Prefix)
	if err != nil {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	fn, ok := i.NSBuiltins[nsKey{NS: ns, Name: c.Callee}]
	if !ok {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown function %q in namespace %q", c.Callee, ns), File: file, Line: line, Col: col}
	}
	args := make([]Value, 0, len(c.Args))
	for _, a := range c.Args {
		v, err := i.evalExpr(a, env)
		if err != nil {
			return Value{}, err
		}
		args = append(args, v)
	}
	ctx := BuiltinCtx{Out: i.Out, In: i.In, InREPL: i.InREPL}
	v, err := fn(ctx, args)
	if err != nil {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return v, nil
}

// evalQualifiedConst handles `prefix.NAME`. Resolution mirrors
// evalQualifiedCall; the result is the constant's value.
func (i *Interpreter) evalQualifiedConst(c *parser.QualifiedConstRefExpr) (Value, error) {
	ns, err := i.resolveNamespacePrefix(c.Prefix)
	if err != nil {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	v, ok := i.NSConstants[nsKey{NS: ns, Name: c.Name}]
	if !ok {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown constant %q in namespace %q", c.Name, ns), File: file, Line: line, Col: col}
	}
	return v, nil
}
