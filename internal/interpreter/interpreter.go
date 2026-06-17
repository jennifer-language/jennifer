// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

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
// `os.getEnv`, etc. without nested maps. The namespace doubles as the
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
	NSBuiltins      map[nsKey]Builtin           // namespaced builtins: os.getEnv, bio.translate, ...
	NSConstants     map[nsKey]Value             // namespaced constants: os.PLATFORM, ...
	NSStructs       map[nsKey]*parser.StructDef // M15.2: namespaced struct definitions (os.Result, time.Time)
	knownLibs       map[string]bool             // libraries with at least one registered builtin OR constant
	knownNamespaces map[string]bool             // libraries that registered through the namespaced API
	libsWithGlobals map[string]bool             // libraries that registered any RegisterGlobal name (M10+)

	// Per-library registries for globals. RegisterGlobal* writes here at
	// Install time; processImports copies an activated library's entries
	// into the resolution maps (Builtins / LibConstants) and runs
	// collision detection against already-active libraries. Keyed
	// lib -> name -> value so two libraries registering the same global
	// name don't silently overwrite each other.
	globalFnsByLib    map[string]map[string]Builtin
	globalConstsByLib map[string]map[string]Value
	imported          map[string]bool   // libraries the program has `use`d
	nsPrefixes        map[string]string // active call-site prefix -> canonical namespace (after aliasing)
	nsAliasedAway     map[string]string // canonical namespace -> alias chosen by `use NAME as ALIAS;`
	methods           map[string]*parser.MethodDef
	structs           map[string]*parser.StructDef // M13.1: top-level struct definitions hoisted at Run() time
	global            *Environment                 // global scope where top-level statements live

	// M16.0: spawned task registry. Every `spawn { ... }` appends its
	// TaskState here; the CLI scans the slice on shutdown to surface
	// unobserved error tasks (the "loud-fail" stance). The mutex
	// protects the slice itself, not the individual TaskStates (those
	// coordinate via their own `done` channels).
	tasksMu sync.Mutex
	tasks   []*TaskState
}

func New() *Interpreter {
	in := &Interpreter{
		Out:               os.Stdout,
		In:                os.Stdin,
		Builtins:          map[string]builtinEntry{},
		LibConstants:      map[string]libConstantEntry{},
		NSBuiltins:        map[nsKey]Builtin{},
		NSConstants:       map[nsKey]Value{},
		NSStructs:         map[nsKey]*parser.StructDef{},
		knownLibs:         map[string]bool{},
		knownNamespaces:   map[string]bool{},
		libsWithGlobals:   map[string]bool{},
		globalFnsByLib:    map[string]map[string]Builtin{},
		globalConstsByLib: map[string]map[string]Value{},
		imported:          map[string]bool{},
		nsPrefixes:        map[string]string{},
		nsAliasedAway:     map[string]string{},
		methods:           map[string]*parser.MethodDef{},
		structs:           map[string]*parser.StructDef{},
	}
	return in
}

// removedCoreLibraryName is the name of the M15.4-removed library.
// Kept as a constant so `use core;` produces a friendly migration
// error rather than the generic "unknown library" message.
const removedCoreLibraryName = "core"

// RegisterGlobal attaches a builtin function under the given Jennifer
// library name AND exposes it as a bare-name global. After M10 this is
// the high-bar API: it's reserved for `core`'s polymorphic structural
// primitives (`len`, `JENNIFER_VERSION`).
//
// Storage is per-library: the entry goes into `globalFnsByLib[lib][name]`,
// not directly into the global resolution map. processImports copies
// the entry into the resolution map (Builtins) when the library is
// activated by `use lib;`, and runs collision detection against any
// already-active library publishing the same global. The library
// that calls RegisterGlobal also receives a duplicate-`use` collision
// rule (see processImports) - any later `use NAME [as ALIAS];` after
// the first is rejected with "library already in scope."
//
// Exception: if the library is already imported when RegisterGlobal
// runs (auto-loaded `core` at startup), the entry is also written
// directly into the resolution map so the names are immediately live.
func (i *Interpreter) RegisterGlobal(lib, name string, fn Builtin) {
	if i.globalFnsByLib[lib] == nil {
		i.globalFnsByLib[lib] = map[string]Builtin{}
	}
	i.globalFnsByLib[lib][name] = fn
	i.knownLibs[lib] = true
	i.libsWithGlobals[lib] = true
	if i.imported[lib] {
		i.Builtins[name] = builtinEntry{Lib: lib, Fn: fn}
	}
}

// RegisterGlobalConst attaches a library-provided constant under the given
// Jennifer library name AND exposes it globally as a bare uppercase
// identifier (e.g. `JENNIFER_VERSION`). Same M10 high bar as
// RegisterGlobal; same per-library storage and collision-at-use semantics.
func (i *Interpreter) RegisterGlobalConst(lib, name string, value Value) {
	if i.globalConstsByLib[lib] == nil {
		i.globalConstsByLib[lib] = map[string]Value{}
	}
	i.globalConstsByLib[lib][name] = value
	i.knownLibs[lib] = true
	i.libsWithGlobals[lib] = true
	if i.imported[lib] {
		i.LibConstants[name] = libConstantEntry{Lib: lib, Value: value}
	}
}

// RegisterNamespaced attaches a namespaced builtin. The library
// name doubles as the namespace prefix: `in.RegisterNamespaced("os",
// "platform", fn)` makes the function callable as `os.platform()` once
// the program writes `use os;`. This is the M10 default; almost every
// library uses this and only `core` adds dual RegisterGlobal exposure
// on top.
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

// RegisterNamespacedStruct attaches a library-provided struct
// definition behind `<lib>.` (M15.2). User code then writes
// `def x as <lib>.<name>;` to declare a variable of that type and
// `<lib>.<name>{ field: expr, ... }` to construct one. Field access,
// chained lvalues, value semantics, and deep-const all reuse the
// M13.1 user-struct machinery; the difference is only the lookup
// path. Same gating model as the other Register* methods: active
// only after `use <lib>;`. Field shape is fixed at registration
// time; the library can't add fields later.
func (i *Interpreter) RegisterNamespacedStruct(lib, name string, fields []parser.StructField) {
	def := &parser.StructDef{Name: name, Fields: fields}
	i.NSStructs[nsKey{NS: lib, Name: name}] = def
	i.knownLibs[lib] = true
	i.knownNamespaces[lib] = true
}

// LookupNamespacedBuiltin returns the registered builtin for
// (namespace, name) or nil if none is registered. Test-only convenience
// so libraries with namespaced builtins can be exercised end-to-end
// without exporting the internal nsKey type.
func (i *Interpreter) LookupNamespacedBuiltin(ns, name string) Builtin {
	return i.NSBuiltins[nsKey{NS: ns, Name: name}]
}

