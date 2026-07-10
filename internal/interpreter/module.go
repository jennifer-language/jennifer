// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mplx/jennifer-lang/internal/module"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// moduleReg is the module registry shared across one program run: the
// run-once cache (canonical path -> loaded module), the in-progress load
// stack for cycle detection, the module search path, and the callbacks the
// loader needs to turn a resolved path into a runnable module. Every module
// loads into a fresh sub-interpreter that shares this registry, so
// run-once, depth-first post-order init, and cycle detection all fall out
// of the recursion.
type moduleReg struct {
	cache  map[string]*loadedModule
	stack  []string                                            // canonical paths currently loading
	search []string                                            // module search dirs (sysmoddir, then -I dirs)
	load   func(canonicalPath string) (*parser.Program, error) // lex/preproc/parse a module file
	setup  func(*Interpreter)                                  // install the standard library into a module interpreter
}

// loadedModule is one initialised module - its own interpreter (holding the
// module's scope and methods) plus the set of top-level names it `export`s.
// Only exported names are reachable through the importer's alias.
type loadedModule struct {
	interp  *Interpreter
	path    string
	ns      string          // module namespace (file stem); struct-identity prefix seen by importers
	exports map[string]bool // top-level names marked `export` (funcs, consts, structs)
}

// isOwnStruct reports whether name is one of this module's declared structs
// (as opposed to a library struct or a value from another module).
func (m *loadedModule) isOwnStruct(name string) bool {
	_, ok := m.interp.structs[name]
	return ok
}

// retagStructs returns a copy of v with every struct whose namespace is `from`
// and whose name is one of the module's own structs re-tagged to `to`,
// recursing through struct fields, list elements, and map values. It bridges a
// module's internal *bare* struct identity (StructNS "") and the namespaced
// identity `(module-stem, name)` importers see, so a value keeps a consistent
// type on each side of the boundary. Library / other-module structs (a
// different namespace) are left untouched.
func retagStructs(v Value, from, to string, isOwn func(string) bool) Value {
	switch v.Kind {
	case KindStruct:
		nv := v
		nv.Fields = make([]StructField, len(v.Fields))
		for i, f := range v.Fields {
			nv.Fields[i] = StructField{Name: f.Name, Value: retagStructs(f.Value, from, to, isOwn)}
		}
		if v.StructNS == from && isOwn(v.StructName) {
			nv.StructNS = to
		}
		nv.shared = nil
		return nv
	case KindList:
		nv := v
		nv.List = make([]Value, len(v.List))
		for i := range v.List {
			nv.List[i] = retagStructs(v.List[i], from, to, isOwn)
		}
		nv.ElemTyp = retagType(v.ElemTyp, from, to, isOwn)
		nv.shared = nil
		return nv
	case KindMap:
		nv := v
		nv.Map = make([]MapEntry, len(v.Map))
		for i := range v.Map {
			nv.Map[i] = MapEntry{Key: v.Map[i].Key, Value: retagStructs(v.Map[i].Value, from, to, isOwn)}
		}
		nv.KeyTyp = retagType(v.KeyTyp, from, to, isOwn)
		nv.ValTyp = retagType(v.ValTyp, from, to, isOwn)
		nv.shared = nil
		return nv
	default:
		return v
	}
}

// retagType returns a copy of a declared type with every struct type whose
// namespace is `from` and whose name is one of the module's own structs
// re-tagged to `to`, recursing through list / map element types. It mirrors
// retagStructs for the *type* metadata a list or map carries alongside its
// elements, so a `list of mod.Struct` handed back into the module reads as
// `list of Struct` (and vice versa) rather than failing the param-type check.
func retagType(t *parser.Type, from, to string, isOwn func(string) bool) *parser.Type {
	if t == nil {
		return nil
	}
	nt := *t
	if t.Kind == parser.TypeStruct && t.StructNS == from && isOwn(t.StructName) {
		nt.StructNS = to
	}
	nt.Element = retagType(t.Element, from, to, isOwn)
	nt.KeyType = retagType(t.KeyType, from, to, isOwn)
	nt.ValType = retagType(t.ValType, from, to, isOwn)
	return &nt
}

