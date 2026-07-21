// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"fmt"
	"sync"
	"time"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// envPool recycles short-lived Environment frames (block bodies + method
// call frames). Jennifer has no closures and no first-class functions,
// so every frame's lifetime ends deterministically when the block /
// call returns - the interpreter's execBlock, evalCall, and CallByName
// borrow an env on entry and return it on the way out. sync.Pool is
// per-P internally, so contention across the goroutine boundaries
// `spawn` introduces stays local.
//
// The New() closure returns an env with an already-allocated vars map;
// borrowBlockEnv resets slots per request (either re-slicing the
// existing backing array or growing it when NumSlots demands more).
// releaseBlockEnv clears the vars map and truncates the slot slice so
// the next Get sees a clean frame. Frames escape the pool through
// snapshotForSpawn's deep-copy path, so post-spawn parent-goroutine
// resets don't reach the goroutine's captured state.
var envPool = sync.Pool{
	New: func() any {
		return &Environment{vars: make(map[string]Binding)}
	},
}

// borrowBlockEnv borrows an Environment from the pool for a fresh
// frame with the given parent and slot count. Callers MUST release via
// releaseBlockEnv when the frame's dynamic extent ends (deferred at
// the callsite covers the error / signal paths).
func borrowBlockEnv(parent *Environment, numSlots int) *Environment {
	e := envPool.Get().(*Environment)
	e.parent = parent
	e.root = rootFor(parent, e)
	// releaseBlockEnv zeroes every used slot before returning the env
	// to the pool, so the backing array's [0, cap) range is Binding{}
	// on entry - we just re-slice to the requested length. When
	// numSlots exceeds the retained capacity we allocate a fresh
	// backing.
	if numSlots > 0 {
		if cap(e.slots) >= numSlots {
			e.slots = e.slots[:numSlots]
		} else {
			e.slots = make([]Binding, numSlots)
		}
	} else {
		e.slots = e.slots[:0]
	}
	return e
}

// releaseBlockEnv returns a previously-borrowed Environment to the
// pool. Only safe to call when no code retains a pointer to `e` - the
// interpreter's block frames satisfy this because Jennifer has no
// closure form that could capture the env. Slot entries are zeroed
// so the pool doesn't hold compound-value backings live between uses.
func releaseBlockEnv(e *Environment) {
	for k := range e.vars {
		delete(e.vars, k)
	}
	for i := range e.slots {
		e.slots[i] = Binding{}
	}
	e.parent = nil
	e.root = nil
	e.profChild = 0
	e.slots = e.slots[:0]
	// Drop any deferred calls so a pooled frame never carries a stale one into
	// its next use. finishFrame runs them before release, so this is normally
	// already empty; nil it out to release the backing for GC.
	e.deferred = nil
	envPool.Put(e)
}

// Binding is one entry in an Environment frame: the current value plus the
// declared static type and whether it's a constant. Slot lets
// name-based writes (Assign, execAppend / execIndexAssign /
// execFieldAssign via GetBinding + Assign) can mirror into the
// slot-indexed storage after finding the binding by name. -1 means
// "no slot mirror" (resolver-less path, REPL, tests).
type Binding struct {
	Value    Value
	DeclType parser.Type
	IsConst  bool
	Slot     int
}

// Environment is a lexically-scoped symbol table.
// Parent chains form the scope stack; Define adds to the current frame only.
// Lookups walk outward. The spec forbids shadowing, so Define returns an error
// if any visible parent already binds the name.
//
// After Resolve() runs on the AST, every variable reference
// carries a (Depth, Slot) coordinate the runtime can use to skip the
// name-map walk. The slot-indexed storage lives in `slots` alongside
// the name map; DefineAt / GetAt / AssignAt operate on it. The name
// map remains as the fallback path for the REPL (which spans multiple
// resolver-less parses) and for any test that hand-builds AST
// fragments without slot annotations.
type Environment struct {
	parent *Environment
	vars   map[string]Binding
	slots  []Binding
	// cached pointer to the outermost ancestor of this
	// environment (the "root" - either the interpreter's global env
	// or a spawn snapshot's globals frame). Set once at construction
	// and inherited from the parent, so effectiveGlobal() becomes an
	// O(1) field read instead of an O(depth) parent-chain walk.
	root *Environment
	// profChild accumulates nested-statement wall-clock time for the
	// statement profiler's self/cumulative split. It lives on the root
	// env, not the Interpreter, so each `spawn` goroutine (which gets its
	// own snapshot root) accumulates independently - a shared field on the
	// interpreter would be raced by parallel spawn bodies. Only ever
	// touched through env.root while statement profiling is active.
	profChild time.Duration
	// callDepth counts the Jennifer method calls currently nested on the Go
	// stack. Like profChild it lives on the per-goroutine root env (i.global
	// or a spawn snapshot root), so parallel `spawn` bodies each track their
	// own depth without racing a shared Interpreter field. evalCall bumps it
	// on entry and drops it on exit, raising a catchable error at
	// limits.MaxCallDepth before the Go goroutine stack overflows fatally.
	// Only ever touched through effectiveGlobal(env).
	callDepth int
	// deferred holds this frame's `defer`red calls, appended as each
	// DeferStmt runs and executed LIFO by finishFrame when the frame exits
	// (see execDefer / finishFrame). nil for the overwhelming majority of
	// frames (no defer), so the frame-exit check is a cheap len == 0 test.
	deferred []deferredCall
}