// availableLibsString returns a sorted, comma-separated list of registered
// library names for use in error messages. "(none)" if nothing was
// installed.
func (i *Interpreter) availableLibsString() string {
	names := make([]string, 0, len(i.knownLibs))
	for n := range i.knownLibs {
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
	// Kind is the symbolic tag surfaced when the error is caught by an
	// M13.2 `try { ... } catch (err) { ... }` block: it becomes
	// `$err.kind`. Empty means "no specific kind"; the wrapper defaults
	// to `"runtime"`. New runtime-error sites should set this to a
	// short snake_case tag (`"out_of_bounds"`, `"type_mismatch"`,
	// ...) so user code can dispatch on it. Existing sites that don't
	// set it keep working - they just appear as kind `"runtime"` to
	// catch blocks.
	Kind string
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

// unhandledLoopFlowError converts a `break` or `continue` that
// reached a boundary it shouldn't have crossed (top of Run, top of a
// method body) into a positioned runtime error. Both signals are only
// valid inside a loop; reaching anywhere else means the source has a
// stray statement.
func unhandledLoopFlowError(r blockResult) error {
	kw := "break"
	if r.hasContinue {
		kw = "continue"
	}
	return &runtimeError{
		Msg:  fmt.Sprintf("`%s` is only valid inside a loop", kw),
		File: r.flowFile, Line: r.flowLine, Col: r.flowCol,
	}
}

// ExitSignal is the sentinel error returned by an `exit;` /
// `exit EXPR;` statement. It bubbles out of every frame to Run /
// EvalInteractive; the CLI catches it and translates Code into the
// process exit status. Distinct from runtimeError so the CLI can tell
// "user asked to terminate cleanly" apart from "interpreter found a
// bug." M11.
type ExitSignal struct {
	Code int
}

func (e *ExitSignal) Error() string {
	return fmt.Sprintf("program requested exit with code %d", e.Code)
}

// ErrorSignal is the sentinel error returned by an M13.2 `throw EXPR;`
// statement, and also produced when a runtime error reaches a `try`
// block (so user code can catch both kinds uniformly). It carries the
// thrown Value - any kind, but the convention is an `Error` struct
// matching the canonicalErrorStructDef shape. Position info captures
// where the `throw` (or originating runtime error) fired. Distinct
// from ExitSignal (uncatchable, program-level escape) and from
// runtimeError (which wraps INTO an ErrorSignal when it enters a try
// block, not the other way around). M13.2.
type ErrorSignal struct {
	Value Value
	File  string
	Line  int
	Col   int
}

func (e *ErrorSignal) Error() string {
	if e.File != "" {
		return fmt.Sprintf("uncaught error at %s:%d:%d: %s", e.File, e.Line, e.Col, e.Value.Display())
	}
	if e.Line != 0 {
		return fmt.Sprintf("uncaught error at %d:%d: %s", e.Line, e.Col, e.Value.Display())
	}
	return "uncaught error: " + e.Value.Display()
}

// Position implements the positioned-error interface used by the CLI.
func (e *ErrorSignal) Position() (file string, line, col int) {
	return e.File, e.Line, e.Col
}

// canonicalErrorStructName is the conventional struct used by the
// runtime to wrap runtime errors for M13.2 catch blocks, and is the
// recommended shape for user-thrown errors. Auto-hoisted by Run and
// EvalInteractive so user code can rely on it without a `def struct`.
const canonicalErrorStructName = "Error"

// canonicalErrorStructDef returns the StructDef the runtime hoists at
// startup. Field order matches the M13.2 spec:
// kind, message, file, line, col.
func canonicalErrorStructDef() *parser.StructDef {
	str := parser.PrimitiveType(parser.TypeString)
	in := parser.PrimitiveType(parser.TypeInt)
	return &parser.StructDef{
		Name: canonicalErrorStructName,
		Fields: []parser.StructField{
			{Name: "kind", Type: str},
			{Name: "message", Type: str},
			{Name: "file", Type: str},
			{Name: "line", Type: in},
			{Name: "col", Type: in},
		},
	}
}

// runtimeErrorToValue converts a *runtimeError into the conventional
// `Error` struct Value so it can be bound to a catch variable. Kind
// falls back to `"runtime"` when the originating site didn't set one.
func runtimeErrorToValue(e *runtimeError) Value {
	kind := e.Kind
	if kind == "" {
		kind = "runtime"
	}
	return StructVal(canonicalErrorStructName, []StructField{
		{Name: "kind", Value: StringVal(kind)},
		{Name: "message", Value: StringVal(e.Msg)},
		{Name: "file", Value: StringVal(e.File)},
		{Name: "line", Value: IntVal(int64(e.Line))},
		{Name: "col", Value: IntVal(int64(e.Col))},
	})
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
// UnwaitedTaskErrors implements the M16.0 exit-time loud-fail.
// Walks the per-run task registry, waits for each unobserved task to
// finish, and returns the errors held by tasks that ended in failure
// without ever being task.wait'd or task.discard'd. Tasks marked
// observed (Phase 3: task.wait on success or rethrow, or
// task.discard) are skipped without waiting - "discard" is the
// fire-and-forget escape hatch the spec promises.
//
// Phase 2 doesn't ship task.wait / discard yet, so every spawn is
// considered unobserved at exit. The CLI prints each returned error
// to stderr and bumps the exit code to 1; if any returned error is
// an *ExitSignal (exit invoked inside the spawn body), the CLI uses
// that ExitSignal's code instead. Unbounded tasks (e.g. a spawn with
// a `while (true)` loop) will hang the program at exit since the
// scan waits for each unobserved task to finish; users opt out by
// calling task.discard once Phase 3 ships, or by ensuring the body
// terminates.
func (i *Interpreter) UnwaitedTaskErrors() []error {
	i.tasksMu.Lock()
	snapshot := append([]*TaskState(nil), i.tasks...)
	i.tasksMu.Unlock()

	var errs []error
	for _, t := range snapshot {
		if t == nil || t.Observed {
			continue
		}
		// Block until the task finishes. Tasks that were already done
		// before the scan reached them receive on a closed channel
		// immediately, so this is no slower than an IsDone check in
		// the common case.
		<-t.Done
		if t.Err != nil {
			errs = append(errs, t.Err)
		}
	}
	return errs
}

// MarkObserved flips the Observed flag on a task so the exit-time
// loud-fail skips it. Used by task.wait (success or rethrow) and
// task.discard once those ship in Phase 3. Exposed here so the
// `task` library can flip the flag without exporting the field
// directly.
func (i *Interpreter) MarkObserved(t *TaskState) {
	if t != nil {
		t.Observed = true
	}
}

// RegisterTaskForTest is the registry-side hook tests use to inject
// a pre-constructed TaskState (already Done, holding a synthetic
// error) so the registry-scan path can be exercised without going
// through evalSpawn. Production code uses registerTask via
// evalSpawn.
func (i *Interpreter) RegisterTaskForTest(t *TaskState) { i.registerTask(t) }

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
	// Structs: hoist before methods so a method body can reference a
	// struct type declared later in source order.
	//
	// The canonical `Error` struct (M13.2) is hoisted first - the
	// runtime wraps every catchable runtime error into a value of this
	// type, so it must be in scope from the program's very first
	// statement. User code may not redefine it; the existing
	// duplicate-struct check catches that as "struct \"Error\" is
	// defined more than once".
	if _, exists := i.structs[canonicalErrorStructName]; !exists {
		i.structs[canonicalErrorStructName] = canonicalErrorStructDef()
	}
	for _, s := range prog.Structs {
		if _, exists := i.structs[s.Name]; exists {
			file, line, col := posFor(s)
			return &runtimeError{Msg: fmt.Sprintf("struct %q is defined more than once", s.Name), File: file, Line: line, Col: col}
		}
		i.structs[s.Name] = s
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
	res, err := i.execStmts(prog.TopLevel, i.global)
	if err != nil {
		return err
	}
	if res.hasBreak || res.hasContinue {
		return unhandledLoopFlowError(res)
	}
	return nil
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
//   - `use core;` is rejected with a migration hint (M15.4 removed
//     the library; `len` is now a built-in, version constants moved
//     to `meta`).
func (i *Interpreter) processImports(prog *parser.Program, repl bool) error {
	// alreadyImported snapshots `imported` at entry. In batch mode it's empty;
	// in REPL it's whatever earlier inputs already activated. The
	// alias-with-globals rule (M10) uses this to silently no-op a repeated
	// `use lib;` in the REPL while still erroring on the in-source
	// duplicate (`use io; use io;` in one batch program).
	alreadyImported := make(map[string]bool, len(i.imported))
	for k := range i.imported {
		alreadyImported[k] = true
	}
	seenThisRun := map[string]bool{}
	for _, imp := range prog.Imports {
		if imp.Name == removedCoreLibraryName {
			file, line, col := posFor(imp)
			return &runtimeError{
				Msg:  "the `core` library was removed in M15.4; `len` is now a language built-in (no import needed) and the version / build constants moved to `meta` (`use meta;` then `meta.VERSION` / `meta.BUILD`)",
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
		// M10 alias-with-globals rule: if the library exposes any
		// RegisterGlobal name, a second `use NAME [as ALIAS];` (in the same
		// batch program) collides on the globals. The first wins; the second
		// is rejected with a positioned error. In the REPL we silently
		// no-op a repeat so re-running a snippet still works.
		duplicate := seenThisRun[imp.Name] || (alreadyImported[imp.Name] && !repl)
		if i.libsWithGlobals[imp.Name] && duplicate {
			file, line, col := posFor(imp)
			return &runtimeError{
				Msg:  fmt.Sprintf("library %q already in scope (it exposes global names; only one `use %s [as ALIAS];` is allowed)", imp.Name, imp.Name),
				File: file, Line: line, Col: col,
			}
		}
		seenThisRun[imp.Name] = true
		// In the REPL, if this library has already been activated in an
		// earlier input we still want the prefix binding to stay - skip the
		// re-registration so we don't double-error or wipe an alias.
		if repl && alreadyImported[imp.Name] {
			continue
		}
		// M10 globals-publishing rules. Two checks before activation:
		//   1. "Alias on a globals-only library is meaningless." If the
		//      library has globals but no namespaced names, `as ALIAS`
		//      has nothing to rename - reject upfront rather than letting
		//      `ALIAS.NAME` fail later at the call site with a confusing
		//      message.
		//   2. "Two libraries cannot publish the same global." If
		//      activating this library would shadow a global already
		//      owned by an active library, reject with a positioned
		//      collision error. In practice `core` is the only library
		//      with globals today, so this is forward-looking; any
		//      future second `RegisterGlobal*`-using library trips here
		//      if it picks a name `core` already owns.
		if i.libsWithGlobals[imp.Name] {
			if imp.AsName != "" && !i.knownNamespaces[imp.Name] {
				file, line, col := posFor(imp)
				return &runtimeError{
					Msg:  fmt.Sprintf("library %q has no namespaced names; `as %s` aliasing is meaningless here", imp.Name, imp.AsName),
					File: file, Line: line, Col: col,
				}
			}
			if other, name, hit := i.findGlobalCollision(imp.Name); hit {
				file, line, col := posFor(imp)
				return &runtimeError{
					Msg:  fmt.Sprintf("library %q collides with already-active library %q on global %q (only one library may publish a given global name)", imp.Name, other, name),
					File: file, Line: line, Col: col,
				}
			}
		}
		i.imported[imp.Name] = true
		// Copy the activated library's globals into the resolution maps.
		// Doing this here (rather than at Register time) is what makes the
		// single-library-imported case work: each library's globals are
		// kept per-library at registration and only the imported one
		// becomes visible.
		for name, fn := range i.globalFnsByLib[imp.Name] {
			i.Builtins[name] = builtinEntry{Lib: imp.Name, Fn: fn}
		}
		for name, val := range i.globalConstsByLib[imp.Name] {
			i.LibConstants[name] = libConstantEntry{Lib: imp.Name, Value: val}
		}
		// Namespace bookkeeping. After M10 every library is namespaced, so
		// every `use` activates a prefix; the flat-only escape hatch is gone.
		// (Exception: a globals-only library activates no namespace prefix -
		// caught above when AsName is set; the bare-`use` case just skips
		// this block because there's nothing in NSBuiltins / NSConstants
		// for the library and a `prefix.NAME` reference will fail with the
		// usual "unknown namespaced reference" error.)
		if !i.knownNamespaces[imp.Name] {
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

// findGlobalCollision checks whether any global name published by `lib`
// is also published by an already-imported library. Returns
// (collidingLib, globalName, true) on the first collision; ("", "",
// false) if none. Used by processImports to reject conflicting
// `use NAME;` activations before they wire up the resolution maps.
func (i *Interpreter) findGlobalCollision(lib string) (string, string, bool) {
	check := func(name string) (string, bool) {
		for other := range i.imported {
			if other == lib {
				continue
			}
			if _, has := i.globalConstsByLib[other][name]; has {
				return other, true
			}
			if _, has := i.globalFnsByLib[other][name]; has {
				return other, true
			}
		}
		return "", false
	}
	for name := range i.globalConstsByLib[lib] {
		if other, hit := check(name); hit {
			return other, name, true
		}
	}
	for name := range i.globalFnsByLib[lib] {
		if other, hit := check(name); hit {
			return other, name, true
		}
	}
	return "", "", false
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
	if _, exists := i.structs[canonicalErrorStructName]; !exists {
		i.structs[canonicalErrorStructName] = canonicalErrorStructDef()
	}
	for _, s := range prog.Structs {
		// REPL: silently re-define so a snippet can redeclare a struct.
		i.structs[s.Name] = s
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
		res, err := i.execStmt(st, i.global)
		if err != nil {
			return Null(), err
		}
		if res.hasBreak || res.hasContinue {
			return Null(), unhandledLoopFlowError(res)
		}
	}
	return last, nil
}

// blockResult carries control flow info out of a block.
//   - hasReturn: a `return` was executed; `value` holds the return value
//     (callers in non-method contexts bubble this up further).
//   - hasBreak: a `break;` was executed (M11). Loop statements catch
//     this and exit; non-loop statements pass it through. A `break`
//     reaching the top level is a positioned runtime error.
//   - hasContinue: a `continue;` was executed (M11). Loop statements
//     catch this and start the next iteration; non-loop statements
//     pass it through. Same misuse rule as break.
//
// At most one of the three flags is true at a time. flowFile / flowLine
// / flowCol carry the source position of the break/continue/return
// statement so an unhandled signal can be reported with the right
// location.
type blockResult struct {
	hasReturn   bool
	hasBreak    bool
	hasContinue bool
	value       Value
	flowFile    string
	flowLine    int
	flowCol     int
}

// flowsOut returns true if any control-flow flag is set - the result
// needs to propagate up through the calling block without executing
// subsequent statements.
func (r blockResult) flowsOut() bool {
	return r.hasReturn || r.hasBreak || r.hasContinue
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
		res, err := i.execStmt(st, env)
		if err != nil {
			return blockResult{}, err
		}
		if res.flowsOut() {
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
	case *parser.FieldAssignStmt:
		return blockResult{}, i.execFieldAssign(st, env)
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
	case *parser.BreakStmt:
		file, line, col := posFor(st)
		return blockResult{hasBreak: true, flowFile: file, flowLine: line, flowCol: col}, nil
	case *parser.ContinueStmt:
		file, line, col := posFor(st)
		return blockResult{hasContinue: true, flowFile: file, flowLine: line, flowCol: col}, nil
	case *parser.RepeatStmt:
		return i.execRepeat(st, env)
	case *parser.ExitStmt:
		return i.execExit(st, env)
	case *parser.TryStmt:
		return i.execTry(st, env)
	case *parser.ThrowStmt:
		return blockResult{}, i.execThrow(st, env)
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
	// M13.1 / M15.2: if the declared type names a struct, verify the
	// struct exists before any other check so an unknown name surfaces
	// as "unknown struct type" rather than a misleading type-mismatch.
	// Bare names look up in i.structs (user-defined); namespaced names
	// resolve the alias prefix first, then look up in i.NSStructs.
	if st.VarType.Kind == parser.TypeStruct {
		if st.VarType.StructNS != "" {
			canonical, err := i.resolveNamespacePrefix(st.VarType.StructNS)
			if err != nil {
				file, line, col := posFor(st)
				return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
			}
			if _, ok := i.NSStructs[nsKey{NS: canonical, Name: st.VarType.StructName}]; !ok {
				file, line, col := posFor(st)
				return &runtimeError{Msg: fmt.Sprintf("unknown struct type %s.%s", st.VarType.StructNS, st.VarType.StructName), File: file, Line: line, Col: col}
			}
			// Stamp the canonical namespace onto the type so subsequent
			// MatchesDeclared / Equal checks compare against the resolved
			// form. Without this, `use os as o; def x as o.Result;` would
			// produce a value tagged ns=os but a declared type tagged
			// ns=o, and they'd mismatch.
			st.VarType.StructNS = canonical
		} else {
			if _, ok := i.structs[st.VarType.StructName]; !ok {
				file, line, col := posFor(st)
				return &runtimeError{Msg: fmt.Sprintf("unknown struct type %q", st.VarType.StructName), File: file, Line: line, Col: col}
			}
		}
	}
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
		// M13.1: structs need access to the interpreter's struct table to
		// populate every field's zero value. Route through a dedicated
		// helper that materialises the full field list (and validates
		// that the named struct actually exists).
		if st.VarType.Kind == parser.TypeStruct {
			zero, err := i.zeroStructFor(st.VarType.StructNS, st.VarType.StructName, st)
			if err != nil {
				return err
			}
			val = zero
		} else {
			val = stampDeclaredType(ZeroFor(st.VarType), st.VarType)
		}
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
	case parser.TypeTask:
		// M16.0: stamp the declared element type onto the task's
		// shared state if it doesn't already have one. The shared
		// pointer means every Value referring to the same task sees
		// the same element type from now on.
		if v.Kind != KindTask || v.Task == nil {
			return v
		}
		if v.Task.ElemTyp == nil && declType.Element != nil {
			et := *declType.Element
			v.Task.ElemTyp = &et
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

	// Route through the unified lvalue walker so mixed chains like
	// `$p.field[0] = ...` work alongside the original `$xs[i][j]` form
	// (M13.1). The walker handles both IndexExpr and FieldAccessExpr
	// nodes; index-only chains still resolve through the same
	// indexInto / writeIndexedSlot helpers as before.
	steps, err := i.collectLvalueSteps(st.Target, env, st)
	if err != nil {
		return err
	}
	rootCopy := binding.Value.Copy()
	if err := i.applyLvalueWrite(&rootCopy, steps, newVal, st); err != nil {
		return err
	}
	if err := env.Assign(rootVar.Name, rootCopy); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// execAppend handles `$xs[] = expr;` (M9) and `$b[] = byte;` (M12).
// Copies the target binding, appends, commits it back. Const-target
// rejection, type check, and per-kind validation all live here.
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
	if binding.Value.Kind != KindList && binding.Value.Kind != KindBytes {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("`$%s[] = ...` requires a list or bytes, got %s", st.Target.Name, binding.Value.Kind), File: file, Line: line, Col: col}
	}
	newVal, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	if binding.Value.Kind == KindBytes {
		if newVal.Kind != KindInt {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes element must be int in [0, 255], got %s", newVal.Kind), File: file, Line: line, Col: col}
		}
		if newVal.Int < 0 || newVal.Int > 255 {
			file, line, col := posFor(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes element value %d out of range [0, 255]", newVal.Int), File: file, Line: line, Col: col}
		}
		rootCopy := binding.Value.Copy()
		rootCopy.Bytes = append(rootCopy.Bytes, byte(newVal.Int))
		if err := env.Assign(st.Target.Name, rootCopy); err != nil {
			file, line, col := posFor(st)
			return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		return nil
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

// execFieldAssign handles `$p.field = expr;` and chained lvalues that
// end at a field (M13.1). The lvalue chain is rooted on a VarExpr,
// just like an IndexAssign, so the same "copy the binding, walk down,
// write at the leaf, reassign" pattern works. We also enforce the
// struct definition's declared type at the leaf write.
func (i *Interpreter) execFieldAssign(st *parser.FieldAssignStmt, env *Environment) error {
	rootVar := findFieldRoot(st.Target)
	if rootVar == nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: "internal: field-assign target has no root variable", File: file, Line: line, Col: col}
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
	// Build the lvalue step list outside-to-leaf - the AST nests with
	// the leaf-most step on the outside (e.g. `$p.a.b.c = ...` has
	// FieldAccess(c, FieldAccess(b, FieldAccess(a, VarExpr(p)))) so we
	// collect from the outside in and reverse.
	steps, err := i.collectLvalueSteps(st.Target, env, st)
	if err != nil {
		return err
	}
	rootCopy := binding.Value.Copy()
	if err := i.applyLvalueWrite(&rootCopy, steps, newVal, st); err != nil {
		return err
	}
	if err := env.Assign(rootVar.Name, rootCopy); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// lvalueStep is one operation in a chained lvalue: either an index
// (for `[i]`) or a field name (for `.field`). Index values are
// pre-evaluated so the walker doesn't need an environment.
type lvalueStep struct {
	isField bool
	field   string
	index   Value
	// Position info for any error raised at this step.
	file string
	line int
	col  int
}

// collectLvalueSteps walks a chained lvalue from its outside in,
// evaluating each `[i]` index, then reverses so the caller gets steps
// in root-to-leaf order.
func (i *Interpreter) collectLvalueSteps(leaf parser.Expr, env *Environment, st parser.Node) ([]lvalueStep, error) {
	var steps []lvalueStep
	for cur := leaf; cur != nil; {
		switch n := cur.(type) {
		case *parser.FieldAccessExpr:
			fl, ln, cl := posFor(n)
			steps = append(steps, lvalueStep{isField: true, field: n.Field, file: fl, line: ln, col: cl})
			cur = n.Target
		case *parser.IndexExpr:
			fl, ln, cl := posFor(n)
			idx, err := i.evalExpr(n.Index, env)
			if err != nil {
				return nil, err
			}
			steps = append(steps, lvalueStep{index: idx, file: fl, line: ln, col: cl})
			cur = n.Target
		case *parser.VarExpr:
			cur = nil
		default:
			file, line, col := posFor(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("internal: unexpected lvalue node %T", n), File: file, Line: line, Col: col}
		}
	}
	// Reverse: AST is leaf-on-outside, we want root-on-outside.
	for l, r := 0, len(steps)-1; l < r; l, r = l+1, r-1 {
		steps[l], steps[r] = steps[r], steps[l]
	}
	return steps, nil
}

// applyLvalueWrite walks a copied root through the step list, descending
// into the structure at each non-leaf step, then writing newVal at the
// leaf. Leaf step semantics match writeIndexedSlot for `[i]` and the
// per-struct-field type check for `.field`.
func (i *Interpreter) applyLvalueWrite(rootCopy *Value, steps []lvalueStep, newVal Value, st parser.Node) error {
	if len(steps) == 0 {
		file, line, col := posFor(st)
		return &runtimeError{Msg: "internal: lvalue write with no steps", File: file, Line: line, Col: col}
	}
	cur := rootCopy
	for k := 0; k < len(steps)-1; k++ {
		next, err := i.lvalueStepInto(cur, steps[k])
		if err != nil {
			return err
		}
		cur = next
	}
	return i.lvalueWriteLeaf(cur, steps[len(steps)-1], newVal, st)
}

// lvalueStepInto descends one level into a struct field or container
// element, returning a *Value pointing into the structure so the next
// step writes through.
func (i *Interpreter) lvalueStepInto(parent *Value, step lvalueStep) (*Value, error) {
	if step.isField {
		if parent.Kind != KindStruct {
			return nil, &runtimeError{Msg: fmt.Sprintf("field access `.%s` requires a struct, got %s", step.field, parent.Kind), File: step.file, Line: step.line, Col: step.col}
		}
		for k := range parent.Fields {
			if parent.Fields[k].Name == step.field {
				return &parent.Fields[k].Value, nil
			}
		}
		return nil, &runtimeError{Msg: fmt.Sprintf("struct %q has no field %q", parent.StructName, step.field), File: step.file, Line: step.line, Col: step.col}
	}
	// Fake a parser.Node for the indexInto call site - it only reads
	// position info from the node.
	return indexInto(parent, step.index, posNode{file: step.file, line: step.line, col: step.col})
}

// lvalueWriteLeaf writes newVal at the leaf step. Field writes consult
// the struct definition for the declared field type so the value can be
// type-checked. Index writes route through writeIndexedSlot which
// already enforces declared element / value types.
func (i *Interpreter) lvalueWriteLeaf(parent *Value, step lvalueStep, newVal Value, st parser.Node) error {
	if step.isField {
		if parent.Kind != KindStruct {
			return &runtimeError{Msg: fmt.Sprintf("field access `.%s` requires a struct, got %s", step.field, parent.Kind), File: step.file, Line: step.line, Col: step.col}
		}
		def, ok := i.lookupStructDef(parent.StructNS, parent.StructName)
		if !ok {
			return &runtimeError{Msg: fmt.Sprintf("internal: struct %q definition missing at assignment", parent.StructName), File: step.file, Line: step.line, Col: step.col}
		}
		for _, decl := range def.Fields {
			if decl.Name != step.field {
				continue
			}
			if !newVal.MatchesDeclared(decl.Type) {
				return &runtimeError{Msg: fmt.Sprintf("field %q of struct %q expects %s, got %s", decl.Name, parent.StructName, decl.Type, newVal.Kind), File: step.file, Line: step.line, Col: step.col}
			}
			for k := range parent.Fields {
				if parent.Fields[k].Name == decl.Name {
					parent.Fields[k].Value = stampDeclaredType(newVal.Copy(), decl.Type)
					return nil
				}
			}
			// Defensive: the field is declared but the runtime value is missing.
			parent.Fields = append(parent.Fields, StructField{Name: decl.Name, Value: stampDeclaredType(newVal.Copy(), decl.Type)})
			return nil
		}
		return &runtimeError{Msg: fmt.Sprintf("struct %q has no field %q", parent.StructName, step.field), File: step.file, Line: step.line, Col: step.col}
	}
	// Index write at the leaf - reuse the existing writer with a
	// synthetic position node so error messages point at the index
	// operation rather than the outer statement.
	return writeIndexedSlot(parent, step.index, newVal, posNode{file: step.file, line: step.line, col: step.col})
}

// posNode lets us synthesise a parser.Node carrying just position info
// for helpers (indexInto, writeIndexedSlot) that expect one.
type posNode struct {
	file string
	line int
	col  int
}

func (p posNode) Pos() (int, int)  { return p.line, p.col }
func (p posNode) Filename() string { return p.file }
func (p posNode) astNode()         {}

// zeroStructFor builds a zero-initialised struct value for the named
// struct. Each field gets its type's zero value, recursing through
// nested struct fields so a `def p as Point;` for
// `def struct Point { name as string, inner as Other };` produces a
// fully-populated value (no nil fields slot through to runtime
// surprises later). M13.1 / M15.2: `ns` is empty for user-defined
// structs and set for library-provided namespaced ones.
func (i *Interpreter) zeroStructFor(ns, name string, st parser.Node) (Value, error) {
	def, ok := i.lookupStructDef(ns, name)
	if !ok {
		file, line, col := posFor(st)
		if ns != "" {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown struct %s.%s", ns, name), File: file, Line: line, Col: col}
		}
		return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown struct %q", name), File: file, Line: line, Col: col}
	}
	fields := make([]StructField, len(def.Fields))
	for k, decl := range def.Fields {
		var fv Value
		if decl.Type.Kind == parser.TypeStruct {
			subNS := decl.Type.StructNS
			if subNS != "" {
				// Field types resolved at registration may carry a
				// call-site alias; canonicalise here. Errors here are
				// non-fatal at this layer: if the prefix can't be
				// resolved we fall through to the bare lookup, which
				// surfaces a clean "unknown struct" error below.
				if canonical, err := i.resolveNamespacePrefix(subNS); err == nil {
					subNS = canonical
				}
			}
			sub, err := i.zeroStructFor(subNS, decl.Type.StructName, st)
			if err != nil {
				return Value{}, err
			}
			fv = sub
		} else {
			fv = stampDeclaredType(ZeroFor(decl.Type), decl.Type)
		}
		fields[k] = StructField{Name: decl.Name, Value: fv}
	}
	if ns != "" {
		return NamespacedStructVal(ns, name, fields), nil
	}
	return StructVal(name, fields), nil
}

// lookupStructDef finds a struct definition by (namespace, name). Bare
// names hit the user-defined table; namespaced names hit the
// M15.2 library-registered table.
func (i *Interpreter) lookupStructDef(ns, name string) (*parser.StructDef, bool) {
	if ns != "" {
		def, ok := i.NSStructs[nsKey{NS: ns, Name: name}]
		return def, ok
	}
	def, ok := i.structs[name]
	return def, ok
}

// findIndexRoot walks an IndexExpr chain back to the underlying VarExpr.
// The parser guarantees that an IndexAssignStmt's Target is rooted on a
// VarExpr (the chain bottom), but production code shouldn't trust that
// invariant blindly: nil indicates "no usable root" and the caller
// surfaces an internal error.
//
// M13.1: the chain may also include FieldAccessExpr nodes (mixed
// `$p.list[0].field` lvalues), so the walker handles both.
func findIndexRoot(ix *parser.IndexExpr) *parser.VarExpr {
	var cur parser.Expr = ix
	for {
		switch n := cur.(type) {
		case *parser.IndexExpr:
			cur = n.Target
		case *parser.FieldAccessExpr:
			cur = n.Target
		case *parser.VarExpr:
			return n
		default:
			return nil
		}
	}
}

// findFieldRoot is findIndexRoot's twin for FieldAssignStmt - both
// chains share the same shape (a mix of `.field` and `[i]` ops rooted
// on a VarExpr).
func findFieldRoot(fa *parser.FieldAccessExpr) *parser.VarExpr {
	var cur parser.Expr = fa
	for {
		switch n := cur.(type) {
		case *parser.IndexExpr:
			cur = n.Target
		case *parser.FieldAccessExpr:
			cur = n.Target
		case *parser.VarExpr:
			return n
		default:
			return nil
		}
	}
}

// indexInto returns a *Value pointing at the slot designated by idx
// within parent. Used by both reads (in evalExpr's IndexExpr case) and
// intermediate steps of index-assign chains. Out-of-bounds list indices
// and missing map keys both error positionally.
// positioned is the minimal interface indexInto / writeIndexedSlot
// need from their statement parameter: just enough to produce
// positioned error messages. parser.Node satisfies it; the M13.1
// synthetic posNode does too without having to implement the rest of
// the unexported-method `Node` interface.
type positioned interface {
	Pos() (line, col int)
	Filename() string
}

// posOf extracts (file, line, col) from any positioned value -
// parser.Node or our synthetic posNode. Use this in helpers that take
// the positioned interface so the same code path serves both.
func posOf(p positioned) (file string, line, col int) {
	line, col = p.Pos()
	return p.Filename(), line, col
}

func indexInto(parent *Value, idx Value, st positioned) (*Value, error) {
	switch parent.Kind {
	case KindList:
		if idx.Kind != KindInt {
			file, line, col := posOf(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("list index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		if n < 0 || n >= len(parent.List) {
			file, line, col := posOf(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("list index %d out of bounds (len %d)", n, len(parent.List)), File: file, Line: line, Col: col}
		}
		return &parent.List[n], nil
	case KindMap:
		for k := range parent.Map {
			if parent.Map[k].Key.Equal(idx) {
				return &parent.Map[k].Value, nil
			}
		}
		file, line, col := posOf(st)
		return nil, &runtimeError{Msg: fmt.Sprintf("map has no entry for key %s", idx.Display()), File: file, Line: line, Col: col}
	}
	file, line, col := posOf(st)
	return nil, &runtimeError{Msg: fmt.Sprintf("cannot index into %s", parent.Kind), File: file, Line: line, Col: col}
}

// writeIndexedSlot sets parent[idx] = newVal. Lists: in-bounds only.
// Maps: existing key updates in place, missing key extends the map
// (insertion order is preserved). Element/value-type mismatches error.
func writeIndexedSlot(parent *Value, idx Value, newVal Value, st positioned) error {
	switch parent.Kind {
	case KindList:
		if idx.Kind != KindInt {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("list index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		if n < 0 || n >= len(parent.List) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("list index %d out of bounds (len %d)", n, len(parent.List)), File: file, Line: line, Col: col}
		}
		if parent.ElemTyp != nil && !newVal.MatchesDeclared(*parent.ElemTyp) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("cannot assign %s to list element of declared type %s", newVal.Kind, parent.ElemTyp), File: file, Line: line, Col: col}
		}
		parent.List[n] = newVal.Copy()
		return nil
	case KindMap:
		if parent.KeyTyp != nil && !idx.MatchesDeclared(*parent.KeyTyp) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("map key must be %s, got %s", parent.KeyTyp, idx.Kind), File: file, Line: line, Col: col}
		}
		if parent.ValTyp != nil && !newVal.MatchesDeclared(*parent.ValTyp) {
			file, line, col := posOf(st)
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
	case KindBytes:
		// M12: byte slot writes accept an int in [0, 255]. Out-of-range
		// writes are positioned runtime errors (same shape as list
		// out-of-bounds), and a non-int RHS is rejected as a type error.
		if idx.Kind != KindInt {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		if n < 0 || n >= len(parent.Bytes) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes index %d out of bounds (len %d)", n, len(parent.Bytes)), File: file, Line: line, Col: col}
		}
		if newVal.Kind != KindInt {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes element must be int in [0, 255], got %s", newVal.Kind), File: file, Line: line, Col: col}
		}
		if newVal.Int < 0 || newVal.Int > 255 {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes element value %d out of range [0, 255]", newVal.Int), File: file, Line: line, Col: col}
		}
		parent.Bytes[n] = byte(newVal.Int)
		return nil
	}
	file, line, col := posOf(st)
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
			if res.hasBreak {
				return blockResult{}, nil
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
			if res.hasBreak {
				return blockResult{}, nil
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
		if res.hasBreak {
			return blockResult{}, nil
		}
		// hasContinue (or no flow): fall through to the next iteration.
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
		if res.hasBreak {
			return blockResult{}, nil
		}
		// On `continue` (or no flow) we still run the step before the
		// next condition check, matching the C-style for loop where
		// `continue` jumps to the step, not past it.
		if st.Step != nil {
			if _, err := i.execStmt(st.Step, forEnv); err != nil {
				return blockResult{}, err
			}
		}
	}
}

// execRepeat runs the body at least once, then re-checks `until`
// AFTER each pass. The loop exits when the condition is true (the
// inversion is the whole reason `until` was picked over `do { } while
// !cond`). Same break/continue handling as the other loops.
func (i *Interpreter) execRepeat(st *parser.RepeatStmt, env *Environment) (blockResult, error) {
	for {
		res, err := i.execBlock(st.Body, env)
		if err != nil {
			return blockResult{}, err
		}
		if res.hasReturn {
			return res, nil
		}
		if res.hasBreak {
			return blockResult{}, nil
		}
		// hasContinue (or no flow): re-evaluate `until` before the next pass.
		done, err := i.evalBool(st.Cond, env, "`repeat ... until` condition")
		if err != nil {
			return blockResult{}, err
		}
		if done {
			return blockResult{}, nil
		}
	}
}

// execExit terminates the program by returning a sentinel `exitSignal`
// error that propagates up through every frame to Run / EvalInteractive.
// The CLI catches the sentinel and translates it into an OS exit code.
// See P4.
func (i *Interpreter) execExit(st *parser.ExitStmt, env *Environment) (blockResult, error) {
	code := int64(0)
	if st.Code != nil {
		v, err := i.evalExpr(st.Code, env)
		if err != nil {
			return blockResult{}, err
		}
		if v.Kind != KindInt {
			file, line, col := posFor(st.Code)
			return blockResult{}, &runtimeError{Msg: fmt.Sprintf("`exit` argument must be int, got %s", v.Kind), File: file, Line: line, Col: col}
		}
		code = v.Int
	}
	return blockResult{}, &ExitSignal{Code: int(code)}
}

// execThrow evaluates the thrown expression and returns an
// *ErrorSignal carrying its value. Position is the `throw` keyword's
// own source location so a top-level uncaught throw points at the
// statement, not deep inside whatever expression built the value.
// M13.2.
func (i *Interpreter) execThrow(st *parser.ThrowStmt, env *Environment) error {
	v, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	file, line, col := posFor(st)
	return &ErrorSignal{Value: v.Copy(), File: file, Line: line, Col: col}
}

// execTry runs the body and, if it produces a catchable error
// (*ErrorSignal from `throw`, or *runtimeError from a builtin /
// language operation), runs the handler with the catch variable
// bound to the thrown value. *ExitSignal propagates uncaught (the
// program-level escape is uncatchable per spec). blockResult flags
// (return/break/continue) flow through unchanged so the surrounding
// method / loop sees them. M13.2.
func (i *Interpreter) execTry(st *parser.TryStmt, env *Environment) (blockResult, error) {
	res, err := i.execStmts(st.Body.Stmts, env)
	if err == nil {
		return res, nil
	}
	// ExitSignal is uncatchable - propagate.
	if _, ok := err.(*ExitSignal); ok {
		return blockResult{}, err
	}
	// Convert err into the catch value.
	var caught Value
	switch e := err.(type) {
	case *ErrorSignal:
		caught = e.Value
	case *runtimeError:
		caught = runtimeErrorToValue(e)
	default:
		// Unknown error type - don't try to catch it; let it propagate.
		return blockResult{}, err
	}
	// Bind the catch variable in a fresh scope, then run the handler.
	// The catch scope shadows nothing because no-shadowing already
	// rejects a CatchName collision with an outer binding at runtime
	// via env.Define.
	catchEnv := NewEnvironment(env)
	declType := parser.StructType(canonicalErrorStructName)
	if caught.Kind != KindStruct || caught.StructName != canonicalErrorStructName {
		// User threw a non-Error value (any kind is permitted by the
		// spec). Bind without a declared type stamp; the catch body
		// uses convert.typeOf / runtime checks if it needs to inspect.
		declType = parser.Type{}
	}
	if err := catchEnv.Define(st.CatchName, caught.Copy(), declType, false); err != nil {
		return blockResult{}, &runtimeError{Msg: err.Error(), File: st.CatchFile, Line: st.CatchLine, Col: st.CatchCol}
	}
	return i.execStmts(st.CatchBody.Stmts, catchEnv)
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
	case *parser.LenExpr:
		return i.evalLen(ex, env)
	case *parser.SpawnExpr:
		return i.evalSpawn(ex, env)
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
	case *parser.StructLit:
		return i.evalStructLit(ex, env)
	case *parser.FieldAccessExpr:
		return i.evalFieldAccess(ex, env)
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

// evalStructLit constructs a struct value from a literal (M13.1). The
// literal's name must match a hoisted top-level `def struct`; every
// declared field of the struct must appear exactly once in the literal
// (no defaults at the literal level - users who want a zero-initialised
// struct write `def p as Point;` without `init`); each value's runtime
// type is checked against the declared field type. Fields are emitted
// in *declaration* order regardless of the literal's source order so
// the resulting Value is canonical.
func (i *Interpreter) evalStructLit(ex *parser.StructLit, env *Environment) (Value, error) {
	// M15.2: namespaced literals (`os.Result{ ... }`) resolve via the
	// alias prefix then the NSStructs table; bare literals use the
	// user-defined struct table as before.
	var def *parser.StructDef
	var resolvedNS string
	if ex.NS != "" {
		canonical, err := i.resolveNamespacePrefix(ex.NS)
		if err != nil {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		d, ok := i.NSStructs[nsKey{NS: canonical, Name: ex.Name}]
		if !ok {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown struct %s.%s", ex.NS, ex.Name), File: file, Line: line, Col: col}
		}
		def = d
		resolvedNS = canonical
	} else {
		d, ok := i.structs[ex.Name]
		if !ok {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown struct %q", ex.Name), File: file, Line: line, Col: col}
		}
		def = d
	}
	// Index the literal's fields by name for the cross-check.
	provided := make(map[string]*parser.StructLitField, len(ex.Fields))
	for k := range ex.Fields {
		provided[ex.Fields[k].Name] = &ex.Fields[k]
	}
	// Reject unknown fields up-front so the user gets one clear error
	// instead of "missing field X" followed by "stray field Y".
	declared := make(map[string]bool, len(def.Fields))
	for _, f := range def.Fields {
		declared[f.Name] = true
	}
	for _, f := range ex.Fields {
		if !declared[f.Name] {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown field %q in struct %q", f.Name, ex.Name), File: f.File, Line: f.Line, Col: f.Col}
		}
	}
	out := make([]StructField, 0, len(def.Fields))
	for _, decl := range def.Fields {
		lit, ok := provided[decl.Name]
		if !ok {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("missing field %q in struct %q literal", decl.Name, ex.Name), File: file, Line: line, Col: col}
		}
		v, err := i.evalExpr(lit.Expr, env)
		if err != nil {
			return Value{}, err
		}
		if !v.MatchesDeclared(decl.Type) {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("field %q of struct %q expects %s, got %s", decl.Name, ex.Name, decl.Type, v.Kind), File: lit.File, Line: lit.Line, Col: lit.Col}
		}
		out = append(out, StructField{Name: decl.Name, Value: stampDeclaredType(v.Copy(), decl.Type)})
	}
	if resolvedNS != "" {
		return NamespacedStructVal(resolvedNS, ex.Name, out), nil
	}
	return StructVal(ex.Name, out), nil
}

// evalFieldAccess reads a struct field. Errors on a non-struct target
// (with a positioned message naming the field that was requested) and
// on an unknown field name (which can happen if the user mistypes a
// field in a getter chain - the struct value carries the field list
// so we can spot it here).
func (i *Interpreter) evalFieldAccess(ex *parser.FieldAccessExpr, env *Environment) (Value, error) {
	parent, err := i.evalExpr(ex.Target, env)
	if err != nil {
		return Value{}, err
	}
	if parent.Kind != KindStruct {
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("field access `.%s` requires a struct, got %s", ex.Field, parent.Kind), File: file, Line: line, Col: col}
	}
	for _, f := range parent.Fields {
		if f.Name == ex.Field {
			return f.Value, nil
		}
	}
	file, line, col := posFor(ex)
	return Value{}, &runtimeError{Msg: fmt.Sprintf("struct %q has no field %q", parent.StructName, ex.Field), File: file, Line: line, Col: col}
}

// evalIndex implements read access for `$xs[i]`, `$m["k"]`, or arbitrary
// nesting. Reads of out-of-bounds list indices and missing map keys are
// positioned runtime errors (no null fallback - that's the M6 decision
// from milestones.md). Bytes (M12) read as int in [0, 255].
func (i *Interpreter) evalIndex(ex *parser.IndexExpr, env *Environment) (Value, error) {
	parent, err := i.evalExpr(ex.Target, env)
	if err != nil {
		return Value{}, err
	}
	idx, err := i.evalExpr(ex.Index, env)
	if err != nil {
		return Value{}, err
	}
	if parent.Kind == KindBytes {
		return readByteAt(parent, idx, ex)
	}
	slot, err := indexInto(&parent, idx, ex)
	if err != nil {
		return Value{}, err
	}
	return *slot, nil
}

// readByteAt returns parent.Bytes[idx] as IntVal, with the same
// out-of-bounds rules the list path uses.
func readByteAt(parent Value, idx Value, node parser.Node) (Value, error) {
	if idx.Kind != KindInt {
		file, line, col := posFor(node)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("bytes index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
	}
	n := int(idx.Int)
	if n < 0 || n >= len(parent.Bytes) {
		file, line, col := posFor(node)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("bytes index %d out of bounds (len %d)", n, len(parent.Bytes)), File: file, Line: line, Col: col}
	}
	return IntVal(int64(parent.Bytes[n])), nil
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
	if isBitOp(b.Op) {
		return i.evalBitOp(b.Op, lv, rv, file, line, col)
	}
	return i.evalArithmetic(b.Op, lv, rv, file, line, col)
}

// isBitOp returns true for the M12 bitwise operators. Kept separate
// from BinaryOp.IsLogical / IsComparison so each category retains its
// own quick-check.
func isBitOp(op parser.BinaryOp) bool {
	switch op {
	case parser.OpBitOr, parser.OpBitXor, parser.OpBitAnd, parser.OpShl, parser.OpShr:
		return true
	}
	return false
}

// evalBitOp evaluates a bitwise operator. Both operands must be `int`;
// `float` is rejected because bit-twiddling a floating-point bit
// pattern is almost always a typo - the user can `convert.toInt` if
// they really mean it. Shift count rules:
//   - Negative count is a runtime error (no implicit reverse-direction).
//   - Count >= 64 is allowed; Go's >> / << produce the "shifted off the
//     end" result (0 for `<<` and for arithmetic `>>` of a non-negative
//     value, -1 for arithmetic `>>` of a negative value). Predictable
//     and matches what hardware does.
func (i *Interpreter) evalBitOp(op parser.BinaryOp, lv, rv Value, file string, line, col int) (Value, error) {
	if lv.Kind != KindInt || rv.Kind != KindInt {
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("operator %s requires int operands, got %s and %s", op, lv.Kind, rv.Kind),
			File: file, Line: line, Col: col,
		}
	}
	a, b := lv.Int, rv.Int
	switch op {
	case parser.OpBitAnd:
		return IntVal(a & b), nil
	case parser.OpBitOr:
		return IntVal(a | b), nil
	case parser.OpBitXor:
		return IntVal(a ^ b), nil
	case parser.OpShl, parser.OpShr:
		if b < 0 {
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("shift count must be non-negative, got %d", b),
				File: file, Line: line, Col: col,
			}
		}
		// Cap the visible shift at 64: Go's runtime panics for >= 64
		// with `>>` of an int when count is uint, but produces a
		// predictable result if we mask it ourselves.
		if b >= 64 {
			if op == parser.OpShl {
				return IntVal(0), nil
			}
			// Arithmetic right shift of >= 64 saturates to 0 or -1.
			if a < 0 {
				return IntVal(-1), nil
			}
			return IntVal(0), nil
		}
		if op == parser.OpShl {
			return IntVal(a << uint(b)), nil
		}
		return IntVal(a >> uint(b)), nil
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown bitwise operator %s", op), File: file, Line: line, Col: col}
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
	case parser.OpBitNot:
		if v.Kind != KindInt {
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("unary `~` requires int, got %s", v.Kind),
				File: file, Line: line, Col: col,
			}
		}
		return IntVal(^v.Int), nil
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

// evalLen is the runtime side of the M15.4 `len(EXPR)` language
// built-in. Polymorphic across the four kinds where "structural
// length" is well-defined; any other kind is a positioned runtime
// error. The shape mirrors what the old core.lenFn did, but the
// invocation is a parser-level primary expression rather than a
// library function call.
func (i *Interpreter) evalLen(ex *parser.LenExpr, env *Environment) (Value, error) {
	v, err := i.evalExpr(ex.Operand, env)
	if err != nil {
		return Value{}, err
	}
	switch v.Kind {
	case KindString:
		// Rune count (Unicode code points), not byte count.
		return IntVal(int64(utf8.RuneCountInString(v.Str))), nil
	case KindList:
		return IntVal(int64(len(v.List))), nil
	case KindMap:
		return IntVal(int64(len(v.Map))), nil
	case KindBytes:
		return IntVal(int64(len(v.Bytes))), nil
	}
	file, line, col := posFor(ex)
	return Value{}, &runtimeError{Msg: fmt.Sprintf("len() expects a string, list, map or bytes, got %s", v.Kind), File: file, Line: line, Col: col}
}

// evalSpawn implements `spawn { ... }` (M16.0). Phase 2 launches a
// goroutine that runs the body and signals completion via the
// TaskState's done channel; evalSpawn itself returns immediately with
// a wrapping Value whose Task field points at the same shared state.
// The spawn frame receives a deep-copy snapshot of every binding
// visible in the caller's scope chain (see snapshotForSpawn), so the
// goroutine never touches caller-owned data after spawn returns -
// value-semantics capture is what keeps the model data-race-free by
// construction.
//
// Three signals that don't fit the "task captures the body's result"
// model get special handling:
//
//   - `exit EXPR;` inside the spawn terminates the whole program. The
//     ExitSignal travels up through the goroutine boundary on the
//     panic / synchronous-return path (it's recorded as the task's
//     `Err`, and the registry scan at exit re-raises it so the CLI
//     observes the requested exit code). Spec: exit is not a task
//     error to be recovered via task.wait.
//   - `break` / `continue` inside the body with no enclosing loop
//     surface as the same misuse-of-loop-flow error a method body
//     would produce. They live on the task's Err field; the body
//     "completed" with an error.
//   - `throw` / runtime errors become the task's Err normally.
//
// The task's declared element type is left for the caller (Define /
// Assign) to enforce via MatchesDeclared; this function just records
// the body's return value when the goroutine finishes. Phase 3
// surfaces the result via task.wait.
func (i *Interpreter) evalSpawn(ex *parser.SpawnExpr, env *Environment) (Value, error) {
	spawnEnv := i.snapshotForSpawn(env)
	state := &TaskState{Done: make(chan struct{})}
	i.registerTask(state)

	go i.runSpawn(state, ex, spawnEnv)
	return wrapTask(state), nil
}

// runSpawn is the goroutine body for a spawned block. It executes the
// body, classifies the result into Result / Err, and closes Done so
// every observer (task.wait future-phase, the registry scan, the
// display form) sees the same final state. Writes to state happen
// before the close; readers must observe close before reading.
func (i *Interpreter) runSpawn(state *TaskState, ex *parser.SpawnExpr, spawnEnv *Environment) {
	defer close(state.Done)

	res, err := i.execBlock(&parser.Block{Stmts: ex.Body}, spawnEnv)
	if err != nil {
		state.Err = err
		return
	}
	if res.hasBreak || res.hasContinue {
		state.Err = unhandledLoopFlowError(res)
		return
	}
	if res.hasReturn {
		state.Result = res.value
	} else {
		state.Result = Null()
	}
}

// registerTask appends a freshly-spawned task to the per-run registry.
// The registry feeds the exit-time loud-fail scan (UnwaitedTaskErrors).
func (i *Interpreter) registerTask(state *TaskState) {
	i.tasksMu.Lock()
	i.tasks = append(i.tasks, state)
	i.tasksMu.Unlock()
}

// effectiveGlobal returns the env that should serve as the "global"
// parent for a fresh user-method call frame. In ordinary (single
// goroutine) execution this is i.global. Inside a spawned goroutine
// the caller's env chain terminates at the spawn snapshot (parent=nil
// by construction in snapshotForSpawn), so the outermost ancestor is
// the snapshot itself. Routing method-call frames through that
// snapshot - instead of the live i.global the parent goroutine is
// still mutating - is what makes spawn bodies that call user
// functions data-race-free.
func effectiveGlobal(env *Environment) *Environment {
	cur := env
	for cur != nil && cur.parent != nil {
		cur = cur.parent
	}
	return cur
}

// snapshotForSpawn flattens every binding visible in the caller's
// scope chain - including top-level definitions in the global frame -
// into a fresh environment with no parent. Deep-copying every value
// means the spawned body can mutate its own copies without affecting
// the caller. Detaching the parent means name lookups stop at the
// snapshot frame, so writes to names that originally lived in an
// outer scope (including the global) don't propagate back. Methods,
// libraries, and namespaced constants live on the Interpreter struct
// (`i.methods`, `i.Builtins`, `i.NSBuiltins`, ...), not the env
// chain, so the detached frame still resolves them through the
// regular evalCall / evalQualified* paths. The no-shadowing rule
// prevents collisions; we keep the innermost binding (most-specific
// wins) if a name somehow appears twice.
func (i *Interpreter) snapshotForSpawn(env *Environment) *Environment {
	// Two-frame snapshot:
	//   1. globals - copies of i.global's bindings only. effectiveGlobal
	//      walks here, so user-method calls inside the spawn see exactly
	//      the same global surface they would in serial code (the
	//      no-shadowing rule doesn't trip on captured locals).
	//   2. locals  - copies of every non-global binding visible at the
	//      spawn site, chained on top of (1).
	// Both frames hold copies, so any post-spawn parent-goroutine writes
	// to i.global or to the caller frame don't reach the spawn body.
	globalSnap := NewEnvironment(nil)
	if i.global != nil {
		for name, b := range i.global.vars {
			globalSnap.vars[name] = Binding{
				Value:    b.Value.Copy(),
				DeclType: b.DeclType,
				IsConst:  b.IsConst,
			}
		}
	}
	localSnap := NewEnvironment(globalSnap)
	for cur := env; cur != nil && cur != i.global; cur = cur.parent {
		for name, b := range cur.vars {
			if _, exists := localSnap.vars[name]; exists {
				continue
			}
			localSnap.vars[name] = Binding{
				Value:    b.Value.Copy(),
				DeclType: b.DeclType,
				IsConst:  b.IsConst,
			}
		}
	}
	return localSnap
}

// wrapTask builds the KindTask Value from a completed (or pending,
// post-Phase 2) TaskState. The element type is unknown at this point;
// the caller's Define / Assign check enforces it via MatchesDeclared.
func wrapTask(state *TaskState) Value {
	return Value{Kind: KindTask, Task: state}
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
		callFrame := NewEnvironment(effectiveGlobal(env))
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
		if res.hasBreak || res.hasContinue {
			// A `break` or `continue` in the method body that wasn't
			// caught by an inner loop is a misuse - they don't cross
			// the method-call boundary into the caller's loop.
			return Value{}, unhandledLoopFlowError(res)
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