// collectExports gathers the names a module publishes: every `export`-marked
// top-level `func`, `def struct`, and `def const`.
func collectExports(prog *parser.Program) map[string]bool {
	exports := map[string]bool{}
	for _, m := range prog.Methods {
		if m.Exported {
			exports[m.Name] = true
		}
	}
	for _, s := range prog.Structs {
		if s.Exported {
			exports[s.Name] = true
		}
	}
	for _, st := range prog.TopLevel {
		if d, ok := st.(*parser.DefineStmt); ok && d.IsConst && d.Exported {
			exports[d.VarName] = true
		}
	}
	return exports
}

// EnableModules wires the module system onto the root interpreter: the base
// directory of the entry file (for local import resolution), the module
// search path (system module dir, then any -I dirs), a loader that turns a
// resolved file into a parsed program, and a setup callback that installs
// the standard library into each module's fresh sub-interpreter.
func (i *Interpreter) EnableModules(baseDir string, searchDirs []string, load func(string) (*parser.Program, error), setup func(*Interpreter)) {
	i.baseDir = baseDir
	i.modReg = &moduleReg{
		cache:  map[string]*loadedModule{},
		search: searchDirs,
		load:   load,
		setup:  setup,
	}
}

// loadModuleImports processes a program's `import "..."` statements before
// its body runs, so a module is fully initialised before the code that
// imports it (depth-first post-order). Errors here are load-time errors:
// they fail the program before the importer's body and are not catchable
// (an `import` is a declaration, not an expression).
func (i *Interpreter) loadModuleImports(prog *parser.Program) error {
	if len(prog.ModuleImports) == 0 {
		return nil
	}
	if i.modReg == nil {
		mi := prog.ModuleImports[0]
		file, line, col := posFor(mi)
		return &runtimeError{Msg: "module imports are not enabled in this context (run a program file)", File: file, Line: line, Col: col}
	}
	for _, mi := range prog.ModuleImports {
		m, err := i.loadModule(mi.Path, mi)
		if err != nil {
			return err
		}
		// Bind the alias (the `as NAME` clause, or the file stem) so
		// `NAME.member` resolves into the loaded module at this importer.
		alias := mi.AsName
		if alias == "" {
			alias = moduleStem(mi.Path)
		}
		if err := i.bindModuleAlias(alias, m, mi); err != nil {
			return err
		}
	}
	return nil
}

// moduleStem is the alias a module import binds to with no `as NAME` clause:
// the file stem of the import path (basename without the `.j` suffix).
func moduleStem(importPath string) string {
	return strings.TrimSuffix(filepath.Base(importPath), ".j")
}

// bindModuleAlias makes `alias.member` at this importer resolve into the
// loaded module. The alias must not collide with an active library prefix
// (`use io;` reserves `io`) or a module alias already bound in this program.
func (i *Interpreter) bindModuleAlias(alias string, m *loadedModule, at parser.Node) error {
	if _, taken := i.nsPrefixes[alias]; taken {
		file, line, col := posFor(at)
		return &runtimeError{Msg: fmt.Sprintf("module alias %q collides with an imported library namespace; import the module `as` a different name", alias), File: file, Line: line, Col: col}
	}
	if i.moduleAliases == nil {
		i.moduleAliases = map[string]*loadedModule{}
	}
	if _, dup := i.moduleAliases[alias]; dup {
		file, line, col := posFor(at)
		return &runtimeError{Msg: fmt.Sprintf("module alias %q is already bound; import the module `as` a different name", alias), File: file, Line: line, Col: col}
	}
	i.moduleAliases[alias] = m
	return nil
}

// callModuleMethod dispatches `alias.method(args)` into the loaded module's
// own interpreter: arguments are evaluated in the consumer's environment, and
// the method body runs against the module's globals and methods (via
// CallByNameWith). Arity / type mismatches are repositioned at the consumer's
// call site; a runtime error, `throw`, or `exit` from the module body
// propagates unchanged so `try`/`catch` and exit codes keep working.
func (i *Interpreter) callModuleMethod(m *loadedModule, c *parser.QualifiedCallExpr, env *Environment) (Value, error) {
	file, line, col := posFor(c)
	if _, ok := m.interp.methods[c.Callee]; !ok {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("module %q has no method %q", c.Prefix, c.Callee), File: file, Line: line, Col: col}
	}
	if !m.exports[c.Callee] {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("%s.%s: %q is not exported from module %q", c.Prefix, c.Callee, c.Callee, c.Prefix), File: file, Line: line, Col: col}
	}
	args := make([]Value, len(c.Args))
	for idx, a := range c.Args {
		v, err := i.evalExpr(a, env)
		if err != nil {
			return Value{}, err
		}
		// Cross the boundary inward: a module struct the consumer holds is
		// tagged `(module-stem, name)`; inside the module it is bare.
		args[idx] = retagStructs(v, m.ns, "", m.isOwnStruct)
	}
	v, err := m.interp.CallByNameWith(c.Callee, args...)
	if err != nil {
		switch err.(type) {
		case *runtimeError, *ExitSignal, *ErrorSignal:
			return Value{}, err // positioned / control-flow: propagate as-is
		default:
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
	}
	// Cross the boundary outward: give the module's own structs their
	// namespaced identity so the consumer can name and re-pass them.
	return retagStructs(v, "", m.ns, m.isOwnStruct), nil
}

