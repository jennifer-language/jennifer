// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"fmt"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/module"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// moduleReg is the module registry shared across one program run: the
// run-once cache (canonical path -> loaded module), the in-progress load
// stack for cycle detection, the module search path, and the callbacks the
// loader needs to turn a resolved path into a runnable module. Every module
// loads into a fresh sub-interpreter that shares this registry, so
// run-once, depth-first post-order init, and cycle detection all fall out
// of the recursion.
type moduleReg struct {
	cache      map[string]*loadedModule
	stack      []string                                            // canonical paths currently loading
	search     []string                                            // module search dirs (sysmoddir, then -I dirs)
	vendorRoot string                                              // root for `@scope/package` deck imports ("" = none)
	load       func(canonicalPath string) (*parser.Program, error) // lex/preproc/parse a module file
	setup      func(*Interpreter)                                  // install the standard library into a module interpreter
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
func retagStructs(v Value, fromNS, toNS, fromMod, toMod string, isOwn func(string) bool) Value {
	// Copy-on-write: most values crossing a module boundary contain no
	// module-own struct (nor a declared type that names one), so return them
	// unchanged instead of deep-copying every list / map / struct. The
	// allocation-free precheck below is complete - it inspects both the values
	// and the declared element / key / value types retagType would rewrite.
	if !subtreeNeedsRetag(v, fromNS, fromMod, isOwn) {
		return v
	}
	switch v.Kind {
	case KindStruct:
		nv := v
		nv.Fields = make([]StructField, len(v.Fields))
		for i, f := range v.Fields {
			nv.Fields[i] = StructField{Name: f.Name, Value: retagStructs(f.Value, fromNS, toNS, fromMod, toMod, isOwn)}
		}
		// Match on (StructNS, ModPath): a foreign struct that merely shares
		// the stem has a different ModPath, so it is left untouched.
		if v.StructNS == fromNS && v.ModPath == fromMod && isOwn(v.StructName) {
			nv.StructNS = toNS
			nv.ModPath = toMod
		}
		return nv
	case KindList:
		nv := v
		nv.List = make([]Value, len(v.List))
		for i := range v.List {
			nv.List[i] = retagStructs(v.List[i], fromNS, toNS, fromMod, toMod, isOwn)
		}
		nv.ElemTyp = retagType(v.ElemTyp, fromNS, toNS, fromMod, toMod, isOwn)
		return nv
	case KindMap:
		nv := v
		nv.Map = make([]MapEntry, len(v.Map))
		for i := range v.Map {
			// Retag both key and value: a `map of Struct to V` returned across
			// the boundary must retag its struct keys too, or the consumer's
			// `$m[$k]` never matches (and retagType already retags KeyTyp).
			nv.Map[i] = MapEntry{
				Key:   retagStructs(v.Map[i].Key, fromNS, toNS, fromMod, toMod, isOwn),
				Value: retagStructs(v.Map[i].Value, fromNS, toNS, fromMod, toMod, isOwn),
			}
		}
		nv.KeyTyp = retagType(v.KeyTyp, fromNS, toNS, fromMod, toMod, isOwn)
		nv.ValTyp = retagType(v.ValTyp, fromNS, toNS, fromMod, toMod, isOwn)
		return nv
	default:
		return v
	}
}

// subtreeNeedsRetag reports whether v contains any struct value, or any
// declared element / key / value type, that retagStructs / retagType would
// rewrite. It must stay in lock-step with those two functions: a false
// negative would skip a needed retag. Allocation-free.
func subtreeNeedsRetag(v Value, fromNS, fromMod string, isOwn func(string) bool) bool {
	switch v.Kind {
	case KindStruct:
		if v.StructNS == fromNS && v.ModPath == fromMod && isOwn(v.StructName) {
			return true
		}
		for _, f := range v.Fields {
			if subtreeNeedsRetag(f.Value, fromNS, fromMod, isOwn) {
				return true
			}
		}
	case KindList:
		if typeNeedsRetag(v.ElemTyp, fromNS, fromMod, isOwn) {
			return true
		}
		for _, e := range v.List {
			if subtreeNeedsRetag(e, fromNS, fromMod, isOwn) {
				return true
			}
		}
	case KindMap:
		if typeNeedsRetag(v.KeyTyp, fromNS, fromMod, isOwn) || typeNeedsRetag(v.ValTyp, fromNS, fromMod, isOwn) {
			return true
		}
		for _, e := range v.Map {
			if subtreeNeedsRetag(e.Key, fromNS, fromMod, isOwn) || subtreeNeedsRetag(e.Value, fromNS, fromMod, isOwn) {
				return true
			}
		}
	}
	return false
}

// typeNeedsRetag reports whether a declared type names a retaggable module
// struct (the type-metadata counterpart to subtreeNeedsRetag). Must mirror
// retagType.
func typeNeedsRetag(t *parser.Type, fromNS, fromMod string, isOwn func(string) bool) bool {
	if t == nil {
		return false
	}
	if t.Kind == parser.TypeStruct && t.StructNS == fromNS && t.ModPath == fromMod && isOwn(t.StructName) {
		return true
	}
	return typeNeedsRetag(t.Element, fromNS, fromMod, isOwn) ||
		typeNeedsRetag(t.KeyType, fromNS, fromMod, isOwn) ||
		typeNeedsRetag(t.ValType, fromNS, fromMod, isOwn)
}

// retagType returns a copy of a declared type with every struct type whose
// namespace is `from` and whose name is one of the module's own structs
// re-tagged to `to`, recursing through list / map element types. It mirrors
// retagStructs for the *type* metadata a list or map carries alongside its
// elements, so a `list of mod.Struct` handed back into the module reads as
// `list of Struct` (and vice versa) rather than failing the param-type check.
func retagType(t *parser.Type, fromNS, toNS, fromMod, toMod string, isOwn func(string) bool) *parser.Type {
	if t == nil {
		return nil
	}
	nt := *t
	if t.Kind == parser.TypeStruct && t.StructNS == fromNS && t.ModPath == fromMod && isOwn(t.StructName) {
		nt.StructNS = toNS
		nt.ModPath = toMod
	}
	nt.Element = retagType(t.Element, fromNS, toNS, fromMod, toMod, isOwn)
	nt.KeyType = retagType(t.KeyType, fromNS, toNS, fromMod, toMod, isOwn)
	nt.ValType = retagType(t.ValType, fromNS, toNS, fromMod, toMod, isOwn)
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

// SetVendorRoot sets the root directory for `@scope/package` deck imports (see
// module.FindVendorRoot). Empty leaves `@` imports erroring with guidance. Must
// be called after EnableModules; a no-op if the module system is not enabled.
func (i *Interpreter) SetVendorRoot(dir string) {
	if i.modReg != nil {
		i.modReg.vendorRoot = dir
	}
}

// loadModuleImports processes a program's `import "..."` statements before
// its body runs, so a module is fully initialised before the code that
// imports it (depth-first post-order). Errors here are load-time errors:
// they fail the program before the importer's body and are not catchable
// (an `import` is a declaration, not an expression).
// The `repl` flag tolerates re-importing the same module under the same alias
// across inputs (the REPL re-runs bindModuleAlias on every submission, and a
// module is cached / run-once), so a user can re-run an `import` snippet.
func (i *Interpreter) loadModuleImports(prog *parser.Program, repl bool) error {
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
		if err := i.bindModuleAlias(alias, m, mi, repl); err != nil {
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
func (i *Interpreter) bindModuleAlias(alias string, m *loadedModule, at parser.Node, repl bool) error {
	if _, taken := i.nsPrefixes[alias]; taken {
		file, line, col := posFor(at)
		return &runtimeError{Msg: fmt.Sprintf("module alias %q collides with an imported library namespace; import the module `as` a different name", alias), File: file, Line: line, Col: col}
	}
	if i.moduleAliases == nil {
		i.moduleAliases = map[string]*loadedModule{}
	}
	if existing, dup := i.moduleAliases[alias]; dup {
		// REPL: re-importing the *same* module under the same alias across
		// submissions is a harmless no-op (a module is run-once / cached), so a
		// re-run snippet works. Binding the alias to a *different* module is
		// still a real collision.
		if repl && existing == m {
			return nil
		}
		file, line, col := posFor(at)
		return &runtimeError{Msg: fmt.Sprintf("module alias %q is already bound; import the module `as` a different name", alias), File: file, Line: line, Col: col}
	}
	i.moduleAliases[alias] = m
	return nil
}

// findImpostorStruct walks v (struct fields, list elements, map keys and
// values) for a struct whose name matches one of the module's own structs but
// whose identity is not the module's. Depth-first; returns the first offender.
// Allocation-free, same traversal shape as subtreeNeedsRetag.
func findImpostorStruct(v Value, m *loadedModule) (Value, bool) {
	switch v.Kind {
	case KindStruct:
		// An impostor is a *bare* struct (no module identity) whose name is
		// module-owned: exactly what the outward retag would re-brand to the
		// module's identity, completing the impersonation. A struct carrying a
		// foreign identity instead - e.g. a `net.Conn` field named "Conn"
		// nested in a genuine module struct - is never re-branded (its NS is
		// not the module's) and would fail the param-type check on its own, so
		// it is not an impostor and must not be flagged here.
		if m.isOwnStruct(v.StructName) && v.StructNS == "" && v.ModPath == "" {
			return v, true
		}
		for _, f := range v.Fields {
			if imp, found := findImpostorStruct(f.Value, m); found {
				return imp, true
			}
		}
	case KindList:
		for _, e := range v.List {
			if imp, found := findImpostorStruct(e, m); found {
				return imp, true
			}
		}
	case KindMap:
		for _, e := range v.Map {
			if imp, found := findImpostorStruct(e.Key, m); found {
				return imp, true
			}
			if imp, found := findImpostorStruct(e.Value, m); found {
				return imp, true
			}
		}
	}
	return Value{}, false
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
		// Reject an impostor: a struct whose name matches one of the module's
		// own structs but whose identity is not the module's (a
		// consumer-defined bare struct sharing the name). Without this it would
		// pass the module's param-type check - which sees the module's structs
		// as bare - then blow up mid-body or be re-branded on return. The check
		// recurses: an impostor nested inside a list / map / struct field
		// crosses the boundary just as unchecked as a top-level one.
		if imp, found := findImpostorStruct(v, m); found {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("argument %d to %s.%s: struct %q is not %s.%s (a consumer-defined struct cannot impersonate a module struct)", idx+1, m.ns, c.Callee, imp.StructName, m.ns, imp.StructName), File: file, Line: line, Col: col}
		}
		// Cross the boundary inward: a module struct the consumer holds is
		// tagged `(module-stem, name)`; inside the module it is bare.
		args[idx] = retagStructs(v, m.ns, "", m.path, "", m.isOwnStruct)
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
	return retagStructs(v, "", m.ns, "", m.path, m.isOwnStruct), nil
}

// stampModuleStructType verifies `alias.Struct` names an exported struct of the
// module and rewrites the declared type from the importer's alias to the
// module's identity: StructNS to the stem (display) and ModPath to the module's
// canonical path (identity), so it matches the identity a module value carries
// once it crosses the boundary - and stays distinct from a same-stem struct in
// another module.
func (i *Interpreter) stampModuleStructType(t *parser.Type, mod *loadedModule, at parser.Node) error {
	file, line, col := posFor(at)
	if !mod.isOwnStruct(t.StructName) {
		return &runtimeError{Msg: fmt.Sprintf("module %q has no struct %q", t.StructNS, t.StructName), File: file, Line: line, Col: col}
	}
	if !mod.exports[t.StructName] {
		return &runtimeError{Msg: fmt.Sprintf("%s.%s: struct %q is not exported from module %q", t.StructNS, t.StructName, t.StructName, t.StructNS), File: file, Line: line, Col: col}
	}
	t.StructNS = mod.ns
	t.ModPath = mod.path
	return nil
}

// resolveDeclaredStructNS resolves every struct type reachable through a
// declared type - the type itself plus any list / map / task element types -
// rewriting an importer's module alias to the module's stem and a library
// alias to its canonical namespace, and verifying each named struct exists. It
// is the recursive form of the per-struct resolution, so a `list of
// alias.Struct` (or `map of K to alias.Struct`) element type is stamped just
// like a bare `alias.Struct` and matches the identity a value carries once it
// crosses the boundary. Non-struct types (scalars, list-of-int, ...) simply
// recurse into their nil sub-types and are left untouched.
func (i *Interpreter) resolveDeclaredStructNS(t *parser.Type, at parser.Node) error {
	if t == nil {
		return nil
	}
	// Already stamped (idempotent no-op). A shared node re-reached from a loop
	// body or a concurrent goroutine reads this flag and returns without
	// touching StructNS, so there is no re-stamp - and no write-write race.
	if t.Resolved {
		return nil
	}
	if t.Kind == parser.TypeStruct {
		// A module struct is named either by the importer's alias (first pass)
		// or, once stamped, by the module's own stem. Recognising the stem
		// keeps re-resolution idempotent: this same DefineStmt type is
		// re-resolved every time the `def` runs (e.g. once per loop
		// iteration), and after the first pass its namespace is the stem, not
		// the alias. moduleByNS also lets a value's canonical stem name the
		// type directly.
		mod, ok := i.moduleAliases[t.StructNS]
		if !ok {
			if byStem := i.moduleByNS(t.StructNS); byStem != nil && byStem.isOwnStruct(t.StructName) {
				mod, ok = byStem, true
			}
		}
		if ok {
			// `alias.Struct` (or already-stamped `stem.Struct`) naming a module
			// struct: verify it is exported and stamp the module's namespace.
			if err := i.stampModuleStructType(t, mod, at); err != nil {
				return err
			}
		} else if t.StructNS != "" {
			canonical, err := i.resolveNamespacePrefix(t.StructNS)
			if err != nil {
				file, line, col := posFor(at)
				return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
			}
			if _, ok := i.NSStructs[nsKey{NS: canonical, Name: t.StructName}]; !ok {
				file, line, col := posFor(at)
				return &runtimeError{Msg: fmt.Sprintf("unknown struct type %s.%s", t.StructNS, t.StructName), File: file, Line: line, Col: col}
			}
			t.StructNS = canonical
		} else {
			if _, ok := i.structs[t.StructName]; !ok {
				file, line, col := posFor(at)
				return &runtimeError{Msg: fmt.Sprintf("unknown struct type %q", t.StructName), File: file, Line: line, Col: col}
			}
		}
	}
	if err := i.resolveDeclaredStructNS(t.Element, at); err != nil {
		return err
	}
	if err := i.resolveDeclaredStructNS(t.KeyType, at); err != nil {
		return err
	}
	if err := i.resolveDeclaredStructNS(t.ValType, at); err != nil {
		return err
	}
	// Mark resolved only after the whole type (and its element / key / value
	// sub-types) resolved cleanly, so a type a best-effort pass left unstamped
	// (an unknown or aliased-canonical name) is retried - and errors - at
	// execution, at its original position.
	t.Resolved = true
	return nil
}

// resolveDeclaredTypesOnce stamps every declared struct type in the program a
// single time, before execution begins, so the per-execution re-resolve in
// execDefine is a guarded no-op write. Method bodies and spawn bodies are
// shared AST that concurrent goroutines re-execute, so stamping the node here -
// once, single-threaded, from Run before any goroutine launches - is what keeps
// a `def x as mod.Struct` reached from two goroutines off a write-write race on
// the shared type node. Best-effort: an unresolvable type is left unstamped for
// execDefine to error on at execution, so error timing and position are
// unchanged (dead code that never runs still never errors). Runs after
// loadModuleImports because it needs i.moduleAliases populated.
func (i *Interpreter) resolveDeclaredTypesOnce(prog *parser.Program) {
	if prog == nil {
		return
	}
	for _, s := range prog.TopLevel {
		i.declTypesStmt(s)
	}
	for _, m := range prog.Methods {
		if m == nil {
			continue
		}
		// Stamp parameter types so a `func f(s as mod.Struct)` param carries the
		// module's (stem, path) identity and matches the value passed in - the
		// arg check at the call site is the only place a module-struct param is
		// compared, and it happens in the module's own interpreter.
		for pi := range m.Params {
			_ = i.resolveDeclaredStructNS(&m.Params[pi].Type, m)
		}
		if m.Body == nil {
			continue
		}
		for _, s := range m.Body.Stmts {
			i.declTypesStmt(s)
		}
	}
}

func (i *Interpreter) declTypesBlock(b *parser.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		i.declTypesStmt(s)
	}
}

func (i *Interpreter) declTypesStmt(s parser.Stmt) {
	switch st := s.(type) {
	case *parser.DefineStmt:
		// Best-effort: ignore the error so an unresolvable type still errors
		// at execution (in execDefine) at its original position.
		_ = i.resolveDeclaredStructNS(&st.VarType, st)
		i.declTypesExpr(st.InitExpr)
	case *parser.AssignStmt:
		i.declTypesExpr(st.Value)
	case *parser.IndexAssignStmt:
		i.declTypesExpr(st.Target)
		i.declTypesExpr(st.Value)
	case *parser.AppendStmt:
		i.declTypesExpr(st.Target)
		i.declTypesExpr(st.Value)
	case *parser.FieldAssignStmt:
		i.declTypesExpr(st.Target)
		i.declTypesExpr(st.Value)
	case *parser.IfStmt:
		i.declTypesExpr(st.Cond)
		i.declTypesBlock(st.Then)
		for idx := range st.ElseIfs {
			i.declTypesExpr(st.ElseIfs[idx])
			i.declTypesBlock(st.ElseIfBodies[idx])
		}
		i.declTypesBlock(st.Else)
	case *parser.WhileStmt:
		i.declTypesExpr(st.Cond)
		i.declTypesBlock(st.Body)
	case *parser.ForStmt:
		i.declTypesStmt(st.Init)
		i.declTypesExpr(st.Cond)
		i.declTypesStmt(st.Step)
		i.declTypesBlock(st.Body)
	case *parser.ForEachStmt:
		i.declTypesExpr(st.Coll)
		i.declTypesBlock(st.Body)
	case *parser.RepeatStmt:
		i.declTypesBlock(st.Body)
		i.declTypesExpr(st.Cond)
	case *parser.ReturnStmt:
		i.declTypesExpr(st.Value)
	case *parser.ExitStmt:
		i.declTypesExpr(st.Code)
	case *parser.ThrowStmt:
		i.declTypesExpr(st.Value)
	case *parser.DeferStmt:
		// A spawn (whose body a goroutine re-executes) can hide in the
		// deferred call's arguments: `defer f(spawn { ... });`.
		i.declTypesExpr(st.Call)
	case *parser.TryStmt:
		i.declTypesBlock(st.Body)
		i.declTypesBlock(st.CatchBody)
	case *parser.ExprStmt:
		i.declTypesExpr(st.Expr)
	case *parser.Block:
		i.declTypesBlock(st)
	}
}

// declTypesExpr descends only where a DefineStmt can hide - inside a spawn
// body (whose statements a goroutine re-executes) and inside sub-expressions
// that may nest a spawn.
func (i *Interpreter) declTypesExpr(e parser.Expr) {
	switch ex := e.(type) {
	case nil:
		return
	case *parser.SpawnExpr:
		for _, s := range ex.Body {
			i.declTypesStmt(s)
		}
	case *parser.CallExpr:
		for _, a := range ex.Args {
			i.declTypesExpr(a)
		}
	case *parser.QualifiedCallExpr:
		for _, a := range ex.Args {
			i.declTypesExpr(a)
		}
	case *parser.BinaryExpr:
		i.declTypesExpr(ex.Left)
		i.declTypesExpr(ex.Right)
	case *parser.UnaryExpr:
		i.declTypesExpr(ex.Operand)
	case *parser.LenExpr:
		i.declTypesExpr(ex.Operand)
	case *parser.IndexExpr:
		i.declTypesExpr(ex.Target)
		i.declTypesExpr(ex.Index)
	case *parser.FieldAccessExpr:
		i.declTypesExpr(ex.Target)
	case *parser.ListLit:
		for _, el := range ex.Elements {
			i.declTypesExpr(el)
		}
	case *parser.MapLit:
		for k := range ex.Keys {
			i.declTypesExpr(ex.Keys[k])
			i.declTypesExpr(ex.Values[k])
		}
	case *parser.StructLit:
		for _, f := range ex.Fields {
			i.declTypesExpr(f.Expr)
		}
	}
}

// moduleByNS returns a loaded module whose namespace (file stem) is ns, or
// nil. It recognises an already-stamped module struct type, whose StructNS is
// the stem rather than an importer alias, so resolveDeclaredStructNS is
// idempotent across repeated executions of the same declaration.
func (i *Interpreter) moduleByNS(ns string) *loadedModule {
	if ns == "" {
		return nil
	}
	// Return a match only when the stem identifies exactly one module. Two
	// modules can now share a stem (they are distinguished by canonical path),
	// so a bare-stem lookup is ambiguous - fall through to nil (forcing the
	// alias) rather than pick one arbitrarily.
	var found *loadedModule
	for _, m := range i.moduleAliases {
		if m.ns == ns {
			if found != nil && found.path != m.path {
				return nil
			}
			found = m
		}
	}
	return found
}

// moduleByPath returns the loaded module with the given canonical path, or
// nil. Unlike moduleByNS this is never ambiguous: the canonical path IS the
// module identity, so it resolves correctly even when two loaded modules
// share a file stem. Prefer it whenever a Value / Type carries a ModPath.
func (i *Interpreter) moduleByPath(path string) *loadedModule {
	if path == "" {
		return nil
	}
	for _, m := range i.moduleAliases {
		if m.path == path {
			return m
		}
	}
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
	return retagStructs(b.Value, "", m.ns, "", m.path, m.isOwnStruct), nil
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
	canonical, err := module.Resolve(importPath, i.baseDir, reg.search, reg.vendorRoot)
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
	sub.isModule = true                  // enables `export`; a script (CLI Run) rejects it
	sub.host = i.Host()                  // entry program, so meta.callMain reaches its handlers
	sub.moduleNS = moduleStem(canonical) // stem, for meta.callMain struct retagging (display)
	sub.modulePath = canonical           // canonical path, the struct-identity half

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