// rootFor computes the root marker for a fresh Environment. When
// parent is nil the env IS the root (globals frame or a spawn
// globals snapshot); otherwise inherit whatever the parent's root
// points at.
func rootFor(parent, self *Environment) *Environment {
	if parent == nil {
		return self
	}
	if parent.root != nil {
		return parent.root
	}
	return parent
}

func NewEnvironment(parent *Environment) *Environment {
	env := &Environment{
		parent: parent,
		vars:   make(map[string]Binding),
	}
	env.root = rootFor(parent, env)
	return env
}

// NewEnvironmentSized creates an environment pre-sized to hold
// numSlots slot-indexed bindings, plus the fallback name map. Called
// from execBlock when Block.NumSlots is nonzero (i.e., the resolver
// pre-computed the slot count).
func NewEnvironmentSized(parent *Environment, numSlots int) *Environment {
	env := &Environment{
		parent: parent,
		vars:   make(map[string]Binding),
	}
	env.root = rootFor(parent, env)
	if numSlots > 0 {
		env.slots = make([]Binding, numSlots)
	}
	return env
}

// DefineAt records a binding at the given slot index in the current
// frame. Also mirrors into the name map so name-based lookups (REPL,
// GetBinding by name, tests) keep working. Returns the same no-shadow
// error as Define when the name collides with an enclosing binding,
// though for resolver-generated slot writes that check has already
// happened at parse time.
func (e *Environment) DefineAt(slot int, name string, val Value, declType parser.Type, isConst bool) error {
	// The resolver-verified slot path (slot >= 0) already rejected shadowing
	// at parse time, so skip the O(depth) enclosing-scope walk here; only the
	// name-based fallback (slot < 0: REPL, hand-built ASTs) still checks.
	if slot < 0 && e.existsInChain(name) {
		return fmt.Errorf("name %q is already defined in an enclosing scope", name)
	}
	b := Binding{Value: val, DeclType: declType, IsConst: isConst, Slot: slot}
	e.vars[name] = b
	if slot >= 0 {
		if slot >= len(e.slots) {
			// Grow to fit. NumSlots hint from the resolver should
			// have made this unnecessary, but the safety net keeps
			// resolver-less code paths (REPL, some tests) working.
			grown := make([]Binding, slot+1)
			copy(grown, e.slots)
			e.slots = grown
		}
		e.slots[slot] = b
	}
	return nil
}

// GetAt reads a binding by (depth, slot). Walks depth parent pointers
// then indexes into slots. Returns the fallback name-based Get error
// on a bad address so the runtime error text stays uniform.
func (e *Environment) GetAt(depth, slot int, name string) (Value, error) {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return Value{}, fmt.Errorf("undefined variable %q", name)
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		// Slot outside range: fall back to name lookup at this depth
		// (covers method-body defs added at runtime by test paths).
		if b, ok := cur.vars[name]; ok {
			return b.Value, nil
		}
		return Value{}, fmt.Errorf("undefined variable %q", name)
	}
	return cur.slots[slot].Value, nil
}

// GetBindingAt is the metadata companion to GetAt.
func (e *Environment) GetBindingAt(depth, slot int, name string) (Binding, error) {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return Binding{}, fmt.Errorf("undefined %q", name)
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		if b, ok := cur.vars[name]; ok {
			return b, nil
		}
		return Binding{}, fmt.Errorf("undefined %q", name)
	}
	return cur.slots[slot], nil
}

// getBindingRoot / assignRoot fetch and write the binding named by a resolved
// root VarExpr - the target of `$x[i] = ...`, `$x[] = ...`, `$x.f = ...`. They
// take the (Depth, Slot) fast path when the resolver stamped it (no name-map
// hash per mutation), and fall back to the chain-walking name path otherwise
// (REPL / hand-built ASTs, where Slot is -1). GetBindingAt / AssignAt only
// consult the frame at Depth on a slot miss, so the guard on Slot >= 0 is what
// keeps the unresolved path (where the binding may live in an enclosing frame)
// correct.
func (e *Environment) getBindingRoot(v *parser.VarExpr) (Binding, error) {
	if v.Slot >= 0 {
		return e.GetBindingAt(v.Depth, v.Slot, v.Name)
	}
	return e.GetBinding(v.Name)
}