// stampModuleStructType verifies `alias.Struct` names an exported struct of
// the module and rewrites the declared type's namespace from the importer's
// alias to the module's own stem, so it matches the identity a module value
// carries once it crosses the boundary.
func (i *Interpreter) stampModuleStructType(t *parser.Type, mod *loadedModule, at parser.Node) error {
	file, line, col := posFor(at)
	if !mod.isOwnStruct(t.StructName) {
		return &runtimeError{Msg: fmt.Sprintf("module %q has no struct %q", t.StructNS, t.StructName), File: file, Line: line, Col: col}
	}
	if !mod.exports[t.StructName] {
		return &runtimeError{Msg: fmt.Sprintf("%s.%s: struct %q is not exported from module %q", t.StructNS, t.StructName, t.StructName, t.StructNS), File: file, Line: line, Col: col}
	}
	t.StructNS = mod.ns
	return nil
}

// moduleConst reads `alias.NAME`, a constant declared at the module's top
// level, from the loaded module's global scope.
func (i *Interpreter) moduleConst(m *loadedModule, c *parser.QualifiedConstRefExpr) (Value, error) {
	b, err := m.interp.global.GetBinding(c.Name)
	if err != nil || !b.IsConst {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("module %q has no constant %q", c.Prefix, c.Name), File: file, Line: line, Col: col}
	}
	if !m.exports[c.Name] {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("%s.%s: %q is not exported from module %q", c.Prefix, c.Name, c.Name, c.Prefix), File: file, Line: line, Col: col}
	}
	return retagStructs(b.Value, "", m.ns, m.isOwnStruct), nil
}

// checkModuleDeclarationsOnly enforces the module top-level grammar: a module
// may contain only declarations - `def const`, `def struct`, `func`, `use`,
// and `import`. Structs, methods, and imports are collected into their own
// Program slices, so `TopLevel` must contain nothing but `def const`
// statements. A mutable `def` or a free-standing statement (assignment, bare
// expression, `if` / `while` / `for` / `repeat`) is a positioned load-time
// error: modules hold no mutable state and have no init body beyond their
// constant initializers. Scripts run through the CLI never reach here, so they
// keep top-level mutable `def` and free-standing statements.
func checkModuleDeclarationsOnly(prog *parser.Program) error {
	for _, s := range prog.TopLevel {
		if d, ok := s.(*parser.DefineStmt); ok && d.IsConst {
			continue // `def const NAME ...;` is the one allowed top-level statement
		}
		file, line, col := posFor(s)
		msg := "a module's top level allows only declarations (`def const`, `def struct`, `func`, `use`, `import`); free-standing statements are not allowed"
		if d, ok := s.(*parser.DefineStmt); ok && !d.IsConst {
			msg = "mutable `def` is not allowed at a module's top level (a module holds no mutable state); use `def const` for a module constant"
		}
		return &runtimeError{Msg: msg, File: file, Line: line, Col: col}
	}
	return nil
}

