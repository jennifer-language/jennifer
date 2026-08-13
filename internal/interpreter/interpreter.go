// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// BuiltinCtx is the I/O context the interpreter passes to every builtin.
// `Out` is where stdout-like effects write (e.g. `printf`); `In` is the
// reader stdin-consuming builtins read from (e.g. `readLine`); `InREPL`
// is true when the call originates inside the interactive REPL, so
// stdin-consuming builtins can refuse rather than racing the line
// editor for input.
type BuiltinCtx struct {
	Out    io.Writer
	Err    io.Writer
	In     io.Reader
	InREPL bool
	// Call-site position, so a builtin that raises a Jennifer error (via
	// RaiseError) can anchor it at the call - e.g. testing assertions point
	// at the failing `testing.assertEqual(...)` line. Zero when unknown.
	File string
	Line int
	Col  int
	// Depth is the caller frame's call-depth counter (env.depth at the call
	// site). A dispatch builtin that re-enters the interpreter - meta.call /
	// meta.callMain - threads it into CallByNameWithDepth / CallHostWithDepth so
	// the re-entered chain keeps accumulating on the caller's goroutine-local
	// counter instead of resetting: that is what keeps recursion that bounces
	// through such a builtin tripping the catchable call-depth guard rather than
	// overflowing the Go stack fatally. nil when unknown (the counter then starts
	// fresh, as at a true root entry).
	Depth *int
	// interp is the owning interpreter, set at the call site. It backs Invoke so a
	// higher-order builtin (lists.map / filter / reduce / ...) can call back a
	// function-value argument without every library importing the dispatch core.
	// Unexported: libraries receive a BuiltinCtx, they never construct one.
	interp *Interpreter
	// Cancel is the caller goroutine's running task state (env.root.cancel):
	// non-nil inside a spawn body, nil on the main goroutine. task.cancelled()
	// reads its Cancelled flag as a non-raising poll, so a spawn body can check for
	// cooperative cancellation at an arbitrary point (the loop-checkpoint raise
	// stays the mechanism for stopping a loop).
	Cancel *TaskState
}

// Invoke calls a first-class function value with args, threading the caller's
// call-depth counter so recursion through the callback still trips the catchable
// depth guard. It is the callback bridge for higher-order builtins (lists.map /
// filter / reduce / ...). Errors if fn is not a callable `func` value.
func (c BuiltinCtx) Invoke(fn Value, args ...Value) (Value, error) {
	if fn.Kind != KindFunc {
		return Value{}, fmt.Errorf("expected a `func` value, got %s", fn.Kind)
	}
	if fn.Fn == nil {
		return Value{}, fmt.Errorf("call of an uninitialized `func` value")
	}
	if c.interp == nil {
		return Value{}, fmt.Errorf("no interpreter bound to this call context")
	}
	return c.interp.callMethodWithDepth(fn.Fn, c.Depth, args...)
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
	Err             io.Writer // defaults to os.Stderr if nil
	In              io.Reader // defaults to os.Stdin if nil
	InREPL          bool      // set by the REPL so stdin-consuming builtins refuse
	Builtins        map[string]builtinEntry
	LibConstants    map[string]libConstantEntry // library-provided constants (math.PI, ...)
	NSBuiltins      map[nsKey]Builtin           // namespaced builtins: os.getEnv, bio.translate, ...
	NSConstants     map[nsKey]Value             // namespaced constants: os.PLATFORM, ...
	NSStructs       map[nsKey]*parser.StructDef // namespaced struct definitions (os.Result, time.Time)
	objectTypes     map[nsKey]bool              // opaque object types (json.Value): registered like structs, but KindObject at runtime
	knownLibs       map[string]bool             // libraries with at least one registered builtin OR constant
	knownNamespaces map[string]bool             // libraries that registered through the namespaced API
	libsWithGlobals map[string]bool             // libraries that registered any RegisterGlobal name

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
	structs           map[string]*parser.StructDef // top-level struct definitions hoisted at Run() time
	enums             map[string]*parser.EnumDef   // top-level enum definitions hoisted at Run() time
	global            *Environment                 // global scope where top-level statements live

	// Module system: the registry shared across a program run (cache,
	// cycle stack, search path, loader), and this interpreter's source
	// directory for resolving local imports. Wired by EnableModules; a nil
	// registry means module imports are not available here.
	modReg  *moduleReg
	baseDir string
	// moduleAliases maps an importer's `import "..." as NAME;` alias (or the
	// file stem) to the loaded module, so `NAME.member` resolves into that
	// module's own interpreter. Populated by loadModuleImports; nil until a
	// program imports a module.
	moduleAliases map[string]*loadedModule
	// isModule is true when this interpreter is running a file loaded as a
	// module (via loadModule), false for a top-level script (CLI Run) or the
	// REPL. It gates `export`: a module publishes names, a script may not.
	isModule bool

	// entryGlobalsImmutable is set by Run for a non-module entry program when
	// the program declares no mutable top-level global (only `def const` /
	// `func` / `def struct` / `def enum`). It widens read-only-parameter borrow
	// (argBorrowed) to a single-file script: with no mutable global for an
	// argument to alias and a body to mutate, the borrow aliasing hole is
	// unrepresentable, exactly as in a module. Left false for the REPL, whose
	// mutable global set grows across inputs.
	entryGlobalsImmutable bool

	// host is the entry-program interpreter for a module sub-interpreter, or
	// nil on the entry program itself. meta.callMain / meta.definedMain resolve
	// against it, so a framework module (e.g. `web`) can dispatch by name to
	// handler methods defined in the program that imported it. Set in
	// loadModule via Host(), so nested module loads still point at the ultimate
	// entry program, not an intermediate module.
	host *Interpreter
	// moduleNS is this sub-interpreter's module stem (e.g. "web"), used to
	// retag its own struct values between the internal bare identity and the
	// (stem, name) identity the entry program sees when meta.callMain crosses
	// the boundary. Empty on the entry program.
	moduleNS string
	// modulePath is this sub-interpreter's canonical path, the struct-identity
	// half of the retag (StructNS is the stem for display, ModPath is the path
	// for identity). Empty on the entry program.
	modulePath string

	// spawned task registry. Every `spawn { ... }` appends its
	// TaskState here; the CLI scans the slice on shutdown to surface
	// unobserved error tasks (the "loud-fail" stance). The mutex
	// protects the slice itself, not the individual TaskStates (those
	// coordinate via their own `done` channels).
	tasksMu       sync.Mutex
	tasks         []*TaskState
	taskCompactAt int   // registry length that triggers pruning observed tasks
	spawnTotal    int64 // monotonic count of every task spawned this run (registry prunes, so len(tasks) is not a total); for diagnostics

	// diagReq is set by RequestDiagnostics (wired to SIGUSR1 by the CLI) and
	// checked at loop-iteration / method-call checkpoints. A signal goroutine
	// only stores the flag; the snapshot is printed on the interpreter goroutine,
	// so it reads interpreter state race-free. The per-checkpoint Load is a plain
	// memory load (free on the hot path); the flag is false unless SIGUSR1 fired.
	diagReq atomic.Bool

	// Profiling (optional dev feature; nil = off, the only cost on the hot
	// path being a nil check). Set via SetProfiler; the concrete collector
	// lives in internal/profile and is wired by `jennifer profile`. The
	// three flags gate the three instrumentation streams so an unused one
	// costs nothing. The statement timer's self/cumulative split
	// accumulates nested-statement time in the root Environment
	// (Environment.profChild), not here, so parallel `spawn` bodies each
	// use their own snapshot root instead of racing a shared field.
	prof       Profiler
	profStmts  bool
	profCalls  bool
	profAllocs bool
	// profModules propagates the profiler into imported modules'
	// sub-interpreters, so their statements are attributed to their own file
	// positions. Off by default (and left off by `test --coverage`, which
	// reuses the profiler for the entry program only); `jennifer profile`
	// turns it on.
	profModules bool
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
		objectTypes:       map[nsKey]bool{},
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
		enums:             map[string]*parser.EnumDef{},
	}
	return in
}

// removedCoreLibraryName is the name of the removed library.
// Kept as a constant so `use core;` produces a friendly migration
// error rather than the generic "unknown library" message.
const removedCoreLibraryName = "core"

// RegisterGlobal attaches a builtin function under the given Jennifer
// library name AND exposes it as a bare-name global. This is
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
// identifier (e.g. `JENNIFER_VERSION`). Same high bar as
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
// the program writes `use os;`. This is the default; almost every
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
// definition behind `<lib>.`. User code then writes
// `def x as <lib>.<name>;` to declare a variable of that type and
// `<lib>.<name>{ field: expr, ... }` to construct one. Field access,
// chained lvalues, value semantics, and deep-const all reuse the
// user-struct machinery; the difference is only the lookup
// path. Same gating model as the other Register* methods: active
// only after `use <lib>;`. Field shape is fixed at registration
// time; the library can't add fields later.
func (i *Interpreter) RegisterNamespacedStruct(lib, name string, fields []parser.StructField) {
	def := &parser.StructDef{Name: name, Fields: fields}
	i.NSStructs[nsKey{NS: lib, Name: name}] = def
	i.knownLibs[lib] = true
	i.knownNamespaces[lib] = true
}

// RegisterNamespacedObject registers an opaque, library-owned object type
// (e.g. json.Value). It registers like a fieldless namespaced struct so
// `def r as json.Value` parses and type-checks, but the runtime values are
// KindObject (built via interpreter.ObjectVal) - operators, `[index]`, and
// `.field` all reject them, and only the owning library's accessors reach
// inside. The library supplies the values through its own builtins. An
// optional display func renders the wrapped payload for Display() (REPL echo,
// `%v`); pass nil to fall back to the bare `<ns.name>` form.
func (i *Interpreter) RegisterNamespacedObject(lib, name string, display ObjectDisplayer) {
	i.RegisterNamespacedStruct(lib, name, nil)
	i.objectTypes[nsKey{NS: lib, Name: name}] = true
	registerObjectDisplayer(lib, name, display)
}