func (e *Environment) assignRoot(v *parser.VarExpr, val Value) error {
	if v.Slot >= 0 {
		return e.AssignAt(v.Depth, v.Slot, v.Name, val)
	}
	return e.Assign(v.Name, val)
}

// AssignAt writes a new value to the binding at (depth, slot). Const
// and type-mismatch checks match the name-based Assign path.
func (e *Environment) AssignAt(depth, slot int, name string, val Value) error {
	cur := e
	for d := 0; d < depth; d++ {
		if cur.parent == nil {
			return fmt.Errorf("undefined variable %q", name)
		}
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		// Fall back to the name path.
		return e.Assign(name, val)
	}
	b := cur.slots[slot]
	if b.IsConst {
		return fmt.Errorf("cannot assign to constant %q", name)
	}
	if !val.MatchesDeclared(b.DeclType) {
		return fmt.Errorf("cannot assign %s to %s variable %q", val.Kind, b.DeclType, name)
	}
	b.Value = val
	cur.slots[slot] = b
	// The slot is authoritative for a slot-backed binding; name-based readers
	// (Get / GetBinding / snapshotForSpawn) consult the slot when Slot >= 0, so
	// we no longer mirror the whole Binding into cur.vars on this hot write
	// path (every `$i = $i + 1;` in a resolved loop). The vars entry from
	// DefineAt stays as the name -> (Slot, metadata) record.
	return nil
}

// Define introduces a new binding in the current frame.
// Returns an error if the name already exists in this frame or any enclosing
// scope (spec: lower scopes may not overwrite existing bindings).
func (e *Environment) Define(name string, val Value, declType parser.Type, isConst bool) error {
	if e.existsInChain(name) {
		return fmt.Errorf("name %q is already defined in an enclosing scope", name)
	}
	e.vars[name] = Binding{Value: val, DeclType: declType, IsConst: isConst, Slot: -1}
	return nil
}

// Assign updates an existing binding, walking up the parent chain to find it.
// Errors if the name is undefined, refers to a constant, or the new value's
// kind doesn't match the declared type. When the binding was
// installed via DefineAt (Slot >= 0), mirror the write into
// cur.slots[Slot] so a subsequent GetAt sees the update.
func (e *Environment) Assign(name string, val Value) error {
	for cur := e; cur != nil; cur = cur.parent {
		if b, ok := cur.vars[name]; ok {
			if b.IsConst {
				return fmt.Errorf("cannot assign to constant %q", name)
			}
			if !val.MatchesDeclared(b.DeclType) {
				return fmt.Errorf("cannot assign %s to %s variable %q", val.Kind, b.DeclType, name)
			}
			b.Value = val
			cur.vars[name] = b
			if b.Slot >= 0 && b.Slot < len(cur.slots) {
				cur.slots[b.Slot] = b
			}
			return nil
		}
	}
	return fmt.Errorf("undefined variable %q", name)
}

// Get looks up a name, walking outward.
func (e *Environment) Get(name string) (Value, error) {
	for cur := e; cur != nil; cur = cur.parent {
		if b, ok := cur.vars[name]; ok {
			// The slot is authoritative for a slot-backed binding (AssignAt no
			// longer mirrors writes into vars).
			if b.Slot >= 0 && b.Slot < len(cur.slots) {
				return cur.slots[b.Slot].Value, nil
			}
			return b.Value, nil
		}
	}
	return Value{}, fmt.Errorf("undefined variable %q", name)
}

// GetBinding looks up a binding (value + metadata) by name, walking outward.
// Used by callers that need to distinguish constants from variables.
func (e *Environment) GetBinding(name string) (Binding, error) {
	for cur := e; cur != nil; cur = cur.parent {
		if b, ok := cur.vars[name]; ok {
			// Fill the current value from the authoritative slot when this is a
			// slot-backed binding (the vars copy's Value may be stale).
			if b.Slot >= 0 && b.Slot < len(cur.slots) {
				b.Value = cur.slots[b.Slot].Value
			}
			return b, nil
		}
	}
	return Binding{}, fmt.Errorf("undefined %q", name)
}

// slotValue returns the authoritative current value of a binding taken from a
// frame's vars map: the slot value when it is slot-backed (AssignAt writes only
// the slot), otherwise the vars copy. Used where a name-based iteration of vars
// needs live values (the spawn snapshot).
func slotValue(frame *Environment, b Binding) Value {
	if b.Slot >= 0 && b.Slot < len(frame.slots) {
		return frame.slots[b.Slot].Value
	}
	return b.Value
}

func (e *Environment) existsInChain(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			return true
		}
	}
	return false
}