// checkReferentialClosure enforces that a module's public surface is
// self-contained: an exported struct field, or an exported function
// parameter, whose type is one of the module's *private* structs is a
// positioned error at the annotation - a caller could receive or be asked for
// a value of a type it can never name. Library / namespaced struct types
// (StructNS != "") always cross the boundary freely, so only the module's own
// bare struct types are checked.
func checkReferentialClosure(prog *parser.Program, exports map[string]bool) error {
	moduleStructs := map[string]bool{}
	for _, s := range prog.Structs {
		moduleStructs[s.Name] = true
	}
	// privateStructIn returns the name of a private module struct reachable
	// through t (directly or as a list / map / task element), or "".
	var privateStructIn func(t parser.Type) string
	privateStructIn = func(t parser.Type) string {
		switch t.Kind {
		case parser.TypeStruct:
			if t.StructNS == "" && moduleStructs[t.StructName] && !exports[t.StructName] {
				return t.StructName
			}
		case parser.TypeList, parser.TypeTask:
			if t.Element != nil {
				return privateStructIn(*t.Element)
			}
		case parser.TypeMap:
			if t.KeyType != nil {
				if n := privateStructIn(*t.KeyType); n != "" {
					return n
				}
			}
			if t.ValType != nil {
				return privateStructIn(*t.ValType)
			}
		}
		return ""
	}
	for _, s := range prog.Structs {
		if !s.Exported {
			continue
		}
		for _, f := range s.Fields {
			if bad := privateStructIn(f.Type); bad != "" {
				return &runtimeError{Msg: fmt.Sprintf("exported struct %q exposes private struct %q through field %q; `export` %q too, or drop the field", s.Name, bad, f.Name, bad), File: f.File, Line: f.Line, Col: f.Col}
			}
		}
	}
	for _, m := range prog.Methods {
		if !m.Exported {
			continue
		}
		for _, p := range m.Params {
			if bad := privateStructIn(p.Type); bad != "" {
				return &runtimeError{Msg: fmt.Sprintf("exported func %q takes private struct %q as parameter %q; `export` %q too", m.Name, bad, p.Name, bad), File: p.File, Line: p.Line, Col: p.Col}
			}
		}
	}
	return nil
}

// rejectExportInScript fails a program that carries an `export` marker but is
// run as a script (not loaded as a module): exports only mean something to an
// importer, and a script has none. Positioned at the marked declaration.
func rejectExportInScript(prog *parser.Program) error {
	const msg = "`export` is only allowed in a module; this file is run as a script, which has no importers"
	for _, m := range prog.Methods {
		if m.Exported {
			file, line, col := posFor(m)
			return &runtimeError{Msg: msg, File: file, Line: line, Col: col}
		}
	}
	for _, s := range prog.Structs {
		if s.Exported {
			file, line, col := posFor(s)
			return &runtimeError{Msg: msg, File: file, Line: line, Col: col}
		}
	}
	for _, st := range prog.TopLevel {
		if d, ok := st.(*parser.DefineStmt); ok && d.Exported {
			file, line, col := posFor(d)
			return &runtimeError{Msg: msg, File: file, Line: line, Col: col}
		}
	}
	return nil
}

// loadModule resolves importPath (relative to this interpreter's base dir,
// or the search path for a bare module name), then loads and runs the
// module exactly once, returning the cached instance. `at` positions any
// resolution / cycle error at the import statement.
func (i *Interpreter) loadModule(importPath string, at parser.Node) (*loadedModule, error) {
	reg := i.modReg
	canonical, err := module.Resolve(importPath, i.baseDir, reg.search)
	if err != nil {
		file, line, col := posFor(at)
		return nil, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}

	// Cycle: the module is already on the load stack.
	for _, p := range reg.stack {
		if p == canonical {
			file, line, col := posFor(at)
			chain := strings.Join(append(append([]string{}, reg.stack...), canonical), " -> ")
			return nil, &runtimeError{Msg: "module cycle: " + chain, File: file, Line: line, Col: col}
		}
	}

	// Run-once: already loaded and initialised.
	if m, ok := reg.cache[canonical]; ok {
		return m, nil
	}

	// Parse the module file (errors are positioned in that file).
	modProg, err := reg.load(canonical)
	if err != nil {
		return nil, err
	}
	if err := checkModuleDeclarationsOnly(modProg); err != nil {
		return nil, err
	}
	exports := collectExports(modProg)
	if err := checkReferentialClosure(modProg, exports); err != nil {
		return nil, err
	}

	// A fresh sub-interpreter is the module's own scope; it shares the
	// registry so its own imports use the same cache / stack.
	sub := New()
	reg.setup(sub)
	sub.modReg = reg
	sub.baseDir = filepath.Dir(canonical)
	sub.isModule = true // enables `export`; a script (CLI Run) rejects it

	reg.stack = append(reg.stack, canonical)
	runErr := sub.Run(modProg) // loads sub's imports (post-order), then runs its body
	reg.stack = reg.stack[:len(reg.stack)-1]
	if runErr != nil {
		return nil, runErr
	}

	m := &loadedModule{interp: sub, path: canonical, ns: moduleStem(canonical), exports: exports}
	reg.cache[canonical] = m
	return m, nil
}