// isObjectType reports whether (ns, name) names a registered opaque object
// type rather than an ordinary namespaced struct.
func (i *Interpreter) isObjectType(ns, name string) bool {
	return i.objectTypes[nsKey{NS: ns, Name: name}]
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
	// `try { ... } catch (err) { ... }` block: it becomes
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
// bug."
type ExitSignal struct {
	Code int
}

func (e *ExitSignal) Error() string {
	return fmt.Sprintf("program requested exit with code %d", e.Code)
}

// ErrorSignal is the sentinel error returned by a `throw EXPR;`
// statement, and also produced when a runtime error reaches a `try`
// block (so user code can catch both kinds uniformly). It carries the
// thrown Value - any kind, but the convention is an `Error` struct
// matching the canonicalErrorStructDef shape. Position info captures
// where the `throw` (or originating runtime error) fired. Distinct
// from ExitSignal (uncatchable, program-level escape) and from
// runtimeError (which wraps INTO an ErrorSignal when it enters a try
// block, not the other way around).
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
// runtime to wrap runtime errors for catch blocks, and is the
// recommended shape for user-thrown errors. Auto-hoisted by Run and
// EvalInteractive so user code can rely on it without a `def struct`.
const canonicalErrorStructName = "Error"

// canonicalErrorStructDef returns the StructDef the runtime hoists at
// startup. Field order matches the spec:
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

// checkStructCycles rejects any struct that would contain itself by value -
// directly (`def struct Node { next as Node }`) or mutually (A holds a B that
// holds an A). Such a struct has no finite zero value under value semantics
// (there is no null / pointer struct field to terminate it), so materializing it
// would recurse forever and stack-overflow; this turns that fatal crash into a
// positioned, actionable error. Recursion *through* a `list` / `map` / `task`
// field is fine - those have a finite (empty / handle) zero - so only a direct
// struct-typed field is a by-value edge, and a module- or library-struct field
// (namespace / module path set) cannot cycle back to a local struct. Runs once
// after all top-level structs are hoisted, so mutual cycles across the whole set
// are visible.
func (i *Interpreter) checkStructCycles() error {
	const white, gray, black = 0, 1, 2
	color := make(map[string]int, len(i.structs))
	var visit func(name string) error
	visit = func(name string) error {
		def, ok := i.structs[name]
		if !ok {
			return nil // not a local struct; unknown-type errors surface elsewhere
		}
		color[name] = gray
		for _, f := range def.Fields {
			t := f.Type
			// Only a direct, local user struct field forms a by-value edge.
			if t.Kind != parser.TypeStruct || t.StructNS != "" || t.ModPath != "" {
				continue
			}
			switch color[t.StructName] {
			case gray:
				return &runtimeError{
					Msg: fmt.Sprintf("struct %q cannot contain itself by value (field %q is %q); a by-value struct cycle has no finite zero value - use `list of %s` (or a `map`) for recursive data",
						name, f.Name, t.StructName, t.StructName),
					File: f.File, Line: f.Line, Col: f.Col,
				}
			case white:
				if err := visit(t.StructName); err != nil {
					return err
				}
			}
		}
		color[name] = black
		return nil
	}
	// Sorted so the reported cycle is stable across runs (map order is random).
	names := make([]string, 0, len(i.structs))
	for name := range i.structs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if color[name] == white {
			if err := visit(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkEnumZeroCycles rejects an enum whose zero value (its first declared
// variant, payload zeroed) would recurse forever. A recursive enum is fine as
// long as its first variant is a safe base case (`enum List { Nil, Cons { tail
// as List } }` zeroes to Nil), but an enum whose *first* variant contains the
// enum by value - directly or through a struct / another enum's first variant -
// has no finite zero and would stack-overflow at materialisation. Only local
// by-value edges count (a `list` / `map` field, or a module / library type,
// terminates); struct-only cycles are already caught by checkStructCycles, so
// this runs after it and surfaces the enum-involving ones.
func (i *Interpreter) checkEnumZeroCycles() error {
	const white, gray, black = 0, 1, 2
	color := make(map[string]int)
	// localNamedDep returns ("E"|"S", name, true) when t is a direct local enum
	// or struct field type - the only by-value zero edges.
	localNamedDep := func(t parser.Type) (string, string, bool) {
		if t.Kind != parser.TypeStruct || t.StructNS != "" || t.ModPath != "" {
			return "", "", false
		}
		if _, ok := i.enums[t.StructName]; ok {
			return "E", t.StructName, true
		}
		if _, ok := i.structs[t.StructName]; ok {
			return "S", t.StructName, true
		}
		return "", "", false
	}
	var visit func(kind, name, rootEnum string) error
	visit = func(kind, name, rootEnum string) error {
		key := kind + ":" + name
		switch color[key] {
		case gray:
			ed := i.enums[rootEnum]
			file, line, col := "", 0, 0
			if ed != nil {
				file, line, col = posFor(ed)
			}
			return &runtimeError{
				Msg:  fmt.Sprintf("enum %q has no finite zero value: its first variant recurses into itself by value; put a payload-less (or non-recursive) variant first, or recurse through a `list` / `map` field", rootEnum),
				File: file, Line: line, Col: col,
			}
		case black:
			return nil
		}
		color[key] = gray
		// An enum's zero uses only its first variant; a struct's uses all fields.
		var deps []parser.StructField
		if kind == "E" {
			if ed := i.enums[name]; ed != nil && len(ed.Variants) > 0 {
				deps = ed.Variants[0].Fields
			}
		} else if sd := i.structs[name]; sd != nil {
			deps = sd.Fields
		}
		for _, f := range deps {
			if dk, dn, ok := localNamedDep(f.Type); ok {
				if err := visit(dk, dn, rootEnum); err != nil {
					return err
				}
			}
		}
		color[key] = black
		return nil
	}
	names := make([]string, 0, len(i.enums))
	for name := range i.enums {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if color["E:"+name] == white {
			if err := visit("E", name, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// ClassifyError extracts the (kind, message, file, line, col) tuple
// from any interpreter error. Exported so libraries that intercept
// errors at the Go level (`testing`) can populate their
// Jennifer-visible result structs without duplicating the
// classification logic.
//
// The `kind` values are the same strings surfaced to Jennifer via
// `try`/`catch`:
//
//   - `"runtime"` (or the runtimeError's own Kind if set) - the
//     built-in class of positioned interpreter errors
//     (out-of-bounds, missing key, type mismatch, ...).
//   - `"error"` - a user `throw`; if the thrown value was an
//     `Error` struct, its fields override this tuple.
//   - `"exit"` - an `ExitSignal`; `message` is
//     "exit code N".
//   - `"unknown"` - anything else (e.g. a boundary error from a
//     Go-side library that doesn't go through *runtimeError).
//
// Positions are zero when the underlying error didn't carry them
// (or is a non-Jennifer error).
func ClassifyError(err error) (kind, message, file string, line, col int) {
	if err == nil {
		return "", "", "", 0, 0
	}
	if re, ok := err.(*runtimeError); ok {
		k := re.Kind
		if k == "" {
			k = "runtime"
		}
		return k, re.Msg, re.File, re.Line, re.Col
	}
	if es, ok := err.(*ErrorSignal); ok {
		// If the thrown value is an Error struct, prefer its fields
		// (matches how try/catch presents them).
		if es.Value.Kind == KindStruct && es.Value.StructName == canonicalErrorStructName {
			var k, m, f string
			var ln, cl int
			for _, fld := range es.Value.Fields {
				switch fld.Name {
				case "kind":
					if fld.Value.Kind == KindString {
						k = fld.Value.Str
					}
				case "message":
					if fld.Value.Kind == KindString {
						m = fld.Value.Str
					}
				case "file":
					if fld.Value.Kind == KindString {
						f = fld.Value.Str
					}
				case "line":
					if fld.Value.Kind == KindInt {
						ln = int(fld.Value.Int)
					}
				case "col":
					if fld.Value.Kind == KindInt {
						cl = int(fld.Value.Int)
					}
				}
			}
			if k == "" {
				k = "error"
			}
			return k, m, f, ln, cl
		}
		return "error", es.Value.Display(), es.File, es.Line, es.Col
	}
	if ex, ok := err.(*ExitSignal); ok {
		return "exit", fmt.Sprintf("exit code %d", ex.Code), "", 0, 0
	}
	return "unknown", err.Error(), "", 0, 0
}

// NewErrorValue builds the canonical `Error` struct Value
// (kind, message, file, line, col). Exported so Go-level libraries can
// construct the exact error shape Jennifer's `try`/`catch` and the testing
// runner understand, without duplicating the field layout.
func NewErrorValue(kind, message, file string, line, col int) Value {
	return StructVal(canonicalErrorStructName, []StructField{
		{Name: "kind", Value: StringVal(kind)},
		{Name: "message", Value: StringVal(message)},
		{Name: "file", Value: StringVal(file)},
		{Name: "line", Value: IntVal(int64(line))},
		{Name: "col", Value: IntVal(int64(col))},
	})
}

// RaiseError returns a catchable *ErrorSignal wrapping a canonical `Error`
// struct. A library builtin returns this to throw a Jennifer error that
// `try`/`catch` catches and `testing.run` classifies by `kind` - the path
// testing assertions use. Pass the call-site position from BuiltinCtx so the
// error anchors at the call.
func RaiseError(kind, message, file string, line, col int) error {
	return &ErrorSignal{
		Value: NewErrorValue(kind, message, file, line, col),
		File:  file,
		Line:  line,
		Col:   col,
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
// UnwaitedTaskErrors implements the exit-time loud-fail.
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
		if t == nil || t.Observed.Load() {
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
		t.Observed.Store(true)
	}
}

// RegisterTaskForTest is the registry-side hook tests use to inject
// a pre-constructed TaskState (already Done, holding a synthetic
// error) so the registry-scan path can be exercised without going
// through evalSpawn. Production code uses registerTask via
// evalSpawn.
func (i *Interpreter) RegisterTaskForTest(t *TaskState) { i.registerTask(t) }

// CallByName invokes a top-level user method by name, with no
// arguments. Exported so libraries that need to dispatch by string
// name can do so - the `testing` library uses this to run
// user-defined test methods.
//
// The method must exist and take zero parameters; anything else
// surfaces as a positioned runtime error. The call runs against the
// interpreter's global env (same shape as calling the method from
// top-level source), and every downstream error (runtimeError,
// ErrorSignal, ExitSignal) propagates unchanged. The caller
// decides how to classify each sentinel.
func (i *Interpreter) CallByName(name string) (Value, error) {
	m, ok := i.methods[name]
	if !ok {
		return Value{}, fmt.Errorf("method %q is not defined", name)
	}
	if len(m.Params) != 0 {
		return Value{}, fmt.Errorf("method %q takes %d parameter(s); CallByName only invokes zero-parameter methods", name, len(m.Params))
	}
	return i.callMethodWithDepth(m, nil)
}

// CallByNameWith invokes a top-level user method by name, binding args to its
// parameters in order with the same arity and declared-type checks as a normal
// call. The variadic sibling to CallByName (which stays the zero-arg compat
// entrypoint); used by testing.runWith and framework dispatchers that reach
// methods by string name with runtime-computed argument lists.
func (i *Interpreter) CallByNameWith(name string, args ...Value) (Value, error) {
	return i.CallByNameWithDepth(name, nil, args...)
}

// CallByNameWithDepth is CallByNameWith threading the caller's call-depth
// counter (see BuiltinCtx.Depth), so recursion that bounces through meta.call
// keeps accumulating on the caller's goroutine-local counter and trips the
// catchable depth guard instead of overflowing the Go stack. A nil counter
// starts fresh (a true root entry).
func (i *Interpreter) CallByNameWithDepth(name string, callerDepth *int, args ...Value) (Value, error) {
	m, ok := i.methods[name]
	if !ok {
		return Value{}, fmt.Errorf("method %q is not defined", name)
	}
	return i.callMethodWithDepth(m, callerDepth, args...)
}

// CallMethodWith dispatches an already-resolved *MethodDef with the given args,
// skipping the name-map lookup CallByNameWith does. It is the shared dispatch
// core: CallByNameWith delegates here after its lookup, and the stamped
// module-method fast path (dispatchModuleMethod) calls it with the *MethodDef
// resolveQualifiedRefs cached on the call node.
func (i *Interpreter) CallMethodWith(m *parser.MethodDef, args ...Value) (Value, error) {
	return i.callMethodWithDepth(m, nil, args...)
}

// callMethodWithDepth is the shared dispatch core. callerDepth threads the
// caller's goroutine-local call-depth counter (env.depth at the dispatch site)
// so a call chain that re-enters the interpreter across a boundary - a module
// call (dispatchModuleMethod), meta.call / meta.callMain - keeps accumulating on
// one counter and trips the catchable call-depth guard, while staying isolated
// from other goroutines (each spawn worker carries its own counter). A nil
// callerDepth mints a fresh counter: the correct behavior at a true root entry
// (the CLI invoking the entry program, a `testing` harness) where there is no
// caller chain to continue and each entry should stand alone.
func (i *Interpreter) callMethodWithDepth(m *parser.MethodDef, callerDepth *int, args ...Value) (Value, error) {
	if len(args) != len(m.Params) {
		return Value{}, fmt.Errorf("method %q takes %d parameter(s), got %d", m.Name, len(m.Params), len(args))
	}
	if i.global == nil {
		i.global = NewEnvironment(nil)
	}
	callFrame := borrowBlockEnv(effectiveGlobal(i.global), len(m.Params))
	dc := callerDepth
	if dc == nil {
		entryDepth := 0
		dc = &entryDepth
	}
	callFrame.depth = dc
	borrowCtx := i.methodBorrowCtx(m)
	for idx, p := range m.Params {
		if !args[idx].MatchesDeclared(p.Type) {
			releaseBlockEnv(callFrame)
			return Value{}, fmt.Errorf("argument %d to %q must be %s, got %s", idx+1, m.Name, p.Type, args[idx].Kind)
		}
		bound := i.bindArg(args[idx], p, borrowCtx)
		if err := callFrame.DefineAt(idx, p.Name, bound, p.Type, false); err != nil {
			releaseBlockEnv(callFrame)
			return Value{}, err
		}
	}
	// Count this cross-boundary dispatch as one call-depth unit: it adds Go
	// stack frames just like an ordinary call, so recursion that bounces through
	// the dispatch entry (meta.call self-recursion, a module method that calls
	// back via meta.callMain) trips the catchable guard instead of overflowing
	// the Go stack fatally. Balanced by the decrement below on every exit path.
	*dc++
	if *dc > limits.MaxCallDepth {
		*dc--
		releaseBlockEnv(callFrame)
		return Value{}, &runtimeError{
			Msg: fmt.Sprintf("call stack too deep: exceeded %d nested method calls (possible infinite recursion)", limits.MaxCallDepth),
		}
	}
	res, err := i.execBlock(m.Body, callFrame)
	*dc--
	releaseBlockEnv(callFrame)
	if err != nil {
		return Value{}, err
	}
	if res.hasBreak || res.hasContinue {
		return Value{}, unhandledLoopFlowError(res)
	}
	if res.hasReturn {
		return res.value, nil
	}
	return Null(), nil
}

// MethodNames returns the names of every top-level user method
// currently defined. Exported so a test runner can enumerate tests
// (e.g. by common prefix) without a separate registration mechanism.
// Order matches the map iteration order (not source order); callers
// that need stable ordering should sort the result.
func (i *Interpreter) MethodNames() []string {
	out := make([]string, 0, len(i.methods))
	for name := range i.methods {
		out = append(out, name)
	}
	return out
}

// HasMethod reports whether a top-level user method with the given name is
// defined. The companion to CallByName / CallByNameWith, so a dispatcher can
// validate a handler name before invoking it (meta.defined builds on this).
func (i *Interpreter) HasMethod(name string) bool {
	_, ok := i.methods[name]
	return ok
}

// Host returns the entry-program interpreter: this interpreter itself when it
// is the entry program (or the REPL), or the program that transitively
// imported this module. meta.callMain / meta.definedMain use it so a framework
// module can reach the entry program's top-level methods (its request
// handlers) - a capability module isolation otherwise denies, granted only
// through this explicit primitive.
func (i *Interpreter) Host() *Interpreter {
	if i.host != nil {
		return i.host
	}
	return i
}

// CallHostWith invokes an entry-program method by name from a module, binding
// args to its parameters. It retags any of the module's own struct arguments
// from their internal bare identity to the (stem, name) identity the entry
// program sees - the same crossing a module's return values make outward - and
// retags a returned module struct back inward. On the entry program itself
// (Host() == i) it is just CallByNameWith. Backs meta.callMain.
func (i *Interpreter) CallHostWith(name string, args ...Value) (Value, error) {
	return i.CallHostWithDepth(name, nil, args...)
}

// CallHostWithDepth is CallHostWith threading the caller's call-depth counter
// (see BuiltinCtx.Depth); meta.callMain passes it so a handler dispatched into
// the host keeps accumulating call depth on the caller's goroutine-local counter
// (recursion through the hop still trips the catchable guard) while staying
// isolated from other concurrent workers. A nil counter starts fresh.
func (i *Interpreter) CallHostWithDepth(name string, callerDepth *int, args ...Value) (Value, error) {
	host := i.Host()
	if host == i {
		return i.CallByNameWithDepth(name, callerDepth, args...)
	}
	retagged := make([]Value, len(args))
	for idx, a := range args {
		retagged[idx] = retagStructs(a, "", i.moduleNS, "", i.modulePath, i.isOwnStructName)
	}
	res, err := host.CallByNameWithDepth(name, callerDepth, retagged...)
	if err != nil {
		return res, err
	}
	return retagStructs(res, i.moduleNS, "", i.modulePath, "", i.isOwnStructName), nil
}

// isOwnStructName reports whether name is a struct declared in this
// interpreter (used by CallHostWith's retagging).
func (i *Interpreter) isOwnStructName(name string) bool {
	// Error is auto-injected into every interpreter, so it is never a module's
	// *declared* struct - retagging it would stamp a module's Error with the
	// module identity and make it unbindable to a host `as Error` parameter.
	if name == canonicalErrorStructName {
		return false
	}
	if _, ok := i.structs[name]; ok {
		return true
	}
	_, ok := i.enums[name]
	return ok
}

// SetModuleContext marks this interpreter as running a module (rather than a
// script), which permits `export`. The `jennifer test` white-box overlay path
// uses it: a `MODULE_test.j` spliced onto its `MODULE.j` is run as the module
// it tests, so the module's `export` markers are legal.
func (i *Interpreter) SetModuleContext(b bool) { i.isModule = b }

func (i *Interpreter) Run(prog *parser.Program) error {
	if i.Out == nil {
		i.Out = os.Stdout
	}
	if i.Err == nil {
		i.Err = os.Stderr
	}
	if i.In == nil {
		i.In = os.Stdin
	}
	// `export` publishes a module's names; it is meaningless in a script.
	// A module interpreter (loadModule) sets isModule; a script (CLI Run) or
	// the REPL does not, so an `export` here is a positioned error.
	if !i.isModule {
		if err := rejectExportInScript(prog); err != nil {
			return err
		}
	}
	// the scope-analysis pass runs here so callers that
	// obtained a *Program via parser.Parse (which itself no longer
	// resolves) still get slot annotations before execution.
	// Idempotent - re-resolving an already-resolved program produces
	// the same annotations.
	if err := parser.Resolve(prog); err != nil {
		return err
	}
	if err := i.processImports(prog, false); err != nil {
		return err
	}
	// Structs: hoist before methods so a method body can reference a
	// struct type declared later in source order.
	//
	// The canonical `Error` struct is hoisted first - the
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
	// Enums are hoisted alongside structs and share the type namespace, so an
	// enum may not collide with a struct (or another enum) of the same name.
	for _, e := range prog.Enums {
		if _, exists := i.structs[e.Name]; exists {
			file, line, col := posFor(e)
			return &runtimeError{Msg: fmt.Sprintf("enum %q collides with a struct of the same name", e.Name), File: file, Line: line, Col: col}
		}
		if _, exists := i.enums[e.Name]; exists {
			file, line, col := posFor(e)
			return &runtimeError{Msg: fmt.Sprintf("enum %q is defined more than once", e.Name), File: file, Line: line, Col: col}
		}
		i.enums[e.Name] = e
	}
	if err := i.checkStructCycles(); err != nil {
		return err
	}
	if err := i.checkEnumZeroCycles(); err != nil {
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
	// Pre-size the global slot slice so top-level `def`s fill fixed slots
	// instead of growing the slice one entry at a time (O(n^2) for n globals).
	if prog.NumGlobals > 0 {
		i.global.slots = make([]Binding, prog.NumGlobals)
	}
	// Module imports load (and their modules initialise) before this
	// program's body runs - depth-first post-order, so an imported module
	// is fully initialised before the code that imports it.
	if err := i.loadModuleImports(prog, false); err != nil {
		return err
	}
	// Pre-resolve every QualifiedCallExpr / QualifiedConstRefExpr against the
	// now-populated namespace / builtin / const tables and module-alias table so
	// the runtime skips the per-access lookup on the hot path. Runs after
	// processImports (library namespaces) AND loadModuleImports (module aliases +
	// their loaded const values), since both dictate which prefixes are valid and
	// module-const stamping needs the imported module's constants to exist.
	i.resolveQualifiedRefs(prog)
	// Per-function escape analysis for read-only-parameter borrow: mark each
	// entry-program method that (transitively over its named calls) mutates no
	// global, so its never-written params may be borrowed even when the script
	// holds mutable globals elsewhere. Runs after loadModuleImports +
	// resolveQualifiedRefs so module-alias and namespace tables are populated
	// (module calls and callback builtins are the analysis's unsafe cases). A
	// module borrows via isModule, so this is entry-program-only.
	if !i.isModule && !hasMutableTopLevelGlobal(prog) {
		i.entryGlobalsImmutable = true
	} else if !i.isModule {
		i.computeEntryGlobalSafe()
	}
	// Stamp every declared struct type once, single-threaded, before any
	// statement (and therefore any spawn) runs, so the per-execution
	// re-resolve in execDefine is a guarded no-op and a shared type node
	// reached from concurrent goroutines never write-races.
	i.resolveDeclaredTypesOnce(prog)
	// Finish the enum-pattern matches the resolver had to defer: their subject's
	// enum is declared in a module, which only exists now that the imports are
	// loaded and the declared types are stamped. This is where a cross-module
	// `match` gets the same variant-name, duplicate-coverage and exhaustiveness
	// checking a same-file one gets at parse time.
	if err := i.validatePendingEnumMatches(prog); err != nil {
		return err
	}
	if i.prof != nil {
		i.prof.Start(time.Now())
	}
	res, err := i.execStmts(prog.TopLevel, i.global)
	// Top-level `defer`s run at program end, on every exit path (including an
	// `exit`, which finishFrame preserves over a failing defer).
	if len(i.global.deferred) > 0 {
		res, err = i.finishFrame(i.global, res, err)
	}
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
//   - `use core;` is rejected with a migration hint (the library was
//     removed; `len` is now a built-in, version constants moved
//     to `meta`).
func (i *Interpreter) processImports(prog *parser.Program, repl bool) error {
	// alreadyImported snapshots `imported` at entry. In batch mode it's empty;
	// in REPL it's whatever earlier inputs already activated. The
	// alias-with-globals rule uses this to silently no-op a repeated
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
		// alias-with-globals rule: if the library exposes any
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
		// Globals-publishing rules. Two checks before activation:
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
		// Namespace bookkeeping. Every library is namespaced, so
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
	if i.Err == nil {
		i.Err = os.Stderr
	}
	if i.In == nil {
		i.In = os.Stdin
	}
	// Spawned tasks resolve method calls, struct lookups, and namespace
	// prefixes by name from their own goroutines (the resolver skips spawn
	// bodies). New methods / structs / imports write those shared tables,
	// which would be a data race against a still-running task - Go's
	// "concurrent map read and map write" is fatal and uncatchable. Refuse
	// the mutating input while any task is live; plain statements are fine
	// (spawn snapshots are isolated from the live global frame).
	if len(prog.Methods) > 0 || len(prog.Structs) > 0 || len(prog.Enums) > 0 || len(prog.Imports) > 0 || len(prog.ModuleImports) > 0 {
		if i.hasLiveTasks() {
			return Null(), fmt.Errorf("cannot define methods or structs or add imports while spawned tasks are running; task.wait() or task.discard() them first")
		}
	}
	if err := i.processImports(prog, true); err != nil {
		return Null(), err
	}
	// The REPL runs script-mode (not a module), so `export` is illegal here too
	// - reject it as batch mode does rather than silently accepting it.
	if !i.isModule {
		if err := rejectExportInScript(prog); err != nil {
			return Null(), err
		}
	}
	if _, exists := i.structs[canonicalErrorStructName]; !exists {
		i.structs[canonicalErrorStructName] = canonicalErrorStructDef()
	}
	for _, s := range prog.Structs {
		// REPL: silently re-define so a snippet can redeclare a struct.
		i.structs[s.Name] = s
	}
	for _, e := range prog.Enums {
		// REPL: silently re-define so a snippet can redeclare an enum.
		i.enums[e.Name] = e
	}
	if err := i.checkStructCycles(); err != nil {
		return Null(), err
	}
	if err := i.checkEnumZeroCycles(); err != nil {
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
	// Module imports (`import "..."`) load and bind their namespaces before the
	// input's body runs, so a later `alias.member` resolves. The REPL flag lets
	// a re-submitted `import` no-op instead of erroring on the bound alias.
	// Modules stay disabled (a positioned error) if the REPL never called
	// EnableModules.
	if err := i.loadModuleImports(prog, true); err != nil {
		return Null(), err
	}
	// A top-level `defer` in the REPL runs at the end of the input that
	// registered it (each input is its own top-level frame). Run them on every
	// exit path; on an error path the original error wins over a defer failure.
	last := Null()
	for _, st := range prog.TopLevel {
		if es, ok := st.(*parser.ExprStmt); ok {
			v, err := i.evalExpr(es.Expr, i.global)
			if err != nil {
				i.runDeferredCalls(i.global, errdeferFires(err))
				return Null(), err
			}
			last = v
			continue
		}
		last = Null()
		res, err := i.execStmt(st, i.global)
		if err != nil {
			i.runDeferredCalls(i.global, errdeferFires(err))
			return Null(), err
		}
		if res.hasBreak || res.hasContinue {
			// Plain defers run on any exit; an unhandled break / continue is
			// control flow, not a propagating error, so errdefers do NOT fire -
			// matching finishFrame and the documented semantics (errdefer is
			// skipped on break / continue).
			i.runDeferredCalls(i.global, false)
			return Null(), unhandledLoopFlowError(res)
		}
	}
	if derr := i.runDeferredCalls(i.global, false); derr != nil {
		return Null(), derr
	}
	return last, nil
}

// blockResult carries control flow info out of a block.
//   - hasReturn: a `return` was executed; `value` holds the return value
//     (callers in non-method contexts bubble this up further).
//   - hasBreak: a `break;` was executed. Loop statements catch
//     this and exit; non-loop statements pass it through. A `break`
//     reaching the top level is a positioned runtime error.
//   - hasContinue: a `continue;` was executed. Loop statements
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
// The resolver's NumSlots hint pre-sizes the slot slice so DefineAt
// avoids a grow on every write. The fresh env is borrowed
// from envPool and returned on the way out; Jennifer has no closures
// so no code retains a reference to the frame after the block ends.
func (i *Interpreter) execBlock(b *parser.Block, parent *Environment) (blockResult, error) {
	env := borrowBlockEnv(parent, b.NumSlots)
	res, err := i.execStmts(b.Stmts, env)
	// Fast path: the vast majority of blocks register no defer, so skip the
	// (non-inlined) finishFrame call - and its by-value blockResult copy - with a
	// cheap length check. finishFrame re-checks, so this is purely an optimization.
	if len(env.deferred) > 0 {
		res, err = i.finishFrame(env, res, err)
	}
	releaseBlockEnv(env)
	return res, err
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

// execStmt executes one statement. When statement profiling is active it
// times the execution, splitting self time (this statement) from cumulative
// time (this statement plus everything it called) via the profChild
// accumulator; otherwise it delegates straight to execStmtRaw with only a nil
// check of overhead.
func (i *Interpreter) execStmt(s parser.Stmt, env *Environment) (blockResult, error) {
	if i.prof == nil || !i.profStmts {
		return i.execStmtRaw(s, env)
	}
	// The self/cumulative split accumulates nested-statement time in
	// root.profChild. It lives on the per-goroutine root env (not the shared
	// Interpreter), so parallel `spawn` bodies each accumulate into their own
	// snapshot root instead of racing one field.
	root := env.root
	if root == nil {
		root = env
	}
	file, line, col := posFor(s)
	start := time.Now()
	savedChild := root.profChild
	root.profChild = 0
	res, err := i.execStmtRaw(s, env)
	elapsed := time.Since(start)
	i.prof.RecordStmt(file, line, col, elapsed-root.profChild, elapsed)
	root.profChild = savedChild + elapsed
	return res, err
}

func (i *Interpreter) execStmtRaw(s parser.Stmt, env *Environment) (blockResult, error) {
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
	case *parser.MatchStmt:
		return i.execMatch(st, env)
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
	case *parser.DeferStmt:
		return blockResult{}, i.execDefer(st, env)
	case *parser.ExprStmt:
		if _, err := i.evalExpr(st.Expr, env); err != nil {
			return blockResult{}, err
		}
		return blockResult{}, nil
	}
	file, line, col := posFor(s)
	return blockResult{}, &runtimeError{Msg: fmt.Sprintf("unsupported statement type %T", s), File: file, Line: line, Col: col}
}

// rhsFreshLiteral reports whether an initializer / assignment RHS yields a
// value that cannot alias any existing binding, so the binding site can store
// it without a redundant eager deep copy. A list / map / struct literal builds
// a brand-new container and its evaluators already Copy() every element into it
// (evalListLit / evalMapLit / evalStructLit), so the whole value is private.
// Var / index / field reads, const refs, and calls can all hand back a
// reference into a live binding, so those are still eager-copied.
func rhsFreshLiteral(e parser.Expr) bool {
	switch e.(type) {
	case *parser.ListLit, *parser.MapLit, *parser.StructLit:
		return true
	}
	return false
}

func (i *Interpreter) execDefine(st *parser.DefineStmt, env *Environment) error {
	// if the declared type names a struct, verify the
	// struct exists before any other check so an unknown name surfaces
	// as "unknown struct type" rather than a misleading type-mismatch.
	// Bare names look up in i.structs (user-defined); namespaced names
	// resolve the alias prefix first, then look up in i.NSStructs.
	// Resolve every struct type the declared type names - the type itself and
	// any list / map / task element types - so an aliased module or library
	// struct is stamped with the identity its values carry (bare `alias.Struct`
	// and `list of alias.Struct` alike). Without recursing into element types,
	// `def xs as list of alias.Struct` would leave the element tagged with the
	// alias and mismatch the stem-tagged values it is filled with.
	if err := i.resolveDeclaredStructNS(&st.VarType, st); err != nil {
		return err
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
		// enforce the declared inner type. A fresh literal RHS is already
		// private (its evaluator copied every element), so skip the
		// redundant whole-value copy - only the stamp is needed.
		if rhsFreshLiteral(st.InitExpr) {
			val = stampDeclaredType(v, st.VarType)
		} else {
			val = stampDeclaredType(i.eagerCopy(v, st), st.VarType)
		}
	} else {
		// Spec decision: uninitialized variables get the zero value of
		// their declared type. Constants must always be initialized (the
		// parser enforces this; the assertion below is defensive).
		if st.IsConst {
			file, line, col := posFor(st)
			return &runtimeError{Msg: "internal: constant without init reached interpreter", File: file, Line: line, Col: col}
		}
		// structs need access to the interpreter's struct table to
		// populate every field's zero value. Route through a dedicated
		// helper that materialises the full field list (and validates
		// that the named struct actually exists).
		if st.VarType.Kind == parser.TypeStruct && i.isEnumType(st.VarType.StructNS, st.VarType.StructName, st.VarType.ModPath) {
			// an enum zeroes to its first declared variant (payload zeroed).
			zero, err := i.zeroEnumFor(st.VarType.StructNS, st.VarType.StructName, st.VarType.ModPath, st)
			if err != nil {
				return err
			}
			val = zero
		} else if st.VarType.Kind == parser.TypeStruct && i.isObjectType(st.VarType.StructNS, st.VarType.StructName) {
			// an opaque object type (json.Value) zeroes to an empty payload:
			// a wrapped null node, not a struct.
			val = ObjectVal(st.VarType.StructNS, st.VarType.StructName, Null())
		} else if st.VarType.Kind == parser.TypeStruct {
			zero, err := i.zeroStructFor(st.VarType.StructNS, st.VarType.StructName, st.VarType.ModPath, st)
			if err != nil {
				return err
			}
			val = zero
		} else {
			val = stampDeclaredType(ZeroFor(st.VarType), st.VarType)
		}
	}
	// prefer the slot-based DefineAt when the resolver
	// already assigned this def a slot. Falls back to name-based
	// Define for REPL / ad-hoc AST paths.
	if st.Slot >= 0 {
		if err := env.DefineAt(st.Slot, st.VarName, val, st.VarType, st.IsConst); err != nil {
			file, line, col := posFor(st)
			return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
	} else {
		if err := env.Define(st.VarName, val, st.VarType, st.IsConst); err != nil {
			file, line, col := posFor(st)
			return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
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
		// Stamp the declared element type onto the task's shared state if it
		// doesn't already have one. The shared pointer means every Value referring
		// to the same task sees the same element type from now on. CompareAndSwap
		// makes the first-stamp set-once and race-free: a generic task bound by two
		// spawns concurrently deterministically keeps the first binder's type.
		if v.Kind != KindTask || v.Task == nil {
			return v
		}
		if declType.Element != nil {
			et := *declType.Element
			v.Task.ElemTyp.CompareAndSwap(nil, &et)
		}
	case parser.TypeChannel:
		// Mirror the task arm: stamp the channel's element type onto its shared
		// state (set-once via CompareAndSwap) so every Value referring to the same
		// channel agrees on T without racing on the stamp.
		if v.Kind != KindChannel || v.Chan == nil {
			return v
		}
		if declType.Element != nil {
			et := *declType.Element
			v.Chan.ElemTyp.CompareAndSwap(nil, &et)
		}
	}
	return v
}

// bindParamValue is the arg-binding fast path used by
// evalCall. For scalar Kinds (int / float / bool / null / string)
// both Value.Copy and stampDeclaredType are no-ops, so we skip both
// function calls and return v directly. Compound Kinds (list / map /
// bytes / struct / task) still go through the copy + stamp path so
// value-semantics + declared-type propagation stay correct. Strings
// count as scalar here because Go strings are immutable at the host
// level; Jennifer never mutates a string in place. A func value is
// likewise immutable (Copy shares the *MethodDef pointer and
// stampDeclaredType is a no-op for the signature-less `func` type), so
// it takes the same no-copy path.
func bindParamValue(v Value, declType parser.Type) Value {
	switch v.Kind {
	case KindInt, KindFloat, KindBool, KindNull, KindString, KindFunc:
		return v
	}
	return stampDeclaredType(v.Copy(), declType)
}

// methodBorrowCtx reports whether read-only-parameter borrow is sound for a call
// to method m in this interpreter - i.e. whether no mutable global can be aliased
// by an argument and mutated during the call. That holds in three cases: a module
// (declarations-only top level, no mutable globals); an entry script that
// declares no mutable top-level global (entryGlobalsImmutable); or a specific
// method proven by the per-function escape analysis to mutate no global
// transitively (m.GlobalSafe), even in a script that has mutable globals
// elsewhere. Combined with Param.Borrow (never-written, borrow-safe type) at the
// bind site.
func (i *Interpreter) methodBorrowCtx(m *parser.MethodDef) bool {
	return i.isModule || i.entryGlobalsImmutable || m.GlobalSafe
}

// hasMutableTopLevelGlobal reports whether the program declares a mutable
// top-level variable - the only construct that creates a global a method could
// alias through an argument and mutate. `def const` is immutable; a `def` nested
// in a top-level block lives in that block's frame, not the global scope reached
// by methods, so only the direct top-level statement list is scanned.
//
// INVARIANT (borrow soundness): this rests on three properties of the current
// evaluation model - (a) only top-level execution creates global-frame bindings,
// (b) `def const` is deep-immutable, (c) a method cannot create or mutate a
// global except through a top-level mutable `def` it aliases. If a future feature
// lets a method reach a global another way (a `global` keyword, a hoisting /
// scoping change, top-level shared state), the "globals-immutable script borrows
// everywhere" widening (entryGlobalsImmutable) becomes unsound and this function
// is the first thing to revisit.
func hasMutableTopLevelGlobal(prog *parser.Program) bool {
	for _, s := range prog.TopLevel {
		if d, ok := s.(*parser.DefineStmt); ok && !d.IsConst {
			return true
		}
	}
	return false
}

// bindArg binds one argument into a call frame, aliasing the argument's backing
// when the parameter is borrowable and borrowCtx (methodBorrowCtx for the callee)
// holds, and copying it otherwise. On the borrow path stampDeclaredType only sets
// header type tags (the type is borrow-safe, so it never recurses into the shared
// backing); the callee never writes the parameter, so the alias is
// observationally identical to a copy.
func (i *Interpreter) bindArg(v Value, p parser.Param, borrowCtx bool) Value {
	if p.Borrow && borrowCtx {
		return stampDeclaredType(v, p.Type)
	}
	return bindParamValue(v, p.Type)
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
	//
	// prefer the slot path when the resolver populated it.
	var b Binding
	if st.Slot >= 0 {
		b, err = env.GetBindingAt(st.Depth, st.Slot, st.VarName)
	} else {
		b, err = env.GetBinding(st.VarName)
	}
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	// Validate against the declared type before stamping: stampDeclaredType
	// writes the declared inner-type pointers onto the value, which would
	// relabel a generic (literal / json.decode) collection as matching even
	// when its entries don't. Check the raw value first, mirroring execDefine.
	if !val.MatchesDeclared(b.DeclType) {
		file, line, col := posFor(st)
		return &runtimeError{
			Msg:  fmt.Sprintf("cannot assign %s to %s variable %q", val.Kind, b.DeclType, st.VarName),
			File: file, Line: line, Col: col,
		}
	}
	// A fresh literal RHS is already private (its evaluator copied every
	// element), so skip the redundant whole-value copy - only the stamp
	// is needed. Any other RHS may alias a live binding and is eager-copied.
	if rhsFreshLiteral(st.Value) {
		val = stampDeclaredType(val, b.DeclType)
	} else {
		val = stampDeclaredType(i.eagerCopy(val, st), b.DeclType)
	}
	if st.Slot >= 0 {
		if err := env.AssignAt(st.Depth, st.Slot, st.VarName, val); err != nil {
			file, line, col := posFor(st)
			return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
	} else {
		if err := env.Assign(st.VarName, val); err != nil {
			file, line, col := posFor(st)
			return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
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
	binding, err := env.getBindingRoot(rootVar)
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
	// `$p.field[0] = ...` work alongside the original `$xs[i][j]` form.
	// The walker handles both IndexExpr and FieldAccessExpr
	// nodes; index-only chains still resolve through the same
	// indexInto / writeIndexedSlot helpers as before.
	steps, err := i.collectLvalueSteps(st.Target, env, st)
	if err != nil {
		return err
	}
	// Re-fetch the root binding: the RHS or an index expression may have
	// reassigned the same variable as a side effect (a method call that
	// writes the global), and committing the pre-evaluation copy would
	// silently discard that write.
	binding, err = env.getBindingRoot(rootVar)
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("undefined variable %q", rootVar.Name), File: file, Line: line, Col: col}
	}
	// Mutate the binding's own backing in place - no other live binding
	// aliases it (eager copies at every store site guarantee that), so this
	// stays amortised O(N) for append-in-a-loop.
	rootCopy := binding.Value
	if err := i.applyLvalueWrite(&rootCopy, steps, newVal, st); err != nil {
		return err
	}
	if err := env.assignRoot(rootVar, rootCopy); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// execAppend handles `$xs[] = expr;` and `$b[] = byte;`.
// Copies the target binding, appends, commits it back. Const-target
// rejection, type check, and per-kind validation all live here.
func (i *Interpreter) execAppend(st *parser.AppendStmt, env *Environment) error {
	binding, err := env.getBindingRoot(st.Target)
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
	// Re-fetch the target binding: the RHS may have reassigned the same
	// variable as a side effect, and appending onto the pre-evaluation
	// copy would silently discard that write. The Kind can't have changed
	// (assignment enforces the declared type), so the checks above hold.
	binding, err = env.getBindingRoot(st.Target)
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("undefined variable %q", st.Target.Name), File: file, Line: line, Col: col}
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
		// Mutate the binding's own backing in place - no other live binding
		// aliases it (eager copies at every store site guarantee that), so this
		// stays amortised O(N) for append-in-a-loop.
		rootCopy := binding.Value
		rootCopy.Bytes = append(rootCopy.Bytes, byte(newVal.Int))
		if err := env.assignRoot(st.Target, rootCopy); err != nil {
			file, line, col := posFor(st)
			return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		return nil
	}
	if binding.Value.ElemTyp != nil && !newVal.MatchesDeclared(*binding.Value.ElemTyp) {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("cannot append %s to list of declared element type %s", newVal.Kind, binding.Value.ElemTyp), File: file, Line: line, Col: col}
	}
	// Mutate the binding's own backing in place - no other live binding
	// aliases it (eager copies at every store site guarantee that), so this
	// stays amortised O(N) for append-in-a-loop. Stamp the appended copy
	// with the declared element type so a nested container keeps its
	// ElemTyp/ValTyp and later writes into it stay type-checked.
	rootCopy := binding.Value
	stored := newVal.Copy()
	if rootCopy.ElemTyp != nil {
		stored = stampDeclaredType(stored, *rootCopy.ElemTyp)
	}
	rootCopy.List = append(rootCopy.List, stored)
	if err := env.assignRoot(st.Target, rootCopy); err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	return nil
}

// execFieldAssign handles `$p.field = expr;` and chained lvalues that
// end at a field. The lvalue chain is rooted on a VarExpr,
// just like an IndexAssign, so the same "copy the binding, walk down,
// write at the leaf, reassign" pattern works. We also enforce the
// struct definition's declared type at the leaf write.
func (i *Interpreter) execFieldAssign(st *parser.FieldAssignStmt, env *Environment) error {
	rootVar := findFieldRoot(st.Target)
	if rootVar == nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: "internal: field-assign target has no root variable", File: file, Line: line, Col: col}
	}
	binding, err := env.getBindingRoot(rootVar)
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
	// Re-fetch the root binding: the RHS or an index expression may have
	// reassigned the same variable as a side effect, and committing the
	// pre-evaluation copy would silently discard that write.
	binding, err = env.getBindingRoot(rootVar)
	if err != nil {
		file, line, col := posFor(st)
		return &runtimeError{Msg: fmt.Sprintf("undefined variable %q", rootVar.Name), File: file, Line: line, Col: col}
	}
	// Mutate the binding's own backing in place - no other live binding
	// aliases it (eager copies at every store site guarantee that), so this
	// stays amortised O(N) for append-in-a-loop.
	rootCopy := binding.Value
	if err := i.applyLvalueWrite(&rootCopy, steps, newVal, st); err != nil {
		return err
	}
	if err := env.assignRoot(rootVar, rootCopy); err != nil {
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
		def, ok := i.lookupStructDef(parent.StructNS, parent.StructName, parent.ModPath)
		if !ok {
			return &runtimeError{Msg: fmt.Sprintf("internal: struct %q definition missing at assignment", parent.StructName), File: step.file, Line: step.line, Col: step.col}
		}
		for _, decl := range def.Fields {
			if decl.Name != step.field {
				continue
			}
			declType := decl.Type
			// A module struct's bare field types name sibling module structs by
			// the module's internal identity; retag to the parent's (ns, path) so
			// the assigned value (which carries that identity across the boundary)
			// matches - the same boundary retag the module-struct literal path does.
			if parent.StructNS != "" && parent.ModPath != "" {
				ns, path := parent.StructNS, parent.ModPath
				declType = *retagType(&decl.Type, "", ns, "", path, func(name string) bool {
					_, ok := i.lookupStructDef(ns, name, path)
					return ok
				})
			}
			if !newVal.MatchesDeclared(declType) {
				return &runtimeError{Msg: fmt.Sprintf("field %q of struct %q expects %s, got %s", decl.Name, parent.StructName, declType, newVal.Kind), File: step.file, Line: step.line, Col: step.col}
			}
			for k := range parent.Fields {
				if parent.Fields[k].Name == decl.Name {
					parent.Fields[k].Value = stampDeclaredType(newVal.Copy(), declType)
					return nil
				}
			}
			// Defensive: the field is declared but the runtime value is missing.
			parent.Fields = append(parent.Fields, StructField{Name: decl.Name, Value: stampDeclaredType(newVal.Copy(), declType)})
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
// surprises later). `ns` is empty for user-defined
// structs and set for library-provided namespaced ones.
func (i *Interpreter) zeroStructFor(ns, name, modPath string, st parser.Node) (Value, error) {
	// A module struct: build it inside the module's own interpreter so nested
	// module-struct fields resolve there, then retag the module's own structs
	// from their internal bare identity to the stem the importer sees - the
	// same boundary crossing the `alias.Struct{...}` literal and the call
	// path make. A non-empty modPath resolves by canonical path (unique even
	// when two loaded modules share a stem); the stem fallback covers values
	// that carry no ModPath. Library namespaced structs (in NSStructs) fall
	// through to the normal path below.
	if ns != "" {
		if _, isLib := i.NSStructs[nsKey{NS: ns, Name: name}]; !isLib {
			mod := i.moduleByPath(modPath)
			if mod == nil {
				mod = i.moduleByNS(ns)
			}
			if mod != nil && mod.isOwnStruct(name) {
				v, err := mod.interp.zeroStructFor("", name, "", st)
				if err != nil {
					return Value{}, err
				}
				return retagStructs(v, "", mod.ns, "", mod.path, mod.isOwnStruct), nil
			}
		}
	}
	def, ok := i.lookupStructDef(ns, name, modPath)
	if !ok {
		file, line, col := posFor(st)
		if ns != "" {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown struct %s.%s", ns, name), File: file, Line: line, Col: col}
		}
		return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown struct %q", name), File: file, Line: line, Col: col}
	}
	fields, err := i.zeroPayload(def.Fields, st)
	if err != nil {
		return Value{}, err
	}
	if ns != "" {
		return NamespacedStructVal(ns, name, fields), nil
	}
	return StructVal(name, fields), nil
}

// lookupStructDef finds a struct definition by (namespace, name, module
// path). Bare names hit the user-defined table; namespaced names hit the
// library-registered table; a non-empty modPath identifies a module struct
// by its canonical path (the identity that stays unique when two loaded
// modules share a file stem).
func (i *Interpreter) lookupStructDef(ns, name, modPath string) (*parser.StructDef, bool) {
	if modPath != "" {
		// A module struct: resolve through the canonical path, never the
		// stem - a stem lookup is ambiguous under a same-stem collision.
		if mod := i.moduleByPath(modPath); mod != nil {
			if def, ok := mod.interp.structs[name]; ok {
				return def, true
			}
		}
		return nil, false
	}
	if ns != "" {
		if def, ok := i.NSStructs[nsKey{NS: ns, Name: name}]; ok {
			return def, true
		}
		// A module struct reached without a ModPath: after
		// resolveDeclaredStructNS its namespace is the module stem, and the
		// definition lives in the module's own interpreter (not i.NSStructs).
		// Consult it so a cross-module field write (`$x.field = ...`) and
		// zero-value construction resolve, matching the `alias.Struct{...}`
		// literal path. Ambiguous stems return nil (the caller needs a
		// ModPath to disambiguate).
		if mod := i.moduleByNS(ns); mod != nil {
			if def, ok := mod.interp.structs[name]; ok {
				return def, true
			}
		}
		return nil, false
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
// the chain may also include FieldAccessExpr nodes (mixed
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
// positioned error messages. parser.Node satisfies it; the
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
		// Compare in int64 before narrowing to int: on a 32-bit build (tinygo)
		// int(1<<32) is 0, so a huge out-of-range index would pass the check and
		// read/write element 0 - a silent wrong-slot instead of an error.
		if idx.Int < 0 || idx.Int >= int64(len(parent.List)) {
			file, line, col := posOf(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("list index %d out of bounds (len %d)", idx.Int, len(parent.List)), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		return &parent.List[n], nil
	case KindMap:
		// Fast path: a complete index plus a hashable key answers hit and
		// miss in O(1). The index being usable means every key is hashable,
		// so a hashable key absent from it is a genuine miss.
		if enc, ok := mapKeyEncode(idx); ok && parent.mapIndexUsable() {
			if pos, hit := parent.mapIdx[enc]; hit {
				return &parent.Map[pos].Value, nil
			}
			file, line, col := posOf(st)
			return nil, &runtimeError{Msg: fmt.Sprintf("map has no entry for key %s", idx.Display()), File: file, Line: line, Col: col}
		}
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
		// int64 compare before narrowing (32-bit safety; see indexInto).
		if idx.Int < 0 || idx.Int >= int64(len(parent.List)) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("list index %d out of bounds (len %d)", idx.Int, len(parent.List)), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
		if parent.ElemTyp != nil && !newVal.MatchesDeclared(*parent.ElemTyp) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("cannot assign %s to list element of declared type %s", newVal.Kind, parent.ElemTyp), File: file, Line: line, Col: col}
		}
		// Stamp the stored copy with the declared element type: an
		// unstamped nested container (a fresh literal RHS) carries no
		// ElemTyp/ValTyp of its own, and later writes into it would skip
		// the declared-type check entirely.
		stored := newVal.Copy()
		if parent.ElemTyp != nil {
			stored = stampDeclaredType(stored, *parent.ElemTyp)
		}
		parent.List[n] = stored
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
		// Stamp the stored copy with the declared value type (same
		// reasoning as the list arm above).
		stored := newVal.Copy()
		if parent.ValTyp != nil {
			stored = stampDeclaredType(stored, *parent.ValTyp)
		}
		// Fast path: a hashable key goes through the index. Build it lazily
		// (once, O(n)) if this map has not been indexed yet, then every
		// update / insert is O(1) - turning `$m[$k] = $v` in a loop from
		// O(n^2) into O(n).
		if enc, hashable := mapKeyEncode(idx); hashable {
			if !parent.mapIndexUsable() {
				parent.buildMapIndex()
			}
			if parent.mapIndexUsable() {
				if pos, hit := parent.mapIdx[enc]; hit {
					parent.Map[pos].Value = stored
				} else {
					parent.Map = append(parent.Map, MapEntry{Key: idx.Copy(), Value: stored})
					parent.mapIdx[enc] = len(parent.Map) - 1
				}
				return nil
			}
			// buildMapIndex declined (duplicate-key literal): fall through.
		}
		// Linear-scan fallback: a non-hashable key, or a map the index can't
		// represent. Adding a non-hashable key disables the index for good.
		for k := range parent.Map {
			if parent.Map[k].Key.Equal(idx) {
				parent.Map[k].Value = stored
				return nil
			}
		}
		// New key: append, preserving insertion order.
		parent.Map = append(parent.Map, MapEntry{Key: idx.Copy(), Value: stored})
		parent.mapIdx = nil
		return nil
	case KindBytes:
		// byte slot writes accept an int in [0, 255]. Out-of-range
		// writes are positioned runtime errors (same shape as list
		// out-of-bounds), and a non-int RHS is rejected as a type error.
		if idx.Kind != KindInt {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes index must be int, got %s", idx.Kind), File: file, Line: line, Col: col}
		}
		// int64 compare before narrowing (32-bit safety; see indexInto).
		if idx.Int < 0 || idx.Int >= int64(len(parent.Bytes)) {
			file, line, col := posOf(st)
			return &runtimeError{Msg: fmt.Sprintf("bytes index %d out of bounds (len %d)", idx.Int, len(parent.Bytes)), File: file, Line: line, Col: col}
		}
		n := int(idx.Int)
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
	// A range source iterates lazily: `for (def i in 0..n)` never materialises
	// the list, so a huge range costs no allocation.
	if rng, ok := st.Coll.(*parser.RangeExpr); ok {
		return i.execForEachRange(st, rng, env)
	}
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
		if err := i.loopCheckpoint(env, st); err != nil {
			return blockResult{}, err
		}
		// Each iteration opens its own scope so the binding is fresh. The
		// iterator and the body's defs share this one frame (matching the
		// resolver, which numbers the iterator slot 0 and body defs after
		// it). Pre-size to Body.NumSlots and bind the iterator at its
		// resolved slot: binding name-only would leave slot 0 empty, and the
		// first body `def` grows the slot slice over it, shadowing the
		// iterator with a zero binding.
		// Borrow the per-iteration frame from the pool instead of allocating
		// a fresh map + slot slice every pass. Safe to release once the body
		// returns: Jennifer has no closure that could retain the frame, and a
		// spawn in the body captures a deep-copied snapshot, not this env.
		iterEnv := borrowBlockEnv(env, st.Body.NumSlots)
		if err := iterEnv.DefineAt(st.IterSlot, st.VarName, iter.Copy(), iterType, false); err != nil {
			releaseBlockEnv(iterEnv)
			file, line, col := posFor(st)
			return blockResult{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		res, err := i.execStmts(st.Body.Stmts, iterEnv)
		if len(iterEnv.deferred) > 0 {
			res, err = i.finishFrame(iterEnv, res, err)
		}
		releaseBlockEnv(iterEnv)
		return res, err
	}

	// Iterate a snapshot of the collection's header so the loop is
	// independent of in-loop mutation to the same binding: an in-place
	// element write or an append (which may or may not reallocate the
	// backing) must not change what the current loop yields. Without the
	// snapshot, iteration behaviour would depend on Go slice capacity.
	switch coll.Kind {
	case KindList:
		snapshot := make([]Value, len(coll.List))
		copy(snapshot, coll.List)
		for _, elem := range snapshot {
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
		snapshot := make([]MapEntry, len(coll.Map))
		copy(snapshot, coll.Map)
		for _, entry := range snapshot {
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

// execMatch evaluates the subject once, then checks each `when` arm top-to-bottom,
// comparing the subject to the arm's values by strict `==` (Value.Equal, the same
// path the `==` operator uses). The first matching arm's block runs in its own
// scope; a `when`'s values are evaluated left-to-right only until one matches.
// With no matching arm and no `else`, the statement is a no-op. There is no
// fall-through, and `break` / `continue` in an arm act on the enclosing loop.
func (i *Interpreter) execMatch(st *parser.MatchStmt, env *Environment) (blockResult, error) {
	subject, err := i.evalExpr(st.Subject, env)
	if err != nil {
		return blockResult{}, err
	}
	isEnum := subject.Kind == KindEnum
	for ai := range st.Arms {
		arm := &st.Arms[ai]
		// Enum-pattern arm (`when Circle(c)` / `when Empty`): the resolver set
		// Variant when the subject is a known enum type. Match on the active
		// variant tag and bind the payload; value comparison never applies.
		if arm.Variant != "" {
			if isEnum && subject.Variant == arm.Variant {
				return i.runPatternArm(arm, arm.Bind, subject, env)
			}
			continue
		}
		// A value arm, OR - in a `spawn` / REPL body the resolver deliberately
		// skips (implementation-note 11) - an enum-pattern arm the resolver never
		// rewrote. If the subject is an enum, read the arm head as a variant
		// pattern at runtime before falling back to `==`, so pattern matching
		// works inside `spawn` too.
		if isEnum && len(arm.Values) == 1 {
			if variant, bind, ok := i.enumArmPattern(arm.Values[0], subject); ok {
				if subject.Variant == variant {
					return i.runPatternArm(arm, bind, subject, env)
				}
				continue
			}
		}
		for _, ve := range arm.Values {
			v, err := i.evalExpr(ve, env)
			if err != nil {
				return blockResult{}, err
			}
			if subject.Equal(v) {
				return i.execBlock(arm.Body, env)
			}
		}
	}
	if st.Else != nil {
		return i.execBlock(st.Else, env)
	}
	return blockResult{}, nil
}

// matchPayloadNS tags the struct value a `match` arm binds for a variant payload
// (`when Circle(c)` -> `$c`). It is deliberately not a valid namespace / module
// stem, so the snapshot carries an identity no declared type can match: without
// this, a payload whose variant name equals its own enum (or an in-scope struct)
// would satisfy an `as ThatType` binding and let a struct value leak into an
// enum-typed slot - a soundness hole that silently breaks exhaustiveness.
const matchPayloadNS = "\x00payload"

// enumArmPattern reads an unresolved `when` arm head as a variant pattern of the
// subject's enum: `Variant` (a ConstRef) or `Variant(bind)` (a Call binding one
// bare name). It returns ok only when the name is an actual variant of the
// subject's enum, so a genuine value arm (`when $other`, `when SOME_CONST`) still
// falls through to value comparison. Only the resolver-skipped paths (spawn /
// REPL) reach here; the batch path arrives pre-rewritten with arm.Variant set.
func (i *Interpreter) enumArmPattern(e parser.Expr, subject Value) (variant, bind string, ok bool) {
	var name string
	switch ex := e.(type) {
	case *parser.ConstRefExpr:
		name = ex.Name
	case *parser.CallExpr:
		if len(ex.Args) != 1 {
			return "", "", false
		}
		a, isRef := ex.Args[0].(*parser.ConstRefExpr)
		if !isRef {
			return "", "", false
		}
		name, bind = ex.Callee, a.Name
	default:
		return "", "", false
	}
	def, found := i.lookupEnumDef(subject.StructNS, subject.StructName, subject.ModPath)
	if !found {
		return "", "", false
	}
	for vi := range def.Variants {
		if def.Variants[vi].Name == name {
			return name, bind, true
		}
	}
	return "", "", false
}

// runPatternArm runs an enum-variant pattern arm's body, binding the variant
// payload (when the arm has a binder) as a read-only snapshot in a fresh frame.
// BindSlot >= 0 uses the resolver-assigned slot; < 0 (spawn / REPL, unresolved)
// binds by name. The payload carries matchPayloadNS so it can never be re-bound
// to a declared type, and is `const` so a write gets a clean "cannot mutate"
// error rather than a struct-def-lookup failure - build a fresh value to
// transform it. Mirrors how execForEach seeds its iteration variable.
func (i *Interpreter) runPatternArm(arm *parser.MatchArm, bind string, subject Value, env *Environment) (blockResult, error) {
	if bind == "" {
		return i.execBlock(arm.Body, env)
	}
	fields := make([]StructField, len(subject.Fields))
	for k, f := range subject.Fields {
		fields[k] = StructField{Name: f.Name, Value: f.Value.Copy()}
	}
	payload := Value{Kind: KindStruct, StructNS: matchPayloadNS, StructName: subject.Variant, Fields: fields}
	frame := borrowBlockEnv(env, arm.Body.NumSlots)
	var derr error
	if arm.BindSlot >= 0 {
		derr = frame.DefineAt(arm.BindSlot, bind, payload, parser.Type{}, true)
	} else {
		derr = frame.Define(bind, payload, parser.Type{}, true)
	}
	if derr != nil {
		releaseBlockEnv(frame)
		file, line, col := posFor(arm)
		return blockResult{}, &runtimeError{Msg: derr.Error(), File: file, Line: line, Col: col}
	}
	res, err := i.execStmts(arm.Body.Stmts, frame)
	if len(frame.deferred) > 0 {
		res, err = i.finishFrame(frame, res, err)
	}
	releaseBlockEnv(frame)
	return res, err
}

func (i *Interpreter) execWhile(st *parser.WhileStmt, env *Environment) (blockResult, error) {
	for {
		if err := i.loopCheckpoint(env, st); err != nil {
			return blockResult{}, err
		}
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
	// is visible in cond/step/body, but NOT after the loop. Borrow the header
	// frame from the pool (freed on every exit path via defer) so a `for`
	// nested in a hot loop doesn't allocate a fresh frame each pass; the body
	// block borrows its own frame under this one. Pre-size to HeaderSlots so
	// the Init `def` binds into an existing slot instead of growing the slice.
	forEnv := borrowBlockEnv(env, st.HeaderSlots)
	defer releaseBlockEnv(forEnv)
	if st.Init != nil {
		if _, err := i.execStmt(st.Init, forEnv); err != nil {
			return blockResult{}, err
		}
	}
	for {
		if err := i.loopCheckpoint(env, st); err != nil {
			return blockResult{}, err
		}
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
		if err := i.loopCheckpoint(env, st); err != nil {
			return blockResult{}, err
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
func (i *Interpreter) execThrow(st *parser.ThrowStmt, env *Environment) error {
	v, err := i.evalExpr(st.Value, env)
	if err != nil {
		return err
	}
	file, line, col := posFor(st)
	return &ErrorSignal{Value: v.Copy(), File: file, Line: line, Col: col}
}

// deferredCall is a `defer`red call: the original call expression plus its
// arguments already evaluated (and snapshotted) at the defer site. Only the args
// are captured early - the callee dispatch happens when the call runs, by feeding
// the captured values back through the normal call path as PreEval literals.
// onError marks an `errdefer`: run only when the frame is exiting with a
// propagating error (see runDeferredCalls).
type deferredCall struct {
	call    parser.Expr // *parser.CallExpr or *parser.QualifiedCallExpr
	args    []Value
	onError bool
}

// execDefer evaluates the deferred call's arguments now and records the call on
// the current frame; the call itself runs when the frame exits (finishFrame).
// Handles both `defer` and `errdefer` (DeferStmt.OnError).
func (i *Interpreter) execDefer(st *parser.DeferStmt, env *Environment) error {
	argExprs := deferCallArgs(st.Call)
	args := make([]Value, len(argExprs))
	for idx, a := range argExprs {
		v, err := i.evalExpr(a, env)
		if err != nil {
			return err
		}
		// Snapshot the argument value at the defer site (Go's defer semantics):
		// a later mutation of the source binding must not change what the
		// deferred call receives. Copy is cheap for scalars and handle structs.
		args[idx] = v.Copy()
	}
	env.deferred = append(env.deferred, deferredCall{call: st.Call, args: args, onError: st.OnError})
	return nil
}

// deferCallArgs returns the argument expressions of the two call shapes the
// parser allows after `defer`.
func deferCallArgs(call parser.Expr) []parser.Expr {
	switch c := call.(type) {
	case *parser.CallExpr:
		return c.Args
	case *parser.QualifiedCallExpr:
		return c.Args
	}
	return nil
}

// finishFrame runs env's deferred calls (LIFO) as the frame exits and merges
// their outcome with the frame's result. It is a no-op (and allocation-free)
// when the frame registered no defer - the common case. A deferred call that
// errors supersedes a normal / return / break / continue / throw outcome (Go's
// panic-in-defer rule), but never an ExitSignal: exit is the strongest,
// uncatchable escape, so it still runs the defers but is not overridden by one
// that fails.
func (i *Interpreter) finishFrame(env *Environment, res blockResult, err error) (blockResult, error) {
	if len(env.deferred) == 0 {
		return res, err
	}
	derr := i.runDeferredCalls(env, errdeferFires(err))
	if derr == nil {
		return res, err
	}
	if _, isExit := err.(*ExitSignal); isExit {
		return res, err
	}
	return blockResult{}, derr
}

// errdeferFires reports whether an `errdefer` should run for a frame exiting with
// err: only a genuinely propagating error (a `throw` or a runtime error) arms
// errdefers, never a nil (success / return / break / continue, none of which
// reach here as an error) or an *ExitSignal (a deliberate termination, not a
// failure). Shared by finishFrame and the REPL teardown so both agree.
func errdeferFires(err error) bool {
	if err == nil {
		return false
	}
	_, isExit := err.(*ExitSignal)
	return !isExit
}

// runDeferredCalls invokes env.deferred in LIFO order, always running every one
// (a failure does not stop the rest - cleanup should complete). failed reports
// whether the frame is exiting with a propagating error: an `errdefer` entry
// runs only then. A deferred call that itself errors flips failed to true for
// the entries still to run - the frame IS now exiting with an error, so an
// earlier-registered errdefer (e.g. the release of a resource acquired before
// the failing defer) still fires. Returns the last error a deferred call
// produced, or nil, and clears the frame's list first so a deferred call cannot
// re-enter its own frame's list.
func (i *Interpreter) runDeferredCalls(env *Environment, failed bool) error {
	d := env.deferred
	env.deferred = nil
	var derr error
	for j := len(d) - 1; j >= 0; j-- {
		if d[j].onError && !failed {
			continue
		}
		if e := i.invokeDeferred(d[j], env); e != nil {
			derr = e
			failed = true
		}
	}
	return derr
}

// invokeDeferred runs one captured call by splicing its snapshotted arguments
// back into a shallow clone of the call expression as PreEval literals, then
// dispatching through the normal call path - so user methods, namespaced
// builtins, and module functions all run unchanged (the clone preserves the
// resolver's pre-stamped Method / Fn pointers).
func (i *Interpreter) invokeDeferred(dc deferredCall, env *Environment) error {
	file, line, col := posFor(dc.call)
	litArgs := make([]parser.Expr, len(dc.args))
	for k, v := range dc.args {
		litArgs[k] = parser.NewPreEval(v, file, line, col)
	}
	switch c := dc.call.(type) {
	case *parser.CallExpr:
		clone := *c
		clone.Args = litArgs
		_, err := i.evalCall(&clone, env)
		return err
	case *parser.QualifiedCallExpr:
		clone := *c
		clone.Args = litArgs
		_, err := i.evalQualifiedCall(&clone, env)
		return err
	}
	return nil
}

// execTry runs the body and, if it produces a catchable error
// (*ErrorSignal from `throw`, or *runtimeError from a builtin /
// language operation), runs the handler with the catch variable
// bound to the thrown value. *ExitSignal propagates uncaught (the
// program-level escape is uncatchable per spec). blockResult flags
// (return/break/continue) flow through unchanged so the surrounding
// method / loop sees them.
func (i *Interpreter) execTry(st *parser.TryStmt, env *Environment) (blockResult, error) {
	// Run the body in its own frame (the resolver scopes body defs there), so
	// a `def` skipped by a throw is out of scope afterward rather than leaving
	// a zeroed slot in the enclosing frame that reads as null.
	bodyEnv := borrowBlockEnv(env, st.Body.NumSlots)
	res, err := i.execStmts(st.Body.Stmts, bodyEnv)
	// Body defers run at body exit, before the catch. A defer that throws is
	// itself catchable here (it is lexically part of the try body).
	if len(bodyEnv.deferred) > 0 {
		res, err = i.finishFrame(bodyEnv, res, err)
	}
	releaseBlockEnv(bodyEnv)
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
	// Bind the catch variable in a fresh handler frame, then run the
	// handler. The frame is pre-sized to CatchBody.NumSlots and the
	// variable bound at its resolved slot (slot 0), mirroring
	// execForEach: binding name-only would leave slot 0 empty, and the
	// first catch-body `def` would grow the slot slice over it, making
	// every later slot-resolved read of the catch variable hit a zeroed
	// (null) binding. In the resolver-less REPL path CatchSlot is -1,
	// so DefineAt falls back to the name map and still runs the
	// no-shadowing check (the resolver rejects shadowing at parse time
	// on the batch path).
	declType := parser.StructType(canonicalErrorStructName)
	if caught.Kind != KindStruct || caught.StructName != canonicalErrorStructName {
		// User threw a non-Error value (any kind is permitted by the
		// spec). Bind without a declared type stamp; the catch body
		// uses convert.typeOf / runtime checks if it needs to inspect.
		declType = parser.Type{}
	}
	catchEnv := borrowBlockEnv(env, st.CatchBody.NumSlots)
	if err := catchEnv.DefineAt(st.CatchSlot, st.CatchName, caught.Copy(), declType, false); err != nil {
		releaseBlockEnv(catchEnv)
		return blockResult{}, &runtimeError{Msg: err.Error(), File: st.CatchFile, Line: st.CatchLine, Col: st.CatchCol}
	}
	res, err = i.execStmts(st.CatchBody.Stmts, catchEnv)
	if len(catchEnv.deferred) > 0 {
		res, err = i.finishFrame(catchEnv, res, err)
	}
	releaseBlockEnv(catchEnv)
	return res, err
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
	case *parser.PreEval:
		// A value captured at a `defer` site, spliced back into the call's
		// argument list (see invokeDeferred). Return it verbatim.
		return ex.Value.(Value), nil
	case *parser.IntLit:
		return IntVal(ex.Value), nil
	case *parser.FloatLit:
		return FloatVal(ex.Value), nil
	case *parser.StringLit:
		return StringVal(ex.Value), nil
	case *parser.InterpStringExpr:
		// Cooked-string interpolation: concatenate literal chunks with each slot
		// value stringified via Display (the same form convert.toString produces),
		// so no `use convert` is needed and any value kind interpolates.
		var b strings.Builder
		for _, part := range ex.Parts {
			if part.Expr == nil {
				b.WriteString(part.Lit)
				continue
			}
			v, err := i.evalExpr(part.Expr, env)
			if err != nil {
				return Value{}, err
			}
			b.WriteString(v.Display())
		}
		return StringVal(b.String()), nil
	case *parser.BoolLit:
		return BoolVal(ex.Value), nil
	case *parser.NullLit:
		return Null(), nil
	case *parser.VarExpr:
		// prefer the O(1) slot path when the resolver
		// annotated this reference. Falls back to the O(depth) name
		// walk otherwise (REPL, hand-built AST fragments).
		var v Value
		var err error
		if ex.Slot >= 0 {
			v, err = env.GetAt(ex.Depth, ex.Slot, ex.Name)
		} else {
			v, err = env.Get(ex.Name)
		}
		if err != nil {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		// Return the binding's value directly. It may share slice / map
		// headers with the stored binding, but that is safe: any site that
		// stores it into another binding (execDefine / execAssign /
		// bindParamValue) deep-copies first, and the mutation sites fetch
		// their target via GetBinding, not this path.
		return v, nil
	case *parser.ConstRefExpr:
		// 1. User scope first (variables and `def const`). Prefer the O(1) slot
		// path when the resolver annotated this reference; fall back to the
		// name-map walk (REPL / hand-built AST) otherwise.
		var b Binding
		var err error
		if ex.Slot >= 0 {
			b, err = env.GetBindingAt(ex.Depth, ex.Slot, ex.Name)
		} else {
			b, err = env.GetBinding(ex.Name)
		}
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
		// 3. A bare top-level method name used in expression position (not
		// followed by `(`) is a first-class function value. Since a method name
		// can never collide with a variable / constant (the def/func namespace is
		// shared), reaching here unambiguously means the function value.
		if m, ok := i.methods[ex.Name]; ok {
			return FuncVal(m), nil
		}
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("undefined name %q", ex.Name), File: file, Line: line, Col: col}
	case *parser.BinaryExpr:
		return i.evalBinary(ex, env)
	case *parser.UnaryExpr:
		return i.evalUnary(ex, env)
	case *parser.CallExpr:
		return i.evalCall(ex, env)
	case *parser.CallValueExpr:
		return i.evalCallValue(ex, env)
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
	case *parser.RangeExpr:
		return i.evalRange(ex, env)
	case *parser.SliceExpr:
		return i.evalSlice(ex, env)
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
	// A duplicate key makes a corrupt two-entry map (only the first reachable),
	// so reject it. Hashable keys go through a seen-set; the rare non-hashable
	// key (a list / map) falls back to an Equal scan of prior such keys.
	seen := make(map[string]bool, len(ex.Keys))
	var nonHashable []Value
	for k, keyExpr := range ex.Keys {
		key, err := i.evalExpr(keyExpr, env)
		if err != nil {
			return Value{}, err
		}
		if enc, hashable := mapKeyEncode(key); hashable {
			if seen[enc] {
				file, line, col := posFor(keyExpr)
				return Value{}, &runtimeError{Msg: fmt.Sprintf("duplicate key %s in map literal", key.Display()), File: file, Line: line, Col: col}
			}
			seen[enc] = true
		} else {
			for _, prev := range nonHashable {
				if prev.Equal(key) {
					file, line, col := posFor(keyExpr)
					return Value{}, &runtimeError{Msg: fmt.Sprintf("duplicate key %s in map literal", key.Display()), File: file, Line: line, Col: col}
				}
			}
			nonHashable = append(nonHashable, key)
		}
		val, err := i.evalExpr(ex.Values[k], env)
		if err != nil {
			return Value{}, err
		}
		entries = append(entries, MapEntry{Key: key.Copy(), Value: val.Copy()})
	}
	out := Value{Kind: KindMap, Map: entries}
	// Index the literal up front: `def m init {...}` skips the binding-site
	// deep copy (rhsFreshLiteral), so this is where a literal map gets its
	// index. buildMapIndex declines (leaves nil) for duplicate or
	// non-hashable keys, so those keep linear-scanning.
	out.buildMapIndex()
	return out, nil
}

// evalStructLit constructs a struct value from a literal. The
// literal's name must match a hoisted top-level `def struct`; every
// declared field of the struct must appear exactly once in the literal
// (no defaults at the literal level - users who want a zero-initialised
// struct write `def p as Point;` without `init`); each value's runtime
// type is checked against the declared field type. Fields are emitted
// in *declaration* order regardless of the literal's source order so
// the resulting Value is canonical.
func (i *Interpreter) evalStructLit(ex *parser.StructLit, env *Environment) (Value, error) {
	// Enum construction reuses the StructLit node: a local `Enum.Variant{...}`
	// (NS names an enum type) or the cross-module `alias.Enum.Variant{...}`
	// (Enum segment set). Everything else is an ordinary struct literal.
	//
	// The two-segment form is ambiguous when a local enum shares its name with
	// an imported module's alias or a library namespace: `m.Opt{ a: 7 }` is a
	// module struct literal, not variant `Opt` of a local `def enum m`. The
	// namespace wins - it is the older, cross-file meaning - so an enum named
	// after an alias shadows nothing.
	if ex.Enum != "" || (ex.NS != "" && i.enums[ex.NS] != nil && !i.isNamespaceOrAlias(ex.NS)) {
		return i.evalEnumLit(ex, env)
	}
	// namespaced literals (`os.Result{ ... }`) resolve via the
	// alias prefix then the NSStructs table; bare literals use the
	// user-defined struct table as before.
	var def *parser.StructDef
	var resolvedNS string
	var resolvedModPath string
	// A module struct's field types name sibling module structs by the module's
	// internal (bare) identity; the field *values* carry the module's (ns, path)
	// identity once they cross the boundary. retagFieldType rewrites a module
	// struct's own bare-struct field types to that (ns, path) identity so the
	// field check matches. Identity for every other case (bare user struct,
	// library struct, main-program struct with an already-stamped module field).
	retagFieldType := func(t parser.Type) parser.Type { return t }
	if mod, ok := i.moduleAliases[ex.NS]; ok {
		// `alias.Struct{...}` - construct an importer-visible module struct.
		d, exists := mod.interp.structs[ex.Name]
		if !exists {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("module %q has no struct %q", ex.NS, ex.Name), File: file, Line: line, Col: col}
		}
		if !mod.exports[ex.Name] {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("%s.%s: struct %q is not exported from module %q", ex.NS, ex.Name, ex.Name, ex.NS), File: file, Line: line, Col: col}
		}
		def = d
		resolvedNS = mod.ns
		resolvedModPath = mod.path
		modInterp, modNS, modPath := mod.interp, mod.ns, mod.path
		retagFieldType = func(t parser.Type) parser.Type {
			return *retagType(&t, "", modNS, "", modPath, func(name string) bool {
				// A field may name a sibling module struct OR a sibling module
				// enum (a Store enum held in a Limiter struct, say); both take the
				// module's (ns, path) identity at the boundary.
				if _, ok := modInterp.structs[name]; ok {
					return true
				}
				_, ok := modInterp.enums[name]
				return ok
			})
		}
	} else if ex.NS != "" {
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
		declType := retagFieldType(decl.Type)
		if !v.MatchesDeclared(declType) {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("field %q of struct %q expects %s, got %s", decl.Name, ex.Name, declType, v.Kind), File: lit.File, Line: lit.Line, Col: lit.Col}
		}
		out = append(out, StructField{Name: decl.Name, Value: stampDeclaredType(v.Copy(), declType)})
	}
	if resolvedNS != "" {
		sv := NamespacedStructVal(resolvedNS, ex.Name, out)
		sv.ModPath = resolvedModPath // module identity (empty for a library struct)
		return sv, nil
	}
	return StructVal(ex.Name, out), nil
}

// lookupEnumDef resolves an enum definition by (ns, name, modPath). A local
// enum has empty ns/modPath; a module enum resolves through the canonical path
// (or the stem fallback) into the module's own interpreter.
func (i *Interpreter) lookupEnumDef(ns, name, modPath string) (*parser.EnumDef, bool) {
	if modPath != "" {
		if mod := i.moduleByPath(modPath); mod != nil {
			if def, ok := mod.interp.enums[name]; ok {
				return def, true
			}
		}
		return nil, false
	}
	if ns != "" {
		if mod := i.moduleByNS(ns); mod != nil {
			if def, ok := mod.interp.enums[name]; ok {
				return def, true
			}
		}
		return nil, false
	}
	def, ok := i.enums[name]
	return def, ok
}

// isEnumType reports whether (ns, name, modPath) names an enum type.
func (i *Interpreter) isEnumType(ns, name, modPath string) bool {
	_, ok := i.lookupEnumDef(ns, name, modPath)
	return ok
}

// zeroEnumFor builds the zero value of an enum type: its first declared
// variant, with any payload fields zeroed. A module enum is built inside the
// module's own interpreter then retagged to the importer-visible identity,
// mirroring zeroStructFor.
func (i *Interpreter) zeroEnumFor(ns, name, modPath string, st parser.Node) (Value, error) {
	if ns != "" || modPath != "" {
		mod := i.moduleByPath(modPath)
		if mod == nil {
			mod = i.moduleByNS(ns)
		}
		if mod != nil {
			if _, ok := mod.interp.enums[name]; ok {
				v, err := mod.interp.zeroEnumFor("", name, "", st)
				if err != nil {
					return Value{}, err
				}
				return retagStructs(v, "", mod.ns, "", mod.path, mod.isOwnStruct), nil
			}
		}
	}
	def, ok := i.lookupEnumDef(ns, name, modPath)
	if !ok {
		file, line, col := posFor(st)
		if ns != "" {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown enum %s.%s", ns, name), File: file, Line: line, Col: col}
		}
		return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown enum %q", name), File: file, Line: line, Col: col}
	}
	v0 := def.Variants[0]
	fields, err := i.zeroPayload(v0.Fields, st)
	if err != nil {
		return Value{}, err
	}
	return EnumVal(ns, modPath, name, v0.Name, fields), nil
}

// validatePendingEnumMatches completes the checks the resolver had to defer for
// a `match` whose subject type is declared in an imported module. The resolver
// rewrote the arms into variant patterns (so binders got their slots) but could
// not see the enum; now the module is loaded, so the variant names, duplicate
// coverage and exhaustiveness are checked exactly as a same-file match's are.
// Runs before the first statement executes, so a bad cross-module match fails
// the program at load rather than falling silently through at runtime.
func (i *Interpreter) validatePendingEnumMatches(prog *parser.Program) error {
	for _, st := range prog.PendingEnumMatches {
		if st == nil || st.EnumType == nil {
			continue
		}
		t := st.EnumType
		// Stamp the alias form (`sh.Shape`) to the module's (stem, path)
		// identity. Idempotent, and best-effort: an unresolvable type falls
		// through to the not-an-enum error below with its own position.
		_ = i.resolveDeclaredStructNS(t, st)
		ed, ok := i.lookupEnumDef(t.StructNS, t.StructName, t.ModPath)
		if !ok {
			file, line, col := posFor(st)
			return &runtimeError{
				Msg:  fmt.Sprintf("`match` uses variant patterns, but its subject type %s is not an enum", t.String()),
				File: file, Line: line, Col: col,
			}
		}
		if err := checkEnumArms(st, ed, t.String()); err != nil {
			return err
		}
	}
	return nil
}

// checkEnumArms validates an already-rewritten pattern match against its enum:
// every arm names a real variant, no variant is covered twice, a binder only
// appears on a variant that has a payload, and the arm set is exhaustive unless
// an `else` is present. Mirrors the resolver's same-file checks, including their
// wording, so a cross-module match reports identically to a local one.
func checkEnumArms(st *parser.MatchStmt, ed *parser.EnumDef, typeName string) error {
	variantByName := make(map[string]*parser.EnumVariant, len(ed.Variants))
	for vi := range ed.Variants {
		variantByName[ed.Variants[vi].Name] = &ed.Variants[vi]
	}
	covered := make(map[string]bool, len(ed.Variants))
	for ai := range st.Arms {
		arm := &st.Arms[ai]
		file, line, col := posFor(arm)
		vdef, isVariant := variantByName[arm.Variant]
		if !isVariant {
			return &runtimeError{Msg: fmt.Sprintf("%q is not a variant of enum %s", arm.Variant, typeName), File: file, Line: line, Col: col}
		}
		if covered[arm.Variant] {
			return &runtimeError{Msg: fmt.Sprintf("variant %s is covered more than once in this `match`", arm.Variant), File: file, Line: line, Col: col}
		}
		covered[arm.Variant] = true
		if arm.Bind != "" && len(vdef.Fields) == 0 {
			return &runtimeError{Msg: fmt.Sprintf("variant %s has no payload to bind (write `when %s`)", arm.Variant, arm.Variant), File: file, Line: line, Col: col}
		}
	}
	if st.Else != nil {
		return nil
	}
	var missing []string
	for vi := range ed.Variants {
		if !covered[ed.Variants[vi].Name] {
			missing = append(missing, ed.Variants[vi].Name)
		}
	}
	if len(missing) > 0 {
		file, line, col := posFor(st)
		return &runtimeError{
			Msg:  fmt.Sprintf("`match` on enum %s is not exhaustive: missing %s (cover every variant or add an `else`)", typeName, strings.Join(missing, ", ")),
			File: file, Line: line, Col: col,
		}
	}
	return nil
}

// zeroPayload builds the zero values for a variant's (or struct's) declared
// field list, recursing into nested struct / enum field types.
func (i *Interpreter) zeroPayload(declFields []parser.StructField, st parser.Node) ([]StructField, error) {
	fields := make([]StructField, len(declFields))
	for k, decl := range declFields {
		var fv Value
		switch {
		case decl.Type.Kind == parser.TypeStruct && i.isEnumType(decl.Type.StructNS, decl.Type.StructName, decl.Type.ModPath):
			sub, err := i.zeroEnumFor(decl.Type.StructNS, decl.Type.StructName, decl.Type.ModPath, st)
			if err != nil {
				return nil, err
			}
			fv = sub
		case decl.Type.Kind == parser.TypeStruct:
			subNS := decl.Type.StructNS
			if subNS != "" {
				if canonical, err := i.resolveNamespacePrefix(subNS); err == nil {
					subNS = canonical
				}
			}
			sub, err := i.zeroStructFor(subNS, decl.Type.StructName, decl.Type.ModPath, st)
			if err != nil {
				return nil, err
			}
			fv = sub
		default:
			fv = stampDeclaredType(ZeroFor(decl.Type), decl.Type)
		}
		fields[k] = StructField{Name: decl.Name, Value: fv}
	}
	return fields, nil
}

// evalEnumLit constructs an enum value from a StructLit reused as an enum
// literal: local `Enum.Variant` / `Enum.Variant{...}` (NS = enum type) or
// cross-module `alias.Enum.Variant[...]` (Enum segment set, NS = module alias).
func (i *Interpreter) evalEnumLit(ex *parser.StructLit, env *Environment) (Value, error) {
	var def *parser.EnumDef
	var resolvedNS, resolvedModPath string
	retagFieldType := func(t parser.Type) parser.Type { return t }
	if ex.Enum != "" {
		mod, ok := i.moduleAliases[ex.NS]
		if !ok {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown module alias %q in enum construction %s.%s.%s", ex.NS, ex.NS, ex.Enum, ex.Name), File: file, Line: line, Col: col}
		}
		d, exists := mod.interp.enums[ex.Enum]
		if !exists {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("module %q has no enum %q", ex.NS, ex.Enum), File: file, Line: line, Col: col}
		}
		if !mod.exports[ex.Enum] {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("%s.%s: enum %q is not exported from module %q", ex.NS, ex.Enum, ex.Enum, ex.NS), File: file, Line: line, Col: col}
		}
		def = d
		resolvedNS = mod.ns
		resolvedModPath = mod.path
		modInterp, modNS, modPath := mod.interp, mod.ns, mod.path
		retagFieldType = func(t parser.Type) parser.Type {
			return *retagType(&t, "", modNS, "", modPath, func(name string) bool {
				if _, ok := modInterp.structs[name]; ok {
					return true
				}
				_, ok := modInterp.enums[name]
				return ok
			})
		}
	} else {
		def = i.enums[ex.NS]
	}
	var vdef *parser.EnumVariant
	for k := range def.Variants {
		if def.Variants[k].Name == ex.Name {
			vdef = &def.Variants[k]
			break
		}
	}
	if vdef == nil {
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("enum %s has no variant %q", def.Name, ex.Name), File: file, Line: line, Col: col}
	}
	if ex.Bare {
		if len(vdef.Fields) > 0 {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("variant %s.%s carries a payload; construct it as %s.%s{ ... }", def.Name, ex.Name, def.Name, ex.Name), File: file, Line: line, Col: col}
		}
		return EnumVal(resolvedNS, resolvedModPath, def.Name, ex.Name, nil), nil
	}
	// The mirror of the check above: a payload-less variant is written bare, so
	// `Shape.Empty{}` is a mistake worth naming rather than silently accepting.
	if len(vdef.Fields) == 0 {
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("variant %s.%s has no payload; write it as %s.%s, without braces", def.Name, ex.Name, def.Name, ex.Name), File: file, Line: line, Col: col}
	}
	provided := make(map[string]*parser.StructLitField, len(ex.Fields))
	for k := range ex.Fields {
		provided[ex.Fields[k].Name] = &ex.Fields[k]
	}
	declared := make(map[string]bool, len(vdef.Fields))
	for _, f := range vdef.Fields {
		declared[f.Name] = true
	}
	for _, f := range ex.Fields {
		if !declared[f.Name] {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown field %q in variant %s.%s", f.Name, def.Name, ex.Name), File: f.File, Line: f.Line, Col: f.Col}
		}
	}
	out := make([]StructField, 0, len(vdef.Fields))
	for _, decl := range vdef.Fields {
		lit, ok := provided[decl.Name]
		if !ok {
			file, line, col := posFor(ex)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("missing field %q in variant %s.%s literal", decl.Name, def.Name, ex.Name), File: file, Line: line, Col: col}
		}
		v, err := i.evalExpr(lit.Expr, env)
		if err != nil {
			return Value{}, err
		}
		declType := retagFieldType(decl.Type)
		if !v.MatchesDeclared(declType) {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("field %q of variant %s.%s expects %s, got %s", decl.Name, def.Name, ex.Name, declType, v.Kind), File: lit.File, Line: lit.Line, Col: lit.Col}
		}
		out = append(out, StructField{Name: decl.Name, Value: stampDeclaredType(v.Copy(), declType)})
	}
	return EnumVal(resolvedNS, resolvedModPath, def.Name, ex.Name, out), nil
}

// evalFieldAccess reads a struct field. Errors on a non-struct target
// (with a positioned message naming the field that was requested) and
// on an unknown field name (which can happen if the user mistypes a
// field in a getter chain - the struct value carries the field list
// so we can spot it here).
// enumVariantFromFieldAccess reads a `Enum.Variant` that the parser shaped as
// field access on a constant, which happens whenever the enum's type name is
// spelled in caps (`RGB.Red`): the constant-name rule sends `RGB` down the
// `ORIGIN.x` deep-const field-access path. handled is false when the target is
// a real binding or is not an enum, leaving ordinary field access to run.
func (i *Interpreter) enumVariantFromFieldAccess(ref *parser.ConstRefExpr, ex *parser.FieldAccessExpr, env *Environment) (Value, bool, error) {
	// A real binding always wins: `ORIGIN.x` keeps meaning field access even if
	// an enum of the same name exists.
	if _, err := env.GetBinding(ref.Name); err == nil {
		return Value{}, false, nil
	}
	ed, isEnum := i.enums[ref.Name]
	if !isEnum {
		return Value{}, false, nil
	}
	for vi := range ed.Variants {
		if ed.Variants[vi].Name != ex.Field {
			continue
		}
		if len(ed.Variants[vi].Fields) > 0 {
			file, line, col := posFor(ex)
			return Value{}, true, &runtimeError{Msg: fmt.Sprintf("variant %s.%s carries a payload; construct it as %s.%s{ ... }", ed.Name, ex.Field, ed.Name, ex.Field), File: file, Line: line, Col: col}
		}
		return EnumVal("", "", ed.Name, ex.Field, nil), true, nil
	}
	file, line, col := posFor(ex)
	return Value{}, true, &runtimeError{Msg: fmt.Sprintf("enum %s has no variant %q", ed.Name, ex.Field), File: file, Line: line, Col: col}
}

func (i *Interpreter) evalFieldAccess(ex *parser.FieldAccessExpr, env *Environment) (Value, error) {
	// `Prefix.name` with a constant-shaped Prefix and a non-constant-shaped name
	// parses as field access on a (deep-const) struct - the `ORIGIN.x` form. An
	// enum whose type name happens to be spelled in caps (`RGB.Red`) lands in
	// exactly that shape, so try the variant reading before evaluating the
	// target: without this the target lookup fails with a baffling
	// "undefined name RGB" even though `def c as RGB;` and `match ($c)` both
	// resolve the type fine. A real binding always wins, so `ORIGIN.x` keeps
	// meaning field access.
	if ref, isRef := ex.Target.(*parser.ConstRefExpr); isRef {
		if v, handled, verr := i.enumVariantFromFieldAccess(ref, ex, env); handled {
			return v, verr
		}
	}
	parent, err := i.evalExpr(ex.Target, env)
	if err != nil {
		return Value{}, err
	}
	if parent.Kind == KindObject {
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("`.%s`: a %s.%s is opaque - use the %s.* accessors (e.g. %s.get) to reach inside", ex.Field, parent.StructNS, parent.StructName, parent.StructNS, parent.StructNS), File: file, Line: line, Col: col}
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
// positioned runtime errors (no null fallback - that's the decision
// from milestones.md). Bytes read as int in [0, 255].
func (i *Interpreter) evalIndex(ex *parser.IndexExpr, env *Environment) (Value, error) {
	parent, err := i.evalExpr(ex.Target, env)
	if err != nil {
		return Value{}, err
	}
	idx, err := i.evalExpr(ex.Index, env)
	if err != nil {
		return Value{}, err
	}
	if parent.Kind == KindObject {
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("a %s.%s is opaque - cannot index with `[ ]`; use %s.get with a JSON pointer", parent.StructNS, parent.StructName, parent.StructNS), File: file, Line: line, Col: col}
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
	// int64 compare before narrowing (32-bit safety; see indexInto).
	if idx.Int < 0 || idx.Int >= int64(len(parent.Bytes)) {
		file, line, col := posFor(node)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("bytes index %d out of bounds (len %d)", idx.Int, len(parent.Bytes)), File: file, Line: line, Col: col}
	}
	n := int(idx.Int)
	return IntVal(int64(parent.Bytes[n])), nil
}

// evalRange materialises a half-open range Lo..Hi into a fresh `list of int`.
// A range used as a `for`-each source iterates lazily (execForEachRange) and
// never reaches here.
func (i *Interpreter) evalRange(ex *parser.RangeExpr, env *Environment) (Value, error) {
	lo, hi, err := i.rangeBounds(ex.Lo, ex.Hi, env, ex)
	if err != nil {
		return Value{}, err
	}
	// A range materialises every element at once. Guard the count so a bad or
	// huge bound raises a positioned, catchable error instead of Go's
	// uncatchable "makeslice: cap out of range" panic (or a multi-GB
	// allocation) - the same reason the call-depth cap exists. rangeBounds
	// guarantees lo <= hi, so a negative `count` means the int64 subtraction
	// itself overflowed (e.g. -9e18..9e18), which is likewise far over the cap.
	// The lazy for-each form (execForEachRange) has no such limit.
	count := hi - lo
	if count < 0 || count > int64(limits.MaxRangeElements) {
		file, line, col := posFor(ex)
		size := "the range"
		if count >= 0 {
			size = fmt.Sprintf("%d elements", count)
		}
		return Value{}, &runtimeError{Msg: fmt.Sprintf("range too large to materialise into a list (%s exceeds the limit of %d); iterate lazily with `for (def i in lo..hi)` instead", size, limits.MaxRangeElements), File: file, Line: line, Col: col}
	}
	out := make([]Value, 0, int(count))
	for n := lo; n < hi; n++ {
		out = append(out, IntVal(n))
	}
	return ListVal(parser.PrimitiveType(parser.TypeInt), out), nil
}

// rangeBounds evaluates a range's endpoints (both required) and checks they are
// int with lo <= hi.
func (i *Interpreter) rangeBounds(loE, hiE parser.Expr, env *Environment, node parser.Node) (int64, int64, error) {
	lv, err := i.evalExpr(loE, env)
	if err != nil {
		return 0, 0, err
	}
	hv, err := i.evalExpr(hiE, env)
	if err != nil {
		return 0, 0, err
	}
	if lv.Kind != KindInt || hv.Kind != KindInt {
		file, line, col := posFor(node)
		return 0, 0, &runtimeError{Msg: fmt.Sprintf("range bounds must be int, got %s..%s", lv.Kind, hv.Kind), File: file, Line: line, Col: col}
	}
	if lv.Int > hv.Int {
		file, line, col := posFor(node)
		return 0, 0, &runtimeError{Msg: fmt.Sprintf("range lower bound %d exceeds upper bound %d", lv.Int, hv.Int), File: file, Line: line, Col: col}
	}
	return lv.Int, hv.Int, nil
}

// execForEachRange iterates `for (def i in lo..hi)` over [lo, hi) without
// materialising a list.
func (i *Interpreter) execForEachRange(st *parser.ForEachStmt, rng *parser.RangeExpr, env *Environment) (blockResult, error) {
	lo, hi, err := i.rangeBounds(rng.Lo, rng.Hi, env, rng)
	if err != nil {
		return blockResult{}, err
	}
	iterType := parser.PrimitiveType(parser.TypeInt)
	for n := lo; n < hi; n++ {
		if err := i.loopCheckpoint(env, st); err != nil {
			return blockResult{}, err
		}
		iterEnv := borrowBlockEnv(env, st.Body.NumSlots)
		if err := iterEnv.DefineAt(st.IterSlot, st.VarName, IntVal(n), iterType, false); err != nil {
			releaseBlockEnv(iterEnv)
			file, line, col := posFor(st)
			return blockResult{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		res, rerr := i.execStmts(st.Body.Stmts, iterEnv)
		if len(iterEnv.deferred) > 0 {
			res, rerr = i.finishFrame(iterEnv, res, rerr)
		}
		releaseBlockEnv(iterEnv)
		if rerr != nil {
			return blockResult{}, rerr
		}
		if res.hasReturn {
			return res, nil
		}
		if res.hasBreak {
			return blockResult{}, nil
		}
	}
	return blockResult{}, nil
}

// evalSlice returns a fresh half-open sub-collection Target[Lo..Hi] for a list,
// bytes, or string (rune-indexed). Open ends default to 0 / len. The result is
// a value-semantic copy, never a view.
func (i *Interpreter) evalSlice(ex *parser.SliceExpr, env *Environment) (Value, error) {
	coll, err := i.evalExpr(ex.Target, env)
	if err != nil {
		return Value{}, err
	}
	var n int
	switch coll.Kind {
	case KindList:
		n = len(coll.List)
	case KindBytes:
		n = len(coll.Bytes)
	case KindString:
		n = utf8.RuneCountInString(coll.Str)
	default:
		file, line, col := posFor(ex)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("cannot slice a %s; `[a..b]` works on a list, bytes, or string", coll.Kind), File: file, Line: line, Col: col}
	}
	lo, hi, err := i.sliceBounds(ex.Lo, ex.Hi, n, env, ex)
	if err != nil {
		return Value{}, err
	}
	switch coll.Kind {
	case KindList:
		out := coll // copy the header (preserves ElemTyp); rebuild the element slice
		out.List = make([]Value, hi-lo)
		for j := lo; j < hi; j++ {
			out.List[j-lo] = coll.List[j].Copy()
		}
		return out, nil
	case KindBytes:
		b := make([]byte, hi-lo)
		copy(b, coll.Bytes[lo:hi])
		return BytesVal(b), nil
	default: // KindString
		return StringVal(string([]rune(coll.Str)[lo:hi])), nil
	}
}

// sliceBounds resolves slice endpoints against a collection of length n: open
// ends default to 0 / n, both endpoints must be int, and 0 <= lo <= hi <= n
// (strict, like an index read).
func (i *Interpreter) sliceBounds(loE, hiE parser.Expr, n int, env *Environment, node parser.Node) (int, int, error) {
	// Bounds-check in int64 BEFORE the int() cast: on a 32-bit target
	// (jennifer-tiny on embedded) a value like 1<<32 would otherwise truncate
	// to a small in-range int and silently return the wrong slice instead of
	// the out-of-bounds error (the same trap the hkdf length guard closes).
	lo, hi := int64(0), int64(n)
	if loE != nil {
		lv, err := i.evalExpr(loE, env)
		if err != nil {
			return 0, 0, err
		}
		if lv.Kind != KindInt {
			file, line, col := posFor(node)
			return 0, 0, &runtimeError{Msg: fmt.Sprintf("slice lower bound must be int, got %s", lv.Kind), File: file, Line: line, Col: col}
		}
		lo = lv.Int
	}
	if hiE != nil {
		hv, err := i.evalExpr(hiE, env)
		if err != nil {
			return 0, 0, err
		}
		if hv.Kind != KindInt {
			file, line, col := posFor(node)
			return 0, 0, &runtimeError{Msg: fmt.Sprintf("slice upper bound must be int, got %s", hv.Kind), File: file, Line: line, Col: col}
		}
		hi = hv.Int
	}
	if lo < 0 || hi > int64(n) || lo > hi {
		file, line, col := posFor(node)
		return 0, 0, &runtimeError{Msg: fmt.Sprintf("slice range [%d, %d) out of bounds for length %d", lo, hi, n), File: file, Line: line, Col: col}
	}
	return int(lo), int(hi), nil
}

func (i *Interpreter) evalBinary(b *parser.BinaryExpr, env *Environment) (Value, error) {
	// constant-fold shortcut. Set by the resolver when both
	// operands were compile-time literals (or nested folded chains).
	// The folded value is itself a literal so evalExpr returns
	// immediately.
	if b.Folded != nil {
		return i.evalExpr(b.Folded, env)
	}
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

// isBitOp returns true for the bitwise operators. Kept separate
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
	// constant-fold shortcut. Set by the resolver when the
	// operand was a compile-time literal; the folded value is itself
	// a literal so evalExpr returns immediately.
	if u.Folded != nil {
		return i.evalExpr(u.Folded, env)
	}
	v, err := i.evalExpr(u.Operand, env)
	if err != nil {
		return Value{}, err
	}
	file, line, col := posFor(u)
	switch u.Op {
	case parser.OpNeg:
		switch v.Kind {
		case KindInt:
			// MinInt64 has no positive image; negating it wraps back to
			// itself. Every other int op errors on overflow, so this must too.
			if v.Int == math.MinInt64 {
				return Value{}, &runtimeError{
					Msg:  fmt.Sprintf("integer overflow: -(%d)", v.Int),
					File: file, Line: line, Col: col,
				}
			}
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
	// `==` / `!=` work for any same-kind comparison (and across int/float); `!=`
	// is exactly the negation of `==`. Other comparisons require numeric operands.
	if op == parser.OpEq {
		return BoolVal(lv.Equal(rv)), nil
	}
	if op == parser.OpNeq {
		return BoolVal(!lv.Equal(rv)), nil
	}
	// pure-int fast path. Every numeric `for` loop
	// (`$i < N`, `$i <= max`) hits this per iteration and would
	// otherwise pay two `AsFloat` conversions per compare.
	if lv.Kind == KindInt && rv.Kind == KindInt {
		a, b := lv.Int, rv.Int
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
	}
	// pure-float fast path. Symmetrical to the int case.
	if lv.Kind == KindFloat && rv.Kind == KindFloat {
		a, b := lv.Float, rv.Float
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
	}
	// Mixed int/float: compare exactly. Converting the int to float64 would
	// lose precision above 2^53 (e.g. 9007199254740993 > 9007199254740992.0
	// must be true), so route through compareIntFloat.
	// A NaN operand is unordered: every ordering comparison (< > <= >=) is false,
	// per IEEE - matching the pure-float fast path above. (== / != already returned
	// via Equal.) Without this, the NaN would route through compareIntFloat and
	// compare as a truncated integer.
	if lv.Kind == KindInt && rv.Kind == KindFloat {
		if isNaN(rv.Float) {
			return BoolVal(false), nil
		}
		return i.orderResult(op, compareIntFloat(lv.Int, rv.Float), file, line, col)
	}
	if lv.Kind == KindFloat && rv.Kind == KindInt {
		if isNaN(lv.Float) {
			return BoolVal(false), nil
		}
		return i.orderResult(op, -compareIntFloat(rv.Int, lv.Float), file, line, col)
	}
	// String ordering: lexicographic by UTF-8 bytes (which is Unicode
	// code-point order for valid UTF-8), matching how == / != already accept
	// strings. A mixed string/number comparison stays a type error below.
	if lv.Kind == KindString && rv.Kind == KindString {
		switch op {
		case parser.OpLt:
			return BoolVal(lv.Str < rv.Str), nil
		case parser.OpGt:
			return BoolVal(lv.Str > rv.Str), nil
		case parser.OpLe:
			return BoolVal(lv.Str <= rv.Str), nil
		case parser.OpGe:
			return BoolVal(lv.Str >= rv.Str), nil
		}
	}
	if !lv.isNumeric() || !rv.isNumeric() {
		return Value{}, &runtimeError{Msg: fmt.Sprintf("operator %s needs two numbers or two strings, got %s and %s", op, lv.Kind, rv.Kind), File: file, Line: line, Col: col}
	}
	// Any remaining numeric combination is same-kind (handled above) or would
	// have matched a fast path; fall back to float compare for completeness.
	a, _ := lv.AsFloat()
	b, _ := rv.AsFloat()
	return i.orderResult(op, floatSign(a, b), file, line, col)
}

// orderResult turns the sign of (lhs - rhs) into the boolean an ordering
// operator asks for.
func (i *Interpreter) orderResult(op parser.BinaryOp, sign int, file string, line, col int) (Value, error) {
	switch op {
	case parser.OpLt:
		return BoolVal(sign < 0), nil
	case parser.OpGt:
		return BoolVal(sign > 0), nil
	case parser.OpLe:
		return BoolVal(sign <= 0), nil
	case parser.OpGe:
		return BoolVal(sign >= 0), nil
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown comparison %s", op), File: file, Line: line, Col: col}
}

func floatSign(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
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
			s, ovf := addOverflow(lv.Int, rv.Int)
			if ovf {
				return Value{}, &runtimeError{Msg: fmt.Sprintf("integer overflow: %d + %d", lv.Int, rv.Int), File: file, Line: line, Col: col}
			}
			return IntVal(s), nil
		case parser.OpSub:
			d, ovf := subOverflow(lv.Int, rv.Int)
			if ovf {
				return Value{}, &runtimeError{Msg: fmt.Sprintf("integer overflow: %d - %d", lv.Int, rv.Int), File: file, Line: line, Col: col}
			}
			return IntVal(d), nil
		case parser.OpMul:
			p, ovf := mulOverflow(lv.Int, rv.Int)
			if ovf {
				return Value{}, &runtimeError{Msg: fmt.Sprintf("integer overflow: %d * %d", lv.Int, rv.Int), File: file, Line: line, Col: col}
			}
			return IntVal(p), nil
		case parser.OpFloorDiv:
			if rv.Int == 0 {
				return Value{}, &runtimeError{Msg: "integer division by zero", File: file, Line: line, Col: col}
			}
			// MinInt64 / -1 overflows (its magnitude has no positive image).
			if lv.Int == minInt64 && rv.Int == -1 {
				return Value{}, &runtimeError{Msg: fmt.Sprintf("integer overflow: %d // %d", lv.Int, rv.Int), File: file, Line: line, Col: col}
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
			return IntVal(flooredMod(lv.Int, rv.Int)), nil
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
		return finiteFloatResult(a+b, a, b, op, file, line, col)
	case parser.OpSub:
		return finiteFloatResult(a-b, a, b, op, file, line, col)
	case parser.OpMul:
		return finiteFloatResult(a*b, a, b, op, file, line, col)
	case parser.OpDiv:
		if b == 0 {
			return Value{}, &runtimeError{Msg: "division by zero", File: file, Line: line, Col: col}
		}
		return finiteFloatResult(a/b, a, b, op, file, line, col)
	case parser.OpFloorDiv:
		if b == 0 {
			return Value{}, &runtimeError{Msg: "division by zero", File: file, Line: line, Col: col}
		}
		return finiteFloatResult(floorDiv(a, b), a, b, op, file, line, col)
	case parser.OpMod:
		return Value{}, &runtimeError{Msg: "operator % requires int operands, got float", File: file, Line: line, Col: col}
	}
	return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown binary operator %s", op), File: file, Line: line, Col: col}
}

// finiteFloatResult wraps a computed float result and enforces the language's
// strict "undefined results error" stance for the operator layer: an IEEE
// overflow to +/-Inf or a NaN is a positioned, catchable error rather than a
// value that silently enters program state (the same discipline int overflow,
// the `1e400` literal, and the math / convert boundaries already apply). The
// constant folder mirrors this (fold.go), so a folded and an unfolded overflow
// behave identically.
func finiteFloatResult(v, a, b float64, op parser.BinaryOp, file string, line, col int) (Value, error) {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("float overflow: %v %s %v produced a non-finite result", a, op, b),
			File: file, Line: line, Col: col,
		}
	}
	return FloatVal(v), nil
}

// int64 bounds as untyped constants, so the integer overflow helpers use no
// function call (math.MinInt64 / MaxInt64 would do), keeping the int fast path
// branch-only.
const (
	minInt64 = -1 << 63
	maxInt64 = 1<<63 - 1
)

// addOverflow / subOverflow / mulOverflow return their result and whether it
// overflowed int64, so integer arithmetic can raise a positioned error instead
// of silently wrapping (the language's "undefined results error" stance).
func addOverflow(a, b int64) (int64, bool) {
	s := a + b
	// Same-sign operands whose sum flips sign overflowed.
	if (a > 0 && b > 0 && s < 0) || (a < 0 && b < 0 && s >= 0) {
		return 0, true
	}
	return s, false
}

func subOverflow(a, b int64) (int64, bool) {
	d := a - b
	if (a >= 0 && b < 0 && d < 0) || (a < 0 && b > 0 && d >= 0) {
		return 0, true
	}
	return d, false
}

func mulOverflow(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	// MinInt64 * -1 has no positive image; guard before the p/a probe (which
	// would itself divide MinInt64 by -1 and panic).
	if (a == minInt64 && b == -1) || (b == minInt64 && a == -1) {
		return 0, true
	}
	p := a * b
	if p/a != b {
		return 0, true
	}
	return p, false
}

// flooredMod is the remainder consistent with floored `//`, so the identity
// (a // b) * b + (a % b) == a holds for negative operands (Python semantics).
// Callers guarantee b != 0.
func flooredMod(a, b int64) int64 {
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		r += b
	}
	return r
}

// floorDiv computes math.Floor(a/b) without importing math (TinyGo size).
// Equivalent to math.Floor(a / b) for finite, non-zero b.
func floorDiv(a, b float64) float64 {
	q := a / b
	// Round toward negative infinity. The intrinsic `math.Floor` is fine but
	// we avoid the import for TinyGo binary size. A magnitude at or beyond 2^63
	// (or NaN/Inf) has no fractional part and would overflow the int64
	// conversion below to platform-defined garbage; it already equals its own
	// floor, so return it directly.
	if q != q || q >= 9223372036854775808.0 || q <= -9223372036854775808.0 {
		return q
	}
	if q < 0 && q != float64(int64(q)) {
		return float64(int64(q) - 1)
	}
	return float64(int64(q))
}

// evalLen is the runtime side of the `len(EXPR)` language
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

// evalSpawn implements `spawn { ... }`. Phase 2 launches a
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
	var spawnEnv *Environment
	if i.prof != nil && i.profAllocs {
		start := time.Now()
		spawnEnv = i.snapshotForSpawn(env)
		file, line, col := posFor(ex)
		i.prof.RecordSpawnCopy(file, line, col, time.Since(start))
	} else {
		spawnEnv = i.snapshotForSpawn(env)
	}
	state := &TaskState{Done: make(chan struct{})}
	i.registerTask(state)

	// Point the spawn root's cancel flag at this task's Cancelled bit. Every
	// frame in the spawned goroutine reaches it via env.root, so a loop
	// checkpoint (loopCheckpoint) observes task.cancel cooperatively. spawnEnv is
	// the locals snapshot; its root is the globals snapshot both frames share.
	if spawnEnv.root != nil {
		spawnEnv.root.cancel = state
	}

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
	// Prune already-observed tasks (irrelevant to the exit-time loud-fail
	// scan) so a long-running spawner - a server that spawns per request /
	// accept - doesn't grow the registry without bound. The threshold grows
	// with the live-task count after each prune, keeping this amortized O(1)
	// per spawn even when few tasks are prunable.
	if len(i.tasks) >= i.taskCompactAt {
		kept := i.tasks[:0]
		for _, t := range i.tasks {
			// A task discarded while still running must stay registered:
			// hasLiveTasks relies on the registry to keep the REPL from
			// mutating shared tables under a live goroutine.
			if t != nil && (!t.Observed.Load() || !t.IsDone()) {
				kept = append(kept, t)
			}
		}
		i.tasks = kept
		i.taskCompactAt = 2*len(i.tasks) + 16
	}
	i.tasks = append(i.tasks, state)
	i.spawnTotal++
	i.tasksMu.Unlock()
}

// RequestDiagnostics asks the interpreter to print a one-shot diagnostic
// snapshot - the current source position and the spawned-task count - to Err at
// its next loop-iteration or method-call checkpoint, then keep running. It is
// safe to call from a signal-handler goroutine: it only stores an atomic flag;
// the snapshot itself runs on the interpreter goroutine. The CLI wires this to
// SIGUSR1 (`kill -USR1 <pid>`), so a program stuck in a loop can be asked
// "where?" without stopping it. If the interpreter is idle (not inside a loop or
// call), the snapshot appears at the next checkpoint reached.
func (i *Interpreter) RequestDiagnostics() { i.diagReq.Store(true) }

// dumpDiagnostics prints the snapshot and clears the request. Called from a
// checkpoint (on the interpreter goroutine) only when diagReq is set, so it reads
// interpreter state without a data race. The format is a fixed, documented block
// (see docs/libraries/os.md) - labeled lines between an `=== ... ===` header and
// a matching rule, on stderr, so it is human-readable and greppable:
//
//	=== jennifer diagnostics (SIGUSR1) ===
//	time:       2026-01-02T15:04:05Z07:00
//	executing:  path/to/file.j:42:5
//	tasks:      3 spawned, 2 live
//	goroutines: 6
//	memory:     heap 12.4 MiB (sys 68.0 MiB), 14 GCs
//	======================================
//
// loopCheckpoint is the per-iteration cooperative checkpoint shared by every
// loop kind (while / C-for / for-each / range-for / repeat). It services a
// pending diagnostics request (SIGUSR1) and observes task cancellation: inside a
// spawn body whose task has been cancelled it returns a catchable "task
// cancelled" runtime error so the loop - and the spawn - stops at this safe
// point. On the main goroutine env.root.cancel is nil, so this is one atomic-nil
// check per iteration and never fires. Placing the check only at loop
// checkpoints (not evalCall) keeps the call hot path untouched, matching where
// the diagnostics poll already lives; a non-terminating recursion with no loop is
// still bounded by the call-depth guard.
func (i *Interpreter) loopCheckpoint(env *Environment, node positioned) error {
	if i.diagReq.Load() {
		i.dumpDiagnostics(node)
	}
	// Inside a cancelled spawn body, raise a catchable "task cancelled" so the loop
	// stops at this safe point. The body catches it to exit cleanly with a partial
	// result; uncaught, it becomes the task's error (surfaced by task.wait). On the
	// main goroutine env.rootCancel() is nil, so this never fires.
	if ts := env.rootCancel(); ts != nil && ts.Cancelled.Load() {
		file, line, col := posOf(node)
		return &runtimeError{Msg: "task cancelled", File: file, Line: line, Col: col}
	}
	return nil
}

func (i *Interpreter) dumpDiagnostics(node positioned) {
	i.diagReq.Store(false)
	w := i.Err
	if w == nil {
		w = os.Stderr
	}
	file, line, col := posOf(node)

	i.tasksMu.Lock()
	spawned := i.spawnTotal
	live := 0
	for _, t := range i.tasks {
		if t != nil && !t.IsDone() {
			live++
		}
	}
	i.tasksMu.Unlock()

	// The runtime read (ReadMemStats stops the world briefly) is behind a
	// build-tag helper: fine for a rare, user-triggered dump, and TinyGo's
	// runtime.MemStats omits NumGC (the dump never runs there anyway).
	goroutines, heap, sys, numGC := runtimeSnapshot()

	const header = "=== jennifer diagnostics (SIGUSR1) ==="
	fmt.Fprintf(w, "\n%s\n", header)
	fmt.Fprintf(w, "time:       %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "executing:  %s:%d:%d\n", file, line, col)
	fmt.Fprintf(w, "tasks:      %d spawned, %d live\n", spawned, live)
	fmt.Fprintf(w, "goroutines: %d\n", goroutines)
	fmt.Fprintf(w, "memory:     heap %s (sys %s), %d GCs\n", mib(heap), mib(sys), numGC)
	fmt.Fprintln(w, strings.Repeat("=", len(header)))
}

// mib renders a byte count as a one-decimal MiB string for the diagnostics dump.
func mib(b uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(b)/(1024*1024))
}

// hasLiveTasks reports whether any spawned task is still running.
// Non-blocking. Used by EvalInteractive to refuse table-mutating REPL
// input while a task's goroutine may still be reading those tables.
func (i *Interpreter) hasLiveTasks() bool {
	i.tasksMu.Lock()
	defer i.tasksMu.Unlock()
	for _, s := range i.tasks {
		if !s.IsDone() {
			return true
		}
	}
	return false
}

// effectiveGlobal returns the env that should serve as the "global"
// parent for a fresh user-method call frame. In ordinary (single
// goroutine) execution this is i.global. Inside a spawned goroutine
// the caller's env chain terminates at the spawn snapshot (parent=nil
// by construction in snapshotForSpawn), so the outermost ancestor is
// the snapshot itself. Routing method-call frames through that
// snapshot - instead of the live i.global the parent goroutine is
// still mutating - is what makes spawn bodies that call user
// functions data-race-free. Cached as env.root at
// construction time, so this is an O(1) field read.
func effectiveGlobal(env *Environment) *Environment {
	if env == nil {
		return nil
	}
	if env.root != nil {
		return env.root
	}
	// Defensive fallback for envs constructed outside the pool / New*
	// paths (should not happen in shipping code but keeps hand-built
	// test fixtures working).
	cur := env
	for cur.parent != nil {
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
	// Snapshot the launching goroutine's OWN global frame, not the live
	// i.global. In serial code effectiveGlobal(env) is i.global; inside a
	// spawn body it is the enclosing spawn's detached global snapshot.
	// Either way it is a frame this goroutine owns and this call runs
	// synchronously on the launching goroutine, so iterating it here never
	// races a parent goroutine still writing i.global (the "concurrent map
	// iteration and map write" fatal a nested spawn would otherwise hit).
	// It is also more correct: a nested spawn captures its enclosing scope,
	// not the main goroutine's live globals.
	root := effectiveGlobal(env)
	globalSnap := NewEnvironment(nil)
	if root != nil {
		// Globals are slot-backed, so copyBindingsInto reconstructs their
		// name->value view from root.slots (plus any name-map fallback bindings).
		root.copyBindingsInto(globalSnap.vars)
	}
	localSnap := NewEnvironment(globalSnap)
	for cur := env; cur != nil && cur != root; cur = cur.parent {
		// Inner frames shadow outer ones: copyBindingsInto leaves a name already
		// captured by a nearer frame untouched.
		cur.copyBindingsInto(localSnap.vars)
	}
	return localSnap
}

// wrapTask builds the KindTask Value from a completed (or pending,
// post-Phase 2) TaskState. The element type is unknown at this point;
// the caller's Define / Assign check enforces it via MatchesDeclared.
func wrapTask(state *TaskState) Value {
	return Value{Kind: KindTask, Task: state}
}

// callUserMethod runs a resolved top-level method `m` with argument expressions
// `argExprs` evaluated in the caller's env. It backs the dynamic function-value
// call (evalCallValue); the static named-call path (evalCall) keeps a
// byte-identical copy of this body inline for hot-path inlining (see the note
// there - keep the two in lock-step). It checks arity, evaluates + type-checks +
// value-copies each argument into a fresh pooled call frame parented at the
// caller's effective globals, threads the caller's call-depth counter onto the
// callee frame (so the chain shares one counter and trips the catchable depth
// guard while concurrent chains stay isolated), runs the body, and maps the block
// result to a return value. `calleeName` and `node` supply the display name and
// positions for errors / the profiler.
func (i *Interpreter) callUserMethod(m *parser.MethodDef, argExprs []parser.Expr, env *Environment, calleeName string, node parser.Expr) (Value, error) {
	if len(argExprs) != len(m.Params) {
		file, line, col := posFor(node)
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("method %q takes %d argument(s), got %d", calleeName, len(m.Params), len(argExprs)),
			File: file, Line: line, Col: col,
		}
	}
	// Evaluate each argument in the caller's env and bind it straight into the
	// fresh call frame, interleaved so no intermediate []Value is allocated per
	// call. The call frame is borrowed from the pool and pre-sized to hold the N
	// parameter slots; its parent is globals - not the caller env - so a
	// partially-bound frame is never visible to arg evaluation.
	numParams := len(m.Params)
	callFrame := borrowBlockEnv(effectiveGlobal(env), numParams)
	borrowCtx := i.methodBorrowCtx(m)
	for idx, p := range m.Params {
		a := argExprs[idx]
		v, err := i.evalExpr(a, env)
		if err != nil {
			releaseBlockEnv(callFrame)
			return Value{}, err
		}
		if !v.MatchesDeclared(p.Type) {
			releaseBlockEnv(callFrame)
			file, line, col := posFor(a)
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("argument %d to %q must be %s, got %s", idx+1, calleeName, p.Type, v.Kind),
				File: file, Line: line, Col: col,
			}
		}
		// Value semantics: arguments copy into the call frame, so callee
		// mutations don't leak back to the caller. bindArg stamps the declared
		// parameter type (so compound params know their element / key+value type
		// for index-write checks) and skips Copy for scalar kinds (a no-op) and
		// for a borrowable parameter (a read-only alias - no copy happens, so no
		// eager-copy is recorded).
		bound := i.bindArg(v, p, borrowCtx)
		if !(p.Borrow && borrowCtx) && i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
			pf, pl, pcol := posFor(a)
			i.prof.RecordEagerCopy(pf, pl, pcol)
		}
		if err := callFrame.DefineAt(idx, p.Name, bound, p.Type, false); err != nil {
			releaseBlockEnv(callFrame)
			return Value{}, &runtimeError{Msg: err.Error(), Line: p.Line, Col: p.Col}
		}
	}
	// Call-depth guard. The counter is threaded from the caller frame onto the
	// callee frame (rather than read off effectiveGlobal's shared root), so a
	// chain shares one counter while concurrent chains stay isolated: parallel
	// `spawn` bodies and handlers dispatched into the shared host via
	// meta.callMain each track their own depth without racing. Raise a
	// positioned, catchable error before the Go goroutine stack overflows into a
	// fatal, unrecoverable crash - the analogue of a RecursionError.
	dc := env.depth
	callFrame.depth = dc
	*dc++
	if *dc > limits.MaxCallDepth {
		*dc--
		releaseBlockEnv(callFrame)
		file, line, col := posFor(node)
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("call stack too deep: exceeded %d nested method calls (possible infinite recursion)", limits.MaxCallDepth),
			File: file, Line: line, Col: col,
		}
	}
	if i.prof != nil && i.profStmts {
		pf, pl, pc := posFor(node)
		i.prof.RecordCallDepth(pf, pl, pc, *dc)
	}
	var res blockResult
	var err error
	if i.prof != nil && i.profCalls {
		start := time.Now()
		res, err = i.execBlock(m.Body, callFrame)
		pf, pl, pc := posFor(node)
		i.prof.RecordCall(m.Name, pf, pl, pc, start, time.Now())
	} else {
		res, err = i.execBlock(m.Body, callFrame)
	}
	*dc--
	releaseBlockEnv(callFrame)
	if err != nil {
		return Value{}, err
	}
	if res.hasBreak || res.hasContinue {
		// A `break` or `continue` in the method body that wasn't caught by an
		// inner loop is a misuse - they don't cross the method-call boundary into
		// the caller's loop.
		return Value{}, unhandledLoopFlowError(res)
	}
	if res.hasReturn {
		return res.value, nil
	}
	return Null(), nil
}

// evalCallValue runs a call through a function-valued expression (`$f(args)`,
// `$fns[0](x)`, `makeAdder(1)(2)`). The callee is evaluated to a value that must
// be a non-null KindFunc; dispatch then reuses callUserMethod, so arity / type
// checks, value-semantics arg copies, and the call-depth guard are identical to
// a named method call.
func (i *Interpreter) evalCallValue(c *parser.CallValueExpr, env *Environment) (Value, error) {
	fv, err := i.evalExpr(c.Callee, env)
	if err != nil {
		return Value{}, err
	}
	if fv.Kind != KindFunc {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{
			Msg:  fmt.Sprintf("cannot call a %s value; only a `func` value is callable", fv.Kind),
			File: file, Line: line, Col: col,
		}
	}
	if fv.Fn == nil {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{
			Msg:  "call of an uninitialized `func` value (it was never assigned a method)",
			File: file, Line: line, Col: col,
		}
	}
	return i.callUserMethod(fv.Fn, c.Args, env, fv.Fn.Name, c)
}

func (i *Interpreter) evalCall(c *parser.CallExpr, env *Environment) (Value, error) {
	// User method? Prefer the pre-resolved pointer the
	// resolver pass stamped onto the CallExpr; fall back to the
	// method-name map for resolver-less paths (REPL turns, tests
	// that hand-build ASTs).
	m := c.Method
	if m == nil {
		if hit, ok := i.methods[c.Callee]; ok {
			m = hit
		}
	}
	if m != nil {
		// The named-method dispatch body is kept INLINE here (rather than
		// delegating to callUserMethod) because evalCall is the hot path: the
		// helper is far too large for Go to inline, so routing every user-method
		// call through it adds a non-inlinable multi-argument call boundary that
		// measurably slows recursion-heavy code. The logic below is byte-identical
		// to callUserMethod (which serves the rarer dynamic `$f(args)` path); the
		// deliberate duplication trades a little repetition for the inlined hot
		// path. Keep the two in lock-step if either changes.
		if len(c.Args) != len(m.Params) {
			file, line, col := posFor(c)
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("method %q takes %d argument(s), got %d", c.Callee, len(m.Params), len(c.Args)),
				File: file, Line: line, Col: col,
			}
		}
		numParams := len(m.Params)
		callFrame := borrowBlockEnv(effectiveGlobal(env), numParams)
		borrowCtx := i.methodBorrowCtx(m)
		for idx, p := range m.Params {
			a := c.Args[idx]
			v, err := i.evalExpr(a, env)
			if err != nil {
				releaseBlockEnv(callFrame)
				return Value{}, err
			}
			if !v.MatchesDeclared(p.Type) {
				releaseBlockEnv(callFrame)
				file, line, col := posFor(a)
				return Value{}, &runtimeError{
					Msg:  fmt.Sprintf("argument %d to %q must be %s, got %s", idx+1, c.Callee, p.Type, v.Kind),
					File: file, Line: line, Col: col,
				}
			}
			bound := i.bindArg(v, p, borrowCtx)
			if !(p.Borrow && borrowCtx) && i.prof != nil && i.profAllocs && isCompoundCopyKind(v.Kind) {
				pf, pl, pcol := posFor(a)
				i.prof.RecordEagerCopy(pf, pl, pcol)
			}
			if err := callFrame.DefineAt(idx, p.Name, bound, p.Type, false); err != nil {
				releaseBlockEnv(callFrame)
				return Value{}, &runtimeError{Msg: err.Error(), Line: p.Line, Col: p.Col}
			}
		}
		dc := env.depth
		callFrame.depth = dc
		*dc++
		if *dc > limits.MaxCallDepth {
			*dc--
			releaseBlockEnv(callFrame)
			file, line, col := posFor(c)
			return Value{}, &runtimeError{
				Msg:  fmt.Sprintf("call stack too deep: exceeded %d nested method calls (possible infinite recursion)", limits.MaxCallDepth),
				File: file, Line: line, Col: col,
			}
		}
		if i.prof != nil && i.profStmts {
			pf, pl, pc := posFor(c)
			i.prof.RecordCallDepth(pf, pl, pc, *dc)
		}
		var res blockResult
		var err error
		if i.prof != nil && i.profCalls {
			start := time.Now()
			res, err = i.execBlock(m.Body, callFrame)
			pf, pl, pc := posFor(c)
			i.prof.RecordCall(m.Name, pf, pl, pc, start, time.Now())
		} else {
			res, err = i.execBlock(m.Body, callFrame)
		}
		*dc--
		releaseBlockEnv(callFrame)
		if err != nil {
			return Value{}, err
		}
		if res.hasBreak || res.hasContinue {
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
		pf, pl, pc := posFor(c)
		ctx := BuiltinCtx{Out: i.Out, Err: i.Err, In: i.In, InREPL: i.InREPL, File: pf, Line: pl, Col: pc, Depth: env.depth, interp: i, Cancel: env.rootCancel()}
		v, err := b.Fn(ctx, args)
		if err != nil {
			return Value{}, builtinError(err, pf, pl, pc)
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
// isNamespaceOrAlias reports whether prefix currently names an imported module
// alias or an active library namespace. Used to settle the `NS.Name{...}`
// ambiguity in favour of the namespace when a local enum shares its name.
func (i *Interpreter) isNamespaceOrAlias(prefix string) bool {
	if _, isModule := i.moduleAliases[prefix]; isModule {
		return true
	}
	_, isNS := i.nsPrefixes[prefix]
	return isNS
}

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
	if _, isModule := i.moduleAliases[prefix]; isModule {
		// The prefix names an imported module. Module functions and
		// constants (`prefix.fn(...)`, `prefix.CONST`) resolve elsewhere;
		// reaching here means it was used as a struct-type prefix
		// (`prefix.Type` / `prefix.Type{...}`), which is not yet available -
		// call a module function that returns the value instead.
		return "", fmt.Errorf("module %q struct types are not available yet; call a `%s.` function that returns the value", prefix, prefix)
	}
	return "", fmt.Errorf("unknown namespace %q", prefix)
}

// evalQualifiedCall handles `prefix.callee(args)`. The prefix is
// resolved to a namespace (alias-aware), then the (namespace, callee)
// pair is looked up in the namespaced-builtin registry.
func (i *Interpreter) evalQualifiedCall(c *parser.QualifiedCallExpr, env *Environment) (Value, error) {
	// Pre-resolved module-alias method stamped by resolveQualifiedRefs: dispatch
	// straight through, skipping the moduleAliases lookup + existence / export
	// checks (already done at stamp time). The Builtin fast path below handles
	// the library case; both fall back to the slow paths when unstamped.
	if mt, ok := c.Fn.(moduleMethodTarget); ok {
		return i.dispatchModuleMethod(mt.mod, mt.method, c, env)
	}
	// A module alias resolves into a loaded module's own interpreter, not a
	// library namespace. Aliases are collision-checked against library
	// prefixes at bind time, so a prefix is one or the other, never both.
	if m, ok := i.moduleAliases[c.Prefix]; ok {
		return i.callModuleMethod(m, c, env)
	}
	// prefer the pre-resolved Builtin pointer stamped by
	// resolveQualifiedRefs. Falls back to the resolveNamespacePrefix
	// + NSBuiltins path for resolver-less callers (REPL, hand-built
	// ASTs, prefixes that weren't valid at resolve time).
	var fn Builtin
	if c.Fn != nil {
		if hit, ok := c.Fn.(Builtin); ok {
			fn = hit
		}
	}
	if fn == nil {
		ns, err := i.resolveNamespacePrefix(c.Prefix)
		if err != nil {
			file, line, col := posFor(c)
			return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
		}
		hit, ok := i.NSBuiltins[nsKey{NS: ns, Name: c.Callee}]
		if !ok {
			file, line, col := posFor(c)
			return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown function %q in namespace %q", c.Callee, ns), File: file, Line: line, Col: col}
		}
		fn = hit
	}
	args := make([]Value, 0, len(c.Args))
	for _, a := range c.Args {
		v, err := i.evalExpr(a, env)
		if err != nil {
			return Value{}, err
		}
		args = append(args, v)
	}
	pf, pl, pc := posFor(c)
	ctx := BuiltinCtx{Out: i.Out, Err: i.Err, In: i.In, InREPL: i.InREPL, File: pf, Line: pl, Col: pc, Depth: env.depth, interp: i, Cancel: env.rootCancel()}
	v, err := fn(ctx, args)
	if err != nil {
		return Value{}, builtinError(err, pf, pl, pc)
	}
	return v, nil
}

// builtinError normalizes an error returned by a builtin. Control-flow
// signals - a thrown Jennifer error (*ErrorSignal) or an exit (*ExitSignal) -
// propagate unwrapped, so a Go builtin can raise a catchable Jennifer error
// (testing assertions, RaiseError). Any other Go error is wrapped into a
// positioned runtimeError at the call site, the long-standing behavior.
func builtinError(err error, file string, line, col int) error {
	switch err.(type) {
	case *ErrorSignal, *ExitSignal:
		return err
	}
	return &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
}

// evalQualifiedConst handles `prefix.NAME`. Resolution mirrors
// evalQualifiedCall; the result is the constant's value.
func (i *Interpreter) evalQualifiedConst(c *parser.QualifiedConstRefExpr) (Value, error) {
	// Prefer the pre-resolved Value stamped by resolveQualifiedRefs - a library
	// OR module-alias const. It is immutable / deep-const and store sites copy,
	// so returning the shared value is safe. This is the O(1) hot path; both the
	// library namespace lookup and the module GetBinding slot scan below are
	// fallbacks reached only for an unstamped ref (REPL, or a missing / private
	// name whose proper error the fallback still produces).
	if c.Const != nil {
		if v, ok := c.Const.(Value); ok {
			return v, nil
		}
	}
	// `Enum.Variant` where the variant name is constant-shaped (e.g. `A`, `OK`)
	// lexes as a qualified constant reference; if the prefix is a local enum it
	// is a payload-less variant construction.
	if ed, ok := i.enums[c.Prefix]; ok {
		for vi := range ed.Variants {
			if ed.Variants[vi].Name == c.Name {
				if len(ed.Variants[vi].Fields) > 0 {
					file, line, col := posFor(c)
					return Value{}, &runtimeError{Msg: fmt.Sprintf("variant %s.%s carries a payload; construct it as %s.%s{ ... }", ed.Name, c.Name, ed.Name, c.Name), File: file, Line: line, Col: col}
				}
				return EnumVal("", "", ed.Name, c.Name, nil), nil
			}
		}
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: fmt.Sprintf("enum %s has no variant %q", ed.Name, c.Name), File: file, Line: line, Col: col}
	}
	// A module alias reads a constant from the loaded module's own scope.
	if m, ok := i.moduleAliases[c.Prefix]; ok {
		return i.moduleConst(m, c)
	}
	ns, err := i.resolveNamespacePrefix(c.Prefix)
	if err != nil {
		file, line, col := posFor(c)
		return Value{}, &runtimeError{Msg: err.Error(), File: file, Line: line, Col: col}
	}
	v, ok := i.NSConstants[nsKey{NS: ns, Name: c.Name}]
	if !ok {
		file, line, col := posFor(c)
		// A name that is a function in the namespace was likely meant as a call
		// - point at the missing `()` rather than reporting a phantom constant.
		if _, isFn := i.NSBuiltins[nsKey{NS: ns, Name: c.Name}]; isFn {
			return Value{}, &runtimeError{Msg: fmt.Sprintf("%s.%s is a function; call it as %s.%s(...)", c.Prefix, c.Name, c.Prefix, c.Name), File: file, Line: line, Col: col}
		}
		return Value{}, &runtimeError{Msg: fmt.Sprintf("unknown constant %q in namespace %q", c.Name, ns), File: file, Line: line, Col: col}
	}
	return v, nil
}

// resolveQualifiedRefs is the second resolver pass. Runs from
// Interpreter.Run after processImports has populated the namespace /
// alias / import tables, walks the AST once, and pre-fills
// QualifiedCallExpr.Fn / QualifiedConstRefExpr.Const with the exact
// Builtin / Value the interpreter would otherwise look up on every
// call. Unresolvable prefixes (bad alias, unimported namespace,
// unknown callee) stay nil - the runtime fallback path handles them
// with the original positioned error messages.
//
// Idempotent: re-running on an already-annotated AST just replaces
// the pointers with the same values.
func (i *Interpreter) resolveQualifiedRefs(prog *parser.Program) {
	if prog == nil {
		return
	}
	for _, s := range prog.TopLevel {
		i.walkStmtForQualifiedRefs(s)
	}
	for _, m := range prog.Methods {
		if m == nil || m.Body == nil {
			continue
		}
		for _, s := range m.Body.Stmts {
			i.walkStmtForQualifiedRefs(s)
		}
	}
}

func (i *Interpreter) walkStmtForQualifiedRefs(s parser.Stmt) {
	switch st := s.(type) {
	case *parser.DefineStmt:
		i.walkExprForQualifiedRefs(st.InitExpr)
	case *parser.AssignStmt:
		i.walkExprForQualifiedRefs(st.Value)
	case *parser.IndexAssignStmt:
		i.walkExprForQualifiedRefs(st.Target)
		i.walkExprForQualifiedRefs(st.Value)
	case *parser.AppendStmt:
		i.walkExprForQualifiedRefs(st.Target)
		i.walkExprForQualifiedRefs(st.Value)
	case *parser.FieldAssignStmt:
		i.walkExprForQualifiedRefs(st.Target)
		i.walkExprForQualifiedRefs(st.Value)
	case *parser.IfStmt:
		i.walkExprForQualifiedRefs(st.Cond)
		i.walkBlockForQualifiedRefs(st.Then)
		for idx := range st.ElseIfs {
			i.walkExprForQualifiedRefs(st.ElseIfs[idx])
			i.walkBlockForQualifiedRefs(st.ElseIfBodies[idx])
		}
		i.walkBlockForQualifiedRefs(st.Else)
	case *parser.WhileStmt:
		i.walkExprForQualifiedRefs(st.Cond)
		i.walkBlockForQualifiedRefs(st.Body)
	case *parser.ForStmt:
		i.walkStmtForQualifiedRefs(st.Init)
		i.walkExprForQualifiedRefs(st.Cond)
		i.walkStmtForQualifiedRefs(st.Step)
		i.walkBlockForQualifiedRefs(st.Body)
	case *parser.ForEachStmt:
		i.walkExprForQualifiedRefs(st.Coll)
		i.walkBlockForQualifiedRefs(st.Body)
	case *parser.RepeatStmt:
		i.walkBlockForQualifiedRefs(st.Body)
		i.walkExprForQualifiedRefs(st.Cond)
	case *parser.ReturnStmt:
		i.walkExprForQualifiedRefs(st.Value)
	case *parser.ExitStmt:
		i.walkExprForQualifiedRefs(st.Code)
	case *parser.ThrowStmt:
		i.walkExprForQualifiedRefs(st.Value)
	case *parser.TryStmt:
		i.walkBlockForQualifiedRefs(st.Body)
		i.walkBlockForQualifiedRefs(st.CatchBody)
	case *parser.ExprStmt:
		i.walkExprForQualifiedRefs(st.Expr)
	case *parser.Block:
		i.walkBlockForQualifiedRefs(st)
	}
}

func (i *Interpreter) walkBlockForQualifiedRefs(b *parser.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		i.walkStmtForQualifiedRefs(s)
	}
}

func (i *Interpreter) walkExprForQualifiedRefs(e parser.Expr) {
	if e == nil {
		return
	}
	switch ex := e.(type) {
	case *parser.QualifiedCallExpr:
		if ns, ok := i.nsPrefixes[ex.Prefix]; ok {
			if fn, hit := i.NSBuiltins[nsKey{NS: ns, Name: ex.Callee}]; hit {
				ex.Fn = fn
			}
		} else if m, ok := i.moduleAliases[ex.Prefix]; ok {
			// Module-alias method: stamp the resolved (module, *MethodDef) so
			// evalQualifiedCall dispatches straight through dispatchModuleMethod,
			// skipping the per-call prefix / method-existence / export lookups.
			// Left unstamped when the name is missing or private, so the runtime
			// path still raises the proper "no method" / "not exported" error.
			if md, found := m.interp.methods[ex.Callee]; found && m.exports[ex.Callee] {
				ex.Fn = moduleMethodTarget{mod: m, method: md}
			}
		}
		for _, a := range ex.Args {
			i.walkExprForQualifiedRefs(a)
		}
	case *parser.QualifiedConstRefExpr:
		if ns, ok := i.nsPrefixes[ex.Prefix]; ok {
			if v, hit := i.NSConstants[nsKey{NS: ns, Name: ex.Name}]; hit {
				ex.Const = v
			}
		} else if m, ok := i.moduleAliases[ex.Prefix]; ok {
			// Module-alias constant: stamp the exported const's
			// boundary-retagged, deep-const value so evalQualifiedConst returns
			// it directly, skipping the per-access GetBinding slot scan +
			// export check + retag. Left unstamped (nil) when the name is
			// missing or private, so the runtime path still raises the proper
			// "no constant" / "not exported" error. Safe to cache: a module is
			// run-once, its consts are deep-const, and retagStructs is a pure
			// transform over stable module identity.
			if b, err := m.interp.global.GetBinding(ex.Name); err == nil && b.IsConst && m.exports[ex.Name] {
				ex.Const = retagStructs(b.Value, "", m.ns, "", m.path, m.isOwnStruct)
			}
		}
	case *parser.CallExpr:
		for _, a := range ex.Args {
			i.walkExprForQualifiedRefs(a)
		}
	case *parser.BinaryExpr:
		i.walkExprForQualifiedRefs(ex.Left)
		i.walkExprForQualifiedRefs(ex.Right)
	case *parser.UnaryExpr:
		i.walkExprForQualifiedRefs(ex.Operand)
	case *parser.LenExpr:
		i.walkExprForQualifiedRefs(ex.Operand)
	case *parser.IndexExpr:
		i.walkExprForQualifiedRefs(ex.Target)
		i.walkExprForQualifiedRefs(ex.Index)
	case *parser.FieldAccessExpr:
		i.walkExprForQualifiedRefs(ex.Target)
	case *parser.ListLit:
		for _, el := range ex.Elements {
			i.walkExprForQualifiedRefs(el)
		}
	case *parser.MapLit:
		for k := range ex.Keys {
			i.walkExprForQualifiedRefs(ex.Keys[k])
			i.walkExprForQualifiedRefs(ex.Values[k])
		}
	case *parser.StructLit:
		for _, f := range ex.Fields {
			i.walkExprForQualifiedRefs(f.Expr)
		}
	case *parser.SpawnExpr:
		for _, s := range ex.Body {
			i.walkStmtForQualifiedRefs(s)
		}
	case *parser.CallValueExpr:
		i.walkExprForQualifiedRefs(ex.Callee)
		for _, a := range ex.Args {
			i.walkExprForQualifiedRefs(a)
		}
	case *parser.RangeExpr:
		i.walkExprForQualifiedRefs(ex.Lo)
		i.walkExprForQualifiedRefs(ex.Hi)
	case *parser.SliceExpr:
		i.walkExprForQualifiedRefs(ex.Target)
		i.walkExprForQualifiedRefs(ex.Lo)
		i.walkExprForQualifiedRefs(ex.Hi)
	case *parser.InterpStringExpr:
		// A `{expr}` interpolation slot is real code: pre-stamp any qualified
		// call / const inside it (`"{io.printf(...)}"`, `{m.CONST}`) so it takes
		// the O(1) path like the same expression anywhere else.
		for p := range ex.Parts {
			i.walkExprForQualifiedRefs(ex.Parts[p].Expr)
		}
	}
}
